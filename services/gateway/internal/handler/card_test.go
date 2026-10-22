package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

type fakeCards struct {
	links     []client.GuardianLink
	student   client.StudentResult
	code      string
	suspended bool
	reissued  string
	photo     []byte
	profile   client.SchoolProfile
	cycles    []client.CycleResult
	verified  client.StudentResult
	valid     bool
	err       error
	revoked   string
}

func (f *fakeCards) ListStudentsForGuardian(_ context.Context, _ string) ([]client.GuardianLink, error) {
	return f.links, nil
}
func (f *fakeCards) GetStudent(_ context.Context, _ string) (client.StudentResult, error) {
	return f.student, nil
}
func (f *fakeCards) CardCode(_ context.Context, _ string) (string, bool, error) {
	return f.code, f.suspended, nil
}
func (f *fakeCards) ReissueCard(_ context.Context, id string) error {
	f.reissued = id
	return nil
}
func (f *fakeCards) RevokeCard(_ context.Context, id string) error {
	f.revoked = id
	return nil
}
func (f *fakeCards) VerifyCard(_ context.Context, _ string) (client.StudentResult, bool, error) {
	return f.verified, f.valid, f.err
}
func (f *fakeCards) GetStudentPhoto(_ context.Context, _ string) ([]byte, string, error) {
	if f.photo == nil {
		return nil, "", connect.NewError(connect.CodeNotFound, nil)
	}
	return f.photo, "image/jpeg", nil
}
func (f *fakeCards) GetSchoolProfile(_ context.Context) (client.SchoolProfile, bool, error) {
	return f.profile, f.profile.Name != "", nil
}
func (f *fakeCards) ListCycles(_ context.Context) ([]client.CycleResult, error) { return f.cycles, nil }

func cardHandler(f *fakeCards) *handler.CardHandler {
	return handler.NewCardHandler(f, f, f, f, f)
}

func asGuardian(req *http.Request, studentID string) *http.Request {
	req.SetPathValue("studentId", studentID)
	return req.WithContext(middleware.WithPrincipal(req.Context(), middleware.Principal{UserID: "guardian-1", Permissions: []string{"family:read"}}))
}

// Task 13: a family reads its own child's card — name, group, cycle,
// matrícula, the school, and the QR's opaque code.
func TestAFamilyReadsItsChildsCard(t *testing.T) {
	f := &fakeCards{
		links:   []client.GuardianLink{{StudentID: "stu-1"}},
		student: client.StudentResult{ID: "stu-1", FirstName: "Zoe", LastName: "Villalobos", Level: "Primaria", GradeGroup: "1° A", EnrollmentNumber: "2026-00042"},
		code:    "GLCARD:AQAAAAAAAAAAAAAAAAAAAA",
		profile: client.SchoolProfile{Name: "COLEGIO GARCÍA LORCA"},
		cycles:  []client.CycleResult{{Name: "2025-2026", Status: "closed"}, {Name: "2026-2027", Status: "active"}},
	}
	rec := httptest.NewRecorder()
	cardHandler(f).FamilyCard(rec, asGuardian(httptest.NewRequest(http.MethodGet, "/api/v1/family/students/stu-1/card", nil), "stu-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for key, want := range map[string]string{
		"name": "Villalobos Zoe", "gradeGroup": "1° A", "level": "Primaria", "cycle": "2026-2027",
		"enrollmentNumber": "2026-00042", "qr": "GLCARD:AQAAAAAAAAAAAAAAAAAAAA",
	} {
		if got[key] != want {
			t.Errorf("%s = %v, want %q", key, got[key], want)
		}
	}
	if school, _ := got["school"].(map[string]any); school["name"] != "COLEGIO GARCÍA LORCA" {
		t.Errorf("school = %v", got["school"])
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store: the code identifies a child", rec.Header().Get("Cache-Control"))
	}
}

// Another family's child is 404, for the card and for the photo: nothing says
// whether that child exists.
func TestAFamilyCannotReadAnotherChildsCard(t *testing.T) {
	f := &fakeCards{links: []client.GuardianLink{{StudentID: "stu-1"}}, code: "GLCARD:x", photo: []byte{0xff}}
	h := cardHandler(f)
	for name, serve := range map[string]http.HandlerFunc{"card": h.FamilyCard, "photo": h.FamilyPhoto} {
		rec := httptest.NewRecorder()
		serve(rec, asGuardian(httptest.NewRequest(http.MethodGet, "/api/v1/family/students/stu-9/"+name, nil), "stu-9"))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s of another child: %d, want 404", name, rec.Code)
		}
	}
}

func TestAFamilyGetsItsChildsPhoto(t *testing.T) {
	f := &fakeCards{links: []client.GuardianLink{{StudentID: "stu-1"}}, photo: []byte{0xff, 0xd8}}
	rec := httptest.NewRecorder()
	cardHandler(f).FamilyPhoto(rec, asGuardian(httptest.NewRequest(http.MethodGet, "/api/v1/family/students/stu-1/photo", nil), "stu-1"))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/jpeg" || rec.Header().Get("Cache-Control") != "private, no-store" {
		t.Errorf("photo: %d %q %q", rec.Code, rec.Header().Get("Content-Type"), rec.Header().Get("Cache-Control"))
	}
}

// Staff verify a scanned code: whose card, and whether it still counts.
func TestStaffVerifyACard(t *testing.T) {
	f := &fakeCards{verified: client.StudentResult{ID: "stu-1", FirstName: "Zoe", LastName: "Villalobos", GradeGroup: "1° A", EnrollmentNumber: "2026-00042", AdmissionStatus: "enrolled"}, valid: true}
	rec := httptest.NewRecorder()
	cardHandler(f).Verify(rec, httptest.NewRequest(http.MethodPost, "/api/v1/students/card/verify", strings.NewReader(`{"code":"GLCARD:AQAAAAAAAAAAAAAAAAAAAA"}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"valid":true`) || !strings.Contains(rec.Body.String(), `"name":"Villalobos Zoe"`) {
		t.Errorf("verify: %d %s", rec.Code, rec.Body.String())
	}

	f.err = connect.NewError(connect.CodeNotFound, nil)
	rec = httptest.NewRecorder()
	cardHandler(f).Verify(rec, httptest.NewRequest(http.MethodPost, "/api/v1/students/card/verify", strings.NewReader(`{"code":"GLCARD:ZZZZZZZZZZZZZZZZZZZZZZ"}`)))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown code: %d, want 404", rec.Code)
	}
}

// PLAN-import-avisos: a draft's status is named only with module_import_review;
// off, the card reads it as it did before the review existed.
func TestVerifyNamesADraftOnlyWithTheReview(t *testing.T) {
	for _, tc := range []struct {
		on   bool
		want string
	}{{false, `"admissionStatus":""`}, {true, `"admissionStatus":"draft"`}} {
		f := &fakeCards{verified: client.StudentResult{ID: "stu-1", AdmissionStatus: "draft", Draft: true}, valid: true}
		h := handler.NewCardHandler(f, f, f, f, f, handler.WithCardImportReview(func(context.Context) bool { return tc.on }))
		rec := httptest.NewRecorder()
		h.Verify(rec, httptest.NewRequest(http.MethodPost, "/api/v1/students/card/verify", strings.NewReader(`{"code":"GLCARD:AQAAAAAAAAAAAAAAAAAAAA"}`)))
		if !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("on=%v: body %s, want %s", tc.on, rec.Body.String(), tc.want)
		}
	}
	f := &fakeCards{verified: client.StudentResult{ID: "stu-1", AdmissionStatus: "draft"}, valid: true}
	rec := httptest.NewRecorder()
	cardHandler(f).Verify(rec, httptest.NewRequest(http.MethodPost, "/api/v1/students/card/verify", strings.NewReader(`{"code":"GLCARD:AQAAAAAAAAAAAAAAAAAAAA"}`)))
	if !strings.Contains(rec.Body.String(), `"admissionStatus":""`) {
		t.Errorf("unwired review must read as off: %s", rec.Body.String())
	}
}

func TestStaffRevokeACard(t *testing.T) {
	f := &fakeCards{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/students/stu-1/card/revoke", nil)
	req.SetPathValue("id", "stu-1")
	rec := httptest.NewRecorder()
	cardHandler(f).Revoke(rec, req)
	if rec.Code != http.StatusNoContent || f.revoked != "stu-1" {
		t.Errorf("revoke: %d, revoked %q", rec.Code, f.revoked)
	}
}

// Task 13 (owner's decision): a revoked card is suspended until the office
// reissues it; the family's card says so and carries no code.
func TestASuspendedCardCarriesNoCode(t *testing.T) {
	f := &fakeCards{
		links: []client.GuardianLink{{StudentID: "stu-1"}}, suspended: true,
		student: client.StudentResult{ID: "stu-1", FirstName: "Zoe", LastName: "Villalobos"},
	}
	rec := httptest.NewRecorder()
	cardHandler(f).FamilyCard(rec, asGuardian(httptest.NewRequest(http.MethodGet, "/api/v1/family/students/stu-1/card", nil), "stu-1"))

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"suspended":true`) || !strings.Contains(rec.Body.String(), `"qr":""`) {
		t.Errorf("suspended card: %d %s", rec.Code, rec.Body.String())
	}
}

func TestStaffReissueACard(t *testing.T) {
	f := &fakeCards{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/students/stu-1/card/reissue", nil)
	req.SetPathValue("id", "stu-1")
	rec := httptest.NewRecorder()
	cardHandler(f).Reissue(rec, req)
	if rec.Code != http.StatusNoContent || f.reissued != "stu-1" {
		t.Errorf("reissue: %d, reissued %q", rec.Code, f.reissued)
	}
}
