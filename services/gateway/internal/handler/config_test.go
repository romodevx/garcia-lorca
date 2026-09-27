package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// fakeConfig is a hand-written double for the catalog surface the handler needs.
// It records the last input per entity and returns canned results/errors so each
// handler is exercised in isolation (no Connect round-trip).
type fakeConfig struct {
	activatedCycle string
	cycleIn        client.CreateCycleInput
	cycles         []client.CycleResult

	levelIn client.CreateLevelInput
	levels  []client.LevelResult

	modeLevelID string
	modeIn      string

	gradingLevelID string
	gradingIn      client.LevelGrading
	gradingErr     error

	periodIn      client.NonWorkingPeriod
	periods       []client.NonWorkingPeriod
	periodsCycle  string
	deletedPeriod string

	profile      client.SchoolProfile
	profileFound bool
	profileIn    client.SchoolProfile

	eventIn      client.SchoolEvent
	events       []client.SchoolEvent
	eventsCycle  string
	deletedEvent string

	gradeLevelIn   client.CreateGradeLevelInput
	gradeLevels    []client.GradeLevelResult
	gradeLevelsFor string // captured level_id filter on list

	subjectIn   client.CreateSubjectInput
	subjects    []client.SubjectResult
	subjectsFor string // captured grade_level_id filter on list

	termIn      client.CreateTermInput
	terms       []client.TermResult
	termsCycle  string
	termsLevel  string
	groupIn     client.CreateGroupInput
	groups      []client.GroupResult
	groupsCycle string
	groupsGrade string

	teacherFirst string
	teacherLast  string
	teacherEmail string
	teacherUser  string
	teachers     []client.Teacher

	assignGroup   string
	assignSubject string
	assignTeacher string
	assignments   []client.TeachingAssignment
	asgCycle      string
	asgGroup      string
	asgTeacher    string

	// teaching load ("carga académica")
	load              *client.TeachingLoadInput
	loadResult        client.TeachingLoadResult
	deletedAssignment string
	reassignID        string
	reassignTo        string

	err error
}

func (f *fakeConfig) CreateSchoolEvent(_ context.Context, in client.SchoolEvent) (client.SchoolEvent, error) {
	f.eventIn = in
	if f.err != nil {
		return client.SchoolEvent{}, f.err
	}
	out := in
	out.ID = "ev-1"
	return out, nil
}

func (f *fakeConfig) GetSchoolProfile(_ context.Context) (client.SchoolProfile, bool, error) {
	return f.profile, f.profileFound, f.err
}

func (f *fakeConfig) SetSchoolProfile(_ context.Context, in client.SchoolProfile) (client.SchoolProfile, error) {
	f.profileIn = in
	return in, f.err
}

func (f *fakeConfig) ListSchoolEvents(_ context.Context, cycleID string) ([]client.SchoolEvent, error) {
	f.eventsCycle = cycleID
	return f.events, f.err
}

func (f *fakeConfig) DeleteSchoolEvent(_ context.Context, id string) error {
	f.deletedEvent = id
	return f.err
}

func (f *fakeConfig) CreateNonWorkingPeriod(_ context.Context, cycleID, label, kind, startsOn, endsOn string) (client.NonWorkingPeriod, error) {
	f.periodIn = client.NonWorkingPeriod{CycleID: cycleID, Label: label, Kind: kind, StartsOn: startsOn, EndsOn: endsOn}
	if f.err != nil {
		return client.NonWorkingPeriod{}, f.err
	}
	return client.NonWorkingPeriod{ID: "per-1", CycleID: cycleID, Label: label, Kind: kind, StartsOn: startsOn, EndsOn: endsOn}, nil
}

func (f *fakeConfig) ListNonWorkingPeriods(_ context.Context, cycleID string) ([]client.NonWorkingPeriod, error) {
	f.periodsCycle = cycleID
	return f.periods, f.err
}

func (f *fakeConfig) DeleteNonWorkingPeriod(_ context.Context, id string) error {
	f.deletedPeriod = id
	return f.err
}

func (f *fakeConfig) CreateCycle(_ context.Context, in client.CreateCycleInput) (client.CycleResult, error) {
	f.cycleIn = in
	if f.err != nil {
		return client.CycleResult{}, f.err
	}
	return client.CycleResult{ID: "cyc-1", Name: in.Name, Status: in.Status, StartsOn: in.StartsOn, EndsOn: in.EndsOn}, nil
}

func (f *fakeConfig) ListCycles(_ context.Context) ([]client.CycleResult, error) {
	return f.cycles, f.err
}

func (f *fakeConfig) CreateLevel(_ context.Context, in client.CreateLevelInput) (client.LevelResult, error) {
	f.levelIn = in
	if f.err != nil {
		return client.LevelResult{}, f.err
	}
	return client.LevelResult{ID: "lvl-1", Name: in.Name, SEPCode: in.SEPCode, Granularity: in.Granularity}, nil
}

func (f *fakeConfig) ListLevels(_ context.Context) ([]client.LevelResult, error) {
	return f.levels, f.err
}

func (f *fakeConfig) SetLevelAttendanceMode(_ context.Context, id, mode string) (client.LevelResult, error) {
	f.modeLevelID, f.modeIn = id, mode
	if f.err != nil {
		return client.LevelResult{}, f.err
	}
	return client.LevelResult{ID: id, Name: "Secundaria", Granularity: "cycle", AttendanceMode: mode}, nil
}

func (f *fakeConfig) SetLevelGradingScale(_ context.Context, id string, g client.LevelGrading) (client.LevelResult, error) {
	f.gradingLevelID, f.gradingIn = id, g
	if f.gradingErr != nil {
		return client.LevelResult{}, f.gradingErr
	}
	mode := f.modeIn
	if mode == "" {
		mode = "daily"
	}
	return client.LevelResult{ID: id, Name: "Primaria", Granularity: "cycle", AttendanceMode: mode, Grading: g}, nil
}

func (f *fakeConfig) CreateGradeLevel(_ context.Context, in client.CreateGradeLevelInput) (client.GradeLevelResult, error) {
	f.gradeLevelIn = in
	if f.err != nil {
		return client.GradeLevelResult{}, f.err
	}
	return client.GradeLevelResult{ID: "grd-1", LevelID: in.LevelID, Name: in.Name, Ordinal: in.Ordinal}, nil
}

func (f *fakeConfig) ListGradeLevels(_ context.Context, levelID string) ([]client.GradeLevelResult, error) {
	f.gradeLevelsFor = levelID
	return f.gradeLevels, f.err
}

func (f *fakeConfig) CreateSubject(_ context.Context, in client.CreateSubjectInput) (client.SubjectResult, error) {
	f.subjectIn = in
	if f.err != nil {
		return client.SubjectResult{}, f.err
	}
	return client.SubjectResult{ID: "sub-1", GradeLevelID: in.GradeLevelID, Name: in.Name}, nil
}

func (f *fakeConfig) ListSubjects(_ context.Context, gradeLevelID string) ([]client.SubjectResult, error) {
	f.subjectsFor = gradeLevelID
	return f.subjects, f.err
}

func (f *fakeConfig) CreateTerm(_ context.Context, in client.CreateTermInput) (client.TermResult, error) {
	f.termIn = in
	if f.err != nil {
		return client.TermResult{}, f.err
	}
	return client.TermResult{ID: "trm-1", CycleID: in.CycleID, LevelID: in.LevelID, Name: in.Name, Ordinal: in.Ordinal, StartsOn: in.StartsOn, EndsOn: in.EndsOn}, nil
}

func (f *fakeConfig) ListTerms(_ context.Context, cycleID, levelID string) ([]client.TermResult, error) {
	f.termsCycle, f.termsLevel = cycleID, levelID
	return f.terms, f.err
}

func (f *fakeConfig) CreateGroup(_ context.Context, in client.CreateGroupInput) (client.GroupResult, error) {
	f.groupIn = in
	if f.err != nil {
		return client.GroupResult{}, f.err
	}
	return client.GroupResult{ID: "grp-1", CycleID: in.CycleID, GradeLevelID: in.GradeLevelID, Name: in.Name, Capacity: in.Capacity}, nil
}

func (f *fakeConfig) ListGroups(_ context.Context, cycleID, gradeLevelID string) ([]client.GroupResult, error) {
	f.groupsCycle, f.groupsGrade = cycleID, gradeLevelID
	return f.groups, f.err
}

func (f *fakeConfig) CreateTeacher(_ context.Context, firstName, lastName, email, userID string) (client.Teacher, error) {
	f.teacherFirst, f.teacherLast, f.teacherEmail, f.teacherUser = firstName, lastName, email, userID
	if f.err != nil {
		return client.Teacher{}, f.err
	}
	return client.Teacher{ID: "tch-1", FirstName: firstName, LastName: lastName, Email: email, UserID: userID}, nil
}

func (f *fakeConfig) ListTeachers(_ context.Context) ([]client.Teacher, error) {
	return f.teachers, f.err
}

func (f *fakeConfig) CreateTeachingAssignment(_ context.Context, groupID, subjectID, teacherID string) (client.TeachingAssignment, error) {
	f.assignGroup, f.assignSubject, f.assignTeacher = groupID, subjectID, teacherID
	if f.err != nil {
		return client.TeachingAssignment{}, f.err
	}
	// cycleId is DERIVED server-side; the fake echoes a canned value to prove the
	// edge surfaces it without ever accepting it from the client.
	return client.TeachingAssignment{ID: "asg-1", CycleID: "cyc-1", GroupID: groupID, SubjectID: subjectID, TeacherID: teacherID}, nil
}

func (f *fakeConfig) ListTeachingAssignments(_ context.Context, cycleID, groupID, teacherID string) ([]client.TeachingAssignment, error) {
	f.asgCycle, f.asgGroup, f.asgTeacher = cycleID, groupID, teacherID
	return f.assignments, f.err
}

func doReq(t *testing.T, hf http.HandlerFunc, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	rec := httptest.NewRecorder()
	hf(rec, r)
	return rec
}

func TestCreateCycleReturns201AndBody(t *testing.T) {
	svc := &fakeConfig{}
	h := handler.NewConfigHandler(svc)

	rec := doReq(t, h.CreateCycle, http.MethodPost, "/api/v1/config/cycles",
		`{"name":"2026-2027","status":"draft","startsOn":"2026-08-01","endsOn":"2027-07-15"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if svc.cycleIn.Name != "2026-2027" || svc.cycleIn.Status != "draft" {
		t.Errorf("forwarded input = %+v", svc.cycleIn)
	}
	if !svc.cycleIn.StartsOn.Equal(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("startsOn not parsed: %v", svc.cycleIn.StartsOn)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if out["id"] != "cyc-1" || out["status"] != "draft" || out["startsOn"] != "2026-08-01" {
		t.Errorf("body = %v", out)
	}
}

func TestCreateCycleInvalidBodyIs400(t *testing.T) {
	h := handler.NewConfigHandler(&fakeConfig{})
	rec := doReq(t, h.CreateCycle, http.MethodPost, "/api/v1/config/cycles", `not json`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCreateCycleBadDateIs400(t *testing.T) {
	h := handler.NewConfigHandler(&fakeConfig{})
	rec := doReq(t, h.CreateCycle, http.MethodPost, "/api/v1/config/cycles",
		`{"name":"x","status":"draft","startsOn":"08/01/2026","endsOn":"2027-07-15"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCreateCycleDuplicateIs409(t *testing.T) {
	svc := &fakeConfig{err: connect.NewError(connect.CodeAlreadyExists, nil)}
	h := handler.NewConfigHandler(svc)
	rec := doReq(t, h.CreateCycle, http.MethodPost, "/api/v1/config/cycles",
		`{"name":"2026-2027","status":"draft","startsOn":"2026-08-01","endsOn":"2027-07-15"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

func TestListCyclesReturnsItems(t *testing.T) {
	svc := &fakeConfig{cycles: []client.CycleResult{
		{ID: "cyc-1", Name: "2026-2027", Status: "draft", StartsOn: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)},
	}}
	h := handler.NewConfigHandler(svc)
	rec := doReq(t, h.ListCycles, http.MethodGet, "/api/v1/config/cycles", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out struct {
		Cycles []map[string]any `json:"cycles"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Cycles) != 1 || out.Cycles[0]["id"] != "cyc-1" || out.Cycles[0]["startsOn"] != "2026-08-01" {
		t.Errorf("body = %v", out)
	}
}

func TestCreateLevelForwardsGranularity(t *testing.T) {
	svc := &fakeConfig{}
	h := handler.NewConfigHandler(svc)
	rec := doReq(t, h.CreateLevel, http.MethodPost, "/api/v1/config/levels",
		`{"name":"Primaria","sepCode":"PRIM","enrollmentGranularity":"cycle"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if svc.levelIn.Granularity != "cycle" || svc.levelIn.SEPCode != "PRIM" {
		t.Errorf("forwarded = %+v", svc.levelIn)
	}
}

func TestListGradeLevelsReadsLevelIDQuery(t *testing.T) {
	svc := &fakeConfig{gradeLevels: []client.GradeLevelResult{{ID: "grd-1", LevelID: "lvl-1", Name: "1°", Ordinal: 1}}}
	h := handler.NewConfigHandler(svc)
	rec := doReq(t, h.ListGradeLevels, http.MethodGet, "/api/v1/config/grade-levels?levelId=lvl-1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.gradeLevelsFor != "lvl-1" {
		t.Errorf("level filter = %q, want lvl-1", svc.gradeLevelsFor)
	}
}

func TestListSubjectsReadsGradeLevelIDQuery(t *testing.T) {
	svc := &fakeConfig{}
	h := handler.NewConfigHandler(svc)
	rec := doReq(t, h.ListSubjects, http.MethodGet, "/api/v1/config/subjects?gradeLevelId=grd-1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.subjectsFor != "grd-1" {
		t.Errorf("grade filter = %q, want grd-1", svc.subjectsFor)
	}
}

func TestCreateTermForwardsParents(t *testing.T) {
	svc := &fakeConfig{}
	h := handler.NewConfigHandler(svc)
	rec := doReq(t, h.CreateTerm, http.MethodPost, "/api/v1/config/terms",
		`{"cycleId":"cyc-1","levelId":"lvl-1","name":"T1","ordinal":1,"startsOn":"2026-08-01","endsOn":"2026-12-15"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if svc.termIn.CycleID != "cyc-1" || svc.termIn.LevelID != "lvl-1" || svc.termIn.Ordinal != 1 {
		t.Errorf("forwarded = %+v", svc.termIn)
	}
}

func TestListTermsReadsCycleAndLevelQuery(t *testing.T) {
	svc := &fakeConfig{}
	h := handler.NewConfigHandler(svc)
	rec := doReq(t, h.ListTerms, http.MethodGet, "/api/v1/config/terms?cycleId=cyc-1&levelId=lvl-1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.termsCycle != "cyc-1" || svc.termsLevel != "lvl-1" {
		t.Errorf("filters = %q/%q", svc.termsCycle, svc.termsLevel)
	}
}

func TestCreateGroupForwardsCapacity(t *testing.T) {
	svc := &fakeConfig{}
	h := handler.NewConfigHandler(svc)
	rec := doReq(t, h.CreateGroup, http.MethodPost, "/api/v1/config/groups",
		`{"cycleId":"cyc-1","gradeLevelId":"grd-1","name":"A","capacity":30}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if svc.groupIn.Capacity != 30 || svc.groupIn.Name != "A" {
		t.Errorf("forwarded = %+v", svc.groupIn)
	}
}

func TestListGroupsReadsCycleAndGradeQuery(t *testing.T) {
	svc := &fakeConfig{}
	h := handler.NewConfigHandler(svc)
	rec := doReq(t, h.ListGroups, http.MethodGet, "/api/v1/config/groups?cycleId=cyc-1&gradeLevelId=grd-1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.groupsCycle != "cyc-1" || svc.groupsGrade != "grd-1" {
		t.Errorf("filters = %q/%q", svc.groupsCycle, svc.groupsGrade)
	}
}

// --- teachers -------------------------------------------------------------

func TestCreateTeacherReturns201AndBody(t *testing.T) {
	svc := &fakeConfig{}
	h := handler.NewConfigHandler(svc)
	rec := doReq(t, h.CreateTeacher, http.MethodPost, "/api/v1/config/teachers",
		`{"firstName":"Ada","lastName":"Lovelace","email":"ada@example.com","userId":"usr-1"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if svc.teacherFirst != "Ada" || svc.teacherLast != "Lovelace" || svc.teacherEmail != "ada@example.com" || svc.teacherUser != "usr-1" {
		t.Errorf("forwarded = %q/%q/%q/%q", svc.teacherFirst, svc.teacherLast, svc.teacherEmail, svc.teacherUser)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if out["id"] != "tch-1" || out["firstName"] != "Ada" || out["email"] != "ada@example.com" || out["userId"] != "usr-1" {
		t.Errorf("body = %v", out)
	}
}

func TestCreateTeacherInvalidBodyIs400(t *testing.T) {
	h := handler.NewConfigHandler(&fakeConfig{})
	rec := doReq(t, h.CreateTeacher, http.MethodPost, "/api/v1/config/teachers", `not json`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCreateTeacherDuplicateIs409(t *testing.T) {
	svc := &fakeConfig{err: connect.NewError(connect.CodeAlreadyExists, nil)}
	h := handler.NewConfigHandler(svc)
	rec := doReq(t, h.CreateTeacher, http.MethodPost, "/api/v1/config/teachers",
		`{"firstName":"Ada","lastName":"Lovelace","email":"ada@example.com"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

func TestListTeachersReturnsItems(t *testing.T) {
	svc := &fakeConfig{teachers: []client.Teacher{
		{ID: "tch-1", FirstName: "Ada", LastName: "Lovelace", Email: "ada@example.com"},
	}}
	h := handler.NewConfigHandler(svc)
	rec := doReq(t, h.ListTeachers, http.MethodGet, "/api/v1/config/teachers", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out struct {
		Teachers []map[string]any `json:"teachers"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Teachers) != 1 || out.Teachers[0]["id"] != "tch-1" || out.Teachers[0]["firstName"] != "Ada" {
		t.Errorf("body = %v", out)
	}
}

// --- teaching_assignments -------------------------------------------------

func TestCreateTeachingAssignmentReturns201AndDerivedCycle(t *testing.T) {
	svc := &fakeConfig{}
	h := handler.NewConfigHandler(svc)
	rec := doReq(t, h.CreateTeachingAssignment, http.MethodPost, "/api/v1/config/teaching-assignments",
		`{"groupId":"grp-1","subjectId":"sub-1","teacherId":"tch-1"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if svc.assignGroup != "grp-1" || svc.assignSubject != "sub-1" || svc.assignTeacher != "tch-1" {
		t.Errorf("forwarded = %q/%q/%q", svc.assignGroup, svc.assignSubject, svc.assignTeacher)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	// cycleId must be present in the response (derived server-side), not the request.
	if out["id"] != "asg-1" || out["cycleId"] != "cyc-1" || out["groupId"] != "grp-1" || out["subjectId"] != "sub-1" || out["teacherId"] != "tch-1" {
		t.Errorf("body = %v", out)
	}
}

func TestCreateTeachingAssignmentInvalidBodyIs400(t *testing.T) {
	h := handler.NewConfigHandler(&fakeConfig{})
	rec := doReq(t, h.CreateTeachingAssignment, http.MethodPost, "/api/v1/config/teaching-assignments", `not json`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// A grade-level mismatch (or a missing parent) surfaces from the config service
// as InvalidArgument, which the edge maps to 400.
func TestCreateTeachingAssignmentMismatchIs400(t *testing.T) {
	svc := &fakeConfig{err: connect.NewError(connect.CodeInvalidArgument, nil)}
	h := handler.NewConfigHandler(svc)
	rec := doReq(t, h.CreateTeachingAssignment, http.MethodPost, "/api/v1/config/teaching-assignments",
		`{"groupId":"grp-1","subjectId":"sub-x","teacherId":"tch-1"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCreateTeachingAssignmentDuplicateIs409(t *testing.T) {
	svc := &fakeConfig{err: connect.NewError(connect.CodeAlreadyExists, nil)}
	h := handler.NewConfigHandler(svc)
	rec := doReq(t, h.CreateTeachingAssignment, http.MethodPost, "/api/v1/config/teaching-assignments",
		`{"groupId":"grp-1","subjectId":"sub-1","teacherId":"tch-1"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

func TestListTeachingAssignmentsReadsQueryFilters(t *testing.T) {
	svc := &fakeConfig{assignments: []client.TeachingAssignment{
		{ID: "asg-1", CycleID: "cyc-1", GroupID: "grp-1", SubjectID: "sub-1", TeacherID: "tch-1"},
	}}
	h := handler.NewConfigHandler(svc)
	rec := doReq(t, h.ListTeachingAssignments, http.MethodGet,
		"/api/v1/config/teaching-assignments?cycleId=cyc-1&groupId=grp-1&teacherId=tch-1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.asgCycle != "cyc-1" || svc.asgGroup != "grp-1" || svc.asgTeacher != "tch-1" {
		t.Errorf("filters = %q/%q/%q", svc.asgCycle, svc.asgGroup, svc.asgTeacher)
	}
	var out struct {
		TeachingAssignments []map[string]any `json:"teachingAssignments"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.TeachingAssignments) != 1 || out.TeachingAssignments[0]["cycleId"] != "cyc-1" {
		t.Errorf("body = %v", out)
	}
}

func (f *fakeConfig) ActivateCycle(_ context.Context, id string) (client.CycleResult, error) {
	f.activatedCycle = id
	if f.err != nil {
		return client.CycleResult{}, f.err
	}
	return client.CycleResult{ID: id, Name: "2026-2027", Status: "active"}, nil
}

// --- school calendar (M16) -------------------------------------------------

// A single-day holiday is entered once, not twice. Making a person type the
// same date in two fields is how a calendar stops getting maintained.
func TestConfigSingleDayHolidayNeedsOnlyOneDate(t *testing.T) {
	svc := &fakeConfig{}
	h := handler.NewConfigHandler(svc)

	rec := doReq(t, h.CreateNonWorkingPeriod, http.MethodPost, "/api/v1/config/calendar",
		`{"cycleId":"c1","label":"Independencia","kind":"holiday","startsOn":"2026-09-16"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if svc.periodIn.EndsOn != "2026-09-16" {
		t.Errorf("endsOn forwarded as %q, want the start date filled in", svc.periodIn.EndsOn)
	}
}

func TestConfigListNonWorkingPeriodsScopesToTheCycle(t *testing.T) {
	svc := &fakeConfig{periods: []client.NonWorkingPeriod{{ID: "per-1", Label: "Independencia"}}}
	h := handler.NewConfigHandler(svc)

	rec := doReq(t, h.ListNonWorkingPeriods, http.MethodGet, "/api/v1/config/calendar?cycleId=c1", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.periodsCycle != "c1" {
		t.Errorf("forwarded cycle = %q, want c1", svc.periodsCycle)
	}
}

// A one-day event is entered once. Same reasoning as a single-day holiday.
func TestConfigSingleDayEventNeedsOnlyOneDate(t *testing.T) {
	svc := &fakeConfig{}
	h := handler.NewConfigHandler(svc)

	rec := doReq(t, h.CreateSchoolEvent, http.MethodPost, "/api/v1/config/events",
		`{"cycleId":"c1","title":"Festival de primavera","startsOn":"2026-09-20"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if svc.eventIn.EndsOn != "2026-09-20" {
		t.Errorf("endsOn forwarded as %q, want the start date filled in", svc.eventIn.EndsOn)
	}
}

// Task 12: the school's details. "Not set" is 200 with found=false (the screen
// shows an empty form, not an error), and a save forwards every field.
func TestConfigSchoolProfile(t *testing.T) {
	svc := &fakeConfig{}
	h := handler.NewConfigHandler(svc)

	rec := doReq(t, h.GetSchoolProfile, http.MethodGet, "/api/v1/config/school-profile", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"found":false`) {
		t.Fatalf("not set: %d %s", rec.Code, rec.Body.String())
	}

	rec = doReq(t, h.PutSchoolProfile, http.MethodPut, "/api/v1/config/school-profile",
		`{"name":"Colegio García Lorca","legalName":"EGL S.C.","rfc":"EGL150101AB1","cct":"09PPR1234X","address":"Centro","phone":"5512345678","legend":"Gracias"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("save: %d %s", rec.Code, rec.Body.String())
	}
	want := client.SchoolProfile{Name: "Colegio García Lorca", LegalName: "EGL S.C.", RFC: "EGL150101AB1", CCT: "09PPR1234X", Address: "Centro", Phone: "5512345678", Legend: "Gracias"}
	if svc.profileIn != want {
		t.Errorf("forwarded %+v, want %+v", svc.profileIn, want)
	}

	rec = doReq(t, h.PutSchoolProfile, http.MethodPut, "/api/v1/config/school-profile", `{not json`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad body: %d, want 400", rec.Code)
	}
}
