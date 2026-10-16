package client

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

// CorrectionInput is a payment correction as the office asks for it (T9).
type CorrectionInput struct {
	DepositID      string
	AmountCents    int64
	PaidAt         time.Time
	Tenders        []Tender
	ChargeIDs      []string
	Reason         string
	IdempotencyKey string
	// By is the authenticated caller — never a field of the request body.
	By Receiver
}

// SnapshotLine is what one side of a correction put on a charge.
type SnapshotLine struct {
	ChargeID    string `json:"chargeId"`
	Description string `json:"description,omitempty"`
	AmountCents int64  `json:"amountCents"`
}

// DepositSnapshot is a deposit as it stood on one side of a correction.
type DepositSnapshot struct {
	DepositID   string `json:"depositId"`
	Folio       string `json:"folio"`
	AmountCents int64  `json:"amountCents"`
	PaidAt      string `json:"paidAt,omitempty"`
	// PaidOn is the school-local day (ADR 0035); empty on snapshots written
	// before it was kept.
	PaidOn  string         `json:"paidOn,omitempty"`
	Tenders []Tender       `json:"tenders"`
	Lines   []SnapshotLine `json:"lines"`
}

// DepositCorrection is one correction: before, after, who, when and why.
type DepositCorrection struct {
	ID                 string          `json:"id"`
	OriginalDepositID  string          `json:"originalDepositId"`
	CorrectedDepositID string          `json:"correctedDepositId"`
	Reason             string          `json:"reason"`
	Before             DepositSnapshot `json:"before"`
	After              DepositSnapshot `json:"after"`
	CorrectedByName    string          `json:"correctedByName,omitempty"`
	CorrectedAt        string          `json:"correctedAt"`
}

// CorrectionResult is what a correction did.
type CorrectionResult struct {
	Original         Deposit           `json:"original"`
	Deposit          Deposit           `json:"deposit"`
	Allocations      []Payment         `json:"allocations"`
	Surcharges       []Charge          `json:"surcharges"`
	UnallocatedCents int64             `json:"unallocatedCents"`
	Replayed         bool              `json:"replayed"`
	Correction       DepositCorrection `json:"correction"`
}

// CorrectDeposit corrects a registered payment (T9).
func (c *FinanceClient) CorrectDeposit(ctx context.Context, in CorrectionInput) (CorrectionResult, error) {
	res, err := c.rpc.CorrectDeposit(ctx, connect.NewRequest(toCorrectRequest(in)))
	if err != nil {
		return CorrectionResult{}, err
	}
	return toCorrectionResult(res.Msg), nil
}

// DepositCorrections is a receipt's correction history, its chain whole.
func (c *FinanceClient) DepositCorrections(ctx context.Context, depositID string) ([]DepositCorrection, error) {
	res, err := c.rpc.ListDepositCorrections(ctx, connect.NewRequest(&financev1.ListDepositCorrectionsRequest{DepositId: depositID}))
	if err != nil {
		return nil, err
	}
	out := make([]DepositCorrection, 0, len(res.Msg.GetCorrections()))
	for _, c := range res.Msg.GetCorrections() {
		out = append(out, toCorrection(c))
	}
	return out, nil
}

func toCorrectRequest(in CorrectionInput) *financev1.CorrectDepositRequest {
	req := &financev1.CorrectDepositRequest{
		DepositId: in.DepositID, AmountCents: in.AmountCents, Tenders: toProtoTenders(in.Tenders),
		ChargeIds: in.ChargeIDs, Reason: in.Reason, IdempotencyKey: in.IdempotencyKey,
		CorrectedBy: in.By.UserID, CorrectedByName: in.By.Name,
	}
	if !in.PaidAt.IsZero() {
		req.PaidAt = timestamppb.New(in.PaidAt)
	}
	return req
}

func toCorrectionResult(m *financev1.CorrectDepositResponse) CorrectionResult {
	allocations := make([]Payment, 0, len(m.GetAllocations()))
	for _, p := range m.GetAllocations() {
		allocations = append(allocations, toPayment(p))
	}
	return CorrectionResult{
		Original: toDeposit(m.GetOriginal()), Deposit: toDeposit(m.GetDeposit()),
		Allocations: allocations, Surcharges: toCharges(m.GetSurcharges()),
		UnallocatedCents: m.GetUnallocatedCents(), Replayed: m.GetReplayed(),
		Correction: toCorrection(m.GetCorrection()),
	}
}

func toCorrection(c *financev1.DepositCorrection) DepositCorrection {
	out := DepositCorrection{
		ID: c.GetId(), OriginalDepositID: c.GetOriginalDepositId(), CorrectedDepositID: c.GetCorrectedDepositId(),
		Reason: c.GetReason(), Before: toSnapshot(c.GetBefore()), After: toSnapshot(c.GetAfter()),
		CorrectedByName: c.GetCorrectedByName(),
	}
	if ts := c.GetCorrectedAt(); ts != nil {
		out.CorrectedAt = ts.AsTime().Format(time.RFC3339)
	}
	return out
}

func toSnapshot(s *financev1.DepositSnapshot) DepositSnapshot {
	out := DepositSnapshot{
		DepositID: s.GetDepositId(), Folio: s.GetFolio(), AmountCents: s.GetAmountCents(), PaidOn: s.GetPaidOn(),
		Tenders: fromProtoTenders(s.GetTenders()), Lines: make([]SnapshotLine, 0, len(s.GetLines())),
	}
	if ts := s.GetPaidAt(); ts != nil {
		out.PaidAt = ts.AsTime().Format(time.RFC3339)
	}
	for _, l := range s.GetLines() {
		out.Lines = append(out.Lines, SnapshotLine{ChargeID: l.GetChargeId(), Description: l.GetDescription(), AmountCents: l.GetAmountCents()})
	}
	return out
}
