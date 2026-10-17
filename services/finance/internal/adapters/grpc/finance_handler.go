// Package grpcadapter adapts the finance use cases to the generated
// garcialorca.finance.v1 Connect service. It only translates: proto messages ↔
// use-case calls, domain entities ↔ proto messages, and domain error Kinds ↔
// Connect codes. All logic lives in usecase/domain — GORM and transport details
// never leak inward.
package grpcadapter

import (
	"context"
	"errors"
	"strings"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/finance/v1/financev1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// Compile-time proof the adapter implements the generated Connect handler.
var _ financev1connect.FinanceServiceHandler = (*FinanceHandler)(nil)

// dueDateLayout is the wire format for a charge's due date: a plain calendar
// day, no time zone (mirrors the use case's parse side).
const dueDateLayout = "2006-01-02"

// FinanceHandler adapts the finance use cases to the generated Connect service.
type FinanceHandler struct {
	finance *usecase.FinanceService
	health  *usecase.HealthService
	levels  *usecase.LevelArchiveService
	// expenses is T10's (WithExpenses); nil leaves those RPCs Unimplemented.
	expenses *usecase.ExpenseService
	// activities is T17's (WithActivities); nil leaves those RPCs Unimplemented.
	activities *usecase.ActivityService
	// scholarships is T18's (WithScholarships); nil leaves those RPCs Unimplemented.
	scholarships *usecase.ScholarshipService
}

// NewFinanceHandler wires the handler to the finance + health use cases.
func NewFinanceHandler(svc *usecase.FinanceService, health *usecase.HealthService) *FinanceHandler {
	return &FinanceHandler{finance: svc, health: health}
}

// Health is the liveness/ping probe present on every service.
func (h *FinanceHandler) Health(ctx context.Context, _ *connect.Request[financev1.HealthRequest]) (*connect.Response[financev1.HealthResponse], error) {
	status, err := h.health.Ping(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.HealthResponse{Status: status.Status}), nil
}

// CreateConcept registers a charge concept in the tenant's catalog.
func (h *FinanceHandler) CreateConcept(ctx context.Context, req *connect.Request[financev1.CreateConceptRequest]) (*connect.Response[financev1.CreateConceptResponse], error) {
	m := req.Msg
	concept, err := h.finance.CreateConcept(ctx, m.GetName(), m.GetDefaultAmountCents(), m.GetRecurrence())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.CreateConceptResponse{Concept: toProtoConcept(concept)}), nil
}

// ListConcepts returns the catalog; deactivated entries appear only when
// include_inactive is set.
func (h *FinanceHandler) ListConcepts(ctx context.Context, req *connect.Request[financev1.ListConceptsRequest]) (*connect.Response[financev1.ListConceptsResponse], error) {
	concepts, err := h.finance.ListConcepts(ctx, req.Msg.GetIncludeInactive())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*financev1.ChargeConcept, 0, len(concepts))
	for _, c := range concepts {
		out = append(out, toProtoConcept(c))
	}
	return connect.NewResponse(&financev1.ListConceptsResponse{Concepts: out}), nil
}

// UpdateConcept replaces the mutable fields of a catalog entry.
func (h *FinanceHandler) UpdateConcept(ctx context.Context, req *connect.Request[financev1.UpdateConceptRequest]) (*connect.Response[financev1.UpdateConceptResponse], error) {
	m := req.Msg
	concept, err := h.finance.UpdateConcept(ctx, m.GetId(), m.GetName(), m.GetDefaultAmountCents(), m.GetRecurrence(), m.GetActive())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.UpdateConceptResponse{Concept: toProtoConcept(concept)}), nil
}

// CreateCharge issues one charge against a student's cycle.
func (h *FinanceHandler) CreateCharge(ctx context.Context, req *connect.Request[financev1.CreateChargeRequest]) (*connect.Response[financev1.CreateChargeResponse], error) {
	m := req.Msg
	charge, err := h.finance.CreateCharge(ctx, usecase.CreateChargeInput{
		StudentID:           m.GetStudentId(),
		CycleID:             m.GetCycleId(),
		ConceptID:           m.GetConceptId(),
		DueDate:             m.GetDueDate(),
		AmountCentsOverride: m.GetAmountCentsOverride(),
		DescriptionOverride: m.GetDescriptionOverride(),
		ExternalRef:         m.GetExternalRef(),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.CreateChargeResponse{Charge: toProtoCharge(charge)}), nil
}

// GenerateCharges issues the concept to every active enrollment of a cycle (or
// of one group within it), skipping students who already owe it.
func (h *FinanceHandler) GenerateCharges(ctx context.Context, req *connect.Request[financev1.GenerateChargesRequest]) (*connect.Response[financev1.GenerateChargesResponse], error) {
	m := req.Msg
	created, skipped, err := h.finance.GenerateCharges(ctx, m.GetConceptId(), m.GetCycleId(), m.GetGroupId(),
		m.GetAmountCentsOverride(), m.GetDueDate())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.GenerateChargesResponse{
		CreatedCount: int32(created),
		SkippedCount: int32(skipped),
	}), nil
}

// ListCharges lists one page of charges, filtered by student and/or cycle.
func (h *FinanceHandler) ListCharges(ctx context.Context, req *connect.Request[financev1.ListChargesRequest]) (*connect.Response[financev1.ListChargesResponse], error) {
	m := req.Msg
	after, err := decodePageToken(m.GetPageToken())
	if err != nil {
		return nil, connectErr(err)
	}
	page, err := h.finance.ListChargesPage(ctx, m.GetStudentId(), m.GetCycleId(), after, int(m.GetPageSize()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.ListChargesResponse{
		Charges:       toProtoCharges(page.Charges),
		NextPageToken: encodePageToken(page.Next),
	}), nil
}

// PreviewWithdrawal returns the charges a dated withdrawal would cancel and
// the balance that would remain. The use case owns all pricing and reads only.
func (h *FinanceHandler) PreviewWithdrawal(ctx context.Context, req *connect.Request[financev1.PreviewWithdrawalRequest]) (*connect.Response[financev1.PreviewWithdrawalResponse], error) {
	m := req.Msg
	plan, err := h.finance.PreviewWithdrawal(ctx, m.GetStudentId(), m.GetCycleId(), m.GetWithdrawnOn())
	if err != nil {
		return nil, connectErr(err)
	}
	toCancel := make([]*financev1.Charge, 0, len(plan.ToCancel))
	for _, charge := range plan.ToCancel {
		toCancel = append(toCancel, toProtoCharge(ports.ChargeWithPaid{Charge: charge}))
	}
	return connect.NewResponse(&financev1.PreviewWithdrawalResponse{
		ToCancel: toCancel, ToCancelCents: plan.ToCancelCents, RemainingDebtCents: plan.RemainingDebtCents,
	}), nil
}

// ApplyDiscount sets the discount on a charge and returns it re-derived.
func (h *FinanceHandler) ApplyDiscount(ctx context.Context, req *connect.Request[financev1.ApplyDiscountRequest]) (*connect.Response[financev1.ApplyDiscountResponse], error) {
	m := req.Msg
	charge, err := h.finance.ApplyDiscount(ctx, m.GetChargeId(), m.GetDiscountCents(), m.GetReason())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.ApplyDiscountResponse{Charge: toProtoCharge(charge)}), nil
}

// CancelCharge voids a charge.
func (h *FinanceHandler) CancelCharge(ctx context.Context, req *connect.Request[financev1.CancelChargeRequest]) (*connect.Response[financev1.CancelChargeResponse], error) {
	charge, err := h.finance.CancelCharge(ctx, req.Msg.GetChargeId(), req.Msg.GetReason())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.CancelChargeResponse{Charge: toProtoCharge(charge)}), nil
}

// CancelChargeIfUnpaid voids a charge only if nothing was collected against it.
// The guarded variant automated callers use; see the RPC comment in the proto.
func (h *FinanceHandler) CancelChargeIfUnpaid(ctx context.Context, req *connect.Request[financev1.CancelChargeIfUnpaidRequest]) (*connect.Response[financev1.CancelChargeIfUnpaidResponse], error) {
	charge, err := h.finance.CancelChargeIfUnpaid(ctx, req.Msg.GetChargeId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.CancelChargeIfUnpaidResponse{Charge: toProtoCharge(charge)}), nil
}

// RegisterPayment applies a payment to a charge and returns the payment plus
// the charge with its recomputed paid/remaining/status. An idempotency key on
// the request makes a retry return the original payment instead of collecting
// again; an empty one keeps the historical behaviour.
func (h *FinanceHandler) RegisterPayment(ctx context.Context, req *connect.Request[financev1.RegisterPaymentRequest]) (*connect.Response[financev1.RegisterPaymentResponse], error) {
	m := req.Msg
	// An unset paid_at arrives as a nil timestamp and becomes the zero time, which
	// the use case reads as "now" — so an older client keeps its behaviour exactly.
	var paidAt time.Time
	if ts := m.GetPaidAt(); ts != nil {
		paidAt = ts.AsTime()
	}
	tenders := fromProtoTenders(m.GetTenders())
	if len(tenders) == 0 {
		// The single method of a client from before T8.
		tenders = []usecase.TenderInput{{Method: m.GetMethod(), AmountCents: m.GetAmountCents(), Reference: m.GetReference()}}
	}
	payments, charge, err := h.finance.RegisterTenderedPayment(ctx, usecase.TenderedPaymentInput{
		ChargeID: m.GetChargeId(), AmountCents: m.GetAmountCents(), Tenders: tenders,
		IdempotencyKey: m.GetIdempotencyKey(), PaidAt: paidAt, ReceivedBy: m.GetReceivedBy(),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	rows := make([]*financev1.Payment, 0, len(payments))
	for _, p := range payments {
		rows = append(rows, toProtoPayment(p))
	}
	return connect.NewResponse(&financev1.RegisterPaymentResponse{
		Payment:  rows[0],
		Charge:   toProtoCharge(charge),
		Payments: rows,
	}), nil
}

// GetAccount returns a student's account statement.
func (h *FinanceHandler) GetAccount(ctx context.Context, req *connect.Request[financev1.GetAccountRequest]) (*connect.Response[financev1.GetAccountResponse], error) {
	read := h.finance.GetAccount
	if req.Msg.GetFamilyView() {
		read = h.finance.GetFamilyAccount
	}
	statement, err := read(ctx, req.Msg.GetStudentId())
	if err != nil {
		return nil, connectErr(err)
	}
	payments := make([]*financev1.Payment, 0, len(statement.Payments))
	for _, p := range statement.Payments {
		payments = append(payments, toProtoPayment(p))
	}
	return connect.NewResponse(&financev1.GetAccountResponse{
		Charges:            toProtoCharges(statement.Charges),
		Payments:           payments,
		TotalBalanceCents:  statement.TotalBalanceCents,
		UnallocatedCents:   statement.UnallocatedCents,
		DueTodayCents:      statement.DueTodayCents,
		SurchargeCents:     statement.SurchargeCents,
		EarlyDiscountCents: statement.EarlyDiscountCents,
		AsOf:               statement.AsOf,
	}), nil
}

// GetStatement returns a student's ledger (T7): movements with the running
// balance, totals, and the counter's figures for today.
func (h *FinanceHandler) GetStatement(ctx context.Context, req *connect.Request[financev1.GetStatementRequest]) (*connect.Response[financev1.GetStatementResponse], error) {
	read := h.finance.GetStatement
	if req.Msg.GetFamilyView() {
		read = h.finance.GetFamilyStatement
	}
	statement, err := read(ctx, req.Msg.GetStudentId())
	if err != nil {
		return nil, connectErr(err)
	}
	movements := make([]*financev1.StatementMovement, 0, len(statement.Movements))
	for _, m := range statement.Movements {
		movements = append(movements, toProtoMovement(m))
	}
	t := statement.Totals
	return connect.NewResponse(&financev1.GetStatementResponse{
		Movements: movements,
		Totals: &financev1.StatementTotals{
			ChargedCents:   t.ChargedCents,
			DiscountCents:  t.DiscountCents,
			PaidCents:      t.PaidCents,
			BalanceCents:   t.BalanceCents,
			NotYetDueCents: t.NotYetDueCents,
		},
		DueTodayCents:      statement.DueTodayCents,
		SurchargeCents:     statement.SurchargeCents,
		EarlyDiscountCents: statement.EarlyDiscountCents,
		UnallocatedCents:   statement.UnallocatedCents,
		AsOf:               statement.AsOf,
	}), nil
}

// toProtoMovement projects one ledger line. Dates go out as the stored date's
// calendar day (its UTC date, domain.CalendarDay; ADR 0035 re-zones only
// "today", never a stored date).
func toProtoMovement(m domain.Movement) *financev1.StatementMovement {
	return &financev1.StatementMovement{
		Kind:         string(m.Kind),
		Date:         m.Date.UTC().Format(dueDateLayout),
		Description:  m.Description,
		AmountCents:  m.AmountCents,
		BalanceCents: m.BalanceCents,
		ChargeId:     m.ChargeID,
		DepositId:    m.DepositID,
		Method:       m.Method,
		CardType:     m.CardType,
		Reference:    m.Reference,
		Folio:        m.Folio,
		Accrued:      m.Accrued,
		NotYetDue:    m.NotYetDue,
		Voided:       m.Voided,
		VoidReason:   m.VoidReason,
		// T9: a receipt replaced by its correction, and the correction's lines.
		Superseded:        m.Superseded,
		SupersededByFolio: m.SupersededByFolio,
		CorrectsFolio:     m.CorrectsFolio,
		AppliedTo:         toProtoAppliedParts(m.AppliedTo),
	}
}

// toProtoAppliedParts projects what a deposit's line paid (ISSUE-T14-106).
func toProtoAppliedParts(parts []domain.AppliedPart) []*financev1.StatementAppliedPart {
	if len(parts) == 0 {
		return nil
	}
	out := make([]*financev1.StatementAppliedPart, 0, len(parts))
	for _, p := range parts {
		out = append(out, &financev1.StatementAppliedPart{ChargeId: p.ChargeID, Description: p.Description, AmountCents: p.AmountCents})
	}
	return out
}

// ListOverdue lists one page of the charges past their due date that still owe
// money. The page may be short, even empty, while a token says more remain.
func (h *FinanceHandler) ListOverdue(ctx context.Context, req *connect.Request[financev1.ListOverdueRequest]) (*connect.Response[financev1.ListOverdueResponse], error) {
	m := req.Msg
	after, err := decodePageToken(m.GetPageToken())
	if err != nil {
		return nil, connectErr(err)
	}
	page, err := h.finance.ListOverduePage(ctx, m.GetCycleId(), after, int(m.GetPageSize()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.ListOverdueResponse{
		Charges:       toProtoCharges(page.Charges),
		NextPageToken: encodePageToken(page.Next),
	}), nil
}

// --- deposits (M16) --------------------------------------------------------

// RegisterDeposit takes money without being told what it pays for.
func (h *FinanceHandler) RegisterDeposit(ctx context.Context, req *connect.Request[financev1.RegisterDepositRequest]) (*connect.Response[financev1.RegisterDepositResponse], error) {
	m := req.Msg
	// An unset paid_at becomes the zero time, which the use case reads as "now".
	var paidAt time.Time
	if ts := m.GetPaidAt(); ts != nil {
		paidAt = ts.AsTime()
	}
	got, err := h.finance.RegisterDeposit(ctx, usecase.DepositInput{
		StudentID:      m.GetStudentId(),
		AmountCents:    m.GetAmountCents(),
		Method:         m.GetMethod(),
		Reference:      m.GetReference(),
		PaidAt:         paidAt,
		IdempotencyKey: m.GetIdempotencyKey(),
		// The school honouring the reward late is a decision it makes at the
		// counter; the use case records it as one.
		GrantEarlyPayment: m.GetGrantEarlyPayment(),
		CardType:          m.GetCardType(),
		Receiver:          domain.Receiver{UserID: m.GetReceivedBy(), Name: m.GetReceivedByName()},
		ChargeIDs:         m.GetChargeIds(),
		Tenders:           fromProtoTenders(m.GetTenders()),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	allocations := make([]*financev1.Payment, 0, len(got.Allocations))
	for _, p := range got.Allocations {
		allocations = append(allocations, toProtoPayment(p))
	}
	surcharges := make([]*financev1.Charge, 0, len(got.Surcharges))
	for _, c := range got.Surcharges {
		// A freshly materialised surcharge has collected nothing yet by
		// definition: it did not exist a moment ago.
		surcharges = append(surcharges, toProtoCharge(ports.ChargeWithPaid{Charge: c}))
	}
	return connect.NewResponse(&financev1.RegisterDepositResponse{
		Deposit:          toProtoDeposit(got.Deposit),
		Allocations:      allocations,
		Surcharges:       surcharges,
		UnallocatedCents: got.UnallocatedCents,
		Replayed:         got.Replayed,
	}), nil
}

// PreviewDeposit says what an amount would cover, without taking it.
func (h *FinanceHandler) PreviewDeposit(ctx context.Context, req *connect.Request[financev1.PreviewDepositRequest]) (*connect.Response[financev1.PreviewDepositResponse], error) {
	m := req.Msg
	var paidAt time.Time
	if ts := m.GetPaidAt(); ts != nil {
		paidAt = ts.AsTime()
	}
	got, err := h.finance.PreviewDeposit(ctx, usecase.DepositInput{
		StudentID:   m.GetStudentId(),
		AmountCents: m.GetAmountCents(),
		// A preview needs no method: nothing is stored, and cash and a transfer
		// cover the same months. Cash keeps the domain constructor satisfied.
		Method:    domain.MethodCash,
		PaidAt:    paidAt,
		ChargeIDs: m.GetChargeIds(),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	allocations := make([]*financev1.Payment, 0, len(got.Allocations))
	for _, p := range got.Allocations {
		allocations = append(allocations, toProtoPayment(p))
	}
	surcharges := make([]*financev1.Charge, 0, len(got.Surcharges))
	for _, c := range got.Surcharges {
		surcharges = append(surcharges, toProtoCharge(ports.ChargeWithPaid{Charge: c}))
	}
	return connect.NewResponse(&financev1.PreviewDepositResponse{
		Allocations:      allocations,
		Surcharges:       surcharges,
		UnallocatedCents: got.UnallocatedCents,
	}), nil
}

// ExpectedToday is what a family must hand over today to be current.
func (h *FinanceHandler) ExpectedToday(ctx context.Context, req *connect.Request[financev1.ExpectedTodayRequest]) (*connect.Response[financev1.ExpectedTodayResponse], error) {
	on := time.Time{}
	if raw := req.Msg.GetOn(); raw != "" {
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			return nil, connectErr(glerrors.New(glerrors.KindInvalid, "date must be YYYY-MM-DD"))
		}
		on = parsed
	}
	quote := h.finance.ExpectedToday
	if req.Msg.GetSkipBalance() {
		quote = h.finance.ExpectedWithoutBalance
	}
	expected, err := quote(ctx, req.Msg.GetStudentId(), on)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.ExpectedTodayResponse{
		AmountCents:        expected.AmountCents,
		SurchargeCents:     expected.SurchargeCents,
		DiscountCents:      expected.DiscountCents,
		AnnualBalanceCents: expected.AnnualBalanceCents,
		OnTimeBalanceCents: expected.OnTimeBalanceCents,
		Lines:              toProtoExpectedLines(expected.Lines),
		ScholarshipCents:   expected.ScholarshipCents,
	}), nil
}

func toProtoExpectedLines(lines []usecase.ExpectedLine) []*financev1.ExpectedLine {
	out := make([]*financev1.ExpectedLine, 0, len(lines))
	for _, l := range lines {
		out = append(out, &financev1.ExpectedLine{
			ChargeId: l.ChargeID, Description: l.Description, DueDate: l.DueDate.Format(time.DateOnly),
			OwedCents: l.OwedCents, SurchargeCents: l.SurchargeCents, DiscountCents: l.DiscountCents,
			TotalCents: l.TotalCents, Due: l.Due, ScholarshipCents: l.ScholarshipCents, InBalance: l.InBalance,
		})
	}
	return out
}

// ListDeposits returns a student's deposits, newest first.
func (h *FinanceHandler) ListDeposits(ctx context.Context, req *connect.Request[financev1.ListDepositsRequest]) (*connect.Response[financev1.ListDepositsResponse], error) {
	deposits, err := h.finance.ListDeposits(ctx, req.Msg.GetStudentId())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*financev1.Deposit, 0, len(deposits))
	for _, d := range deposits {
		out = append(out, toProtoDeposit(d))
	}
	return connect.NewResponse(&financev1.ListDepositsResponse{Deposits: out}), nil
}

// DailyCashUp is the day's takings, split by how the money arrived — and,
// with the expenses wired (T10), what went out, the cash expected in the
// drawer and the day's closure.
func (h *FinanceHandler) DailyCashUp(ctx context.Context, req *connect.Request[financev1.DailyCashUpRequest]) (*connect.Response[financev1.DailyCashUpResponse], error) {
	day, err := parseOptionalDay(req.Msg.GetDay())
	if err != nil {
		return nil, connectErr(glerrors.New(glerrors.KindInvalid, "day must be YYYY-MM-DD"))
	}
	if h.expenses != nil {
		cut, err := h.expenses.CashCut(ctx, day)
		if err != nil {
			return nil, connectErr(err)
		}
		return connect.NewResponse(cashCutResponse(cut)), nil
	}
	// An unset day is the school clock's today, decided in the use case.
	cut, err := h.finance.DailyCashUp(ctx, day)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(takingsResponse(cut)), nil
}

// takingsResponse renders the day's takings: what came in, and in what form.
func takingsResponse(cut usecase.CashUp) *financev1.DailyCashUpResponse {
	return &financev1.DailyCashUpResponse{
		Day:           cut.Day.Format(dayLayout),
		Deposits:      toProtoDeposits(cut.Deposits),
		Cancelled:     toProtoDeposits(cut.Cancelled),
		Superseded:    toProtoDeposits(cut.Superseded),
		CashCents:     cut.ByMethod[domain.MethodCash],
		CardCents:     cut.ByMethod[domain.MethodCard],
		TransferCents: cut.ByMethod[domain.MethodTransfer],
		TotalCents:    cut.TotalCents,
		Payments:      toProtoCashUpPayments(cut.Payments),
		// Not money (T8): reported on its own line, outside the totals.
		CreditNoteCents: cut.CreditNoteCents,
	}
}

func toProtoCashUpPayments(ps []ports.LoosePayment) []*financev1.CashUpPayment {
	out := make([]*financev1.CashUpPayment, 0, len(ps))
	for _, p := range ps {
		out = append(out, &financev1.CashUpPayment{Payment: toProtoPayment(p.Payment), Description: p.Description})
	}
	return out
}

func toProtoDeposits(ds []domain.Deposit) []*financev1.Deposit {
	out := make([]*financev1.Deposit, 0, len(ds))
	for _, d := range ds {
		out = append(out, toProtoDeposit(d))
	}
	return out
}

// IncomeByConcept is what the school earned in a period.
func (h *FinanceHandler) IncomeByConcept(ctx context.Context, req *connect.Request[financev1.IncomeByConceptRequest]) (*connect.Response[financev1.IncomeByConceptResponse], error) {
	from, err := parseIncomeDate(req.Msg.GetFrom())
	if err != nil {
		return nil, connectErr(err)
	}
	to, err := parseIncomeDate(req.Msg.GetTo())
	if err != nil {
		return nil, connectErr(err)
	}
	income, err := h.finance.IncomeByConcept(ctx, from, to)
	if err != nil {
		return nil, connectErr(err)
	}
	lines := make([]*financev1.IncomeLine, 0, len(income.Lines))
	for _, l := range income.Lines {
		lines = append(lines, &financev1.IncomeLine{
			ConceptId: l.ConceptID, Concept: l.Concept, AmountCents: l.AmountCents,
		})
	}
	return connect.NewResponse(&financev1.IncomeByConceptResponse{
		Lines: lines, TotalCents: income.TotalCents, CreditNoteCents: income.CreditNoteCents,
	}), nil
}

// parseIncomeDate reads a period bound. Empty is refused rather than defaulted:
// a report over "everything since the beginning" is almost never what was meant,
// and it is the kind of mistake read as a real number.
func parseIncomeDate(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, glerrors.New(glerrors.KindInvalid, "the period needs both dates")
	}
	d, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, glerrors.New(glerrors.KindInvalid, "dates must be YYYY-MM-DD")
	}
	return d, nil
}

// GetDepositReceipt returns a deposit and what it paid, concept by concept.
func (h *FinanceHandler) GetDepositReceipt(ctx context.Context, req *connect.Request[financev1.GetDepositReceiptRequest]) (*connect.Response[financev1.GetDepositReceiptResponse], error) {
	got, err := h.finance.DepositReceipt(ctx, req.Msg.GetDepositId())
	if err != nil {
		return nil, connectErr(err)
	}
	lines := make([]*financev1.ReceiptLine, 0, len(got.Lines))
	for _, l := range got.Lines {
		lines = append(lines, &financev1.ReceiptLine{
			ChargeId: l.ChargeID, Description: l.Description, AmountCents: l.AmountCents,
			Surcharge: l.Surcharge, DiscountCents: l.DiscountCents,
			ScholarshipCents: l.ScholarshipCents, ScholarshipName: l.ScholarshipName,
		})
	}
	return connect.NewResponse(&financev1.GetDepositReceiptResponse{
		Deposit: toProtoDeposit(got.Deposit), Lines: lines, UnallocatedCents: got.UnallocatedCents,
		PendingCents: got.PendingCents,
	}), nil
}

// VoidDeposit cancels a deposit and releases what it settled.
func (h *FinanceHandler) VoidDeposit(ctx context.Context, req *connect.Request[financev1.VoidDepositRequest]) (*connect.Response[financev1.VoidDepositResponse], error) {
	if err := h.finance.VoidDeposit(ctx, req.Msg.GetId(), req.Msg.GetReason()); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.VoidDepositResponse{}), nil
}

func toProtoDeposit(d domain.Deposit) *financev1.Deposit {
	out := &financev1.Deposit{
		Id:              d.ID(),
		StudentId:       d.StudentID(),
		AmountCents:     d.Amount().Int64(),
		Method:          d.Method(),
		Reference:       d.Reference(),
		PaidAt:          timestamppb.New(d.PaidAt()),
		PaidOn:          formatPlanDate(d.PaidOn()),
		CalendarVersion: d.CalendarVersion(),
		CardType:        d.CardType(),
		VoidReason:      d.VoidReason(),
		ReceivedBy:      d.ReceivedBy().UserID,
		ReceivedByName:  d.ReceivedBy().Name,
		Folio:           d.Folio(),
		Tenders:         toProtoTenders(d.Tenders()),
	}
	if d.Voided() {
		out.VoidedAt = timestamppb.New(d.VoidedAt())
	}
	if d.Superseded() {
		out.SupersededAt = timestamppb.New(d.SupersededAt())
		out.SupersededBy = d.SupersededBy()
		out.SupersededByFolio = d.SupersededByFolio()
	}
	out.CorrectsDepositId, out.CorrectsFolio = d.Corrects(), d.CorrectsFolio()
	if !d.CapturedAt().IsZero() {
		out.CapturedAt = timestamppb.New(d.CapturedAt())
	}
	return out
}

// --- corrections (T9) ------------------------------------------------------

// CorrectDeposit corrects a registered payment. An id that cannot be a
// deposit's is "no such deposit" — never a query the database refuses.
func (h *FinanceHandler) CorrectDeposit(ctx context.Context, req *connect.Request[financev1.CorrectDepositRequest]) (*connect.Response[financev1.CorrectDepositResponse], error) {
	m := req.Msg
	if !usecase.IsDepositID(m.GetDepositId()) {
		return nil, connectErr(glerrors.ErrNotFound)
	}
	var paidAt time.Time
	if ts := m.GetPaidAt(); ts != nil {
		paidAt = ts.AsTime()
	}
	got, err := h.finance.CorrectDeposit(ctx, usecase.CorrectionInput{
		DepositID:      m.GetDepositId(),
		AmountCents:    m.GetAmountCents(),
		PaidAt:         paidAt,
		Tenders:        fromProtoTenders(m.GetTenders()),
		ChargeIDs:      m.GetChargeIds(),
		Reason:         m.GetReason(),
		IdempotencyKey: m.GetIdempotencyKey(),
		By:             domain.Receiver{UserID: m.GetCorrectedBy(), Name: m.GetCorrectedByName()},
	})
	if err != nil {
		return nil, connectErr(err)
	}
	allocations := make([]*financev1.Payment, 0, len(got.Corrected.Allocations))
	for _, p := range got.Corrected.Allocations {
		allocations = append(allocations, toProtoPayment(p))
	}
	surcharges := make([]*financev1.Charge, 0, len(got.Corrected.Surcharges))
	for _, c := range got.Corrected.Surcharges {
		surcharges = append(surcharges, toProtoCharge(ports.ChargeWithPaid{Charge: c}))
	}
	return connect.NewResponse(&financev1.CorrectDepositResponse{
		Original:         toProtoDeposit(got.Original),
		Deposit:          toProtoDeposit(got.Corrected.Deposit),
		Allocations:      allocations,
		Surcharges:       surcharges,
		UnallocatedCents: got.Corrected.UnallocatedCents,
		Replayed:         got.Replayed,
		Correction:       toProtoCorrection(got.Correction, nil),
	}), nil
}

// ListDepositCorrections is a receipt's correction history, its chain whole.
func (h *FinanceHandler) ListDepositCorrections(ctx context.Context, req *connect.Request[financev1.ListDepositCorrectionsRequest]) (*connect.Response[financev1.ListDepositCorrectionsResponse], error) {
	if !usecase.IsDepositID(req.Msg.GetDepositId()) {
		return nil, connectErr(glerrors.ErrNotFound)
	}
	history, err := h.finance.DepositCorrections(ctx, req.Msg.GetDepositId())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*financev1.DepositCorrection, 0, len(history.Corrections))
	for _, c := range history.Corrections {
		out = append(out, toProtoCorrection(c, history.Descriptions))
	}
	return connect.NewResponse(&financev1.ListDepositCorrectionsResponse{Corrections: out}), nil
}

func toProtoCorrection(c domain.DepositCorrection, descriptions map[string]string) *financev1.DepositCorrection {
	return &financev1.DepositCorrection{
		Id:                 c.ID(),
		OriginalDepositId:  c.OriginalID(),
		CorrectedDepositId: c.CorrectedID(),
		Reason:             c.Reason(),
		Before:             toProtoSnapshot(c.Before(), descriptions),
		After:              toProtoSnapshot(c.After(), descriptions),
		CorrectedBy:        c.By().UserID,
		CorrectedByName:    c.By().Name,
		CorrectedAt:        timestamppb.New(c.At()),
	}
}

func toProtoSnapshot(s domain.DepositSnapshot, descriptions map[string]string) *financev1.DepositSnapshot {
	out := &financev1.DepositSnapshot{DepositId: s.DepositID, Folio: s.Folio, AmountCents: s.AmountCents, PaidOn: formatPlanDate(s.PaidOn)}
	if !s.PaidAt.IsZero() {
		out.PaidAt = timestamppb.New(s.PaidAt)
	}
	for _, t := range s.Tenders {
		out.Tenders = append(out.Tenders, &financev1.Tender{Method: t.Method, AmountCents: t.AmountCents, Reference: t.Reference, CardType: t.CardType})
	}
	for _, l := range s.Lines {
		out.Lines = append(out.Lines, &financev1.SnapshotLine{ChargeId: l.ChargeID, Description: descriptions[l.ChargeID], AmountCents: l.AmountCents})
	}
	return out
}

// toProtoTenders is a deposit's tenders on the wire (T8).
func toProtoTenders(ts []domain.Tender) []*financev1.Tender {
	out := make([]*financev1.Tender, 0, len(ts))
	for _, t := range ts {
		out = append(out, &financev1.Tender{
			Method: t.Method(), AmountCents: t.Amount().Int64(), Reference: t.Reference(), CardType: t.CardType(),
		})
	}
	return out
}

// fromProtoTenders is the counter's tenders, for the use case to validate.
func fromProtoTenders(ts []*financev1.Tender) []usecase.TenderInput {
	out := make([]usecase.TenderInput, 0, len(ts))
	for _, t := range ts {
		out = append(out, usecase.TenderInput{
			Method: t.GetMethod(), AmountCents: t.GetAmountCents(), Reference: t.GetReference(), CardType: t.GetCardType(),
		})
	}
	return out
}

// DrainOutbox publishes the events still waiting.
func (h *FinanceHandler) DrainOutbox(ctx context.Context, req *connect.Request[financev1.DrainOutboxRequest]) (*connect.Response[financev1.DrainOutboxResponse], error) {
	limit := int(req.Msg.GetLimit())
	published, err := h.finance.DrainOutbox(ctx, limit)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.DrainOutboxResponse{Published: int32(published)}), nil
}

// ExportContpaqi is the CONTPAQi XML export (spike S02). The contract reserves
// the RPC; the field mapping is a later phase, so the boundary answers
// Unimplemented rather than an empty document that could be mistaken for one.
func (h *FinanceHandler) ExportContpaqi(_ context.Context, _ *connect.Request[financev1.ExportContpaqiRequest]) (*connect.Response[financev1.ExportContpaqiResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("CONTPAQi export is not implemented yet"))
}

// toProtoConcept maps a domain ChargeConcept to its proto message.
func toProtoConcept(c domain.ChargeConcept) *financev1.ChargeConcept {
	return &financev1.ChargeConcept{
		Id:                 c.ID(),
		TenantId:           c.TenantID(),
		Name:               c.Name(),
		DefaultAmountCents: c.DefaultAmount().Int64(),
		Recurrence:         c.Recurrence(),
		Kind:               c.Kind(),
		Active:             c.Active(),
	}
}

// toProtoCharges maps a slice of charges with their collected totals.
func toProtoCharges(cs []ports.ChargeWithPaid) []*financev1.Charge {
	out := make([]*financev1.Charge, 0, len(cs))
	for _, c := range cs {
		out = append(out, toProtoCharge(c))
	}
	return out
}

// toProtoCharge maps a charge plus what has been collected against it, filling
// the DERIVED wire fields the domain deliberately does not store: paid_cents
// comes from the payments, and remaining_cents is the effective amount minus
// what is paid, floored at zero — an over-collected charge is a credit to
// reconcile, never a negative debt.
func toProtoCharge(cwp ports.ChargeWithPaid) *financev1.Charge {
	c := cwp.Charge
	return &financev1.Charge{
		Id:               c.ID(),
		TenantId:         c.TenantID(),
		StudentId:        c.StudentID(),
		CycleId:          c.CycleID(),
		ConceptId:        c.ConceptID(),
		Description:      c.Description(),
		AmountCents:      c.Amount().Int64(),
		DiscountCents:    c.Discount().Int64(),
		DueDate:          c.DueDate().Format(dueDateLayout),
		StartsOn:         formatPlanDate(c.StartsOn()),
		ExternalRef:      c.ExternalRef(),
		Status:           c.Status(),
		PaidCents:        cwp.PaidCents,
		RemainingCents:   max(0, c.Effective().Int64()-cwp.PaidCents),
		ScholarshipCents: c.Scholarship().Int64(),
		ScholarshipId:    c.ScholarshipID(),
		CreditNoteCents:  cwp.CreditNoteCents,
	}
}

// toProtoPayment maps a domain Payment to its proto message.
func toProtoPayment(p domain.Payment) *financev1.Payment {
	return &financev1.Payment{
		Id:          p.ID(),
		TenantId:    p.TenantID(),
		ChargeId:    p.ChargeID(),
		AmountCents: p.Amount().Int64(),
		Method:      p.Method(),
		Reference:   p.Reference(),
		PaidAt:      timestamppb.New(p.PaidAt()),
		PaidOn:      formatPlanDate(p.PaidOn()),
		CardType:    p.CardType(),
		DepositId:   p.DepositID(),
	}
}

// connectErr maps a domain error Kind to the Connect status code. Internal
// failures return a generic message (no internal detail leaks to the client);
// the other Kinds carry already-generic domain messages.
// Every code goes through glerrors.SafeMessage, not just the internal one. The
// kind decides the STATUS; it does not decide whether the wrapped cause is the
// caller's business, and it never is. Error() concatenates the cause, so handing
// the error straight to connect.NewError shipped Postgres constraint names and
// SQLSTATEs — and on some constraints the conflicting value — to whoever called.
func connectErr(err error) *connect.Error {
	ce := connectErrForKind(err)
	if code := levelarchive.CodeOf(err); code != "" {
		ce.Meta().Set(errorCodeMeta, code)
	}
	return ce
}

// errorCodeMeta is the error-metadata key a refusal's machine code rides in
// (e.g. LEVEL_BEING_DELETED); the gateway forwards it as X-Error-Code.
const errorCodeMeta = "X-Error-Code"

func connectErrForKind(err error) *connect.Error {
	safe := errors.New(glerrors.SafeMessage(err))

	switch glerrors.KindOf(err) {
	case glerrors.KindInvalid:
		return connect.NewError(connect.CodeInvalidArgument, safe)
	case glerrors.KindUnauthorized:
		return connect.NewError(connect.CodeUnauthenticated, safe)
	case glerrors.KindForbidden:
		return connect.NewError(connect.CodePermissionDenied, safe)
	case glerrors.KindNotFound:
		return connect.NewError(connect.CodeNotFound, safe)
	case glerrors.KindConflict:
		return connect.NewError(connect.CodeAlreadyExists, safe)
	default:
		return connect.NewError(connect.CodeInternal, glerrors.New(glerrors.KindInternal, "internal error"))
	}
}

// --- tuition (M15) ----------------------------------------------------------

// UpsertTuitionPlan sets what one level of one cycle costs.
func (h *FinanceHandler) UpsertTuitionPlan(ctx context.Context, req *connect.Request[financev1.UpsertTuitionPlanRequest]) (*connect.Response[financev1.UpsertTuitionPlanResponse], error) {
	m := req.Msg
	enrollmentDue, err := parsePlanDate(m.GetEnrollmentDueOn())
	if err != nil {
		return nil, connectErr(err)
	}
	enrollmentStarts, err := parsePlanDate(m.GetEnrollmentStartsOn())
	if err != nil {
		return nil, connectErr(err)
	}
	concepts, err := toDomainEnrollmentConcepts(m.GetEnrollmentConcepts())
	if err != nil {
		return nil, connectErr(err)
	}
	plan, err := h.finance.UpsertTuitionPlan(ctx, usecase.TuitionPlanInput{
		CycleID:              m.GetCycleId(),
		LevelID:              m.GetLevelId(),
		EnrollmentCents:      m.GetEnrollmentCents(),
		MonthlyCents:         m.GetMonthlyCents(),
		Months:               int(m.GetMonths()),
		FirstBilledMonth:     int(m.GetFirstBilledMonth()),
		DueDay:               int(m.GetDueDay()),
		LateFeeCents:         m.GetLateFeeCents(),
		Active:               m.GetActive(),
		Policy:               toDomainPolicy(m.GetPolicy()),
		Schedule:             toDomainSchedule(m.GetSchedule()),
		MonthlyDiscountCents: m.GetMonthlyDiscountCents(),
		Fees:                 toDomainFees(m.GetFees()),
		EnrollmentDueOn:      enrollmentDue,
		EnrollmentStartsOn:   enrollmentStarts,
		EnrollmentConcepts:   concepts,
	})
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.UpsertTuitionPlanResponse{Plan: toProtoPlan(plan)}), nil
}

// ListTuitionPlans returns the whole tuition table of a cycle.
func (h *FinanceHandler) ListTuitionPlans(ctx context.Context, req *connect.Request[financev1.ListTuitionPlansRequest]) (*connect.Response[financev1.ListTuitionPlansResponse], error) {
	plans, err := h.finance.ListTuitionPlans(ctx, req.Msg.GetCycleId())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*financev1.TuitionPlan, 0, len(plans))
	for _, p := range plans {
		out = append(out, toProtoPlan(p))
	}
	return connect.NewResponse(&financev1.ListTuitionPlansResponse{Plans: out}), nil
}

// RunBilling generates what the school is owed as of a date. Idempotent, so the
// same call from a button and from a scheduler is safe.
func (h *FinanceHandler) RunBilling(ctx context.Context, req *connect.Request[financev1.RunBillingRequest]) (*connect.Response[financev1.RunBillingResponse], error) {
	got, err := h.finance.RunBilling(ctx, req.Msg.GetCycleId(), req.Msg.GetAsOfDate())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.RunBillingResponse{
		TuitionCreated:     int32(got.TuitionCreated),
		EnrollmentCreated:  int32(got.EnrollmentCreated),
		LateFeesCreated:    int32(got.LateFeesCreated),
		FeesCreated:        int32(got.FeesCreated),
		ActivitiesCreated:  int32(got.ActivitiesCreated),
		EnrollmentMoved:    int32(got.EnrollmentMoved),
		EnrollmentRepriced: int32(got.EnrollmentRepriced),
		Skipped:            int32(got.Skipped),
		LevelsWithoutPlan:  got.LevelsWithoutPlan,
	}), nil
}

// toProtoPlan maps a domain TuitionPlan to its proto message.
func toProtoPlan(p domain.TuitionPlan) *financev1.TuitionPlan {
	return &financev1.TuitionPlan{
		Id:                   p.ID(),
		TenantId:             p.TenantID(),
		CycleId:              p.CycleID(),
		LevelId:              p.LevelID(),
		EnrollmentCents:      p.EnrollmentCents(),
		MonthlyCents:         p.MonthlyCents(),
		Months:               int32(p.Months()),
		FirstBilledMonth:     int32(p.FirstBilledMonth()),
		DueDay:               int32(p.DueDay()),
		LateFeeCents:         p.LateFeeCents(),
		Active:               p.Active(),
		Policy:               toProtoPolicy(p.Policy()),
		Schedule:             toProtoSchedule(p.Schedule()),
		MonthlyDiscountCents: p.MonthlyDiscountCents(),
		Fees:                 toProtoFees(p.Fees()),
		EnrollmentDueOn:      formatPlanDate(p.EnrollmentDueOn()),
		EnrollmentStartsOn:   formatPlanDate(p.EnrollmentStartsOn()),
		EnrollmentConcepts:   toProtoEnrollmentConcepts(p.EnrollmentConcepts()),
	}
}

// toDomainEnrollmentConcepts reads the inscription's priced lines off the wire.
// An absent list is the single-amount inscription every plan had until M18,
// which is what a client written before this sends.
//
// A malformed date is REFUSED rather than skipped. A ladder that quietly lost a
// step would charge a family the wrong tier for a year, and the request that
// caused it would have looked like it worked.
func toDomainEnrollmentConcepts(in []*financev1.EnrollmentConcept) ([]domain.EnrollmentConcept, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]domain.EnrollmentConcept, 0, len(in))
	for _, c := range in {
		tiers := make([]domain.PriceTier, 0, len(c.GetTiers()))
		for _, t := range c.GetTiers() {
			from, err := parsePlanDate(t.GetEffectiveFrom())
			if err != nil {
				return nil, err
			}
			to, err := parsePlanDate(t.GetEffectiveTo())
			if err != nil {
				return nil, err
			}
			tiers = append(tiers, domain.PriceTier{
				EffectiveFrom: from, EffectiveTo: to, AmountCents: t.GetAmountCents(),
			})
		}
		out = append(out, domain.EnrollmentConcept{Name: c.GetName(), Tiers: tiers})
	}
	return out, nil
}

func toProtoEnrollmentConcepts(in []domain.EnrollmentConcept) []*financev1.EnrollmentConcept {
	if len(in) == 0 {
		return nil
	}
	out := make([]*financev1.EnrollmentConcept, 0, len(in))
	for _, c := range in {
		tiers := make([]*financev1.PriceTier, 0, len(c.Tiers))
		for _, t := range c.Tiers {
			tiers = append(tiers, &financev1.PriceTier{
				EffectiveFrom: formatPlanDate(t.EffectiveFrom),
				EffectiveTo:   formatPlanDate(t.EffectiveTo),
				AmountCents:   t.AmountCents,
			})
		}
		out = append(out, &financev1.EnrollmentConcept{Name: c.Name, Tiers: tiers})
	}
	return out
}

// toDomainFees reads the level's extra fees off the wire. An absent list is a
// level that collects only its tuition, which is what a client written before
// this sends.
func toDomainFees(in []*financev1.Fee) []domain.Fee {
	if len(in) == 0 {
		return nil
	}
	out := make([]domain.Fee, 0, len(in))
	for _, f := range in {
		months := make([]int, 0, len(f.GetMonths()))
		for _, m := range f.GetMonths() {
			months = append(months, int(m))
		}
		out = append(out, domain.Fee{
			Name:        f.GetName(),
			AmountCents: f.GetAmountCents(),
			Months:      months,
		})
	}
	return out
}

func toProtoFees(in []domain.Fee) []*financev1.Fee {
	out := make([]*financev1.Fee, 0, len(in))
	for _, f := range in {
		months := make([]int32, 0, len(f.Months))
		for _, m := range f.Months {
			months = append(months, int32(m))
		}
		out = append(out, &financev1.Fee{Name: f.Name, AmountCents: f.AmountCents, Months: months})
	}
	return out
}

// parsePlanDate reads an optional YYYY-MM-DD off the wire.
//
// A malformed date is REFUSED rather than read as "unset". Swallowing it would
// move every inscription of the level back to the default rule, silently, and
// the school would find out from the due dates months later.
func parsePlanDate(s string) (time.Time, error) {
	if strings.TrimSpace(s) == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(dueDateLayout, s)
	if err != nil {
		return time.Time{}, glerrors.New(glerrors.KindInvalid, "the date is written YYYY-MM-DD")
	}
	return t, nil
}

// formatPlanDate is a stored date as the wire's "YYYY-MM-DD" (a plan's
// window, a payment's paid_on); a date never recorded is empty, not
// "0001-01-01".
func formatPlanDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(dueDateLayout)
}

// toDomainPolicy reads the collection rule off the wire. A nil message is the
// zero policy, which is the original monthly rule — an older client that does
// not know about M16 keeps meaning exactly what it meant.
func toDomainPolicy(p *financev1.CollectionPolicy) domain.CollectionPolicy {
	if p == nil {
		return domain.CollectionPolicy{}
	}
	return domain.CollectionPolicy{
		DailyMode:              p.GetDailyMode(),
		LateFeeDailyCents:      p.GetLateFeeDailyCents(),
		SurchargeCountsAllDays: p.GetSurchargeCountsAllDays(),
		EarlyPaymentCents:      p.GetEarlyPaymentCents(),
		EarlyPaymentDays:       int(p.GetEarlyPaymentDays()),
	}
}

// toDomainSchedule reads an explicit payment plan off the wire. An absent list
// is the uniform year, which is what a client written before this sends.
func toDomainSchedule(in []*financev1.Installment) []domain.Installment {
	if len(in) == 0 {
		return nil
	}
	out := make([]domain.Installment, 0, len(in))
	for _, i := range in {
		out = append(out, domain.Installment{Month: int(i.GetMonth()), AmountCents: i.GetAmountCents()})
	}
	return out
}

func toProtoSchedule(in []domain.Installment) []*financev1.Installment {
	out := make([]*financev1.Installment, 0, len(in))
	for _, i := range in {
		out = append(out, &financev1.Installment{Month: int32(i.Month), AmountCents: i.AmountCents})
	}
	return out
}

func toProtoPolicy(p domain.CollectionPolicy) *financev1.CollectionPolicy {
	return &financev1.CollectionPolicy{
		DailyMode:              p.DailyMode,
		LateFeeDailyCents:      p.LateFeeDailyCents,
		SurchargeCountsAllDays: p.SurchargeCountsAllDays,
		EarlyPaymentCents:      p.EarlyPaymentCents,
		EarlyPaymentDays:       int32(p.EarlyPaymentDays),
	}
}
