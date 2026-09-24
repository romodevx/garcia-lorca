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
	// C ctype on purpose: there lower() folds ASCII only, so a migration that
	// relied on the database's locale would pass on en_US and fail in the field.
	if err := admin.Exec("CREATE DATABASE " + name + " TEMPLATE template0 LC_COLLATE 'C' LC_CTYPE 'C'").Error; err != nil {
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

func seedTeacher0005(t *testing.T, db *gorm.DB, id, email string) {
	t.Helper()
	if err := db.Exec(
		"INSERT INTO teachers (id, tenant_id, first_name, last_name, email) VALUES (?, ?, 'x', 'y', ?)",
		id, schoolTenant, email).Error; err != nil {
		t.Fatalf("seed %q: %v", email, err)
	}
}

// Migration 0005 stores teacher emails trimmed and lowercase, then forbids
// anything else.
func TestMigration0005NormalizesTeacherEmails(t *testing.T) {
	db := migrateUpTo(t, "mig_cfg_email_clean", "0005")
	seedTeacher0005(t, db, "cccccccc-0000-0000-0000-000000000001", " Rosa@Colegio.MX")
	seedTeacher0005(t, db, "cccccccc-0000-0000-0000-000000000002", padded("Pepe@Colegio.mx"))

	applyMigration(t, db, "0005_email_lowercase.sql")

	for id, want := range map[string]string{
		"cccccccc-0000-0000-0000-000000000001": "rosa@colegio.mx",
		"cccccccc-0000-0000-0000-000000000002": "pepe@colegio.mx",
	} {
		if got := scalar[string](t, db, "SELECT email FROM teachers WHERE id = ?", id); got != want {
			t.Errorf("teacher %s email = %q, want %q", id, got, want)
		}
	}
	if !hasConstraint(t, db, "teachers_email_normalized_ck") {
		t.Fatal("the normalization CHECK must be in place")
	}
}

// Two spellings of one teacher's address stop the migration, named by id, with
// nothing deleted or rewritten.
func TestMigration0005StopsOnCollisionsAndChangesNothing(t *testing.T) {
	db := migrateUpTo(t, "mig_cfg_email_dup", "0005")
	seedTeacher0005(t, db, "dddddddd-0000-0000-0000-000000000001", "Pepe@Colegio.mx")
	seedTeacher0005(t, db, "dddddddd-0000-0000-0000-000000000002", "pepe@colegio.mx")
	seedTeacher0005(t, db, "dddddddd-0000-0000-0000-000000000003", "Rosa@Colegio.mx")

	err := tryMigration(t, db, "0005_email_lowercase.sql")
	if err == nil {
		t.Fatal("colliding emails must stop the migration")
	}
	for _, id := range []string{"dddddddd-0000-0000-0000-000000000001", "dddddddd-0000-0000-0000-000000000002"} {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("the error must name colliding row %s: %v", id, err)
		}
	}
	if strings.Contains(err.Error(), "dddddddd-0000-0000-0000-000000000003") {
		t.Errorf("a row that does not collide is not named: %v", err)
	}
	if n := scalar[int64](t, db, "SELECT count(*) FROM teachers"); n != 3 {
		t.Errorf("no teacher may be deleted, have %d", n)
	}
	if got := scalar[string](t, db, "SELECT email FROM teachers WHERE id = 'dddddddd-0000-0000-0000-000000000001'"); got != "Pepe@Colegio.mx" {
		t.Errorf("a stopped migration rewrites nothing, got %q", got)
	}
	if hasConstraint(t, db, "teachers_email_normalized_ck") {
		t.Error("a stopped migration adds no CHECK")
	}
}
