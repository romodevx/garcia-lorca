package usecase_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// 16 Oct 2026, 01:00 UTC: 19:00 on the 15th in Morelia — the school's today is
// the 15th, not the UTC 16th.
var schoolZone = mustZone("America/Mexico_City")

func mustZone(name string) *time.Location {
	zone, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return zone
}

var eveningOf15Oct = time.Date(2026, time.October, 16, 1, 0, 0, 0, time.UTC)

func calDay(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

type entryFixture struct {
	svc     *usecase.EnrollmentService
	enrolls *fakeEnrollRepo
	pub     *fakePub
}

// newEntryFixture is a school whose cycle opened on 24 August 2026, with group
// grp-b ("1° B") and grade gl-1 holding it.
func newEntryFixture(t *testing.T) entryFixture {
	t.Helper()
	students := newRepo()
	enrolls := newEnrollRepo()
	catalog := newCatalog()
	pub := &fakePub{}
	seedStudent(t, students, "stu-1")
	catalog.cycles["cyc-26"] = ports.CycleRef{ID: "cyc-26", StartsOn: calDay(2026, time.August, 24)}
	catalog.groups["grp-b"] = groupRef{ID: "grp-b", CycleID: "cyc-26", Name: "1° B", GradeLevelID: "gl-1", Capacity: 25}
	catalog.gradeGroups["gl-1"] = []ports.GroupSeats{{ID: "grp-b", Name: "1° B", CycleID: "cyc-26", Capacity: 25}}
	svc := usecase.NewEnrollmentService(students, enrolls, catalog, pub, tenant,
		func() string { return "enr-new" }, func() time.Time { return eveningOf15Oct }).WithSchoolZone(schoolZone)
	return entryFixture{svc: svc, enrolls: enrolls, pub: pub}
}

type enrolFn func(f entryFixture, on time.Time) error

var enrolPaths = map[string]enrolFn{
	"explicit group": func(f entryFixture, on time.Time) error {
		_, err := f.svc.Enroll(context.Background(), "stu-1", "grp-b", "", on)
		return err
	},
	"by grade": func(f entryFixture, on time.Time) error {
		_, err := f.svc.EnrollIntoGrade(context.Background(), "stu-1", "gl-1", on)
		return err
	},
}

// T9: a chosen entry day is stored as 00:00 of that day in Morelia and travels
// on StudentEnrolled; no day keeps the alta's instant.
func TestEnrolmentStoresTheEntryDayAtMoreliaMidnight(t *testing.T) {
	for name, enrol := range enrolPaths {
		t.Run(name, func(t *testing.T) {
			for _, tc := range []struct {
				name string
				on   time.Time
				want time.Time
			}{
				{"the cycle's first day", calDay(2026, time.August, 24), time.Date(2026, time.August, 24, 0, 0, 0, 0, schoolZone)},
				{"the school's today", calDay(2026, time.October, 15), time.Date(2026, time.October, 15, 0, 0, 0, 0, schoolZone)},
				{"no day is the alta's instant", time.Time{}, eveningOf15Oct},
			} {
				t.Run(tc.name, func(t *testing.T) {
					f := newEntryFixture(t)
					if err := enrol(f, tc.on); err != nil {
						t.Fatalf("enrol: %v", err)
					}
					if len(f.enrolls.createSeen) != 1 {
						t.Fatalf("persisted %d enrolments", len(f.enrolls.createSeen))
					}
					if got := f.enrolls.createSeen[0].EnrolledAt(); !got.Equal(tc.want) {
						t.Errorf("enrolled_at = %s, want %s", got, tc.want)
					}
					if len(f.pub.enrolled) != 1 || !f.pub.enrolled[0].EnrolledAt.Equal(tc.want) {
						t.Errorf("StudentEnrolled = %+v, want enrolled_at %s", f.pub.enrolled, tc.want)
					}
				})
			}
		})
	}
}

func TestEnrolmentRefusesAnEntryDayOutsideTheCycleAndToday(t *testing.T) {
	for name, enrol := range enrolPaths {
		for _, on := range []time.Time{
			calDay(2026, time.August, 23),  // the day before the cycle
			calDay(2026, time.October, 16), // tomorrow in Morelia, today in UTC
		} {
			t.Run(name+"/"+on.Format("2006-01-02"), func(t *testing.T) {
				f := newEntryFixture(t)
				err := enrol(f, on)
				if glerrors.KindOf(err) != glerrors.KindInvalid {
					t.Fatalf("err = %v, want KindInvalid", err)
				}
				if want := "La fecha de ingreso debe estar entre el inicio del ciclo (24 ago 2026) y hoy."; err.Error() != want {
					t.Errorf("message = %q, want %q", err.Error(), want)
				}
				if len(f.enrolls.createSeen) != 0 || len(f.pub.enrolled) != 0 {
					t.Errorf("a refused day was persisted or published")
				}
			})
		}
	}
}
