//go:build integration

package postgres_test

import (
	"os"
	"path/filepath"
	"testing"

	"gorm.io/gorm"
)

// schoolTenant is the tenant a data migration binds with SET LOCAL: the one
// school this deployment serves (see migrations/iam/0017).
const schoolTenant = "00000000-0000-0000-0000-000000000001"

// migrateUpTo builds a throwaway database with every migration applied BEFORE
// target, so a test can seed what a data migration finds in the field and then
// apply target to it. The returned connection is the owner's.
func migrateUpTo(t *testing.T, name, target string) *gorm.DB {
	t.Helper()
	if err := superDB.Exec("CREATE DATABASE " + name).Error; err != nil {
		t.Fatalf("create database %s: %v", name, err)
	}
	db, err := open(superDSNFor(name))
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			if err := sqlDB.Close(); err != nil {
				t.Logf("close pool: %v", err)
			}
		}
		if err := superDB.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)").Error; err != nil {
			t.Errorf("drop database %s: %v", name, err)
		}
	})
	files, err := sqlFiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file >= target {
			break
		}
		applyMigration(t, db, file)
	}
	return db
}

func applyMigration(t *testing.T, db *gorm.DB, file string) {
	t.Helper()
	sql, err := os.ReadFile(filepath.Join(migrationsDir, file))
	if err != nil {
		t.Fatalf("read migration %s: %v", file, err)
	}
	if err := db.Exec(string(sql)).Error; err != nil {
		t.Fatalf("apply migration %s: %v", file, err)
	}
}

func scalar[T any](t *testing.T, db *gorm.DB, query string, args ...any) T {
	t.Helper()
	var v T
	if err := db.Raw(query, args...).Scan(&v).Error; err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return v
}

func hasConstraint(t *testing.T, db *gorm.DB, name string) bool {
	t.Helper()
	return scalar[int64](t, db, "SELECT count(*) FROM pg_constraint WHERE conname = ?", name) > 0
}

// Migration 0007 frees the level in the shop too: a garment is scoped by a
// config level id, and an order keeps the level id and name the pupil had.
// Existing rows are linked only when exactly one config level folds to their
// old key; anything else keeps a readable name and no id.
func TestMigration0007LinksGarmentsAndOrdersToConfigLevels(t *testing.T) {
	db := migrateUpTo(t, "mig_un_level", "0007")
	const kinderID = "a0000000-0000-0000-0000-00000000000a"
	if err := db.Exec("CREATE TABLE levels (id uuid PRIMARY KEY, tenant_id uuid NOT NULL, name text NOT NULL)").Error; err != nil {
		t.Fatalf("create levels: %v", err)
	}
	if err := db.Exec("INSERT INTO levels VALUES (?, ?, 'Kínder')", kinderID, schoolTenant).Error; err != nil {
		t.Fatalf("seed level: %v", err)
	}
	for _, stmt := range []string{
		`INSERT INTO uniform_items (id, tenant_id, code, name, price_cents, level) VALUES ('b0000000-0000-0000-0000-000000000001', '` + schoolTenant + `', 'BAT', 'Batita', 100, 'kinder')`,
		`INSERT INTO uniform_items (id, tenant_id, code, name, price_cents, level) VALUES ('b0000000-0000-0000-0000-000000000002', '` + schoolTenant + `', 'PLA', 'Playera', 100, NULL)`,
		`INSERT INTO uniform_orders (id, tenant_id, folio, student_id, student_name, student_level, cycle_id, source) VALUES ('c0000000-0000-0000-0000-000000000001', '` + schoolTenant + `', 'U-2026-000001', 'd0000000-0000-0000-0000-000000000001', 'Ana', 'kinder', 'e0000000-0000-0000-0000-000000000001', 'counter')`,
		`INSERT INTO uniform_orders (id, tenant_id, folio, student_id, student_name, student_level, cycle_id, source) VALUES ('c0000000-0000-0000-0000-000000000002', '` + schoolTenant + `', 'U-2026-000002', 'd0000000-0000-0000-0000-000000000002', 'Luis', 'maternal', 'e0000000-0000-0000-0000-000000000001', 'counter')`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	applyMigration(t, db, "0007_free_level.sql")

	if got := scalar[string](t, db, "SELECT coalesce(level_id::text, '') FROM uniform_items WHERE code = 'BAT'"); got != kinderID {
		t.Errorf("kinder smock level_id = %q", got)
	}
	if got := scalar[string](t, db, "SELECT coalesce(level_id::text, '') FROM uniform_items WHERE code = 'PLA'"); got != "" {
		t.Errorf("an every-level garment stays every-level, got %q", got)
	}
	for folio, want := range map[string][2]string{
		"U-2026-000001": {kinderID, "Kínder"},
		"U-2026-000002": {"", "Maternal"},
	} {
		id := scalar[string](t, db, "SELECT coalesce(student_level_id::text, '') FROM uniform_orders WHERE folio = ?", folio)
		name := scalar[string](t, db, "SELECT student_level_name FROM uniform_orders WHERE folio = ?", folio)
		if id != want[0] || name != want[1] {
			t.Errorf("order %s level = (%q, %q), want (%q, %q)", folio, id, name, want[0], want[1])
		}
	}
	if hasConstraint(t, db, "uniform_items_level_check") || hasConstraint(t, db, "uniform_orders_student_level_check") {
		t.Error("the fixed SEP lists must be gone")
	}
}
