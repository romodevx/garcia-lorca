package domain_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

func planWithFees(t *testing.T, fees ...domain.Fee) domain.TuitionPlan {
	t.Helper()
	p, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: "tp-1", TenantID: "t-1", CycleID: "c-1", LevelID: "lv-1",
		EnrollmentCents: 500000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10,
		Fees: fees,
	})
	if err != nil {
		t.Fatalf("NewTuitionPlan: %v", err)
	}
	return p
}

// The school's real arrangement: a fee for July, a month the year does not
// bill, collected across two months that it does.
func TestAFeeIsSplitAcrossTheMonthsTheSchoolChose(t *testing.T) {
	plan := planWithFees(t, domain.Fee{Name: "Cuota de Julio", AmountCents: 160000, Months: []int{9, 10}})
	cycleStart := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)

	got := plan.FeeInstallments(cycleStart)

	if len(got) != 2 {
		t.Fatalf("want 2 instalments, got %d", len(got))
	}
	for i, want := range []struct {
		amount int64
		month  time.Month
		year   int
		part   int
	}{
		{80000, time.September, 2026, 1},
		{80000, time.October, 2026, 2},
	} {
		if got[i].AmountCents != want.amount {
			t.Errorf("part %d: amount = %d, want %d", i+1, got[i].AmountCents, want.amount)
		}
		if got[i].DueOn.Month() != want.month || got[i].DueOn.Year() != want.year {
			t.Errorf("part %d: due %s, want %s %d", i+1, got[i].DueOn.Format("2006-01"), want.month, want.year)
		}
		if got[i].Part != want.part || got[i].Of != 2 {
			t.Errorf("part %d: labelled %d de %d", i+1, got[i].Part, got[i].Of)
		}
		if got[i].Name != "Cuota de Julio" {
			t.Errorf("part %d: name = %q", i+1, got[i].Name)
		}
	}
}

// The rounding trap. $1,000.00 over three months is not three times $333.33:
// that collects a cent less than the school asked for, every time, on every
// pupil. The remainder goes to the first payment.
func TestSplittingAFeeLosesNoCent(t *testing.T) {
	plan := planWithFees(t, domain.Fee{Name: "Graduación", AmountCents: 100000, Months: []int{9, 10, 11}})
	cycleStart := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)

	got := plan.FeeInstallments(cycleStart)

	var total int64
	for _, i := range got {
		total += i.AmountCents
	}
	if total != 100000 {
		t.Fatalf("the three parts collect %d, the school asked for 100000", total)
	}
	if got[0].AmountCents != 33334 {
		t.Errorf("the odd cent belongs to the first payment, got %d", got[0].AmountCents)
	}
}

// A fee collected in one month is not "1 de 1" — it is just the fee.
func TestAFeeInOneMonthIsNotLabelledAsAPart(t *testing.T) {
	plan := planWithFees(t, domain.Fee{Name: "Seguro escolar", AmountCents: 45000, Months: []int{9}})
	got := plan.FeeInstallments(time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC))

	if len(got) != 1 {
		t.Fatalf("want 1 instalment, got %d", len(got))
	}
	if got[0].Of != 1 || got[0].AmountCents != 45000 {
		t.Errorf("got %d de %d for %d", got[0].Part, got[0].Of, got[0].AmountCents)
	}
}

// A cycle that opens in August and collects in February is collecting in the
// NEXT calendar year. Nobody writing "septiembre y febrero" thinks of that as
// two years, so the plan works it out.
func TestFeeMonthsWalkIntoTheNextYear(t *testing.T) {
	plan := planWithFees(t, domain.Fee{Name: "Cuota de Julio", AmountCents: 100000, Months: []int{9, 2}})
	got := plan.FeeInstallments(time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC))

	if got[0].DueOn.Year() != 2026 || got[1].DueOn.Year() != 2027 {
		t.Fatalf("got %s and %s", got[0].DueOn.Format("2006-01"), got[1].DueOn.Format("2006-01"))
	}
}

// The fee falls due on the same day of the month the tuition does, because a
// family pays once and should not have two deadlines in one month.
func TestAFeeFallsDueOnTheTuitionDay(t *testing.T) {
	plan := planWithFees(t, domain.Fee{Name: "Cuota de Julio", AmountCents: 100000, Months: []int{9}})
	got := plan.FeeInstallments(time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC))

	if got[0].DueOn.Day() != 10 {
		t.Errorf("due on the %d, the plan's due day is the 10th", got[0].DueOn.Day())
	}
}

func TestNewTuitionPlanRefusesAFeeASchoolCannotMean(t *testing.T) {
	base := domain.TuitionPlanSpec{
		ID: "tp-1", TenantID: "t-1", CycleID: "c-1", LevelID: "lv-1",
		EnrollmentCents: 500000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10,
	}
	cases := []struct {
		name string
		fee  domain.Fee
	}{
		{"no name", domain.Fee{Name: "  ", AmountCents: 1000, Months: []int{9}}},
		{"no money", domain.Fee{Name: "Cuota", AmountCents: 0, Months: []int{9}}},
		{"negative", domain.Fee{Name: "Cuota", AmountCents: -1, Months: []int{9}}},
		{"no months", domain.Fee{Name: "Cuota", AmountCents: 1000, Months: nil}},
		{"month zero", domain.Fee{Name: "Cuota", AmountCents: 1000, Months: []int{0}}},
		{"month thirteen", domain.Fee{Name: "Cuota", AmountCents: 1000, Months: []int{13}}},
		// Twice the same month is not "collect it twice": it is a slip, and
		// silently accepting it would halve the payment the school expects.
		{"the same month twice", domain.Fee{Name: "Cuota", AmountCents: 1000, Months: []int{9, 9}}},
		// Split so thin that a part rounds to nothing. A charge for zero pesos
		// sits in a family's statement forever, already settled, explaining
		// nothing.
		{"less than a cent each", domain.Fee{Name: "Cuota", AmountCents: 2, Months: []int{9, 10, 11}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := base
			spec.Fees = []domain.Fee{tc.fee}
			if _, err := domain.NewTuitionPlan(spec); err == nil || glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Fatalf("want invalid, got %v", err)
			}
		})
	}
}

// The inscription is its own charge with its own deadline. A school collects it
// in June for a year that starts in August, and deriving it from the cycle's
// first month made that impossible to say.
func TestTheInscriptionCarriesItsOwnDueDate(t *testing.T) {
	due := time.Date(2026, time.June, 30, 0, 0, 0, 0, time.UTC)
	p, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: "tp-1", TenantID: "t-1", CycleID: "c-1", LevelID: "lv-1",
		EnrollmentCents: 500000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10,
		EnrollmentDueOn: due,
	})
	if err != nil {
		t.Fatalf("NewTuitionPlan: %v", err)
	}
	if !p.EnrollmentDueOn().Equal(due) {
		t.Fatalf("got %s, want %s", p.EnrollmentDueOn(), due)
	}
}

// Unset is the ordinary state, and it must stay distinguishable from a date so
// the run can fall back to the rule it always used.
func TestAnUnsetInscriptionDateIsZero(t *testing.T) {
	if !validPlan(t).EnrollmentDueOn().IsZero() {
		t.Fatal("an unset inscription deadline must read as unset")
	}
}

// A pupil brought over from the previous system in October already settled
// August and September there. The fees of the months before they joined are not
// theirs here; the month they join is, but not late: its deadline moves to the
// day they joined, inside the same month so the charge's reference holds.
func TestFeeInstallmentsFromSkipsTheMonthsBeforeTheJoinDate(t *testing.T) {
	plan := planWithFees(t, domain.Fee{Name: "Materiales", AmountCents: 300000, Months: []int{8, 9, 10, 11}})
	cycleStart := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	joined := time.Date(2026, time.October, 20, 0, 0, 0, 0, time.UTC)

	got := plan.FeeInstallmentsFrom(cycleStart, joined)

	if len(got) != 2 {
		t.Fatalf("want the October and November parts, got %d", len(got))
	}
	if got[0].DueOn != joined {
		t.Errorf("October part due %s, want the join day %s", got[0].DueOn.Format(time.DateOnly), joined.Format(time.DateOnly))
	}
	if got[1].DueOn.Month() != time.November || got[1].DueOn.Day() != 10 {
		t.Errorf("November part due %s, want 2026-11-10", got[1].DueOn.Format(time.DateOnly))
	}
	if got[0].Part != 3 || got[0].Of != 4 {
		t.Errorf("October part labelled %d de %d, want 3 de 4", got[0].Part, got[0].Of)
	}
}

// Nobody recorded a join date, or they joined when the year opened: the whole
// list, exactly as FeeInstallments gives it.
func TestFeeInstallmentsFromWithoutALaterJoinIsTheWholeList(t *testing.T) {
	plan := planWithFees(t, domain.Fee{Name: "Materiales", AmountCents: 300000, Months: []int{8, 9, 10}})
	cycleStart := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)

	for name, joined := range map[string]time.Time{"zero": {}, "at start": cycleStart} {
		if got := plan.FeeInstallmentsFrom(cycleStart, joined); len(got) != 3 {
			t.Errorf("%s: got %d parts, want 3", name, len(got))
		}
	}
}
