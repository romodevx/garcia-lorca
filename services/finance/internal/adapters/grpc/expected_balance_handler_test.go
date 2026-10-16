package grpcadapter_test

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

// D1 (T5 review): the wire carries which lines the balance holds and the
// "si paga a tiempo" figure; skip_balance spares the ledger for the search.
func TestExpectedTodayCarriesTheBalanceItHolds(t *testing.T) {
	f := newFixture(t)
	concept := f.seedConcept(t, "Taller", 100000)
	charge := f.seedCharge(t, concept.GetId(), 100000)

	ask := func(skip bool) *financev1.ExpectedTodayResponse {
		t.Helper()
		res, err := f.h.ExpectedToday(context.Background(), connect.NewRequest(&financev1.ExpectedTodayRequest{
			StudentId: testStudent, SkipBalance: skip,
		}))
		if err != nil {
			t.Fatalf("ExpectedToday: %v", err)
		}
		return res.Msg
	}

	full := ask(false)
	if full.GetAnnualBalanceCents() != 100000 || full.GetOnTimeBalanceCents() != 100000 {
		t.Errorf("balance = %d, on time = %d, want 100000 both", full.GetAnnualBalanceCents(), full.GetOnTimeBalanceCents())
	}
	if len(full.GetLines()) != 1 || full.GetLines()[0].GetChargeId() != charge.GetId() || !full.GetLines()[0].GetInBalance() {
		t.Errorf("lines = %v, want the open charge in the balance", full.GetLines())
	}

	quick := ask(true)
	if quick.GetAnnualBalanceCents() != 0 || quick.GetOnTimeBalanceCents() != 0 {
		t.Errorf("skip_balance still read the balance: %d / %d", quick.GetAnnualBalanceCents(), quick.GetOnTimeBalanceCents())
	}
	if quick.GetAmountCents() != full.GetAmountCents() {
		t.Errorf("amount = %d, want %d", quick.GetAmountCents(), full.GetAmountCents())
	}
}
