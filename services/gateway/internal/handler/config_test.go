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
	cycleIn client.CreateCycleInput
	cycles  []client.CycleResult

	levelIn client.CreateLevelInput
	levels  []client.LevelResult

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

	err error
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
