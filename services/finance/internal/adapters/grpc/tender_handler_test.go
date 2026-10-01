package grpcadapter_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

// T8 across the wire: the tenders reach the use case, and every row comes back.
func TestRegisterPaymentCarriesTenders(t *testing.T) {
	f := newFixture(t)
	charge := f.seedCharge(t, f.seedConcept(t, "Uniformes", 100000).GetId(), 100000)

	res, err := f.h.RegisterPayment(context.Background(), connect.NewRequest(&financev1.RegisterPaymentRequest{
		ChargeId: charge.GetId(), AmountCents: 100000, IdempotencyKey: "mix-1",
		Tenders: []*financev1.Tender{
			{Method: domain.MethodCreditNote, AmountCents: 30000, Reference: "NC-3"},
			{Method: domain.MethodCard, AmountCents: 70000, CardType: domain.CardDebit},
		},
	}))
	if err != nil {
		t.Fatalf("RegisterPayment: %v", err)
	}
	rows := res.Msg.GetPayments()
	if len(rows) != 2 || rows[0].GetMethod() != domain.MethodCreditNote || rows[0].GetReference() != "NC-3" ||
		rows[1].GetMethod() != domain.MethodCard || rows[1].GetAmountCents() != 70000 || rows[1].GetCardType() != domain.CardDebit {
		t.Fatalf("payments = %+v", rows)
	}
	if res.Msg.GetPayment().GetId() != rows[0].GetId() {
		t.Error("payment is not the first row")
	}
	if res.Msg.GetCharge().GetRemainingCents() != 0 {
		t.Errorf("remaining = %d, want 0", res.Msg.GetCharge().GetRemainingCents())
	}

	if _, err := f.h.RegisterPayment(context.Background(), connect.NewRequest(&financev1.RegisterPaymentRequest{
		ChargeId: charge.GetId(), AmountCents: 50000,
		Tenders: []*financev1.Tender{{Method: domain.MethodCash, AmountCents: 10000}},
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("tenders short of the amount: want InvalidArgument, got %v", err)
	}
}

// A deposit's tenders and the cash-up's credit-note line reach the proto.
func TestDepositTendersAndCreditNotesCrossTheWire(t *testing.T) {
	f := newFixture(t)
	d, err := domain.NewDeposit("dep-1", testTenant, "st-1", 300000, domain.MethodCreditNote, "", fixedNow, "", "")
	if err != nil {
		t.Fatalf("NewDeposit: %v", err)
	}
	note, _ := domain.NewTender(domain.MethodCreditNote, 100000, "NC-1", "")
	cash, _ := domain.NewTender(domain.MethodCash, 200000, "", "")
	d, err = d.WithTenders([]domain.Tender{note, cash})
	if err != nil {
		t.Fatalf("WithTenders: %v", err)
	}
	f.repo.deposits = append(f.repo.deposits, d)

	res, err := f.h.DailyCashUp(context.Background(), connect.NewRequest(&financev1.DailyCashUpRequest{Day: fixedNow.Format("2006-01-02")}))
	if err != nil {
		t.Fatalf("DailyCashUp: %v", err)
	}
	if res.Msg.GetCreditNoteCents() != 100000 || res.Msg.GetTotalCents() != 200000 || res.Msg.GetCashCents() != 200000 {
		t.Errorf("credit notes %d, total %d, cash %d; want 100000, 200000, 200000",
			res.Msg.GetCreditNoteCents(), res.Msg.GetTotalCents(), res.Msg.GetCashCents())
	}
	if len(res.Msg.GetDeposits()) != 1 {
		t.Fatalf("deposits = %d, want 1", len(res.Msg.GetDeposits()))
	}
	got := res.Msg.GetDeposits()[0]
	if got.GetMethod() != domain.MethodMixed || len(got.GetTenders()) != 2 ||
		got.GetTenders()[0].GetReference() != "NC-1" || got.GetTenders()[1].GetAmountCents() != 200000 {
		t.Errorf("deposit = %+v", got)
	}
}
