//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"gorm.io/gorm"
)

// seedConfigLevels stands in for config's `levels` table: every service shares
// one database and config's migrations run first, but this harness applies only
// the students migrations.
func seedConfigLevels(t *testing.T, db *gorm.DB, levels map[string]string) {
	t.Helper()
	if err := db.Exec("CREATE TABLE levels (id uuid PRIMARY KEY, tenant_id uuid NOT NULL, name text NOT NULL)").Error; err != nil {
		t.Fatalf("create levels: %v", err)
	}
	for id, name := range levels {
		if err := db.Exec("INSERT INTO levels (id, tenant_id, name) VALUES (?, ?, ?)", id, schoolTenant, name).Error; err != nil {
			t.Fatalf("seed level %s: %v", name, err)
		}
	}
}

func seedLegacyStudent(t *testing.T, db *gorm.DB, id, curp, level string) {
	t.Helper()
	if err := db.Exec(
		"INSERT INTO students (id, tenant_id, curp, first_name, last_name, level) VALUES (?, ?, ?, 'x', 'y', ?)",
		id, schoolTenant, curp, level).Error; err != nil {
		t.Fatalf("seed student %s: %v", level, err)
	}
}

// Migration 0007 frees the level: each pupil now points at a config level (by
// id) and keeps its name. Existing pupils are matched to the config level whose
// name folds to their old SEP key ("Kínder" ↔ kinder); a pupil with no match
// keeps a readable name and no id — it is never guessed.
func TestMigration0007LinksPupilsToConfigLevels(t *testing.T) {
	db := migrateUpTo(t, "mig_st_level", "0007")
	const kinderID, primariaID = "a0000000-0000-0000-0000-00000000000a", "a0000000-0000-0000-0000-00000000000b"
	seedConfigLevels(t, db, map[string]string{kinderID: "Kínder", primariaID: "PRIMARIA"})
	seedLegacyStudent(t, db, "b0000000-0000-0000-0000-000000000001", "HEGA050312MDFRNN08", "kinder")
	seedLegacyStudent(t, db, "b0000000-0000-0000-0000-000000000002", "HEGA050312MDFRNN09", "primaria")
	seedLegacyStudent(t, db, "b0000000-0000-0000-0000-000000000003", "HEGA050312MDFRNN07", "maternal")

	applyMigration(t, db, "0007_free_level.sql")

	for id, want := range map[string][2]string{
		"b0000000-0000-0000-0000-000000000001": {kinderID, "Kínder"},
		"b0000000-0000-0000-0000-000000000002": {primariaID, "PRIMARIA"},
		"b0000000-0000-0000-0000-000000000003": {"", "Maternal"},
	} {
		gotID := scalar[string](t, db, "SELECT coalesce(level_id::text, '') FROM students WHERE id = ?", id)
		gotName := scalar[string](t, db, "SELECT level_name FROM students WHERE id = ?", id)
		if gotID != want[0] || gotName != want[1] {
			t.Errorf("student %s = (%q, %q), want (%q, %q)", id, gotID, gotName, want[0], want[1])
		}
	}
	if hasConstraint(t, db, "students_level_check") {
		t.Error("the fixed SEP list must be gone")
	}
	if err := db.Exec(
		"INSERT INTO students (tenant_id, curp, first_name, last_name, level_id, level_name) VALUES (?, 'HEGA050312MDFRNN06', 'x', 'y', ?, 'Preescolar')",
		schoolTenant, kinderID).Error; err != nil {
		t.Errorf("a pupil on a free level must be storable: %v", err)
	}
}

// Two config levels that fold to the same old key are ambiguous: the pupil is
// left unlinked rather than attached to a guess.
func TestMigration0007LeavesAmbiguousPupilsUnlinked(t *testing.T) {
	db := migrateUpTo(t, "mig_st_level_ambiguous", "0007")
	seedConfigLevels(t, db, map[string]string{
		"a0000000-0000-0000-0000-00000000001a": "Kínder",
		"a0000000-0000-0000-0000-00000000001b": "Kinder",
	})
	seedLegacyStudent(t, db, "b0000000-0000-0000-0000-000000000011", "HEGA050312MDFRNN05", "kinder")

	applyMigration(t, db, "0007_free_level.sql")

	if got := scalar[string](t, db, "SELECT coalesce(level_id::text, '') FROM students"); got != "" {
		t.Errorf("an ambiguous pupil must stay unlinked, got %q", got)
	}
	if got := scalar[string](t, db, "SELECT level_name FROM students"); got != "Kinder" {
		t.Errorf("level_name = %q, want the readable old key", got)
	}
}

// Where config's table does not exist (this service's own test databases), the
// migration still frees the column and keeps every pupil readable.
func TestMigration0007WithoutConfigLevels(t *testing.T) {
	db := migrateUpTo(t, "mig_st_level_noconfig", "0007")
	seedLegacyStudent(t, db, "b0000000-0000-0000-0000-000000000021", "HEGA050312MDFRNN04", "secundaria")

	applyMigration(t, db, "0007_free_level.sql")

	if got := scalar[string](t, db, "SELECT level_name FROM students"); got != "Secundaria" {
		t.Errorf("level_name = %q", got)
	}
}

// During a deploy the previous revision keeps writing for a few minutes: its
// rows carry only the old `level` key, and migration 0007 has already run. They
// are read with that key as the name rather than as a blank level.
func TestAPupilWrittenByThePreviousRevisionReadsWithItsOldLevel(t *testing.T) {
	const id = "b0000000-0000-0000-0000-0000000000a1"
	if err := superDB.Exec(
		"INSERT INTO students (id, tenant_id, curp, first_name, last_name, level) VALUES (?, ?, 'HEGA050312MDFRNN03', 'Old', 'Revision', 'primaria')",
		id, tenantA).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	got, err := appRepo(t, tenantA).FindByID(context.Background(), id)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Level().Name() != "Primaria" || got.Level().ID() != "" {
		t.Errorf("level = (%q, %q), want (\"\", \"Primaria\")", got.Level().ID(), got.Level().Name())
	}
}
