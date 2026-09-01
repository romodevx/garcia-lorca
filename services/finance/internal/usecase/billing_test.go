package usecase_test

import (
	"context"
	"fmt"
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

// The run issues the WHOLE year at once: the eleven months and the inscription,
// due dates and all.
//
// The school asked for this in as many words — "el monto total que debe pagar el
// año" — and it is what makes the rest work. A family that hands over $6,000 in
// September has six months marked paid, which cannot happen if those months are
// not charges yet. The earlier rule (bill a month when it falls due) left the
// other five as an unexplained credit.
func TestRunBillingIssuesTheWholeYearUpFront(t *testing.T) {
	f := billingFixture(t)

	// 15 November, but the year is billed in full: the plan has ten months.
	got := runOn(t, f, "2026-11-15")

	if got.TuitionCreated != 10 {
		t.Errorf("tuition created = %d, want 10 — the whole year", got.TuitionCreated)
	}
	if got.EnrollmentCreated != 1 {
		t.Errorf("enrollment created = %d, want 1", got.EnrollmentCreated)
	}
}

// Billing the year up front must not bill the FUTURE as overdue. A month that
// has not arrived owes nothing extra, and a family opening the app in September
// must not find surcharges on June.
func TestTheFutureMonthsCarryNoSurcharge(t *testing.T) {
	f := billingFixture(t)

	got := runOn(t, f, "2026-11-12")

	// Only September (2 months late), October (1) and the inscription (3).
	if got.LateFeesCreated != 6 {
		t.Errorf("late fees = %d, want 6 — nothing accrues on a month that has not come due", got.LateFeesCreated)
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

// TestRunBillingAccruesBeyondTheListCap is the regression for a live money bug.
//
// ListOverdue caps at 500 rows. A school with 400 pupils and eleven monthly
// charges holds ~4,400 charges per cycle, so once more than 500 were overdue the
// run stopped accruing surcharges on the newest debt — silently. The school
// under-billed and nobody was told.
//
// The unit tests could not see it because the fake repository returned
// everything: the double was MORE capable than production. It now models the cap
// on ListOverdue and offers the paged read the run actually uses.
func TestRunBillingAccruesBeyondTheListCap(t *testing.T) {
	f := billingFixture(t)
	const pupils = 700 // deliberately more than the 500-row cap

	rows := make([]ports.EnrollmentRef, 0, pupils)
	for i := 0; i < pupils; i++ {
		rows = append(rows, ports.EnrollmentRef{
			ID:        fmt.Sprintf("enr-%04d", i),
			StudentID: fmt.Sprintf("st-%04d", i),
			GroupID:   testGroup,
			CycleID:   testCycle,
			Status:    "active",
		})
	}
	f.enrollments.byCycle[testCycle] = rows

	// ONE run, one assertion. The enrolment charge falls due when the cycle
	// opens (2026-08-10), so by 15 September every one of the 700 pupils is
	// overdue on it and must earn exactly one surcharge.
	//
	// Asserting across two runs would be worse than useless: the totals
	// accumulate and cross 700 by accident even while each individual run is
	// capped at 500. This test was written that way first and passed against the
	// bug it exists to catch.
	got := runOn(t, f, "2026-09-15")

	if got.LateFeesCreated != pupils {
		t.Fatalf("late fees = %d, want exactly %d — the run stopped at the read cap and silently under-billed the newest debt",
			got.LateFeesCreated, pupils)
	}
}

// dailyFixture is the same school on the owner's new rule: $10 a day from the
// day after the due date, every day of the week, and no fixed monthly fee.
func dailyFixture(t *testing.T) *fixture {
	t.Helper()
	f := billingFixture(t)
	if _, err := f.svc.UpsertTuitionPlan(context.Background(), usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel,
		EnrollmentCents: 50000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, Active: true,
		Policy: domain.CollectionPolicy{
			DailyMode: true, LateFeeDailyCents: 1000, SurchargeCountsAllDays: true,
		},
	}); err != nil {
		t.Fatalf("seed daily plan: %v", err)
	}
	return f
}

// Under the daily rule the monthly run must NOT issue surcharge charges. What is
// owed in surcharge changes every night; a charge written today would be wrong
// tomorrow and would still be sitting in the family's balance. The amount is
// derived at the moment of payment instead.
func TestDailyModeIssuesNoSurchargeCharges(t *testing.T) {
	f := dailyFixture(t)

	got := runOn(t, f, "2026-11-12")

	if got.LateFeesCreated != 0 {
		t.Errorf("late fees created = %d, want 0 — the daily rule is derived, not charged", got.LateFeesCreated)
	}
	// The tuition itself is still billed — the whole year of it. Turning the
	// surcharge rule off must not turn the school's income off with it.
	if got.TuitionCreated != 10 {
		t.Errorf("tuition created = %d, want 10", got.TuitionCreated)
	}
}

// A due date on a day the school is shut is a date nobody could have paid on.
// It moves to the next working day, and only then does being late start
// costing: the review asks for exactly this, and the surcharge rule counting
// every day makes it matter — an unmoved holiday would charge a family for the
// day the office was closed.
func TestADueDateOnAClosedDayMovesToTheNextWorkingDay(t *testing.T) {
	f := billingFixture(t)
	// 10 October 2026 is a Saturday, and the school also closes the Monday.
	f.calendar.days = []time.Time{time.Date(2026, time.October, 12, 0, 0, 0, 0, time.UTC)}

	runOn(t, f, "2026-11-15")

	charges, err := f.svc.ListCharges(context.Background(), testStudent, testCycle)
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	var october time.Time
	for _, c := range charges {
		if strings.Contains(c.Charge.Description(), "2026-10") {
			october = c.Charge.DueDate()
		}
	}
	if october.IsZero() {
		t.Fatal("October was not billed")
	}
	// Saturday 10 → Sunday 11 → Monday 12 is closed → Tuesday 13.
	if october.Day() != 13 {
		t.Errorf("October fell due on the %d, want the 13th — the first day the school was open", october.Day())
	}
}

// The school's arrangement for maternal: a fixed amount off every month of that
// level. It is set once on the tuition table, and the run applies it to every
// pupil in the level without anybody typing it four hundred times.
func TestALevelDiscountComesOffEveryMonth(t *testing.T) {
	f := billingFixture(t)
	if _, err := f.svc.UpsertTuitionPlan(context.Background(), usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel,
		EnrollmentCents: 50000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, Active: true,
		MonthlyDiscountCents: 50000,
	}); err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	runOn(t, f, "2026-09-15")

	charges, err := f.svc.ListCharges(context.Background(), testStudent, testCycle)
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	tuition := f.repo.conceptIDOfKind(domain.KindTuition)
	var discounted, months int
	for _, c := range charges {
		if c.Charge.ConceptID() != tuition {
			continue
		}
		months++
		if c.Charge.Discount().Int64() == 50000 && c.Charge.Effective().Int64() == 270000 {
			discounted++
		}
	}
	if months == 0 || discounted != months {
		t.Errorf("%d of %d months carry the level discount, want all of them", discounted, months)
	}
	// And it says why, so a parent asking is answered by the row itself.
	for _, c := range charges {
		if c.Charge.ConceptID() == tuition && c.Charge.DiscountReason() != domain.ReasonLevelDiscount {
			t.Errorf("discount reason = %q, want the level arrangement", c.Charge.DiscountReason())
			break
		}
	}
}

// The inscription is not a month, so a monthly discount does not touch it.
func TestALevelDiscountDoesNotTouchTheInscription(t *testing.T) {
	f := billingFixture(t)
	if _, err := f.svc.UpsertTuitionPlan(context.Background(), usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel,
		EnrollmentCents: 50000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, Active: true,
		MonthlyDiscountCents: 50000,
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
		if c.Charge.ConceptID() == enrollment && c.Charge.Discount().Int64() != 0 {
			t.Errorf("the inscription was discounted by %d", c.Charge.Discount().Int64())
		}
	}
}
