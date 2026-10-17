package usecase_test

import (
	"context"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
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
