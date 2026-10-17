//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/config/internal/adapters/postgres"
	"github.com/garcia-lorca/services/config/internal/domain"
)

// Migration 0009 gives every existing level the school's default scale and
// switches numeric capture off for the early-years levels, matched regardless
// of case, accents and surrounding spaces.
func TestMigration0009BackfillsGradingByLevelName(t *testing.T) {
	db := migrateUpTo(t, "mig_cfg_grading", "0009")
	levels := map[string]struct {
		name        string
		wantEnabled bool
	}{
		"aaaa0009-0000-0000-0000-000000000001": {"Preescolar", false},
		"aaaa0009-0000-0000-0000-000000000002": {" KÍNDER ", false},
		"aaaa0009-0000-0000-0000-000000000003": {"kinder", false},
		"aaaa0009-0000-0000-0000-000000000004": {"Maternal", false},
		"aaaa0009-0000-0000-0000-000000000005": {"Primaria", true},
		"aaaa0009-0000-0000-0000-000000000006": {"Secundaria", true},
		"aaaa0009-0000-0000-0000-000000000007": {"Preescolar Bilingüe", true},
	}
	for id, l := range levels {
		require.NoError(t, db.Exec(
			"INSERT INTO levels (id, tenant_id, name, enrollment_granularity) VALUES (?, ?, ?, 'CYCLE')",
			id, schoolTenant, l.name).Error)
	}

	applyMigration(t, db, "0009_level_grading_scale.sql")

	for id, l := range levels {
		assert.Equal(t, l.wantEnabled, scalar[bool](t, db, "SELECT grading_enabled FROM levels WHERE id = ?", id), l.name)
		assert.Equal(t, "5.00|10.00|1|6.00", scalar[string](t, db,
			"SELECT grade_min::text || '|' || grade_max::text || '|' || grade_decimals::text || '|' || grade_passing::text FROM levels WHERE id = ?", id), l.name)
	}
	const id = "aaaa0009-0000-0000-0000-000000000005"
	for _, bad := range []string{
		"UPDATE levels SET grade_passing = 5 WHERE id = '" + id + "'",  // passing must exceed min
		"UPDATE levels SET grade_passing = 11 WHERE id = '" + id + "'", // passing must not exceed max
		"UPDATE levels SET grade_min = -1 WHERE id = '" + id + "'",     // min >= 0
		"UPDATE levels SET grade_max = 100.5 WHERE id = '" + id + "'",  // max <= 100
		"UPDATE levels SET grade_decimals = 3 WHERE id = '" + id + "'", // decimals 0..2
	} {
		assert.Error(t, tryMigrationSQL(t, db, bad), bad)
	}
	assert.NoError(t, tryMigrationSQL(t, db, "UPDATE levels SET grade_max = 100, grade_passing = 70, grade_min = 0 WHERE id = '"+id+"'"),
		"a 0–100 scale must fit")
}

// A created level persists its default scale; a replaced scale reads back — and
// keeps doing so on a second update over the same row (the path a school walks
// after the first save); another tenant can neither see nor change it.
func TestLevelGradingRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantA)

	kinder, err := domain.NewLevel("eeee0009-0000-0000-0000-000000000001", tenantA, "Kínder", "", domain.GranularityCycle)
	require.NoError(t, err)
	prim, err := domain.NewLevel("eeee0009-0000-0000-0000-000000000002", tenantA, "Primaria T6", "", domain.GranularityCycle)
	require.NoError(t, err)
	for _, l := range []domain.Level{kinder, prim} {
		require.NoError(t, repo.CreateLevel(ctx, l))
	}
	got, err := repo.GetLevel(ctx, prim.ID())
	require.NoError(t, err)
	assert.Equal(t, prim.Grading(), got.Grading(), "created scale must persist")

	first, err := domain.NewGradingScale(true, 0, 10000, 7000, 2)
	require.NoError(t, err)
	second, err := domain.NewGradingScale(false, 500, 900, 700, 0)
	require.NoError(t, err)
	for _, scale := range []domain.GradingScale{first, second} {
		require.NoError(t, repo.UpdateLevelGrading(ctx, prim.ID(), scale))
		got, err := repo.GetLevel(ctx, prim.ID())
		require.NoError(t, err)
		assert.Equal(t, scale, got.Grading())
	}
	gotKinder, err := repo.GetLevel(ctx, kinder.ID())
	require.NoError(t, err)
	assert.False(t, gotKinder.Grading().Enabled(), "updating one level must not touch another")

	err = repo.UpdateLevelGrading(ctx, "eeee0009-0000-0000-0000-00000000ffff", first)
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
	other := pgadapter.NewRepository(appDB(t), tenantB)
	err = other.UpdateLevelGrading(ctx, prim.ID(), first)
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err), "RLS: a foreign tenant cannot change the scale")
	got, err = repo.GetLevel(ctx, prim.ID())
	require.NoError(t, err)
	assert.Equal(t, second, got.Grading())
}

// tryMigrationSQL runs one statement on a dedicated connection and returns its
// error, so a statement a CHECK refuses leaves no aborted transaction behind.
func tryMigrationSQL(t *testing.T, db *gorm.DB, stmt string) error {
	t.Helper()
	return db.Connection(func(conn *gorm.DB) error { return conn.Exec(stmt).Error })
}
