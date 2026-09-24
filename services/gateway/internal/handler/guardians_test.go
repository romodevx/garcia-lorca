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

// fakeGuardians is a hand-written double for the guardian-link surface the
// handler needs. It records the last input and returns canned results/errors so
// each handler is exercised in isolation (no Connect round-trip).
type fakeGuardians struct {
	linkGuardianID   string
	linkStudentID    string
	linkRelationship string
	link             client.GuardianLink

	unlinkGuardianID string
	unlinkStudentID  string

	listStudentID string
	links         []client.GuardianLink

	contactStudentID string
	contactName      string
	contactPhone     string
	contactIn        client.ContactInput
	contact          client.GuardianLink
	updatedContactID string
	removedContactID string
	removedStudentID string

	err error
}

func (f *fakeGuardians) AddContact(_ context.Context, in client.ContactInput) (client.GuardianLink, error) {
	f.contactStudentID, f.contactName, f.contactPhone, f.contactIn = in.StudentID, in.FullName, in.Phone, in
	if f.err != nil {
		return client.GuardianLink{}, f.err
	}
	f.contact = client.GuardianLink{
		ID: "gl-9", StudentID: in.StudentID, Relationship: in.Relationship,
		FullName: in.FullName, Phone: in.Phone, Email: in.Email, IsEmergency: in.IsEmergency,
	}
	return f.contact, nil
}

func (f *fakeGuardians) UpdateContact(_ context.Context, contactID string, in client.ContactInput) (client.GuardianLink, error) {
	f.updatedContactID, f.contactIn = contactID, in
	if f.err != nil {
		return client.GuardianLink{}, f.err
	}
	return client.GuardianLink{ID: contactID, StudentID: in.StudentID, FullName: in.FullName, IsEmergency: in.IsEmergency}, nil
}

func (f *fakeGuardians) RemoveContact(_ context.Context, contactID, studentID string) error {
	f.removedContactID, f.removedStudentID = contactID, studentID
	return f.err
}

func (f *fakeGuardians) LinkGuardian(_ context.Context, guardianUserID, studentID, relationship string) (client.GuardianLink, error) {
	f.linkGuardianID, f.linkStudentID, f.linkRelationship = guardianUserID, studentID, relationship
	if f.err != nil {
		return client.GuardianLink{}, f.err
	}
	return f.link, nil
}

func (f *fakeGuardians) UnlinkGuardian(_ context.Context, guardianUserID, studentID string) error {
	f.unlinkGuardianID, f.unlinkStudentID = guardianUserID, studentID
	return f.err
}

func (f *fakeGuardians) ListGuardiansForStudent(_ context.Context, studentID string) ([]client.GuardianLink, error) {
	f.listStudentID = studentID
	return f.links, f.err
}

// --- link ------------------------------------------------------------------

func TestLinkGuardianReturns201AndBody(t *testing.T) {
	svc := &fakeGuardians{link: client.GuardianLink{
		ID: "gl-1", TenantID: "t1", GuardianUserID: "usr-9", StudentID: "stu-1", Relationship: "mother",
	}}
	h := handler.NewGuardiansHandler(svc)

	rec := doReq(t, h.LinkGuardian, http.MethodPost, "/api/v1/guardians",
		`{"guardianUserId":"usr-9","studentId":"stu-1","relationship":"mother"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["id"] != "gl-1" || out["guardianUserId"] != "usr-9" || out["studentId"] != "stu-1" || out["relationship"] != "mother" {
		t.Errorf("body = %v", out)
	}
	if svc.linkGuardianID != "usr-9" || svc.linkStudentID != "stu-1" || svc.linkRelationship != "mother" {
		t.Errorf("forwarded = %q/%q/%q", svc.linkGuardianID, svc.linkStudentID, svc.linkRelationship)
	}
}

func TestLinkGuardianMalformedBodyIs400(t *testing.T) {
	h := handler.NewGuardiansHandler(&fakeGuardians{})
	rec := doReq(t, h.LinkGuardian, http.MethodPost, "/api/v1/guardians", `not json`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestLinkGuardianDuplicateIs409(t *testing.T) {
	h := handler.NewGuardiansHandler(&fakeGuardians{
		err: connect.NewError(connect.CodeAlreadyExists, errString("already linked")),
	})
	rec := doReq(t, h.LinkGuardian, http.MethodPost, "/api/v1/guardians",
		`{"guardianUserId":"usr-9","studentId":"stu-1","relationship":"mother"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

// On LinkGuardian the students service treats BOTH references as arguments — an
// unknown user, an unknown student and a bad relationship are all
// InvalidArgument, never NotFound — so the edge answers 400. There is no
// addressed resource in the URL that could be missing.
func TestLinkGuardianUnknownUserIsBadRequest(t *testing.T) {
	h := handler.NewGuardiansHandler(&fakeGuardians{
		err: connect.NewError(connect.CodeInvalidArgument, errString("guardian user not found")),
	})
	rec := doReq(t, h.LinkGuardian, http.MethodPost, "/api/v1/guardians",
		`{"guardianUserId":"ghost","studentId":"stu-1","relationship":"mother"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// --- unlink ----------------------------------------------------------------

func TestUnlinkGuardianIs204(t *testing.T) {
	svc := &fakeGuardians{}
	h := handler.NewGuardiansHandler(svc)

	rec := doReq(t, h.UnlinkGuardian, http.MethodPost, "/api/v1/guardians/unlink",
		`{"guardianUserId":"usr-9","studentId":"stu-1"}`)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if svc.unlinkGuardianID != "usr-9" || svc.unlinkStudentID != "stu-1" {
		t.Errorf("forwarded = %q/%q", svc.unlinkGuardianID, svc.unlinkStudentID)
	}
}

// Unlinking is NOT idempotent: the students contract answers NotFound when the
// pair is not linked, which the edge surfaces as 404 — the admin UI's "there was
// nothing to revoke" signal.
func TestUnlinkGuardianUnknownLinkIs404(t *testing.T) {
	h := handler.NewGuardiansHandler(&fakeGuardians{
		err: connect.NewError(connect.CodeNotFound, errString("guardian link not found")),
	})
	rec := doReq(t, h.UnlinkGuardian, http.MethodPost, "/api/v1/guardians/unlink",
		`{"guardianUserId":"usr-9","studentId":"stu-1"}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestUnlinkGuardianMalformedBodyIs400(t *testing.T) {
	h := handler.NewGuardiansHandler(&fakeGuardians{})
	rec := doReq(t, h.UnlinkGuardian, http.MethodPost, "/api/v1/guardians/unlink", `not json`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestUnlinkGuardianInvalidArgumentIs400(t *testing.T) {
	h := handler.NewGuardiansHandler(&fakeGuardians{
		err: connect.NewError(connect.CodeInvalidArgument, errString("missing student id")),
	})
	rec := doReq(t, h.UnlinkGuardian, http.MethodPost, "/api/v1/guardians/unlink",
		`{"guardianUserId":"usr-9","studentId":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// --- list a student's guardians --------------------------------------------

func TestListGuardiansForStudentReadsPathValue(t *testing.T) {
	svc := &fakeGuardians{links: []client.GuardianLink{
		{ID: "gl-1", TenantID: "t1", GuardianUserID: "usr-9", StudentID: "stu-1", Relationship: "mother"},
		{ID: "gl-2", TenantID: "t1", GuardianUserID: "usr-8", StudentID: "stu-1", Relationship: "father"},
	}}
	h := handler.NewGuardiansHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/students/stu-1/guardians", nil)
	// httptest.NewRequest does not populate path values; set it explicitly so the
	// handler sees what ServeMux would inject in production.
	req.SetPathValue("studentId", "stu-1")
	h.ListGuardiansForStudent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.listStudentID != "stu-1" {
		t.Errorf("studentId forwarded = %q, want stu-1", svc.listStudentID)
	}
	var out struct {
		Guardians []map[string]any `json:"guardians"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Guardians) != 2 || out.Guardians[1]["relationship"] != "father" {
		t.Errorf("guardians = %v", out.Guardians)
	}
}

func TestListGuardiansForStudentEmptyIsEmptyArray(t *testing.T) {
	h := handler.NewGuardiansHandler(&fakeGuardians{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/students/stu-1/guardians", nil)
	req.SetPathValue("studentId", "stu-1")
	h.ListGuardiansForStudent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out struct {
		Guardians []map[string]any `json:"guardians"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Guardians == nil || len(out.Guardians) != 0 {
		t.Errorf("guardians = %v, want an empty array (never null)", out.Guardians)
	}
}

func TestListGuardiansForStudentPropagatesErrorKind(t *testing.T) {
	h := handler.NewGuardiansHandler(&fakeGuardians{
		err: connect.NewError(connect.CodeNotFound, errString("no student")),
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/students/ghost/guardians", nil)
	req.SetPathValue("studentId", "ghost")
	h.ListGuardiansForStudent(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// A contact with no account is the ordinary case, and the response has to say
// plainly that this person cannot open the app — otherwise the office assumes
// they can and stops calling.
func TestAddContactReturnsSomebodyWithoutAnAccount(t *testing.T) {
	svc := &fakeGuardians{}
	h := handler.NewGuardiansHandler(svc)

	rec := doReq(t, h.AddContact, http.MethodPost, "/api/v1/contacts",
		`{"studentId":"stu-1","relationship":"other","fullName":"Rosa Méndez","phone":"5551234567"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if svc.contactName != "Rosa Méndez" || svc.contactPhone != "5551234567" {
		t.Errorf("forwarded %q / %q", svc.contactName, svc.contactPhone)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["hasAccount"] != false {
		t.Errorf("hasAccount = %v, want false", body["hasAccount"])
	}
	if body["phone"] != "5551234567" {
		t.Errorf("phone = %v", body["phone"])
	}
}

// A contact is edited and removed under its pupil: the path names both, so a
// contact id alone never reaches another family's child.
func TestContactIsEditedAndRemovedUnderItsPupil(t *testing.T) {
	svc := &fakeGuardians{}
	h := handler.NewGuardiansHandler(svc)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/students/stu-1/contacts/gl-9",
		strings.NewReader(`{"relationship":"guardian","fullName":"Rosa Méndez","phone":"3312345678","isEmergency":true}`))
	req.SetPathValue("studentId", "stu-1")
	req.SetPathValue("contactId", "gl-9")
	rec := httptest.NewRecorder()
	h.UpdateContact(rec, req)
	if rec.Code != http.StatusOK || svc.updatedContactID != "gl-9" || svc.contactIn.StudentID != "stu-1" || !svc.contactIn.IsEmergency {
		t.Fatalf("update: status %d forwarded %q %+v", rec.Code, svc.updatedContactID, svc.contactIn)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["isEmergency"] != true {
		t.Errorf("isEmergency in body = %v (%v)", body["isEmergency"], err)
	}

	del := httptest.NewRequest(http.MethodDelete, "/api/v1/students/stu-1/contacts/gl-9", nil)
	del.SetPathValue("studentId", "stu-1")
	del.SetPathValue("contactId", "gl-9")
	rec = httptest.NewRecorder()
	h.RemoveContact(rec, del)
	if rec.Code != http.StatusNoContent || svc.removedContactID != "gl-9" || svc.removedStudentID != "stu-1" {
		t.Errorf("remove: status %d forwarded %q %q", rec.Code, svc.removedContactID, svc.removedStudentID)
	}
}

func TestAddContactForwardsTheEmergencyMark(t *testing.T) {
	svc := &fakeGuardians{}
	h := handler.NewGuardiansHandler(svc)
	rec := doReq(t, h.AddContact, http.MethodPost, "/api/v1/contacts",
		`{"studentId":"stu-1","relationship":"other","fullName":"Rosa","isEmergency":true}`)
	if rec.Code != http.StatusCreated || !svc.contactIn.IsEmergency {
		t.Errorf("status %d emergency %v", rec.Code, svc.contactIn.IsEmergency)
	}
}
