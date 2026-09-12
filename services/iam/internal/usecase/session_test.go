package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"

	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/usecase"
)

const (
	sessionEmail    = "ana@garcialorca.mx"
	sessionPassword = "secret"
)

// newAuthServiceForSession wires a service over the shared fakes and hands back
// the token issuer, which is where the session kind is observable.
func newAuthServiceForSession(t *testing.T) (*usecase.AuthService, *fakeTokens) {
	t.Helper()
	svc, _, tokens := newAuthServiceWithUsersForSession(t)
	return svc, tokens
}

// newAuthServiceWithUsersForSession also hands back the user repository, for the
// tests that need to make it fail or make a user disappear.
func newAuthServiceWithUsersForSession(t *testing.T) (*usecase.AuthService, *fakeUsers, *fakeTokens) {
	t.Helper()
	email, _ := domain.NewEmail(sessionEmail)
	role, _ := domain.NewRole("r1", "Docente", []string{"students:read"})
	user, _ := domain.NewUser("u1", testTenant, email, "Ana", domain.StatusActive, false, []domain.Role{role})
	users := &fakeUsers{
		byEmail: map[domain.Email]struct {
			user domain.User
			hash string
		}{email: {user: user, hash: "h:" + sessionPassword}},
		byID: map[string]domain.User{"u1": user},
	}
	tokens := &fakeTokens{}
	now := func() time.Time { return time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC) }
	svc := usecase.NewAuthService(users, &fakeRoles{}, fakeHasher{}, tokens, tokens, nil, nil, testTenant, 15*time.Minute, now)
	return svc, users, tokens
}

// The school's panel gets the short session and the family app the long one, and
// the difference is decided at login by the surface — never by the caller asking
// for more.
func TestAuthenticateIssuesTheSessionKindItWasAskedFor(t *testing.T) {
	svc, tokens := newAuthServiceForSession(t)

	if _, err := svc.AuthenticateAs(context.Background(), sessionEmail, sessionPassword, domain.SessionFamily); err != nil {
		t.Fatalf("AuthenticateAs(family): %v", err)
	}
	if tokens.lastKind != domain.SessionFamily {
		t.Errorf("issued a %v session, want family", tokens.lastKind)
	}

	if _, err := svc.AuthenticateAs(context.Background(), sessionEmail, sessionPassword, domain.SessionStaff); err != nil {
		t.Fatalf("AuthenticateAs(staff): %v", err)
	}
	if tokens.lastKind != domain.SessionStaff {
		t.Errorf("issued a %v session, want staff", tokens.lastKind)
	}
}

// Rotating keeps the kind. The two lifetimes are equal today, so nothing breaks
// if this slips right now — which is exactly why it is pinned. The kind is the
// seam the surfaces diverge along, and the last time they did, a rotation that
// forgot it was the difference between a parent's phone and the office panel.
func TestRefreshKeepsTheSessionKind(t *testing.T) {
	svc, tokens := newAuthServiceForSession(t)

	family, err := svc.AuthenticateAs(context.Background(), sessionEmail, sessionPassword, domain.SessionFamily)
	if err != nil {
		t.Fatalf("AuthenticateAs: %v", err)
	}
	tokens.lastKind = domain.SessionStaff // so a miss is visible

	if _, err := svc.RefreshToken(context.Background(), family.RefreshToken); err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if tokens.lastKind != domain.SessionFamily {
		t.Errorf("the rotated session became %v, want family", tokens.lastKind)
	}
}

// The plain Login keeps meaning what it meant, and what it meant is the panel.
func TestAuthenticateDefaultsToTheStaffSession(t *testing.T) {
	svc, tokens := newAuthServiceForSession(t)

	if _, err := svc.Authenticate(context.Background(), sessionEmail, sessionPassword); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if tokens.lastKind != domain.SessionStaff {
		t.Errorf("plain Authenticate issued a %v session, want staff", tokens.lastKind)
	}
}

// A DATABASE FAILURE IS NOT A REFUSED SESSION, and conflating the two is how
// somebody gets thrown out of a session that was perfectly valid.
//
// The repository already tells them apart — translateFindErr returns ErrNotFound
// for a missing row and KindInternal for a query that failed — and this service
// used to flatten both into "invalid refresh token". The gateway turned that
// into 401, the web read 401 as "your session is over" and made the person type
// their password again, for a database that was merely busy.
//
// It matters most exactly when the school is busiest: every active session
// rotates on a timer, each rotation takes a connection from a small pool, and
// the more people are working the likelier one of those rotations is the one
// that fails. The failure looked like other people's logins ending your session.
//
// KindInternal leaves here, becomes a 500, and a 500 is retried rather than
// obeyed.
func TestARefreshDoesNotEndTheSessionWhenTheDatabaseFails(t *testing.T) {
	svc, users, _ := newAuthServiceWithUsersForSession(t)

	session, err := svc.AuthenticateAs(context.Background(), sessionEmail, sessionPassword, domain.SessionStaff)
	if err != nil {
		t.Fatalf("AuthenticateAs: %v", err)
	}

	users.findByIDErr = glerrors.Wrap(glerrors.KindInternal, "query failed", errors.New("connection reset"))

	_, err = svc.RefreshToken(context.Background(), session.RefreshToken)
	if got := glerrors.KindOf(err); got != glerrors.KindInternal {
		t.Errorf("a failing database made the refresh answer %v, want KindInternal — %v reads as \"your session is over\"", got, got)
	}
}

// The other half of the same line, and the one that must not regress: a refresh
// token for a user who is genuinely gone IS a refused session, and has to stay
// a generic refusal that says nothing about why.
func TestARefreshIsRefusedWhenTheUserIsGone(t *testing.T) {
	svc, users, _ := newAuthServiceWithUsersForSession(t)

	session, err := svc.AuthenticateAs(context.Background(), sessionEmail, sessionPassword, domain.SessionStaff)
	if err != nil {
		t.Fatalf("AuthenticateAs: %v", err)
	}
	delete(users.byID, "u1")

	_, err = svc.RefreshToken(context.Background(), session.RefreshToken)
	if got := glerrors.KindOf(err); got != glerrors.KindUnauthorized {
		t.Errorf("refresh for a deleted user answered %v, want KindUnauthorized", got)
	}
}
