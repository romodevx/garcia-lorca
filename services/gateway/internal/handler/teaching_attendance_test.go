package handler_test

import (
	"context"
	"time"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// fakeTeachingAttendance doubles the pass-list surface the attendance handlers
// consume, counting its calls so the ownership guard's "a list the caller may not
// touch must never reach downstream" contract is observable, and recording what
// each call was keyed on.
type fakeTeachingAttendance struct {
	gotRosterGroupID   string
	gotRosterDate      string
	gotRosterSubjectID string
	roster             []client.AttendanceRosterRow
	session            *client.AttendanceSession
	noClasses          bool
	rosterErr          error
	rosterCalls        int

	gotSave    client.SaveAttendanceInput
	saveAbsent int32
	saveMarked int32
	saveHeld   []string
	saveErr    error
	saveCalls  int
}

func (f *fakeTeachingAttendance) GetDailyRoster(_ context.Context, groupID, date, subjectID string) (client.AttendanceRoster, error) {
	f.rosterCalls++
	f.gotRosterGroupID, f.gotRosterDate, f.gotRosterSubjectID = groupID, date, subjectID
	if f.rosterErr != nil {
		return client.AttendanceRoster{}, f.rosterErr
	}
	return client.AttendanceRoster{Rows: f.roster, Session: f.session, NoClasses: f.noClasses}, nil
}

func (f *fakeTeachingAttendance) SaveDailyAttendance(_ context.Context, in client.SaveAttendanceInput) (client.SaveAttendanceResult, error) {
	f.saveCalls++
	f.gotSave = in
	if f.saveErr != nil {
		return client.SaveAttendanceResult{}, f.saveErr
	}
	return client.SaveAttendanceResult{AbsentCount: f.saveAbsent, MarkedCount: f.saveMarked, HeldElsewhere: f.saveHeld}, nil
}

// fakeUsers doubles the iam directory the handlers read the taker's display
// name from.
type fakeUsers struct {
	users map[string]client.User
	err   error
}

func (f *fakeUsers) GetUser(_ context.Context, id string) (client.User, error) {
	if f.err != nil {
		return client.User{}, f.err
	}
	u, ok := f.users[id]
	if !ok {
		return client.User{}, connect.NewError(connect.CodeNotFound, errString("user not found"))
	}
	return u, nil
}

// attendanceNow is the fixed clock of every attendance test: 20:00 UTC on
// 2026-09-15, so the system's "today" is 2026-09-15 and the teacher's window
// runs 2026-09-08..2026-09-15.
var attendanceNow = time.Date(2026, 9, 15, 20, 0, 0, 0, time.UTC)

func fixedClock() time.Time { return attendanceNow }

var teacherUsers = &fakeUsers{users: map[string]client.User{"usr-t": {ID: "usr-t", DisplayName: "Gabriela Rojas"}}}

// newTeachingAttendance builds the teacher self-scoped attendance handler over a
// config (caller→teacher→assignments scoping + catalog), an attendance service
// (roster + save) and a students directory (roster name enrichment), on the
// fixed test clock. The grades dependency of the scope handler is unused here.
func newTeachingAttendance(config handler.TeachingConfigReader, att handler.TeachingAttendanceService, students handler.TeachingStudentReader) *handler.TeachingAttendanceHandler {
	scope := handler.NewTeachingHandler(config, &fakeTeachingGrades{}, students)
	return handler.NewTeachingAttendanceHandler(scope, att, teacherUsers, handler.WithAttendanceClock(fixedClock))
}

// newTeachingAttendanceWithUsers is newTeachingAttendance with its own directory.
func newTeachingAttendanceWithUsers(config handler.TeachingConfigReader, att handler.TeachingAttendanceService, users handler.AttendanceUserReader) *handler.TeachingAttendanceHandler {
	scope := handler.NewTeachingHandler(config, &fakeTeachingGrades{}, &fakeTeachingStudents{})
	return handler.NewTeachingAttendanceHandler(scope, att, users, handler.WithAttendanceClock(fixedClock))
}
