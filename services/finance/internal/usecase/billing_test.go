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
		if strings.Contains(c.Charge.Description(), "octubre 2026") {
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

const cycleOpens = "2026-08-01"

func aug(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// ladderFixture is the school with the flyer's three-line inscription in place
// of one amount: $2,200 until November, $3,300 after.
func ladderFixture(t *testing.T) *fixture {
	t.Helper()
	f := billingFixture(t)
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
			{Name: "Inscripción", Tiers: []domain.PriceTier{
				{EffectiveFrom: aug(2026, time.August, 1), AmountCents: 220000},
				{EffectiveFrom: aug(2026, time.November, 1), AmountCents: 330000},
			}},
			flat("Gastos adm y seguro", 470000),
			flat("Cuota Asociación Padres de Familia", 85000),
		},
	}); err != nil {
		t.Fatalf("seed ladder plan: %v", err)
	}
	return f
}

// issuedTo maps a pupil's charges by description.
func issuedTo(t *testing.T, f *fixture, student string) map[string]int64 {
	t.Helper()
	list, err := f.svc.ListCharges(context.Background(), student, testCycle)
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	out := map[string]int64{}
	for _, c := range list {
		out[c.Charge.Description()] = c.Charge.Amount().Int64()
	}
	return out
}

// R1. The run must bill the LINES, never the single amount a concepts plan no
// longer carries.
//
// A plan with concepts is forced to EnrollmentCents == 0 (ADR 0011) and
// NewCents(0) is legal, so a run still reading that field issues a $0
// inscription to every pupil of the level — and CreateChargesBulk's ON CONFLICT
// makes that zero PERMANENT. It is the "family charged nothing is a problem a
// school discovers in June" failure this file warns about, produced by the
// feature meant to prevent it.
func TestRunBillingIssuesOneChargePerInscriptionLine(t *testing.T) {
	f := ladderFixture(t)

	got := runOn(t, f, "2026-09-15")

	if got.EnrollmentCreated != 3 {
		t.Fatalf("enrollment charges = %d, want the flyer's 3 lines", got.EnrollmentCreated)
	}
	issued := issuedTo(t, f, testStudent)
	for name, want := range map[string]int64{
		"Inscripción":                        220000,
		"Gastos adm y seguro":                470000,
		"Cuota Asociación Padres de Familia": 85000,
	} {
		if issued[name] != want {
			t.Errorf("%q = %d, want %d", name, issued[name], want)
		}
	}
	for name, amount := range issued {
		if amount == 0 {
			t.Errorf("%q was issued for nothing", name)
		}
	}
}

// The ladder goes on climbing after the charge exists, and the run says how many
// prices it moved: one that changes four hundred in silence is one nobody can audit.
func TestRunBillingRepricesTheUnpaidInscription(t *testing.T) {
	f := ladderFixture(t)
	runOn(t, f, "2026-09-15")

	got := runOn(t, f, "2026-11-15")

	if got.EnrollmentRepriced != 1 {
		t.Fatalf("repriced = %d, want 1", got.EnrollmentRepriced)
	}
	if amount := issuedTo(t, f, testStudent)["Inscripción"]; amount != 330000 {
		t.Errorf("Inscripción = %d, want November's 330000", amount)
	}
}

// R6. A scholarship is a price a PERSON decided. The ladder is the list price,
// and it must never overwrite a decision somebody made by hand.
//
// The 100% case is the one that proves it: a free inscription never receives a
// peso, so the freeze rule — which keys on payments — never protects it. Raising
// the amount while the discount stands turns a gift into a debt.
func TestTheLadderNeverRepricesWhatSomebodyNegotiated(t *testing.T) {
	f := ladderFixture(t)
	runOn(t, f, "2026-09-15")

	list, err := f.svc.ListCharges(context.Background(), testStudent, testCycle)
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	var inscription string
	for _, c := range list {
		if c.Charge.Description() == "Inscripción" {
			inscription = c.Charge.ID()
		}
	}
	// The whole inscription forgiven: a full scholarship.
	if _, err := f.svc.ApplyDiscount(context.Background(), inscription, 220000, "beca completa"); err != nil {
		t.Fatalf("ApplyDiscount: %v", err)
	}

	got := runOn(t, f, "2026-11-15")

	if got.EnrollmentRepriced != 0 {
		t.Errorf("repriced %d negotiated charges, want 0", got.EnrollmentRepriced)
	}
	after, err := f.svc.ListCharges(context.Background(), testStudent, testCycle)
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	for _, c := range after {
		if c.Charge.ID() != inscription {
			continue
		}
		if owed := c.Charge.Effective().Int64(); owed != 0 {
			t.Errorf("the free inscription now owes %d — a gift became a debt", owed)
		}
	}
}

// A plan that still carries one amount goes on billing exactly one inscription.
func TestRunBillingStillIssuesTheSingleInscription(t *testing.T) {
	f := billingFixture(t)

	got := runOn(t, f, "2026-09-15")

	if got.EnrollmentCreated != 1 {
		t.Fatalf("enrollment charges = %d, want 1", got.EnrollmentCreated)
	}
	if amount := issuedTo(t, f, testStudent)["Inscripción"]; amount != 50000 {
		t.Errorf("Inscripción = %d, want the plan's 50000", amount)
	}
}

// D3. A pupil who arrives in March owes March onwards, not the whole year.
func TestRunBillingDoesNotChargeAMidYearJoinerForMonthsBeforeTheyArrived(t *testing.T) {
	f := billingFixture(t)
	f.enrollments.byCycle[testCycle] = []ports.EnrollmentRef{{
		ID: "enr-late", StudentID: "stu-late", GroupID: testGroup, CycleID: testCycle,
		Status: "active", EnrolledAt: aug(2027, time.March, 20),
	}}

	got := runOn(t, f, "2027-03-25")

	if got.TuitionCreated != 4 {
		t.Errorf("tuition = %d, want March..June — 4 months", got.TuitionCreated)
	}
	for name := range issuedTo(t, f, "stu-late") {
		if strings.Contains(name, "septiembre") || strings.Contains(name, "octubre") {
			t.Errorf("%q was billed to a pupil who arrived in March", name)
		}
	}
}

// T13. The counter must collect the price of the day the money arrives.
//
// The nightly run writes the tier down, but a tier that turns over at midnight
// leaves the whole of the next day quoting yesterday's price until the run
// fires. So the payment path prices it again at paidOn, before the money is
// split — and that write is what freezes it.
func TestPayingCollectsTheTierInForceOnTheDayTheMoneyArrives(t *testing.T) {
	f := ladderFixture(t)
	runOn(t, f, "2026-09-15")

	// November: the inscription's second rung. No run has fired since.
	f.clock = aug(2026, time.November, 5)
	if _, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 330000, Method: domain.MethodCash,
		PaidAt: aug(2026, time.November, 5),
	}); err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}

	list, err := f.svc.ListCharges(context.Background(), testStudent, testCycle)
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	for _, c := range list {
		if c.Charge.Description() != "Inscripción" {
			continue
		}
		if amount := c.Charge.Amount().Int64(); amount != 330000 {
			t.Fatalf("Inscripción = %d, want November's 330000", amount)
		}
		if c.PaidCents != 330000 {
			t.Errorf("collected %d against it, want the whole 330000", c.PaidCents)
		}
		return
	}
	t.Fatal("no inscription charge found")
}
