package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// inviteAdmin extends the admin double with the invite surface, recording what
// iam was asked for.
type inviteAdmin struct {
	fakeAdmin
	inviteIn struct {
		email, display string
		roleIDs        []string
	}
	called   bool
	resentTo string
}

func (f *inviteAdmin) ResendInvitation(_ context.Context, userID string) (client.Invitation, error) {
	f.resentTo = userID
	if f.err != nil {
		return client.Invitation{}, f.err
	}
	return client.Invitation{URL: "https://escuela.example/activar?token=t", Sent: true}, nil
}

func (f *inviteAdmin) InviteUser(_ context.Context, email, displayName string, roleIDs []string) (client.User, client.Invitation, error) {
	f.called = true
	f.inviteIn.email, f.inviteIn.display, f.inviteIn.roleIDs = email, displayName, roleIDs
	if f.err != nil {
		return client.User{}, client.Invitation{}, f.err
	}
	// Status "invited": an account nobody has opened yet is not active, and the
	// fixture used to say otherwise — which is the state the resend button now
	// keys on.
	return client.User{ID: "usr-2", Email: email, DisplayName: displayName, Status: "invited", RoleIDs: roleIDs},
		client.Invitation{URL: "https://escuela.example/activar?token=t", Sent: true}, nil
}

// TestAdminInviteUser: the route exists, reaches iam, and answers with the user.
func TestAdminInviteUser(t *testing.T) {
	f := &inviteAdmin{}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).InviteUser)
	w := doAdminReq(t, h, http.MethodPost, "/api/v1/users/invite",
		`{"email":"nueva.maestra@garcialorca.mx","displayName":"Nueva Maestra","roleIds":["rol-1"]}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
	}
	if f.inviteIn.email != "nueva.maestra@garcialorca.mx" || len(f.inviteIn.roleIDs) != 1 {
		t.Errorf("captured input mismatch: %+v", f.inviteIn)
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// No temporary password exists, so nothing is pending rotation.
	if out["id"] != "usr-2" || out["mustChangePassword"] != false {
		t.Errorf("body = %v", out)
	}
}

// TestAdminInviteUserNeverEchoesAPassword. There is no password anywhere in this
// flow, and the response must not grow one: the whole reason to prefer invite
// over create is that nobody but the account's owner ever holds it.
func TestAdminInviteUserNeverEchoesAPassword(t *testing.T) {
	f := &inviteAdmin{}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).InviteUser)
	w := doAdminReq(t, h, http.MethodPost, "/api/v1/users/invite",
		`{"email":"otra@garcialorca.mx","displayName":"Otra"}`)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	for _, k := range []string{"password", "tempPassword", "temp_password"} {
		if _, ok := out[k]; ok {
			t.Errorf("response carries %q", k)
		}
	}
}

// TestAdminInviteUserAppliesTheSubsetGuard is the security assertion, and it is
// SHARPER here than on CreateUser. Inviting an owner-powered account to an
// address the caller controls is a direct route to owner: the link arrives in
// their own mailbox and they choose the password. So a caller may only invite
// into roles whose permissions it already holds.
func TestAdminInviteUserAppliesTheSubsetGuard(t *testing.T) {
	f := &inviteAdmin{}
	f.rolesList = []client.Role{{ID: "rol-owner", Name: "owner", Permissions: []string{"users:write", "finance:write"}}}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).InviteUser)

	// The caller administers users but holds no finance:write.
	w := adminReqAs(t, h, principalHolding("users:write", "users:read", "roles:read"),
		http.MethodPost, "/api/v1/users/invite",
		`{"email":"escalada@garcialorca.mx","displayName":"Escalada","roleIds":["rol-owner"]}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	if f.called {
		t.Fatal("iam must not be called: no account may exist for a refused invitation")
	}
}

// TestAdminInviteUserRejectsAMalformedBody without reaching iam.
func TestAdminInviteUserRejectsAMalformedBody(t *testing.T) {
	f := &inviteAdmin{}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).InviteUser)
	w := doAdminReq(t, h, http.MethodPost, "/api/v1/users/invite", `{"email":`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if f.called {
		t.Fatal("iam must not be called for an undecodable body")
	}
}

// TestAdminResendInvitation: the route reaches iam with the USER ID from the
// path. Keyed by id, so it can never be aimed at an arbitrary address.
//
// It answers 200 WITH A BODY, and it used to answer 204. That change is the
// feature: "no content" made a delivered invitation and a dropped one look
// identical to the caller, which is exactly how the original bug stayed
// invisible. The body now says whether the message left, why not, and what link
// to hand over instead.
func TestAdminResendInvitation(t *testing.T) {
	f := &inviteAdmin{}
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).ResendInvitation)
	w := doAdminReq(t, h, http.MethodPost, "/api/v1/users/usr-7/invite", "", "id", "usr-7")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if f.resentTo != "usr-7" {
		t.Errorf("resent to %q, want usr-7", f.resentTo)
	}
	var body struct {
		URL  string `json:"url"`
		Sent bool   `json:"sent"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}
	if !body.Sent || body.URL == "" {
		t.Errorf("body = %+v, want the delivery outcome and the link", body)
	}
}
