//go:build integration

package postgres_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"github.com/garcia-lorca/services/uniforms/internal/domain"
	"github.com/garcia-lorca/services/uniforms/internal/ports"
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

// An order the previous revision wrote mid-deploy carries only the old
// student_level key; it is read with that key as the level name, not blank.
func TestAnOrderWrittenByThePreviousRevisionReadsWithItsOldLevel(t *testing.T) {
	orderID := newUUID(t)
	if err := superDB.Exec(`INSERT INTO uniform_orders
	    (id, tenant_id, folio, student_id, student_name, student_level, cycle_id, source)
	    VALUES (?, ?, ?, ?, 'Old Revision', 'secundaria', ?, 'counter')`,
		orderID, tenantA, folioFor(t), newUUID(t), newUUID(t)).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	agg, err := appRepo(t, tenantA).GetOrder(context.Background(), orderID, nil)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if agg.Order.StudentLevelName() != "Secundaria" {
		t.Errorf("student level name = %q, want Secundaria", agg.Order.StudentLevelName())
	}
}

// A garment the previous revision scoped by an old key mid-deploy (level set,
// level_id NULL) must not read as every level: it is unresolved, and a pupil's
// catalogue does not offer it.
func TestAGarmentScopedByThePreviousRevisionIsNotEveryLevel(t *testing.T) {
	itemID := newUUID(t)
	if err := superDB.Exec(`INSERT INTO uniform_items (id, tenant_id, code, name, price_cents, level)
	    VALUES (?, ?, ?, 'Batita vieja', 100, 'kinder')`, itemID, tenantA, "OLD-"+itemID[:8]).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := appRepo(t, tenantA)
	forPupil, err := repo.ListItems(context.Background(), ports.ListItemsFilter{Level: domain.Level(newUUID(t))})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	for _, iws := range forPupil {
		if iws.Item.ID() == itemID {
			t.Fatal("an unresolved garment was offered to a pupil")
		}
	}
	all, err := repo.ListItems(context.Background(), ports.ListItemsFilter{IncludeInactive: true})
	if err != nil {
		t.Fatalf("ListItems all: %v", err)
	}
	for _, iws := range all {
		if iws.Item.ID() == itemID && iws.Item.Level() != domain.LevelUnresolved {
			t.Errorf("level = %q, want unresolved", iws.Item.Level())
		}
	}
}

// Re-scoping an unresolved garment is an explicit decision: saving it for every
// level makes it every level, instead of leaving the old key to keep it hidden.
func TestReScopingAnUnresolvedGarmentToEveryLevelSticks(t *testing.T) {
	itemID := newUUID(t)
	if err := superDB.Exec(`INSERT INTO uniform_items (id, tenant_id, code, name, price_cents, level)
	    VALUES (?, ?, ?, 'Batita vieja', 100, 'kinder')`, itemID, tenantA, "RES-"+itemID[:8]).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := appRepo(t, tenantA)
	all, err := repo.ListItems(context.Background(), ports.ListItemsFilter{IncludeInactive: true})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	var stored domain.Item
	for _, iws := range all {
		if iws.Item.ID() == itemID {
			stored = iws.Item
		}
	}
	everyLevel, err := stored.WithDetails(stored.Name(), "", domain.LevelAny, stored.PriceCents())
	if err != nil {
		t.Fatalf("WithDetails: %v", err)
	}
	if _, err := repo.UpdateItem(context.Background(), everyLevel); err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}
	reread, err := repo.ListItems(context.Background(), ports.ListItemsFilter{IncludeInactive: true})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	for _, iws := range reread {
		if iws.Item.ID() == itemID && iws.Item.Level() != domain.LevelAny {
			t.Errorf("level after re-scope = %q, want every level", iws.Item.Level())
		}
	}
}
