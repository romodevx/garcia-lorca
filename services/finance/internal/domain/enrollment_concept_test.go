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

// A concept whose ladder is empty must not take the process down. validate()
// refuses one, so this is unreachable through a constructed plan — but the type
// is exported with exported fields, and a handler pricing a preview straight off
// a request payload holds one before any plan exists.
func TestPricingAMalformedConceptDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("AmountOn panicked on a concept with no steps: %v", r)
		}
	}()
	empty := domain.EnrollmentConcept{Name: "Inscripción"}
	if got := empty.AmountOn(day(2027, time.March, 1)); got != 0 {
		t.Errorf("AmountOn = %d, want 0 for a concept that prices nothing", got)
	}
}

// The uniqueness guard has to normalize exactly as hard as the identity it
// protects. "Cuota APF" and "Cuota A.P.F." are two names to a human and ONE
// external_ref to the ledger, so CreateChargesBulk would drop one line on
// conflict and report it as skipped: the family is under-billed for the year and
// nothing says so.
func TestTwoNamesThatCollapseToOneReferenceAreRefused(t *testing.T) {
	tier := []domain.PriceTier{{EffectiveFrom: day(2027, time.January, 1), AmountCents: 85000}}
	for _, pair := range [][2]string{
		{"Cuota APF", "Cuota A.P.F."},
		{"Cuota APF", "cuota  apf"},
		{"Gastos adm y seguro", "GASTOS-ADM-Y-SEGURO"},
	} {
		t.Run(pair[0]+" vs "+pair[1], func(t *testing.T) {
			_, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
				ID: "tp-1", TenantID: "t-1", CycleID: "c-1", LevelID: "lv-1",
				MonthlyCents: 320000, Months: 10, FirstBilledMonth: 9, DueDay: 10,
				EnrollmentConcepts: []domain.EnrollmentConcept{
					{Name: pair[0], Tiers: tier},
					{Name: pair[1], Tiers: tier},
				},
			})
			if err == nil || glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Fatalf("err = %v, want KindInvalid", err)
			}
		})
	}
}

// Names that only LOOK alike are still two concepts. The guard must not be so
// eager that a school cannot print two lines it means to print.
func TestNamesThatMerelyResembleEachOtherAreBothAllowed(t *testing.T) {
	tier := []domain.PriceTier{{EffectiveFrom: day(2027, time.January, 1), AmountCents: 85000}}
	_, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: "tp-1", TenantID: "t-1", CycleID: "c-1", LevelID: "lv-1",
		MonthlyCents: 320000, Months: 10, FirstBilledMonth: 9, DueDay: 10,
		EnrollmentConcepts: []domain.EnrollmentConcept{
			{Name: "Cuota APF 1", Tiers: tier},
			{Name: "Cuota APF 2", Tiers: tier},
		},
	})
	if err != nil {
		t.Fatalf("two genuinely different names were refused: %v", err)
	}
}

// EnrollmentCents and EnrollmentConcepts are two answers to one question, and
// during the rollout four callers still read the first one
// (usecase/billing.go, usecase/enrollment_charge.go, the grpc handler and the
// gateway client). A plan carrying both would show $5,000 in the editor while
// the statement totals $7,750, with nothing saying they disagree. Made
// unrepresentable rather than documented.
func TestAPlanCannotCarryBothASingleInscriptionAndConcepts(t *testing.T) {
	_, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: "tp-1", TenantID: "t-1", CycleID: "c-1", LevelID: "lv-1",
		EnrollmentCents: 500000,
		MonthlyCents:    320000, Months: 10, FirstBilledMonth: 9, DueDay: 10,
		EnrollmentConcepts: []domain.EnrollmentConcept{inscriptionLadder()},
	})
	if err == nil || glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("err = %v, want KindInvalid", err)
	}
}

// R2. The ladder only ever climbs.
//
// Everything else in the design leans on that: ADR 0011 ("la deuda sube sola"),
// the freeze rule — which is only defensible as a one-way ratchet, because a
// family can be surprised upward but never billed below what they were already
// quoted — and the re-price itself. A transposed digit (33,000 for 3,300) would
// otherwise walk four hundred issued inscriptions DOWN with nothing recording
// that it happened.
func TestAPriceLadderCannotGoDown(t *testing.T) {
	_, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: "tp-1", TenantID: "t-1", CycleID: "c-1", LevelID: "lv-1",
		MonthlyCents: 320000, Months: 10, FirstBilledMonth: 9, DueDay: 10,
		EnrollmentConcepts: []domain.EnrollmentConcept{{
			Name: "Inscripción",
			Tiers: []domain.PriceTier{
				{EffectiveFrom: day(2027, time.January, 1), AmountCents: 330000},
				{EffectiveFrom: day(2027, time.March, 1), AmountCents: 33000},
			},
		}},
	})
	if err == nil || glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("err = %v, want KindInvalid", err)
	}
}

// A step that repeats the same price is not a mistake — a school may write the
// months out in full and leave two of them alike.
func TestAPriceLadderMayStandStill(t *testing.T) {
	_, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: "tp-1", TenantID: "t-1", CycleID: "c-1", LevelID: "lv-1",
		MonthlyCents: 320000, Months: 10, FirstBilledMonth: 9, DueDay: 10,
		EnrollmentConcepts: []domain.EnrollmentConcept{{
			Name: "Gastos adm y seguro",
			Tiers: []domain.PriceTier{
				{EffectiveFrom: day(2027, time.January, 1), AmountCents: 470000},
				{EffectiveFrom: day(2027, time.March, 1), AmountCents: 470000},
			},
		}},
	})
	if err != nil {
		t.Fatalf("a flat second step was refused: %v", err)
	}
}

// R8. AmountOn must not depend on an ordering it does not itself check.
//
// Its own doc says it is reached by "a handler holding one straight off a
// request payload" — before validate() has run. Stopping at the first future
// step would return March's price for a January date on a ladder written
// backwards: wrong money, no error, no panic.
func TestPricingDoesNotTrustAnOrderItDidNotCheck(t *testing.T) {
	backwards := domain.EnrollmentConcept{
		Name: "Inscripción",
		Tiers: []domain.PriceTier{
			{EffectiveFrom: day(2027, time.March, 1), AmountCents: 330000},
			{EffectiveFrom: day(2027, time.January, 1), AmountCents: 220000},
		},
	}
	if got := backwards.AmountOn(day(2027, time.January, 15)); got != 220000 {
		t.Errorf("AmountOn(15 Jan) = %d, want January's 220000", got)
	}
	if got := backwards.AmountOn(day(2027, time.March, 15)); got != 330000 {
		t.Errorf("AmountOn(15 Mar) = %d, want March's 330000", got)
	}
}

// R4. A plan is a value: what it costs must not change under it.
//
// AmountOn depends absolutely on tier order, and that order is checked ONCE, in
// the constructor. The grpc handler builds the concept slice and hands it
// straight over, so without a copy it still owns the plan's prices — and any
// later sort, dedupe or reslice there would move the ladder behind every guard
// that already ran.
func TestAPlansPricesCannotBeChangedFromOutside(t *testing.T) {
	tiers := []domain.PriceTier{
		{EffectiveFrom: day(2027, time.January, 1), AmountCents: 220000},
		{EffectiveFrom: day(2027, time.March, 1), AmountCents: 330000},
	}
	concepts := []domain.EnrollmentConcept{{Name: "Inscripción", Tiers: tiers}}
	plan, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: "tp-1", TenantID: "t-1", CycleID: "c-1", LevelID: "lv-1",
		MonthlyCents: 320000, Months: 10, FirstBilledMonth: 9, DueDay: 10,
		EnrollmentConcepts: concepts,
	})
	if err != nil {
		t.Fatalf("NewTuitionPlan: %v", err)
	}

	// The caller reuses what it handed over, and a reader writes into the getter.
	concepts[0].Name = "Otra cosa"
	tiers[0].AmountCents = 1
	plan.EnrollmentConcepts()[0].Tiers[1].AmountCents = 2

	lines := plan.EnrollmentLinesOn(day(2027, time.January, 15))
	if lines[0].Name != "Inscripción" || lines[0].AmountCents != 220000 {
		t.Errorf("January = %q/%d, want Inscripción/220000", lines[0].Name, lines[0].AmountCents)
	}
	if got := plan.EnrollmentLinesOn(day(2027, time.March, 15))[0].AmountCents; got != 330000 {
		t.Errorf("March = %d, want 330000", got)
	}
}
