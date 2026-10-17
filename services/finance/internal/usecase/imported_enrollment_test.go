package usecase_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// importedFixture is the billing school with its one pupil brought over from the
// previous system and dated 6 October: already inscribed there, so the
// inscription of this cycle is not theirs to pay here (owner decision O5, ADR
// 0033). Since T14 (2026-10-17) students dates an import on the cycle start
// (imported_cycle_start_test.go); an enrollment still dated mid-year, as the
// imports before it were until a re-upload moves them, is billed from its month.
func importedFixture(t *testing.T) *fixture {
	t.Helper()
	f := billingFixture(t)
	f.enrollments.byCycle[testCycle] = []ports.EnrollmentRef{{
		ID: "enr-imp", StudentID: testStudent, GroupID: testGroup, CycleID: testCycle,
		Status: "active", EnrolledAt: aug(2026, time.October, 6), Origin: ports.EnrollmentOriginImport,
	}}
	return f
}

// The daily run bills the inscription of "anybody enrolled without one". An
// imported pupil is enrolled without one on purpose.
func TestRunBillingDoesNotChargeAnImportedPupilTheInscription(t *testing.T) {
	f := importedFixture(t)

	got := runOn(t, f, "2026-10-07")

	if got.EnrollmentCreated != 0 {
		t.Errorf("enrollment charges = %d, want 0 for an imported pupil", got.EnrollmentCreated)
	}
	if _, ok := issuedTo(t, f, testStudent)["Inscripción"]; ok {
		t.Error("the imported pupil was charged the inscription")
	}
}

// Tuition runs from the month they arrived — September stays with the old
// system — and October, theirs, is not late on the day it is issued.
func TestRunBillingChargesAnImportedPupilTuitionFromTheImportMonth(t *testing.T) {
	f := importedFixture(t)

	got := runOn(t, f, "2026-10-07")

	if got.TuitionCreated != 9 {
		t.Errorf("tuition = %d, want October..June — 9 months", got.TuitionCreated)
	}
	if got.LateFeesCreated != 0 {
		t.Errorf("late fees = %d, want 0 on the day after the import", got.LateFeesCreated)
	}
	for name := range issuedTo(t, f, testStudent) {
		if strings.Contains(name, "septiembre") {
			t.Errorf("%q was billed to a pupil imported in October", name)
		}
	}
}

// The extras follow the same rule (owner decision O8): the September share was
// collected by the previous system.
func TestRunBillingChargesAnImportedPupilFeesFromTheImportMonth(t *testing.T) {
	f := importedFixture(t)
	seedPlanWithFees(t, f, domain.Fee{Name: "Cuota de Julio", AmountCents: 160000, Months: []int{9, 10}})

	runOn(t, f, "2026-10-07")

	got := chargeDescriptions(t, f, domain.KindFee)
	if len(got) != 1 || got[0] != "Cuota de Julio (2 de 2)" {
		t.Errorf("fees = %v, want only the October share", got)
	}
}

// A pupil enrolled by hand mid-year keeps today's rule for the extras: the
// import does not quietly change how everybody else is billed.
func TestRunBillingStillChargesAManualMidYearJoinerEveryFee(t *testing.T) {
	f := billingFixture(t)
	f.enrollments.byCycle[testCycle] = []ports.EnrollmentRef{{
		ID: "enr-late", StudentID: testStudent, GroupID: testGroup, CycleID: testCycle,
		Status: "active", EnrolledAt: aug(2026, time.October, 6),
	}}
	seedPlanWithFees(t, f, domain.Fee{Name: "Cuota de Julio", AmountCents: 160000, Months: []int{9, 10}})

	runOn(t, f, "2026-10-07")

	if got := chargeDescriptions(t, f, domain.KindFee); len(got) != 2 {
		t.Errorf("fees = %v, want both shares for a manual enrolment", got)
	}
}

// The event path must agree with the run: a StudentEnrolled for an imported
// enrolment (should one ever be published) charges nothing.
func TestEnsureEnrollmentChargeSkipsAnImportedPupil(t *testing.T) {
	f := importedFixture(t)

	created, err := f.svc.EnsureEnrollmentCharge(context.Background(), testCycle, testStudent)
	if err != nil {
		t.Fatalf("EnsureEnrollmentCharge: %v", err)
	}
	if created {
		t.Error("an imported pupil was charged the inscription by the event path")
	}
}
