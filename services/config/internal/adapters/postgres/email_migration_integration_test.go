//go:build integration

package postgres_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gorm.io/gorm"
)

// schoolTenant is the tenant a data migration binds with SET LOCAL: the one
// school this deployment serves (see migrations/iam/0017).
const schoolTenant = "00000000-0000-0000-0000-000000000001"

// migrateUpTo builds a throwaway database with every migration applied BEFORE
// target, so a test can seed the data a data-migration will find in the field
// and then apply target to it. The returned connection is a superuser one.
func migrateUpTo(t *testing.T, name, target string) *gorm.DB {
	t.Helper()
	admin, err := open(superDSNFor(dbName))
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	if err := admin.Exec("CREATE DATABASE " + name).Error; err != nil {
		t.Fatalf("create database %s: %v", name, err)
	}
	db, err := open(superDSNFor(name))
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	for _, file := range orderedMigrations(t) {
		if file >= target {
			break
		}
		applyMigration(t, db, file)
	}
	return db
}

func orderedMigrations(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	return files
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

func hasConstraint(t *testing.T, db *gorm.DB, name string) bool {
	t.Helper()
	var n int64
	if err := db.Raw("SELECT count(*) FROM pg_constraint WHERE conname = ?", name).Scan(&n).Error; err != nil {
		t.Fatalf("look up constraint %s: %v", name, err)
	}
	return n > 0
}

// Migration 0005 stores teacher emails trimmed and lowercase; collisions are
// recorded in teacher_email_conflicts, never deleted.
func TestMigration0005NormalizesTeacherEmails(t *testing.T) {
	db := migrateUpTo(t, "mig_cfg_email", "0005")
	for id, email := range map[string]string{
		"cccccccc-0000-0000-0000-000000000001": " Rosa@Colegio.MX",
		"cccccccc-0000-0000-0000-000000000002": "Pepe@Colegio.mx",
		"cccccccc-0000-0000-0000-000000000003": "pepe@colegio.mx",
	} {
		if err := db.Exec(
			"INSERT INTO teachers (id, tenant_id, first_name, last_name, email) VALUES (?, ?, 'x', 'y', ?)",
			id, schoolTenant, email).Error; err != nil {
			t.Fatalf("seed %s: %v", email, err)
		}
	}

	applyMigration(t, db, "0005_email_lowercase.sql")

	var rosa string
	db.Raw("SELECT email FROM teachers WHERE id = 'cccccccc-0000-0000-0000-000000000001'").Scan(&rosa)
	if rosa != "rosa@colegio.mx" {
		t.Errorf("rosa = %q", rosa)
	}
	var teachers, conflicts int64
	db.Raw("SELECT count(*) FROM teachers").Scan(&teachers)
	db.Raw("SELECT count(*) FROM teacher_email_conflicts").Scan(&conflicts)
	if teachers != 3 || conflicts != 2 {
		t.Fatalf("teachers=%d conflicts=%d, want 3 and 2", teachers, conflicts)
	}
	if hasConstraint(t, db, "teachers_email_normalized_ck") {
		t.Error("the CHECK must wait until the collisions are resolved")
	}
}

func TestMigration0005AddsTheCheckWhenClean(t *testing.T) {
	db := migrateUpTo(t, "mig_cfg_email_clean", "0005")
	if err := db.Exec("INSERT INTO teachers (tenant_id, first_name, last_name, email) VALUES (?, 'x', 'y', 'Ok@Colegio.mx')", schoolTenant).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	applyMigration(t, db, "0005_email_lowercase.sql")
	if !hasConstraint(t, db, "teachers_email_normalized_ck") {
		t.Fatal("with no conflicts the normalization CHECK must be in place")
	}
}
