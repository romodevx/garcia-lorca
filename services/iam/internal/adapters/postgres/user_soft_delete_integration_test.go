//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/iam/internal/adapters/postgres"
	"github.com/garcia-lorca/services/iam/internal/domain"
)

// T2 — soft delete. The row stays (history names it by id); the account leaves
// the listings, cannot authenticate, and loses every session it held.

func TestSoftDeleteHidesTheUserAndKeepsTheRow(t *testing.T) {
	ctx := context.Background()
	uid := "d1d1d1d1-0000-0000-0000-000000000001"
	actor := "d1d1d1d1-0000-0000-0000-0000000000ac"
	seedUser(t, uid, tenantA, "gone@garcialorca.mx", "x", "active")
	before := time.Now().Add(-time.Second)

	repo := pgadapter.NewUserRepo(appDB(t), tenantA)
	if err := repo.SoftDelete(ctx, uid, actor, "d1d1d1d1-0000-0000-0000-0000000000c1"); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	users, err := repo.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	for _, u := range users {
		if u.ID() == uid {
			t.Fatal("a deleted user is still listed")
		}
	}
	got, err := repo.FindByID(ctx, uid)
	if err != nil {
		t.Fatalf("FindByID: %v (the row must stay — history references it)", err)
	}
	if !got.IsDeleted() || got.CanAuthenticate() {
		t.Errorf("status = %v, want deleted and unable to authenticate", got.Status())
	}
	if got.PasswordChangedAt().Before(before) {
		t.Errorf("password_changed_at = %v, want moved to now so every refresh token dies", got.PasswordChangedAt())
	}
	var row struct {
		DeletedAt *time.Time
		DeletedBy *string
	}
	if err := superDB.Raw(`SELECT deleted_at, deleted_by::text AS deleted_by FROM users WHERE id = ?`, uid).Scan(&row).Error; err != nil {
		t.Fatalf("read stamp: %v", err)
	}
	if row.DeletedAt == nil || row.DeletedBy == nil || *row.DeletedBy != actor {
		t.Errorf("deletion stamp = %+v, want when and by %s", row, actor)
	}

	if err := repo.SoftDelete(ctx, uid, actor, "d1d1d1d1-0000-0000-0000-0000000000c1"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("deleting twice = %v, want KindNotFound", err)
	}
	if err := repo.SetStatus(ctx, uid, domain.StatusActive); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("SetStatus on a deleted user = %v, want KindNotFound — only Restore brings it back", err)
	}
}

func TestSoftDeleteKeepsOneActiveHolderOfTheGuardedRole(t *testing.T) {
	ctx := context.Background()
	role := "d2d2d2d2-0000-0000-0000-0000000000c1"
	first := "d2d2d2d2-0000-0000-0000-000000000001"
	second := "d2d2d2d2-0000-0000-0000-000000000002"
	dormant := "d2d2d2d2-0000-0000-0000-000000000003"
	seedRole(t, role, tenantA, "GuardedOwner", []string{"users:delete"})
	seedUser(t, first, tenantA, "first@garcialorca.mx", "x", "active")
	seedUser(t, second, tenantA, "second@garcialorca.mx", "x", "active")
	seedUser(t, dormant, tenantA, "dormant@garcialorca.mx", "x", "suspended")
	for _, id := range []string{first, second, dormant} {
		assign(t, id, role, tenantA)
	}
	repo := pgadapter.NewUserRepo(appDB(t), tenantA)

	if err := repo.SoftDelete(ctx, first, second, role); err != nil {
		t.Fatalf("deleting one of two active holders: %v", err)
	}
	if err := repo.SoftDelete(ctx, second, first, role); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("deleting the last active holder = %v, want KindConflict", err)
	}
	if got, _ := repo.FindByID(ctx, second); got.IsDeleted() {
		t.Error("the refused deletion was written anyway")
	}
	// A suspended holder is not what keeps the school administrable.
	if err := repo.SoftDelete(ctx, dormant, second, role); err != nil {
		t.Errorf("deleting a suspended holder: %v", err)
	}
}

func TestRestoreBringsADeletedUserBackAsInvited(t *testing.T) {
	ctx := context.Background()
	uid := "d3d3d3d3-0000-0000-0000-000000000001"
	seedUser(t, uid, tenantA, "back@garcialorca.mx", "x", "active")
	repo := pgadapter.NewUserRepo(appDB(t), tenantA)

	oldRole := "d3d3d3d3-0000-0000-0000-0000000000c1"
	newRole := "d3d3d3d3-0000-0000-0000-0000000000c2"
	seedRole(t, oldRole, tenantA, "RestoreOld", []string{"users:delete"})
	seedRole(t, newRole, tenantA, "RestoreNew", []string{"grades:read"})
	assign(t, uid, oldRole, tenantA)
	if err := repo.Restore(ctx, uid, "Vuelve", []string{newRole}); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Fatalf("Restore on a live account = %v, want KindNotFound", err)
	}
	if err := repo.SoftDelete(ctx, uid, uid, "d3d3d3d3-0000-0000-0000-0000000000ff"); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	// A bad role id rolls the WHOLE restore back: the account stays deleted
	// rather than coming back with the roles it had.
	if err := repo.Restore(ctx, uid, "Vuelve", []string{"d3d3d3d3-0000-0000-0000-00000000dead"}); err == nil {
		t.Fatal("Restore with an unknown role succeeded")
	}
	if got, _ := repo.FindByID(ctx, uid); !got.IsDeleted() {
		t.Fatal("a failed restore left the account restored")
	}
	if err := repo.Restore(ctx, uid, "Vuelve", []string{newRole}); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	got, err := repo.FindByID(ctx, uid)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Status() != domain.StatusInvited || got.DisplayName() != "Vuelve" {
		t.Errorf("restored = (%v, %q), want (invited, Vuelve)", got.Status(), got.DisplayName())
	}
	if ids := got.RoleIDs(); len(ids) != 1 || ids[0] != newRole {
		t.Errorf("roles = %v, want only the new invitation's role", ids)
	}
	var stamped int64
	if err := superDB.Raw(`SELECT count(*) FROM users WHERE id = ? AND (deleted_at IS NOT NULL OR deleted_by IS NOT NULL)`, uid).Scan(&stamped).Error; err != nil {
		t.Fatalf("read stamp: %v", err)
	}
	if stamped != 0 {
		t.Error("the deletion stamp survived the restore")
	}
}

// The schema itself refuses a half-deleted row: status and stamp move together.
func TestDeletedStatusAndStampAreConsistent(t *testing.T) {
	uid := "d4d4d4d4-0000-0000-0000-000000000001"
	seedUser(t, uid, tenantA, "half@garcialorca.mx", "x", "active")

	if err := superDB.Exec(`UPDATE users SET status = 'deleted' WHERE id = ?`, uid).Error; err == nil {
		t.Error("a deleted status without deleted_at was accepted")
	}
	if err := superDB.Exec(`UPDATE users SET deleted_at = now() WHERE id = ?`, uid).Error; err == nil {
		t.Error("a deletion stamp on an active account was accepted")
	}
}

// RLS: tenant B's repository cannot delete, restore or even find tenant A's
// account.
func TestSoftDeleteAndRestoreAreTenantIsolated(t *testing.T) {
	ctx := context.Background()
	uid := "d5d5d5d5-0000-0000-0000-000000000001"
	seedUser(t, uid, tenantA, "isolated@garcialorca.mx", "x", "active")
	other := pgadapter.NewUserRepo(appDB(t), tenantB)

	if err := other.SoftDelete(ctx, uid, uid, "d5d5d5d5-0000-0000-0000-0000000000c1"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("tenant B SoftDelete = %v, want KindNotFound", err)
	}
	if got, _ := pgadapter.NewUserRepo(appDB(t), tenantA).FindByID(ctx, uid); got.IsDeleted() {
		t.Fatal("tenant B deleted tenant A's account")
	}
	if err := pgadapter.NewUserRepo(appDB(t), tenantA).SoftDelete(ctx, uid, uid, "d5d5d5d5-0000-0000-0000-0000000000c1"); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	if err := other.Restore(ctx, uid, "Robada", nil); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("tenant B Restore = %v, want KindNotFound", err)
	}
}

// A malformed id is "no such user", not a database failure.
func TestMalformedUserIDIsNotFound(t *testing.T) {
	repo := pgadapter.NewUserRepo(appDB(t), tenantA)
	if _, err := repo.FindByID(context.Background(), "not-a-uuid"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("FindByID(not-a-uuid) = %v, want KindNotFound", err)
	}
}
