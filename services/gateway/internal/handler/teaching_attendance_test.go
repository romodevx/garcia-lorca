package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// fakeTeachingAttendance doubles the daily pass-list surface the teaching
// attendance handler consumes, counting its calls so the ownership guard's "a
// group the caller does not teach must never reach downstream" contract is
// observable, and recording the group id it was keyed on.
type fakeTeachingAttendance struct {
	gotRosterGroupID string
	gotRosterDate    string
	roster           []client.AttendanceRosterRow
	rosterErr        error
	rosterCalls      int

	gotSaveGroupID string
	gotSaveDate    string
	gotSaveMarks   []client.Mark
	saveAbsent     int32
	saveMarked     int32
	saveErr        error
	saveCalls      int
}

func (f *fakeTeachingAttendance) GetDailyRoster(_ context.Context, groupID, date string) ([]client.AttendanceRosterRow, error) {
	f.rosterCalls++
	f.gotRosterGroupID = groupID
	f.gotRosterDate = date
	if f.rosterErr != nil {
		return nil, f.rosterErr
	}
	return f.roster, nil
}

func (f *fakeTeachingAttendance) SaveDailyAttendance(_ context.Context, groupID, date string, marks []client.Mark) (int32, int32, error) {
	f.saveCalls++
	f.gotSaveGroupID = groupID
	f.gotSaveDate = date
	f.gotSaveMarks = marks
	if f.saveErr != nil {
		return 0, 0, f.saveErr
	}
	return f.saveAbsent, f.saveMarked, nil
}

// newTeachingAttendance builds the teacher self-scoped attendance handler over a
// config (caller→teacher→groups scoping + group-name enrichment), an attendance
// service (roster + save) and a students directory (roster name enrichment). The
// grades dependency of the scope handler is unused here, so a bare fake suffices.
func newTeachingAttendance(config handler.TeachingConfigReader, att handler.TeachingAttendanceService, students handler.TeachingStudentReader) *handler.TeachingAttendanceHandler {
	scope := handler.NewTeachingHandler(config, &fakeTeachingGrades{}, students)
	return handler.NewTeachingAttendanceHandler(scope, att)
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

// --- roster ----------------------------------------------------------------

func TestTeachingAttendanceRosterOwnedGroupReads(t *testing.T) {
	config := oneTeacher() // teaches grp-1 (asg-1)
	att := &fakeTeachingAttendance{roster: []client.AttendanceRosterRow{
		{EnrollmentID: "enr-1", StudentID: "stu-1", Status: "present"},
		{EnrollmentID: "enr-2", StudentID: "stu-2", Status: "absent"},
	}}
	// The Maestro holds no students:read, so the gateway resolves the names on its
	// behalf: ONE list read builds the studentId→name map.
	students := &fakeTeachingStudents{students: []client.StudentResult{
		{ID: "stu-1", FirstName: "Ana", LastName: "García"},
		{ID: "stu-2", FirstName: "Luis", LastName: "Pérez"},
		{ID: "stu-9", FirstName: "Otra", LastName: "Persona"},
	}}
	h := newTeachingAttendance(config, att, students)

	rec := httptest.NewRecorder()
	h.Roster(rec, teacherGET("/api/v1/teaching/attendance/roster?groupId=grp-1&date=2026-09-01", "usr-t"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if att.rosterCalls != 1 || att.gotRosterGroupID != "grp-1" || att.gotRosterDate != "2026-09-01" {
		t.Errorf("roster read = %d call(s) for %q/%q, want 1 for grp-1/2026-09-01",
			att.rosterCalls, att.gotRosterGroupID, att.gotRosterDate)
	}
	if students.gotListCalls != 1 {
		t.Errorf("students listed %d time(s), want exactly 1 for the whole roster", students.gotListCalls)
	}
	var out struct {
		Roster []map[string]any `json:"roster"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Roster) != 2 {
		t.Fatalf("roster = %v, want two rows", out.Roster)
	}
	byEnr := map[string]map[string]any{}
	for _, row := range out.Roster {
		byEnr[row["enrollmentId"].(string)] = row
	}
	if byEnr["enr-1"]["studentId"] != "stu-1" || byEnr["enr-1"]["studentName"] != "Ana García" ||
		byEnr["enr-1"]["status"] != "present" {
		t.Errorf("enr-1 = %v, want fields kept and studentName enriched", byEnr["enr-1"])
	}
	if byEnr["enr-2"]["studentName"] != "Luis Pérez" || byEnr["enr-2"]["status"] != "absent" {
		t.Errorf("enr-2 = %v, want Luis Pérez / absent", byEnr["enr-2"])
	}
}

// A students-lookup miss must never fail the roster read: the names degrade to
// empty strings while the rows still come back 200.
func TestTeachingAttendanceRosterStudentLookupErrorDegradesToEmptyName(t *testing.T) {
	config := oneTeacher()
	att := &fakeTeachingAttendance{roster: []client.AttendanceRosterRow{
		{EnrollmentID: "enr-1", StudentID: "stu-1", Status: "late"},
	}}
	students := &fakeTeachingStudents{err: connect.NewError(connect.CodeUnavailable, errString("students down"))}
	h := newTeachingAttendance(config, att, students)

	rec := httptest.NewRecorder()
	h.Roster(rec, teacherGET("/api/v1/teaching/attendance/roster?groupId=grp-1&date=2026-09-01", "usr-t"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 despite the students error", rec.Code)
	}
	var out struct {
		Roster []map[string]any `json:"roster"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Roster) != 1 {
		t.Fatalf("roster = %d, want 1 (the row still comes back)", len(out.Roster))
	}
	row := out.Roster[0]
	if row["studentId"] != "stu-1" || row["studentName"] != "" || row["status"] != "late" {
		t.Errorf("row = %v, want ids/status kept and studentName degraded to empty string", row)
	}
}

// The ownership core: a groupId NOT belonging to the caller answers 404 and NEVER
// reaches the attendance service — no roster is read for another teacher's group.
func TestTeachingAttendanceRosterForeignGroupIs404AndDoesNotRead(t *testing.T) {
	config := oneTeacher() // teaches only grp-1
	att := &fakeTeachingAttendance{}
	h := newTeachingAttendance(config, att, &fakeTeachingStudents{})

	rec := httptest.NewRecorder()
	h.Roster(rec, teacherGET("/api/v1/teaching/attendance/roster?groupId=grp-999&date=2026-09-01", "usr-t"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (never 403 — no existence leak)", rec.Code)
	}
	if att.rosterCalls != 0 {
		t.Errorf("roster was read %d time(s); an unowned group must never reach downstream", att.rosterCalls)
	}
}

// An empty principal resolves to an empty group set, so even a real group id
// answers 404 and never reaches the attendance service.
func TestTeachingAttendanceRosterEmptyPrincipalIs404AndDoesNotRead(t *testing.T) {
	config := oneTeacher()
	att := &fakeTeachingAttendance{}
	h := newTeachingAttendance(config, att, &fakeTeachingStudents{})

	rec := httptest.NewRecorder()
	h.Roster(rec, teacherGET("/api/v1/teaching/attendance/roster?groupId=grp-1&date=2026-09-01", ""))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (empty principal owns no group)", rec.Code)
	}
	if att.rosterCalls != 0 {
		t.Errorf("roster was read %d time(s); an empty principal must never reach downstream", att.rosterCalls)
	}
}

func TestTeachingAttendanceRosterWithoutPrincipalIs401(t *testing.T) {
	h := newTeachingAttendance(oneTeacher(), &fakeTeachingAttendance{}, &fakeTeachingStudents{})
	rec := httptest.NewRecorder()
	h.Roster(rec, httptest.NewRequest(http.MethodGet, "/api/v1/teaching/attendance/roster?groupId=grp-1", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// --- save ------------------------------------------------------------------

func TestTeachingAttendanceSaveOwnedGroupSaves(t *testing.T) {
	config := oneTeacher()
	att := &fakeTeachingAttendance{saveAbsent: 1, saveMarked: 2}
	h := newTeachingAttendance(config, att, &fakeTeachingStudents{})

	rec := httptest.NewRecorder()
	h.SaveAttendance(rec, teacherPOST("/api/v1/teaching/attendance", "usr-t",
		`{"groupId":"grp-1","date":"2026-09-01","marks":[{"enrollmentId":"enr-1","status":"absent","note":"cita médica"},{"enrollmentId":"enr-2","status":"late"}]}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if att.saveCalls != 1 || att.gotSaveGroupID != "grp-1" || att.gotSaveDate != "2026-09-01" {
		t.Errorf("save = %d call(s) for %q/%q, want 1 for grp-1/2026-09-01",
			att.saveCalls, att.gotSaveGroupID, att.gotSaveDate)
	}
	if len(att.gotSaveMarks) != 2 || att.gotSaveMarks[0].EnrollmentID != "enr-1" ||
		att.gotSaveMarks[0].Status != "absent" || att.gotSaveMarks[0].Note != "cita médica" {
		t.Errorf("marks = %+v, want the exceptions passed through unchanged", att.gotSaveMarks)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["absentCount"] != float64(1) || out["markedCount"] != float64(2) {
		t.Errorf("summary = %v, want absentCount 1 / markedCount 2", out)
	}
}

// The ownership core on the write path: saving against a group the caller does
// not teach answers 404 and NEVER reaches the attendance service.
func TestTeachingAttendanceSaveForeignGroupIs404AndDoesNotSave(t *testing.T) {
	config := oneTeacher() // teaches only grp-1
	att := &fakeTeachingAttendance{}
	h := newTeachingAttendance(config, att, &fakeTeachingStudents{})

	rec := httptest.NewRecorder()
	h.SaveAttendance(rec, teacherPOST("/api/v1/teaching/attendance", "usr-t",
		`{"groupId":"grp-999","date":"2026-09-01","marks":[]}`))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (not the caller's group)", rec.Code)
	}
	if att.saveCalls != 0 {
		t.Errorf("save ran %d time(s); an unowned group must never reach downstream", att.saveCalls)
	}
}

// An empty principal owns no group, so even a real group id answers 404 and never
// reaches the save.
func TestTeachingAttendanceSaveEmptyPrincipalIs404AndDoesNotSave(t *testing.T) {
	config := oneTeacher()
	att := &fakeTeachingAttendance{}
	h := newTeachingAttendance(config, att, &fakeTeachingStudents{})

	rec := httptest.NewRecorder()
	h.SaveAttendance(rec, teacherPOST("/api/v1/teaching/attendance", "",
		`{"groupId":"grp-1","date":"2026-09-01","marks":[]}`))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (empty principal owns no group)", rec.Code)
	}
	if att.saveCalls != 0 {
		t.Errorf("save ran %d time(s); an empty principal must never reach downstream", att.saveCalls)
	}
}

func TestTeachingAttendanceSaveInvalidBodyIs400(t *testing.T) {
	att := &fakeTeachingAttendance{}
	h := newTeachingAttendance(oneTeacher(), att, &fakeTeachingStudents{})

	rec := httptest.NewRecorder()
	h.SaveAttendance(rec, teacherPOST("/api/v1/teaching/attendance", "usr-t", `{not json`))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if att.saveCalls != 0 {
		t.Errorf("save ran %d time(s); a malformed body must never reach downstream", att.saveCalls)
	}
}

func TestTeachingAttendanceSaveWithoutPrincipalIs401(t *testing.T) {
	h := newTeachingAttendance(oneTeacher(), &fakeTeachingAttendance{}, &fakeTeachingStudents{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/teaching/attendance",
		strings.NewReader(`{"groupId":"grp-1","date":"2026-09-01","marks":[]}`))
	h.SaveAttendance(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
