package usecase_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// T14 (owner decision 2026-10-17): students now dates an imported pupil's
// enrollment on the cycle's first day. Finance needs no change for it — these
// pin what that date means for the money, the late fees included, so the
// owner decides on numbers and not on a guess.

// importedFromStartFixture is the billing school (cycle from 1 August, tuition
// September..June due on the 10th, $150 fixed surcharge) with its one pupil
// imported and enrolled from the cycle's first day.
func importedFromStartFixture(t *testing.T, enrolledAt time.Time) *fixture {
	t.Helper()
	f := billingFixture(t)
	f.enrollments.byCycle[testCycle] = []ports.EnrollmentRef{{
		ID: "enr-imp", StudentID: testStudent, GroupID: testGroup, CycleID: testCycle,
		Status: "active", EnrolledAt: enrolledAt, Origin: ports.EnrollmentOriginImport,
	}}
	return f
}

func lateFeeDescriptions(t *testing.T, f *fixture) []string {
	t.Helper()
	return chargeDescriptions(t, f, domain.KindLateFee)
}

// Enrolled from the start, an imported pupil owes the whole year from
// September and still no inscription.
func TestRunBillingChargesAnImportedPupilFromTheCycleStartSeptember(t *testing.T) {
	f := importedFromStartFixture(t, aug(2026, time.August, 1))

	got := runOn(t, f, "2026-10-17")

	if got.TuitionCreated != 10 || got.EnrollmentCreated != 0 {
		t.Fatalf("tuition = %d, inscriptions = %d; want September..June (10) and none", got.TuitionCreated, got.EnrollmentCreated)
	}
	if !issuedInMonth(issuedTo(t, f, testStudent), "septiembre") {
		t.Error("September was not billed to a pupil enrolled from the cycle start")
	}
}

// The money side effect, made visible (not decided): issued on 17 October,
// September (due the 10th) has one COMPLETED overdue month, so the same run
// adds ONE fixed surcharge for it. October is overdue but not a completed
// month, so it adds none yet.
func TestRunBillingSurchargesTheImportedPupilsSeptemberOnTheSameRun(t *testing.T) {
	f := importedFromStartFixture(t, aug(2026, time.August, 1))

	got := runOn(t, f, "2026-10-17")

	fees := lateFeeDescriptions(t, f)
	if got.LateFeesCreated != 1 || len(fees) != 1 || !strings.Contains(fees[0], "septiembre") {
		t.Fatalf("late fees = %d %v; want exactly one, on September", got.LateFeesCreated, fees)
	}
	if amount := issuedTo(t, f, testStudent)[fees[0]]; amount != 15000 {
		t.Errorf("surcharge = %d cents; want the plan's fixed $150", amount)
	}
}

// A fee split September/October is owed whole from the start too, and its
// September share is late the same way as the tuition month.
func TestRunBillingSurchargesTheImportedPupilsSeptemberFeeShare(t *testing.T) {
	f := importedFromStartFixture(t, aug(2026, time.August, 1))
	if _, err := f.svc.UpsertTuitionPlan(context.Background(), usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel, EnrollmentCents: 50000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, LateFeeCents: 15000, Active: true,
		Fees: []domain.Fee{{Name: "Cuota de Julio", AmountCents: 160000, Months: []int{9, 10}}},
	}); err != nil {
		t.Fatalf("seed plan with a fee: %v", err)
	}

	got := runOn(t, f, "2026-10-17")

	if shares := chargeDescriptions(t, f, domain.KindFee); len(shares) != 2 {
		t.Fatalf("fee shares = %v; want both (September and October)", shares)
	}
	if got.LateFeesCreated != 2 {
		t.Fatalf("late fees = %d %v; want 2 — September's tuition and September's fee share", got.LateFeesCreated, lateFeeDescriptions(t, f))
	}
}

// The repaired pupil: billed on 7 October from an import dated the 6th, then
// moved to the cycle start by the re-upload. The next run adds only September
// and its one surcharge; October, already issued, is not issued again.
func TestRunBillingAfterTheMoveAddsOnlySeptemberAndItsSurcharge(t *testing.T) {
	f := importedFromStartFixture(t, aug(2026, time.October, 6))
	if first := runOn(t, f, "2026-10-07"); first.TuitionCreated != 9 || first.LateFeesCreated != 0 {
		t.Fatalf("first run = %+v; want October..June and no surcharge", first)
	}
	f.enrollments.byCycle[testCycle][0].EnrolledAt = aug(2026, time.August, 1)

	got := runOn(t, f, "2026-10-17")

	if got.TuitionCreated != 1 || got.LateFeesCreated != 1 {
		t.Fatalf("after the move = %+v; want September only and its one surcharge", got)
	}
	if again := runOn(t, f, "2026-10-17"); again.TuitionCreated != 0 || again.LateFeesCreated != 0 {
		t.Fatalf("second run = %+v; want nothing new", again)
	}
}

func issuedInMonth(issued map[string]int64, month string) bool {
	for name := range issued {
		if strings.Contains(name, month) {
			return true
		}
	}
	return false
}
