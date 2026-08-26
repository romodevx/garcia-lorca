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

const (
	testLevel = "lv-primaria"
	testGroup = "grp-1a"
)

// billingFixture is a school with one level, one group, one enrolled pupil and a
// tuition table: ten payments of $3,200 from September, due on the 10th, plus a
// $500 inscription and a $150 surcharge per overdue month.
func billingFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.cycles.byID[testCycle] = ports.CycleRef{
		ID:       testCycle,
		StartsOn: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
	}
	f.catalog.levels = []ports.LevelRef{{ID: testLevel, Name: "Primaria"}}
	f.catalog.groupLevels = []ports.GroupLevel{{GroupID: testGroup, LevelID: testLevel}}
	f.enrollments.byCycle[testCycle] = []ports.EnrollmentRef{
		{ID: "enr-1", StudentID: testStudent, GroupID: testGroup, CycleID: testCycle, Status: "active"},
	}
	if _, err := f.svc.UpsertTuitionPlan(context.Background(), usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel,
		EnrollmentCents: 50000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, LateFeeCents: 15000, Active: true,
	}); err != nil {
		t.Fatalf("seed tuition plan: %v", err)
	}
	return f
}

func runOn(t *testing.T, f *fixture, day string) usecase.BillingResult {
	t.Helper()
	got, err := f.svc.RunBilling(context.Background(), testCycle, day)
	if err != nil {
		t.Fatalf("RunBilling(%s): %v", day, err)
	}
	return got
}

// The run bills only the months whose due date has ARRIVED. A school that opens
// in September must not hand a family ten months of debt on day one.
func TestRunBillingChargesOnlyTheMonthsThatHaveComeDue(t *testing.T) {
	f := billingFixture(t)

	// 15 November: September, October and November are due. December is not.
	got := runOn(t, f, "2026-11-15")

	if got.TuitionCreated != 3 {
		t.Errorf("tuition created = %d, want 3 (Sep, Oct, Nov)", got.TuitionCreated)
	}
	if got.EnrollmentCreated != 1 {
		t.Errorf("enrollment created = %d, want 1", got.EnrollmentCreated)
	}
}

// The property that lets a button and a scheduler both call this.
func TestRunBillingIsIdempotent(t *testing.T) {
	f := billingFixture(t)

	first := runOn(t, f, "2026-11-15")
	second := runOn(t, f, "2026-11-15")

	if second.TuitionCreated != 0 || second.EnrollmentCreated != 0 || second.LateFeesCreated != 0 {
		t.Errorf("a second run created something: %+v", second)
	}
	// Everything the first run made — surcharges included — is skipped, not
	// re-issued. That total IS the idempotency claim.
	made := first.TuitionCreated + first.EnrollmentCreated + first.LateFeesCreated
	if second.Skipped != made {
		t.Errorf("skipped = %d, want %d — everything the first run made", second.Skipped, made)
	}
}

// The surcharge: one fixed fee per COMPLETED overdue month, per unpaid charge.
func TestRunBillingAddsOneSurchargePerCompletedOverdueMonth(t *testing.T) {
	f := billingFixture(t)

	// By 12 November nothing has been paid. September (due the 10th) is two
	// months overdue, October one, November none. The inscription, due when the
	// cycle opened, is three.
	got := runOn(t, f, "2026-11-12")

	// 2 (Sep) + 1 (Oct) + 0 (Nov) + 3 (inscription, due 2026-08-10) = 6
	if got.LateFeesCreated != 6 {
		t.Errorf("late fees = %d, want 6", got.LateFeesCreated)
	}
}

// A surcharge must never earn a surcharge of its own: that is how a fixed fee
// silently turns into compound interest.
func TestSurchargesDoNotThemselvesAccrueSurcharges(t *testing.T) {
	f := billingFixture(t)
	runOn(t, f, "2026-11-12")

	// Three months later the original charges have accrued more, but not one
	// extra penny of it may come from the surcharges already issued.
	later := runOn(t, f, "2027-02-12")

	lateFeeConcept := f.repo.conceptIDOfKind(domain.KindLateFee)
	surcharges := map[string]bool{}
	for _, c := range f.repo.charges {
		if c.ConceptID() == lateFeeConcept {
			surcharges[c.ID()] = true
		}
	}
	for _, c := range f.repo.charges {
		if !strings.HasPrefix(c.ExternalRef(), "latefee:") {
			continue
		}
		// latefee:<chargeId>:<YYYY-MM> — the middle field must never name a
		// charge that is itself a surcharge, or a fixed monthly fee has quietly
		// become compound interest.
		parent := strings.Split(c.ExternalRef(), ":")[1]
		if surcharges[parent] {
			t.Fatalf("a surcharge accrued a surcharge: %s", c.ExternalRef())
		}
	}
	if later.LateFeesCreated == 0 {
		t.Error("three months on, the unpaid tuition should have accrued more")
	}
}

// A level nobody priced is REPORTED, never billed at zero. Silently charging a
// family nothing is the failure a school finds out about in June.
func TestRunBillingReportsLevelsWithNoTuitionTable(t *testing.T) {
	f := billingFixture(t)
	f.catalog.levels = append(f.catalog.levels, ports.LevelRef{ID: "lv-kinder", Name: "Kínder"})

	got := runOn(t, f, "2026-11-15")

	if len(got.LevelsWithoutPlan) != 1 || got.LevelsWithoutPlan[0] != "Kínder" {
		t.Errorf("levels without a plan = %v, want [Kínder]", got.LevelsWithoutPlan)
	}
}

// Only live enrolments are billed. A pupil who withdrew in October does not keep
// receiving tuition for the rest of the year.
func TestRunBillingIgnoresEnrolmentsThatAreNotActive(t *testing.T) {
	f := billingFixture(t)
	f.enrollments.byCycle[testCycle] = []ports.EnrollmentRef{
		{ID: "enr-1", StudentID: testStudent, GroupID: testGroup, CycleID: testCycle, Status: "withdrawn"},
	}

	got := runOn(t, f, "2026-11-15")

	if got.TuitionCreated != 0 || got.EnrollmentCreated != 0 {
		t.Errorf("a withdrawn pupil was billed: %+v", got)
	}
}

func TestUpsertTuitionPlanRejectsAnUnknownCycle(t *testing.T) {
	f := newFixture(t)

	_, err := f.svc.UpsertTuitionPlan(context.Background(), usecase.TuitionPlanInput{
		CycleID: "nope", LevelID: testLevel, MonthlyCents: 1, Months: 10,
		FirstBilledMonth: 9, DueDay: 10, Active: true,
	})

	if err == nil {
		t.Fatal("a tuition table for a cycle that does not exist must be refused")
	}
}
