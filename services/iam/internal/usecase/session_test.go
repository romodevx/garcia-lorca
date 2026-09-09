package usecase_test

import (
	"context"
	"testing"
	"time"

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
	return svc, tokens
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

// Rotating keeps the kind. A family session that renewed itself into a staff one
// would log a parent out every fifteen minutes; a staff session that renewed
// into a family one would leave the office panel open all day, which is the
// failure that matters.
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
