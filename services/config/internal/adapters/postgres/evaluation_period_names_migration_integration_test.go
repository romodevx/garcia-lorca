//go:build integration

package postgres_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Migration 0022 (T10, QA ISSUE-020): an evaluation period still carrying the
// name the system gave it ("PERIODO n" from the screen's prefill, "Periodo n"
// from migration 0016) at its own ordinal, in a grade whose periods are
// exactly the 1st, 2nd and 3rd, takes the trimester name the month calendar
// uses. A name the school typed, a number that no longer matches the ordinal,
// a grade with two periods (semesters) or four (blocks) and a level being
// deleted are left alone.
func TestMigration0022NamesDefaultPeriodsAfterTheTrimester(t *testing.T) {
	db := migrateUpTo(t, "mig_cfg_period_names", "0022")
	const (
		cycle    = "22000000-0000-0000-0000-000000000001"
		level    = "22000000-0000-0000-0000-000000000002"
		deleting = "22000000-0000-0000-0000-000000000003"
		thirds   = "22000000-0000-0000-0000-000000000011" // PERIODO 1..3
		mixed    = "22000000-0000-0000-0000-000000000012" // a migrated, an edited and a shifted name
		blocks   = "22000000-0000-0000-0000-000000000013" // four periods: not trimesters
		frozen   = "22000000-0000-0000-0000-000000000014" // its level is being deleted
		halves   = "22000000-0000-0000-0000-000000000015" // two periods: semesters
	)
	exec16(t, db, `INSERT INTO cycles (id, tenant_id, name, status, starts_on, ends_on)
		VALUES (?, ?, '2026-2027', 'active', '2026-08-24', '2027-07-09')`, cycle, schoolTenant)
	exec16(t, db, `INSERT INTO levels (id, tenant_id, name, enrollment_granularity) VALUES (?, ?, 'PRIMARIA', 'CYCLE'), (?, ?, 'SECUNDARIA', 'CYCLE')`,
		level, schoolTenant, deleting, schoolTenant)
	exec16(t, db, `INSERT INTO grade_levels (id, tenant_id, level_id, name, ordinal) VALUES
		(?, ?, ?, '1°', 1), (?, ?, ?, '2°', 2), (?, ?, ?, '3°', 3), (?, ?, ?, '1° sec', 1), (?, ?, ?, '4°', 4)`,
		thirds, schoolTenant, level, mixed, schoolTenant, level, blocks, schoolTenant, level, frozen, schoolTenant, deleting,
		halves, schoolTenant, level)
	period := func(grade string, ordinal int, name, from, to string) {
		exec16(t, db, `INSERT INTO evaluation_periods (tenant_id, cycle_id, grade_level_id, ordinal, name, starts_on, ends_on)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, schoolTenant, cycle, grade, ordinal, name, from, to)
	}
	for _, g := range []string{thirds, frozen} {
		period(g, 1, "PERIODO 1", "2026-08-24", "2026-11-23")
		period(g, 2, "PERIODO 2", "2026-11-24", "2027-02-23")
		period(g, 3, "PERIODO 3", "2027-02-24", "2027-07-09")
	}
	period(mixed, 1, "Periodo 1", "2026-08-24", "2026-11-23")
	period(mixed, 2, "Evaluación de invierno", "2026-11-24", "2027-02-23")
	period(mixed, 3, "PERIODO 4", "2027-02-24", "2027-07-09")
	period(blocks, 1, "PERIODO 1", "2026-08-24", "2026-10-31")
	period(blocks, 2, "PERIODO 2", "2026-11-01", "2027-01-31")
	period(blocks, 3, "PERIODO 3", "2027-02-01", "2027-04-30")
	period(blocks, 4, "PERIODO 4", "2027-05-01", "2027-07-09")
	period(halves, 1, "PERIODO 1", "2026-08-24", "2027-01-31")
	period(halves, 2, "PERIODO 2", "2027-02-01", "2027-07-09")
	exec16(t, db, `UPDATE levels SET status = 'deleting' WHERE id = ?`, deleting)

	applyMigration(t, db, "0022_evaluation_period_trimester_names.sql")

	names := func(grade string) []string {
		var out []string
		if err := db.Raw(`SELECT name FROM evaluation_periods WHERE grade_level_id = ? ORDER BY ordinal`, grade).Scan(&out).Error; err != nil {
			t.Fatalf("read names: %v", err)
		}
		return out
	}
	assert.Equal(t, []string{"1.er trimestre", "2.º trimestre", "3.er trimestre"}, names(thirds))
	assert.Equal(t, []string{"1.er trimestre", "Evaluación de invierno", "PERIODO 4"}, names(mixed))
	assert.Equal(t, []string{"PERIODO 1", "PERIODO 2", "PERIODO 3", "PERIODO 4"}, names(blocks))
	assert.Equal(t, []string{"PERIODO 1", "PERIODO 2"}, names(halves), "semesters are not trimesters")
	assert.Equal(t, []string{"PERIODO 1", "PERIODO 2", "PERIODO 3"}, names(frozen), "a level on its way to the trash is frozen")
}
