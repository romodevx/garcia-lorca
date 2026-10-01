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
		EnrollmentDueOn:    "2026-06-30",
		EnrollmentStartsOn: "2026-06-01",
	}

	res, err := f.h.UpsertTuitionPlan(context.Background(), connect.NewRequest(sent))
	if err != nil {
		t.Fatalf("UpsertTuitionPlan: %v", err)
	}
	got := res.Msg.GetPlan()

	if got.GetEnrollmentDueOn() != "2026-06-30" {
		t.Errorf("enrollment_due_on came back %q, sent 2026-06-30", got.GetEnrollmentDueOn())
	}
	if got.GetEnrollmentStartsOn() != "2026-06-01" {
		t.Errorf("enrollment_starts_on came back %q, sent 2026-06-01", got.GetEnrollmentStartsOn())
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
	if len(stored.GetFees()) != 2 || stored.GetEnrollmentDueOn() != "2026-06-30" ||
		stored.GetEnrollmentStartsOn() != "2026-06-01" {
		t.Errorf("the listed plan lost fields: %d fees, window %q..%q",
			len(stored.GetFees()), stored.GetEnrollmentStartsOn(), stored.GetEnrollmentDueOn())
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

// The inscription's price ladder must survive the wire too, in both directions.
//
// Same test, same reason, third time: the field exists in the proto and in the
// domain, and an adapter that does not carry it turns a saved price list into a
// silent nothing. A ladder is worse than most: enrollment_cents is forced to
// zero whenever concepts are present, so dropping them stores an inscription
// that costs the family nothing at all.
func TestAPriceLadderSurvivesTheWireWhole(t *testing.T) {
	f := newFixture(t)
	sent := &financev1.UpsertTuitionPlanRequest{
		CycleId: testCycle, LevelId: "lv-ladder",
		MonthlyCents: 320000, Months: 11, FirstBilledMonth: 9, DueDay: 10, Active: true,
		EnrollmentConcepts: []*financev1.EnrollmentConcept{
			{Name: "Inscripción", Tiers: []*financev1.PriceTier{
				{EffectiveFrom: "2027-01-01", AmountCents: 220000},
				{EffectiveFrom: "2027-03-01", AmountCents: 330000},
				{EffectiveFrom: "2027-04-01", AmountCents: 440000},
			}},
			{Name: "Gastos adm y seguro", Tiers: []*financev1.PriceTier{
				{EffectiveFrom: "2027-01-01", AmountCents: 470000},
			}},
			{Name: "Cuota Asociación Padres de Familia", Tiers: []*financev1.PriceTier{
				{EffectiveFrom: "2027-01-01", AmountCents: 85000},
			}},
		},
	}

	res, err := f.h.UpsertTuitionPlan(context.Background(), connect.NewRequest(sent))
	if err != nil {
		t.Fatalf("UpsertTuitionPlan: %v", err)
	}
	got := res.Msg.GetPlan().GetEnrollmentConcepts()
	if len(got) != 3 {
		t.Fatalf("got %d concepts back, want the 3 that were sent", len(got))
	}

	for i, want := range sent.GetEnrollmentConcepts() {
		if got[i].GetName() != want.GetName() {
			t.Errorf("concept %d name = %q, want %q", i, got[i].GetName(), want.GetName())
		}
		if len(got[i].GetTiers()) != len(want.GetTiers()) {
			t.Fatalf("concept %q: %d steps back, want %d", want.GetName(),
				len(got[i].GetTiers()), len(want.GetTiers()))
		}
		for j, wantTier := range want.GetTiers() {
			gotTier := got[i].GetTiers()[j]
			if gotTier.GetEffectiveFrom() != wantTier.GetEffectiveFrom() {
				t.Errorf("concept %q step %d starts %q, want %q", want.GetName(), j,
					gotTier.GetEffectiveFrom(), wantTier.GetEffectiveFrom())
			}
			if gotTier.GetAmountCents() != wantTier.GetAmountCents() {
				t.Errorf("concept %q step %d = %d, want %d", want.GetName(), j,
					gotTier.GetAmountCents(), wantTier.GetAmountCents())
			}
		}
	}
}

// T5: an inscription that opens after its deadline, or a malformed opening day,
// is refused as invalid — never stored as "no start".
func TestABadInscriptionWindowIsRefused(t *testing.T) {
	for name, starts := range map[string]string{
		"after the deadline": "2026-07-01",
		"malformed":          "01/06/2026",
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			_, err := f.h.UpsertTuitionPlan(context.Background(), connect.NewRequest(&financev1.UpsertTuitionPlanRequest{
				CycleId: testCycle, LevelId: "lv-1",
				MonthlyCents: 320000, Months: 10, FirstBilledMonth: 9, DueDay: 10, Active: true,
				EnrollmentDueOn: "2026-06-30", EnrollmentStartsOn: starts,
			}))
			if connect.CodeOf(err) != connect.CodeInvalidArgument {
				t.Errorf("got %v, want invalid argument", err)
			}
		})
	}
}
