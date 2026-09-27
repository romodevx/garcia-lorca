package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// A term belongs to a cycle AND a level. The teacher of a Primaria group must
// see only Primaria's terms of that cycle, never Secundaria's (T2 QA finding).
func levelTermsConfig() *fakeTeachingConfig {
	return termsCatalog(&fakeTeachingConfig{
		teachers: []client.Teacher{{ID: "tch-1", UserID: "usr-t"}},
		assignmentsByTeacher: map[string][]client.TeachingAssignment{
			"tch-1": {
				{ID: "asg-p", CycleID: "cyc-1", GroupID: "grp-p", TeacherID: "tch-1"},
				{ID: "asg-s", CycleID: "cyc-1", GroupID: "grp-s", TeacherID: "tch-1"},
				{ID: "asg-ghost", CycleID: "cyc-1", GroupID: "grp-unknown", TeacherID: "tch-1"},
			},
		},
		termsByCycle: map[string][]client.TermResult{
			"cyc-1": {
				{ID: "trm-p1", CycleID: "cyc-1", LevelID: "lvl-p", Name: "Primaria 1"},
				{ID: "trm-s1", CycleID: "cyc-1", LevelID: "lvl-s", Name: "Secundaria 1"},
				{ID: "trm-x1", CycleID: "cyc-1", LevelID: "lvl-x", Name: "Otro nivel"},
			},
		},
	})
}

func termIDs(t *testing.T, rec *httptest.ResponseRecorder) []string {
	t.Helper()
	var out struct {
		Terms []struct {
			ID      string `json:"id"`
			LevelID string `json:"levelId"`
		} `json:"terms"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	ids := []string{}
	for _, tm := range out.Terms {
		ids = append(ids, tm.ID)
	}
	return ids
}

func TestTeachingTermsOnlyTheGroupsLevels(t *testing.T) {
	config := levelTermsConfig()
	h := handler.NewTeachingHandler(config, &fakeTeachingGrades{}, &fakeTeachingStudents{})
	rec := httptest.NewRecorder()
	h.Terms(rec, teacherGET("/api/v1/teaching/terms", "usr-t"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.ElementsMatch(t, []string{"trm-p1", "trm-s1"}, termIDs(t, rec),
		"the levels the caller teaches, never another level's; a group whose level is unknown lists nothing")
	assert.NotContains(t, config.gotTermsLevels, "", "never an unfiltered (all levels) terms read")
}

func TestTeachingTermsForOneAssignment(t *testing.T) {
	config := levelTermsConfig()
	h := handler.NewTeachingHandler(config, &fakeTeachingGrades{}, &fakeTeachingStudents{})

	rec := httptest.NewRecorder()
	h.Terms(rec, teacherGET("/api/v1/teaching/terms?assignmentId=asg-p", "usr-t"))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"trm-p1"}, termIDs(t, rec))

	rec = httptest.NewRecorder()
	h.Terms(rec, teacherGET("/api/v1/teaching/terms?assignmentId=asg-of-someone-else", "usr-t"))
	assert.Equal(t, http.StatusNotFound, rec.Code, "an assignment the caller does not teach is 404")
}

// --- director ----------------------------------------------------------------

type fakeAssignmentTerms struct {
	*fakeTeachingConfig
	assignments map[string]client.TeachingAssignment
}

func (f *fakeAssignmentTerms) GetTeachingAssignment(_ context.Context, id string) (client.TeachingAssignment, error) {
	a, ok := f.assignments[id]
	if !ok {
		return client.TeachingAssignment{}, connect.NewError(connect.CodeNotFound, errString("no assignment"))
	}
	return a, nil
}

func TestGradesTermsForAssignmentOnlyItsLevel(t *testing.T) {
	cfg := &fakeAssignmentTerms{fakeTeachingConfig: levelTermsConfig(), assignments: map[string]client.TeachingAssignment{
		"asg-s": {ID: "asg-s", CycleID: "cyc-1", GroupID: "grp-s"},
	}}
	h := handler.NewGradesTermsHandler(cfg)

	rec := httptest.NewRecorder()
	h.Terms(rec, httptest.NewRequest(http.MethodGet, "/api/v1/grades/terms?assignmentId=asg-s", nil))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, []string{"trm-s1"}, termIDs(t, rec))

	rec = httptest.NewRecorder()
	h.Terms(rec, httptest.NewRequest(http.MethodGet, "/api/v1/grades/terms?assignmentId=ghost", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)

	rec = httptest.NewRecorder()
	h.Terms(rec, httptest.NewRequest(http.MethodGet, "/api/v1/grades/terms", nil))
	assert.Equal(t, http.StatusBadRequest, rec.Code, "assignmentId is required")
}
