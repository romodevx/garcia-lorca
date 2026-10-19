package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/ports"
	"github.com/garcia-lorca/services/iam/internal/usecase"
)

// T2 — deleting a user. A soft delete: the account can no longer sign in and
// leaves the listings, and its row stays, because payments, grades and the
// audit trail name it by id.

const (
	directorID = "u-director"
	teacherID  = "u-teacher"
)

type deleteFixture struct {
	users *fakeUsers
	prov  *fakeProvisioner
	inv   *fakeInvitations
}

func mustUserUC(t *testing.T, id, email string, status domain.Status, roles ...domain.Role) domain.User {
	t.Helper()
	addr, err := domain.NewEmail(email)
	if err != nil {
		t.Fatalf("NewEmail: %v", err)
	}
	u, err := domain.NewUser(id, "t1", addr, id, status, false, roles)
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	return u
}

func ownerRole(t *testing.T) domain.Role {
	return mustRoleUC(t, domain.OwnerRoleID, "owner", []string{"users:read", "users:write", "users:delete"})
}

func teacherRole(t *testing.T) domain.Role {
	return mustRoleUC(t, "r1", "maestro", []string{"grades:create"})
}

// newDeleteFixture seeds an active director (owner) and an active teacher.
func newDeleteFixture(t *testing.T, extra ...domain.User) deleteFixture {
	t.Helper()
	users := &fakeUsers{byID: map[string]domain.User{}, byEmail: map[domain.Email]struct {
		user domain.User
		hash string
	}{}}
	seed := append([]domain.User{
		mustUserUC(t, directorID, "director@gl.mx", domain.StatusActive, ownerRole(t)),
		mustUserUC(t, teacherID, "maestra@gl.mx", domain.StatusActive, teacherRole(t)),
	}, extra...)
	for _, u := range seed {
		users.byID[u.ID()] = u
		users.byEmail[u.Email()] = struct {
			user domain.User
			hash string
		}{user: u}
	}
	return deleteFixture{users: users, prov: &fakeProvisioner{}, inv: &fakeInvitations{}}
}

func (f deleteFixture) service(t *testing.T) *usecase.AdminService {
	return newInviteService(t, f.users, f.prov, f.inv, &fakeMailer{})
}

func TestDeleteUserSoftDeletesRevokesAndDisablesAtProvider(t *testing.T) {
	f := newDeleteFixture(t)

	if err := f.service(t).DeleteUser(context.Background(), directorID, teacherID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	if got := f.users.deletedBy[teacherID]; got != directorID {
		t.Errorf("deleted by %q, want the director %q (who did it is part of the record)", got, directorID)
	}
	if len(f.inv.revoked) != 1 || f.inv.revoked[0] != teacherID {
		t.Errorf("revoked invitations of %v, want [%s] — a link in the mailbox must not outlive the deletion", f.inv.revoked, teacherID)
	}
	if len(f.prov.disabled) != 1 || f.prov.disabled[0] != "maestra@gl.mx:true" {
		t.Errorf("provider calls %v, want the account disabled at the identity provider", f.prov.disabled)
	}
}

func TestDeleteUserRefusals(t *testing.T) {
	adminRole := mustRoleUC(t, "r-admin", "admin", []string{"users:delete"})
	cases := []struct {
		name     string
		extra    []domain.User
		actor    string
		target   string
		wantKind glerrors.Kind
	}{
		{
			name:     "a user cannot delete themself",
			actor:    directorID,
			target:   directorID,
			wantKind: glerrors.KindForbidden,
		},
		{
			// Postgres reads a uuid in any case or shape, so comparing the
			// raw strings let a director delete themself by upper-casing
			// their own id. The guard compares the loaded rows.
			name:     "a user cannot delete themself under another spelling of their id",
			actor:    directorID,
			target:   strings.ToUpper(directorID),
			wantKind: glerrors.KindForbidden,
		},
		{
			name:     "a caller without users:delete is refused",
			actor:    teacherID,
			target:   directorID,
			wantKind: glerrors.KindForbidden,
		},
		{
			name:     "a suspended caller is refused even holding the permission",
			extra:    []domain.User{mustUserUC(t, "u-susp", "susp@gl.mx", domain.StatusSuspended, ownerRole(t))},
			actor:    "u-susp",
			target:   teacherID,
			wantKind: glerrors.KindForbidden,
		},
		{
			name:     "an unknown target is not found",
			actor:    directorID,
			target:   "u-nobody",
			wantKind: glerrors.KindNotFound,
		},
		{
			// Defense in depth: users:delete is owner-only, so in production
			// the caller is itself an active owner. Should that ever loosen,
			// the rule still holds.
			name:     "the last active owner cannot be deleted",
			extra:    []domain.User{mustUserUC(t, "u-admin", "admin@gl.mx", domain.StatusActive, adminRole)},
			actor:    "u-admin",
			target:   directorID,
			wantKind: glerrors.KindConflict,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newDeleteFixture(t, tc.extra...)

			err := f.service(t).DeleteUser(context.Background(), tc.actor, tc.target)

			if glerrors.KindOf(err) != tc.wantKind {
				t.Fatalf("err = %v (kind %v), want kind %v", err, glerrors.KindOf(err), tc.wantKind)
			}
			if len(f.users.deletedBy) != 0 || len(f.prov.disabled) != 0 || len(f.inv.revoked) != 0 {
				t.Errorf("a refused deletion touched something: deleted=%v provider=%v revoked=%v",
					f.users.deletedBy, f.prov.disabled, f.inv.revoked)
			}
		})
	}
}

func TestDeleteUserAnotherOwnerWhileOneRemains(t *testing.T) {
	f := newDeleteFixture(t, mustUserUC(t, "u-owner2", "sub@gl.mx", domain.StatusActive, ownerRole(t)))

	if err := f.service(t).DeleteUser(context.Background(), directorID, "u-owner2"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if f.users.deletedBy["u-owner2"] != directorID {
		t.Errorf("second owner not deleted: %v", f.users.deletedBy)
	}
}

func TestDeleteUserProviderOutcomes(t *testing.T) {
	cases := []struct {
		name    string
		prov    ports.IdentityProvisioner
		wantErr bool
	}{
		{name: "no provider configured", prov: nil},
		{
			// Created while passwords were local: nothing to disable there.
			name: "no account at the provider",
			prov: &fakeProvisioner{disabledErr: glerrors.New(glerrors.KindNotFound, "no account")},
		},
		{
			// The row goes first (it is what every sign-in path checks); a
			// provider that fails afterwards is reported with its own code so
			// the screen can say exactly that and offer the retry.
			name:    "provider unavailable",
			prov:    &fakeProvisioner{disabledErr: glerrors.New(glerrors.KindInternal, "503")},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newDeleteFixture(t)
			svc := newInviteService(t, f.users, tc.prov, f.inv, &fakeMailer{})

			err := svc.DeleteUser(context.Background(), directorID, teacherID)

			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if f.users.deletedBy[teacherID] != directorID {
				t.Errorf("the row must be deleted whatever the provider says: %v", f.users.deletedBy)
			}
			if tc.wantErr && !errors.Is(err, domain.ErrDeletionPending) {
				t.Errorf("err = %v, want it marked ErrDeletionPending", err)
			}
		})
	}
}

// Deleting an already-deleted account is how the director retries after the
// provider half failed: the row is not written again, the links and the
// provider account are switched off (again), and it answers success.
func TestDeleteUserAlreadyDeletedRetriesTheRest(t *testing.T) {
	f := newDeleteFixture(t, mustUserUC(t, "u-gone", "gone@gl.mx", domain.StatusDeleted, teacherRole(t)))

	if err := f.service(t).DeleteUser(context.Background(), directorID, "u-gone"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if len(f.users.deletedBy) != 0 {
		t.Errorf("an already-deleted row was written again: %v", f.users.deletedBy)
	}
	if len(f.prov.disabled) != 1 || f.prov.disabled[0] != "gone@gl.mx:true" {
		t.Errorf("provider calls %v, want one disable", f.prov.disabled)
	}
	if len(f.inv.revoked) != 1 {
		t.Errorf("revoked %v, want the links revoked again", f.inv.revoked)
	}
}

// --- a deleted account is out of reach of the other admin actions ----------

func TestSetUserStatusCannotResurrectADeletedUser(t *testing.T) {
	f := newDeleteFixture(t, mustUserUC(t, "u-gone", "gone@gl.mx", domain.StatusDeleted, teacherRole(t)))

	err := f.service(t).SetUserStatus(context.Background(), directorID, "u-gone", domain.StatusActive)

	if glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("err = %v, want KindNotFound — users:write must not undo an owner-only deletion", err)
	}
}

func TestAcceptResetOnADeletedUserIsDead(t *testing.T) {
	f := newDeleteFixture(t, mustUserUC(t, "u-gone", "gone@gl.mx", domain.StatusDeleted, teacherRole(t)))
	reset, err := domain.NewInvitation("inv1", "u-gone", "hash", domain.PurposeReset, testNow.Add(domain.ResetLifetime), time.Time{})
	if err != nil {
		t.Fatalf("NewInvitation: %v", err)
	}
	f.inv.consume = reset

	err = f.service(t).AcceptInvitation(context.Background(), "token", testPassword)

	if glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("err = %v, want the dead-link answer", err)
	}
	if f.prov.passwordFor != "" {
		t.Errorf("a deleted account had its password written at the provider (%q)", f.prov.passwordFor)
	}
}

// --- owner answer 3: inviting a deleted address brings the SAME account back

func TestInviteDeletedEmailReactivatesTheSameAccount(t *testing.T) {
	f := newDeleteFixture(t, mustUserUC(t, "u-gone", "gone@gl.mx", domain.StatusDeleted, teacherRole(t)))

	got, err := f.service(t).InviteUser(context.Background(), "gone@gl.mx", "Vuelve", []string{"r1"})
	if err != nil {
		t.Fatalf("InviteUser: %v", err)
	}

	if got.User.ID() != "u-gone" {
		t.Errorf("user id = %q, want the old account u-gone (its history hangs off that id)", got.User.ID())
	}
	if len(f.users.created) != 0 {
		t.Errorf("a new row was created: %v", f.users.created)
	}
	if f.users.restored["u-gone"] != "Vuelve" {
		t.Errorf("restored = %v, want u-gone restored with the new display name", f.users.restored)
	}
	// The roles are replaced in the SAME write that restores the row: a
	// restored account must never come back holding what it held before.
	if roles := f.users.restoredRole["u-gone"]; len(roles) != 1 || roles[0] != "r1" {
		t.Errorf("roles = %v, want the roles of this invitation, set by Restore itself", roles)
	}
	// Re-enabled WITHOUT its old password: whoever knew it must not walk
	// back in past the invitation.
	if len(f.prov.disabled) != 1 || f.prov.disabled[0] != "gone@gl.mx:reenabled" {
		t.Errorf("provider calls %v, want the account re-enabled with its credential replaced", f.prov.disabled)
	}
	if len(f.inv.revoked) != 1 || f.inv.revoked[0] != "u-gone" {
		t.Errorf("revoked %v, want every old link of u-gone dead before the new one", f.inv.revoked)
	}
	if len(f.prov.created) != 0 {
		t.Errorf("a second provider account was created: %v", f.prov.created)
	}
	if len(f.inv.issued) != 1 || f.inv.issued[0].UserID() != "u-gone" {
		t.Errorf("issued %v, want one invitation for u-gone", f.inv.issued)
	}
}

func TestInviteDeletedEmailWithNoProviderAccountCreatesOne(t *testing.T) {
	f := newDeleteFixture(t, mustUserUC(t, "u-gone", "gone@gl.mx", domain.StatusDeleted, teacherRole(t)))
	f.prov.reenableErr = glerrors.New(glerrors.KindNotFound, "no account")

	if _, err := f.service(t).InviteUser(context.Background(), "gone@gl.mx", "Vuelve", []string{"r1"}); err != nil {
		t.Fatalf("InviteUser: %v", err)
	}
	if len(f.prov.created) != 1 || f.prov.created[0] != "gone@gl.mx" {
		t.Errorf("provider created %v, want the account created there", f.prov.created)
	}
}

func TestInviteActiveEmailIsStillAConflict(t *testing.T) {
	f := newDeleteFixture(t)

	_, err := f.service(t).InviteUser(context.Background(), "maestra@gl.mx", "Otra", []string{"r1"})

	if glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("err = %v, want KindConflict", err)
	}
	if len(f.users.restored) != 0 {
		t.Errorf("a live account was restored: %v", f.users.restored)
	}
}

// The temporary-password route cannot restore anybody; it says where to go
// instead of "already exists" about a user nobody can see in the list.
func TestCreateUserOnADeletedAddressPointsToInviting(t *testing.T) {
	f := newDeleteFixture(t, mustUserUC(t, "u-gone", "gone@gl.mx", domain.StatusDeleted, teacherRole(t)))
	svc := newInviteService(t, f.users, nil, f.inv, &fakeMailer{})

	_, err := svc.CreateUser(context.Background(), "gone@gl.mx", "Vuelve", "temporal-123", nil)

	if glerrors.KindOf(err) != glerrors.KindConflict || !strings.Contains(glerrors.SafeMessage(err), "invít") {
		t.Errorf("err = %v, want a conflict that says to invite the address", err)
	}
}

// Only "no such account" falls through; a lookup that fails is the answer.
func TestCreateUserPropagatesALookupFailure(t *testing.T) {
	f := newDeleteFixture(t)
	f.users.findByEmailErr = glerrors.New(glerrors.KindInternal, "db down")
	svc := newInviteService(t, f.users, nil, f.inv, &fakeMailer{})

	_, err := svc.CreateUser(context.Background(), "nueva@gl.mx", "Nueva", "temporal-123", nil)

	if glerrors.KindOf(err) != glerrors.KindInternal {
		t.Errorf("err = %v, want the lookup failure", err)
	}
}
