package usecase_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// T14 (owner decisions 2026-10-17): an imported pupil is enrolled from the
// cycle's first day and billed from September like everybody else, but the
// months that fell due BEFORE the day of the import are owed at the normal
// price with no surcharge — no fixed monthly fee, no daily accrual, fee shares
// included. From the import day on, the normal rules apply.

// importDay is the instant students reports for the import: 10:00 in Morelia
// on 17 October.
var importDay = time.Date(2026, time.October, 17, 16, 0, 0, 0, time.UTC)

// importedFromStartFixture is the billing school (cycle from 1 August, tuition
// September..June due on the 10th, $150 fixed surcharge) with its one pupil
// imported on importedAt and enrolled from enrolledAt.
func importedFromStartFixture(t *testing.T, enrolledAt, importedAt time.Time) *fixture {
	t.Helper()
	f := billingFixture(t)
	importPupil(f, enrolledAt, importedAt)
	return f
}

func importPupil(f *fixture, enrolledAt, importedAt time.Time) {
	f.enrollments.byCycle[testCycle] = []ports.EnrollmentRef{{
		ID: "enr-imp", StudentID: testStudent, GroupID: testGroup, CycleID: testCycle,
		Status: "active", EnrolledAt: enrolledAt, Origin: ports.EnrollmentOriginImport, ImportedOn: importedAt,
	}}
}

func lateFeeDescriptions(t *testing.T, f *fixture) []string {
	t.Helper()
	return chargeDescriptions(t, f, domain.KindLateFee)
}

// Enrolled from the start, an imported pupil owes the whole year from
// September and still no inscription.
func TestRunBillingChargesAnImportedPupilFromTheCycleStartSeptember(t *testing.T) {
	f := importedFromStartFixture(t, aug(2026, time.August, 1), importDay)

	got := runOn(t, f, "2026-10-17")

	if got.TuitionCreated != 10 || got.EnrollmentCreated != 0 {
		t.Fatalf("tuition = %d, inscriptions = %d; want September..June (10) and none", got.TuitionCreated, got.EnrollmentCreated)
	}
	if !issuedInMonth(issuedTo(t, f, testStudent), "septiembre") {
		t.Error("September was not billed to a pupil enrolled from the cycle start")
	}
}

// September and October fell due before the import (10 Sep, 10 Oct < 17 Oct):
// the monthly run never surcharges them. November, due after, is late as usual.
func TestRunBillingNeverSurchargesAMonthDueBeforeTheImport(t *testing.T) {
	f := importedFromStartFixture(t, aug(2026, time.August, 1), importDay)

	if got := runOn(t, f, "2026-10-17"); got.LateFeesCreated != 0 {
		t.Fatalf("late fees on the import day = %d %v; want none", got.LateFeesCreated, lateFeeDescriptions(t, f))
	}
	runOn(t, f, "2026-12-15")

	fees := lateFeeDescriptions(t, f)
	if len(fees) != 1 || !strings.Contains(fees[0], "noviembre") {
		t.Fatalf("late fees by 15 Dec = %v; want only November's", fees)
	}
}

// A fee split September/October is owed whole from the start, and its shares
// due before the import are not surcharged either.
func TestRunBillingNeverSurchargesAFeeShareDueBeforeTheImport(t *testing.T) {
	f := importedFromStartFixture(t, aug(2026, time.August, 1), importDay)
	if _, err := f.svc.UpsertTuitionPlan(context.Background(), usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel, EnrollmentCents: 50000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, LateFeeCents: 15000, Active: true,
		Fees: []domain.Fee{{Name: "Cuota de Julio", AmountCents: 160000, Months: []int{9, 10}}},
	}); err != nil {
		t.Fatalf("seed plan with a fee: %v", err)
	}

	got := runOn(t, f, "2026-11-11")

	if shares := chargeDescriptions(t, f, domain.KindFee); len(shares) != 2 {
		t.Fatalf("fee shares = %v; want both (September and October)", shares)
	}
	if got.LateFeesCreated != 0 {
		t.Fatalf("late fees = %d %v; want none — every overdue line fell due before the import", got.LateFeesCreated, lateFeeDescriptions(t, f))
	}
}

// An enrollment without an import instant (a students that does not send it
// yet) keeps the normal rule: no grace is invented.
func TestRunBillingWithoutAnImportDaySurchargesAsUsual(t *testing.T) {
	f := importedFromStartFixture(t, aug(2026, time.August, 1), time.Time{})

	if got := runOn(t, f, "2026-10-17"); got.LateFeesCreated != 1 {
		t.Fatalf("late fees = %d; want September's one", got.LateFeesCreated)
	}
}

// The repaired pupil: billed on 7 October from an import dated the 6th, then
// moved to the cycle start by the re-upload. The next run adds only September,
// without a surcharge; October, already issued, is not issued again.
func TestRunBillingAfterTheMoveAddsOnlySeptemberWithoutASurcharge(t *testing.T) {
	importedOn6 := time.Date(2026, time.October, 6, 16, 0, 0, 0, time.UTC)
	f := importedFromStartFixture(t, importedOn6, importedOn6)
	if first := runOn(t, f, "2026-10-07"); first.TuitionCreated != 9 || first.LateFeesCreated != 0 {
		t.Fatalf("first run = %+v; want October..June and no surcharge", first)
	}
	f.enrollments.byCycle[testCycle][0].EnrolledAt = aug(2026, time.August, 1)

	got := runOn(t, f, "2026-10-17")

	if got.TuitionCreated != 1 || got.LateFeesCreated != 0 {
		t.Fatalf("after the move = %+v; want September only, with no surcharge", got)
	}
	if again := runOn(t, f, "2026-10-17"); again.TuitionCreated != 0 || again.LateFeesCreated != 0 {
		t.Fatalf("second run = %+v; want nothing new", again)
	}
}

// importedDailyFixture is the owner's daily rule ($10 a day, every day) with
// only September billed, the pupil imported on 17 October and the clock there.
func importedDailyFixture(t *testing.T, importedAt time.Time) *fixture {
	t.Helper()
	f := dailyFixture(t)
	onlySeptember(t, f, usecase.TuitionPlanInput{
		EnrollmentCents: 50000, MonthlyCents: 320000,
		Policy: domain.CollectionPolicy{DailyMode: true, LateFeeDailyCents: 1000, SurchargeCountsAllDays: true},
	})
	importPupil(f, aug(2026, time.August, 1), importedAt)
	runOn(t, f, "2026-10-17")
	f.clock = noon(2026, time.October, 17)
	return f
}

// Every surface that prices the daily surcharge on read — the account, the
// statement, the family's account and statement, the counter — owes
// September at its normal price and nothing more.
func TestEverySurfaceOwesTheImportedPupilsSeptemberWithoutASurcharge(t *testing.T) {
	requireOneBalance(t, importedDailyFixture(t, importDay), 320000)
}

// The same pupil without an import day is 37 days late (11 Sep..17 Oct): the
// grace is the import's, not a change to the daily rule.
func TestWithoutAnImportDayTheDailyRuleStillAccrues(t *testing.T) {
	requireOneBalance(t, importedDailyFixture(t, time.Time{}), 320000+37*1000)
}

// Paying it at the counter writes no surcharge row and earns no early-payment
// reward: the normal price, exactly.
func TestADepositSettlesTheImportedPupilsSeptemberAtTheNormalPrice(t *testing.T) {
	f := importedDailyFixture(t, importDay)

	got := deposit(t, f, 320000, "2026-10-17")

	assert.Empty(t, got.Surcharges, "a surcharge row was written for a month due before the import")
	assert.Zero(t, got.UnallocatedCents)
	require.NotEmpty(t, got.Allocations)
	requireOneBalance(t, f, 0)
}

func issuedInMonth(issued map[string]int64, month string) bool {
	for name := range issued {
		if strings.Contains(name, month) {
			return true
		}
	}
	return false
}
