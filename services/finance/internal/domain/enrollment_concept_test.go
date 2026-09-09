package domain_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

// inscriptionLadder is the school's own price list, in cents: $2,200 in January
// and February, $3,300 in March, $4,400 from April on.
func inscriptionLadder() domain.EnrollmentConcept {
	return domain.EnrollmentConcept{
		Name: "Inscripción",
		Tiers: []domain.PriceTier{
			{EffectiveFrom: day(2027, time.January, 1), AmountCents: 220000},
			{EffectiveFrom: day(2027, time.March, 1), AmountCents: 330000},
			{EffectiveFrom: day(2027, time.April, 1), AmountCents: 440000},
		},
	}
}

// The rule, in the school's own numbers. The tier that applies is the LAST one
// whose date has arrived, so the first of the month is already the new price —
// a family paying on 1 March pays March's price, not February's.
func TestTheTierThatAppliesIsTheLastOneWhoseDateArrived(t *testing.T) {
	ladder := inscriptionLadder()

	for _, tc := range []struct {
		name string
		on   time.Time
		want int64
	}{
		{"mid January", day(2027, time.January, 15), 220000},
		{"last day of February", day(2027, time.February, 28), 220000},
		{"the day March opens", day(2027, time.March, 1), 330000},
		{"last day of March", day(2027, time.March, 31), 330000},
		{"the day April opens", day(2027, time.April, 1), 440000},
		{"still April's price in September", day(2027, time.September, 30), 440000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ladder.AmountOn(tc.on); got != tc.want {
				t.Errorf("AmountOn(%s) = %d, want %d", tc.on.Format("2006-01-02"), got, tc.want)
			}
		})
	}
}

// Asked about before the ladder starts, a concept costs its FIRST step. Zero
// would price the year free for anybody who enquired in December.
func TestBeforeTheLadderStartsAConceptCostsItsFirstStep(t *testing.T) {
	if got := inscriptionLadder().AmountOn(day(2026, time.December, 15)); got != 220000 {
		t.Errorf("AmountOn(before the first tier) = %d, want the first step 220000", got)
	}
}

// Most concepts do not move. One step is a flat price, and saying so must not
// require a ladder of identical rows.
func TestOneStepIsAFlatPrice(t *testing.T) {
	flat := domain.EnrollmentConcept{
		Name:  "Gastos adm y seguro",
		Tiers: []domain.PriceTier{{EffectiveFrom: day(2027, time.January, 1), AmountCents: 470000}},
	}
	for _, on := range []time.Time{
		day(2026, time.November, 1), day(2027, time.January, 1), day(2027, time.September, 30),
	} {
		if got := flat.AmountOn(on); got != 470000 {
			t.Errorf("AmountOn(%s) = %d, want 470000 every day", on.Format("2006-01-02"), got)
		}
	}
}

// What a school cannot mean by a concept. Each of these would produce a
// statement nobody could explain, so each is refused at the boundary.
func TestAConceptTheSchoolCannotMean(t *testing.T) {
	for _, tc := range []struct {
		name    string
		concept domain.EnrollmentConcept
	}{
		{
			"no name at all",
			domain.EnrollmentConcept{Tiers: []domain.PriceTier{{EffectiveFrom: day(2027, time.January, 1), AmountCents: 100}}},
		},
		{
			"a name of only spaces",
			domain.EnrollmentConcept{Name: "   ", Tiers: []domain.PriceTier{{EffectiveFrom: day(2027, time.January, 1), AmountCents: 100}}},
		},
		{
			"no steps",
			domain.EnrollmentConcept{Name: "Inscripción"},
		},
		{
			"a step for no money",
			domain.EnrollmentConcept{Name: "Inscripción", Tiers: []domain.PriceTier{{EffectiveFrom: day(2027, time.January, 1)}}},
		},
		{
			"a step for negative money",
			domain.EnrollmentConcept{Name: "Inscripción", Tiers: []domain.PriceTier{{EffectiveFrom: day(2027, time.January, 1), AmountCents: -1}}},
		},
		{
			"a step with no date",
			domain.EnrollmentConcept{Name: "Inscripción", Tiers: []domain.PriceTier{{AmountCents: 220000}}},
		},
		{
			"steps written backwards",
			domain.EnrollmentConcept{Name: "Inscripción", Tiers: []domain.PriceTier{
				{EffectiveFrom: day(2027, time.March, 1), AmountCents: 330000},
				{EffectiveFrom: day(2027, time.January, 1), AmountCents: 220000},
			}},
		},
		{
			"two steps on the same day",
			domain.EnrollmentConcept{Name: "Inscripción", Tiers: []domain.PriceTier{
				{EffectiveFrom: day(2027, time.January, 1), AmountCents: 220000},
				{EffectiveFrom: day(2027, time.January, 1), AmountCents: 330000},
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
				ID: "tp-1", TenantID: "t-1", CycleID: "c-1", LevelID: "lv-1",
				MonthlyCents: 320000, Months: 10, FirstBilledMonth: 9, DueDay: 10,
				EnrollmentConcepts: []domain.EnrollmentConcept{tc.concept},
			})
			if err == nil || glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Fatalf("err = %v, want KindInvalid", err)
			}
		})
	}
}

// Two lines with one name cannot be told apart on a receipt, and the charge's
// own reference is derived from that name (ADR 0011) — so one of the two would
// silently never be issued.
func TestTwoConceptsCannotShareAName(t *testing.T) {
	same := domain.EnrollmentConcept{
		Name:  "Inscripción",
		Tiers: []domain.PriceTier{{EffectiveFrom: day(2027, time.January, 1), AmountCents: 220000}},
	}
	_, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: "tp-1", TenantID: "t-1", CycleID: "c-1", LevelID: "lv-1",
		MonthlyCents: 320000, Months: 10, FirstBilledMonth: 9, DueDay: 10,
		EnrollmentConcepts: []domain.EnrollmentConcept{same, same},
	})
	if err == nil || glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("err = %v, want KindInvalid", err)
	}
}

// A bound, not a business rule: what stops a malformed payload turning one
// billing run into four hundred pupils times an unbounded list.
func TestThereIsALimitToHowManyConceptsAndStepsAPlanCarries(t *testing.T) {
	tooMany := make([]domain.EnrollmentConcept, 0, 13)
	for i := 0; i < 13; i++ {
		tooMany = append(tooMany, domain.EnrollmentConcept{
			Name:  string(rune('a' + i)),
			Tiers: []domain.PriceTier{{EffectiveFrom: day(2027, time.January, 1), AmountCents: 100}},
		})
	}
	_, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: "tp-1", TenantID: "t-1", CycleID: "c-1", LevelID: "lv-1",
		MonthlyCents: 320000, Months: 10, FirstBilledMonth: 9, DueDay: 10,
		EnrollmentConcepts: tooMany,
	})
	if err == nil || glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("13 concepts: err = %v, want KindInvalid", err)
	}

	steps := make([]domain.PriceTier, 0, 13)
	for i := 0; i < 13; i++ {
		steps = append(steps, domain.PriceTier{EffectiveFrom: day(2027, time.January, 1+i), AmountCents: 100})
	}
	_, err = domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: "tp-1", TenantID: "t-1", CycleID: "c-1", LevelID: "lv-1",
		MonthlyCents: 320000, Months: 10, FirstBilledMonth: 9, DueDay: 10,
		EnrollmentConcepts: []domain.EnrollmentConcept{{Name: "Inscripción", Tiers: steps}},
	})
	if err == nil || glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("13 steps: err = %v, want KindInvalid", err)
	}
}
