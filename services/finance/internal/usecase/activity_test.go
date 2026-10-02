package usecase_test

import (
	"context"
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

const otherStudent = "stu-2"

// activityFixture is billingFixture (a pupil enrolled in the cycle, a monthly
// tuition plan with a $150 monthly surcharge) plus activities.
type activityFixture struct {
	*fixture
	activities *fakeActivityRepo
	svcA       *usecase.ActivityService
}

func newActivityFixture(t *testing.T) *activityFixture {
	t.Helper()
	f := billingFixture(t)
	f.students.byID[otherStudent] = ports.StudentRef{ID: otherStudent, Status: "active"}
	repo := newFakeActivityRepo()
	f.svc.WithActivities(repo)
	svcA := usecase.NewActivityService(repo, f.students, f.svc, seqID("a"), func() time.Time { return f.clock })
	return &activityFixture{fixture: f, activities: repo, svcA: svcA}
}

// swimming: Tuesdays and Thursdays, monthly from 1 Sept, due on the 10th,
// $400 one day / $700 two days / $900 flat, $50 late fee.
func swimmingInput() usecase.ActivityInput {
	return usecase.ActivityInput{
		Name: "Natación",
		Schedule: []domain.ActivitySlot{
			{Weekday: 2, StartMin: 15 * 60, EndMin: 16 * 60},
			{Weekday: 4, StartMin: 16 * 60, EndMin: 17*60 + 30},
		},
		Periodicity:     domain.PeriodicityMonthly,
		BillingStartsOn: day(2026, 9, 1),
		FirstDueOn:      day(2026, 9, 10),
		LateFeeCents:    5_000,
		Active:          true,
		Options: []domain.PriceOption{
			{DaysPerWeek: 1, AmountCents: 40_000},
			{DaysPerWeek: 2, AmountCents: 70_000},
			{DaysPerWeek: 0, AmountCents: 90_000},
		},
	}
}

func (f *activityFixture) create(t *testing.T, in usecase.ActivityInput) domain.Activity {
	t.Helper()
	a, err := f.svcA.CreateActivity(context.Background(), in)
	if err != nil {
		t.Fatalf("create activity: %v", err)
	}
	return a
}

func optionFor(t *testing.T, a domain.Activity, days int) string {
	t.Helper()
	for _, o := range a.Options() {
		if o.DaysPerWeek == days {
			return o.ID
		}
	}
	t.Fatalf("activity has no %d-day option", days)
	return ""
}

func (f *activityFixture) enroll(t *testing.T, a domain.Activity, startsOn time.Time, items ...usecase.EnrollItem) usecase.EnrollResult {
	t.Helper()
	got, err := f.svcA.Enroll(context.Background(), a.ID(), startsOn, items)
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	return got
}

// activityCharges are the student's charges issued for activities, by ref.
func (f *activityFixture) activityCharges(t *testing.T, studentID string) map[string]domain.Charge {
	t.Helper()
	rows, err := f.repo.ListCharges(context.Background(), studentID, "")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]domain.Charge{}
	for _, c := range rows {
		if domain.IsActivityRef(c.Charge.ExternalRef()) {
			out[c.Charge.ExternalRef()] = c.Charge
		}
	}
	return out
}

func TestCreateActivityMintsOptionIDsAndRefusesANameUsedWithOtherAccents(t *testing.T) {
	f := newActivityFixture(t)
	a := f.create(t, swimmingInput())
	for _, o := range a.Options() {
		if o.ID == "" {
			t.Fatalf("option without id: %+v", o)
		}
	}
	dup := swimmingInput()
	dup.Name = "NATACION"
	_, err := f.svcA.CreateActivity(context.Background(), dup)
	requireKind(t, err, glerrors.KindConflict)
}

func TestUpdateActivityKeepsOptionIDsAndRefusesAForeignOne(t *testing.T) {
	f := newActivityFixture(t)
	a := f.create(t, swimmingInput())
	in := swimmingInput()
	in.Options = a.Options()
	in.Options[0].AmountCents = 45_000
	got, err := f.svcA.UpdateActivity(context.Background(), a.ID(), in)
	requireNoError(t, err)
	if o, ok := got.Option(a.Options()[0].ID); !ok || o.AmountCents != 45_000 {
		t.Fatalf("option not re-priced in place: %+v %v", o, ok)
	}
	in.Options[0].ID = "not-mine"
	_, err = f.svcA.UpdateActivity(context.Background(), a.ID(), in)
	requireKind(t, err, glerrors.KindInvalid)
}

func TestRemovingAnOptionAnEnrolledPupilUsesIsAConflict(t *testing.T) {
	f := newActivityFixture(t)
	a := f.create(t, swimmingInput())
	f.enroll(t, a, day(2026, 9, 1), usecase.EnrollItem{StudentID: testStudent, OptionID: optionFor(t, a, 1)})
	in := swimmingInput()
	for _, o := range a.Options() {
		if o.DaysPerWeek != 1 {
			in.Options = append(in.Options[:0:0], o)
		}
	}
	_, err := f.svcA.UpdateActivity(context.Background(), a.ID(), in)
	requireKind(t, err, glerrors.KindConflict)
}

func TestEnrollValidatesTheBatch(t *testing.T) {
	f := newActivityFixture(t)
	a := f.create(t, swimmingInput())
	ctx := context.Background()

	_, err := f.svcA.Enroll(ctx, a.ID(), time.Time{}, nil)
	requireKind(t, err, glerrors.KindInvalid)

	_, err = f.svcA.Enroll(ctx, a.ID(), time.Time{}, []usecase.EnrollItem{{StudentID: testStudent, OptionID: "nope"}})
	requireKind(t, err, glerrors.KindInvalid)

	_, err = f.svcA.Enroll(ctx, a.ID(), time.Time{}, []usecase.EnrollItem{{StudentID: "ghost", OptionID: optionFor(t, a, 1)}})
	requireKind(t, err, glerrors.KindInvalid)

	many := make([]usecase.EnrollItem, 201)
	for i := range many {
		many[i] = usecase.EnrollItem{StudentID: testStudent, OptionID: optionFor(t, a, 1)}
	}
	_, err = f.svcA.Enroll(ctx, a.ID(), time.Time{}, many)
	requireKind(t, err, glerrors.KindInvalid)

	inactive := swimmingInput()
	inactive.Name, inactive.Active = "Ajedrez", false
	off := f.create(t, inactive)
	_, err = f.svcA.Enroll(ctx, off.ID(), time.Time{}, []usecase.EnrollItem{{StudentID: testStudent, OptionID: optionFor(t, off, 1)}})
	requireKind(t, err, glerrors.KindConflict)
}

// Enrolling charges the periods already billable at once — the office does not
// wait for tomorrow's run to collect — and a second enrolment of the same pupil
// changes nothing.
func TestEnrollChargesTheBillablePeriodsAtOnce(t *testing.T) {
	f := newActivityFixture(t) // today: 15 Sept 2026
	a := f.create(t, swimmingInput())

	got := f.enroll(t, a, day(2026, 9, 1), usecase.EnrollItem{StudentID: testStudent, OptionID: optionFor(t, a, 2)})
	if len(got.Enrollments) != 1 || got.ChargesCreated != 1 {
		t.Fatalf("enroll = %+v, want 1 enrolment and September charged", got)
	}
	ref := domain.ActivityChargeRef(a.ID(), testStudent, "2026-09")
	c, ok := f.activityCharges(t, testStudent)[ref]
	if !ok {
		t.Fatalf("no charge %s", ref)
	}
	if c.Amount().Int64() != 70_000 || c.CycleID() != testCycle {
		t.Errorf("charge amount/cycle = %d/%s", c.Amount().Int64(), c.CycleID())
	}
	if !c.StartsOn().Equal(day(2026, 9, 1)) || !c.DueDate().Equal(day(2026, 9, 10)) {
		t.Errorf("window = %s..%s, want 09-01..09-10", c.StartsOn(), c.DueDate())
	}
	if !strings.Contains(c.Description(), "Natación") || !strings.Contains(c.Description(), "septiembre 2026") {
		t.Errorf("description = %q", c.Description())
	}

	again := f.enroll(t, a, day(2026, 9, 1), usecase.EnrollItem{StudentID: testStudent, OptionID: optionFor(t, a, 1)})
	if len(again.Enrollments) != 0 || again.ChargesCreated != 0 {
		t.Fatalf("re-enrolling = %+v, want nothing", again)
	}
}

// The approved rule: joining after the period's due date starts billing with
// the next period. No proration.
func TestJoiningAfterTheDueDateStartsWithTheNextPeriod(t *testing.T) {
	f := newActivityFixture(t)
	a := f.create(t, swimmingInput())
	f.enroll(t, a, day(2026, 9, 11), usecase.EnrollItem{StudentID: testStudent, OptionID: optionFor(t, a, 1)})

	runOn(t, f.fixture, "2026-10-02")

	got := f.activityCharges(t, testStudent)
	if _, sept := got[domain.ActivityChargeRef(a.ID(), testStudent, "2026-09")]; sept {
		t.Errorf("September charged to a pupil who joined on the 11th")
	}
	if _, oct := got[domain.ActivityChargeRef(a.ID(), testStudent, "2026-10")]; !oct {
		t.Errorf("October not charged: %v", got)
	}
}

// The run issues each period on its billing date, once, and several activities
// add up on the same pupil.
func TestRunBillingIssuesActivityPeriodsIdempotentlyAndActivitiesAddUp(t *testing.T) {
	f := newActivityFixture(t)
	swim := f.create(t, swimmingInput())
	music := swimmingInput()
	music.Name = "Música"
	music.Options = []domain.PriceOption{{DaysPerWeek: 0, AmountCents: 30_000}}
	mus := f.create(t, music)
	f.enroll(t, swim, day(2026, 9, 1), usecase.EnrollItem{StudentID: testStudent, OptionID: optionFor(t, swim, 0)})
	f.enroll(t, mus, day(2026, 9, 1), usecase.EnrollItem{StudentID: testStudent, OptionID: optionFor(t, mus, 0)})

	first := runOn(t, f.fixture, "2026-11-01")
	if first.ActivitiesCreated != 4 { // Oct+Nov of each; September came with the enrolment
		t.Errorf("activities created = %d, want 4", first.ActivitiesCreated)
	}
	second := runOn(t, f.fixture, "2026-11-01")
	if second.ActivitiesCreated != 0 {
		t.Errorf("second run created %d, want 0 — idempotent", second.ActivitiesCreated)
	}
	var total int64
	for _, c := range f.activityCharges(t, testStudent) {
		total += c.Amount().Int64()
	}
	if want := int64(3 * (90_000 + 30_000)); total != want {
		t.Errorf("activities owed = %d, want %d", total, want)
	}
	// Not yet: December bills on 1 Dec.
	if _, dec := f.activityCharges(t, testStudent)[domain.ActivityChargeRef(swim.ID(), testStudent, "2026-12")]; dec {
		t.Errorf("December issued before its billing date")
	}
}

// Unenrolling stops the periods billed from that day on, and never touches what
// was already issued.
func TestUnenrollStopsFutureChargesOnly(t *testing.T) {
	f := newActivityFixture(t)
	a := f.create(t, swimmingInput())
	f.enroll(t, a, day(2026, 9, 1), usecase.EnrollItem{StudentID: testStudent, OptionID: optionFor(t, a, 1)})

	n, err := f.svcA.Unenroll(context.Background(), a.ID(), []string{testStudent, otherStudent}, time.Time{}) // today, 15 Sept
	requireNoError(t, err)
	if n != 1 {
		t.Fatalf("ended = %d, want 1 (the other pupil was not enrolled)", n)
	}
	runOn(t, f.fixture, "2026-12-01")
	got := f.activityCharges(t, testStudent)
	if len(got) != 1 {
		t.Fatalf("charges after unenrolling = %v, want only September", got)
	}
	if c := got[domain.ActivityChargeRef(a.ID(), testStudent, "2026-09")]; c.Status() == domain.StatusCancelled {
		t.Errorf("the issued charge was touched")
	}
	listed, err := f.svcA.ListEnrollments(context.Background(), a.ID())
	requireNoError(t, err)
	if len(listed) != 0 {
		t.Errorf("an ended enrolment is still listed: %v", listed)
	}
}

// A pupil enrolled in the activity but not placed in the cycle being billed is
// not charged by that cycle's run.
func TestRunBillingSkipsPupilsNotInTheCycle(t *testing.T) {
	f := newActivityFixture(t)
	a := f.create(t, swimmingInput())
	got := f.enroll(t, a, day(2026, 9, 1), usecase.EnrollItem{StudentID: otherStudent, OptionID: optionFor(t, a, 1)})
	if got.ChargesCreated != 0 {
		t.Fatalf("charged a pupil with no cycle: %+v", got)
	}
	runOn(t, f.fixture, "2026-11-01")
	if c := f.activityCharges(t, otherStudent); len(c) != 0 {
		t.Fatalf("charged a pupil outside the cycle: %v", c)
	}
}

func TestInactiveActivitiesAreNotBilled(t *testing.T) {
	f := newActivityFixture(t)
	a := f.create(t, swimmingInput())
	f.enroll(t, a, day(2026, 9, 1), usecase.EnrollItem{StudentID: testStudent, OptionID: optionFor(t, a, 1)})
	in := swimmingInput()
	in.Options, in.Active = a.Options(), false
	_, err := f.svcA.UpdateActivity(context.Background(), a.ID(), in)
	requireNoError(t, err)

	if got := runOn(t, f.fixture, "2026-11-01"); got.ActivitiesCreated != 0 {
		t.Fatalf("an inactive activity was billed: %d", got.ActivitiesCreated)
	}
}

func TestChangeOptionAppliesToPeriodsBilledAfterwards(t *testing.T) {
	f := newActivityFixture(t)
	a := f.create(t, swimmingInput())
	f.enroll(t, a, day(2026, 9, 1), usecase.EnrollItem{StudentID: testStudent, OptionID: optionFor(t, a, 1)})

	_, err := f.svcA.ChangeOption(context.Background(), a.ID(), testStudent, "nope")
	requireKind(t, err, glerrors.KindInvalid)
	moved, err := f.svcA.ChangeOption(context.Background(), a.ID(), testStudent, optionFor(t, a, 2))
	requireNoError(t, err)
	if moved.OptionID() != optionFor(t, a, 2) {
		t.Fatalf("option = %s", moved.OptionID())
	}
	runOn(t, f.fixture, "2026-10-01")
	got := f.activityCharges(t, testStudent)
	if got[domain.ActivityChargeRef(a.ID(), testStudent, "2026-09")].Amount().Int64() != 40_000 {
		t.Errorf("September re-priced")
	}
	if got[domain.ActivityChargeRef(a.ID(), testStudent, "2026-10")].Amount().Int64() != 70_000 {
		t.Errorf("October not at the new option")
	}
}

// The activity's own late fee — fixed, once — and never the tuition table's
// monthly surcharge.
func TestAnOverdueActivityPeriodEarnsItsFixedLateFeeOnce(t *testing.T) {
	f := newActivityFixture(t)
	a := f.create(t, swimmingInput())
	f.enroll(t, a, day(2026, 9, 1), usecase.EnrollItem{StudentID: testStudent, OptionID: optionFor(t, a, 1)})

	runOn(t, f.fixture, "2026-11-20")
	runOn(t, f.fixture, "2026-11-21")

	sept := f.activityCharges(t, testStudent)[domain.ActivityChargeRef(a.ID(), testStudent, "2026-09")]
	rows, err := f.repo.ListCharges(context.Background(), testStudent, "")
	requireNoError(t, err)
	var fees []domain.Charge
	for _, c := range rows {
		if strings.HasPrefix(c.Charge.ExternalRef(), domain.LateFeeRefPrefix+sept.ID()+":") {
			fees = append(fees, c.Charge)
		}
	}
	if len(fees) != 1 || fees[0].Amount().Int64() != 5_000 {
		t.Fatalf("late fees on September's activity = %+v, want one of $50", fees)
	}
}

func TestAnActivityWithoutLateFeeEarnsNone(t *testing.T) {
	f := newActivityFixture(t)
	in := swimmingInput()
	in.LateFeeCents = 0
	a := f.create(t, in)
	f.enroll(t, a, day(2026, 9, 1), usecase.EnrollItem{StudentID: testStudent, OptionID: optionFor(t, a, 1)})

	runOn(t, f.fixture, "2026-11-20")

	sept := f.activityCharges(t, testStudent)[domain.ActivityChargeRef(a.ID(), testStudent, "2026-09")]
	rows, err := f.repo.ListCharges(context.Background(), testStudent, "")
	requireNoError(t, err)
	for _, c := range rows {
		if strings.Contains(c.Charge.ExternalRef(), sept.ID()) {
			t.Fatalf("a surcharge on an activity with no late fee: %s", c.Charge.ExternalRef())
		}
	}
}

// Under the tuition table's DAILY rule an activity period accrues nothing by
// the day: its only surcharge is its own fixed fee.
func TestTheDailyTuitionRuleDoesNotAccrueOnActivities(t *testing.T) {
	f := newActivityFixture(t)
	if _, err := f.svc.UpsertTuitionPlan(context.Background(), usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel, EnrollmentCents: 50000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, Active: true,
		Policy: domain.CollectionPolicy{DailyMode: true, LateFeeDailyCents: 1000, SurchargeCountsAllDays: true},
	}); err != nil {
		t.Fatal(err)
	}
	in := swimmingInput()
	in.LateFeeCents = 0
	a := f.create(t, in)
	f.enroll(t, a, day(2026, 9, 1), usecase.EnrollItem{StudentID: testStudent, OptionID: optionFor(t, a, 1)})

	got, err := f.svc.ExpectedToday(context.Background(), testStudent, day(2026, 9, 20))
	requireNoError(t, err)
	sept := f.activityCharges(t, testStudent)[domain.ActivityChargeRef(a.ID(), testStudent, "2026-09")]
	for _, l := range got.Lines {
		if l.ChargeID == sept.ID() && l.SurchargeCents != 0 {
			t.Fatalf("the activity accrued the daily tuition surcharge: %+v", l)
		}
	}
}

// The counter's deposit, under the daily rule, writes no daily surcharge on an
// activity period: what it collects is the period itself.
func TestADepositUnderTheDailyRuleWritesNoSurchargeOnAnActivity(t *testing.T) {
	f := newActivityFixture(t)
	if _, err := f.svc.UpsertTuitionPlan(context.Background(), usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel, EnrollmentCents: 50000, MonthlyCents: 320000,
		Months: 10, FirstBilledMonth: 9, DueDay: 10, Active: true,
		Policy: domain.CollectionPolicy{DailyMode: true, LateFeeDailyCents: 1000, SurchargeCountsAllDays: true},
	}); err != nil {
		t.Fatal(err)
	}
	in := swimmingInput()
	in.LateFeeCents = 0
	a := f.create(t, in)
	f.enroll(t, a, day(2026, 9, 1), usecase.EnrollItem{StudentID: testStudent, OptionID: optionFor(t, a, 1)})
	sept := f.activityCharges(t, testStudent)[domain.ActivityChargeRef(a.ID(), testStudent, "2026-09")]

	got, err := f.svc.PreviewDeposit(context.Background(), usecase.DepositInput{
		StudentID: testStudent, AmountCents: 40_000, Method: domain.MethodCash,
		ChargeIDs: []string{sept.ID()},
	})
	requireNoError(t, err)
	if len(got.Surcharges) != 0 {
		t.Fatalf("surcharges written on an activity: %+v", got.Surcharges)
	}
	if got.UnallocatedCents != 0 {
		t.Fatalf("the $400 did not settle the activity period: unallocated %d", got.UnallocatedCents)
	}
}
