package googleid_test

import (
	"context"
	"errors"
	"testing"

	firebaseauth "firebase.google.com/go/v4/auth"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/adapters/googleid"
)

/*
What this adapter is responsible for, and what it deliberately is not.

The Firebase Admin SDK does the cryptography: signature, issuer, audience and
expiry. Re-testing that would be testing Google. What IS ours, and what these
tests pin down, is the policy applied to a token the SDK already accepted:

  1. the email must be present, and
  2. the email must be VERIFIED.

(2) is not a nicety. Identity Platform accepts self sign-up by default, so
without it anyone could register `director@garcialorca.edu.mx` in the provider
without owning that mailbox, present the resulting token, and be handed the
director's session — our lookup would find the row, because the row exists.
Demanding a verified claim means the address can only be asserted by whoever
actually receives mail at it.

Everything below drives the SDK's own Token shape through the mapping, which is
why the fake returns a *firebaseauth.Token and not a hand-rolled struct.
*/

// fakeSDK stands in for the Admin SDK's VerifyIDToken.
type fakeSDK struct {
	token *firebaseauth.Token
	err   error
	calls int
}

func (f *fakeSDK) VerifyIDTokenAndCheckRevoked(_ context.Context, _ string) (*firebaseauth.Token, error) {
	f.calls++
	return f.token, f.err
}

func tokenWith(uid string, claims map[string]any) *firebaseauth.Token {
	return &firebaseauth.Token{UID: uid, Claims: claims}
}

func TestVerifyReturnsIdentityForVerifiedEmail(t *testing.T) {
	v := googleid.NewVerifier(&fakeSDK{token: tokenWith("google-uid-1", map[string]any{
		"email":          "ana@garcialorca.mx",
		"email_verified": true,
	})})

	id, err := v.Verify(context.Background(), "any-token")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if id.Email != "ana@garcialorca.mx" {
		t.Errorf("email = %q, want ana@garcialorca.mx", id.Email)
	}
	if id.Subject != "google-uid-1" {
		t.Errorf("subject = %q, want the provider uid", id.Subject)
	}
}

func TestVerifyRejectsUnverifiedEmail(t *testing.T) {
	// The impersonation case. The provider vouches that this token is real; it
	// does NOT vouch that the holder owns the mailbox.
	v := googleid.NewVerifier(&fakeSDK{token: tokenWith("google-uid-2", map[string]any{
		"email":          "director@garcialorca.edu.mx",
		"email_verified": false,
	})})

	_, err := v.Verify(context.Background(), "any-token")
	if glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Fatalf("error = %v, want KindUnauthorized for an unverified email", err)
	}
}

func TestVerifyRejectsMissingEmailVerifiedClaim(t *testing.T) {
	// Absent is not the same as true. A token shaped differently by a future
	// provider change must fail closed, not default to trusted.
	v := googleid.NewVerifier(&fakeSDK{token: tokenWith("google-uid-3", map[string]any{
		"email": "ana@garcialorca.mx",
	})})

	_, err := v.Verify(context.Background(), "any-token")
	if glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Fatalf("error = %v, want KindUnauthorized when the claim is absent", err)
	}
}

func TestVerifyRejectsMissingEmail(t *testing.T) {
	// Phone or anonymous sign-in would produce this. Both are disabled in the
	// provider config, and this is the second lock on that door.
	v := googleid.NewVerifier(&fakeSDK{token: tokenWith("google-uid-4", map[string]any{
		"email_verified": true,
	})})

	_, err := v.Verify(context.Background(), "any-token")
	if glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Fatalf("error = %v, want KindUnauthorized with no email claim", err)
	}
}

func TestVerifyTranslatesSDKFailure(t *testing.T) {
	// A bad signature, a wrong audience, an expired token: the SDK's own error.
	// It must arrive at the use case as KindUnauthorized and nothing else — the
	// caller learns that it failed, never why.
	v := googleid.NewVerifier(&fakeSDK{err: errors.New("id token has invalid signature")})

	_, err := v.Verify(context.Background(), "any-token")
	if glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Fatalf("error = %v, want KindUnauthorized", err)
	}
	if got := glerrors.SafeMessage(err); got == "id token has invalid signature" {
		t.Error("the SDK's message reached the caller; it must not leak verification detail")
	}
}

func TestVerifyChecksRevocation(t *testing.T) {
	// Plain VerifyIDToken accepts a token for the rest of its ~1h life even
	// after the account's password was reset or the account was disabled at the
	// provider. This service would then mint a fresh 15-minute access token AND
	// a 24-hour refresh from it — quietly undoing the revocation-on-password-
	// change guarantee the password path was deliberately built to give
	// (RefreshToken's password_changed_at floor, ADR 0005).
	//
	// So the adapter must use the checking variant. This test exists to make
	// that a decision the code states, not one a future edit can drop by
	// swapping a method name.
	sdk := &fakeSDK{token: tokenWith("google-uid-1", map[string]any{
		"email":          "ana@garcialorca.mx",
		"email_verified": true,
	})}
	v := googleid.NewVerifier(sdk)

	if _, err := v.Verify(context.Background(), "any-token"); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if sdk.calls != 1 {
		t.Fatalf("revocation-checking verify called %d times, want 1", sdk.calls)
	}
}
