//go:build integration

// Integration tests for uniforms' half of the level-deletion trash (ADR 0021):
// a garment scoped to the deleted level is kept but left "unresolved" (offered
// to nobody), its level comes back on restore unless staff re-scoped it
// meanwhile, a retried batch changes nothing more, the purge honours the
// retention and RLS keeps schools apart.
package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/uniforms/internal/domain"
)

// Tenant D owns every row of this file.
const tenantD = "44444444-4444-4444-4444-444444444444"

const (
	uaLevel      = "d0000000-0000-0000-0000-0000000000a1" // the doomed level
	uaOther      = "d0000000-0000-0000-0000-0000000000b1" // a bystander level
	uaItem1      = "d0000000-0000-0000-0000-0000000000a2"
	uaItem2      = "d0000000-0000-0000-0000-0000000000a3"
	uaOtherItem  = "d0000000-0000-0000-0000-0000000000b2"
	uaEveryLevel = "d0000000-0000-0000-0000-0000000000c2"
)

func uaScope() levelarchive.Scope { return levelarchive.Scope{LevelID: uaLevel} }

// seedUniformsArchive wipes tenant D's garments and trash and seeds two
// garments of the doomed level, one of the bystander and one for every level.
func seedUniformsArchive(t *testing.T) {
	t.Helper()
	for _, stmt := range []string{
		"DELETE FROM uniforms_deletion_archive WHERE tenant_id = ?",
		"DELETE FROM uniform_items WHERE tenant_id = ?",
	} {
		require.NoError(t, superDB.Exec(stmt, tenantD).Error)
	}
	require.NoError(t, superDB.Exec(`INSERT INTO uniform_items (id, tenant_id, code, name, price_cents, level_id) VALUES
		(?, ?, 'D-1', 'Playera primaria', 30000, ?),
		(?, ?, 'D-2', 'Falda primaria', 40000, ?),
		(?, ?, 'D-3', 'Playera secundaria', 30000, ?),
		(?, ?, 'D-4', 'Chamarra', 50000, NULL)`,
		uaItem1, tenantD, uaLevel, uaItem2, tenantD, uaLevel, uaOtherItem, tenantD, uaOther, uaEveryLevel, tenantD).Error)
}

func countD(t *testing.T, table, where string, args ...any) int64 {
	t.Helper()
	var n int64
	q := "SELECT count(*) FROM " + table + " WHERE tenant_id = '" + tenantD + "'"
	if where != "" {
		q += " AND " + where
	}
	require.NoError(t, superDB.Raw(q, args...).Scan(&n).Error)
	return n
}

func TestUniformsLevelArchivePreview(t *testing.T) {
	seedUniformsArchive(t)
	repo := appRepo(t, tenantD)

	got, err := repo.PreviewLevelDeletion(context.Background(), uaScope())
	require.NoError(t, err)
	assert.Equal(t, levelarchive.Counts{"uniformItems": 2}, got)
	assert.Equal(t, int64(2), countD(t, "uniform_items", "level_id = ?", uaLevel), "a preview changes nothing")
}

func TestUniformsLevelArchiveKeepsGarmentsUnresolvedAndRestores(t *testing.T) {
	seedUniformsArchive(t)
	ctx := context.Background()
	repo := appRepo(t, tenantD)
	const batch = "d0000000-0000-0000-0000-00000000b001"

	counts, err := repo.ArchiveLevel(ctx, batch, uaScope())
	require.NoError(t, err)
	assert.Equal(t, levelarchive.Counts{"uniformItems": 2}, counts)
	assert.Equal(t, int64(4), countD(t, "uniform_items", ""), "no garment is deleted")
	assert.Zero(t, countD(t, "uniform_items", "level_id = ?", uaLevel))

	item, err := repo.GetItem(ctx, uaItem1)
	require.NoError(t, err)
	assert.Equal(t, domain.LevelUnresolved, item.Level(), "the garment reads as unresolved, never as every level")
	every, err := repo.GetItem(ctx, uaEveryLevel)
	require.NoError(t, err)
	assert.NotEqual(t, domain.LevelUnresolved, every.Level(), "an every-level garment is untouched")

	again, err := repo.ArchiveLevel(ctx, batch, uaScope())
	require.NoError(t, err)
	assert.Equal(t, counts, again, "a retried batch answers its first counts")

	rep, err := repo.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	assert.Equal(t, levelarchive.Counts{"uniformItems": 2}, rep.Restored)
	assert.Empty(t, rep.Skipped)
	assert.Equal(t, int64(2), countD(t, "uniform_items", "level_id = ? AND level IS NULL", uaLevel),
		"the level and the empty legacy key both come back")

	rep2, err := repo.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	assert.Equal(t, rep.Restored, rep2.Restored, "a retried restore answers its first report")
}

func TestUniformsLevelArchiveRestoreSkipsARescopedGarment(t *testing.T) {
	seedUniformsArchive(t)
	ctx := context.Background()
	repo := appRepo(t, tenantD)
	const batch = "d0000000-0000-0000-0000-00000000b002"
	_, err := repo.ArchiveLevel(ctx, batch, uaScope())
	require.NoError(t, err)

	// Staff re-scoped one garment to the bystander level meanwhile (saving a
	// real level clears the legacy key).
	require.NoError(t, superDB.Exec("UPDATE uniform_items SET level_id = ?, level = NULL WHERE id = ?", uaOther, uaItem2).Error)

	rep, err := repo.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	assert.Equal(t, int64(1), rep.Restored["uniformItems"])
	require.Len(t, rep.Skipped, 1)
	assert.Equal(t, "uniformItems", rep.Skipped[0].Kind)
	assert.Equal(t, int64(1), rep.Skipped[0].Count)
	assert.NotEmpty(t, rep.Skipped[0].Reason)
	assert.Equal(t, int64(1), countD(t, "uniform_items", "id = ? AND level_id = ?", uaItem2, uaOther), "staff's choice wins")
}

func TestUniformsLevelArchivePurgeHonoursTheRetention(t *testing.T) {
	seedUniformsArchive(t)
	ctx := context.Background()
	repo := appRepo(t, tenantD)
	const batch = "d0000000-0000-0000-0000-00000000b003"
	_, err := repo.ArchiveLevel(ctx, batch, uaScope())
	require.NoError(t, err)

	n, err := repo.PurgeLevelArchives(ctx, levelarchive.Cutoff(time.Now()))
	require.NoError(t, err)
	assert.Zero(t, n, "fresh trash is never purged")

	require.NoError(t, superDB.Exec("UPDATE uniforms_deletion_archive SET created_at = now() - interval '8 days' WHERE batch_id = ?", batch).Error)
	n, err = repo.PurgeLevelArchives(ctx, levelarchive.Cutoff(time.Now()))
	require.NoError(t, err)
	assert.Positive(t, n)
	assert.Zero(t, countD(t, "uniforms_deletion_archive", "batch_id = ?", batch))
}

func TestUniformsLevelArchiveIsTenantIsolated(t *testing.T) {
	seedUniformsArchive(t)
	ctx := context.Background()
	const batch = "d0000000-0000-0000-0000-00000000b004"
	_, err := appRepo(t, tenantD).ArchiveLevel(ctx, batch, uaScope())
	require.NoError(t, err)

	outsider := appRepo(t, tenantB)
	rep, err := outsider.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	assert.Empty(t, rep.Restored, "B cannot restore D's batch")
	assert.Zero(t, countD(t, "uniform_items", "level_id = ?", uaLevel))

	counts, err := outsider.ArchiveLevel(ctx, "d0000000-0000-0000-0000-00000000b005", levelarchive.Scope{LevelID: uaOther})
	require.NoError(t, err)
	assert.Equal(t, levelarchive.Counts{"uniformItems": 0}, counts, "B reaches none of D's garments")
	assert.Equal(t, int64(1), countD(t, "uniform_items", "level_id = ?", uaOther))

	var visible int64
	db, err := open(appDSN)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantB).Error; err != nil {
			return err
		}
		return tx.Raw("SELECT count(*) FROM uniforms_deletion_archive WHERE batch_id = ?", batch).Scan(&visible).Error
	}))
	assert.Zero(t, visible, "RLS hides D's archive rows from B")
}
