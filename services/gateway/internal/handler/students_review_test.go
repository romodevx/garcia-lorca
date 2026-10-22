package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// --- PLAN-import-avisos on the students JSON (module_import_review) ----------

func reviewFlag(on bool) handler.StudentsOption {
	return handler.WithImportReview(func(context.Context) bool { return on })
}

func draftPupil() client.StudentResult {
	return client.StudentResult{ID: "stu-1", FirstName: "ANA", LastName: "SOTO", AdmissionStatus: "draft",
		OpenIssues: 2, Draft: true}
}

func decodeReviewObject(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func getStudent(h *handler.StudentsHandler) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/students/{id}", h.Get)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/students/stu-1", nil))
	return rec
}

func TestStudentJSONShowsImportIssuesOnlyWithTheReview(t *testing.T) {
	cases := []struct {
		name       string
		on         bool
		wantStatus string
		wantKeys   bool
	}{
		{"on", true, "draft", true},
		{"off: as before the review", false, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeCreator{res: draftPupil(), list: []client.StudentResult{draftPupil()}}
			h := handler.NewStudentsHandler(svc, reviewFlag(tc.on))

			one := decodeReviewObject(t, getStudent(h))
			listRec := doReq(t, h.List, http.MethodGet, "/api/v1/students", "")
			var list struct {
				Students []map[string]any `json:"students"`
			}
			if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil || len(list.Students) != 1 {
				t.Fatalf("list = %s (%v)", listRec.Body.String(), err)
			}

			for where, body := range map[string]map[string]any{"get": one, "list": list.Students[0]} {
				if body["admissionStatus"] != tc.wantStatus {
					t.Errorf("%s: admissionStatus = %v, want %q", where, body["admissionStatus"], tc.wantStatus)
				}
				_, hasIssues := body["openIssues"]
				_, hasDraft := body["draft"]
				if hasIssues != tc.wantKeys || hasDraft != tc.wantKeys {
					t.Errorf("%s: openIssues present %v, draft present %v, want %v", where, hasIssues, hasDraft, tc.wantKeys)
				}
				if tc.wantKeys && (body["openIssues"] != float64(2) || body["draft"] != true) {
					t.Errorf("%s: body = %v", where, body)
				}
			}
		})
	}
}

// Off, ?status=draft filters nothing, as an unknown status always did.
func TestStudentListDraftFilterNeedsTheReview(t *testing.T) {
	for _, tc := range []struct {
		on   bool
		want string
	}{{true, "draft"}, {false, ""}} {
		svc := &fakeCreator{}
		h := handler.NewStudentsHandler(svc, reviewFlag(tc.on))
		doReq(t, h.List, http.MethodGet, "/api/v1/students?status=draft", "")
		if svc.gotListStat != tc.want {
			t.Errorf("on=%v: status sent = %q, want %q", tc.on, svc.gotListStat, tc.want)
		}
	}
}

func putStudent(h *handler.StudentsHandler, userID string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v1/students/{id}", h.Update)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/students/stu-1",
		strings.NewReader(`{"firstName":"ANA","lastName":"SOTO","maternalLastName":"","curp":"SOXA120101MJCRRNA1","profile":{}}`))
	req = req.WithContext(middleware.WithPrincipal(req.Context(), middleware.Principal{UserID: userID}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// The save names who made it (students records it on the issues it closes)
// and, with the review, says whether it promoted a draft and where the import
// meant the pupil to go.
func TestUpdateStudentCarriesTheActorAndThePromotion(t *testing.T) {
	promoted := client.StudentResult{ID: "stu-1", AdmissionStatus: "pending", PromotedFromDraft: true, IntendedGroupID: "grp-1"}
	for _, tc := range []struct {
		on       bool
		wantKeys bool
	}{{true, true}, {false, false}} {
		svc := &fakeCreator{res: promoted}
		body := decodeReviewObject(t, putStudent(handler.NewStudentsHandler(svc, reviewFlag(tc.on)), "usr-7"))

		if svc.gotUpdate.ActorUserID != "usr-7" {
			t.Errorf("on=%v: actor = %q, want usr-7", tc.on, svc.gotUpdate.ActorUserID)
		}
		_, hasPromoted := body["promotedFromDraft"]
		_, hasGroup := body["intendedGroupId"]
		if hasPromoted != tc.wantKeys || hasGroup != tc.wantKeys {
			t.Errorf("on=%v: body = %v", tc.on, body)
		}
		if tc.wantKeys && (body["promotedFromDraft"] != true || body["intendedGroupId"] != "grp-1") {
			t.Errorf("on=%v: body = %v", tc.on, body)
		}
	}
}

// --- the import issues of a pupil --------------------------------------------

type fakeIssues struct {
	gotStudent, gotIssue, gotUser string
	gotResolved                   bool
	issues                        []client.StudentImportIssue
	err                           error
}

func (f *fakeIssues) ListImportIssues(_ context.Context, studentID string, includeResolved bool) ([]client.StudentImportIssue, error) {
	f.gotStudent, f.gotResolved = studentID, includeResolved
	return f.issues, f.err
}

func (f *fakeIssues) AcceptImportIssue(_ context.Context, studentID, issueID, userID string) (client.StudentImportIssue, error) {
	f.gotStudent, f.gotIssue, f.gotUser = studentID, issueID, userID
	if f.err != nil {
		return client.StudentImportIssue{}, f.err
	}
	return f.issues[0], nil
}

func issuesMux(h *handler.ImportIssuesHandler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/students/{id}/import-issues", h.List)
	mux.HandleFunc("POST /api/v1/students/{id}/import-issues/{issueId}/accept", h.Accept)
	return mux
}

func serveIssues(h *handler.ImportIssuesHandler, method, target, userID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req = req.WithContext(middleware.WithPrincipal(req.Context(), middleware.Principal{UserID: userID}))
	rec := httptest.NewRecorder()
	issuesMux(h).ServeHTTP(rec, req)
	return rec
}

var openIssue = client.StudentImportIssue{
	ID: "iss-1", StudentID: "stu-1", Field: "phone_mobile", Code: "phone_invalid", OriginalValue: "55 1234",
	CreatedAt: time.Date(2026, 10, 21, 15, 0, 0, 0, time.UTC),
}

func TestImportIssuesListTheirFields(t *testing.T) {
	resolved := openIssue
	resolved.ID, resolved.Resolution, resolved.ResolvedAt = "iss-2", "accepted", time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	resolved.Field, resolved.IntendedGroupID = "curp", "grp-1"
	svc := &fakeIssues{issues: []client.StudentImportIssue{openIssue, resolved}}

	rec := serveIssues(handler.NewImportIssuesHandler(svc), http.MethodGet, "/api/v1/students/stu-1/import-issues?includeResolved=true", "usr-1")

	body := decodeReviewObject(t, rec)
	if svc.gotStudent != "stu-1" || !svc.gotResolved {
		t.Errorf("forwarded %q resolved=%v", svc.gotStudent, svc.gotResolved)
	}
	list, _ := body["issues"].([]any)
	if len(list) != 2 {
		t.Fatalf("issues = %v", body)
	}
	first, second := list[0].(map[string]any), list[1].(map[string]any)
	want := map[string]any{"id": "iss-1", "studentId": "stu-1", "field": "phone_mobile", "code": "phone_invalid",
		"originalValue": "55 1234", "storedValue": "", "createdAt": "2026-10-21T15:00:00Z", "resolution": ""}
	for k, v := range want {
		if first[k] != v {
			t.Errorf("open issue %s = %v, want %v", k, first[k], v)
		}
	}
	if _, has := first["resolvedAt"]; has {
		t.Errorf("an open issue carries resolvedAt: %v", first)
	}
	if second["resolvedAt"] != "2026-10-22T09:00:00Z" || second["resolution"] != "accepted" || second["intendedGroupId"] != "grp-1" {
		t.Errorf("resolved issue = %v", second)
	}
}

func TestImportIssuesListRefusesAMalformedFilter(t *testing.T) {
	svc := &fakeIssues{}
	rec := serveIssues(handler.NewImportIssuesHandler(svc), http.MethodGet, "/api/v1/students/stu-1/import-issues?includeResolved=maybe", "usr-1")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	empty := serveIssues(handler.NewImportIssuesHandler(&fakeIssues{}), http.MethodGet, "/api/v1/students/stu-1/import-issues", "usr-1")
	if body := decodeReviewObject(t, empty); body["issues"] == nil {
		t.Errorf("no issues must read as an empty list: %v", body)
	}
}

func TestAcceptImportIssueIsByTheSignedInCaller(t *testing.T) {
	accepted := openIssue
	accepted.Resolution, accepted.ResolvedAt = "accepted", time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	svc := &fakeIssues{issues: []client.StudentImportIssue{accepted}}

	body := decodeReviewObject(t, serveIssues(handler.NewImportIssuesHandler(svc), http.MethodPost,
		"/api/v1/students/stu-1/import-issues/iss-1/accept", "usr-7"))

	if svc.gotStudent != "stu-1" || svc.gotIssue != "iss-1" || svc.gotUser != "usr-7" {
		t.Errorf("forwarded %q %q %q", svc.gotStudent, svc.gotIssue, svc.gotUser)
	}
	if body["resolution"] != "accepted" {
		t.Errorf("body = %v", body)
	}
}

func draftRefusal() error {
	ce := connect.NewError(connect.CodeFailedPrecondition, errors.New("ese alumno es un borrador sin CURP"))
	ce.Meta().Set("X-Error-Code", "STUDENT_DRAFT")
	return ce
}

func assertDraftRefusal(t *testing.T, where string, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusConflict || rec.Header().Get("X-Error-Code") != "STUDENT_DRAFT" {
		t.Errorf("%s: status %d code %q body %q, want 409 STUDENT_DRAFT", where, rec.Code, rec.Header().Get("X-Error-Code"), rec.Body.String())
	}
}

// A draft's curp issue closes only with a CURP; students says so.
func TestAcceptImportIssueRelaysTheDraftRefusal(t *testing.T) {
	svc := &fakeIssues{err: draftRefusal()}
	assertDraftRefusal(t, "accept", serveIssues(handler.NewImportIssuesHandler(svc), http.MethodPost,
		"/api/v1/students/stu-1/import-issues/iss-1/accept", "usr-7"))
}

// students refuses to place or link a draft; the edge relays its code.
func TestDraftRefusalsReachTheScreenWithTheirCode(t *testing.T) {
	enroll := handler.NewEnrollmentsHandler(&fakeEnrollmentSvc{err: draftRefusal()})
	assertDraftRefusal(t, "enroll", doReq(t, enroll.Enroll, http.MethodPost, "/api/v1/enrollments",
		`{"studentId":"stu-1","groupId":"grp-1"}`))
	assertDraftRefusal(t, "transfer", doReq(t, enroll.Transfer, http.MethodPost, "/api/v1/enrollments/transfer",
		`{"enrollmentId":"enr-1","toGroupId":"grp-2","reason":"x"}`))
	guardians := handler.NewGuardiansHandler(&fakeGuardians{err: draftRefusal()})
	assertDraftRefusal(t, "link", doReq(t, guardians.LinkGuardian, http.MethodPost, "/api/v1/guardians",
		`{"guardianUserId":"usr-9","studentId":"stu-1","relationship":"mother"}`))
}

// iam does not know the pupils, so the edge refuses a draft BEFORE iam is
// asked: no account is minted and no invitation is mailed for a pupil the
// school cannot link a family to.
func TestInviteRefusesADraftBeforeAskingIAM(t *testing.T) {
	iam := issuerWithGuardianRole()
	students := granterWithContact(plainContact())
	students.student = client.StudentResult{ID: "stu-1", AdmissionStatus: "draft", Draft: true}

	rec := doInvite(t, handler.NewContactAccessHandler(iam, students), "stu-1", "gl-1")

	assertDraftRefusal(t, "invite", rec)
	if iam.invitedEmail != "" || students.grantContactID != "" {
		t.Errorf("iam asked (%q) or grant made (%q) for a draft", iam.invitedEmail, students.grantContactID)
	}
}

func TestAcceptImportIssueNeedsASignedInCaller(t *testing.T) {
	svc := &fakeIssues{issues: []client.StudentImportIssue{openIssue}}
	mux := issuesMux(handler.NewImportIssuesHandler(svc))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/students/stu-1/import-issues/iss-1/accept", nil))
	if rec.Code != http.StatusUnauthorized || svc.gotIssue != "" {
		t.Errorf("status = %d, forwarded %q; want 401 and nothing asked", rec.Code, svc.gotIssue)
	}
}
