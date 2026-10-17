package usecase_test

import (
	"context"
	"strings"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
)

// ISSUE-CC-117 — the school's only director could suspend themself, and within
// one access-token lifetime the school had no owner at all: refresh and login
// both refuse a suspended account. Suspending now follows the same two rules
// deleting already did: nobody suspends themself, and the last active owner
// stays.

const otherDirectorID = "u-director-2"

func TestSetUserStatusSuspensionRefusals(t *testing.T) {
	adminRole := mustRoleUC(t, "r-admin", "admin", []string{"users:write"})
	cases := []struct {
		name     string
		extra    []domain.User
		actor    string
		target   string
		wantKind glerrors.Kind
		wantText string
	}{
		{
			name:     "a user cannot suspend themself",
			actor:    directorID,
			target:   directorID,
			wantKind: glerrors.KindForbidden,
			wantText: "no puedes suspender tu propia cuenta",
		},
		{
			name:     "a user cannot suspend themself under another spelling of their id",
			actor:    directorID,
			target:   strings.ToUpper(directorID),
			wantKind: glerrors.KindForbidden,
			wantText: "no puedes suspender tu propia cuenta",
		},
		{
			// A delegated administrator holding users:write is not the owner,
			// so the self rule does not catch it: the repository's last-holder
			// rule does.
			name:     "the last active owner cannot be suspended by somebody else",
			extra:    []domain.User{mustUserUC(t, "u-admin", "admin@gl.mx", domain.StatusActive, adminRole)},
			actor:    "u-admin",
			target:   directorID,
			wantKind: glerrors.KindConflict,
			wantText: "no se puede suspender al último director activo",
		},
		{
			name:     "a target that does not exist is not found",
			actor:    directorID,
			target:   "u-nobody",
			wantKind: glerrors.KindNotFound,
		},
		{
			name:     "no caller is refused",
			actor:    "",
			target:   teacherID,
			wantKind: glerrors.KindUnauthorized,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newDeleteFixture(t, tc.extra...)

			err := f.service(t).SetUserStatus(context.Background(), tc.actor, tc.target, domain.StatusSuspended)

			if glerrors.KindOf(err) != tc.wantKind {
				t.Fatalf("err = %v (kind %v), want kind %v", err, glerrors.KindOf(err), tc.wantKind)
			}
			if tc.wantText != "" && !strings.Contains(glerrors.SafeMessage(err), tc.wantText) {
				t.Errorf("message = %q, want it to say %q", glerrors.SafeMessage(err), tc.wantText)
			}
			if len(f.users.statusSet) != 0 {
				t.Errorf("a refused suspension wrote %v", f.users.statusSet)
			}
			if len(f.inv.revoked) != 0 {
				t.Errorf("a refused suspension revoked links %v", f.inv.revoked)
			}
		})
	}
}

func TestSetUserStatusSuspendsAnotherOwnerWhenOneStaysActive(t *testing.T) {
	f := newDeleteFixture(t, mustUserUC(t, otherDirectorID, "otro@gl.mx", domain.StatusActive, ownerRole(t)))

	if err := f.service(t).SetUserStatus(context.Background(), directorID, otherDirectorID, domain.StatusSuspended); err != nil {
		t.Fatalf("SetUserStatus: %v", err)
	}
	if got := f.users.statusSet[otherDirectorID]; got != domain.StatusSuspended {
		t.Errorf("status = %v, want suspended", got)
	}
	if got := f.users.statusKeep[otherDirectorID]; got != domain.OwnerRoleID {
		t.Errorf("last-holder rule keyed on %q, want the owner role %q", got, domain.OwnerRoleID)
	}
}

func TestSetUserStatusSuspendsATeacher(t *testing.T) {
	f := newDeleteFixture(t)

	if err := f.service(t).SetUserStatus(context.Background(), directorID, teacherID, domain.StatusSuspended); err != nil {
		t.Fatalf("SetUserStatus: %v", err)
	}
	if got := f.users.statusSet[teacherID]; got != domain.StatusSuspended {
		t.Errorf("status = %v, want suspended", got)
	}
}

// Reactivating is never refused by the suspension rules: it adds an active
// holder, it cannot remove the last one.
func TestSetUserStatusReactivationSkipsTheSuspensionGuards(t *testing.T) {
	f := newDeleteFixture(t, mustUserUC(t, otherDirectorID, "otro@gl.mx", domain.StatusSuspended, ownerRole(t)))

	if err := f.service(t).SetUserStatus(context.Background(), directorID, otherDirectorID, domain.StatusActive); err != nil {
		t.Fatalf("SetUserStatus: %v", err)
	}
	if got := f.users.statusSet[otherDirectorID]; got != domain.StatusActive {
		t.Errorf("status = %v, want active", got)
	}
}
