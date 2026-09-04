package usecase_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/ports"
	"github.com/garcia-lorca/services/iam/internal/usecase"
)

/*
Signing in with an identity provider's ID token (F2 of
.claude/docs/PLAN-identity-platform.md).

The exchange happens HERE, not at the edge. The client authenticates against
Identity Platform, hands us the resulting ID token once, and we mint OUR OWN
access and refresh pair — the same one `Authenticate` issues for a password.

That is the whole point of doing it this way. The gateway, its middleware, the
HttpOnly cookies and the mobile Bearer flow all keep working untouched, and the
session keeps our TTLs and our "a password change revokes older refresh tokens"
rule. What moves to the provider is password STORAGE, recovery and MFA — not the
session.

The verifier is a port, so these tests never touch the network: they assert what
the use case does with a verdict, not how the signature is checked.
*/

// fakeIDTokens implements ports.IDTokenVerifier. `verified` maps a raw token to
// the identity it proves; anything absent is rejected the way a bad signature,
// a wrong audience or an expired token would be.
type fakeIDTokens struct {
	verified map[string]ports.VerifiedIdentity
	calls    int
}

func (f *fakeIDTokens) Verify(_ context.Context, idToken string) (ports.VerifiedIdentity, error) {
	f.calls++
	id, ok := f.verified[idToken]
	if !ok {
		return ports.VerifiedIdentity{}, glerrors.New(glerrors.KindUnauthorized, "invalid id token")
	}
	return id, nil
}

// idTokenService builds an AuthService whose only user is Ana, in the given
// status, and whose verifier accepts exactly one token: "google:ana".
func idTokenService(t *testing.T, status domain.Status) (*usecase.AuthService, *fakeIDTokens) {
	t.Helper()
	email, _ := domain.NewEmail("ana@garcialorca.mx")
	role, _ := domain.NewRole("r1", "Docente", []string{"students:read"})
	user, _ := domain.NewUser("u1", testTenant, email, "Ana", status, false, []domain.Role{role})
	users := &fakeUsers{
		byEmail: map[domain.Email]struct {
			user domain.User
			hash string
		}{email: {user: user, hash: "h:secret"}},
		byID: map[string]domain.User{"u1": user},
	}
	verifier := &fakeIDTokens{verified: map[string]ports.VerifiedIdentity{
		"google:ana": {Email: "ana@garcialorca.mx", Subject: "google-uid-1"},
	}}
	now := func() time.Time { return time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC) }
	svc := usecase.NewAuthService(
		users, &fakeRoles{}, fakeHasher{}, &fakeTokens{}, &fakeTokens{},
		verifier, testTenant, 15*time.Minute, now,
	)
	return svc, verifier
}

// changePasswordFixture builds the collaborators for a password-path test.
func changePasswordFixture(t *testing.T) (*fakeUsers, *fakeRoles, fakeHasher, *fakeTokens) {
	t.Helper()
	email, _ := domain.NewEmail("ana@garcialorca.mx")
	role, _ := domain.NewRole("r1", "Docente", []string{"students:read"})
	user, _ := domain.NewUser("u1", testTenant, email, "Ana", domain.StatusActive, false, []domain.Role{role})
	users := &fakeUsers{
		byEmail: map[domain.Email]struct {
			user domain.User
			hash string
		}{email: {user: user, hash: "h:secret"}},
		byID: map[string]domain.User{"u1": user},
	}
	return users, &fakeRoles{}, fakeHasher{}, &fakeTokens{}
}

func TestAuthenticateWithIDTokenIssuesOurOwnSession(t *testing.T) {
	svc, verifier := idTokenService(t, domain.StatusActive)

	res, err := svc.AuthenticateWithIDToken(context.Background(), "google:ana", domain.SessionStaff)
	if err != nil {
		t.Fatalf("AuthenticateWithIDToken: %v", err)
	}
	// OUR tokens, not the provider's. If this ever returned the ID token back,
	// the session would live and die on Google's clock instead of ours.
	if res.AccessToken != "access:u1" {
		t.Errorf("access token = %q, want our own minted token", res.AccessToken)
	}
	if res.RefreshToken != "refresh:u1" {
		t.Errorf("refresh token = %q, want our own minted token", res.RefreshToken)
	}
	if res.ExpiresIn != int64((15 * time.Minute).Seconds()) {
		t.Errorf("expiresIn = %d, want our access TTL", res.ExpiresIn)
	}
	if verifier.calls != 1 {
		t.Errorf("verifier called %d times, want exactly 1", verifier.calls)
	}
}

func TestAuthenticateWithIDTokenRejectsUnverifiableToken(t *testing.T) {
	svc, _ := idTokenService(t, domain.StatusActive)

	_, err := svc.AuthenticateWithIDToken(context.Background(), "google:forged", domain.SessionStaff)
	if glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Fatalf("error = %v, want KindUnauthorized", err)
	}
}

func TestAuthenticateWithIDTokenRejectsUnknownUser(t *testing.T) {
	// A token the provider vouches for, belonging to somebody with no user row
	// here. Authenticating at the provider is not authorisation to enter: the
	// roles and permissions live in OUR database, and without a row there is
	// nothing to be.
	svc, verifier := idTokenService(t, domain.StatusActive)
	verifier.verified["google:stranger"] = ports.VerifiedIdentity{Email: "nadie@garcialorca.mx", Subject: "google-uid-9"}

	_, err := svc.AuthenticateWithIDToken(context.Background(), "google:stranger", domain.SessionStaff)
	if glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Fatalf("error = %v, want KindUnauthorized (never leak that the email is unknown)", err)
	}
}

func TestAuthenticateWithIDTokenRejectsSuspendedUser(t *testing.T) {
	svc, _ := idTokenService(t, domain.StatusSuspended)

	// Suspending an account has to hold whatever door the person knocks on.
	// A provider that still believes in them is not our problem.
	_, err := svc.AuthenticateWithIDToken(context.Background(), "google:ana", domain.SessionStaff)
	if glerrors.KindOf(err) != glerrors.KindForbidden {
		t.Fatalf("error = %v, want KindForbidden", err)
	}
}

func TestAuthenticateWithIDTokenFailsClosedWithoutVerifier(t *testing.T) {
	email, _ := domain.NewEmail("ana@garcialorca.mx")
	role, _ := domain.NewRole("r1", "Docente", []string{"students:read"})
	user, _ := domain.NewUser("u1", testTenant, email, "Ana", domain.StatusActive, false, []domain.Role{role})
	users := &fakeUsers{
		byEmail: map[domain.Email]struct {
			user domain.User
			hash string
		}{email: {user: user, hash: "h:secret"}},
		byID: map[string]domain.User{"u1": user},
	}
	now := func() time.Time { return time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC) }
	// nil verifier: the environment has no identity provider configured.
	svc := usecase.NewAuthService(
		users, &fakeRoles{}, fakeHasher{}, &fakeTokens{}, &fakeTokens{},
		nil, testTenant, 15*time.Minute, now,
	)

	// Fail CLOSED. An unconfigured provider must refuse, never wave people
	// through on the strength of an unchecked token.
	_, err := svc.AuthenticateWithIDToken(context.Background(), "google:ana", domain.SessionStaff)
	if err == nil {
		t.Fatal("want an error when no verifier is configured, got nil")
	}
	if glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Fatalf("error = %v, want KindUnauthorized", err)
	}
}

func TestChangePasswordRefusesWhenTheProviderOwnsCredentials(t *testing.T) {
	svc, _ := idTokenService(t, domain.StatusActive)

	// Once the provider authenticates people, OUR stored password is no longer
	// the credential that opens the door. Writing a new one to Postgres would
	// report success, clear must_change_password, and change NOTHING about what
	// actually lets somebody in.
	//
	// That is worse than having no rotation at all. Somebody whose password was
	// compromised rotates it, sees a confirmation, and closes the incident —
	// while the attacker's copy keeps working. A refusal is honest; a silent
	// no-op is a lie the user acts on.
	err := svc.ChangePassword(context.Background(), "access:u1", "vieja", "nueva")
	if err == nil {
		t.Fatal("want a refusal while an identity provider is configured, got nil")
	}
	if glerrors.KindOf(err) != glerrors.KindInternal {
		t.Fatalf("error = %v (kind %v), want KindInternal — the caller did nothing wrong",
			err, glerrors.KindOf(err))
	}
}

func TestChangePasswordStillWorksWithoutAProvider(t *testing.T) {
	// The password path is untouched wherever the provider is not configured:
	// local development, and every environment before the cutover. This test is
	// what keeps the refusal above from becoming a blanket outage.
	users, roles, hasher, tokens := changePasswordFixture(t)
	svc := usecase.NewAuthService(users, roles, hasher, tokens, tokens, nil, testTenant, 15*time.Minute,
		func() time.Time { return time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC) })

	if err := svc.ChangePassword(context.Background(), "access:u1", "secret", "nueva-larga-123"); err != nil {
		t.Fatalf("ChangePassword without a provider: %v", err)
	}
}

func TestAuthenticateWithIDTokenNeverDemandsAPasswordChange(t *testing.T) {
	// A user row still carrying must_change_password from the password era.
	email, _ := domain.NewEmail("ana@garcialorca.mx")
	role, _ := domain.NewRole("r1", "Docente", []string{"students:read"})
	user, _ := domain.NewUser("u1", testTenant, email, "Ana", domain.StatusActive, true, []domain.Role{role})
	users := &fakeUsers{
		byEmail: map[domain.Email]struct {
			user domain.User
			hash string
		}{email: {user: user, hash: "h:secret"}},
		byID: map[string]domain.User{"u1": user},
	}
	verifier := &fakeIDTokens{verified: map[string]ports.VerifiedIdentity{
		"google:ana": {Email: "ana@garcialorca.mx", Subject: "google-uid-1"},
	}}
	svc := usecase.NewAuthService(users, &fakeRoles{}, fakeHasher{}, &fakeTokens{}, &fakeTokens{},
		verifier, testTenant, 15*time.Minute,
		func() time.Time { return time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC) })

	res, err := svc.AuthenticateWithIDToken(context.Background(), "google:ana", domain.SessionStaff)
	if err != nil {
		t.Fatalf("AuthenticateWithIDToken: %v", err)
	}

	// This trapped a real user, and only running it found the trap: signing in
	// through the provider answered mustChangePassword=true, the app routed her
	// to the change-password screen, and ChangePassword answered 500 because the
	// provider owns credentials now. No way forward, no way in.
	//
	// The flag is a leftover from the password era. Where the provider holds the
	// credential there is no password of ours to change, so the honest answer is
	// false — password lifecycle belongs to the provider's own reset flow.
	if res.MustChangePassword {
		t.Fatal("the provider path demanded a password change that cannot be performed")
	}
}
