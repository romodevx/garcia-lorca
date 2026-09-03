// Package googleid verifies ID tokens minted by Google Cloud Identity Platform.
//
// It is the only place in iam that knows the provider is Google. Everything
// above it depends on ports.IDTokenVerifier, so swapping providers is one
// package, and the use case tests never touch the network.
//
// The cryptography is the Admin SDK's job: signature, issuer, audience, expiry.
// What lives here is the policy applied to a token the SDK already accepted.
package googleid

import (
	"context"

	firebase "firebase.google.com/go/v4"
	firebaseauth "firebase.google.com/go/v4/auth"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/ports"
)

// tokenVerifier is the slice of the Admin SDK's auth client this adapter uses.
// Declared here, where it is consumed, so the policy above can be tested
// without a project, a network or a key.
//
// The CHECKING variant, not plain VerifyIDToken. A plain verify accepts a token
// for the rest of its ~1h life even after the account's password was reset or
// the account was disabled at the provider — and this service would then mint a
// fresh 15-minute access token and a 24-hour refresh from it. That would quietly
// undo the revocation-on-password-change guarantee the password path was built
// to give (RefreshToken's password_changed_at floor, ADR 0005). Revoking has to
// mean revoked on both doors.
type tokenVerifier interface {
	VerifyIDTokenAndCheckRevoked(ctx context.Context, idToken string) (*firebaseauth.Token, error)
}

// Verifier implements ports.IDTokenVerifier over Identity Platform.
type Verifier struct {
	sdk tokenVerifier
}

// NewVerifier wraps an already-built SDK client. Prefer NewFromProject in
// production; this constructor exists so the policy is testable.
func NewVerifier(sdk tokenVerifier) *Verifier {
	return &Verifier{sdk: sdk}
}

// NewFromProject builds a Verifier for a Google Cloud project using Application
// Default Credentials — on Cloud Run, the runtime service account, with no key
// file anywhere.
func NewFromProject(ctx context.Context, projectID string) (*Verifier, error) {
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID})
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "identity provider init", err)
	}
	client, err := app.Auth(ctx)
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "identity provider auth client", err)
	}
	return &Verifier{sdk: client}, nil
}

// errUnauthorized is the ONE answer this adapter gives for every failure. A
// caller learns that verification failed and never why: which claim was missing,
// whether the signature or the audience was wrong, or whether the address is
// known here. Each of those is a probe somebody could run.
func errUnauthorized() error {
	return glerrors.New(glerrors.KindUnauthorized, "invalid credentials")
}

// Verify checks an ID token and reports the identity it proves.
func (v *Verifier) Verify(ctx context.Context, idToken string) (ports.VerifiedIdentity, error) {
	token, err := v.sdk.VerifyIDTokenAndCheckRevoked(ctx, idToken)
	if err != nil || token == nil {
		return ports.VerifiedIdentity{}, errUnauthorized()
	}

	email, _ := token.Claims["email"].(string)
	if email == "" {
		// No email means phone or anonymous sign-in. Both are disabled in the
		// provider config (deploy/terraform/identity_platform.tf); this is the
		// second lock on that door, because a config is one console click from
		// being different.
		return ports.VerifiedIdentity{}, errUnauthorized()
	}

	// VERIFIED, not merely present. Identity Platform accepts self sign-up by
	// default, so an unverified claim proves nothing about who holds the
	// mailbox: anyone could register a known address, present the token, and be
	// handed that person's session, because the user row does exist on our side.
	// A missing claim counts as unverified — absent is not true.
	verified, _ := token.Claims["email_verified"].(bool)
	if !verified {
		return ports.VerifiedIdentity{}, errUnauthorized()
	}

	return ports.VerifiedIdentity{Email: email, Subject: token.UID}, nil
}

// compile-time proof the adapter satisfies the port it exists for.
var _ ports.IDTokenVerifier = (*Verifier)(nil)
