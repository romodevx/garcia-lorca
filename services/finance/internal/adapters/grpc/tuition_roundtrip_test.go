package grpcadapter_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

// Every field of a tuition table must survive the wire, in both directions.
//
// This test exists because the same bug has been shipped twice: a new field is
// added to the plan, the adapter that maps it is not updated, and the panel
// looks like it saved. Nothing errors. The school finds out when the billing
// run charges last month's rules.
func TestATuitionPlanSurvivesTheWireWhole(t *testing.T) {
	f := newFixture(t)
	sent := &financev1.UpsertTuitionPlanRequest{
		CycleId: testCycle, LevelId: "lv-1",
		EnrollmentCents: 500000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, LateFeeCents: 15000, Active: true,
		Policy: &financev1.CollectionPolicy{
			DailyMode: true, LateFeeDailyCents: 1000,
			SurchargeCountsAllDays: true,
			EarlyPaymentCents:      10000, EarlyPaymentDays: 5,
		},
		MonthlyDiscountCents: 50000,
		Fees: []*financev1.Fee{
			{Name: "Cuota de Julio", AmountCents: 160000, Months: []int32{9, 10}},
			{Name: "Seguro escolar", AmountCents: 45000, Months: []int32{9}},
		},
		EnrollmentDueOn: "2026-06-30",
	}

	res, err := f.h.UpsertTuitionPlan(context.Background(), connect.NewRequest(sent))
	if err != nil {
		t.Fatalf("UpsertTuitionPlan: %v", err)
	}
	got := res.Msg.GetPlan()

	if got.GetEnrollmentDueOn() != "2026-06-30" {
		t.Errorf("enrollment_due_on came back %q, sent 2026-06-30", got.GetEnrollmentDueOn())
	}
	if len(got.GetFees()) != 2 {
		t.Fatalf("fees came back as %d, sent 2", len(got.GetFees()))
	}
	first := got.GetFees()[0]
	if first.GetName() != "Cuota de Julio" || first.GetAmountCents() != 160000 {
		t.Errorf("first fee came back as %q for %d", first.GetName(), first.GetAmountCents())
	}
	if len(first.GetMonths()) != 2 || first.GetMonths()[0] != 9 || first.GetMonths()[1] != 10 {
		t.Errorf("first fee's months came back as %v, sent [9 10]", first.GetMonths())
	}

	// And again on the way out of the list, which is the path the panel reads.
	listed, err := f.h.ListTuitionPlans(context.Background(),
		connect.NewRequest(&financev1.ListTuitionPlansRequest{CycleId: testCycle}))
	if err != nil {
		t.Fatalf("ListTuitionPlans: %v", err)
	}
	if len(listed.Msg.GetPlans()) != 1 {
		t.Fatalf("listed %d plans, want 1", len(listed.Msg.GetPlans()))
	}
	stored := listed.Msg.GetPlans()[0]
	if len(stored.GetFees()) != 2 || stored.GetEnrollmentDueOn() != "2026-06-30" {
		t.Errorf("the listed plan lost fields: %d fees, deadline %q",
			len(stored.GetFees()), stored.GetEnrollmentDueOn())
	}
}

// A date the school cannot mean is refused where the message can say so, not
// swallowed into "no deadline" — which would silently move every inscription
// back to the default rule.
func TestABadInscriptionDateIsRefused(t *testing.T) {
	f := newFixture(t)
	_, err := f.h.UpsertTuitionPlan(context.Background(), connect.NewRequest(&financev1.UpsertTuitionPlanRequest{
		CycleId: testCycle, LevelId: "lv-1",
		MonthlyCents: 320000, Months: 10, FirstBilledMonth: 9, DueDay: 10, Active: true,
		EnrollmentDueOn: "30/06/2026",
	}))
	if err == nil {
		t.Fatal("a malformed inscription deadline was accepted")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("got code %v, want invalid argument", connect.CodeOf(err))
	}
}
