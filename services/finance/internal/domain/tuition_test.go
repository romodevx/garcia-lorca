package domain_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

func validPlan(t *testing.T) domain.TuitionPlan {
	t.Helper()
	p, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: "tp-1", TenantID: "t-1", CycleID: "c-1", LevelID: "lv-1",
		EnrollmentCents: 500000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, LateFeeCents: 15000,
	})
	if err != nil {
		t.Fatalf("NewTuitionPlan: %v", err)
	}
	return p
}

func TestNewTuitionPlanRefusesWhatASchoolCannotMean(t *testing.T) {
	base := domain.TuitionPlanSpec{
		ID: "tp-1", TenantID: "t-1", CycleID: "c-1", LevelID: "lv-1",
		EnrollmentCents: 500000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10,
	}
	cases := []struct {
		name  string
		mutha func(*domain.TuitionPlanSpec)
	}{
		{"no cycle", func(s *domain.TuitionPlanSpec) { s.CycleID = "" }},
		{"no level", func(s *domain.TuitionPlanSpec) { s.LevelID = "" }},
		{"negative monthly", func(s *domain.TuitionPlanSpec) { s.MonthlyCents = -1 }},
		{"negative enrollment", func(s *domain.TuitionPlanSpec) { s.EnrollmentCents = -1 }},
		{"negative late fee", func(s *domain.TuitionPlanSpec) { s.LateFeeCents = -1 }},
		{"zero months", func(s *domain.TuitionPlanSpec) { s.Months = 0 }},
		{"thirteen months", func(s *domain.TuitionPlanSpec) { s.Months = 13 }},
		{"month zero", func(s *domain.TuitionPlanSpec) { s.FirstBilledMonth = 0 }},
		{"month thirteen", func(s *domain.TuitionPlanSpec) { s.FirstBilledMonth = 13 }},
		{"due day zero", func(s *domain.TuitionPlanSpec) { s.DueDay = 0 }},
		// The one worth naming: a limit on the 29th, 30th or 31st does not exist
		// in February. Accepting it would mean silently moving somebody's
		// deadline, which is exactly the kind of quiet lie a school gets shouted
		// at for.
		{"due day 29", func(s *domain.TuitionPlanSpec) { s.DueDay = 29 }},
		{"due day 31", func(s *domain.TuitionPlanSpec) { s.DueDay = 31 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := base
			tc.mutha(&spec)
			if _, err := domain.NewTuitionPlan(spec); err == nil || glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("want KindInvalid, got %v", err)
			}
		})
	}
}

// BilledMonths is what turns "10 mensualidades desde septiembre" into the ten
// actual months. Getting this wrong bills a family in July or misses June.
func TestBilledMonthsWalksForwardAcrossTheNewYear(t *testing.T) {
	p := validPlan(t)
	cycleStart := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	got := p.BilledMonths(cycleStart)

	if len(got) != 10 {
		t.Fatalf("got %d months, want 10", len(got))
	}
	// September 2026 through June 2027, on the 10th.
	first, last := got[0], got[9]
	if first.Year() != 2026 || first.Month() != time.September || first.Day() != 10 {
		t.Errorf("first due date = %s; want 2026-09-10", first.Format("2006-01-02"))
	}
	if last.Year() != 2027 || last.Month() != time.June || last.Day() != 10 {
		t.Errorf("last due date = %s; want 2027-06-10", last.Format("2006-01-02"))
	}
}

// A cycle that starts AFTER the plan's first billed month must not bill months
// that fall before the school year began.
func TestBilledMonthsStartsInTheCycleNotThePreviousYear(t *testing.T) {
	p, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: "tp-2", TenantID: "t-1", CycleID: "c-1", LevelID: "lv-1",
		MonthlyCents: 100, Months: 3, FirstBilledMonth: 1, DueDay: 5,
	})
	if err != nil {
		t.Fatalf("NewTuitionPlan: %v", err)
	}
	// The cycle opens in August 2026; January is therefore January 2027.
	got := p.BilledMonths(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	if len(got) != 3 || got[0].Year() != 2027 || got[0].Month() != time.January {
		t.Fatalf("first due date = %s; want 2027-01-05", got[0].Format("2006-01-02"))
	}
}

// The surcharge rule: one fixed fee per COMPLETED month past the due date. Days
// must not matter — running the job on the 3rd or the 27th has to give the same
// answer, or a school's debt would depend on when somebody pressed a button.
func TestOverdueMonthsCountsCompletedMonthsNotDays(t *testing.T) {
	due := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		asOf string
		want int
	}{
		{"2026-09-09", 0}, // not due yet
		{"2026-09-10", 0}, // due today, not late
		{"2026-10-09", 0}, // a day short of a month
		{"2026-10-10", 1}, // exactly one month
		{"2026-10-27", 1}, // same month, later day — same answer
		{"2026-12-10", 3},
		{"2027-01-09", 3},
		{"2027-01-10", 4},
	}
	for _, tc := range cases {
		t.Run(tc.asOf, func(t *testing.T) {
			asOf, _ := time.Parse("2006-01-02", tc.asOf)
			if got := domain.OverdueMonths(due, asOf); got != tc.want {
				t.Errorf("OverdueMonths(%s) = %d, want %d", tc.asOf, got, tc.want)
			}
		})
	}
}
