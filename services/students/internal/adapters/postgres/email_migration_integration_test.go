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

// migrationsDir is where the students migrations live.
const migrationsDir = "../../../../../migrations/students"

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
	if err := tryMigration(t, db, file); err != nil {
		t.Fatalf("apply migration %s: %v", file, err)
	}
}

// tryMigration applies one file on a dedicated connection, so a migration that
// aborts leaves no half-open transaction behind for the test's next query.
func tryMigration(t *testing.T, db *gorm.DB, file string) error {
	t.Helper()
	sql, err := os.ReadFile(filepath.Join(migrationsDir, file))
	if err != nil {
		t.Fatalf("read migration %s: %v", file, err)
	}
	return db.Connection(func(conn *gorm.DB) error {
		return conn.Exec(string(sql)).Error
	})
}

func hasConstraint(t *testing.T, db *gorm.DB, name string) bool {
	t.Helper()
	return scalar[int64](t, db, "SELECT count(*) FROM pg_constraint WHERE conname = ?", name) > 0
}

// scalar runs a single-value query and fails the test on any error, so a broken
// query never reads as a misleading zero value.
func scalar[T any](t *testing.T, db *gorm.DB, query string, args ...any) T {
	t.Helper()
	var v T
	if err := db.Raw(query, args...).Scan(&v).Error; err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return v
}

// padded wraps an address in the whitespace Go's TrimSpace removes and SQL's
// btrim does not: a tab, a no-break space and an ideographic space.
func padded(email string) string {
	return "\t\u00a0" + email + "\u3000"
}

// Migration 0006 lowercases contact emails. Contacts carry no uniqueness on
// email, so nothing can collide: every row is normalized and the CHECK added.
func TestMigration0006NormalizesContactEmails(t *testing.T) {
	db := migrateUpTo(t, "mig_st_email", "0006")
	if err := db.Exec(
		"INSERT INTO students (id, tenant_id, curp, first_name, last_name, level) VALUES ('dddddddd-0000-0000-0000-000000000001', ?, 'HEGA050312MDFRNN08', 'x', 'y', 'primaria')",
		schoolTenant).Error; err != nil {
		t.Fatalf("seed student: %v", err)
	}
	for _, email := range []string{" Ana@Correo.COM ", padded("Luis@Correo.com")} {
		if err := db.Exec(
			"INSERT INTO guardian_students (tenant_id, student_id, full_name, email) VALUES (?, 'dddddddd-0000-0000-0000-000000000001', 'x', ?)",
			schoolTenant, email).Error; err != nil {
			t.Fatalf("seed contact %q: %v", email, err)
		}
	}

	applyMigration(t, db, "0006_email_lowercase.sql")

	got := scalar[string](t, db, "SELECT string_agg(email, ',' ORDER BY email) FROM guardian_students")
	if got != "ana@correo.com,luis@correo.com" {
		t.Errorf("emails = %q", got)
	}
	if !hasConstraint(t, db, "guardian_students_email_normalized_ck") {
		t.Fatal("the normalization CHECK must be in place")
	}
}
