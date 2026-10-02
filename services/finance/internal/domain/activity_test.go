package domain_test

import (
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

func validActivitySpec() domain.ActivitySpec {
	return domain.ActivitySpec{
		ID:   "act-1",
		Name: "  Natación ",
		Schedule: []domain.ActivitySlot{
			{Weekday: 2, StartMin: 15 * 60, EndMin: 16*60 + 30},
			{Weekday: 4, StartMin: 16 * 60, EndMin: 17 * 60},
		},
		Periodicity:     domain.PeriodicityMonthly,
		BillingStartsOn: day(2026, 9, 1),
		FirstDueOn:      day(2026, 9, 10),
		LateFeeCents:    5_000,
		Active:          true,
		Options: []domain.PriceOption{
			{ID: "opt-1", DaysPerWeek: 1, AmountCents: 40_000},
			{ID: "opt-2", DaysPerWeek: 2, AmountCents: 70_000},
			{ID: "opt-flat", DaysPerWeek: 0, AmountCents: 90_000},
		},
	}
}

func mustActivity(t *testing.T, mutate func(*domain.ActivitySpec)) domain.Activity {
	t.Helper()
	spec := validActivitySpec()
	if mutate != nil {
		mutate(&spec)
	}
	a, err := domain.NewActivity(spec)
	if err != nil {
		t.Fatalf("new activity: %v", err)
	}
	return a
}

func TestNewActivityTrimsAndKeepsEverything(t *testing.T) {
	a := mustActivity(t, nil)
	if a.Name() != "Natación" {
		t.Fatalf("name = %q", a.Name())
	}
	if len(a.Schedule()) != 2 || len(a.Options()) != 3 {
		t.Fatalf("schedule/options lost: %d/%d", len(a.Schedule()), len(a.Options()))
	}
	if a.LateFee().Int64() != 5_000 || !a.Active() || a.Periodicity() != domain.PeriodicityMonthly {
		t.Fatalf("fields lost: %+v", a)
	}
	if o, ok := a.Option("opt-flat"); !ok || !o.Flat() || o.AmountCents != 90_000 {
		t.Fatalf("flat option = %+v %v", o, ok)
	}
}

func TestNewActivityRejects(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*domain.ActivitySpec)
	}{
		{"blank name", func(s *domain.ActivitySpec) { s.Name = "  " }},
		{"long name", func(s *domain.ActivitySpec) { s.Name = strings.Repeat("a", 81) }},
		{"no id", func(s *domain.ActivitySpec) { s.ID = "" }},
		{"no schedule", func(s *domain.ActivitySpec) { s.Schedule = nil }},
		{"weekday 0", func(s *domain.ActivitySpec) { s.Schedule[0].Weekday = 0 }},
		{"weekday 8", func(s *domain.ActivitySpec) { s.Schedule[0].Weekday = 8 }},
		{"end before start", func(s *domain.ActivitySpec) { s.Schedule[0].EndMin = s.Schedule[0].StartMin }},
		{"past midnight", func(s *domain.ActivitySpec) { s.Schedule[0].EndMin = 24 * 60 }},
		{"overlap same day", func(s *domain.ActivitySpec) {
			s.Schedule = append(s.Schedule, domain.ActivitySlot{Weekday: 2, StartMin: 16 * 60, EndMin: 17 * 60})
		}},
		{"bad periodicity", func(s *domain.ActivitySpec) { s.Periodicity = "daily" }},
		{"no billing start", func(s *domain.ActivitySpec) { s.BillingStartsOn = time.Time{} }},
		{"no due", func(s *domain.ActivitySpec) { s.FirstDueOn = time.Time{} }},
		{"due before start", func(s *domain.ActivitySpec) { s.FirstDueOn = day(2026, 8, 31) }},
		{"monthly due in next period", func(s *domain.ActivitySpec) { s.FirstDueOn = day(2026, 10, 1) }},
		{"weekly due in next week", func(s *domain.ActivitySpec) {
			s.Periodicity = domain.PeriodicityWeekly
			s.FirstDueOn = day(2026, 9, 8)
		}},
		{"negative late fee", func(s *domain.ActivitySpec) { s.LateFeeCents = -1 }},
		{"no options", func(s *domain.ActivitySpec) { s.Options = nil }},
		{"zero amount", func(s *domain.ActivitySpec) { s.Options[0].AmountCents = 0 }},
		{"days 8", func(s *domain.ActivitySpec) { s.Options[0].DaysPerWeek = 8 }},
		{"negative days", func(s *domain.ActivitySpec) { s.Options[0].DaysPerWeek = -1 }},
		{"two options same days", func(s *domain.ActivitySpec) { s.Options[1].DaysPerWeek = 1 }},
		{"two flat fees", func(s *domain.ActivitySpec) { s.Options[0].DaysPerWeek = 0 }},
		{"option without id", func(s *domain.ActivitySpec) { s.Options[0].ID = "" }},
		{"too many options", func(s *domain.ActivitySpec) {
			for i := 0; i < 8; i++ {
				s.Options = append(s.Options, domain.PriceOption{ID: "x" + string(rune('a'+i)), DaysPerWeek: 0, AmountCents: 1})
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec := validActivitySpec()
			tc.mutate(&spec)
			_, err := domain.NewActivity(spec)
			if glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Fatalf("err = %v, want KindInvalid", err)
			}
		})
	}
}

func TestWeeklyDueOnLastDayOfTheWeekIsValid(t *testing.T) {
	mustActivity(t, func(s *domain.ActivitySpec) {
		s.Periodicity = domain.PeriodicityWeekly
		s.FirstDueOn = day(2026, 9, 7)
	})
}

func TestPerCycleDueMayBeAnyLaterDay(t *testing.T) {
	mustActivity(t, func(s *domain.ActivitySpec) {
		s.Periodicity = domain.PeriodicityPerCycle
		s.FirstDueOn = day(2027, 1, 15)
	})
}

func TestParseClock(t *testing.T) {
	got, err := domain.ParseClock("15:30")
	if err != nil || got != 15*60+30 {
		t.Fatalf("ParseClock = %d, %v", got, err)
	}
	if domain.FormatClock(got) != "15:30" || domain.FormatClock(7*60+5) != "07:05" {
		t.Fatalf("FormatClock round trip broke")
	}
	for _, bad := range []string{"", "24:00", "7:5", "15:60", "ab:cd", "15-30"} {
		if _, err := domain.ParseClock(bad); glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Errorf("ParseClock(%q) err = %v, want KindInvalid", bad, err)
		}
	}
}

func TestMonthlyPeriodsRepeatTheDaysAndClampAtMonthEnd(t *testing.T) {
	a := mustActivity(t, func(s *domain.ActivitySpec) {
		s.BillingStartsOn = day(2026, 10, 31)
		s.FirstDueOn = day(2026, 10, 31)
	})
	got := a.PeriodsUpTo(day(2027, 1, 31))
	want := []domain.ActivityPeriod{
		{Key: "2026-10", BillsOn: day(2026, 10, 31), DueOn: day(2026, 10, 31)},
		{Key: "2026-11", BillsOn: day(2026, 11, 30), DueOn: day(2026, 11, 30)},
		{Key: "2026-12", BillsOn: day(2026, 12, 31), DueOn: day(2026, 12, 31)},
		{Key: "2027-01", BillsOn: day(2027, 1, 31), DueOn: day(2027, 1, 31)},
	}
	assertPeriods(t, got, want)
}

func TestMonthlyPeriodsOnlyThoseWhoseBillingDateArrived(t *testing.T) {
	a := mustActivity(t, nil)
	got := a.PeriodsUpTo(day(2026, 10, 31))
	want := []domain.ActivityPeriod{
		{Key: "2026-09", BillsOn: day(2026, 9, 1), DueOn: day(2026, 9, 10)},
		{Key: "2026-10", BillsOn: day(2026, 10, 1), DueOn: day(2026, 10, 10)},
	}
	assertPeriods(t, got, want)
	if len(a.PeriodsUpTo(day(2026, 8, 31))) != 0 {
		t.Fatalf("nothing is billable before the first billing date")
	}
}

func TestWeeklyPeriodsAreKeyedByISOWeek(t *testing.T) {
	a := mustActivity(t, func(s *domain.ActivitySpec) {
		s.Periodicity = domain.PeriodicityWeekly
		s.BillingStartsOn = day(2026, 9, 28) // Monday, ISO week 40
		s.FirstDueOn = day(2026, 9, 30)
	})
	got := a.PeriodsUpTo(day(2026, 10, 12))
	want := []domain.ActivityPeriod{
		{Key: "2026-W40", BillsOn: day(2026, 9, 28), DueOn: day(2026, 9, 30)},
		{Key: "2026-W41", BillsOn: day(2026, 10, 5), DueOn: day(2026, 10, 7)},
		{Key: "2026-W42", BillsOn: day(2026, 10, 12), DueOn: day(2026, 10, 14)},
	}
	assertPeriods(t, got, want)
}

func TestPerCycleIsASingleCharge(t *testing.T) {
	a := mustActivity(t, func(s *domain.ActivitySpec) { s.Periodicity = domain.PeriodicityPerCycle })
	got := a.PeriodsUpTo(day(2027, 6, 30))
	want := []domain.ActivityPeriod{{Key: "2026-09-01", BillsOn: day(2026, 9, 1), DueOn: day(2026, 9, 10)}}
	assertPeriods(t, got, want)
}

func assertPeriods(t *testing.T, got, want []domain.ActivityPeriod) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("periods = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].Key != want[i].Key || !got[i].BillsOn.Equal(want[i].BillsOn) || !got[i].DueOn.Equal(want[i].DueOn) {
			t.Fatalf("period %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestActivityChargeRef(t *testing.T) {
	ref := domain.ActivityChargeRef("act-1", "stu-1", "2026-10")
	if ref != "activity:act-1:stu-1:2026-10" {
		t.Fatalf("ref = %q", ref)
	}
	if !domain.IsActivityRef(ref) || domain.IsActivityRef("tuition:c:s:2026-10") {
		t.Fatalf("IsActivityRef wrong")
	}
	if id, ok := domain.ActivityOfRef(ref); !ok || id != "act-1" {
		t.Fatalf("ActivityOfRef = %q %v", id, ok)
	}
	if _, ok := domain.ActivityOfRef("activity:"); ok {
		t.Fatalf("an empty activity id is not a ref")
	}
}

func mustEnrollment(t *testing.T, startsOn time.Time) domain.ActivityEnrollment {
	t.Helper()
	e, err := domain.NewActivityEnrollment("enr-1", "act-1", "stu-1", "opt-1", startsOn)
	if err != nil {
		t.Fatalf("new enrollment: %v", err)
	}
	return e
}

func TestNewActivityEnrollmentRejectsMissingParts(t *testing.T) {
	cases := [][5]string{
		{"", "a", "s", "o", "x"}, {"e", "", "s", "o", "x"}, {"e", "a", "", "o", "x"}, {"e", "a", "s", "", "x"}, {"e", "a", "s", "o", ""},
	}
	for _, c := range cases {
		starts := day(2026, 9, 1)
		if c[4] == "" {
			starts = time.Time{}
		}
		if _, err := domain.NewActivityEnrollment(c[0], c[1], c[2], c[3], starts); glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Errorf("%v: err = %v, want KindInvalid", c, err)
		}
	}
}

// The approved rule (plan T17): the period in course is charged when the pupil
// joined on or before its due date; otherwise billing starts with the next one.
// No proration.
func TestEnrollmentBillsThePeriodInCourseOnlyUpToItsDueDate(t *testing.T) {
	sept := domain.ActivityPeriod{Key: "2026-09", BillsOn: day(2026, 9, 1), DueOn: day(2026, 9, 10)}
	oct := domain.ActivityPeriod{Key: "2026-10", BillsOn: day(2026, 10, 1), DueOn: day(2026, 10, 10)}

	onDue := mustEnrollment(t, day(2026, 9, 10))
	if !onDue.Bills(sept) || !onDue.Bills(oct) {
		t.Fatalf("joined on the due date: the period in course is charged")
	}
	late := mustEnrollment(t, day(2026, 9, 11))
	if late.Bills(sept) || !late.Bills(oct) {
		t.Fatalf("joined after the due date: billing starts next period")
	}
}

func TestEndedEnrollmentStopsPeriodsBilledOnOrAfterTheEnd(t *testing.T) {
	sept := domain.ActivityPeriod{Key: "2026-09", BillsOn: day(2026, 9, 1), DueOn: day(2026, 9, 10)}
	oct := domain.ActivityPeriod{Key: "2026-10", BillsOn: day(2026, 10, 1), DueOn: day(2026, 10, 10)}

	e := mustEnrollment(t, day(2026, 8, 20)).End(day(2026, 10, 1))
	if !e.Bills(sept) || e.Bills(oct) {
		t.Fatalf("ended on Oct 1: September charged, October not")
	}
	if e.ActiveOn(day(2026, 10, 1)) || !e.ActiveOn(day(2026, 9, 30)) {
		t.Fatalf("ActiveOn wrong around the end")
	}
	// Ending before it started bills nothing at all.
	never := mustEnrollment(t, day(2026, 9, 5)).End(day(2026, 9, 2))
	if never.Bills(sept) || never.Bills(oct) {
		t.Fatalf("an enrolment ended before it started bills nothing")
	}
}

func TestEnrollmentWithOption(t *testing.T) {
	e := mustEnrollment(t, day(2026, 9, 1))
	moved, err := e.WithOption("opt-2")
	if err != nil || moved.OptionID() != "opt-2" || e.OptionID() != "opt-1" {
		t.Fatalf("WithOption = %+v %v", moved, err)
	}
	if _, err := e.WithOption(" "); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("blank option err = %v", err)
	}
}

func TestActivityIsASystemConceptKind(t *testing.T) {
	c, err := domain.NewChargeConcept("c1", "t1", "Actividad", 0, domain.RecurrenceOnce, true)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.WithKind(domain.KindActivity)
	if err != nil || got.Kind() != "activity" {
		t.Fatalf("WithKind(activity) = %v, %v", got.Kind(), err)
	}
}
