package handler_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// ADR 0041 (T3): the families edge. These tests drive the handler through a
// mux with the production patterns; the module guards and permissions are
// covered through the real router in cmd/families_routes_test.go.

const (
	famID  = "0f1c0e2a-0000-4000-8000-0000000000f1"
	stuA   = "0f1c0e2a-0000-4000-8000-0000000000a1"
	stuB   = "0f1c0e2a-0000-4000-8000-0000000000b1"
	stuC   = "0f1c0e2a-0000-4000-8000-0000000000c1"
	userID = "0f1c0e2a-0000-4000-8000-0000000000e1"
)

// fakeFamilies is the students surface of the families edge.
type fakeFamilies struct {
	family   client.Family
	families []client.Family
	students []client.StudentResult
	links    map[string][]client.GuardianLink
	err      error
	// removeDissolves answers RemoveStudentFromFamily with a dissolved family.
	removeDissolves bool

	// mu guards calls: the adults route reads each pupil's contacts
	// concurrently.
	mu           sync.Mutex
	calls        []string
	byIDsCalls   [][]string
	gotName      string
	gotFamilyID  string
	gotStudentID string
	gotIDs       []string
	gotUserID    string
	gotContact   client.FamilyContactInput
	gotGuardian  string
}

func (f *fakeFamilies) record(call string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	return f.err
}

func (f *fakeFamilies) ListFamilies(context.Context) ([]client.Family, error) {
	return f.families, f.record("ListFamilies")
}

func (f *fakeFamilies) GetFamily(_ context.Context, id string) (client.Family, error) {
	f.gotFamilyID = id
	return f.family, f.record("GetFamily")
}

func (f *fakeFamilies) CreateFamily(_ context.Context, name string, ids []string) (client.Family, error) {
	f.gotName, f.gotIDs = name, ids
	return f.family, f.record("CreateFamily")
}

func (f *fakeFamilies) RenameFamily(_ context.Context, id, name string) (client.Family, error) {
	f.gotFamilyID, f.gotName = id, name
	return f.family, f.record("RenameFamily")
}

func (f *fakeFamilies) AddStudentsToFamily(_ context.Context, id string, ids []string) (client.Family, error) {
	f.gotFamilyID, f.gotIDs = id, ids
	return f.family, f.record("AddStudentsToFamily")
}

func (f *fakeFamilies) RemoveStudentFromFamily(_ context.Context, id, studentID string) (client.Family, bool, error) {
	f.gotFamilyID, f.gotStudentID = id, studentID
	if err := f.record("RemoveStudentFromFamily"); err != nil {
		return client.Family{}, false, err
	}
	if f.removeDissolves {
		return client.Family{}, false, nil
	}
	return f.family, true, nil
}

func (f *fakeFamilies) DissolveFamily(_ context.Context, id string) error {
	f.gotFamilyID = id
	return f.record("DissolveFamily")
}

func (f *fakeFamilies) SuggestFamilies(context.Context) ([]client.FamilySuggestion, error) {
	return []client.FamilySuggestion{{
		Signature: "sig", ProposedName: "PÉREZ RUIZ", StudentIDs: []string{stuA, stuB},
		Reasons: []string{"same_surnames"}, Confidence: "medium",
	}}, f.record("SuggestFamilies")
}

func (f *fakeFamilies) DismissFamilySuggestion(_ context.Context, ids []string, userID string) error {
	f.gotIDs, f.gotUserID = ids, userID
	return f.record("DismissFamilySuggestion")
}

func (f *fakeFamilies) AddFamilyContact(_ context.Context, in client.FamilyContactInput) ([]client.GuardianLink, error) {
	f.gotContact = in
	return []client.GuardianLink{{ID: "c-1", StudentID: stuA, FullName: in.FullName}}, f.record("AddFamilyContact")
}

func (f *fakeFamilies) RevokeFamilyAccess(_ context.Context, id, guardianUserID string) (int, error) {
	f.gotFamilyID, f.gotGuardian = id, guardianUserID
	return 2, f.record("RevokeFamilyAccess")
}

func (f *fakeFamilies) StudentsByIDs(_ context.Context, ids []string) ([]client.StudentResult, error) {
	f.byIDsCalls = append(f.byIDsCalls, ids)
	return f.students, f.record("StudentsByIDs")
}

func (f *fakeFamilies) ListGuardiansForStudent(_ context.Context, studentID string) ([]client.GuardianLink, error) {
	return f.links[studentID], f.record("ListGuardiansForStudent")
}

func twoPupilFamily() client.Family {
	return client.Family{ID: famID, Name: "GARCÍA LÓPEZ", StudentIDs: []string{stuA, stuB}, AccountCount: 1, CreatedAt: "2026-10-20T15:04:05Z"}
}

// serveFamilies routes one request through the production patterns.
func serveFamilies(t *testing.T, svc *fakeFamilies, method, path, body string, principal *middleware.Principal) *httptest.ResponseRecorder {
	t.Helper()
	h := handler.NewFamiliesHandler(svc)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/families", h.List)
	mux.HandleFunc("POST /api/v1/families", h.Create)
	mux.HandleFunc("GET /api/v1/families/suggestions", h.Suggestions)
	mux.HandleFunc("POST /api/v1/families/suggestions/dismiss", h.DismissSuggestion)
	mux.HandleFunc("GET /api/v1/families/{familyId}", h.Get)
	mux.HandleFunc("PATCH /api/v1/families/{familyId}", h.Rename)
	mux.HandleFunc("DELETE /api/v1/families/{familyId}", h.Dissolve)
	mux.HandleFunc("GET /api/v1/families/{familyId}/contacts", h.Contacts)
	mux.HandleFunc("POST /api/v1/families/{familyId}/contacts", h.AddContact)
	mux.HandleFunc("POST /api/v1/families/{familyId}/students", h.AddStudents)
	mux.HandleFunc("DELETE /api/v1/families/{familyId}/students/{studentId}", h.RemoveStudent)
	mux.HandleFunc("POST /api/v1/families/{familyId}/access/revoke", h.RevokeAccess)
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if principal != nil {
		req = req.WithContext(middleware.WithPrincipal(req.Context(), *principal))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), dst), rec.Body.String())
}

func TestListFamiliesAnswersEachFamily(t *testing.T) {
	svc := &fakeFamilies{families: []client.Family{twoPupilFamily()}}
	rec := serveFamilies(t, svc, http.MethodGet, "/api/v1/families", "", nil)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out struct {
		Families []map[string]any `json:"families"`
	}
	decodeBody(t, rec, &out)
	require.Len(t, out.Families, 1)
	assert.Equal(t, map[string]any{
		"id": famID, "name": "GARCÍA LÓPEZ", "studentIds": []any{stuA, stuB},
		"accountCount": float64(1), "createdAt": "2026-10-20T15:04:05Z",
	}, out.Families[0])
}

// An empty school answers an empty list, never null.
func TestListFamiliesWithNoneIsAnEmptyList(t *testing.T) {
	rec := serveFamilies(t, &fakeFamilies{}, http.MethodGet, "/api/v1/families", "", nil)
	assert.JSONEq(t, `{"families":[]}`, rec.Body.String())
}

// The detail reads every member in ONE ListStudents(ids) call, not one per
// pupil, and lists them in the family's order.
func TestGetFamilyReadsItsMembersInOneCall(t *testing.T) {
	svc := &fakeFamilies{
		family: twoPupilFamily(),
		students: []client.StudentResult{
			{ID: stuB, FirstName: "LUIS", LastName: "GARCÍA", MaternalLastName: "LÓPEZ", GradeGroup: "B", AdmissionStatus: "enrolled", FamilyID: famID},
			{ID: stuA, FirstName: "ANA", LastName: "GARCÍA", MaternalLastName: "LÓPEZ", GradeGroup: "A", AdmissionStatus: "draft", Draft: true, FamilyID: famID},
		},
	}
	rec := serveFamilies(t, svc, http.MethodGet, "/api/v1/families/"+famID, "", nil)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, [][]string{{stuA, stuB}}, svc.byIDsCalls)
	var out struct {
		ID      string           `json:"id"`
		Members []map[string]any `json:"members"`
	}
	decodeBody(t, rec, &out)
	assert.Equal(t, famID, out.ID)
	require.Len(t, out.Members, 2)
	assert.Equal(t, stuA, out.Members[0]["id"])
	assert.Equal(t, true, out.Members[0]["draft"])
	assert.Equal(t, "ANA", out.Members[0]["firstName"])
	assert.Equal(t, stuB, out.Members[1]["id"])
	assert.Equal(t, "enrolled", out.Members[1]["admissionStatus"])
	assert.Equal(t, "B", out.Members[1]["gradeGroup"])
}

func TestGetFamilyForwardsNotFound(t *testing.T) {
	svc := &fakeFamilies{err: connect.NewError(connect.CodeNotFound, nil)}
	rec := serveFamilies(t, svc, http.MethodGet, "/api/v1/families/"+famID, "", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// The family's adults, as people: contacts of different pupils that are the
// same person (same account, else same email, else same phone) are one
// entry, and the adults with the app come first.
func TestFamilyContactsGroupTheSamePersonAcrossPupils(t *testing.T) {
	family := twoPupilFamily()
	family.StudentIDs = []string{stuA, stuB, stuC}
	svc := &fakeFamilies{family: family, links: map[string][]client.GuardianLink{
		stuA: {
			{ID: "a-grandma", StudentID: stuA, FullName: "ROSA", Relationship: "other", Phone: "55 1234 5678"},
			{ID: "a-mother", StudentID: stuA, FullName: "ANA LÓPEZ", Relationship: "mother", Email: "ana@example.com", GuardianUserID: userID},
			{ID: "a-father", StudentID: stuA, FullName: "LUIS GARCÍA", Relationship: "father", Email: "luis@example.com", Phone: "5599990000"},
		},
		stuB: {
			{ID: "b-mother", StudentID: stuB, FullName: "ANA LOPEZ", Relationship: "mother", Email: "Ana@Example.com"},
			// Same home phone as the father, different email: another person.
			{ID: "b-aunt", StudentID: stuB, FullName: "EVA", Relationship: "other", Email: "eva@example.com", Phone: "5599990000"},
		},
		stuC: {
			{ID: "c-mother", StudentID: stuC, FullName: "ANA LÓPEZ", Relationship: "mother", Email: "ana@example.com", GuardianUserID: userID},
			{ID: "c-grandma", StudentID: stuC, FullName: "ROSA M.", Relationship: "other", Phone: "5512345678"},
		},
	}}
	rec := serveFamilies(t, svc, http.MethodGet, "/api/v1/families/"+famID+"/contacts", "", nil)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out struct {
		People []struct {
			FullName       string `json:"fullName"`
			GuardianUserID string `json:"guardianUserId"`
			HasApp         bool   `json:"hasApp"`
			Contacts       []struct {
				StudentID string `json:"studentId"`
				ContactID string `json:"contactId"`
			} `json:"contacts"`
		} `json:"people"`
	}
	decodeBody(t, rec, &out)
	require.Len(t, out.People, 4, rec.Body.String())

	mother := out.People[0]
	assert.True(t, mother.HasApp)
	assert.Equal(t, userID, mother.GuardianUserID)
	assert.Equal(t, "ANA LÓPEZ", mother.FullName)
	refs := map[string]string{}
	for _, c := range mother.Contacts {
		refs[c.StudentID] = c.ContactID
	}
	assert.Equal(t, map[string]string{stuA: "a-mother", stuB: "b-mother", stuC: "c-mother"}, refs)

	byName := map[string]int{}
	for _, p := range out.People[1:] {
		assert.False(t, p.HasApp)
		byName[p.FullName] = len(p.Contacts)
	}
	assert.Equal(t, map[string]int{"ROSA": 2, "LUIS GARCÍA": 1, "EVA": 1}, byName)
}

func TestFamilySuggestionsAnswerTheGroups(t *testing.T) {
	rec := serveFamilies(t, &fakeFamilies{}, http.MethodGet, "/api/v1/families/suggestions", "", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"suggestions":[{"signature":"sig","proposedName":"PÉREZ RUIZ","studentIds":["`+stuA+`","`+stuB+`"],"reasons":["same_surnames"],"confidence":"medium"}]}`, rec.Body.String())
}

// The dismissal is recorded for the signed-in caller; a userId in the body is
// ignored, whatever it says.
func TestDismissSuggestionIsByTheCallerNeverTheBody(t *testing.T) {
	svc := &fakeFamilies{}
	body := `{"studentIds":["` + stuA + `","` + stuB + `"],"userId":"someone-else","user_id":"someone-else"}`
	rec := serveFamilies(t, svc, http.MethodPost, "/api/v1/families/suggestions/dismiss", body, &middleware.Principal{UserID: userID})

	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Equal(t, userID, svc.gotUserID)
	assert.Equal(t, []string{stuA, stuB}, svc.gotIDs)
}

func TestDismissSuggestionWithoutAPrincipalIsUnauthorized(t *testing.T) {
	svc := &fakeFamilies{}
	rec := serveFamilies(t, svc, http.MethodPost, "/api/v1/families/suggestions/dismiss", `{"studentIds":["`+stuA+`","`+stuB+`"]}`, nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Empty(t, svc.calls)
}

func TestCreateFamilyAnswersTheNewFamily(t *testing.T) {
	svc := &fakeFamilies{family: twoPupilFamily()}
	// An upper-case id is the same pupil: it travels canonical.
	body := `{"name":"GARCÍA LÓPEZ","studentIds":["` + strings.ToUpper(stuA) + `","` + stuB + `"]}`
	rec := serveFamilies(t, svc, http.MethodPost, "/api/v1/families", body, nil)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Equal(t, "GARCÍA LÓPEZ", svc.gotName)
	assert.Equal(t, []string{stuA, stuB}, svc.gotIDs)
	var out map[string]any
	decodeBody(t, rec, &out)
	assert.Equal(t, famID, out["id"])
}

// studentIds is checked at the edge: present, at most 20, every one a uuid.
// The service is never asked about a request the contract refuses.
func TestFamilyStudentIdsAreValidatedAtTheEdge(t *testing.T) {
	ids := make([]string, 21)
	for i := range ids {
		ids[i] = `"` + stuA + `"`
	}
	tooMany := `{"studentIds":[` + strings.Join(ids, ",") + `]}`
	cases := map[string]struct{ method, path, body string }{
		"create without pupils":  {http.MethodPost, "/api/v1/families", `{"studentIds":[]}`},
		"create with 21 pupils":  {http.MethodPost, "/api/v1/families", tooMany},
		"create with a bad id":   {http.MethodPost, "/api/v1/families", `{"studentIds":["stu-1"]}`},
		"create malformed body":  {http.MethodPost, "/api/v1/families", `{"studentIds":`},
		"add with a bad id":      {http.MethodPost, "/api/v1/families/" + famID + "/students", `{"studentIds":["x"]}`},
		"add without pupils":     {http.MethodPost, "/api/v1/families/" + famID + "/students", `{}`},
		"dismiss a single pupil": {http.MethodPost, "/api/v1/families/suggestions/dismiss", `{"studentIds":["` + stuA + `"]}`},
		"dismiss 21 pupils":      {http.MethodPost, "/api/v1/families/suggestions/dismiss", tooMany},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc := &fakeFamilies{}
			rec := serveFamilies(t, svc, tc.method, tc.path, tc.body, &middleware.Principal{UserID: userID})
			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Empty(t, svc.calls)
		})
	}
}

// A path id that is not a uuid is refused before students is asked.
func TestFamilyPathIdsAreValidatedAtTheEdge(t *testing.T) {
	cases := map[string]struct{ method, path, body string }{
		"get":           {http.MethodGet, "/api/v1/families/fam-1", ""},
		"contacts":      {http.MethodGet, "/api/v1/families/fam-1/contacts", ""},
		"rename":        {http.MethodPatch, "/api/v1/families/fam-1", `{"name":"X"}`},
		"dissolve":      {http.MethodDelete, "/api/v1/families/fam-1", ""},
		"add":           {http.MethodPost, "/api/v1/families/fam-1/students", `{"studentIds":["` + stuA + `"]}`},
		"remove family": {http.MethodDelete, "/api/v1/families/fam-1/students/" + stuA, ""},
		"remove pupil":  {http.MethodDelete, "/api/v1/families/" + famID + "/students/stu-1", ""},
		"add contact":   {http.MethodPost, "/api/v1/families/fam-1/contacts", `{"fullName":"ANA"}`},
		"revoke family": {http.MethodPost, "/api/v1/families/fam-1/access/revoke", `{"guardianUserId":"` + userID + `"}`},
		"revoke user":   {http.MethodPost, "/api/v1/families/" + famID + "/access/revoke", `{"guardianUserId":"usr-1"}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc := &fakeFamilies{}
			rec := serveFamilies(t, svc, tc.method, tc.path, tc.body, nil)
			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Empty(t, svc.calls)
		})
	}
}

func TestRenameFamilyAnswersTheFamily(t *testing.T) {
	svc := &fakeFamilies{family: twoPupilFamily()}
	rec := serveFamilies(t, svc, http.MethodPatch, "/api/v1/families/"+famID, `{"name":"LOS GARCÍA"}`, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, famID, svc.gotFamilyID)
	assert.Equal(t, "LOS GARCÍA", svc.gotName)
}

func TestAddStudentsToFamilyAnswersTheFamily(t *testing.T) {
	svc := &fakeFamilies{family: twoPupilFamily()}
	rec := serveFamilies(t, svc, http.MethodPost, "/api/v1/families/"+famID+"/students", `{"studentIds":["`+stuC+`"]}`, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, []string{stuC}, svc.gotIDs)
}

func TestRemoveStudentFromFamily(t *testing.T) {
	svc := &fakeFamilies{family: twoPupilFamily()}
	rec := serveFamilies(t, svc, http.MethodDelete, "/api/v1/families/"+famID+"/students/"+stuB, "", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, stuB, svc.gotStudentID)

	// The last pupil leaving dissolves the family: nothing left to show.
	svc = &fakeFamilies{removeDissolves: true}
	rec = serveFamilies(t, svc, http.MethodDelete, "/api/v1/families/"+famID+"/students/"+stuB, "", nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestDissolveFamilyAnswersNoContent(t *testing.T) {
	svc := &fakeFamilies{}
	rec := serveFamilies(t, svc, http.MethodDelete, "/api/v1/families/"+famID, "", nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, famID, svc.gotFamilyID)
}

func TestAddFamilyContactRecordsTheAdultOnEveryPupil(t *testing.T) {
	svc := &fakeFamilies{}
	body := `{"relationship":"mother","fullName":"ANA LÓPEZ","phone":"5512345678","email":"ana@example.com","isEmergency":true}`
	rec := serveFamilies(t, svc, http.MethodPost, "/api/v1/families/"+famID+"/contacts", body, nil)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Equal(t, client.FamilyContactInput{
		FamilyID: famID, Relationship: "mother", FullName: "ANA LÓPEZ", Phone: "5512345678",
		Email: "ana@example.com", IsEmergency: true,
	}, svc.gotContact)
	var out struct {
		Contacts []map[string]any `json:"contacts"`
	}
	decodeBody(t, rec, &out)
	require.Len(t, out.Contacts, 1)
	assert.Equal(t, "c-1", out.Contacts[0]["id"])
}

func TestRevokeFamilyAccessAnswersHowManyContactsLostIt(t *testing.T) {
	svc := &fakeFamilies{}
	rec := serveFamilies(t, svc, http.MethodPost, "/api/v1/families/"+famID+"/access/revoke", `{"guardianUserId":"`+strings.ToUpper(userID)+`"}`, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, userID, svc.gotGuardian)
	assert.JSONEq(t, `{"contacts":2}`, rec.Body.String())
}

// The service's refusal reaches the screen with its sentence and its code:
// a third account is 409 FAMILY_ACCOUNT_LIMIT, like every FailedPrecondition.
func TestFamilyRefusalsForwardTheirCode(t *testing.T) {
	for _, code := range []string{"FAMILY_ACCOUNT_LIMIT", "FAMILY_BUSY", "FAMILY_MANAGED_ACCESS"} {
		t.Run(code, func(t *testing.T) {
			ce := connect.NewError(connect.CodeFailedPrecondition, errString("una familia tiene como máximo dos cuentas de la app"))
			ce.Meta().Set("X-Error-Code", code)
			svc := &fakeFamilies{err: ce}
			rec := serveFamilies(t, svc, http.MethodPost, "/api/v1/families/"+famID+"/students", `{"studentIds":["`+stuC+`"]}`, nil)
			assert.Equal(t, http.StatusConflict, rec.Code)
			assert.Equal(t, code, rec.Header().Get("X-Error-Code"))
			assert.Contains(t, rec.Body.String(), "como máximo dos cuentas")
		})
	}
}
