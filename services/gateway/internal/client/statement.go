package client

import (
	"context"

	"connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

// Statement is a student's ledger (T7), as finance worked it out: the movements
// in date order with the running balance, the totals, and the counter's
// figures for today. It is also the JSON the gateway answers with.
type Statement struct {
	Movements          []StatementMovement `json:"movements"`
	Totals             StatementTotals     `json:"totals"`
	DueTodayCents      int64               `json:"dueTodayCents"`
	SurchargeCents     int64               `json:"surchargeCents"`
	EarlyDiscountCents int64               `json:"earlyDiscountCents"`
	UnallocatedCents   int64               `json:"unallocatedCents"`
	AsOf               string              `json:"asOf"`
}

// StatementMovement is one line of the ledger. AmountCents is positive; Kind
// (charge | surcharge | discount | scholarship | payment | credit_note) says
// which way it moves the balance.
type StatementMovement struct {
	Kind         string `json:"kind"`
	Date         string `json:"date"`
	Description  string `json:"description"`
	AmountCents  int64  `json:"amountCents"`
	BalanceCents int64  `json:"balanceCents"`
	ChargeID     string `json:"chargeId,omitempty"`
	DepositID    string `json:"depositId,omitempty"`
	Method       string `json:"method,omitempty"`
	CardType     string `json:"cardType,omitempty"`
	Reference    string `json:"reference,omitempty"`
	Folio        string `json:"folio,omitempty"`
	Accrued      bool   `json:"accrued"`
	NotYetDue    bool   `json:"notYetDue"`
	Voided       bool   `json:"voided"`
	VoidReason   string `json:"voidReason,omitempty"`
	// T9: a receipt replaced by its correction (office only, not counted),
	// and on a correction's lines the receipt it replaces.
	Superseded        bool   `json:"superseded"`
	SupersededByFolio string `json:"supersededByFolio,omitempty"`
	CorrectsFolio     string `json:"correctsFolio,omitempty"`
	// AppliedTo is, on a deposit's lines, what that deposit paid
	// (ISSUE-T14-106); absent on every other line.
	AppliedTo []StatementAppliedPart `json:"appliedTo,omitempty"`
}

// StatementAppliedPart is the part of a deposit one charge took.
type StatementAppliedPart struct {
	ChargeID    string `json:"chargeId"`
	Description string `json:"description"`
	AmountCents int64  `json:"amountCents"`
}

// StatementTotals: BalanceCents = ChargedCents − DiscountCents − PaidCents.
type StatementTotals struct {
	ChargedCents   int64 `json:"chargedCents"`
	DiscountCents  int64 `json:"discountCents"`
	PaidCents      int64 `json:"paidCents"`
	BalanceCents   int64 `json:"balanceCents"`
	NotYetDueCents int64 `json:"notYetDueCents"`
}

// GetStatement is the office's ledger: charges not yet opened are marked,
// voided deposits are struck through, neither counts.
func (c *FinanceClient) GetStatement(ctx context.Context, studentID string) (Statement, error) {
	return c.statement(ctx, studentID, false)
}

// GetFamilyStatement is the family's ledger: the same balance, without what
// has not opened and without voided deposits.
func (c *FinanceClient) GetFamilyStatement(ctx context.Context, studentID string) (Statement, error) {
	return c.statement(ctx, studentID, true)
}

func (c *FinanceClient) statement(ctx context.Context, studentID string, familyView bool) (Statement, error) {
	res, err := c.rpc.GetStatement(ctx, connect.NewRequest(&financev1.GetStatementRequest{
		StudentId: studentID, FamilyView: familyView,
	}))
	if err != nil {
		return Statement{}, err
	}
	return toStatement(res.Msg), nil
}

// toAppliedParts maps what a deposit's line paid; nil when it paid nothing
// named, so the JSON omits it.
func toAppliedParts(parts []*financev1.StatementAppliedPart) []StatementAppliedPart {
	if len(parts) == 0 {
		return nil
	}
	out := make([]StatementAppliedPart, 0, len(parts))
	for _, p := range parts {
		out = append(out, StatementAppliedPart{ChargeID: p.GetChargeId(), Description: p.GetDescription(), AmountCents: p.GetAmountCents()})
	}
	return out
}

func toStatement(m *financev1.GetStatementResponse) Statement {
	movements := make([]StatementMovement, 0, len(m.GetMovements()))
	for _, mv := range m.GetMovements() {
		movements = append(movements, StatementMovement{
			Kind: mv.GetKind(), Date: mv.GetDate(), Description: mv.GetDescription(),
			AmountCents: mv.GetAmountCents(), BalanceCents: mv.GetBalanceCents(),
			ChargeID: mv.GetChargeId(), DepositID: mv.GetDepositId(),
			Method: mv.GetMethod(), CardType: mv.GetCardType(), Reference: mv.GetReference(), Folio: mv.GetFolio(),
			Accrued: mv.GetAccrued(), NotYetDue: mv.GetNotYetDue(), Voided: mv.GetVoided(), VoidReason: mv.GetVoidReason(),
			Superseded: mv.GetSuperseded(), SupersededByFolio: mv.GetSupersededByFolio(), CorrectsFolio: mv.GetCorrectsFolio(),
			AppliedTo: toAppliedParts(mv.GetAppliedTo()),
		})
	}
	t := m.GetTotals()
	return Statement{
		Movements: movements,
		Totals: StatementTotals{
			ChargedCents: t.GetChargedCents(), DiscountCents: t.GetDiscountCents(), PaidCents: t.GetPaidCents(),
			BalanceCents: t.GetBalanceCents(), NotYetDueCents: t.GetNotYetDueCents(),
		},
		DueTodayCents:      m.GetDueTodayCents(),
		SurchargeCents:     m.GetSurchargeCents(),
		EarlyDiscountCents: m.GetEarlyDiscountCents(),
		UnallocatedCents:   m.GetUnallocatedCents(),
		AsOf:               m.GetAsOf(),
	}
}
