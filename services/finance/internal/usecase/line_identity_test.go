package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// T1 (review 2026-09-30): "se creó una inscripción con 3 conceptos y uno de
// ellos aparece dos veces".
//
// The identity of an issued line was its external ref, and the ref was built
// from the plan's SHAPE (one amount → `enrollment:<cycle>:<student>`) and from
// the NAME's lossy slug (concepts → `...:<slug>`, where "Inscripción" slugs to
// "inscripcin" and "INSCRIPCION" to "inscripcion"). "Already issued" was an
// exact ref match, so converting a plan or re-spelling a concept minted a new
// identity for the same line, and the old unpaid charge stayed next to it.

// conceptsPlan re-prices the fixture's level with the three lines the Kinder
// plan in dev carries, spelled as the director typed them (upper case, no
// accent).
func conceptsPlan(t *testing.T, f *fixture, inscription string) {
	t.Helper()
	flat := func(name string, cents int64) domain.EnrollmentConcept {
		return domain.EnrollmentConcept{Name: name, Tiers: []domain.PriceTier{
			{EffectiveFrom: aug(2026, time.August, 1), AmountCents: cents},
		}}
	}
	if _, err := f.svc.UpsertTuitionPlan(context.Background(), usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel,
		MonthlyCents: 320000, Months: 10, FirstBilledMonth: 9, DueDay: 10,
		LateFeeCents: 15000, Active: true,
		EnrollmentConcepts: []domain.EnrollmentConcept{
			flat("GASTOS ADMINISTRATIVOS Y SEGURO", 470000),
			flat(inscription, 240000),
			flat("APF", 85000),
		},
	}); err != nil {
		t.Fatalf("convert the plan to concepts: %v", err)
	}
}

// inscriptionCharges lists the pupil's inscription lines by what they read as.
func inscriptionCharges(t *testing.T, f *fixture) map[string]int {
	t.Helper()
	list, err := f.svc.ListCharges(context.Background(), testStudent, testCycle)
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	enrolment, ok := f.repo.conceptOfKind(domain.KindEnrollment)
	if !ok {
		t.Fatal("no inscription concept was created")
	}
	out := map[string]int{}
	for _, c := range list {
		if c.Charge.ConceptID() == enrolment {
			out[domain.LineKey(c.Charge.Description())]++
		}
	}
	return out
}

// H1 — the dev case. The Kinder plan was written on 09-07 as ONE amount (before
// M18) and turned into three concepts on 09-28. A pupil billed in between owes
// the single-amount "Inscripción"; the next run issued "INSCRIPCION" again.
func TestConvertingASingleAmountPlanDoesNotBillTheInscriptionTwice(t *testing.T) {
	f := billingFixture(t)
	runOn(t, f, "2026-09-15")
	conceptsPlan(t, f, "INSCRIPCION")

	got := runOn(t, f, "2026-09-16")

	if got.EnrollmentCreated != 2 {
		t.Errorf("created %d inscription lines, want the 2 the pupil did not owe yet", got.EnrollmentCreated)
	}
	lines := inscriptionCharges(t, f)
	if lines["inscripcion"] != 1 {
		t.Fatalf("the pupil owes the inscription %d times, want once: %v", lines["inscripcion"], lines)
	}
	if lines["gastosadministrativosyseguro"] != 1 || lines["apf"] != 1 {
		t.Errorf("the lines the pupil did not owe yet were not issued: %v", lines)
	}
}

// The single amount was the price of the WHOLE old inscription, not a rung of
// the new ladder. The run must not re-price it as if it were one: that is a
// decision about money nobody made.
func TestTheSingleAmountInscriptionIsNotRepricedByTheNewLadder(t *testing.T) {
	f := billingFixture(t)
	runOn(t, f, "2026-09-15")
	conceptsPlan(t, f, "INSCRIPCION")

	got := runOn(t, f, "2026-09-16")

	if got.EnrollmentRepriced != 0 {
		t.Errorf("repriced %d charges, want 0", got.EnrollmentRepriced)
	}
	if amount := issuedTo(t, f, testStudent)["Inscripción"]; amount != 50000 {
		t.Errorf("the single-amount inscription is now %d, want the 50000 it was issued at", amount)
	}
}

// H2 — correcting the spelling of a concept ("Inscripción" → "INSCRIPCION")
// changed its slug, so the next run issued the same line a second time.
func TestRespellingAConceptDoesNotIssueItAgain(t *testing.T) {
	f := ladderFixture(t)
	runOn(t, f, "2026-09-15")
	respell(t, f, "INSCRIPCION")

	got := runOn(t, f, "2026-09-16")

	if got.EnrollmentCreated != 0 {
		t.Errorf("created %d inscription lines after a spelling fix, want 0", got.EnrollmentCreated)
	}
	if lines := inscriptionCharges(t, f); lines["inscripcion"] != 1 {
		t.Fatalf("the pupil owes the inscription %d times, want once", lines["inscripcion"])
	}
}

// The re-spelled line is still the same line of the same ladder, so it keeps
// climbing: the run re-prices the charge it already issued.
func TestARespelledConceptKeepsClimbingItsLadder(t *testing.T) {
	f := ladderFixture(t)
	runOn(t, f, "2026-09-15")
	respell(t, f, "INSCRIPCION")

	got := runOn(t, f, "2026-11-15")

	if got.EnrollmentRepriced != 1 {
		t.Fatalf("repriced = %d, want 1", got.EnrollmentRepriced)
	}
	if amount := issuedTo(t, f, testStudent)["Inscripción"]; amount != 330000 {
		t.Errorf("Inscripción = %d, want November's 330000", amount)
	}
}

// The counter quotes the ladder by the same identity the run bills by, or it
// would show one number and collect another.
func TestTheQuoteFollowsTheLadderOfARespelledConcept(t *testing.T) {
	f := ladderFixture(t)
	runOn(t, f, "2026-09-15")
	respell(t, f, "INSCRIPCION")
	day := aug(2026, time.November, 5)
	f.clock = day

	got, err := f.svc.ExpectedToday(context.Background(), testStudent, day)
	if err != nil {
		t.Fatalf("ExpectedToday: %v", err)
	}

	for _, line := range got.Lines {
		if line.Description == "Inscripción" && line.OwedCents != 330000 {
			t.Errorf("the counter quotes %d for the inscription, want the ladder's 330000", line.OwedCents)
		}
	}
}

// The enrolment event bills through the same door as the run.
func TestTheEnrolmentEventDoesNotBillAConvertedInscriptionTwice(t *testing.T) {
	f := billingFixture(t)
	ctx := context.Background()
	if _, err := f.svc.EnsureEnrollmentCharge(ctx, testCycle, testStudent); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	conceptsPlan(t, f, "Inscripción")

	if _, err := f.svc.EnsureEnrollmentCharge(ctx, testCycle, testStudent); err != nil {
		t.Fatalf("redelivery: %v", err)
	}

	if lines := inscriptionCharges(t, f); lines["inscripcion"] != 1 {
		t.Fatalf("the pupil owes the inscription %d times, want once", lines["inscripcion"])
	}
}

// Fees carry the same kind of reference (`fee:<cycle>:<student>:<slug>:<month>`)
// and had the same defect.
func TestRespellingAFeeDoesNotIssueItAgain(t *testing.T) {
	f := billingFixture(t)
	feePlan(t, f, "Graduacion")
	first := runOn(t, f, "2026-09-15")
	if first.FeesCreated != 2 {
		t.Fatalf("fees created = %d, want the 2 shares", first.FeesCreated)
	}
	feePlan(t, f, "Graduación")

	got := runOn(t, f, "2026-09-16")

	if got.FeesCreated != 0 {
		t.Errorf("created %d fee shares after a spelling fix, want 0", got.FeesCreated)
	}
}

// Two lines that are one identity cannot both be billed, so the table refuses
// them when it is saved instead of dropping one in silence at night.
func TestAPlanRefusesTwoLinesThatDifferOnlyInAccents(t *testing.T) {
	flat := func(name string) domain.EnrollmentConcept {
		return domain.EnrollmentConcept{Name: name, Tiers: []domain.PriceTier{
			{EffectiveFrom: aug(2026, time.August, 1), AmountCents: 1000},
		}}
	}
	fee := func(name string) domain.Fee {
		return domain.Fee{Name: name, AmountCents: 1000, Months: []int{9}}
	}
	for name, in := range map[string]usecase.TuitionPlanInput{
		"concepts": {EnrollmentConcepts: []domain.EnrollmentConcept{flat("INSCRIPCION"), flat("Inscripción")}},
		"fees":     {EnrollmentCents: 1000, Fees: []domain.Fee{fee("Graduacion"), fee("GRADUACIÓN")}},
	} {
		t.Run(name, func(t *testing.T) {
			f := billingFixture(t)
			in.CycleID, in.LevelID = testCycle, testLevel
			in.MonthlyCents, in.Months, in.FirstBilledMonth, in.DueDay, in.Active = 320000, 10, 9, 10, true

			if _, err := f.svc.UpsertTuitionPlan(context.Background(), in); err == nil {
				t.Error("saved a plan with two lines that bill as one")
			}
		})
	}
}

// respell re-saves the ladder fixture with its first line spelled differently.
func respell(t *testing.T, f *fixture, name string) {
	t.Helper()
	flat := func(name string, cents int64) domain.EnrollmentConcept {
		return domain.EnrollmentConcept{Name: name, Tiers: []domain.PriceTier{
			{EffectiveFrom: aug(2026, time.August, 1), AmountCents: cents},
		}}
	}
	if _, err := f.svc.UpsertTuitionPlan(context.Background(), usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel,
		MonthlyCents: 320000, Months: 10, FirstBilledMonth: 9, DueDay: 10,
		LateFeeCents: 15000, Active: true,
		EnrollmentConcepts: []domain.EnrollmentConcept{
			{Name: name, Tiers: []domain.PriceTier{
				{EffectiveFrom: aug(2026, time.August, 1), AmountCents: 220000},
				{EffectiveFrom: aug(2026, time.November, 1), AmountCents: 330000},
			}},
			flat("Gastos adm y seguro", 470000),
			flat("Cuota Asociación Padres de Familia", 85000),
		},
	}); err != nil {
		t.Fatalf("respell the plan: %v", err)
	}
}

// feePlan prices the fixture's level with one fee split over September and
// October.
func feePlan(t *testing.T, f *fixture, name string) {
	t.Helper()
	if _, err := f.svc.UpsertTuitionPlan(context.Background(), usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel,
		EnrollmentCents: 50000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, LateFeeCents: 15000, Active: true,
		Fees: []domain.Fee{{Name: name, AmountCents: 160000, Months: []int{9, 10}}},
	}); err != nil {
		t.Fatalf("price the fee: %v", err)
	}
}
