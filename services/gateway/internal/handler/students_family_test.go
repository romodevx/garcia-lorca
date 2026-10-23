package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// ADR 0041 (T3): the students JSON carries each pupil's family, and the
// directory filters by it.

type familyRoster struct {
	all       []client.StudentResult
	listCalls int

	gotFamily, gotStatus string
	familyCalls          int
}

func (f *familyRoster) CreateApplicant(context.Context, client.CreateApplicantInput) (client.StudentResult, error) {
	return client.StudentResult{}, nil
}

func (f *familyRoster) ListStudents(context.Context, string) ([]client.StudentResult, error) {
	f.listCalls++
	return f.all, nil
}

func (f *familyRoster) GetStudent(_ context.Context, id string) (client.StudentResult, error) {
	return client.StudentResult{ID: id, FamilyID: famID}, nil
}

func (f *familyRoster) UpdateStudent(context.Context, client.UpdateStudentInput) (client.StudentResult, error) {
	return client.StudentResult{}, nil
}

func (f *familyRoster) ListStudentsInFamily(_ context.Context, familyID, status string) ([]client.StudentResult, error) {
	f.familyCalls++
	f.gotFamily, f.gotStatus = familyID, status
	return []client.StudentResult{{ID: stuA, FamilyID: familyID}}, nil
}

func serveStudents(t *testing.T, roster *familyRoster, path string) *httptest.ResponseRecorder {
	t.Helper()
	return serveStudentsWith(t, roster, path, true)
}

// serveStudentsWith serves with module_families on or off.
func serveStudentsWith(t *testing.T, roster *familyRoster, path string, familiesOn bool) *httptest.ResponseRecorder {
	t.Helper()
	on := func(context.Context) bool { return familiesOn }
	h := handler.NewStudentsHandler(roster, handler.WithFamilies(on, roster))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/students", h.List)
	mux.HandleFunc("GET /api/v1/students/{id}", h.Get)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestStudentsJSONCarriesTheFamily(t *testing.T) {
	roster := &familyRoster{all: []client.StudentResult{{ID: stuA, FamilyID: famID}, {ID: stuB}}}

	rec := serveStudents(t, roster, "/api/v1/students")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var list struct {
		Students []map[string]any `json:"students"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list.Students, 2)
	assert.Equal(t, famID, list.Students[0]["familyId"])
	_, present := list.Students[1]["familyId"]
	assert.False(t, present, "a pupil without a family carries no familyId")

	rec = serveStudents(t, roster, "/api/v1/students/"+stuA)
	var one map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &one))
	assert.Equal(t, famID, one["familyId"])
}

func TestStudentsListFiltersByFamily(t *testing.T) {
	roster := &familyRoster{}
	rec := serveStudents(t, roster, "/api/v1/students?status=enrolled&familyId="+strings.ToUpper(famID))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 1, roster.familyCalls)
	assert.Zero(t, roster.listCalls, "a family filter must never read the whole school")
	assert.Equal(t, famID, roster.gotFamily)
	assert.Equal(t, "enrolled", roster.gotStatus)
}

func TestStudentsListRefusesAMalformedFamilyId(t *testing.T) {
	roster := &familyRoster{}
	rec := serveStudents(t, roster, "/api/v1/students?familyId=fam-1")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Zero(t, roster.familyCalls+roster.listCalls)
}

// Security L2: with module_families off the students JSON says nothing about
// families, and the filter is refused rather than ignored — ignoring it would
// answer the whole school to a screen that asked for one family.
func TestStudentsFamilyFieldsHideWhileFamiliesIsOff(t *testing.T) {
	roster := &familyRoster{all: []client.StudentResult{{ID: stuA, FamilyID: famID}}}

	for _, path := range []string{"/api/v1/students", "/api/v1/students/" + stuA} {
		rec := serveStudentsWith(t, roster, path, false)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.NotContains(t, rec.Body.String(), "familyId", path)
	}

	rec := serveStudentsWith(t, roster, "/api/v1/students?familyId="+famID, false)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "families are not enabled")
	assert.Zero(t, roster.familyCalls)
}

// A handler built without the families option behaves as module_families off.
func TestStudentsWithoutTheFamiliesOptionIsFamiliesOff(t *testing.T) {
	roster := &familyRoster{all: []client.StudentResult{{ID: stuA, FamilyID: famID}}}
	h := handler.NewStudentsHandler(roster)
	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/api/v1/students", nil))
	assert.NotContains(t, rec.Body.String(), "familyId")

	rec = httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/api/v1/students?familyId="+famID, nil))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 1, roster.listCalls, "only the unfiltered list read the roster")
}
