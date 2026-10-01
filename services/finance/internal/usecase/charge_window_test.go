package usecase_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// T5 — an inscription opens on a day and falls due on another, and the school
// can move both after the year has been billed.

const (
	partlyPaidStudent = "stu-2"
	paidStudent       = "stu-3"
	inscriptionCents  = 50000
)

func ymdUTC(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// windowPlan is the billing fixture's plan with an inscription window.
func windowPlan(starts, due time.Time) usecase.TuitionPlanInput {
	return usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel,
		EnrollmentCents: inscriptionCents, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, Active: true,
		EnrollmentStartsOn: starts, EnrollmentDueOn: due,
	}
}

// windowFixture is the billing school with three pupils, billed under an
// inscription that opens on 1 June and falls due on 30 June 2026.
func windowFixture(t *testing.T) *fixture {
	t.Helper()
	f := billingFixture(t)
	for _, id := range []string{partlyPaidStudent, paidStudent} {
		f.students.byID[id] = ports.StudentRef{ID: id, Status: "active"}
		f.enrollments.byCycle[testCycle] = append(f.enrollments.byCycle[testCycle], ports.EnrollmentRef{
			ID: "enr-" + id, StudentID: id, GroupID: testGroup, CycleID: testCycle, Status: "active",
		})
	}
	savePlan(t, f, windowPlan(ymdUTC(2026, time.June, 1), ymdUTC(2026, time.June, 30)))
	return f
}

func savePlan(t *testing.T, f *fixture, in usecase.TuitionPlanInput) {
	t.Helper()
	if _, err := f.svc.UpsertTuitionPlan(context.Background(), in); err != nil {
		t.Fatalf("UpsertTuitionPlan: %v", err)
	}
}

// inscriptionOf is the pupil's inscription charge.
func inscriptionOf(t *testing.T, f *fixture, student string) ports.ChargeWithPaid {
	t.Helper()
	charges, err := f.svc.ListCharges(context.Background(), student, testCycle)
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	enrollment := f.repo.conceptIDOfKind(domain.KindEnrollment)
	for _, c := range charges {
		if c.Charge.ConceptID() == enrollment {
			return c
		}
	}
	t.Fatalf("%s has no inscription", student)
	return ports.ChargeWithPaid{}
}

func pay(t *testing.T, f *fixture, chargeID string, cents int64) {
	t.Helper()
	if _, _, err := f.svc.RegisterPayment(context.Background(), chargeID, cents, domain.MethodCash, "", ""); err != nil {
		t.Fatalf("RegisterPayment: %v", err)
	}
}

func dateOf(t time.Time) string { return t.Format("2006-01-02") }

func TestTheInscriptionIsIssuedWithTheWindowTheSchoolSet(t *testing.T) {
	f := windowFixture(t)

	runOn(t, f, "2026-05-15")

	got := inscriptionOf(t, f, testStudent).Charge
	if dateOf(got.StartsOn()) != "2026-06-01" || dateOf(got.DueDate()) != "2026-06-30" {
		t.Fatalf("window = %s..%s, want 2026-06-01..2026-06-30", dateOf(got.StartsOn()), dateOf(got.DueDate()))
	}
}

func TestTheSchoolCannotSaveAnInscriptionThatOpensAfterItsDeadline(t *testing.T) {
	f := billingFixture(t)
	_, err := f.svc.UpsertTuitionPlan(context.Background(),
		windowPlan(ymdUTC(2026, time.July, 1), ymdUTC(2026, time.June, 30)))
	requireKind(t, err, glerrors.KindInvalid)
}

// Saving new dates reaches every inscription still owing — untouched AND part
// paid — at once, without waiting for the next run. A settled one is history.
func TestSavingNewDatesMovesWhatIsStillOwedAndNothingSettled(t *testing.T) {
	f := windowFixture(t)
	runOn(t, f, "2026-05-15")
	pay(t, f, inscriptionOf(t, f, partlyPaidStudent).Charge.ID(), 10000)
	pay(t, f, inscriptionOf(t, f, paidStudent).Charge.ID(), inscriptionCents)

	savePlan(t, f, windowPlan(ymdUTC(2026, time.July, 1), ymdUTC(2026, time.August, 31)))

	for _, student := range []string{testStudent, partlyPaidStudent} {
		got := inscriptionOf(t, f, student).Charge
		if dateOf(got.StartsOn()) != "2026-07-01" || dateOf(got.DueDate()) != "2026-08-31" {
			t.Errorf("%s: window = %s..%s, want 2026-07-01..2026-08-31",
				student, dateOf(got.StartsOn()), dateOf(got.DueDate()))
		}
	}
	settled := inscriptionOf(t, f, paidStudent).Charge
	if dateOf(settled.StartsOn()) != "2026-06-01" || dateOf(settled.DueDate()) != "2026-06-30" {
		t.Errorf("a paid inscription was re-dated to %s..%s", dateOf(settled.StartsOn()), dateOf(settled.DueDate()))
	}
}

// The run reports a start-only change as a move too: the date the family sees
// the charge is as much the school's decision as the deadline.
func TestTheRunMovesAChangedStartAlone(t *testing.T) {
	f := windowFixture(t)
	runOn(t, f, "2026-05-15")
	f.repo.plans[f.repo.planKey(testCycle, testLevel)] = mustPlan(t,
		windowPlan(ymdUTC(2026, time.June, 15), ymdUTC(2026, time.June, 30)))

	got := runOn(t, f, "2026-05-15")

	if got.EnrollmentMoved != 3 {
		t.Fatalf("moved = %d, want the 3 unpaid inscriptions", got.EnrollmentMoved)
	}
	if s := inscriptionOf(t, f, testStudent).Charge.StartsOn(); dateOf(s) != "2026-06-15" {
		t.Errorf("starts on %s, want 2026-06-15", dateOf(s))
	}
}

// Moving the deadline forward takes the pending surcharge away by itself: it is
// derived when read, never written down while unpaid.
func TestExtendingTheDeadlineRemovesThePendingSurcharge(t *testing.T) {
	f := windowFixture(t)
	daily := windowPlan(ymdUTC(2026, time.June, 1), ymdUTC(2026, time.June, 30))
	daily.LateFeeCents = 15000
	daily.Policy = domain.CollectionPolicy{DailyMode: true, LateFeeDailyCents: 1000, SurchargeCountsAllDays: true}
	savePlan(t, f, daily)
	runOn(t, f, "2026-05-15")
	f.clock = ymdUTC(2026, time.July, 10)

	before, err := f.svc.ExpectedToday(context.Background(), testStudent, time.Time{})
	requireNoError(t, err)
	if before.SurchargeCents <= 0 {
		t.Fatalf("surcharge = %d, want some: the inscription is ten days late", before.SurchargeCents)
	}

	daily.EnrollmentDueOn = ymdUTC(2026, time.July, 31)
	savePlan(t, f, daily)

	after, err := f.svc.ExpectedToday(context.Background(), testStudent, time.Time{})
	requireNoError(t, err)
	if after.SurchargeCents != 0 {
		t.Errorf("surcharge = %d after the deadline moved to 31 July, want 0", after.SurchargeCents)
	}
}

// A charge that has not opened is not shown to the family, nor counted in what
// the app says they owe — until its day comes.
func TestTheFamilyDoesNotSeeAChargeBeforeItOpens(t *testing.T) {
	f := windowFixture(t)
	runOn(t, f, "2026-05-15")
	inscription := inscriptionOf(t, f, testStudent).Charge.ID()

	f.clock = time.Date(2026, time.May, 31, 23, 0, 0, 0, time.UTC)
	early, err := f.svc.GetFamilyAccount(context.Background(), testStudent)
	requireNoError(t, err)
	if hasCharge(early, inscription) {
		t.Error("the family sees the inscription the day before it opens")
	}
	staff, err := f.svc.GetAccount(context.Background(), testStudent)
	requireNoError(t, err)
	if !hasCharge(staff, inscription) {
		t.Error("the school must always see the inscription")
	}
	if early.TotalBalanceCents != staff.TotalBalanceCents-inscriptionCents {
		t.Errorf("family balance = %d, want the school's %d less the hidden inscription",
			early.TotalBalanceCents, staff.TotalBalanceCents)
	}

	f.clock = ymdUTC(2026, time.June, 1)
	opened, err := f.svc.GetFamilyAccount(context.Background(), testStudent)
	requireNoError(t, err)
	if !hasCharge(opened, inscription) {
		t.Error("the family does not see the inscription on the day it opens")
	}
}

// Money already taken against a charge is part of the family's history: its
// charge is shown even before it opens, or the payment explains nothing.
func TestAChargeWithMoneyOnItIsShownBeforeItOpens(t *testing.T) {
	f := windowFixture(t)
	runOn(t, f, "2026-05-15")
	inscription := inscriptionOf(t, f, partlyPaidStudent).Charge.ID()
	f.clock = ymdUTC(2026, time.May, 20)
	pay(t, f, inscription, 10000)

	got, err := f.svc.GetFamilyAccount(context.Background(), partlyPaidStudent)
	requireNoError(t, err)
	if !hasCharge(got, inscription) {
		t.Error("a part-paid inscription vanished from the family's statement")
	}
}

// The counter lists a charge that has not opened, but does not ask for it.
func TestTheCounterDoesNotAskForAChargeBeforeItOpens(t *testing.T) {
	f := billingFixture(t)
	savePlan(t, f, windowPlan(ymdUTC(2026, time.June, 20), ymdUTC(2026, time.June, 30)))
	runOn(t, f, "2026-05-15")
	inscription := inscriptionOf(t, f, testStudent).Charge.ID()

	got, err := f.svc.ExpectedToday(context.Background(), testStudent, ymdUTC(2026, time.June, 10))
	requireNoError(t, err)
	for _, l := range got.Lines {
		if l.ChargeID == inscription && l.Due {
			t.Error("the inscription is asked for ten days before it opens")
		}
	}
	if got.AmountCents != 0 {
		t.Errorf("due today = %d before anything opened, want 0", got.AmountCents)
	}

	opened, err := f.svc.ExpectedToday(context.Background(), testStudent, ymdUTC(2026, time.June, 20))
	requireNoError(t, err)
	if opened.AmountCents != inscriptionCents {
		t.Errorf("due on the day it opens = %d, want %d", opened.AmountCents, inscriptionCents)
	}
}

func hasCharge(a usecase.AccountStatement, id string) bool {
	for _, c := range a.Charges {
		if c.Charge.ID() == id {
			return true
		}
	}
	return false
}

func mustPlan(t *testing.T, in usecase.TuitionPlanInput) domain.TuitionPlan {
	t.Helper()
	p, err := domain.NewTuitionPlan(domain.TuitionPlanSpec{
		ID: "plan-direct", TenantID: testTenant, CycleID: in.CycleID, LevelID: in.LevelID,
		EnrollmentCents: in.EnrollmentCents, MonthlyCents: in.MonthlyCents,
		Months: in.Months, FirstBilledMonth: in.FirstBilledMonth, DueDay: in.DueDay, Active: in.Active,
		EnrollmentStartsOn: in.EnrollmentStartsOn, EnrollmentDueOn: in.EnrollmentDueOn,
	})
	if err != nil {
		t.Fatalf("NewTuitionPlan: %v", err)
	}
	return p
}

// Saving and re-dating must not split: when what the re-dating needs cannot be
// read, nothing is saved — never "no se pudo guardar" over a plan that was.
func TestAPlanIsNotSavedWhenItsInscriptionsCannotBeRedated(t *testing.T) {
	f := windowFixture(t)
	f.enrollments.err = glerrors.New(glerrors.KindInternal, "students is down")

	_, err := f.svc.UpsertTuitionPlan(context.Background(),
		windowPlan(ymdUTC(2026, time.July, 1), ymdUTC(2026, time.August, 31)))
	if err == nil {
		t.Fatal("the save reported success without re-dating")
	}
	stored := f.repo.plans[f.repo.planKey(testCycle, testLevel)]
	if dateOf(stored.EnrollmentDueOn()) != "2026-06-30" {
		t.Errorf("the plan was stored (deadline %s) although the save failed", dateOf(stored.EnrollmentDueOn()))
	}
}
