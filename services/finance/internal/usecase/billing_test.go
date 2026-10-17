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
// everything: the double was MORE capable than production. It now offers only
// the keyset pages the run actually walks, cut the way the real query cuts them.
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

	charges, err := allCharges(context.Background(), f.svc, testStudent, testCycle)
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

	charges, err := allCharges(context.Background(), f.svc, testStudent, testCycle)
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

	charges, err := allCharges(context.Background(), f.svc, testStudent, testCycle)
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
	return ladderFixtureFrom(t, aug(2026, time.November, 1))
}

// ladderFixtureFrom is the same school with the second rung starting on a day
// the caller chooses, so two of them can differ in exactly one thing.
func ladderFixtureFrom(t *testing.T, secondRungFrom time.Time) *fixture {
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
				{EffectiveFrom: secondRungFrom, AmountCents: 330000},
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
	list, err := allCharges(context.Background(), f.svc, student, testCycle)
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

	list, err := allCharges(context.Background(), f.svc, testStudent, testCycle)
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
	after, err := allCharges(context.Background(), f.svc, testStudent, testCycle)
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
// There is NO nightly run — RunBilling fires only when somebody presses
// "generar" — so between one press and the next, the payment path is what puts
// today's rung on the charge, before the money is split. That write is also what
// freezes it.
func TestPayingCollectsTheTierInForceOnTheDayTheMoneyArrives(t *testing.T) {
	f := ladderFixture(t)
	runOn(t, f, "2026-09-15")

	// November: the inscription's second rung. No run has fired since. Noon:
	// midnight UTC is still the 4th in Morelia, and the 5th would be tomorrow.
	f.clock = aug(2026, time.November, 5).Add(12 * time.Hour)
	if _, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 330000, Method: domain.MethodCash,
		PaidAt: aug(2026, time.November, 5),
	}); err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}

	list, err := allCharges(context.Background(), f.svc, testStudent, testCycle)
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

// Etapa 4. The statement the FAMILY reads must say what is owed TODAY.
//
// Until now GetAccount was the plain arithmetic of charge minus paid — the
// whole year still owing — while the counter quoted the derived figure with the
// accrued surcharge and the early-payment reward. Two numbers that disagreed by
// design: the office said one thing and the app in the parent's hand said
// another.
func TestTheStatementSaysWhatIsOwedToday(t *testing.T) {
	f := billingFixture(t)
	runOn(t, f, "2026-09-15")
	// Midday UTC, 06:00 in Morelia: midnight UTC would be 18:00 on the 19th,
	// and the school's day decides AsOf (ADR 0035).
	f.clock = aug(2026, time.November, 20).Add(12 * time.Hour)

	got, err := f.svc.GetAccount(context.Background(), testStudent)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}

	// The year still owing is unchanged: ten months, the inscription, and the
	// surcharges the run wrote down.
	if got.TotalBalanceCents <= 0 {
		t.Fatalf("total balance = %d, want the year still owed", got.TotalBalanceCents)
	}
	// What is being asked for today is a DIFFERENT, smaller number: only the
	// months that have come due.
	if got.DueTodayCents <= 0 {
		t.Fatal("due today = 0 — the statement says the family owes nothing")
	}
	if got.DueTodayCents >= got.TotalBalanceCents {
		t.Errorf("due today (%d) is not less than the year (%d) — the two numbers are the same figure",
			got.DueTodayCents, got.TotalBalanceCents)
	}
	if got.AsOf != "2026-11-20" {
		t.Errorf("as of %q, want 2026-11-20 — a screen that cannot say which day it quotes cannot be trusted", got.AsOf)
	}
}

// The parts have to add up, or a family asking "why that number?" is answered by
// a phone call instead of by the statement.
func TestTheStatementExplainsTheNumberItQuotes(t *testing.T) {
	f := billingFixture(t)
	runOn(t, f, "2026-09-15")
	f.clock = aug(2026, time.November, 20)

	account, err := f.svc.GetAccount(context.Background(), testStudent)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	expected, err := f.svc.ExpectedToday(context.Background(), testStudent, f.clock)
	if err != nil {
		t.Fatalf("ExpectedToday: %v", err)
	}

	// One arithmetic, not two: the statement quotes exactly what the counter
	// would collect on that day.
	if account.DueTodayCents != expected.AmountCents {
		t.Errorf("statement says %d, counter says %d", account.DueTodayCents, expected.AmountCents)
	}
	if account.SurchargeCents != expected.SurchargeCents {
		t.Errorf("surcharge %d vs %d", account.SurchargeCents, expected.SurchargeCents)
	}
	if account.EarlyDiscountCents != expected.DiscountCents {
		t.Errorf("early discount %d vs %d", account.EarlyDiscountCents, expected.DiscountCents)
	}
}

// B1. The counter must quote the ladder, not the amount last written down.
//
// There is no nightly run: RunBilling fires only when somebody presses
// "generar". So on 5 November the stored inscription still says September's
// price, and a read path that trusts it quotes $2,200 while the payment path —
// which DOES apply the ladder — takes the money at $3,300 inside the same
// transaction. The family walks out with a receipt for one number owing
// another, and the freeze rule has just fixed the price at the higher rung.
//
// Asserted as a DELTA between two schools identical in every way except WHEN
// the second rung starts. Everything else — tuition months, surcharges, the
// flat lines — cancels out, so what is left is the rung itself and nothing else
// can make this pass by accident.
func TestWhatIsQuotedIsWhatTheLadderSaysToday(t *testing.T) {
	const when = "2026-11-05"

	quote := func(t *testing.T, secondRungFrom time.Time) int64 {
		t.Helper()
		f := ladderFixtureFrom(t, secondRungFrom)
		runOn(t, f, "2026-09-15")
		day := aug(2026, time.November, 5)
		f.clock = day
		got, err := f.svc.ExpectedToday(context.Background(), testStudent, day)
		if err != nil {
			t.Fatalf("ExpectedToday: %v", err)
		}
		return got.AmountCents
	}

	// One school's second rung started on 1 November; the other's starts in
	// December and has not arrived on the 5th.
	arrived := quote(t, aug(2026, time.November, 1))
	notYet := quote(t, aug(2026, time.December, 1))

	if got := arrived - notYet; got != 110000 {
		t.Fatalf("the quote moved by %d on %s, want the rung's 110000 — a quote that does not "+
			"move is reading the stored amount, and the counter will collect more than it showed",
			got, when)
	}
}

// The same rule the ledger uses: a negotiated price is not re-quoted either.
// Otherwise the screen would ask a scholarship family for the list price, and
// the money taken would not match the money shown.
func TestTheQuoteDoesNotRePriceWhatSomebodyNegotiated(t *testing.T) {
	quote := func(t *testing.T, secondRungFrom time.Time) int64 {
		t.Helper()
		f := ladderFixtureFrom(t, secondRungFrom)
		runOn(t, f, "2026-09-15")

		list, err := allCharges(context.Background(), f.svc, testStudent, testCycle)
		if err != nil {
			t.Fatalf("ListCharges: %v", err)
		}
		for _, c := range list {
			if c.Charge.Description() != "Inscripción" {
				continue
			}
			// The whole inscription forgiven: a full scholarship.
			if _, err := f.svc.ApplyDiscount(context.Background(), c.Charge.ID(), 220000, "beca completa"); err != nil {
				t.Fatalf("ApplyDiscount: %v", err)
			}
		}
		day := aug(2026, time.November, 5)
		f.clock = day
		got, err := f.svc.ExpectedToday(context.Background(), testStudent, day)
		if err != nil {
			t.Fatalf("ExpectedToday: %v", err)
		}
		return got.AmountCents
	}

	if got := quote(t, aug(2026, time.November, 1)) - quote(t, aug(2026, time.December, 1)); got != 0 {
		t.Errorf("the quote moved by %d on a negotiated inscription, want 0", got)
	}
}

// SEC-1. A back-dated payment must not walk the price DOWN.
//
// paidAt comes from the client and is only bounded in the future. Before this
// guard it reached the ladder: a $1 deposit dated in December priced March's
// inscription at the earliest rung, that peso froze it, and the $1,100
// difference left no discount row, no reason and no event. Repeatable pupil by
// pupil, and quieter than doing it by hand — which is what makes it the path an
// insider picks.
func TestABackDatedPaymentCannotLowerTheInscription(t *testing.T) {
	f := ladderFixture(t)
	runOn(t, f, "2026-09-15")
	// November: the inscription is at the second rung.
	f.clock = aug(2026, time.November, 20)
	runOn(t, f, "2026-11-20")
	if got := issuedTo(t, f, testStudent)["Inscripción"]; got != 330000 {
		t.Fatalf("setup: Inscripción = %d, want November's 330000", got)
	}

	// A peso, dated back before the second rung ever started.
	if _, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 100, Method: domain.MethodCash,
		PaidAt: aug(2026, time.September, 1),
	}); err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}

	if got := issuedTo(t, f, testStudent)["Inscripción"]; got != 330000 {
		t.Errorf("Inscripción = %d after a back-dated peso, want it held at 330000", got)
	}
}

// Task 7 (owner's decision). A family that paid before the price rose, and was
// captured after it, pays the price of the day they paid — but only when THAT
// payment settles the inscription at that price. Otherwise the price is
// today's, which is what keeps the $1 trick above closed.
func TestAPaymentThatSettlesTheInscriptionPaysThePriceOfItsDate(t *testing.T) {
	f := ladderFixture(t)
	runOn(t, f, "2026-09-15")
	f.clock = aug(2026, time.November, 20) // nobody has pressed "generar" since

	if _, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 220000, Method: domain.MethodCash,
		PaidAt: aug(2026, time.October, 15),
	}); err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}

	inscription := chargeNamed(t, f, testStudent, "Inscripción")
	if inscription.Charge.Amount().Int64() != 220000 || inscription.PaidCents != 220000 {
		t.Errorf("Inscripción = %d, paid %d; want 220000 settled at October's price",
			inscription.Charge.Amount().Int64(), inscription.PaidCents)
	}
}

// The inscription was already raised (a run after the price change). Settling
// it at the price of the payment date lowers it — and a lowering is never
// silent: it is a discount with its own reason.
func TestSettlingAtAnEarlierPriceLeavesAReasonedDiscount(t *testing.T) {
	f := ladderFixture(t)
	runOn(t, f, "2026-09-15")
	f.clock = aug(2026, time.November, 20)
	runOn(t, f, "2026-11-20")

	if _, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 220000, Method: domain.MethodCash,
		PaidAt: aug(2026, time.October, 15),
	}); err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}

	inscription := chargeNamed(t, f, testStudent, "Inscripción")
	c := inscription.Charge
	if c.Amount().Int64() != 330000 || c.Discount().Int64() != 110000 || c.DiscountReason() != domain.ReasonPriceAtPaymentDate {
		t.Errorf("Inscripción = %d − %d (%q); want 330000 − 110000 (%q)",
			c.Amount().Int64(), c.Discount().Int64(), c.DiscountReason(), domain.ReasonPriceAtPaymentDate)
	}
	if inscription.PaidCents != 220000 {
		t.Errorf("paid = %d, want 220000", inscription.PaidCents)
	}
}

// A back-dated payment that does NOT settle the inscription pays today's
// price: only a full settlement earns the price of its date.
func TestABackDatedPartPaymentPaysTodaysPrice(t *testing.T) {
	f := ladderFixture(t)
	runOn(t, f, "2026-09-15")
	f.clock = aug(2026, time.November, 20)

	if _, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 100000, Method: domain.MethodCash,
		PaidAt: aug(2026, time.October, 15),
	}); err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}

	if got := chargeNamed(t, f, testStudent, "Inscripción").Charge.Amount().Int64(); got != 330000 {
		t.Errorf("Inscripción = %d, want today's 330000", got)
	}
}

// The quote for a payment date agrees with what that payment would be charged.
func TestTheQuoteForAPaymentDateAgreesWithTheCollection(t *testing.T) {
	f := ladderFixture(t)
	runOn(t, f, "2026-09-15")
	f.clock = aug(2026, time.November, 20)
	runOn(t, f, "2026-11-20")

	quote, err := f.svc.PreviewDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 220000, Method: domain.MethodCash,
		PaidAt: aug(2026, time.October, 15),
	})
	if err != nil {
		t.Fatalf("PreviewDeposit: %v", err)
	}
	var toInscription int64
	for _, a := range quote.Allocations {
		if a.ChargeID() == chargeNamed(t, f, testStudent, "Inscripción").Charge.ID() {
			toInscription += a.Amount().Int64()
		}
	}
	if toInscription != 220000 {
		t.Errorf("the preview puts %d on the inscription, want all 220000 settling it", toInscription)
	}
}

// Owner's decision: no floor beyond the table itself. A date typed years back
// prices at the cycle's FIRST tier — a price the school set — and never below
// it; the discount is also capped by the climb the table shows.
func TestAPaymentDatedYearsBackReachesNothingBelowTheFirstTier(t *testing.T) {
	f := ladderFixture(t)
	runOn(t, f, "2026-09-15")
	f.clock = aug(2026, time.November, 20)
	runOn(t, f, "2026-11-20")

	if _, err := f.svc.RegisterDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 220000, Method: domain.MethodCash,
		PaidAt: aug(2020, time.January, 1),
	}); err != nil {
		t.Fatalf("RegisterDeposit: %v", err)
	}

	c := chargeNamed(t, f, testStudent, "Inscripción").Charge
	if c.Effective().Int64() != 220000 {
		t.Errorf("Inscripción settles at %d, want the first tier 220000 and nothing lower", c.Effective().Int64())
	}
}

func chargeNamed(t *testing.T, f *fixture, student, name string) ports.ChargeWithPaid {
	t.Helper()
	list, err := allCharges(context.Background(), f.svc, student, testCycle)
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	for _, c := range list {
		if c.Charge.Description() == name {
			return c
		}
	}
	t.Fatalf("no charge named %q", name)
	return ports.ChargeWithPaid{}
}

// SEC-2. The ratchet has to hold ACROSS saves, not only inside one list.
//
// validate() refuses a step cheaper than its predecessor within one concept.
// Nothing compared the new plan against what was already issued, so editing the
// table downward walked every unpaid inscription down with it — below what the
// families had already been quoted, and with nothing recording it.
func TestEditingTheTableDownwardDoesNotWalkIssuedChargesDown(t *testing.T) {
	f := ladderFixture(t)
	runOn(t, f, "2026-09-15")
	if got := issuedTo(t, f, testStudent)["Inscripción"]; got != 220000 {
		t.Fatalf("setup: Inscripción = %d, want 220000", got)
	}

	// The school re-prices the level cheaper — a correction, or a typo.
	if _, err := f.svc.UpsertTuitionPlan(context.Background(), usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel,
		MonthlyCents: 320000, Months: 10, FirstBilledMonth: 9, DueDay: 10,
		LateFeeCents: 15000, Active: true,
		EnrollmentConcepts: []domain.EnrollmentConcept{
			{Name: "Inscripción", Tiers: []domain.PriceTier{
				{EffectiveFrom: aug(2026, time.August, 1), AmountCents: 22000},
			}},
		},
	}); err != nil {
		t.Fatalf("UpsertTuitionPlan: %v", err)
	}

	got := runOn(t, f, "2026-09-20")

	if got.EnrollmentRepriced != 0 {
		t.Errorf("repriced %d charges downward, want 0", got.EnrollmentRepriced)
	}
	if amount := issuedTo(t, f, testStudent)["Inscripción"]; amount != 220000 {
		t.Errorf("Inscripción = %d, want it held at the 220000 already quoted", amount)
	}
}

// I1. D3 was only applied to the tuition. The inscription kept the cycle's first
// due day, so a pupil who joined on 20 March was handed one due 10 September —
// six months overdue the day it was issued, and accrueLateFees does not spare
// inscriptions. Six surcharges on the first run, for months they were not a
// pupil. It is the same money the tuition rule already refuses to charge.
func TestTheInscriptionOfAMidYearJoinerIsNotBornOverdue(t *testing.T) {
	f := billingFixture(t)
	joined := aug(2027, time.March, 20)
	// Noon UTC: EnrolledAt is an instant read in Morelia (ADR 0035), and
	// midnight UTC of the 20th is still the 19th there.
	f.enrollments.byCycle[testCycle] = []ports.EnrollmentRef{{
		ID: "enr-late", StudentID: "stu-late", GroupID: testGroup, CycleID: testCycle,
		Status: "active", EnrolledAt: joined.Add(12 * time.Hour),
	}}

	got := runOn(t, f, "2027-03-25")

	list, err := allCharges(context.Background(), f.svc, "stu-late", testCycle)
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	for _, c := range list {
		if c.Charge.Description() != "Inscripción" {
			continue
		}
		if c.Charge.DueDate().Before(joined) {
			t.Errorf("the inscription fell due %s, before the pupil joined on %s",
				c.Charge.DueDate().Format("2006-01-02"), joined.Format("2006-01-02"))
		}
	}
	if got.LateFeesCreated > 0 {
		t.Errorf("%d surcharges on a pupil who arrived five days ago", got.LateFeesCreated)
	}
}

func TestRunBillingSkipsEveryChargeForWithdrawnEnrollment(t *testing.T) {
	f := billingFixture(t)
	f.enrollments.byCycle[testCycle][0].Status = "withdrawn"
	if _, err := f.svc.UpsertTuitionPlan(context.Background(), usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel, EnrollmentCents: 50000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, LateFeeCents: 15000, Active: true,
		Fees: []domain.Fee{{Name: "Materiales", AmountCents: 20000, Months: []int{9, 10}}},
	}); err != nil {
		t.Fatalf("add fee: %v", err)
	}
	activities := newFakeActivityRepo()
	activity, err := domain.NewActivity(domain.ActivitySpec{
		ID: "act-basketball", Name: "Basquetbol",
		Schedule:        []domain.ActivitySlot{{Weekday: 2, StartMin: 15 * 60, EndMin: 16 * 60}},
		Periodicity:     domain.PeriodicityMonthly,
		BillingStartsOn: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		FirstDueOn:      time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC),
		Active:          true, Options: []domain.PriceOption{{ID: "opt-1", DaysPerWeek: 1, AmountCents: 25000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	activities.activities[activity.ID()] = activity
	activityEnrollment, err := domain.NewActivityEnrollment("act-enr-1", activity.ID(), testStudent, "opt-1", time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	activities.enrollments = append(activities.enrollments, activityEnrollment)
	f.svc.WithActivities(activities)

	got := runOn(t, f, "2026-10-15")
	if got.TuitionCreated != 0 || got.EnrollmentCreated != 0 || got.FeesCreated != 0 || got.ActivitiesCreated != 0 {
		t.Errorf("withdrawn pupil received charges: %+v", got)
	}
	if len(f.repo.charges) != 0 {
		t.Errorf("ledger has %d charges for withdrawn pupil", len(f.repo.charges))
	}
}

// TestRunBillingAccruesPastAPageOfSettledCharges is the regression for the
// empty-page stop in the run's overdue walk.
//
// The repository drops settled charges AFTER it fetches a page, so a page of
// 500 charges families already paid comes back empty while the walk is not
// over. The run used to stop there, and every debt ordered after those 500
// earned no surcharge: the same silent under-billing the keyset walk ended.
func TestRunBillingAccruesPastAPageOfSettledCharges(t *testing.T) {
	f := billingFixture(t)
	concept := f.seedConcept(t, "Colegiatura anterior", 100000)
	const settled = usecase.MaxPageSize + 1 // a whole page of them, and then some
	for i := 0; i < settled; i++ {
		// Due before the run's inscription (2026-08-10), so in (due_date, id)
		// order every one of them is examined first.
		c, err := f.svc.CreateCharge(context.Background(), seedChargeInput(concept.ID(), i, "2026-08-01"))
		requireNoError(t, err)
		if _, _, err := f.svc.RegisterPayment(context.Background(), c.Charge.ID(), 100000, domain.MethodCash, "", ""); err != nil {
			t.Fatalf("settle %d: %v", i, err)
		}
	}

	// The inscription, still owed and overdue by 15 September, is the one
	// charge that earns a surcharge (see TestRunBillingAccruesBeyondTheListCap).
	got := runOn(t, f, "2026-09-15")

	if got.LateFeesCreated != 1 {
		t.Fatalf("late fees = %d, want 1 — the run stopped on a page of settled charges and skipped the debt behind it",
			got.LateFeesCreated)
	}
}
