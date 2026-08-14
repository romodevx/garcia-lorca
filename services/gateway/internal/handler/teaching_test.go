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
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// fakeTeachingConfig doubles the config surface the teaching handler consumes:
// the teacher directory (to resolve caller→teacher by user id), the teacher's
// assignments, and the group/subject catalog used to enrich assignment names. It
// records the teacher id it was asked for — the assertion that matters, since the
// teacher must come from the PRINCIPAL, never from a request field.
type fakeTeachingConfig struct {
	teachers        []client.Teacher
	teachersErr     error
	gotListTeachers int

	assignmentsByTeacher map[string][]client.TeachingAssignment
	assignmentsErr       error
	gotTeacherID         string
	gotListAssignments   int

	groups          []client.GroupResult
	subjects        []client.SubjectResult
	groupsErr       error
	subjectsErr     error
	gotListGroups   int
	gotListSubjects int
}

func (f *fakeTeachingConfig) ListTeachers(_ context.Context) ([]client.Teacher, error) {
	f.gotListTeachers++
	return f.teachers, f.teachersErr
}

func (f *fakeTeachingConfig) ListTeachingAssignments(_ context.Context, _, _, teacherID string) ([]client.TeachingAssignment, error) {
	f.gotListAssignments++
	f.gotTeacherID = teacherID
	if f.assignmentsErr != nil {
		return nil, f.assignmentsErr
	}
	return f.assignmentsByTeacher[teacherID], nil
}

func (f *fakeTeachingConfig) ListGroups(_ context.Context, _, _ string) ([]client.GroupResult, error) {
	f.gotListGroups++
	return f.groups, f.groupsErr
}

func (f *fakeTeachingConfig) ListSubjects(_ context.Context, _ string) ([]client.SubjectResult, error) {
	f.gotListSubjects++
	return f.subjects, f.subjectsErr
}

// fakeTeachingGrades doubles the grades surface, counting its calls so the
// ownership guard's "an unowned assignment must never reach downstream" contract
// is observable, and recording the assignment id it was keyed on.
type fakeTeachingGrades struct {
	gotRosterAssignmentID string
	gotRosterTermID       string
	roster                []client.RosterRow
	rosterErr             error
	rosterCalls           int

	gotCaptureAssignmentID string
	gotCaptureEnrollmentID string
	gotCaptureTermID       string
	gotCaptureValue        float64
	captured               client.GradeEntry
	captureErr             error
	captureCalls           int
}

func (f *fakeTeachingGrades) GetRoster(_ context.Context, assignmentID, termID string) ([]client.RosterRow, error) {
	f.rosterCalls++
	f.gotRosterAssignmentID = assignmentID
	f.gotRosterTermID = termID
	if f.rosterErr != nil {
		return nil, f.rosterErr
	}
	return f.roster, nil
}

func (f *fakeTeachingGrades) CaptureGrade(_ context.Context, assignmentID, enrollmentID, termID string, value float64) (client.GradeEntry, error) {
	f.captureCalls++
	f.gotCaptureAssignmentID = assignmentID
	f.gotCaptureEnrollmentID = enrollmentID
	f.gotCaptureTermID = termID
	f.gotCaptureValue = value
	if f.captureErr != nil {
		return client.GradeEntry{}, f.captureErr
	}
	return f.captured, nil
}

// teacherGET builds an authenticated GET against the teaching surface, with
// userID as the principal the middleware would have injected.
func teacherGET(target, userID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	return req.WithContext(middleware.WithPrincipal(req.Context(), middleware.Principal{
		UserID: userID, TenantID: "t1", Email: "prof@garcialorca.mx", Permissions: []string{"teaching:read"},
	}))
}

// teacherPOST builds an authenticated POST with a JSON body against the teaching
// surface.
func teacherPOST(target, userID, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	return req.WithContext(middleware.WithPrincipal(req.Context(), middleware.Principal{
		UserID: userID, TenantID: "t1", Email: "prof@garcialorca.mx", Permissions: []string{"teaching:write"},
	}))
}

// oneTeacher is the common fixture: the principal usr-t is linked to teacher
// tch-1, who teaches exactly asg-1.
func oneTeacher() *fakeTeachingConfig {
	return &fakeTeachingConfig{
		teachers: []client.Teacher{
			{ID: "tch-0", FirstName: "Otra", LastName: "Maestra", UserID: "usr-other"},
			{ID: "tch-1", FirstName: "Juan", LastName: "Pérez", UserID: "usr-t"},
		},
		assignmentsByTeacher: map[string][]client.TeachingAssignment{
			"tch-1": {{ID: "asg-1", CycleID: "cyc-1", GroupID: "grp-1", SubjectID: "sub-1", TeacherID: "tch-1"}},
		},
	}
}

// --- assignments ----------------------------------------------------------

func TestTeachingAssignmentsReturnsCallerOwnAssignmentsEnriched(t *testing.T) {
	config := &fakeTeachingConfig{
		teachers: []client.Teacher{
			{ID: "tch-1", UserID: "usr-t"},
			{ID: "tch-2", UserID: "usr-other"},
		},
		assignmentsByTeacher: map[string][]client.TeachingAssignment{
			"tch-1": {
				{ID: "asg-1", CycleID: "cyc-1", GroupID: "grp-1", SubjectID: "sub-1", TeacherID: "tch-1"},
				{ID: "asg-2", CycleID: "cyc-1", GroupID: "grp-2", SubjectID: "sub-2", TeacherID: "tch-1"},
			},
			"tch-2": {{ID: "asg-x", TeacherID: "tch-2"}},
		},
		groups:   []client.GroupResult{{ID: "grp-1", Name: "3° A"}, {ID: "grp-2", Name: "4° B"}},
		subjects: []client.SubjectResult{{ID: "sub-1", Name: "Matemáticas"}, {ID: "sub-2", Name: "Español"}},
	}
	h := handler.NewTeachingHandler(config, &fakeTeachingGrades{})

	rec := httptest.NewRecorder()
	h.Assignments(rec, teacherGET("/api/v1/teaching/assignments", "usr-t"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// The teacher id is derived from the principal, never from the request.
	if config.gotTeacherID != "tch-1" {
		t.Errorf("assignments read for teacher %q, want the principal's own tch-1", config.gotTeacherID)
	}
	var out struct {
		Assignments []map[string]any `json:"assignments"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Assignments) != 2 {
		t.Fatalf("assignments = %d, want 2 (only the caller's own)", len(out.Assignments))
	}
	byID := map[string]map[string]any{}
	for _, a := range out.Assignments {
		byID[a["id"].(string)] = a
	}
	if byID["asg-1"]["groupName"] != "3° A" || byID["asg-1"]["subjectName"] != "Matemáticas" {
		t.Errorf("asg-1 = %v, want ids kept and names enriched", byID["asg-1"])
	}
	if byID["asg-2"]["groupName"] != "4° B" || byID["asg-2"]["subjectName"] != "Español" {
		t.Errorf("asg-2 = %v, want 4° B / Español", byID["asg-2"])
	}
	if byID["asg-1"]["groupId"] != "grp-1" || byID["asg-1"]["subjectId"] != "sub-1" {
		t.Errorf("asg-1 = %v, want the bare ids kept alongside the names", byID["asg-1"])
	}
}

// A principal with no linked teacher record simply teaches nothing: an empty
// array, never a 500 and never a downstream assignment read.
func TestTeachingAssignmentsNoLinkedTeacherIsEmptyArray(t *testing.T) {
	config := &fakeTeachingConfig{
		teachers: []client.Teacher{{ID: "tch-1", UserID: "usr-someone-else"}},
	}
	h := handler.NewTeachingHandler(config, &fakeTeachingGrades{})

	rec := httptest.NewRecorder()
	h.Assignments(rec, teacherGET("/api/v1/teaching/assignments", "usr-nobody"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if config.gotListAssignments != 0 {
		t.Errorf("assignments were read %d times; a principal with no teacher record needs no lookup", config.gotListAssignments)
	}
	var out struct {
		Assignments []map[string]any `json:"assignments"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Assignments == nil || len(out.Assignments) != 0 {
		t.Errorf("assignments = %v, want an empty array (never null)", out.Assignments)
	}
}

// A catalog lookup miss must never fail the assignments read: the names degrade
// to empty strings while the assignments still come back 200.
func TestTeachingAssignmentsEnrichmentDegradesToEmptyNames(t *testing.T) {
	config := oneTeacher()
	config.groupsErr = connect.NewError(connect.CodeUnavailable, errString("config down"))
	config.subjectsErr = connect.NewError(connect.CodeUnavailable, errString("config down"))
	h := handler.NewTeachingHandler(config, &fakeTeachingGrades{})

	rec := httptest.NewRecorder()
	h.Assignments(rec, teacherGET("/api/v1/teaching/assignments", "usr-t"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 despite the catalog error", rec.Code)
	}
	var out struct {
		Assignments []map[string]any `json:"assignments"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Assignments) != 1 {
		t.Fatalf("assignments = %d, want 1 (still returned)", len(out.Assignments))
	}
	a := out.Assignments[0]
	if a["id"] != "asg-1" || a["groupName"] != "" || a["subjectName"] != "" {
		t.Errorf("assignment = %v, want ids kept and names degraded to empty strings", a)
	}
}

func TestTeachingAssignmentsPropagatesDirectoryError(t *testing.T) {
	config := &fakeTeachingConfig{teachersErr: connect.NewError(connect.CodeUnavailable, errString("config down"))}
	h := handler.NewTeachingHandler(config, &fakeTeachingGrades{})

	rec := httptest.NewRecorder()
	h.Assignments(rec, teacherGET("/api/v1/teaching/assignments", "usr-t"))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestTeachingAssignmentsWithoutPrincipalIs401(t *testing.T) {
	h := handler.NewTeachingHandler(oneTeacher(), &fakeTeachingGrades{})
	rec := httptest.NewRecorder()
	h.Assignments(rec, httptest.NewRequest(http.MethodGet, "/api/v1/teaching/assignments", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// Defense-in-depth on the authz boundary: an empty principal UserID must NEVER
// match an unlinked catalog teacher (UserID==""), which would inherit that
// teacher's assignments through "" == "". A principal with no subject teaches
// nothing — zero assignments, no downstream lookup — even when unlinked teachers
// exist in the catalog. A properly linked teacher still resolves normally.
func TestTeachingAssignmentsEmptyPrincipalNeverMatchesUnlinkedTeacher(t *testing.T) {
	config := &fakeTeachingConfig{
		teachers: []client.Teacher{
			{ID: "tch-unlinked", UserID: ""}, // an unlinked catalog teacher
			{ID: "tch-1", UserID: "usr-t"},
		},
		assignmentsByTeacher: map[string][]client.TeachingAssignment{
			"tch-unlinked": {{ID: "asg-ghost", TeacherID: "tch-unlinked"}},
			"tch-1":        {{ID: "asg-1", TeacherID: "tch-1"}},
		},
	}
	h := handler.NewTeachingHandler(config, &fakeTeachingGrades{})

	// A principal with an empty UserID must resolve to zero assignments and never
	// scan for a teacher.
	rec := httptest.NewRecorder()
	h.Assignments(rec, teacherGET("/api/v1/teaching/assignments", ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if config.gotListAssignments != 0 {
		t.Errorf("assignments were read %d time(s); an empty principal must never resolve a teacher", config.gotListAssignments)
	}
	var out struct {
		Assignments []map[string]any `json:"assignments"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Assignments) != 0 {
		t.Fatalf("assignments = %d, want 0 (empty principal never inherits an unlinked teacher)", len(out.Assignments))
	}

	// The same catalog still resolves a properly linked teacher normally.
	rec = httptest.NewRecorder()
	h.Assignments(rec, teacherGET("/api/v1/teaching/assignments", "usr-t"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if config.gotTeacherID != "tch-1" {
		t.Errorf("assignments read for teacher %q, want the linked tch-1", config.gotTeacherID)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Assignments) != 1 || out.Assignments[0]["id"] != "asg-1" {
		t.Errorf("assignments = %v, want the linked teacher's own asg-1", out.Assignments)
	}
}

// --- roster ----------------------------------------------------------------

func TestTeachingRosterOwnedAssignmentReads(t *testing.T) {
	config := oneTeacher()
	grades := &fakeTeachingGrades{roster: []client.RosterRow{
		{EnrollmentID: "enr-1", StudentID: "stu-1", Value: 8.5, HasValue: true},
	}}
	h := handler.NewTeachingHandler(config, grades)

	rec := httptest.NewRecorder()
	h.Roster(rec, teacherGET("/api/v1/teaching/roster?assignmentId=asg-1&termId=trm-1", "usr-t"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if grades.rosterCalls != 1 || grades.gotRosterAssignmentID != "asg-1" || grades.gotRosterTermID != "trm-1" {
		t.Errorf("roster read = %d call(s) for %q/%q, want 1 for asg-1/trm-1",
			grades.rosterCalls, grades.gotRosterAssignmentID, grades.gotRosterTermID)
	}
	var out struct {
		Roster []map[string]any `json:"roster"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Roster) != 1 || out.Roster[0]["enrollmentId"] != "enr-1" {
		t.Errorf("roster = %v, want one row keyed on enr-1", out.Roster)
	}
}

// The ownership core: an assignmentId NOT belonging to the caller answers 404 and
// NEVER reaches the grades service — no roster is read for another teacher's group.
func TestTeachingRosterUnownedAssignmentIs404AndDoesNotRead(t *testing.T) {
	config := oneTeacher() // teaches only asg-1
	grades := &fakeTeachingGrades{}
	h := handler.NewTeachingHandler(config, grades)

	rec := httptest.NewRecorder()
	h.Roster(rec, teacherGET("/api/v1/teaching/roster?assignmentId=asg-999&termId=trm-1", "usr-t"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (never 403 — no existence leak)", rec.Code)
	}
	if grades.rosterCalls != 0 {
		t.Errorf("roster was read %d time(s); an unowned assignment must never reach downstream", grades.rosterCalls)
	}
}

func TestTeachingRosterWithoutPrincipalIs401(t *testing.T) {
	h := handler.NewTeachingHandler(oneTeacher(), &fakeTeachingGrades{})
	rec := httptest.NewRecorder()
	h.Roster(rec, httptest.NewRequest(http.MethodGet, "/api/v1/teaching/roster?assignmentId=asg-1", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// --- capture ---------------------------------------------------------------

func TestTeachingCaptureOwnedAssignmentCaptures(t *testing.T) {
	config := oneTeacher()
	grades := &fakeTeachingGrades{captured: client.GradeEntry{
		ID: "grd-1", EnrollmentID: "enr-1", SubjectID: "sub-1", TermID: "trm-1", TeacherID: "tch-1", Value: 9,
	}}
	h := handler.NewTeachingHandler(config, grades)

	rec := httptest.NewRecorder()
	h.CaptureGrade(rec, teacherPOST("/api/v1/teaching/grades", "usr-t",
		`{"assignmentId":"asg-1","enrollmentId":"enr-1","termId":"trm-1","value":9}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if grades.captureCalls != 1 || grades.gotCaptureAssignmentID != "asg-1" ||
		grades.gotCaptureEnrollmentID != "enr-1" || grades.gotCaptureTermID != "trm-1" || grades.gotCaptureValue != 9 {
		t.Errorf("capture = %d call(s) asg %q enr %q trm %q val %v, want 1 for asg-1/enr-1/trm-1/9",
			grades.captureCalls, grades.gotCaptureAssignmentID, grades.gotCaptureEnrollmentID, grades.gotCaptureTermID, grades.gotCaptureValue)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["id"] != "grd-1" {
		t.Errorf("response id = %v, want grd-1", out["id"])
	}
}

// The ownership core on the write path: capturing against an assignment the
// caller does not teach answers 404 and NEVER reaches the grades service.
func TestTeachingCaptureUnownedAssignmentIs404AndDoesNotCapture(t *testing.T) {
	config := oneTeacher() // teaches only asg-1
	grades := &fakeTeachingGrades{}
	h := handler.NewTeachingHandler(config, grades)

	rec := httptest.NewRecorder()
	h.CaptureGrade(rec, teacherPOST("/api/v1/teaching/grades", "usr-t",
		`{"assignmentId":"asg-999","enrollmentId":"enr-1","termId":"trm-1","value":9}`))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (never 403 — no existence leak)", rec.Code)
	}
	if grades.captureCalls != 0 {
		t.Errorf("capture ran %d time(s); an unowned assignment must never reach downstream", grades.captureCalls)
	}
}

// A teacher capturing against ANOTHER teacher's assignment (one that exists, but
// is not theirs) is the same 404: ownership is by construction, not by existence.
func TestTeachingCaptureAnotherTeachersAssignmentIs404(t *testing.T) {
	config := &fakeTeachingConfig{
		teachers: []client.Teacher{
			{ID: "tch-1", UserID: "usr-t"},
			{ID: "tch-2", UserID: "usr-other"},
		},
		assignmentsByTeacher: map[string][]client.TeachingAssignment{
			"tch-1": {{ID: "asg-1", TeacherID: "tch-1"}},
			"tch-2": {{ID: "asg-2", TeacherID: "tch-2"}},
		},
	}
	grades := &fakeTeachingGrades{}
	h := handler.NewTeachingHandler(config, grades)

	rec := httptest.NewRecorder()
	// asg-2 belongs to tch-2, not to the caller's tch-1.
	h.CaptureGrade(rec, teacherPOST("/api/v1/teaching/grades", "usr-t",
		`{"assignmentId":"asg-2","enrollmentId":"enr-1","termId":"trm-1","value":9}`))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (another teacher's assignment is not the caller's)", rec.Code)
	}
	if grades.captureCalls != 0 {
		t.Errorf("capture ran %d time(s); another teacher's assignment must never reach downstream", grades.captureCalls)
	}
}

func TestTeachingCaptureWithoutPrincipalIs401(t *testing.T) {
	h := handler.NewTeachingHandler(oneTeacher(), &fakeTeachingGrades{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/teaching/grades",
		strings.NewReader(`{"assignmentId":"asg-1","enrollmentId":"enr-1","termId":"trm-1","value":9}`))
	h.CaptureGrade(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestTeachingCaptureInvalidBodyIs400(t *testing.T) {
	h := handler.NewTeachingHandler(oneTeacher(), &fakeTeachingGrades{})
	rec := httptest.NewRecorder()
	h.CaptureGrade(rec, teacherPOST("/api/v1/teaching/grades", "usr-t", `{not json`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
