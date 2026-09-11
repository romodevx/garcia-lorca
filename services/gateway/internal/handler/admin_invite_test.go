package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
	// The link is deliberately absent on a DELIVERED invitation: it comes back
	// only when nothing went out and somebody has to hand it over by hand.
	// Asserting it here was asserting an extra copy of a live token nobody
	// needed.
	if !body.Sent {
		t.Errorf("body = %+v, want the delivery outcome", body)
	}
	if body.URL != "" {
		t.Errorf("the link came back on a delivered invitation: %+v", body)
	}
}

/*
The resend route, after the security review.

Returning the activation link to whoever called the endpoint turned
`users:write` from "administer accounts within your own permission subset" into
"take over any account that is not active" — the caller-subset guard that fences
every other admin write did not apply here, because the old 204 response meant
the link only ever reached the account's own mailbox.
*/

// resendAdmin embeds the shared double and overrides only what this guard
// reads: the target user, the role catalogue, and the resend itself.
type resendAdmin struct {
	*inviteAdmin
	target client.User
	roles  []client.Role
	inv    client.Invitation
}

func (f *resendAdmin) GetUser(_ context.Context, id string) (client.User, error) {
	if id != f.target.ID {
		return client.User{}, errors.New("not found")
	}
	return f.target, nil
}
func (f *resendAdmin) ListRoles(context.Context) ([]client.Role, error) { return f.roles, nil }
func (f *resendAdmin) ResendInvitation(_ context.Context, id string) (client.Invitation, error) {
	f.resentTo = id
	return f.inv, nil
}

// doResend drives the route as a caller holding exactly `perms`.
func doResend(t *testing.T, f *resendAdmin, perms []string, id string) *httptest.ResponseRecorder {
	t.Helper()
	h := http.HandlerFunc(newAdminHandler(f, &fakeLinker{}).ResendInvitation)
	return adminReqAs(t, h, principalHolding(perms...), http.MethodPost,
		"/api/v1/users/"+id+"/invite", "", "id", id)
}

// A caller may not pull the access link of somebody whose powers it does not
// already hold. Same rule as assigning a role, for the same reason.
func TestResendRefusesWhenTheTargetOutranksTheCaller(t *testing.T) {
	f := &resendAdmin{
		inviteAdmin: &inviteAdmin{},
		target:      client.User{ID: "owner-1", Status: "invited", RoleIDs: []string{"r-owner"}},
		roles:       []client.Role{{ID: "r-owner", Permissions: []string{"users:write", "finance:read"}}},
		inv:         client.Invitation{URL: "https://escuela.example/activar?token=secreto"},
	}
	// The caller holds users:write but NOT finance:read.
	w := doResend(t, f, []string{"users:write"}, "owner-1")

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", w.Code, w.Body.String())
	}
	if f.resentTo != "" {
		t.Error("iam was called anyway")
	}
	if strings.Contains(w.Body.String(), "secreto") {
		t.Error("the token leaked in the refusal")
	}
}

func TestResendAllowedWhenTheCallerHoldsEverythingTheTargetDoes(t *testing.T) {
	f := &resendAdmin{
		inviteAdmin: &inviteAdmin{},
		target:      client.User{ID: "u2", Status: "invited", RoleIDs: []string{"r-maestro"}},
		roles:       []client.Role{{ID: "r-maestro", Permissions: []string{"grades:create"}}},
		inv:         client.Invitation{URL: "https://escuela.example/activar?token=t", Sent: false},
	}
	w := doResend(t, f, []string{"users:write", "grades:create"}, "u2")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if f.resentTo != "u2" {
		t.Errorf("resent to %q", f.resentTo)
	}
}

// The link comes back ONLY when nothing was delivered — that is the case it
// exists for. When the message went out, handing the token to the caller buys
// nothing and widens who holds a credential for somebody else's account.
func TestResendWithholdsTheLinkWhenTheMessageWentOut(t *testing.T) {
	f := &resendAdmin{
		inviteAdmin: &inviteAdmin{},
		target:      client.User{ID: "u2", Status: "invited", RoleIDs: []string{}},
		inv:         client.Invitation{URL: "https://escuela.example/activar?token=secreto", Sent: true, Via: "smtp"},
	}
	w := doResend(t, f, []string{"users:write"}, "u2")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if strings.Contains(w.Body.String(), "secreto") {
		t.Errorf("the link came back on a delivered invitation: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"sent":true`) {
		t.Errorf("the outcome is missing: %s", w.Body.String())
	}
}
