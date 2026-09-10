package domain_test

import (
	"testing"
	"time"

	"github.com/garcia-lorca/services/finance/internal/domain"
)

// preescolar2027 is the school's printed price list for Preescolar, exactly as
// the flyer states it. It is the acceptance case for the whole feature: if the
// three totals below come out, the system charges what the school advertises.
func preescolar2027(t *testing.T) domain.TuitionPlan {
	t.Helper()
	flat := func(name string, cents int64) domain.EnrollmentConcept {
		return domain.EnrollmentConcept{
			Name:  name,
			Tiers: []domain.PriceTier{{EffectiveFrom: day(2027, time.January, 1), AmountCents: cents}},
		}
	}
	plan, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: "tp-preescolar", TenantID: "t-1", CycleID: "c-2027", LevelID: "lv-preescolar",
		MonthlyCents: 320000, Months: 11, FirstBilledMonth: 9, DueDay: 10,
		EnrollmentConcepts: []domain.EnrollmentConcept{
			inscriptionLadder(),
			flat("Gastos adm y seguro", 470000),
			flat("Cuota Asociación Padres de Familia", 85000),
		},
	})
	if err != nil {
		t.Fatalf("NewTuitionPlan: %v", err)
	}
	return plan
}

// TOTAL A PAGAR, straight off the flyer: $7,750 in January and February,
// $8,850 in March, $9,950 from April to September.
func TestJoiningCostsWhatTheFlyerSaysOnTheDay(t *testing.T) {
	plan := preescolar2027(t)

	for _, tc := range []struct {
		name string
		on   time.Time
		want int64
	}{
		{"Ene/Feb", day(2027, time.January, 15), 775000},
		{"Ene/Feb, last day", day(2027, time.February, 28), 775000},
		{"Marzo", day(2027, time.March, 3), 885000},
		{"Abril a Sept", day(2027, time.April, 20), 995000},
		{"Abril a Sept, still", day(2027, time.September, 15), 995000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := plan.EnrollmentLinesOn(tc.on)
			if len(lines) != 3 {
				t.Fatalf("want the flyer's 3 lines, got %d", len(lines))
			}
			var total int64
			for _, l := range lines {
				if l.Name == "" {
					t.Errorf("a line the family reads has no name")
				}
				total += l.AmountCents
			}
			if total != tc.want {
				t.Errorf("total on %s = %d, want %d", tc.on.Format("2006-01-02"), total, tc.want)
			}
		})
	}
}

// The lines come back in the order the school wrote them. A statement whose
// rows reshuffle between two reads is one a parent cannot check against the
// flyer in their hand.
func TestTheLinesKeepTheOrderTheSchoolWroteThem(t *testing.T) {
	lines := preescolar2027(t).EnrollmentLinesOn(day(2027, time.March, 3))

	want := []string{"Inscripción", "Gastos adm y seguro", "Cuota Asociación Padres de Familia"}
	for i, name := range want {
		if lines[i].Name != name {
			t.Errorf("line %d = %q, want %q", i, lines[i].Name, name)
		}
	}
	if lines[0].AmountCents != 330000 {
		t.Errorf("March inscription = %d, want 330000", lines[0].AmountCents)
	}
}

// Every plan written before ADR 0011 goes on meaning what it meant: ONE
// inscription, at enrollment_cents, on any day.
//
// Unnamed, because the school may have renamed that concept in its own catalog
// and the domain does not hold the catalog. The caller supplies the description
// exactly as it always has.
func TestAPlanWithoutConceptsStillChargesItsSingleInscription(t *testing.T) {
	plan, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: "tp-1", TenantID: "t-1", CycleID: "c-1", LevelID: "lv-1",
		EnrollmentCents: 500000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10,
	})
	if err != nil {
		t.Fatalf("NewTuitionPlan: %v", err)
	}

	for _, on := range []time.Time{day(2026, time.August, 1), day(2027, time.April, 1)} {
		lines := plan.EnrollmentLinesOn(on)
		if len(lines) != 1 {
			t.Fatalf("want 1 line, got %d", len(lines))
		}
		if lines[0].AmountCents != 500000 {
			t.Errorf("amount = %d, want the plan's 500000", lines[0].AmountCents)
		}
		if lines[0].Name != "" {
			t.Errorf("name = %q, want empty so the caller names it", lines[0].Name)
		}
	}
}

// --- joining part-way through a year already running ------------------------

func yearFromSeptember(t *testing.T) (domain.TuitionPlan, time.Time) {
	t.Helper()
	plan, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: "tp-1", TenantID: "t-1", CycleID: "c-1", LevelID: "lv-1",
		EnrollmentCents: 500000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10,
	})
	if err != nil {
		t.Fatalf("NewTuitionPlan: %v", err)
	}
	return plan, day(2026, time.August, 1)
}

// A pupil who arrives in March owes March onwards, and nothing before it. The
// run bills the whole year up front, which for somebody who was not there in
// September means billing them for months the school never taught them.
func TestAPupilWhoArrivesMidYearOwesFromTheirOwnMonth(t *testing.T) {
	plan, cycleStart := yearFromSeptember(t)

	got := plan.InstallmentsFrom(cycleStart, day(2027, time.March, 20))

	if len(got) != 4 {
		t.Fatalf("want March, April, May and June — 4 payments, got %d", len(got))
	}
	for i, want := range []time.Month{time.March, time.April, time.May, time.June} {
		if got[i].DueOn.Month() != want {
			t.Errorf("payment %d falls in %s, want %s", i+1, got[i].DueOn.Month(), want)
		}
	}
}

// The month they arrive in is theirs, but they are not late for it. March fell
// due on the 10th and they enrolled on the 20th; charging them the surcharge of
// ten days they were not yet a pupil is money the school cannot defend.
func TestTheMonthAPupilArrivesInIsNotAlreadyOverdue(t *testing.T) {
	plan, cycleStart := yearFromSeptember(t)
	joined := day(2027, time.March, 20)

	got := plan.InstallmentsFrom(cycleStart, joined)

	if !got[0].DueOn.Equal(joined) {
		t.Errorf("first payment due %s, want the day they joined (%s)",
			got[0].DueOn.Format("2006-01-02"), joined.Format("2006-01-02"))
	}
	// Only that first one moves. April keeps the plan's own due day.
	if got[1].DueOn.Day() != 10 {
		t.Errorf("April due on the %d, want the plan's day 10", got[1].DueOn.Day())
	}
}

// Arriving BEFORE the due day of their own month leaves the deadline alone: a
// pupil enrolling on 3 March still pays March on the 10th, like everybody else.
func TestArrivingBeforeTheDeadlineKeepsTheDeadline(t *testing.T) {
	plan, cycleStart := yearFromSeptember(t)

	got := plan.InstallmentsFrom(cycleStart, day(2027, time.March, 3))

	if got[0].DueOn.Day() != 10 {
		t.Errorf("March due on the %d, want the plan's day 10", got[0].DueOn.Day())
	}
}

// Everybody who was already there. A join date at or before the cycle opens —
// and a missing one, which is every pupil enrolled before the field existed —
// bills the whole year, exactly as it does today.
func TestAPupilWhoWasAlreadyThereOwesTheWholeYear(t *testing.T) {
	plan, cycleStart := yearFromSeptember(t)
	whole := plan.Installments(cycleStart)

	for _, tc := range []struct {
		name   string
		joined time.Time
	}{
		{"joined the day the cycle opened", cycleStart},
		{"joined before the cycle opened", day(2026, time.July, 15)},
		{"no join date recorded at all", time.Time{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := plan.InstallmentsFrom(cycleStart, tc.joined)
			if len(got) != len(whole) {
				t.Fatalf("got %d payments, want the whole year's %d", len(got), len(whole))
			}
			for i := range got {
				if !got[i].DueOn.Equal(whole[i].DueOn) || got[i].AmountCents != whole[i].AmountCents {
					t.Errorf("payment %d = %s/%d, want %s/%d", i+1,
						got[i].DueOn.Format("2006-01-02"), got[i].AmountCents,
						whole[i].DueOn.Format("2006-01-02"), whole[i].AmountCents)
				}
			}
		})
	}
}

// A pupil arriving after the last payment of the year owes no tuition at all —
// only the inscription. Returning the whole year instead would bill eleven
// months to somebody with weeks left in the school.
func TestArrivingAfterTheLastPaymentOwesNoTuition(t *testing.T) {
	plan, cycleStart := yearFromSeptember(t)

	if got := plan.InstallmentsFrom(cycleStart, day(2027, time.August, 1)); len(got) != 0 {
		t.Errorf("got %d payments, want none", len(got))
	}
}

// A split year keeps its own amounts. The filter chooses WHICH payments are
// owed; it never re-divides them.
func TestJoiningMidYearKeepsTheSplitAmounts(t *testing.T) {
	plan, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: "tp-1", TenantID: "t-1", CycleID: "c-1", LevelID: "lv-1",
		EnrollmentCents: 500000, MonthlyCents: 320000,
		Months: 3, FirstBilledMonth: 11, DueDay: 10,
		Schedule: []domain.Installment{
			{Month: 11, AmountCents: 320000},
			{Month: 12, AmountCents: 160000},
			{Month: 3, AmountCents: 160000},
		},
	})
	if err != nil {
		t.Fatalf("NewTuitionPlan: %v", err)
	}
	cycleStart := day(2026, time.August, 1)

	got := plan.InstallmentsFrom(cycleStart, day(2026, time.December, 1))

	if len(got) != 2 {
		t.Fatalf("want December and March, got %d", len(got))
	}
	if got[0].AmountCents != 160000 || got[1].AmountCents != 160000 {
		t.Errorf("amounts = %d and %d, want the schedule's 160000 each",
			got[0].AmountCents, got[1].AmountCents)
	}
	if got[1].DueOn.Year() != 2027 || got[1].DueOn.Month() != time.March {
		t.Errorf("last payment %s, want March 2027", got[1].DueOn.Format("2006-01"))
	}
}

// EVERY payment of the join month moves, not just the first one kept.
//
// Schedule validation allows two payments in one month — "diciembre a la mitad"
// split within December is a real arrangement — and a pupil joining on the 20th
// found the second one still dated the 10th, overdue the moment they enrolled.
// That is precisely the surcharge-for-days-they-were-not-a-pupil this rule
// exists to prevent.
func TestEveryPaymentOfTheJoinMonthMovesToTheDayTheyJoined(t *testing.T) {
	plan, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: "tp-1", TenantID: "t-1", CycleID: "c-1", LevelID: "lv-1",
		MonthlyCents: 320000, Months: 3, FirstBilledMonth: 3, DueDay: 10,
		Schedule: []domain.Installment{
			{Month: 3, AmountCents: 160000},
			{Month: 3, AmountCents: 160000},
			{Month: 4, AmountCents: 320000},
		},
	})
	if err != nil {
		t.Fatalf("NewTuitionPlan: %v", err)
	}
	cycleStart := day(2026, time.August, 1)
	joined := day(2027, time.March, 20)

	got := plan.InstallmentsFrom(cycleStart, joined)

	if len(got) != 3 {
		t.Fatalf("want 3 payments, got %d", len(got))
	}
	for i := 0; i < 2; i++ {
		if !got[i].DueOn.Equal(joined) {
			t.Errorf("March payment %d due %s, want the join day %s",
				i+1, got[i].DueOn.Format("2006-01-02"), joined.Format("2006-01-02"))
		}
	}
	if got[2].DueOn.Day() != 10 {
		t.Errorf("April due on the %d, want the plan's day 10", got[2].DueOn.Day())
	}
}
