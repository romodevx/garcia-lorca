package domain_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

func planWith(t *testing.T, months int, monthly int64, schedule []domain.Installment) domain.TuitionPlan {
	t.Helper()
	p, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: "tp-1", TenantID: "t1", CycleID: "c1", LevelID: "lv1",
		EnrollmentCents: 200000, MonthlyCents: monthly, Months: months,
		FirstBilledMonth: 9, DueDay: 10, Active: true, Schedule: schedule,
	})
	if err != nil {
		t.Fatalf("NewTuitionPlan: %v", err)
	}
	return p
}

var cycleStart = time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)

// With no schedule the plan bills what it always billed: one equal payment a
// month. Every school that never asks for anything fancy keeps working.
func TestWithoutAScheduleTheMonthsAreEqual(t *testing.T) {
	plan := planWith(t, 11, 100000, nil)

	due := plan.Installments(cycleStart)

	if len(due) != 11 {
		t.Fatalf("%d installments, want 11", len(due))
	}
	for i, d := range due {
		if d.AmountCents != 100000 {
			t.Errorf("installment %d = %d, want 100000", i, d.AmountCents)
		}
	}
	if due[0].DueOn.Month() != time.September {
		t.Errorf("the first falls in %v, want September", due[0].DueOn.Month())
	}
}

// The school's own example: eleven months, but December is halved and the other
// half is collected in March. The year still adds up to the same money.
func TestAMonthCanBeSplitAcrossTwoDates(t *testing.T) {
	plan := planWith(t, 11, 100000, []domain.Installment{
		{Month: 9, AmountCents: 100000},
		{Month: 10, AmountCents: 100000},
		{Month: 11, AmountCents: 100000},
		{Month: 12, AmountCents: 50000},
		{Month: 1, AmountCents: 100000},
		{Month: 2, AmountCents: 100000},
		{Month: 3, AmountCents: 150000},
		{Month: 4, AmountCents: 100000},
		{Month: 5, AmountCents: 100000},
		{Month: 6, AmountCents: 100000},
		{Month: 7, AmountCents: 100000},
	})

	due := plan.Installments(cycleStart)

	if len(due) != 11 {
		t.Fatalf("%d installments, want 11", len(due))
	}
	var total int64
	for _, d := range due {
		total += d.AmountCents
	}
	if total != 1100000 {
		t.Errorf("the year adds up to %d, want 1100000", total)
	}
	if due[3].AmountCents != 50000 || due[3].DueOn.Month() != time.December {
		t.Errorf("December = %+v, want half in December", due[3])
	}
	if due[6].AmountCents != 150000 || due[6].DueOn.Month() != time.March {
		t.Errorf("March = %+v, want a month and a half", due[6])
	}
}

// A month that runs past December belongs to the next calendar year. Getting
// this wrong bills January before September.
func TestTheScheduleCrossesTheYear(t *testing.T) {
	plan := planWith(t, 2, 100000, []domain.Installment{
		{Month: 12, AmountCents: 100000},
		{Month: 1, AmountCents: 100000},
	})

	due := plan.Installments(cycleStart)

	if due[0].DueOn.Year() != 2026 || due[1].DueOn.Year() != 2027 {
		t.Errorf("years = %d and %d, want 2026 then 2027", due[0].DueOn.Year(), due[1].DueOn.Year())
	}
}

// An installment of nothing is a charge for nothing, and a month outside 1-12
// is a typo. Both are refused where the message can say which.
func TestAScheduleIsValidated(t *testing.T) {
	cases := map[string][]domain.Installment{
		"month zero":     {{Month: 0, AmountCents: 100000}},
		"month thirteen": {{Month: 13, AmountCents: 100000}},
		"no money":       {{Month: 9, AmountCents: 0}},
		"negative":       {{Month: 9, AmountCents: -100}},
	}
	for name, schedule := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
				ID: "tp-1", TenantID: "t1", CycleID: "c1", LevelID: "lv1",
				MonthlyCents: 100000, Months: 1, FirstBilledMonth: 9, DueDay: 10,
				Active: true, Schedule: schedule,
			})
			if glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("%s: want KindInvalid, got %v", name, err)
			}
		})
	}
}
