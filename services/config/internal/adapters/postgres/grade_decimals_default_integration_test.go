//go:build integration

package postgres_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Migration 0021 (T10, ISSUE-011): grades are whole numbers, so a level row
// that does not name its decimals starts «Sin decimales». A level that already
// exists keeps the decimals the school gave it.
func TestMigration0021LevelsDefaultToWholeNumbers(t *testing.T) {
	db := migrateUpTo(t, "mig_cfg_grade_decimals", "0021")
	const existing = "aaaa0021-0000-0000-0000-000000000001"
	const created = "aaaa0021-0000-0000-0000-000000000002"
	insert := "INSERT INTO levels (id, tenant_id, name, enrollment_granularity) VALUES (?, ?, ?, 'CYCLE')"
	if err := db.Exec(insert, existing, schoolTenant, "Primaria").Error; err != nil {
		t.Fatalf("seed level: %v", err)
	}

	applyMigration(t, db, "0021_level_grade_decimals_default.sql")

	if err := db.Exec(insert, created, schoolTenant, "Secundaria").Error; err != nil {
		t.Fatalf("insert level: %v", err)
	}
	assert.Equal(t, 1, scalar[int](t, db, "SELECT grade_decimals FROM levels WHERE id = ?", existing), "existing levels untouched")
	assert.Equal(t, 0, scalar[int](t, db, "SELECT grade_decimals FROM levels WHERE id = ?", created), "new levels: whole numbers")
}
