//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/iam/internal/adapters/postgres"
	"github.com/garcia-lorca/services/iam/internal/domain"
)

// ISSUE-CC-117 — suspending follows the last-holder rule deleting follows, under
// the same row locks, and ends the account's refresh tokens (ADR 0005).

func TestSuspendKeepsOneActiveHolderOfTheGuardedRole(t *testing.T) {
	ctx := context.Background()
	role := "e1e1e1e1-0000-0000-0000-0000000000c1"
	first := "e1e1e1e1-0000-0000-0000-000000000001"
	second := "e1e1e1e1-0000-0000-0000-000000000002"
	seedRole(t, role, tenantA, "SuspendGuardedOwner", []string{"users:delete"})
	seedUser(t, first, tenantA, "susp-first@garcialorca.mx", "x", "active")
	seedUser(t, second, tenantA, "susp-second@garcialorca.mx", "x", "active")
	assign(t, first, role, tenantA)
	assign(t, second, role, tenantA)
	repo := pgadapter.NewUserRepo(appDB(t), tenantA)

	if err := repo.SetStatus(ctx, first, domain.StatusSuspended, role); err != nil {
		t.Fatalf("suspending one of two active holders: %v", err)
	}
	if err := repo.SetStatus(ctx, second, domain.StatusSuspended, role); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("suspending the last active holder = %v, want KindConflict", err)
	}
	if got, _ := repo.FindByID(ctx, second); got.Status() != domain.StatusActive {
		t.Errorf("the refused suspension was written anyway: %v", got.Status())
	}
	// Reactivating is never refused: it adds an active holder.
	if err := repo.SetStatus(ctx, first, domain.StatusActive, role); err != nil {
		t.Fatalf("reactivating: %v", err)
	}
	// And re-asserting active on the only holder is no suspension either.
	if err := repo.SetStatus(ctx, second, domain.StatusActive, role); err != nil {
		t.Errorf("SetStatus(active) on an active holder: %v", err)
	}
}

func TestSuspendMovesTheRefreshFloorAndReactivateDoesNot(t *testing.T) {
	ctx := context.Background()
	uid := "e2e2e2e2-0000-0000-0000-000000000001"
	seedUser(t, uid, tenantA, "susp-floor@garcialorca.mx", "x", "active")
	repo := pgadapter.NewUserRepo(appDB(t), tenantA)
	before := time.Now().Add(-time.Second)

	if err := repo.SetStatus(ctx, uid, domain.StatusSuspended, "e2e2e2e2-0000-0000-0000-0000000000ff"); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	suspended, err := repo.FindByID(ctx, uid)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if suspended.PasswordChangedAt().Before(before) {
		t.Errorf("password_changed_at = %v, want moved so the suspended account's refresh tokens die", suspended.PasswordChangedAt())
	}

	if err := repo.SetStatus(ctx, uid, domain.StatusActive, "e2e2e2e2-0000-0000-0000-0000000000ff"); err != nil {
		t.Fatalf("reactivate: %v", err)
	}
	active, _ := repo.FindByID(ctx, uid)
	if !active.PasswordChangedAt().Equal(suspended.PasswordChangedAt()) {
		t.Errorf("reactivating moved the floor %v -> %v; only suspending ends sessions",
			suspended.PasswordChangedAt(), active.PasswordChangedAt())
	}
}

func TestStatusOfReadsTheLifecycleStateUnderRLS(t *testing.T) {
	ctx := context.Background()
	uid := "e3e3e3e3-0000-0000-0000-000000000001"
	seedUser(t, uid, tenantA, "susp-status@garcialorca.mx", "x", "suspended")

	got, err := pgadapter.NewUserRepo(appDB(t), tenantA).StatusOf(ctx, uid)
	if err != nil {
		t.Fatalf("StatusOf: %v", err)
	}
	if got != domain.StatusSuspended {
		t.Errorf("status = %v, want suspended", got)
	}

	// Another tenant cannot see the row at all.
	if _, err := pgadapter.NewUserRepo(appDB(t), tenantB).StatusOf(ctx, uid); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("StatusOf from tenant B = %v, want KindNotFound (RLS)", err)
	}
	if _, err := pgadapter.NewUserRepo(appDB(t), tenantA).StatusOf(ctx, "e3e3e3e3-0000-0000-0000-0000000000ff"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("StatusOf of an unknown id = %v, want KindNotFound", err)
	}
}

// Two directors suspending each other at the same instant: exactly one wins and
// the other is refused, never both (which would leave the school with no active
// director). The race is FORCED, not hoped for: a blocker transaction holds
// both rows, the two suspensions are released only once Postgres shows both
// waiting on a lock, so each must re-read the other's row after it commits.
// Without FOR UPDATE in guardLastActiveHolder both count two holders before
// blocking and both succeed — this test fails.
func TestConcurrentMutualSuspensionLeavesOneActiveDirector(t *testing.T) {
	ctx := context.Background()
	role := "e4e4e4e4-0000-0000-0000-0000000000c1"
	first := "e4e4e4e4-0000-0000-0000-000000000001"
	second := "e4e4e4e4-0000-0000-0000-000000000002"
	seedRole(t, role, tenantA, "RaceOwner", []string{"users:delete"})
	seedUser(t, first, tenantA, "race-first@garcialorca.mx", "x", "active")
	seedUser(t, second, tenantA, "race-second@garcialorca.mx", "x", "active")
	assign(t, first, role, tenantA)
	assign(t, second, role, tenantA)
	db := appDB(t)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	repo := pgadapter.NewUserRepo(db, tenantA)

	blocker := superDB.Begin()
	if err := blocker.Exec("SELECT 1 FROM users WHERE id IN (?, ?) FOR UPDATE", first, second).Error; err != nil {
		t.Fatalf("blocker: %v", err)
	}
	results := make(chan error, 2)
	go func() { results <- repo.SetStatus(ctx, second, domain.StatusSuspended, role) }() // first suspends second
	go func() { results <- repo.SetStatus(ctx, first, domain.StatusSuspended, role) }()  // second suspends first
	waitForLockWaiters(t, superDB, 2)
	if err := blocker.Commit().Error; err != nil {
		t.Fatalf("release blocker: %v", err)
	}

	var ok, refused int
	for range 2 {
		switch err := <-results; {
		case err == nil:
			ok++
		case glerrors.KindOf(err) == glerrors.KindConflict:
			refused++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if ok != 1 || refused != 1 {
		t.Fatalf("successes=%d refusals=%d, want exactly one of each", ok, refused)
	}
	var active int64
	if err := superDB.Raw("SELECT count(*) FROM users WHERE id IN (?, ?) AND status = 'active'", first, second).Scan(&active).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if active != 1 {
		t.Errorf("active directors = %d, want 1", active)
	}
}

// waitForLockWaiters polls until n backends of this database wait on a lock.
func waitForLockWaiters(t *testing.T, db *gorm.DB, n int64) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int64
		if err := db.Raw("SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'").Scan(&waiting).Error; err != nil {
			t.Fatalf("pg_stat_activity: %v", err)
		}
		if waiting >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d lock waiters", n)
}
