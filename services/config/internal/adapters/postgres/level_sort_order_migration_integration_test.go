//go:build integration

package postgres_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const migration0020 = "0020_level_sort_order.sql"

type levelOrderRow struct {
	Name      string
	SortOrder int
}

func levelsInOrder(t *testing.T, db *gorm.DB) []levelOrderRow {
	t.Helper()
	var rows []levelOrderRow
	require.NoError(t, db.Raw(`SELECT name, sort_order FROM levels WHERE tenant_id = ? ORDER BY sort_order, name`, schoolTenant).
		Scan(&rows).Error)
	return rows
}

// The levels a school already has are put in school order by their folded
// name — maternal, kínder/preescolar, primaria, secundaria, preparatoria/
// bachillerato — and whatever else follows, alphabetically.
func TestMigration0020BackfillsSchoolOrder(t *testing.T) {
	db := migrateUpTo(t, "mig_cfg_level_order", "0020")
	for _, name := range []string{"Talleres", "SECUNDARIA", "Bachillerato", "Primaria", "Kínder", "Inglés", "  Maternal "} {
		exec16(t, db, `INSERT INTO levels (tenant_id, name, enrollment_granularity) VALUES (?, ?, 'CYCLE')`, schoolTenant, name)
	}

	applyMigration(t, db, migration0020)

	assert.Equal(t, []levelOrderRow{
		{"  Maternal ", 1}, {"Kínder", 2}, {"Primaria", 3}, {"SECUNDARIA", 4},
		{"Bachillerato", 5}, {"Inglés", 6}, {"Talleres", 7},
	}, levelsInOrder(t, db))
}

func TestMigration0020PlacesNewLevelsLast(t *testing.T) {
	db := migrateUpTo(t, "mig_cfg_level_order_new", "0020")
	exec16(t, db, `INSERT INTO levels (tenant_id, name, enrollment_granularity) VALUES (?, 'Primaria', 'CYCLE'), (?, 'Preescolar', 'CYCLE')`,
		schoolTenant, schoolTenant)
	applyMigration(t, db, migration0020)

	exec16(t, db, `INSERT INTO levels (tenant_id, name, enrollment_granularity) VALUES (?, 'Maternal', 'CYCLE')`, schoolTenant)
	exec16(t, db, `INSERT INTO levels (tenant_id, name, enrollment_granularity, sort_order) VALUES (?, 'Restaurado', 'CYCLE', 9)`, schoolTenant)

	assert.Equal(t, []levelOrderRow{{"Preescolar", 1}, {"Primaria", 2}, {"Maternal", 3}, {"Restaurado", 9}}, levelsInOrder(t, db),
		"a new level goes after the last; one that carries its place (a restore) keeps it")
}

// A level trashed before the column existed must still restore: its archived
// JSON gets a sort_order (after every live level), or jsonb_populate_record
// would rebuild it with NULL and the whole restore would roll back.
func TestMigration0020GivesArchivedLevelsASortOrder(t *testing.T) {
	db := migrateUpTo(t, "mig_cfg_level_order_archive", "0020")
	exec16(t, db, `INSERT INTO levels (tenant_id, name, enrollment_granularity) VALUES (?, 'Primaria', 'CYCLE')`, schoolTenant)
	exec16(t, db, `INSERT INTO config_deletion_archive (tenant_id, batch_id, table_name, "row", "position")
		VALUES (?, gen_random_uuid(), 'levels', '{"id":"20000000-0000-0000-0000-000000000001","name":"Secundaria"}', 1),
		       (?, gen_random_uuid(), 'grade_levels', '{"id":"20000000-0000-0000-0000-000000000002"}', 2)`,
		schoolTenant, schoolTenant)

	applyMigration(t, db, migration0020)

	assert.Equal(t, int64(2), scalar[int64](t, db,
		`SELECT ("row"->>'sort_order')::bigint FROM config_deletion_archive WHERE table_name = 'levels'`))
	assert.Equal(t, int64(0), scalar[int64](t, db,
		`SELECT count(*) FROM config_deletion_archive WHERE table_name <> 'levels' AND "row" ? 'sort_order'`),
		"only archived levels change")
}
