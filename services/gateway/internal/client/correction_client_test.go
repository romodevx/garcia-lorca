package client

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

// T9, both directions of the wire: the correction request carries every field
// the office typed plus who corrected it; the answer brings back both ends of
// the link and the history row.
func TestCorrectionCrossesTheWireBothWays(t *testing.T) {
	paidAt := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	req := toCorrectRequest(CorrectionInput{
		DepositID: "dep-1", AmountCents: 100000, PaidAt: paidAt,
		Tenders:   []Tender{{Method: "cash", AmountCents: 100000}},
		ChargeIDs: []string{"ch-1"}, Reason: "monto", IdempotencyKey: "k-1",
		By: Receiver{UserID: "u-1", Name: "Directora"},
	})
	if req.GetDepositId() != "dep-1" || req.GetAmountCents() != 100000 || !req.GetPaidAt().AsTime().Equal(paidAt) ||
		len(req.GetTenders()) != 1 || req.GetChargeIds()[0] != "ch-1" || req.GetReason() != "monto" ||
		req.GetIdempotencyKey() != "k-1" || req.GetCorrectedBy() != "u-1" || req.GetCorrectedByName() != "Directora" {
		t.Errorf("request = %+v", req)
	}
	if toCorrectRequest(CorrectionInput{DepositID: "d"}).GetPaidAt() != nil {
		t.Error("an empty date must stay empty (keep the original's)")
	}

	at := timestamppb.New(paidAt)
	got := toCorrectionResult(&financev1.CorrectDepositResponse{
		Original: &financev1.Deposit{Id: "dep-1", SupersededAt: at, SupersededBy: "dep-2", SupersededByFolio: "P-2026-000002"},
		Deposit:  &financev1.Deposit{Id: "dep-2", CorrectsDepositId: "dep-1", CorrectsFolio: "P-2026-000001", Folio: "P-2026-000002"},
		Correction: &financev1.DepositCorrection{
			Id: "c-1", OriginalDepositId: "dep-1", CorrectedDepositId: "dep-2", Reason: "monto",
			Before:          &financev1.DepositSnapshot{AmountCents: 50000, Folio: "P-2026-000001"},
			After:           &financev1.DepositSnapshot{AmountCents: 100000, PaidAt: at, Tenders: []*financev1.Tender{{Method: "cash", AmountCents: 100000}}, Lines: []*financev1.SnapshotLine{{ChargeId: "ch-1", Description: "Colegiatura", AmountCents: 100000}}},
			CorrectedByName: "Directora", CorrectedAt: at,
		},
		Replayed: true,
	})
	if got.Original.SupersededBy != "dep-2" || got.Original.SupersededByFolio != "P-2026-000002" || got.Original.SupersededAt == "" {
		t.Errorf("original = %+v", got.Original)
	}
	if got.Deposit.CorrectsDepositID != "dep-1" || got.Deposit.CorrectsFolio != "P-2026-000001" || !got.Replayed {
		t.Errorf("deposit = %+v", got.Deposit)
	}
	c := got.Correction
	if c.Reason != "monto" || c.CorrectedByName != "Directora" || c.Before.AmountCents != 50000 || c.After.AmountCents != 100000 ||
		c.After.Lines[0].Description != "Colegiatura" || c.After.Tenders[0].Method != "cash" || c.CorrectedAt == "" || c.After.PaidAt == "" {
		t.Errorf("correction = %+v", c)
	}
}

func TestTheCashUpAndStatementCarryTheSuperseded(t *testing.T) {
	cut := toCashUp(&financev1.DailyCashUpResponse{Superseded: []*financev1.Deposit{{Id: "dep-1", SupersededByFolio: "P-2"}}})
	if len(cut.Superseded) != 1 || cut.Superseded[0].SupersededByFolio != "P-2" {
		t.Errorf("cut = %+v", cut)
	}
	st := toStatement(&financev1.GetStatementResponse{Movements: []*financev1.StatementMovement{
		{DepositId: "dep-1", Superseded: true, SupersededByFolio: "P-2"},
		{DepositId: "dep-2", CorrectsFolio: "P-1"},
	}})
	if !st.Movements[0].Superseded || st.Movements[0].SupersededByFolio != "P-2" || st.Movements[1].CorrectsFolio != "P-1" {
		t.Errorf("movements = %+v", st.Movements)
	}
}

// T9: a payment carries the deposit it is a slice of, to the front.
func TestAPaymentCarriesItsDeposit(t *testing.T) {
	if p := toPayment(&financev1.Payment{Id: "p1", DepositId: "dep-1"}); p.DepositID != "dep-1" {
		t.Errorf("payment deposit = %q, want dep-1", p.DepositID)
	}
}
