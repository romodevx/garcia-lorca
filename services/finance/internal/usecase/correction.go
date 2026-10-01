package usecase

import (
	"context"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// CorrectionInput is a correction as the office asks for it (T9): the deposit
// to correct and what it should have been — amount, day, methods, and which
// charges it may pay — with the reason, which is required.
type CorrectionInput struct {
	DepositID   string
	AmountCents int64
	// PaidAt is the corrected day the money changed hands; zero keeps the
	// original's. A future date is refused, as for any deposit.
	PaidAt time.Time
	// Tenders are 1–4 methods adding up exactly to AmountCents (ADR 0025).
	Tenders []TenderInput
	// ChargeIDs restrict what the money may pay; empty is oldest debt first.
	ChargeIDs      []string
	Reason         string
	IdempotencyKey string
	// By is the authenticated caller, set by the gateway.
	By domain.Receiver
}

// CorrectionResult is what a correction did: the original as it now stands
// (superseded), the corrected deposit and its split, and the history row.
type CorrectionResult struct {
	Original   domain.Deposit
	Corrected  DepositResult
	Correction domain.DepositCorrection
	Replayed   bool
}

// CorrectDeposit corrects a registered payment (T9).
//
// The money is not edited in place. The original deposit is superseded — kept,
// with its folio, counting for nothing — and a new deposit with its own folio
// carries the corrected figures. The new one is built and split exactly as a
// deposit taken at the counter (prepareDeposit + the planner), on the corrected
// date, so the surcharge, the early-payment reward and the inscription price
// are worked out again by the same arithmetic rather than patched. Who took the
// money stays the original's cashier; who corrected it, when and why go to the
// history. The family is told the corrected amount (PaymentCorrected).
func (s *FinanceService) CorrectDeposit(ctx context.Context, in CorrectionInput) (CorrectionResult, error) {
	if err := domain.ValidateCorrectionReason(in.Reason); err != nil {
		return CorrectionResult{}, err
	}
	if len(in.Tenders) == 0 {
		// Always explicit: a correction states how the money came in, it does
		// not inherit a single method from the deposit it replaces.
		return CorrectionResult{}, glerrors.New(glerrors.KindInvalid, "falta la forma de pago")
	}
	original, err := s.repo.GetDeposit(ctx, in.DepositID)
	if err != nil {
		return CorrectionResult{}, err
	}
	paidAt := in.PaidAt
	if paidAt.IsZero() {
		paidAt = original.PaidAt()
	}
	deposit, planner, err := s.prepareDeposit(ctx, DepositInput{
		StudentID: original.StudentID(), AmountCents: in.AmountCents, PaidAt: paidAt,
		IdempotencyKey: in.IdempotencyKey, ChargeIDs: in.ChargeIDs,
		Receiver: original.ReceivedBy(), Tenders: in.Tenders,
	})
	if err != nil {
		return CorrectionResult{}, err
	}
	record, replayed, err := s.repo.CorrectDeposit(ctx, ports.DepositCorrectionWrite{
		OriginalID:   original.ID(),
		Corrected:    deposit.WithCorrects(original.ID(), original.Folio()),
		CorrectionID: s.newID(),
		Reason:       in.Reason,
		By:           in.By,
		At:           s.now(),
	}, planner.plan, s.correctionEvents)
	if err != nil {
		return CorrectionResult{}, err
	}
	if !replayed {
		s.drainQuietly(ctx)
	}
	return CorrectionResult{
		Original: record.Original,
		Corrected: DepositResult{
			Deposit:          record.Corrected.Deposit,
			Surcharges:       record.Corrected.Surcharges,
			Allocations:      record.Corrected.Allocations,
			UnallocatedCents: record.Corrected.UnallocatedCents,
			Replayed:         replayed,
		},
		Correction: record.Correction,
		Replayed:   replayed,
	}, nil
}

// correctionEvents is what a correction announces, queued inside its
// transaction: ONE PaymentCorrected (the family is told the right amount once —
// not a second "payment received"), and ChargeSettled for every charge the
// correction newly closed.
func (s *FinanceService) correctionEvents(record ports.CorrectionRecord) []ports.OutboxEvent {
	corrected := record.Corrected.Deposit
	events := []ports.OutboxEvent{
		outboxEvent(ports.OutboxPaymentCorrected, ports.PaymentCorrectedEvent{
			PaymentID:           corrected.ID(),
			OriginalPaymentID:   record.Original.ID(),
			StudentID:           corrected.StudentID(),
			TenantID:            s.tenantID,
			Method:              corrected.Method(),
			AmountCents:         corrected.Amount().Int64(),
			PreviousAmountCents: record.Original.Amount().Int64(),
			PaidAt:              corrected.PaidAt(),
			Folio:               corrected.Folio(),
		}),
	}
	for _, settled := range record.Corrected.Settled {
		events = append(events, outboxEvent(ports.OutboxChargeSettled, ports.ChargeSettledEvent{
			ChargeID:    settled.Charge.ID(),
			StudentID:   settled.Charge.StudentID(),
			TenantID:    s.tenantID,
			ExternalRef: settled.Charge.ExternalRef(),
			AmountCents: settled.Charge.Effective().Int64(),
			SettledAt:   corrected.PaidAt(),
		}))
	}
	return events
}

// CorrectionHistory is a receipt's correction chain, oldest first, with the
// concepts its lines paid named.
type CorrectionHistory struct {
	Corrections []domain.DepositCorrection
	// Descriptions names each charge a snapshot line paid.
	Descriptions map[string]string
}

// DepositCorrections returns every correction in a deposit's chain: from the
// first receipt to the one standing now, whichever link is asked about.
func (s *FinanceService) DepositCorrections(ctx context.Context, depositID string) (CorrectionHistory, error) {
	d, err := s.repo.GetDeposit(ctx, depositID)
	if err != nil {
		return CorrectionHistory{}, err
	}
	all, err := s.repo.ListDepositCorrections(ctx, d.StudentID())
	if err != nil {
		return CorrectionHistory{}, err
	}
	charges, err := s.repo.ListCharges(ctx, d.StudentID(), "")
	if err != nil {
		return CorrectionHistory{}, err
	}
	descriptions := make(map[string]string, len(charges))
	for _, c := range charges {
		descriptions[c.Charge.ID()] = c.Charge.Description()
	}
	return CorrectionHistory{Corrections: chainOf(depositID, all), Descriptions: descriptions}, nil
}

// chainOf keeps the corrections linked to depositID through any number of
// steps, in the order given. A deposit is corrected at most once and corrects
// at most one, so the chain is a line; the walk stops when a pass adds nothing.
func chainOf(depositID string, all []domain.DepositCorrection) []domain.DepositCorrection {
	linked := map[string]bool{depositID: true}
	in := make([]bool, len(all))
	for grew := true; grew; {
		grew = false
		for i, c := range all {
			if in[i] || (!linked[c.OriginalID()] && !linked[c.CorrectedID()]) {
				continue
			}
			in[i], grew = true, true
			linked[c.OriginalID()], linked[c.CorrectedID()] = true, true
		}
	}
	out := make([]domain.DepositCorrection, 0, len(all))
	for i, c := range all {
		if in[i] {
			out = append(out, c)
		}
	}
	return out
}
