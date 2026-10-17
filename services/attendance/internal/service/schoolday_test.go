package service_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

// fakeCalendar stands in for config's school calendar (T14): every date is a
// school day unless listed in closed (label "" = a weekend or outside the cycle).
type fakeCalendar struct {
	closed map[string]string
	err    error
	asked  []string // groupID|date of every question
}

func (f *fakeCalendar) SchoolDayOf(_ context.Context, groupID string, date time.Time) (service.SchoolDay, error) {
	key := date.Format(dateLayout)
	f.asked = append(f.asked, groupID+"|"+key)
	if f.err != nil {
		return service.SchoolDay{}, f.err
	}
	if label, ok := f.closed[key]; ok {
		return service.SchoolDay{Closed: label}, nil
	}
	return service.SchoolDay{Open: true}, nil
}

const (
	saturday = "2026-10-17"
	holiday  = "2026-11-16"
)

func calendarService(t *testing.T, repo *fakeRepo, pub service.Publisher, cal *fakeCalendar) *service.AttendanceService {
	t.Helper()
	return newServiceWith(t, repo, pub, slog.New(slog.DiscardHandler), nil, service.WithSchoolCalendar(cal))
}

func newCalendar() *fakeCalendar {
	return &fakeCalendar{closed: map[string]string{saturday: "", holiday: "Revolución mexicana"}}
}

// ISSUE-T14-101: a list could be saved on a Saturday or a holiday, which
// notified the family and counted as "lista tomada".
func TestSaveRefusesADayWithoutClasses(t *testing.T) {
	tests := []struct {
		name, date, want string
	}{
		{"a weekend", saturday, "Ese día no hay clases."},
		{"a closed day names it", holiday, "Ese día no hay clases (Revolución mexicana)."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			pub := &spyPublisher{}
			cal := newCalendar()
			svc := calendarService(t, repo, pub, cal)

			_, _, err := save(svc, context.Background(), groupID, tt.date, []service.Mark{{EnrollmentID: enrollmentAna, Status: "excused"}})

			wantInvalid(t, err)
			if got := glerrors.SafeMessage(err); got != tt.want {
				t.Errorf("message = %q, want %q", got, tt.want)
			}
			if got := service.CodeOf(err); got != service.CodeNoClasses {
				t.Errorf("code = %q, want %q", got, service.CodeNoClasses)
			}
			if repo.reconcileCalls != 0 || len(pub.marked)+len(pub.absences) != 0 {
				t.Errorf("a refused day wrote %d reconciles and published %d events", repo.reconcileCalls, len(pub.marked)+len(pub.absences))
			}
			if len(cal.asked) != 1 || cal.asked[0] != groupID+"|"+tt.date {
				t.Errorf("calendar asked %v, want the group's day once", cal.asked)
			}
		})
	}
}

func TestSaveOnASchoolDayStillWorks(t *testing.T) {
	repo := newFakeRepo()
	svc := calendarService(t, repo, nil, newCalendar())
	if _, _, err := save(svc, context.Background(), groupID, schoolDay, []service.Mark{{EnrollmentID: enrollmentAna, Status: "absent"}}); err != nil {
		t.Fatalf("SaveDailyAttendance on a school day: %v", err)
	}
	if repo.reconcileCalls != 1 {
		t.Errorf("reconcileCalls = %d, want 1", repo.reconcileCalls)
	}
}

// The group is validated first: an unknown group is "group not found", never
// a calendar question.
func TestSaveChecksTheGroupBeforeTheCalendar(t *testing.T) {
	cal := newCalendar()
	svc := calendarService(t, newFakeRepo(), nil, cal)
	_, _, err := save(svc, context.Background(), "ghost", saturday, nil)
	wantInvalid(t, err)
	if len(cal.asked) != 0 {
		t.Errorf("calendar asked %v before the group was validated", cal.asked)
	}
}

func TestSavePropagatesACalendarFailure(t *testing.T) {
	cal := newCalendar()
	cal.err = glerrors.Wrap(glerrors.KindInternal, "config school day lookup failed", errors.New("boom"))
	repo := newFakeRepo()
	svc := calendarService(t, repo, nil, cal)
	_, _, err := save(svc, context.Background(), groupID, schoolDay, nil)
	if glerrors.KindOf(err) != glerrors.KindInternal {
		t.Fatalf("KindOf(%v) = %v, want KindInternal", err, glerrors.KindOf(err))
	}
	if repo.reconcileCalls != 0 {
		t.Error("a calendar failure must not write")
	}
}

// The roster asks the calendar right after the group check: a calendar
// failure costs no repository read.
func TestRosterAsksTheCalendarBeforeTheRepository(t *testing.T) {
	cal := newCalendar()
	cal.err = glerrors.Wrap(glerrors.KindInternal, "config school day lookup failed", errors.New("boom"))
	repo := newFakeRepo()
	svc := calendarService(t, repo, nil, cal)
	_, err := svc.GetDailyRoster(context.Background(), groupID, schoolDay, "")
	if glerrors.KindOf(err) != glerrors.KindInternal {
		t.Fatalf("KindOf(%v) = %v, want KindInternal", err, glerrors.KindOf(err))
	}
	if repo.marksForDateCalls != 0 {
		t.Errorf("MarksForList called %d times before the calendar answered", repo.marksForDateCalls)
	}
}

// Reading stays open (old weekend lists must remain visible), but the roster
// says the day has no classes so the screen can lock it.
func TestRosterOfADayWithoutClassesIsReadableAndSaysSo(t *testing.T) {
	svc := calendarService(t, newFakeRepo(), nil, newCalendar())
	closed, err := svc.GetDailyRoster(context.Background(), groupID, saturday, "")
	if err != nil {
		t.Fatalf("GetDailyRoster on a Saturday: %v", err)
	}
	if !closed.NoClasses || len(closed.Rows) != 3 {
		t.Errorf("Saturday roster = NoClasses %v with %d rows, want true with 3", closed.NoClasses, len(closed.Rows))
	}
	open, err := svc.GetDailyRoster(context.Background(), groupID, schoolDay, "")
	if err != nil {
		t.Fatalf("GetDailyRoster on a school day: %v", err)
	}
	if open.NoClasses {
		t.Error("a school day's roster says NoClasses")
	}
}
