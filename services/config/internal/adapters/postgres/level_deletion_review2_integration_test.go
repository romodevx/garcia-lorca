//go:build integration

// Confirmation-review fixes of the level deletion (ADR 0021): the freeze has
// one owner (levels.deleting_batch_id), an expired batch never leaves its level
// frozen, and a restore in flight is invisible to the sweep.
package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/config/internal/adapters/postgres"
	"github.com/garcia-lorca/services/config/internal/domain"
)

// expire moves a batch's window into the past, on the database's clock.
func expire(t *testing.T, batch string) {
	t.Helper()
	require.NoError(t, superDB(t).Exec("UPDATE level_deletions SET deleted_at = now() - interval '8 days', restorable_until = now() - interval '1 minute' WHERE batch_id = ?", batch).Error)
}

// failPartial leaves a batch failed_partial with the level still in place and
// frozen (config never archived).
func failPartial(t *testing.T, repo *pgadapter.Repository, batch string) {
	t.Helper()
	ctx := context.Background()
	_, err := repo.BeginLevelDeletion(ctx, beginInput(batch))
	require.NoError(t, err)
	_, err = repo.FailLevelDeletion(ctx, batch, []string{"grades"}, nil)
	require.NoError(t, err)
	require.Equal(t, "deleting", levelStatus(t, ldLevel))
}

func levelOwner(t *testing.T, id string) *string {
	t.Helper()
	var owner *string
	require.NoError(t, superDB(t).Raw("SELECT deleting_batch_id::text FROM levels WHERE id = ?", id).Scan(&owner).Error)
	return owner
}

// Gap 1: a failed_partial batch whose window passes is purged — and the level
// it froze (still in place: config never archived) comes back to active. What
// the other services archived is gone; config's structure never left.
func TestPurgingAFailedPartialBatchUnfreezesItsLevel(t *testing.T) {
	seedLevelDeletion(t)
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantF)
	const batch = "f0000000-0000-0000-0000-00000000d001"
	failPartial(t, repo, batch)
	expire(t, batch)

	rec, err := repo.MarkLevelDeletionPurged(ctx, batch)
	require.NoError(t, err)
	assert.Equal(t, domain.LevelDeletionPurged, rec.Status)
	assert.Equal(t, "active", levelStatus(t, ldLevel), "an expired batch never freezes its level forever")
	assert.Nil(t, levelOwner(t, ldLevel))
	g, err := domain.NewGradeLevel("f0000000-0000-0000-0000-0000000000d9", tenantF, ldLevel, "2°", 2)
	require.NoError(t, err)
	require.NoError(t, repo.CreateGradeLevel(ctx, g), "the level is writable again")
}

// Gap 1: a restore that answers TRASH_EXPIRED also lets the level go — on both
// doors (config's restore step and the restore's start).
func TestAnExpiredRestoreUnfreezesItsLevel(t *testing.T) {
	for name, restore := range map[string]func(*pgadapter.Repository, string) error{
		"restore step": func(r *pgadapter.Repository, b string) error {
			_, err := r.RestoreLevelBatch(context.Background(), b)
			return err
		},
		"restore start": func(r *pgadapter.Repository, b string) error {
			_, err := r.BeginLevelRestore(context.Background(), b)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			seedLevelDeletion(t)
			repo := pgadapter.NewRepository(appDB(t), tenantF)
			const batch = "f0000000-0000-0000-0000-00000000d002"
			failPartial(t, repo, batch)
			expire(t, batch)

			err := restore(repo, batch)
			assert.Equal(t, domain.CodeTrashExpired, domain.RefusalCodeOf(err), "err = %v", err)
			assert.Equal(t, "active", levelStatus(t, ldLevel))
			rec, err := repo.GetLevelDeletion(context.Background(), batch)
			require.NoError(t, err)
			assert.Equal(t, domain.LevelDeletionFailedPartial, rec.Status, "the sweep still purges it")
		})
	}
}

// Gap 2: the freeze has one owner. An old batch closing (a restore retried,
// a purge) never lifts the freeze a newer deletion of the same level holds.
func TestClosingAnOldBatchKeepsTheNewerFreeze(t *testing.T) {
	closes := map[string]func(*pgadapter.Repository, string) error{
		"restored again": func(r *pgadapter.Repository, b string) error {
			_, err := r.MarkLevelDeletionRestored(context.Background(), b, false)
			return err
		},
		"purged": func(r *pgadapter.Repository, b string) error {
			expire(t, b)
			_, err := r.MarkLevelDeletionPurged(context.Background(), b)
			return err
		},
	}
	for name, closeOld := range closes {
		t.Run(name, func(t *testing.T) {
			seedLevelDeletion(t)
			ctx := context.Background()
			repo := pgadapter.NewRepository(appDB(t), tenantF)
			const old, newer = "f0000000-0000-0000-0000-00000000d003", "f0000000-0000-0000-0000-00000000d004"
			beginAndArchive(t, repo, old)
			_, err := repo.RestoreLevelBatch(ctx, old)
			require.NoError(t, err)
			_, err = repo.MarkLevelDeletionRestored(ctx, old, true) // restored_partial: still in the trash
			require.NoError(t, err)
			require.Equal(t, "active", levelStatus(t, ldLevel))

			_, err = repo.BeginLevelDeletion(ctx, beginInput(newer))
			require.NoError(t, err)
			owner := levelOwner(t, ldLevel)
			require.NotNil(t, owner)
			assert.Equal(t, newer, *owner, "the freeze names its batch")

			require.NoError(t, closeOld(repo, old))
			assert.Equal(t, "deleting", levelStatus(t, ldLevel), "the newer deletion keeps its freeze")
			assert.Equal(t, newer, *levelOwner(t, ldLevel))

			_, err = repo.CancelLevelDeletion(ctx, newer)
			require.NoError(t, err)
			assert.Equal(t, "active", levelStatus(t, ldLevel), "its owner lifts it")
		})
	}
}

// Gap 4: a restore marks its batch `restoring` (compare-and-set) so the sweep
// never purges it mid-way; a second restore is refused; a restore that dies is
// put back by the watchdog after 30 minutes; the batch closes from restoring.
func TestARestoringBatchIsInvisibleToTheSweep(t *testing.T) {
	seedLevelDeletion(t)
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantF)
	const batch = "f0000000-0000-0000-0000-00000000d005"
	beginAndArchive(t, repo, batch)

	rec, err := repo.BeginLevelRestore(ctx, batch)
	require.NoError(t, err)
	assert.Equal(t, domain.LevelDeletionRestoring, rec.Status)
	assert.Equal(t, domain.LevelDeletionDeleted, rec.RestoringFrom)

	_, err = repo.BeginLevelRestore(ctx, batch)
	assert.Equal(t, domain.CodeRestoreInProgress, domain.RefusalCodeOf(err), "one restore at a time")

	// Its window passes while it restores: the sweep leaves it alone.
	require.NoError(t, superDB(t).Exec("UPDATE level_deletions SET restorable_until = now() - interval '1 minute', deleted_at = now() - interval '8 days' WHERE batch_id = ?", batch).Error)
	expired, _, err := repo.SweepLevelDeletions(ctx, domain.SagaStallAfter)
	require.NoError(t, err)
	assert.Empty(t, expired, "a restoring batch is never swept")

	trash, err := repo.ListLevelDeletions(ctx)
	require.NoError(t, err)
	require.Len(t, trash, 1, "the trash still shows it")

	// The gateway died mid-restore 31 minutes ago: the watchdog puts it back.
	require.NoError(t, superDB(t).Exec("UPDATE level_deletions SET restoring_since = now() - interval '31 minutes' WHERE batch_id = ?", batch).Error)
	expired, _, err = repo.SweepLevelDeletions(ctx, domain.SagaStallAfter)
	require.NoError(t, err)
	require.Len(t, expired, 1, "back to deleted, and past its window: swept")
	assert.Equal(t, domain.LevelDeletionDeleted, expired[0].Status)
}

func TestARestoreClosesOrFallsBackFromRestoring(t *testing.T) {
	seedLevelDeletion(t)
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantF)
	const batch = "f0000000-0000-0000-0000-00000000d006"
	failPartial(t, repo, batch)

	_, err := repo.BeginLevelRestore(ctx, batch)
	require.NoError(t, err)
	back, err := repo.AbandonLevelRestore(ctx, batch)
	require.NoError(t, err)
	assert.Equal(t, domain.LevelDeletionFailedPartial, back.Status, "a failed restore falls back to where it was")
	assert.Empty(t, back.RestoringFrom)
	assert.Equal(t, []string{"grades"}, back.PendingSteps)

	_, err = repo.BeginLevelRestore(ctx, batch)
	require.NoError(t, err)
	done, err := repo.MarkLevelDeletionRestored(ctx, batch, false)
	require.NoError(t, err)
	assert.Equal(t, domain.LevelDeletionRestored, done.Status)
	assert.Equal(t, "active", levelStatus(t, ldLevel))

	_, err = repo.BeginLevelRestore(ctx, batch)
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err), "a closed batch is not in the trash")
	_, err = repo.BeginLevelRestore(ctx, "f0000000-0000-0000-0000-00000000dfff")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}
