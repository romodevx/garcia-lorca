package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// PLAN-familias-importacion I2: with module_families on and guardians:write
// held, the import commit builds the batch's very likely families, best
// effort, and answers what it did under `families`. The undo route takes it
// back.

const (
	importBatchID   = "0f1c0e2a-0000-4000-8000-0000000000d1"
	importFamilyID  = "0f1c0e2a-0000-4000-8000-0000000000f1"
	importFamilyID2 = "0f1c0e2a-0000-4000-8000-0000000000f2"
	importPupilA    = "0f1c0e2a-0000-4000-8000-0000000000a1"
	importPupilB    = "0f1c0e2a-0000-4000-8000-0000000000b1"
	importOwnerID   = "0f1c0e2a-0000-4000-8000-0000000000e1"
	// studentsLeak is what a students refusal might say: it must never reach
	// the import's answer.
	studentsLeak = "GARCÍA TORRES could not join"
)

type fakeImportFamilies struct {
	build       client.FamilyImportBuild
	buildErr    error
	undo        client.FamilyImportUndo
	undoErr     error
	suggestions []client.FamilySuggestion
	suggestErr  error

	builtBatches  []string
	undoneBatches []string
	suggestCalls  int
}

func (f *fakeImportFamilies) BuildFamiliesFromImport(_ context.Context, batchID string) (client.FamilyImportBuild, error) {
	f.builtBatches = append(f.builtBatches, batchID)
	return f.build, f.buildErr
}

func (f *fakeImportFamilies) UndoFamiliesFromImport(_ context.Context, batchID string) (client.FamilyImportUndo, error) {
	f.undoneBatches = append(f.undoneBatches, batchID)
	return f.undo, f.undoErr
}

func (f *fakeImportFamilies) SuggestFamilies(context.Context) ([]client.FamilySuggestion, error) {
	f.suggestCalls++
	return f.suggestions, f.suggestErr
}

func builtFamilies() client.FamilyImportBuild {
	return client.FamilyImportBuild{
		Created: []client.Family{{ID: importFamilyID, Name: "GARCÍA TORRES", StudentIDs: []string{importPupilA, importPupilB}}},
		Joined:  []client.Family{{ID: importFamilyID2, Name: "PÉREZ RUIZ", StudentIDs: []string{importPupilB}}},
		Skipped: 2,
	}
}

func twoSuggestions() []client.FamilySuggestion {
	return []client.FamilySuggestion{{Signature: "s1"}, {Signature: "s2"}}
}

// familyImportOwner holds what the owner holds for this flow.
func familyImportOwner() middleware.Principal {
	return middleware.Principal{
		UserID: importOwnerID, TenantID: "t-1",
		Permissions: []string{"students:import", "guardians:write", "students:read"},
	}
}

func newFamilyImportFixture(families *fakeImportFamilies, on bool) importFixture {
	f := newImportFixture()
	f.h.d.Families = families
	f.h.d.FamiliesEnabled = func(context.Context) bool { return on }
	return f
}

// commitAs previews then commits one file as principal and returns the raw
// answer.
func commitAs(t *testing.T, f importFixture, principal middleware.Principal) *httptest.ResponseRecorder {
	t.Helper()
	req := importRequest{FileBase64: importFile(t, pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"))}
	req.PlanHash = decodePreview(t, post(t, f.h.Preview, req)).PlanHash
	raw, err := json.Marshal(req)
	require.NoError(t, err)
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
	r = r.WithContext(middleware.WithPrincipal(r.Context(), principal))
	rec := httptest.NewRecorder()
	f.h.Commit(rec, r)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return rec
}

// familiesOf decodes the `families` object; nil when the answer has none.
func familiesOf(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	fam, ok := body["families"]
	if !ok {
		return nil
	}
	out, ok := fam.(map[string]any)
	require.True(t, ok, "families = %v", fam)
	return out
}

func TestCommitBuildsTheBatchFamilies(t *testing.T) {
	families := &fakeImportFamilies{build: builtFamilies(), suggestions: twoSuggestions()}
	f := newFamilyImportFixture(families, true)

	rec := commitAs(t, f, familyImportOwner())

	assert.Equal(t, []string{"batch-1"}, families.builtBatches)
	var body struct {
		Families importFamilies `json:"families"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	toReview := 2
	assert.Equal(t, importFamilies{
		Built: true,
		Created: []importFamily{{
			ID: importFamilyID, Name: "GARCÍA TORRES", StudentIDs: []string{importPupilA, importPupilB},
		}},
		Joined:   []importFamily{{ID: importFamilyID2, Name: "PÉREZ RUIZ", StudentIDs: []string{importPupilB}}},
		Skipped:  2,
		ToReview: &toReview,
	}, body.Families)
}

func TestCommitWithNoFamilyToBuildAnswersEmptyLists(t *testing.T) {
	families := &fakeImportFamilies{}
	f := newFamilyImportFixture(families, true)

	got := familiesOf(t, commitAs(t, f, familyImportOwner()))

	assert.Equal(t, true, got["built"])
	assert.Equal(t, []any{}, got["created"])
	assert.Equal(t, []any{}, got["joined"])
	assert.Equal(t, float64(0), got["toReview"])
}

// Off, the import answers exactly as before and asks nothing about families.
func TestCommitWithFamiliesOffBuildsNothing(t *testing.T) {
	for name, enabled := range map[string]func(context.Context) bool{
		"flag off":  func(context.Context) bool { return false },
		"not wired": nil,
	} {
		t.Run(name, func(t *testing.T) {
			families := &fakeImportFamilies{build: builtFamilies()}
			f := newImportFixture()
			f.h.d.Families = families
			f.h.d.FamiliesEnabled = enabled

			got := familiesOf(t, commitAs(t, f, familyImportOwner()))

			assert.Nil(t, got)
			assert.Empty(t, families.builtBatches)
			assert.Zero(t, families.suggestCalls)
		})
	}
}

// Building a family hands its accounts to every pupil in it (ADR 0041, H1):
// without guardians:write the import does not build them.
func TestCommitWithoutGuardiansWriteBuildsNothing(t *testing.T) {
	families := &fakeImportFamilies{build: builtFamilies()}
	f := newFamilyImportFixture(families, true)
	importer := familyImportOwner()
	importer.Permissions = []string{"students:import", "students:read"}

	got := familiesOf(t, commitAs(t, f, importer))

	assert.Nil(t, got)
	assert.Empty(t, families.builtBatches)
}

// Names and pupils are reading the pupils: without students:read the answer
// keeps only the ids, enough to count.
func TestCommitFamiliesNeedStudentsReadForTheirNames(t *testing.T) {
	families := &fakeImportFamilies{build: builtFamilies()}
	f := newFamilyImportFixture(families, true)
	importer := familyImportOwner()
	importer.Permissions = []string{"students:import", "guardians:write"}

	rec := commitAs(t, f, importer)

	assert.NotContains(t, rec.Body.String(), "GARCÍA TORRES")
	assert.NotContains(t, rec.Body.String(), importPupilA)
	got := familiesOf(t, rec)
	assert.Equal(t, []any{map[string]any{"id": importFamilyID}}, got["created"])
	assert.Equal(t, []any{map[string]any{"id": importFamilyID2}}, got["joined"])
}

// A failed build never fails the import: built is false, what was built
// before the failure is still reported, and students' words stay out of the
// answer and the log.
func TestCommitSurvivesAFailedFamilyBuild(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	partial := builtFamilies()
	partial.Joined = nil
	families := &fakeImportFamilies{
		build:       partial,
		buildErr:    connect.NewError(connect.CodeFailedPrecondition, errors.New(studentsLeak)),
		suggestions: twoSuggestions(),
	}
	f := newFamilyImportFixture(families, true)

	rec := commitAs(t, f, familyImportOwner())

	got := familiesOf(t, rec)
	assert.Equal(t, false, got["built"])
	assert.Len(t, got["created"], 1)
	assert.Equal(t, []any{}, got["joined"])
	assert.Equal(t, float64(2), got["toReview"])
	assert.NotContains(t, rec.Body.String(), studentsLeak)
	assert.NotContains(t, logs.String(), studentsLeak)
	assert.NotContains(t, logs.String(), "GARCÍA TORRES")
	assert.Contains(t, logs.String(), "batch-1")
}

func TestCommitOmitsToReviewWhenSuggestionsFail(t *testing.T) {
	families := &fakeImportFamilies{build: builtFamilies(), suggestErr: errors.New("down")}
	f := newFamilyImportFixture(families, true)

	got := familiesOf(t, commitAs(t, f, familyImportOwner()))

	assert.Equal(t, true, got["built"])
	_, ok := got["toReview"]
	assert.False(t, ok, "toReview = %v", got["toReview"])
}

// The preview writes nothing, families included.
func TestPreviewBuildsNoFamily(t *testing.T) {
	families := &fakeImportFamilies{build: builtFamilies()}
	f := newFamilyImportFixture(families, true)
	req := importRequest{FileBase64: importFile(t, pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"))}

	rec := post(t, f.h.Preview, req)

	assert.Nil(t, familiesOf(t, rec))
	assert.Empty(t, families.builtBatches)
}

// --- undo ----------------------------------------------------------------------

const undoFamiliesPattern = "POST /api/v1/imports/students/{batchId}/families/undo"

func serveUndoFamilies(t *testing.T, families *fakeImportFamilies, batchID string, principal *middleware.Principal) *httptest.ResponseRecorder {
	t.Helper()
	f := newFamilyImportFixture(families, true)
	mux := http.NewServeMux()
	mux.HandleFunc(undoFamiliesPattern, f.h.UndoFamilies)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/imports/students/"+batchID+"/families/undo", nil)
	if principal != nil {
		r = r.WithContext(middleware.WithPrincipal(r.Context(), *principal))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	return rec
}

func TestUndoFamiliesAnswersTheCounts(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	families := &fakeImportFamilies{undo: client.FamilyImportUndo{Removed: 5, Dissolved: 2, KeptWithAccess: 1}}
	owner := familyImportOwner()

	rec := serveUndoFamilies(t, families, strings.ToUpper(importBatchID), &owner)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"removed":5,"dissolved":2,"keptWithAccess":1}`, rec.Body.String())
	assert.Equal(t, []string{importBatchID}, families.undoneBatches, "the batch is canonicalised at the edge")
	assert.Contains(t, logs.String(), importBatchID)
	assert.Contains(t, logs.String(), importOwnerID)
}

func TestUndoFamiliesValidatesTheBatchAtTheEdge(t *testing.T) {
	owner := familyImportOwner()
	for _, batch := range []string{"batch-1", "0f1c0e2a-0000-4000-8000", "%20"} {
		families := &fakeImportFamilies{}
		rec := serveUndoFamilies(t, families, batch, &owner)
		assert.Equal(t, http.StatusBadRequest, rec.Code, batch)
		assert.Empty(t, families.undoneBatches, batch)
	}
}

func TestUndoFamiliesForwardsTheRefusal(t *testing.T) {
	families := &fakeImportFamilies{undoErr: connect.NewError(connect.CodeInvalidArgument, errors.New("batch_id must be a uuid"))}
	owner := familyImportOwner()

	rec := serveUndoFamilies(t, families, importBatchID, &owner)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUndoFamiliesWithoutAPrincipalIsUnauthorized(t *testing.T) {
	families := &fakeImportFamilies{}

	rec := serveUndoFamilies(t, families, importBatchID, nil)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Empty(t, families.undoneBatches)
}

// The contract documents exactly what the import's families answer, and the
// undo route like every families write: session cookie, CSRF header, 400 for
// a malformed batch, 404 when a module is off.
func TestImportFamiliesAreInTheContract(t *testing.T) {
	doc := readFamilyContract(t)
	for schema, response := range map[string]any{
		"StudentImportFamilies":     importFamilies{},
		"StudentImportFamily":       importFamily{},
		"StudentImportFamiliesUndo": importFamiliesUndo{},
	} {
		s, ok := doc.Components.Schemas[schema]
		if !ok {
			t.Errorf("the %s schema is not documented", schema)
			continue
		}
		assert.Equal(t, jsonFields(response), propertyNames(s.Properties), schema)
	}
	if _, ok := doc.Components.Schemas["StudentImportResult"].Properties["families"]; !ok {
		t.Error("StudentImportResult has no families")
	}
	op, ok := doc.operation(t, "post", "/api/v1/imports/students/{batchId}/families/undo")
	require.True(t, ok, "the undo route is not documented")
	_, cookie := firstSecurity(op)["sessionCookie"]
	assert.True(t, cookie, "security = %v", op.Security)
	assert.True(t, refsParameter(t, op, "CsrfToken"), "a write must require the CSRF header")
	for _, code := range []string{"200", "400", "401", "403", "404"} {
		_, ok := op.Responses[code]
		assert.True(t, ok, "no %s response", code)
	}
}
