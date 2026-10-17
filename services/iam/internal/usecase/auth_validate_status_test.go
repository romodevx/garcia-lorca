package usecase_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/usecase"
)

// ISSUE-CC-116 — suspending somebody did not end their session: the access
// token they already held kept validating until it expired (up to 15 minutes),
// because ValidateToken only checked the signature. The gateway asks iam on
// EVERY authenticated request, so that is where the account's current state is
// read: a suspended or deleted account stops at the very next request.
func TestValidateTokenFollowsTheAccountsCurrentStatus(t *testing.T) {
	cases := []struct {
		name     string
		status   domain.Status
		wantKind glerrors.Kind
	}{
		{name: "active account validates", status: domain.StatusActive, wantKind: glerrors.KindUnknown},
		{name: "suspended account is refused", status: domain.StatusSuspended, wantKind: glerrors.KindUnauthorized},
		{name: "deleted account is refused", status: domain.StatusDeleted, wantKind: glerrors.KindUnauthorized},
		{name: "invited account is refused", status: domain.StatusInvited, wantKind: glerrors.KindUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := fixtureService(t, tc.status)

			_, err := svc.ValidateToken(context.Background(), "access:u1")

			if tc.wantKind == glerrors.KindUnknown {
				if err != nil {
					t.Fatalf("ValidateToken: %v", err)
				}
				return
			}
			if glerrors.KindOf(err) != tc.wantKind {
				t.Errorf("err = %v (kind %v), want kind %v", err, glerrors.KindOf(err), tc.wantKind)
			}
		})
	}
}

func TestValidateTokenForAnAccountThatNoLongerExistsIsUnauthorized(t *testing.T) {
	svc, users := fixtureService(t, domain.StatusActive)
	delete(users.byID, "u1")

	if _, err := svc.ValidateToken(context.Background(), "access:u1"); glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Errorf("err = %v, want KindUnauthorized", err)
	}
}

// A database that failed to answer is NOT a revoked session. Reporting it as
// Unauthorized would sign everybody out on a connection blip; it stays a
// server fault the client retries.
func TestValidateTokenWhenTheStatusCannotBeReadIsAServerFault(t *testing.T) {
	svc, users := fixtureService(t, domain.StatusActive)
	users.findByIDErr = glerrors.New(glerrors.KindInternal, "pool exhausted")

	if _, err := svc.ValidateToken(context.Background(), "access:u1"); glerrors.KindOf(err) != glerrors.KindInternal {
		t.Errorf("err = %v, want KindInternal", err)
	}
}

func TestCheckPermissionRefusesASuspendedAccount(t *testing.T) {
	svc, _ := fixtureService(t, domain.StatusSuspended)

	if _, err := svc.CheckPermission(context.Background(), "access:u1", "students:read"); glerrors.KindOf(err) != glerrors.KindUnauthorized {
		t.Errorf("err = %v, want KindUnauthorized", err)
	}
}

// CC-401 — reactivating an account brought its OLD access token back to life:
// the status check passed again and the token was still within its fifteen
// minutes. Suspending moves password_changed_at, so an access token minted
// before it is refused exactly as RefreshToken refuses a refresh token: compared
// at whole seconds, so the session re-issued in the same second as the change
// survives.
func TestValidateTokenRefusesATokenMintedBeforeTheSessionFloor(t *testing.T) {
	floor := time.Date(2026, 10, 17, 12, 0, 0, 500_000_000, time.UTC) // sub-second, like the column
	cases := []struct {
		name     string
		floor    time.Time
		issuedAt time.Time
		wantOK   bool
	}{
		{name: "no floor yet", floor: time.Time{}, issuedAt: floor, wantOK: true},
		{name: "minted a second before the floor", floor: floor, issuedAt: floor.Truncate(time.Second).Add(-time.Second), wantOK: false},
		{name: "minted long before the floor", floor: floor, issuedAt: floor.Add(-time.Hour), wantOK: false},
		{name: "minted in the floor's own second", floor: floor, issuedAt: floor.Truncate(time.Second), wantOK: true},
		{name: "minted after the floor", floor: floor, issuedAt: floor.Add(time.Minute), wantOK: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, users, tokens := validateFixture(t)
			users.byID["u1"] = users.byID["u1"].WithPasswordChangedAt(tc.floor)
			tokens.accessIssuedAt = tc.issuedAt

			_, err := svc.ValidateToken(context.Background(), "access:u1")

			if tc.wantOK && err != nil {
				t.Fatalf("ValidateToken: %v", err)
			}
			if !tc.wantOK && glerrors.KindOf(err) != glerrors.KindUnauthorized {
				t.Errorf("err = %v, want KindUnauthorized", err)
			}
		})
	}
}

// validateFixture is fixtureService (active u1) with the token double in reach.
func validateFixture(t *testing.T) (*usecase.AuthService, *fakeUsers, *fakeTokens) {
	t.Helper()
	_, users := fixtureService(t, domain.StatusActive)
	tokens := &fakeTokens{}
	roles := &fakeRoles{byID: map[string]domain.Role{}}
	now := func() time.Time { return time.Date(2026, 10, 17, 12, 5, 0, 0, time.UTC) }
	svc := usecase.NewAuthService(users, roles, fakeHasher{}, tokens, tokens, nil, nil, testTenant, 15*time.Minute, now)
	return svc, users, tokens
}
