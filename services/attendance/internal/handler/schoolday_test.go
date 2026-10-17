package handler_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	attendancev1 "github.com/garcia-lorca/gen/go/garcialorca/attendance/v1"
	"github.com/garcia-lorca/services/attendance/internal/handler"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

const weekendDate = "2026-10-17" // a Saturday

// weekendCalendar closes Saturdays and Sundays (config's rule, faked).
type weekendCalendar struct{}

func (weekendCalendar) SchoolDayOf(_ context.Context, _ string, date time.Time) (service.SchoolDay, error) {
	if wd := date.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return service.SchoolDay{}, nil
	}
	return service.SchoolDay{Open: true}, nil
}

func newCalendarHandler() *handler.AttendanceHandler {
	enr := &fakeEnrollments{byGroup: map[string][]service.EnrollmentRef{groupID: {activeEnrollment("enr-1", "stu-1")}}}
	groups := &fakeGroups{existing: map[string]bool{groupID: true}}
	svc := service.NewAttendanceService(newFakeRepo(), enr, groups, nil, testTenant, func() string { return "rec-1" },
		slog.New(slog.DiscardHandler), service.WithSchoolCalendar(weekendCalendar{}))
	return handler.NewAttendanceHandler(svc)
}

// ISSUE-T14-101 at the wire: InvalidArgument, the sentence, and NO_CLASSES in
// the error metadata the gateway forwards as X-Error-Code.
func TestSaveOnAWeekendIsInvalidArgumentCodedNoClasses(t *testing.T) {
	h := newCalendarHandler()
	_, err := h.SaveDailyAttendance(context.Background(), connect.NewRequest(&attendancev1.SaveDailyAttendanceRequest{
		GroupId: groupID, Date: weekendDate, TakenBy: takerID, TakenByName: "Gabriela Rojas",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
	var ce *connect.Error
	if !errors.As(err, &ce) {
		t.Fatalf("not a connect error: %v", err)
	}
	if ce.Message() != "Ese día no hay clases." {
		t.Errorf("message = %q", ce.Message())
	}
	if got := ce.Meta().Get(handler.ErrorCodeMeta); got != service.CodeNoClasses {
		t.Errorf("X-Error-Code = %q, want %q", got, service.CodeNoClasses)
	}
}

func TestRosterCarriesNoClasses(t *testing.T) {
	h := newCalendarHandler()
	for date, want := range map[string]bool{weekendDate: true, testDate: false} {
		res, err := h.GetDailyRoster(context.Background(), connect.NewRequest(&attendancev1.GetDailyRosterRequest{GroupId: groupID, Date: date}))
		if err != nil {
			t.Fatalf("GetDailyRoster(%s): %v", date, err)
		}
		if res.Msg.GetNoClasses() != want {
			t.Errorf("no_classes(%s) = %v, want %v", date, res.Msg.GetNoClasses(), want)
		}
	}
}
