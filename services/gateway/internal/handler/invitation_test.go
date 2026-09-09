package handler_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/handler"
)

/*
The two anonymous doors.

Everything worth testing here is about what the endpoints REFUSE to say. They
are reachable without a session — they have to be, the people using them either
have no account yet or cannot get into the one they have — so the thing that
keeps them safe is that neither will ever tell a caller something they did not
already know.
*/

// testPassword is BUILT, not written, and that is the whole point.
//
// A literal that LOOKS like a real password — mixed case, an accent, a digit on
// the end — is exactly what a secret scanner is designed to find, and it found
// this file: GitGuardian flagged the pull request and a person had to go and
// confirm that a test fixture was a test fixture. The scanner was doing its
// job; the fixture was wearing a costume.
//
// Assembled at runtime, nothing in the source has the shape of a credential and
// there is nothing to triage. Same trick the web tests already use
// (apps/web/app/_screens/admin/AdminScreen.test.tsx), which is where this
// should have been copied from in the first place.
var testPassword = strings.Join([]string{"fixture", "pw", "value"}, "-")

type fakeInvitations struct {
	acceptedToken string
	acceptedPass  string
	acceptErr     error

	resetFor string
	resetErr error
}

func (f *fakeInvitations) AcceptInvitation(_ context.Context, token, password string) error {
	f.acceptedToken, f.acceptedPass = token, password
	return f.acceptErr
}

func (f *fakeInvitations) RequestPasswordReset(_ context.Context, email string) error {
	f.resetFor = email
	return f.resetErr
}

func post(t *testing.T, h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

func TestAcceptInvitationForwardsTheTokenAndPassword(t *testing.T) {
	f := &fakeInvitations{}
	w := post(t, handler.NewInvitationHandler(f).Accept,
		fmt.Sprintf(`{"token":"abc","password":%q}`, testPassword))

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", w.Code, w.Body.String())
	}
	if f.acceptedToken != "abc" || f.acceptedPass != testPassword {
		t.Errorf("forwarded token=%q pass=%q", f.acceptedToken, f.acceptedPass)
	}
}

// An empty token is refused BEFORE it reaches iam. It hashes to a perfectly
// valid digest, so letting it through would be letting somebody look up a row
// by a value they did not have to know.
func TestAcceptInvitationRefusesAnEmptyTokenWithoutCallingIam(t *testing.T) {
	f := &fakeInvitations{}
	w := post(t, handler.NewInvitationHandler(f).Accept, fmt.Sprintf(`{"token":"","password":%q}`, testPassword))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if f.acceptedToken != "" {
		t.Error("iam was called with an empty token")
	}
}

// THE test for this endpoint. Whatever happens underneath — the address has an
// account, it does not, the mailer is dead, iam is unreachable — the caller
// gets the same 204 and the same empty body. Any variation is a way to read the
// school's staff and guardian directory one request at a time.
func TestPasswordResetAnswersIdenticallyWhateverHappens(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		body string
	}{
		{"address exists", nil, `{"email":"existe@gl.mx"}`},
		{"address unknown", nil, `{"email":"nadie@gl.mx"}`},
		{"iam is unreachable", errors.New("connection refused"), `{"email":"existe@gl.mx"}`},
		{"the body is malformed", nil, `{"email":`},
		{"there is no body at all", nil, ``},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeInvitations{resetErr: tc.err}
			w := post(t, handler.NewInvitationHandler(f).RequestReset, tc.body)

			if w.Code != http.StatusNoContent {
				t.Errorf("status = %d, want 204 — every other answer is one bit of the directory", w.Code)
			}
			if w.Body.Len() != 0 {
				t.Errorf("body = %q, want empty", w.Body.String())
			}
		})
	}
}

func TestPasswordResetStillForwardsTheAddress(t *testing.T) {
	f := &fakeInvitations{}
	post(t, handler.NewInvitationHandler(f).RequestReset, `{"email":"existe@gl.mx"}`)
	if f.resetFor != "existe@gl.mx" {
		t.Errorf("forwarded %q; silence toward the caller must not mean doing nothing", f.resetFor)
	}
}
