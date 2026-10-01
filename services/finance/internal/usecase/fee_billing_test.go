package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// chargeDescriptions lists what one pupil's statement actually says.
func chargeDescriptions(t *testing.T, f *fixture, kind string) []string {
	t.Helper()
	charges, err := f.svc.ListCharges(context.Background(), testStudent, testCycle)
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	want := f.repo.conceptIDOfKind(kind)
	var out []string
	for _, c := range charges {
		if c.Charge.ConceptID() == want {
			out = append(out, c.Charge.Description())
		}
	}
	return out
}

// "Colegiatura 2026-09" is a machine's idea of a month. The school reads these
// rows out to parents on the phone, and the parent is not holding a calendar.
func TestTheMonthlyChargeNamesItsMonthInWords(t *testing.T) {
	f := billingFixture(t)

	runOn(t, f, "2026-09-15")

	got := chargeDescriptions(t, f, domain.KindTuition)
	if len(got) == 0 {
		t.Fatal("no tuition charges")
	}
	want := "Colegiatura septiembre 2026"
	var found bool
	for _, d := range got {
		if d == want {
			found = true
		}
		if d == "Colegiatura 2026-09" {
			t.Errorf("still naming the month as %q", d)
		}
	}
	if !found {
		t.Errorf("want a charge called %q, got %v", want, got)
	}
}

// A year that opens in August reaches June of the NEXT calendar year, and the
// name has to say which year it is or two Junes look like a duplicate.
func TestTheMonthlyChargeNamesTheYearItFallsIn(t *testing.T) {
	f := billingFixture(t)

	runOn(t, f, "2026-09-15")

	var found bool
	for _, d := range chargeDescriptions(t, f, domain.KindTuition) {
		if d == "Colegiatura junio 2027" {
			found = true
		}
	}
	if !found {
		t.Errorf("want the tenth month named as junio 2027, got %v",
			chargeDescriptions(t, f, domain.KindTuition))
	}
}

// Renaming a month must NOT re-issue it. The description is for a person; the
// external_ref is the identity, and it stays the machine-readable month.
func TestRenamingTheMonthDoesNotDoubleBill(t *testing.T) {
	f := billingFixture(t)

	first := runOn(t, f, "2026-09-15")
	second := runOn(t, f, "2026-09-15")

	if second.TuitionCreated != 0 {
		t.Errorf("the second run created %d tuition charges, want 0", second.TuitionCreated)
	}
	if first.TuitionCreated == 0 {
		t.Fatal("the first run created nothing")
	}
}

// The school's real arrangement: a fee for July — a month the year does not
// bill — collected half in September and half in October.
func TestAFeeIsBilledAcrossTheMonthsItWasSplitInto(t *testing.T) {
	f := billingFixture(t)
	seedPlanWithFees(t, f, domain.Fee{Name: "Cuota de Julio", AmountCents: 160000, Months: []int{9, 10}})

	runOn(t, f, "2026-09-15")

	got := chargeDescriptions(t, f, domain.KindFee)
	if len(got) != 2 {
		t.Fatalf("want 2 fee charges, got %d: %v", len(got), got)
	}
	for i, want := range []string{"Cuota de Julio (1 de 2)", "Cuota de Julio (2 de 2)"} {
		if got[i] != want {
			t.Errorf("charge %d says %q, want %q", i+1, got[i], want)
		}
	}
}

// The whole point of the split: what the family owes in September is the month
// plus its share of the fee, and the system does the adding.
func TestTheSplitFeeAddsToTheMonthItLandsIn(t *testing.T) {
	f := billingFixture(t)
	seedPlanWithFees(t, f, domain.Fee{Name: "Cuota de Julio", AmountCents: 160000, Months: []int{9, 10}})

	runOn(t, f, "2026-09-15")

	charges, err := f.svc.ListCharges(context.Background(), testStudent, testCycle)
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	september := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	october := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	var total int64
	enrollment := f.repo.conceptIDOfKind(domain.KindEnrollment)
	for _, c := range charges {
		due := c.Charge.DueDate()
		if c.Charge.ConceptID() == enrollment || due.Before(september) || !due.Before(october) {
			continue
		}
		total += c.Charge.Effective().Int64()
	}
	// $3,200 of tuition and $800 of the July fee.
	if total != 400000 {
		t.Errorf("September owes %d, want 400000 — the month plus half the fee", total)
	}
}

// A fee falls due the same day the tuition does, so a family has one deadline
// in the month rather than two.
func TestAFeeFallsDueWithTheMonthsTuition(t *testing.T) {
	f := billingFixture(t)
	seedPlanWithFees(t, f, domain.Fee{Name: "Cuota de Julio", AmountCents: 160000, Months: []int{9, 10}})

	runOn(t, f, "2026-09-15")

	charges, err := f.svc.ListCharges(context.Background(), testStudent, testCycle)
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	fee := f.repo.conceptIDOfKind(domain.KindFee)
	tuition := f.repo.conceptIDOfKind(domain.KindTuition)
	// The month's tuition deadline, whatever the calendar made of it.
	dueOfMonth := make(map[string]time.Time)
	for _, c := range charges {
		if c.Charge.ConceptID() == tuition {
			dueOfMonth[c.Charge.DueDate().Format("2006-01")] = c.Charge.DueDate()
		}
	}
	var checked int
	for _, c := range charges {
		if c.Charge.ConceptID() != fee {
			continue
		}
		want, ok := dueOfMonth[c.Charge.DueDate().Format("2006-01")]
		if !ok {
			t.Errorf("%q falls in a month with no tuition", c.Charge.Description())
			continue
		}
		checked++
		if !c.Charge.DueDate().Equal(want) {
			t.Errorf("%q falls due %s, the month's tuition is due %s",
				c.Charge.Description(), c.Charge.DueDate().Format("2006-01-02"), want.Format("2006-01-02"))
		}
	}
	if checked != 2 {
		t.Errorf("checked %d fee charges, want 2", checked)
	}
}

// Running twice must not collect the fee twice.
func TestFeesAreIdempotent(t *testing.T) {
	f := billingFixture(t)
	seedPlanWithFees(t, f, domain.Fee{Name: "Cuota de Julio", AmountCents: 160000, Months: []int{9, 10}})

	runOn(t, f, "2026-09-15")
	runOn(t, f, "2026-09-15")

	if got := chargeDescriptions(t, f, domain.KindFee); len(got) != 2 {
		t.Fatalf("want 2 fee charges after two runs, got %d: %v", len(got), got)
	}
}

// A level with no fees bills none. The ordinary case must stay ordinary.
func TestALevelWithoutFeesBillsNone(t *testing.T) {
	f := billingFixture(t)

	runOn(t, f, "2026-09-15")

	if got := chargeDescriptions(t, f, domain.KindFee); len(got) != 0 {
		t.Errorf("want no fee charges, got %v", got)
	}
}

// The school sets the inscription's own deadline: collected in June for a year
// that opens in August.
func TestTheInscriptionUsesTheDeadlineTheSchoolSet(t *testing.T) {
	f := billingFixture(t)
	if _, err := f.svc.UpsertTuitionPlan(context.Background(), usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel,
		EnrollmentCents: 50000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, Active: true,
		EnrollmentDueOn: time.Date(2026, time.June, 30, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	runOn(t, f, "2026-09-15")

	charges, err := f.svc.ListCharges(context.Background(), testStudent, testCycle)
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	enrollment := f.repo.conceptIDOfKind(domain.KindEnrollment)
	for _, c := range charges {
		if c.Charge.ConceptID() != enrollment {
			continue
		}
		if got := c.Charge.DueDate().Format("2006-01-02"); got != "2026-06-30" {
			t.Errorf("the inscription falls due %s, the school said 2026-06-30", got)
		}
		return
	}
	t.Fatal("no inscription charge")
}

// Unset keeps the rule the run has always used, so a plan written before the
// field existed goes on meaning what it meant.
func TestAnUnsetInscriptionDeadlineKeepsTheOldRule(t *testing.T) {
	f := billingFixture(t)

	runOn(t, f, "2026-09-15")

	charges, err := f.svc.ListCharges(context.Background(), testStudent, testCycle)
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	enrollment := f.repo.conceptIDOfKind(domain.KindEnrollment)
	for _, c := range charges {
		if c.Charge.ConceptID() != enrollment {
			continue
		}
		// The plan's due day in the cycle's first month: 10 August 2026.
		if got := c.Charge.DueDate().Format("2006-01-02"); got != "2026-08-10" {
			t.Errorf("the inscription falls due %s, want the old rule's 2026-08-10", got)
		}
		return
	}
	t.Fatal("no inscription charge")
}

func seedPlanWithFees(t *testing.T, f *fixture, fees ...domain.Fee) {
	t.Helper()
	if _, err := f.svc.UpsertTuitionPlan(context.Background(), usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel,
		EnrollmentCents: 50000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, Active: true,
		Fees: fees,
	}); err != nil {
		t.Fatalf("seed plan with fees: %v", err)
	}
}

// Setting the inscription's deadline has to reach the pupils who were already
// billed. The run never re-issues a charge it has written — that is what makes
// it idempotent — and without this the setting did nothing for anybody already
// in the system, which is everybody by the time a school notices it wants a
// different date.
func TestChangingTheInscriptionDeadlineMovesChargesAlreadyIssued(t *testing.T) {
	f := billingFixture(t)
	runOn(t, f, "2026-09-15")

	if _, err := f.svc.UpsertTuitionPlan(context.Background(), usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel,
		EnrollmentCents: 50000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, Active: true,
		EnrollmentDueOn: time.Date(2026, time.June, 30, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("re-price: %v", err)
	}
	// Saving the plan moves them at once (T5); the run after it finds nothing
	// left to move. The run's own move is TestTheRunMovesAChangedStartAlone.
	charges, err := f.svc.ListCharges(context.Background(), testStudent, testCycle)
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	enrollment := f.repo.conceptIDOfKind(domain.KindEnrollment)
	for _, c := range charges {
		if c.Charge.ConceptID() != enrollment {
			continue
		}
		if got := c.Charge.DueDate().Format("2006-01-02"); got != "2026-06-30" {
			t.Errorf("the inscription still falls due %s", got)
		}
		return
	}
	t.Fatal("no inscription charge")
}

// Nothing to move is not something to report. A run that says it moved zero
// every month teaches the director to stop reading the summary.
func TestARunThatMovesNothingSaysSo(t *testing.T) {
	f := billingFixture(t)
	runOn(t, f, "2026-09-15")

	got := runOn(t, f, "2026-09-15")

	if got.EnrollmentMoved != 0 {
		t.Errorf("the second identical run moved %d inscriptions, want 0", got.EnrollmentMoved)
	}
}
