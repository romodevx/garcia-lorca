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

// --- doubles ---------------------------------------------------------------

// fakeIssuer is the iam half: the users that already exist, and what a
// guardian invitation produces. There is no role to pass: InviteGuardian is
// the one iam road into the guardian role, and iam picks the role itself
// (ISSUE-CC-115) — InviteUser refuses it.
type fakeIssuer struct {
	users []client.User

	invitedEmail string
	invitedName  string
	invitedUser  client.User
	invitation   client.Invitation
	inviteErr    error
}

func (f *fakeIssuer) ListUsers(context.Context) ([]client.User, error) { return f.users, nil }

func (f *fakeIssuer) InviteGuardian(_ context.Context, email, displayName string) (client.User, client.Invitation, error) {
	f.invitedEmail, f.invitedName = email, displayName
	if f.inviteErr != nil {
		return client.User{}, client.Invitation{}, f.inviteErr
	}
	return f.invitedUser, f.invitation, nil
}

// fakeGranter is the students half: the pupil's contact list and the grant.
type fakeGranter struct {
	links []client.GuardianLink

	grantContactID string
	grantStudentID string
	grantUserID    string
	granted        client.GuardianLink
	grantErr       error
	listErr        error
}

func (f *fakeGranter) ListGuardiansForStudent(_ context.Context, studentID string) ([]client.GuardianLink, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]client.GuardianLink, 0, len(f.links))
	for _, l := range f.links {
		if l.StudentID == studentID {
			out = append(out, l)
		}
	}
	return out, nil
}

func (f *fakeGranter) GrantContactAccess(_ context.Context, contactID, studentID, guardianUserID string) (client.GuardianLink, error) {
	f.grantContactID, f.grantStudentID, f.grantUserID = contactID, studentID, guardianUserID
	if f.grantErr != nil {
		return client.GuardianLink{}, f.grantErr
	}
	return f.granted, nil
}

// --- helpers ---------------------------------------------------------------

func issuerWithGuardianRole() *fakeIssuer {
	return &fakeIssuer{
		invitedUser: client.User{ID: "usr-new", Email: "mateo@example.com"},
		invitation:  client.Invitation{URL: "https://x/invite/abc", Sent: true},
	}
}

func granterWithContact(contact client.GuardianLink) *fakeGranter {
	return &fakeGranter{
		links:   []client.GuardianLink{contact},
		granted: client.GuardianLink{ID: contact.ID, StudentID: contact.StudentID, FullName: contact.FullName, Email: contact.Email, GuardianUserID: "usr-new"},
	}
}

func plainContact() client.GuardianLink {
	return client.GuardianLink{
		ID: "gl-1", StudentID: "stu-1", Relationship: "father",
		FullName: "Mateo Contreras", Email: "mateo@example.com",
	}
}

// doInvite drives the handler through a router so the path values are real.
func doInvite(t *testing.T, h *handler.ContactAccessHandler, studentID, contactID string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/students/{studentId}/guardians/{contactId}/invite", h.Invite)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/students/"+studentID+"/guardians/"+contactID+"/invite", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// --- the happy path --------------------------------------------------------

// The school gives a contact it already recorded access to the app: an account
// is invited into the guardian role and attached to that contact.
func TestInviteContactCreatesTheAccountAndAttachesIt(t *testing.T) {
	iam := issuerWithGuardianRole()
	students := granterWithContact(plainContact())
	h := handler.NewContactAccessHandler(iam, students)

	rec := doInvite(t, h, "stu-1", "gl-1")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if iam.invitedEmail != "mateo@example.com" || iam.invitedName != "Mateo Contreras" {
		t.Errorf("invited %q/%q, want the contact's own address and name", iam.invitedEmail, iam.invitedName)
	}
	if students.grantContactID != "gl-1" || students.grantStudentID != "stu-1" || students.grantUserID != "usr-new" {
		t.Errorf("granted %q/%q/%q", students.grantContactID, students.grantStudentID, students.grantUserID)
	}

	var out struct {
		Contact    map[string]any `json:"contact"`
		Invitation map[string]any `json:"invitation"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Contact["id"] != "gl-1" || out.Contact["hasAccount"] != true {
		t.Errorf("contact = %v", out.Contact)
	}
	if out.Invitation["sent"] != true {
		t.Errorf("invitation = %v", out.Invitation)
	}
	// The link is withheld once the message has gone out: it buys nothing then,
	// and every extra copy of a token is another place it can leak from. It
	// comes back only when the send failed — see the test below.
	if out.Invitation["url"] != "" {
		t.Errorf("url = %v, want empty — the message was delivered", out.Invitation["url"])
	}
}

// THE ONE THAT MATTERS TO A FAMILY. A mother of two children signs in once: the
// second child attaches to the account she already has instead of minting a
// second one she would never use.
func TestInviteContactReusesAnExistingAccount(t *testing.T) {
	iam := issuerWithGuardianRole()
	iam.users = []client.User{{ID: "usr-old", Email: "Mateo@Example.com"}}
	students := granterWithContact(plainContact())
	students.granted.GuardianUserID = "usr-old"
	h := handler.NewContactAccessHandler(iam, students)

	rec := doInvite(t, h, "stu-1", "gl-1")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if iam.invitedEmail != "" {
		t.Errorf("a second account was minted for %q — the address already had one", iam.invitedEmail)
	}
	if students.grantUserID != "usr-old" {
		t.Errorf("attached %q, want the account that already existed", students.grantUserID)
	}

	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["invitation"] != nil {
		t.Errorf("invitation = %v, want null — nothing was sent and nothing needed to be", out["invitation"])
	}
}

// --- refusals --------------------------------------------------------------

// An invitation with nowhere to go is not an invitation. Refusing BEFORE iam is
// asked is what keeps a half-made account from existing.
func TestInviteContactWithoutEmailIs400(t *testing.T) {
	iam := issuerWithGuardianRole()
	contact := plainContact()
	contact.Email = ""
	h := handler.NewContactAccessHandler(iam, granterWithContact(contact))

	rec := doInvite(t, h, "stu-1", "gl-1")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if iam.invitedEmail != "" {
		t.Error("iam was asked to invite somebody with no address")
	}
}

// Granting access to a contact that already has it would be a second account
// for one person.
func TestInviteContactThatAlreadyHasAnAccountIs400(t *testing.T) {
	iam := issuerWithGuardianRole()
	contact := plainContact()
	contact.GuardianUserID = "usr-already"
	h := handler.NewContactAccessHandler(iam, granterWithContact(contact))

	rec := doInvite(t, h, "stu-1", "gl-1")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if iam.invitedEmail != "" {
		t.Error("iam was asked to invite a contact that can already open the app")
	}
}

// A contact of another pupil is NOT FOUND — the same answer as a contact that
// does not exist, so the reply never confirms somebody else's contact is there.
func TestInviteContactOfAnotherPupilIs404(t *testing.T) {
	iam := issuerWithGuardianRole()
	other := plainContact()
	other.StudentID = "stu-2"
	h := handler.NewContactAccessHandler(iam, &fakeGranter{links: []client.GuardianLink{other}})

	rec := doInvite(t, h, "stu-1", "gl-1")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if iam.invitedEmail != "" {
		t.Error("iam was asked to invite for a contact the pupil does not have")
	}
}

func TestInviteUnknownContactIs404(t *testing.T) {
	h := handler.NewContactAccessHandler(issuerWithGuardianRole(), &fakeGranter{})
	if rec := doInvite(t, h, "stu-1", "gl-nope"); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// Without the guardian role there is nothing to invite anybody INTO, and an
// account with no role would be a login that can see nothing. That is the
// deployment's fault, not the caller's, so it must not read as a bad request.
func TestInviteContactWithoutTheGuardianRoleIs500(t *testing.T) {
	iam := issuerWithGuardianRole()
	iam.inviteErr = connect.NewError(connect.CodeInternal, errString("the guardian role is missing from this deployment"))
	students := granterWithContact(plainContact())
	h := handler.NewContactAccessHandler(iam, students)

	rec := doInvite(t, h, "stu-1", "gl-1")

	if rec.Code < 500 {
		t.Errorf("status = %d, want a 5xx — a missing role is the deployment's fault", rec.Code)
	}
	if students.grantUserID != "" {
		t.Errorf("granted %q after a failed invitation", students.grantUserID)
	}
}

// An undelivered message must never read as a delivered one: the office needs
// to know the mail did not leave so it can hand the link over by other means.
func TestInviteContactSurfacesAnUndeliveredMessage(t *testing.T) {
	iam := issuerWithGuardianRole()
	iam.invitation = client.Invitation{URL: "https://x/invite/abc", Sent: false, SendError: "smtp: no credential"}
	h := handler.NewContactAccessHandler(iam, granterWithContact(plainContact()))

	rec := doInvite(t, h, "stu-1", "gl-1")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — the account WAS created", rec.Code)
	}
	var out struct {
		Invitation struct {
			URL       string `json:"url"`
			Sent      bool   `json:"sent"`
			SendError string `json:"sendError"`
		} `json:"invitation"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Invitation.Sent {
		t.Error("sent = true for a message that was dropped")
	}
	if out.Invitation.URL == "" || !strings.Contains(out.Invitation.SendError, "smtp") {
		t.Errorf("the office cannot hand the link over: %+v", out.Invitation)
	}
}

// The students service is the authoritative validator; its refusal keeps its
// own meaning at the edge.
func TestInviteContactPropagatesAGrantConflict(t *testing.T) {
	students := granterWithContact(plainContact())
	students.grantErr = connect.NewError(connect.CodeAlreadyExists, errString("already has an account"))
	h := handler.NewContactAccessHandler(issuerWithGuardianRole(), students)

	if rec := doInvite(t, h, "stu-1", "gl-1"); rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}
