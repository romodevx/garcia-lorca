package client

import (
	"testing"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

// The mapping this pins was silently missing twice: a field existed on the
// struct, carried a JSON tag, and was never copied to the proto. The screen
// saved a plan, the API answered zeros, and everything compiled and linted.
func TestTuitionPlanRequestCarriesEveryField(t *testing.T) {
	sent := tuitionPlanRequest(TuitionPlanInput{
		CycleID: "c1", LevelID: "lv1", EnrollmentCents: 500000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, LateFeeCents: 15000, Active: true,
		MonthlyDiscountCents: 50000,
		Schedule:             []Installment{{Month: 9, AmountCents: 320000}},
		Policy: CollectionPolicy{
			DailyMode: true, LateFeeDailyCents: 1000, SurchargeCountsAllDays: true,
			EarlyPaymentCents: 10000, EarlyPaymentDays: 5,
		},
	})

	if sent.GetMonthlyDiscountCents() != 50000 {
		t.Errorf("the level discount was not sent: %d", sent.GetMonthlyDiscountCents())
	}
	if len(sent.GetSchedule()) != 1 || sent.GetSchedule()[0].GetAmountCents() != 320000 {
		t.Errorf("the payment plan was not sent: %+v", sent.GetSchedule())
	}
	if !sent.GetPolicy().GetDailyMode() || sent.GetPolicy().GetLateFeeDailyCents() != 1000 {
		t.Errorf("the collection rule was not sent: %+v", sent.GetPolicy())
	}
	if sent.GetPolicy().GetEarlyPaymentDays() != 5 || sent.GetPolicy().GetEarlyPaymentCents() != 10000 {
		t.Errorf("the early-payment reward was not sent: %+v", sent.GetPolicy())
	}
	if sent.GetEnrollmentCents() != 500000 || sent.GetLateFeeCents() != 15000 {
		t.Errorf("the plain amounts were not sent: %+v", sent)
	}
}

// And back: the same fields have to survive the answer, or the screen reopens
// showing a plan the school did not save.
func TestTuitionPlanReadsEveryFieldBack(t *testing.T) {
	got := toTuitionPlan(&financev1.TuitionPlan{
		Id: "tp-1", CycleId: "c1", LevelId: "lv1",
		MonthlyCents: 320000, MonthlyDiscountCents: 50000,
		Schedule: []*financev1.Installment{{Month: 12, AmountCents: 160000}},
		Policy:   &financev1.CollectionPolicy{DailyMode: true, LateFeeDailyCents: 1000},
	})

	if got.MonthlyDiscountCents != 50000 {
		t.Errorf("the level discount was not read back: %d", got.MonthlyDiscountCents)
	}
	if len(got.Schedule) != 1 || got.Schedule[0].Month != 12 {
		t.Errorf("the payment plan was not read back: %+v", got.Schedule)
	}
	if !got.Policy.DailyMode || got.Policy.LateFeeDailyCents != 1000 {
		t.Errorf("the collection rule was not read back: %+v", got.Policy)
	}
}
