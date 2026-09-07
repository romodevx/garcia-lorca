package googleid_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/adapters/googleid"
)

/*
Asking the provider to deliver the invitation.

The Admin SDK generates action links but does not send them, and this project
has no mail sender of its own (comms does push, not email). The identity
toolkit's sendOobCode endpoint DOES send, so Google composes and delivers the
message and we own no mail infrastructure at all.

The same call is the password RESET iam never had: for an existing account,
"resend the invitation" and "I forgot my password" are the same request.

These tests drive a stub server rather than Google, so what gets asserted is the
request we make and how we read the answer — not Google's mail delivery.
*/

func TestSendInvitationAsksTheProviderToMail(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		if r.URL.Query().Get("key") == "" {
			t.Error("the api key must travel as a query parameter")
		}
		_, _ = w.Write([]byte(`{"email":"ana@garcialorca.mx"}`))
	}))
	defer srv.Close()

	inv := googleid.NewInviter("clave-de-prueba", srv.URL, srv.Client())
	if err := inv.SendInvitation(context.Background(), "ana@garcialorca.mx"); err != nil {
		t.Fatalf("SendInvitation: %v", err)
	}

	// PASSWORD_RESET is what makes the link a "set your password" link for an
	// account created without one. It is the same request type for a genuine
	// reset, which is exactly why one button serves both.
	if got["requestType"] != "PASSWORD_RESET" {
		t.Errorf("requestType = %q, want PASSWORD_RESET", got["requestType"])
	}
	if got["email"] != "ana@garcialorca.mx" {
		t.Errorf("email = %q", got["email"])
	}
}

func TestSendInvitationReportsAProviderRefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"EMAIL_NOT_FOUND"}}`))
	}))
	defer srv.Close()

	inv := googleid.NewInviter("clave-de-prueba", srv.URL, srv.Client())
	err := inv.SendInvitation(context.Background(), "nadie@garcialorca.mx")
	if err == nil {
		t.Fatal("want an error when the provider refuses, got nil")
	}
	// The director needs to know the mail did not go out; a silent success would
	// leave them telling somebody to check an inbox nothing was sent to.
	if glerrors.KindOf(err) != glerrors.KindInternal {
		t.Fatalf("error = %v (kind %v), want KindInternal", err, glerrors.KindOf(err))
	}
}

func TestSendInvitationRefusesWithoutAnApiKey(t *testing.T) {
	inv := googleid.NewInviter("", "https://example.invalid", http.DefaultClient)

	// Fail closed and say why. Without a key this endpoint answers 400 for every
	// call, and "the invitation was not sent" with no reason is a support ticket
	// nobody can close.
	if err := inv.SendInvitation(context.Background(), "ana@garcialorca.mx"); err == nil {
		t.Fatal("want an error with no api key configured, got nil")
	}
}
