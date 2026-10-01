package domain_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

// T5: the window a charge is open in. One rule, read by the plan and by the
// charge, so the editor and the ledger cannot disagree on what is valid.
func TestValidateChargeWindow(t *testing.T) {
	cases := []struct {
		name      string
		starts    time.Time
		due       time.Time
		wantValid bool
		wantKind  glerrors.Kind
	}{
		{name: "no start is the charge as it always was", due: day(2027, 6, 30), wantValid: true},
		{name: "no dates at all", wantValid: true},
		{name: "start before the deadline", starts: day(2027, 1, 10), due: day(2027, 6, 30), wantValid: true},
		{name: "start on the deadline itself", starts: day(2027, 6, 30), due: day(2027, 6, 30), wantValid: true},
		{name: "start after the deadline", starts: day(2027, 7, 1), due: day(2027, 6, 30), wantKind: glerrors.KindInvalid},
		{name: "a start with no deadline", starts: day(2027, 1, 10), wantKind: glerrors.KindInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := domain.ValidateChargeWindow(tc.starts, tc.due)
			if tc.wantValid {
				if err != nil {
					t.Fatalf("want valid, got %v", err)
				}
				return
			}
			if glerrors.KindOf(err) != tc.wantKind {
				t.Fatalf("want kind %v, got %v", tc.wantKind, err)
			}
		})
	}
}

func TestATuitionPlanCarriesTheInscriptionWindow(t *testing.T) {
	starts, due := day(2027, 1, 10), day(2027, 6, 30)
	p, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		CycleID: "cyc", LevelID: "lvl", MonthlyCents: 100000, Months: 10, FirstBilledMonth: 9, DueDay: 10,
		EnrollmentStartsOn: starts, EnrollmentDueOn: due,
	})
	if err != nil {
		t.Fatalf("NewTuitionPlan: %v", err)
	}
	if !p.EnrollmentStartsOn().Equal(starts) || !p.EnrollmentDueOn().Equal(due) {
		t.Fatalf("window = %s..%s, want %s..%s", p.EnrollmentStartsOn(), p.EnrollmentDueOn(), starts, due)
	}
}

func TestATuitionPlanRefusesAnInscriptionThatStartsAfterItsDeadline(t *testing.T) {
	_, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		CycleID: "cyc", LevelID: "lvl", MonthlyCents: 100000, Months: 10, FirstBilledMonth: 9, DueDay: 10,
		EnrollmentStartsOn: day(2027, 7, 1), EnrollmentDueOn: day(2027, 6, 30),
	})
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("want KindInvalid, got %v", err)
	}
}

func TestAChargeCarriesTheDayItOpens(t *testing.T) {
	c := validCharge(t, 100000, 0) // due 2026-09-05
	opened, err := c.WithStartsOn(day(2026, 9, 1))
	if err != nil {
		t.Fatalf("WithStartsOn: %v", err)
	}
	if !opened.StartsOn().Equal(day(2026, 9, 1)) {
		t.Fatalf("StartsOn = %s", opened.StartsOn())
	}
	if _, err := c.WithStartsOn(day(2026, 9, 6)); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("a charge opening after its deadline: want KindInvalid, got %v", err)
	}
}

func TestAChargeIsOpenFromItsStartDay(t *testing.T) {
	plain := validCharge(t, 100000, 0)
	if !plain.OpenOn(day(2020, 1, 1)) {
		t.Fatal("a charge with no start date is open from the day it exists")
	}
	c, err := plain.WithStartsOn(day(2026, 9, 1))
	if err != nil {
		t.Fatalf("WithStartsOn: %v", err)
	}
	cases := []struct {
		on   time.Time
		want bool
	}{
		{day(2026, 8, 31), false},
		// The school's day is the UTC day: late on the eve is still the eve.
		{time.Date(2026, 8, 31, 23, 59, 0, 0, time.UTC), false},
		{day(2026, 9, 1), true},
		{time.Date(2026, 9, 1, 15, 0, 0, 0, time.UTC), true},
		{day(2026, 9, 20), true},
	}
	for _, tc := range cases {
		if got := c.OpenOn(tc.on); got != tc.want {
			t.Errorf("OpenOn(%s) = %v, want %v", tc.on.Format(time.RFC3339), got, tc.want)
		}
	}
}
