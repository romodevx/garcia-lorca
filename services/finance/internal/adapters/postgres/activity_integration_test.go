//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

// T17 against real SQL: activities, their options and enrolments, under RLS.
// Every id here starts with "ac" so no other test of the shared container
// collides with them.

const (
	actSwim     = "ac000000-0000-0000-0000-000000000001"
	actOpt1     = "ac000000-0000-0000-0000-0000000000a1"
	actOpt2     = "ac000000-0000-0000-0000-0000000000a2"
	actOptFlat  = "ac000000-0000-0000-0000-0000000000af"
	actStudent1 = "ac000000-0000-0000-0000-00000000005a"
	actStudent2 = "ac000000-0000-0000-0000-00000000005b"
)

var actDay = time.Date(2031, 9, 1, 0, 0, 0, 0, time.UTC)

func swimSpec(id, name string, options ...domain.PriceOption) domain.ActivitySpec {
	if len(options) == 0 {
		options = []domain.PriceOption{
			{ID: actOpt1, DaysPerWeek: 1, AmountCents: 40_000},
			{ID: actOpt2, DaysPerWeek: 2, AmountCents: 70_000},
		}
	}
	return domain.ActivitySpec{
		ID: id, Name: name,
		Schedule: []domain.ActivitySlot{
			{Weekday: 2, StartMin: 15 * 60, EndMin: 16 * 60},
			{Weekday: 4, StartMin: 16 * 60, EndMin: 17*60 + 30},
		},
		Periodicity: domain.PeriodicityMonthly, BillingStartsOn: actDay, FirstDueOn: actDay.AddDate(0, 0, 9),
		LateFeeCents: 5_000, Active: true, Options: options,
	}
}

func mustActivityOf(t *testing.T, spec domain.ActivitySpec) domain.Activity {
	t.Helper()
	a, err := domain.NewActivity(spec)
	if err != nil {
		t.Fatalf("NewActivity: %v", err)
	}
	return a
}

func actEnrollment(t *testing.T, id, activityID, studentID, optionID string, startsOn time.Time) domain.ActivityEnrollment {
	t.Helper()
	e, err := domain.NewActivityEnrollment(id, activityID, studentID, optionID, startsOn)
	if err != nil {
		t.Fatalf("NewActivityEnrollment: %v", err)
	}
	return e
}

func TestActivitiesRoundTripAndUpdateTwice(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	created, err := repo.CreateActivity(ctx, mustActivityOf(t, swimSpec(actSwim, "Natación")))
	if err != nil {
		t.Fatalf("CreateActivity: %v", err)
	}
	got, err := repo.GetActivity(ctx, actSwim)
	if err != nil {
		t.Fatalf("GetActivity: %v", err)
	}
	if got.Name() != "Natación" || len(got.Schedule()) != 2 || got.Schedule()[1].EndMin != 17*60+30 ||
		!got.BillingStartsOn().Equal(actDay) || !got.FirstDueOn().Equal(actDay.AddDate(0, 0, 9)) ||
		got.LateFee().Int64() != 5_000 || got.Periodicity() != domain.PeriodicityMonthly || len(got.Options()) != 2 {
		t.Fatalf("round trip lost fields: %+v (created %+v)", got, created)
	}

	// Edit twice on the same key (checklist #1): re-price one option, add the
	// flat fee, retire the 2-day option, rename, change every field.
	spec := swimSpec(actSwim, "Natación libre",
		domain.PriceOption{ID: actOpt1, DaysPerWeek: 1, AmountCents: 45_000},
		domain.PriceOption{ID: actOptFlat, DaysPerWeek: 0, AmountCents: 90_000})
	spec.Periodicity, spec.LateFeeCents, spec.Active = domain.PeriodicityWeekly, 0, false
	spec.FirstDueOn = actDay.AddDate(0, 0, 2)
	spec.Schedule = []domain.ActivitySlot{{Weekday: 6, StartMin: 9 * 60, EndMin: 11 * 60}}
	for i := 0; i < 2; i++ {
		if _, err := repo.UpdateActivity(ctx, mustActivityOf(t, spec)); err != nil {
			t.Fatalf("UpdateActivity #%d: %v", i+1, err)
		}
	}
	got, err = repo.GetActivity(ctx, actSwim)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name() != "Natación libre" || got.Periodicity() != domain.PeriodicityWeekly || got.LateFee().Int64() != 0 ||
		got.Active() || len(got.Schedule()) != 1 || got.Schedule()[0].Weekday != 6 || !got.FirstDueOn().Equal(actDay.AddDate(0, 0, 2)) {
		t.Fatalf("update lost fields: %+v", got)
	}
	if o, ok := got.Option(actOpt1); !ok || o.AmountCents != 45_000 {
		t.Fatalf("option not re-priced: %+v", o)
	}
	if o, ok := got.Option(actOptFlat); !ok || !o.Flat() {
		t.Fatalf("flat option missing: %+v", got.Options())
	}
	if _, ok := got.Option(actOpt2); ok {
		t.Fatalf("the retired option is still live")
	}
	// Retired, then brought back with the same days: the unique index is on
	// LIVE options only.
	spec.Options = append(spec.Options, domain.PriceOption{ID: "ac000000-0000-0000-0000-0000000000a3", DaysPerWeek: 2, AmountCents: 1})
	if _, err := repo.UpdateActivity(ctx, mustActivityOf(t, spec)); err != nil {
		t.Fatalf("re-adding a 2-day option: %v", err)
	}
}

func TestActivityNamesAreUniquePerSchool(t *testing.T) {
	ctx := context.Background()
	const id1, id2, id3 = "ac000000-0000-0000-0000-000000000011", "ac000000-0000-0000-0000-000000000012", "ac000000-0000-0000-0000-000000000013"
	opt := func(id string) domain.PriceOption {
		return domain.PriceOption{ID: id, DaysPerWeek: 0, AmountCents: 100}
	}
	if _, err := appRepo(t, tenantA).CreateActivity(ctx, mustActivityOf(t, swimSpec(id1, "Ajedrez", opt("ac000000-0000-0000-0000-0000000001a1")))); err != nil {
		t.Fatal(err)
	}
	_, err := appRepo(t, tenantA).CreateActivity(ctx, mustActivityOf(t, swimSpec(id2, " AJEDREZ", opt("ac000000-0000-0000-0000-0000000001a2"))))
	if glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("same name: want KindConflict, got %v", err)
	}
	if _, err := appRepo(t, tenantB).CreateActivity(ctx, mustActivityOf(t, swimSpec(id3, "Ajedrez", opt("ac000000-0000-0000-0000-0000000001a3")))); err != nil {
		t.Fatalf("another school may use the name: %v", err)
	}
}

func TestActivityEnrollmentsLifecycleAndRLS(t *testing.T) {
	ctx := context.Background()
	const actID = "ac000000-0000-0000-0000-000000000021"
	const o1, o2 = "ac000000-0000-0000-0000-0000000002a1", "ac000000-0000-0000-0000-0000000002a2"
	repo := appRepo(t, tenantA)
	if _, err := repo.CreateActivity(ctx, mustActivityOf(t, swimSpec(actID, "Robótica",
		domain.PriceOption{ID: o1, DaysPerWeek: 1, AmountCents: 1000},
		domain.PriceOption{ID: o2, DaysPerWeek: 2, AmountCents: 2000}))); err != nil {
		t.Fatal(err)
	}
	created, err := repo.CreateEnrollments(ctx, []domain.ActivityEnrollment{
		actEnrollment(t, "ac000000-0000-0000-0000-0000000002e1", actID, actStudent1, o1, actDay),
		actEnrollment(t, "ac000000-0000-0000-0000-0000000002e2", actID, actStudent2, o2, actDay),
	})
	if err != nil || len(created) != 2 {
		t.Fatalf("CreateEnrollments = %d, %v", len(created), err)
	}
	// The same pupil again is skipped, not duplicated.
	again, err := repo.CreateEnrollments(ctx, []domain.ActivityEnrollment{
		actEnrollment(t, "ac000000-0000-0000-0000-0000000002e3", actID, actStudent1, o2, actDay),
	})
	if err != nil || len(again) != 0 {
		t.Fatalf("re-enrolling = %d, %v; want 0, nil", len(again), err)
	}
	// An option of ANOTHER activity is refused by the composite FK.
	if _, err := repo.CreateEnrollments(ctx, []domain.ActivityEnrollment{
		actEnrollment(t, "ac000000-0000-0000-0000-0000000002e4", actID, "ac000000-0000-0000-0000-00000000005c", actOpt1, actDay),
	}); err == nil {
		t.Fatalf("an option of another activity was accepted")
	}

	// Retiring an option in use is a conflict.
	if _, err := repo.UpdateActivity(ctx, mustActivityOf(t, swimSpec(actID, "Robótica",
		domain.PriceOption{ID: o1, DaysPerWeek: 1, AmountCents: 1000}))); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("retiring a used option: want KindConflict, got %v", err)
	}

	moved, err := repo.ChangeEnrollmentOption(ctx, actID, actStudent1, o2, actDay)
	if err != nil || moved.OptionID() != o2 {
		t.Fatalf("ChangeEnrollmentOption = %+v, %v", moved, err)
	}
	counts, err := repo.CountActiveEnrollments(ctx, actDay)
	if err != nil || counts[actID] != 2 {
		t.Fatalf("counts = %v, %v", counts, err)
	}

	end := actDay.AddDate(0, 1, 0)
	n, err := repo.EndEnrollments(ctx, actID, []string{actStudent1, "ac000000-0000-0000-0000-00000000005f"}, end)
	if err != nil || n != 1 {
		t.Fatalf("EndEnrollments = %d, %v", n, err)
	}
	all, err := repo.ListActivityEnrollments(ctx, actID)
	if err != nil || len(all) != 2 {
		t.Fatalf("ListActivityEnrollments = %d, %v", len(all), err)
	}
	for _, e := range all {
		if e.StudentID() == actStudent1 && !e.EndsOn().Equal(end) {
			t.Fatalf("ends_on = %v, want %v", e.EndsOn(), end)
		}
	}
	// Ended: the pupil may be enrolled again (the open index is on ends_on NULL).
	if again, err := repo.CreateEnrollments(ctx, []domain.ActivityEnrollment{
		actEnrollment(t, "ac000000-0000-0000-0000-0000000002e5", actID, actStudent1, o1, end),
	}); err != nil || len(again) != 1 {
		t.Fatalf("re-enrolling after the end = %d, %v", len(again), err)
	}
	if _, err := repo.ChangeEnrollmentOption(ctx, actID, "ac000000-0000-0000-0000-00000000005e", o1, actDay); !isNotFound(err) {
		t.Fatalf("changing a pupil not enrolled: want NotFound, got %v", err)
	}

	billable, err := repo.ListBillableEnrollments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, e := range billable {
		if e.ActivityID() == actID {
			found++
		}
	}
	if found != 3 {
		t.Fatalf("billable enrolments of the activity = %d, want 3 (ended ones too)", found)
	}

	// RLS: another school sees none of it.
	other := appRepo(t, tenantB)
	if _, err := other.GetActivity(ctx, actID); !isNotFound(err) {
		t.Fatalf("tenant B read tenant A's activity: %v", err)
	}
	if rows, err := other.ListActivityEnrollments(ctx, actID); err != nil || len(rows) != 0 {
		t.Fatalf("tenant B listed tenant A's enrolments: %d, %v", len(rows), err)
	}
	if n, err := other.EndEnrollments(ctx, actID, []string{actStudent2}, end); err != nil || n != 0 {
		t.Fatalf("tenant B ended tenant A's enrolment: %d, %v", n, err)
	}
}

// The system concept activity charges hang from is accepted by the kind CHECK,
// and an activity charge is deduplicated by its ref.
func TestActivityChargesUseTheActivityKindAndTheirRef(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	concept, err := repo.EnsureConceptOfKind(ctx, "ac000000-0000-0000-0000-0000000000c1", domain.KindActivity, "Actividad")
	if err != nil || concept.Kind() != domain.KindActivity {
		t.Fatalf("EnsureConceptOfKind(activity) = %+v, %v", concept, err)
	}
	ref := domain.ActivityChargeRef(actSwim, actStudent1, "2031-09")
	mk := func(id string) domain.Charge {
		c := newCharge(t, id, tenantA, actStudent1, "ac000000-0000-0000-0000-0000000000cc", concept.ID(), 40_000, 0, actDay.AddDate(0, 0, 9))
		c, err := c.WithExternalRef(ref)
		if err != nil {
			t.Fatal(err)
		}
		c, err = c.WithStartsOn(actDay)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	for i, id := range []string{"ac000000-0000-0000-0000-0000000000d1", "ac000000-0000-0000-0000-0000000000d2"} {
		n, err := repo.CreateChargesBulk(ctx, []domain.Charge{mk(id)})
		if err != nil || n != 1-i {
			t.Fatalf("run %d created %d, %v; want %d", i+1, n, err, 1-i)
		}
	}
}

// Review: swapping the days of two options in one edit must not trip the
// live-days unique index, and the resumed_on day survives the round trip.
func TestActivityOptionsSwapDaysAndResumedOnRoundTrips(t *testing.T) {
	ctx := context.Background()
	const id = "ac000000-0000-0000-0000-000000000031"
	const o2, o3 = "ac000000-0000-0000-0000-0000000003a2", "ac000000-0000-0000-0000-0000000003a3"
	repo := appRepo(t, tenantA)
	spec := swimSpec(id, "Teatro",
		domain.PriceOption{ID: o2, DaysPerWeek: 2, AmountCents: 200},
		domain.PriceOption{ID: o3, DaysPerWeek: 3, AmountCents: 300})
	if _, err := repo.CreateActivity(ctx, mustActivityOf(t, spec)); err != nil {
		t.Fatal(err)
	}
	spec.Options = []domain.PriceOption{{ID: o2, DaysPerWeek: 3, AmountCents: 300}, {ID: o3, DaysPerWeek: 2, AmountCents: 200}}
	spec.ResumedOn = actDay.AddDate(0, 2, 0)
	if _, err := repo.UpdateActivity(ctx, mustActivityOf(t, spec)); err != nil {
		t.Fatalf("swapping days: %v", err)
	}
	got, err := repo.GetActivity(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if o, _ := got.Option(o2); o.DaysPerWeek != 3 {
		t.Fatalf("swap lost: %+v", got.Options())
	}
	if !got.ResumedOn().Equal(actDay.AddDate(0, 2, 0)) {
		t.Fatalf("resumed_on = %v", got.ResumedOn())
	}
}

// Review: an option still used by an enrolment that ends in the future (it
// still owes the periods before its end) cannot be retired.
func TestAnOptionUsedByAnEnrolmentEndingLaterCannotBeRetired(t *testing.T) {
	ctx := context.Background()
	const id = "ac000000-0000-0000-0000-000000000041"
	const o1, o2 = "ac000000-0000-0000-0000-0000000004a1", "ac000000-0000-0000-0000-0000000004a2"
	repo := appRepo(t, tenantA)
	spec := swimSpec(id, "Ballet",
		domain.PriceOption{ID: o1, DaysPerWeek: 1, AmountCents: 100},
		domain.PriceOption{ID: o2, DaysPerWeek: 2, AmountCents: 200})
	if _, err := repo.CreateActivity(ctx, mustActivityOf(t, spec)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateEnrollments(ctx, []domain.ActivityEnrollment{
		actEnrollment(t, "ac000000-0000-0000-0000-0000000004e1", id, actStudent1, o1, actDay),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.EndEnrollments(ctx, id, []string{actStudent1}, time.Now().AddDate(1, 0, 0)); err != nil {
		t.Fatal(err)
	}
	spec.Options = spec.Options[1:]
	if _, err := repo.UpdateActivity(ctx, mustActivityOf(t, spec)); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("retiring an option an enrolment still bills under: want KindConflict, got %v", err)
	}
}
