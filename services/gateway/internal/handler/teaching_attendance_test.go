package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
	rosterErr          error
	rosterCalls        int

	gotSave    client.SaveAttendanceInput
	saveAbsent int32
	saveMarked int32
	saveErr    error
	saveCalls  int
}

func (f *fakeTeachingAttendance) GetDailyRoster(_ context.Context, groupID, date, subjectID string) (client.AttendanceRoster, error) {
	f.rosterCalls++
	f.gotRosterGroupID, f.gotRosterDate, f.gotRosterSubjectID = groupID, date, subjectID
	if f.rosterErr != nil {
		return client.AttendanceRoster{}, f.rosterErr
	}
	return client.AttendanceRoster{Rows: f.roster, Session: f.session}, nil
}

func (f *fakeTeachingAttendance) SaveDailyAttendance(_ context.Context, in client.SaveAttendanceInput) (int32, int32, error) {
	f.saveCalls++
	f.gotSave = in
	if f.saveErr != nil {
		return 0, 0, f.saveErr
	}
	return f.saveAbsent, f.saveMarked, nil
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

// twoGroupTeacher is the multi-group fixture: the principal usr-t is linked to
// teacher tch-1, who teaches THREE assignments but across only TWO distinct
// groups (grp-1 twice, for two subjects, plus grp-2). tch-2 teaches grp-9, which
// the caller must never see.
func twoGroupTeacher() *fakeTeachingConfig {
	return &fakeTeachingConfig{
		teachers: []client.Teacher{
			{ID: "tch-1", UserID: "usr-t"},
			{ID: "tch-2", UserID: "usr-other"},
		},
		assignmentsByTeacher: map[string][]client.TeachingAssignment{
			"tch-1": {
				{ID: "asg-1", CycleID: "cyc-1", GroupID: "grp-1", SubjectID: "sub-1", TeacherID: "tch-1"},
				{ID: "asg-2", CycleID: "cyc-1", GroupID: "grp-2", SubjectID: "sub-2", TeacherID: "tch-1"},
				{ID: "asg-3", CycleID: "cyc-1", GroupID: "grp-1", SubjectID: "sub-3", TeacherID: "tch-1"}, // same group as asg-1
			},
			"tch-2": {{ID: "asg-x", GroupID: "grp-9", TeacherID: "tch-2"}},
		},
		groups: []client.GroupResult{
			{ID: "grp-1", Name: "3° A"},
			{ID: "grp-2", Name: "4° B"},
			{ID: "grp-9", Name: "5° C"},
		},
	}
}

// --- groups ----------------------------------------------------------------

// Attendance is taken per GROUP, so the caller's scope is the DISTINCT groups
// across their assignments: grp-1 twice + grp-2 collapses to two, each enriched
// with its display name, and the other teacher's grp-9 never appears.
func TestTeachingGroupsReturnsDistinctGroupsEnriched(t *testing.T) {
	config := twoGroupTeacher()
	h := newTeachingAttendance(config, &fakeTeachingAttendance{}, &fakeTeachingStudents{})

	rec := httptest.NewRecorder()
	h.Groups(rec, teacherGET("/api/v1/teaching/groups", "usr-t"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// The teacher id is derived from the principal, never from the request.
	if config.gotTeacherID != "tch-1" {
		t.Errorf("groups read for teacher %q, want the principal's own tch-1", config.gotTeacherID)
	}
	var out struct {
		Groups []map[string]any `json:"groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Groups) != 2 {
		t.Fatalf("groups = %d, want 2 (distinct, grp-1 deduped across two subjects)", len(out.Groups))
	}
	byID := map[string]map[string]any{}
	for _, g := range out.Groups {
		byID[g["id"].(string)] = g
	}
	if byID["grp-1"]["name"] != "3° A" || byID["grp-2"]["name"] != "4° B" {
		t.Errorf("groups = %v, want grp-1/3° A and grp-2/4° B enriched", out.Groups)
	}
	if _, ok := byID["grp-9"]; ok {
		t.Errorf("groups = %v, must never include another teacher's grp-9", out.Groups)
	}
}

// A principal with no linked teacher record teaches no groups: an empty array,
// never a 500 and never a downstream assignment read.
func TestTeachingGroupsNoLinkedTeacherIsEmptyArray(t *testing.T) {
	config := &fakeTeachingConfig{
		teachers: []client.Teacher{{ID: "tch-1", UserID: "usr-someone-else"}},
	}
	h := newTeachingAttendance(config, &fakeTeachingAttendance{}, &fakeTeachingStudents{})

	rec := httptest.NewRecorder()
	h.Groups(rec, teacherGET("/api/v1/teaching/groups", "usr-nobody"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if config.gotListAssignments != 0 {
		t.Errorf("assignments were read %d time(s); a principal with no teacher record needs no lookup", config.gotListAssignments)
	}
	var out struct {
		Groups []map[string]any `json:"groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Groups == nil || len(out.Groups) != 0 {
		t.Errorf("groups = %v, want an empty array (never null)", out.Groups)
	}
}

// A catalog lookup miss must never fail the groups read: the names degrade to
// empty strings while the groups still come back 200.
func TestTeachingGroupsEnrichmentDegradesToEmptyNames(t *testing.T) {
	config := twoGroupTeacher()
	config.groupsErr = connect.NewError(connect.CodeUnavailable, errString("config down"))
	h := newTeachingAttendance(config, &fakeTeachingAttendance{}, &fakeTeachingStudents{})

	rec := httptest.NewRecorder()
	h.Groups(rec, teacherGET("/api/v1/teaching/groups", "usr-t"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 despite the catalog error", rec.Code)
	}
	var out struct {
		Groups []map[string]any `json:"groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Groups) != 2 {
		t.Fatalf("groups = %d, want 2 (still returned)", len(out.Groups))
	}
	for _, g := range out.Groups {
		if g["name"] != "" {
			t.Errorf("group %v, want name degraded to empty string", g)
		}
	}
}

func TestTeachingGroupsPropagatesDirectoryError(t *testing.T) {
	config := &fakeTeachingConfig{teachersErr: connect.NewError(connect.CodeUnavailable, errString("config down"))}
	h := newTeachingAttendance(config, &fakeTeachingAttendance{}, &fakeTeachingStudents{})

	rec := httptest.NewRecorder()
	h.Groups(rec, teacherGET("/api/v1/teaching/groups", "usr-t"))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestTeachingGroupsWithoutPrincipalIs401(t *testing.T) {
	h := newTeachingAttendance(twoGroupTeacher(), &fakeTeachingAttendance{}, &fakeTeachingStudents{})
	rec := httptest.NewRecorder()
	h.Groups(rec, httptest.NewRequest(http.MethodGet, "/api/v1/teaching/groups", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// Defense-in-depth: an empty principal UserID must NEVER match an unlinked
// catalog teacher (UserID==""), which would inherit that teacher's groups. An
// empty principal teaches no groups — zero, no downstream lookup.
func TestTeachingGroupsEmptyPrincipalNeverMatchesUnlinkedTeacher(t *testing.T) {
	config := &fakeTeachingConfig{
		teachers: []client.Teacher{
			{ID: "tch-unlinked", UserID: ""},
			{ID: "tch-1", UserID: "usr-t"},
		},
		assignmentsByTeacher: map[string][]client.TeachingAssignment{
			"tch-unlinked": {{ID: "asg-ghost", GroupID: "grp-ghost", TeacherID: "tch-unlinked"}},
			"tch-1":        {{ID: "asg-1", GroupID: "grp-1", TeacherID: "tch-1"}},
		},
		groups: []client.GroupResult{{ID: "grp-1", Name: "3° A"}},
	}
	h := newTeachingAttendance(config, &fakeTeachingAttendance{}, &fakeTeachingStudents{})

	rec := httptest.NewRecorder()
	h.Groups(rec, teacherGET("/api/v1/teaching/groups", ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if config.gotListAssignments != 0 {
		t.Errorf("assignments were read %d time(s); an empty principal must never resolve a teacher", config.gotListAssignments)
	}
	var out struct {
		Groups []map[string]any `json:"groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Groups) != 0 {
		t.Fatalf("groups = %d, want 0 (empty principal never inherits an unlinked teacher)", len(out.Groups))
	}
}

// newTeachingAttendanceWithUsers is newTeachingAttendance with its own directory.
func newTeachingAttendanceWithUsers(config handler.TeachingConfigReader, att handler.TeachingAttendanceService, users handler.AttendanceUserReader) *handler.TeachingAttendanceHandler {
	scope := handler.NewTeachingHandler(config, &fakeTeachingGrades{}, &fakeTeachingStudents{})
	return handler.NewTeachingAttendanceHandler(scope, att, users, handler.WithAttendanceClock(fixedClock))
}
