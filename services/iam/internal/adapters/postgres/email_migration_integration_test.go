//go:build integration

package postgres_test

import (
	"os"
	"path/filepath"
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
	t.Cleanup(func() { dropDatabase(t, admin, db, name) })
	for _, file := range orderedMigrations(t) {
		if file >= target {
			break
		}
		applyMigration(t, db, file)
	}
	return db
}

// dropDatabase closes the throwaway database's pool and drops it, so a run
// leaves nothing behind in the container and can be repeated (-count=N).
func dropDatabase(t *testing.T, admin, db *gorm.DB, name string) {
	t.Helper()
	closePool(t, db)
	if err := admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)").Error; err != nil {
		t.Errorf("drop database %s: %v", name, err)
	}
	closePool(t, admin)
}

func closePool(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	if err == nil {
		err = sqlDB.Close()
	}
	if err != nil {
		t.Logf("close pool: %v", err)
	}
}

func orderedMigrations(t *testing.T) []string {
	t.Helper()
	files, err := sqlFiles()
	if err != nil {
		t.Fatal(err)
	}
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

func seedUser0018(t *testing.T, db *gorm.DB, id, email string) {
	t.Helper()
	if err := db.Exec(
		"INSERT INTO users (id, tenant_id, email, display_name, password_hash, status) VALUES (?, ?, ?, 'x', 'x', 'active')",
		id, schoolTenant, email).Error; err != nil {
		t.Fatalf("seed %q: %v", email, err)
	}
}

// Migration 0018 stores every login email trimmed and lowercase — trimmed of
// every whitespace Go trims, not only U+0020 — and then forbids anything else.
func TestMigration0018NormalizesUserEmails(t *testing.T) {
	db := migrateUpTo(t, "mig_iam_email_clean", "0018")
	seedUser0018(t, db, "aaaaaaaa-0000-0000-0000-000000000001", "  Ana.Cruz@Colegio.MX ")
	seedUser0018(t, db, "aaaaaaaa-0000-0000-0000-000000000002", padded("Beto@Colegio.mx"))
	seedUser0018(t, db, "aaaaaaaa-0000-0000-0000-000000000003", "carla@colegio.mx")
	seedUser0018(t, db, "aaaaaaaa-0000-0000-0000-000000000004", "JOSÉ@Colegio.mx")
	// U+001F is not whitespace to Go's TrimSpace, so SQL must not strip it
	// either: the two spellings of "normalized" must agree byte for byte.
	seedUser0018(t, db, "aaaaaaaa-0000-0000-0000-000000000005", "Dora@Colegio.mx\x1f")

	applyMigration(t, db, "0018_email_lowercase.sql")

	for id, want := range map[string]string{
		"aaaaaaaa-0000-0000-0000-000000000001": "ana.cruz@colegio.mx",
		"aaaaaaaa-0000-0000-0000-000000000002": "beto@colegio.mx",
		"aaaaaaaa-0000-0000-0000-000000000003": "carla@colegio.mx",
		// Go's strings.ToLower folds É; lower() under a C ctype would not.
		"aaaaaaaa-0000-0000-0000-000000000004": "josé@colegio.mx",
		"aaaaaaaa-0000-0000-0000-000000000005": "dora@colegio.mx\x1f",
	} {
		if got := scalar[string](t, db, "SELECT email FROM users WHERE id = ?", id); got != want {
			t.Errorf("user %s email = %q, want %q", id, got, want)
		}
	}
	if !hasConstraint(t, db, "users_email_normalized_ck") {
		t.Fatal("the normalization CHECK must be in place")
	}
	for _, email := range []string{"Mixed@Case.mx", padded("pad@colegio.mx")} {
		err := db.Exec("INSERT INTO users (tenant_id, email, display_name, password_hash, status) VALUES (?, ?, 'x', 'x', 'active')", schoolTenant, email).Error
		if err == nil || !strings.Contains(err.Error(), "users_email_normalized_ck") {
			t.Errorf("insert %q: want a users_email_normalized_ck violation, got %v", email, err)
		}
	}
}

// Two spellings of one address are two rows that would become one. The
// migration refuses to guess which is the real person: it stops, names the
// rows by id, and changes NOTHING — no row deleted, none rewritten.
func TestMigration0018StopsOnCollisionsAndChangesNothing(t *testing.T) {
	db := migrateUpTo(t, "mig_iam_email_dup", "0018")
	seedUser0018(t, db, "bbbbbbbb-0000-0000-0000-000000000001", "Ana@Colegio.mx")
	seedUser0018(t, db, "bbbbbbbb-0000-0000-0000-000000000002", "ana@colegio.mx")
	seedUser0018(t, db, "bbbbbbbb-0000-0000-0000-000000000003", "Luis@Colegio.mx")

	err := tryMigration(t, db, "0018_email_lowercase.sql")
	if err == nil {
		t.Fatal("colliding emails must stop the migration")
	}
	for _, id := range []string{"bbbbbbbb-0000-0000-0000-000000000001", "bbbbbbbb-0000-0000-0000-000000000002"} {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("the error must name colliding row %s: %v", id, err)
		}
	}
	if strings.Contains(err.Error(), "colegio.mx") {
		t.Errorf("the error must not carry the addresses themselves: %v", err)
	}
	if n := scalar[int64](t, db, "SELECT count(*) FROM users"); n != 3 {
		t.Errorf("no user may be deleted, have %d", n)
	}
	if got := scalar[string](t, db, "SELECT email FROM users WHERE id = 'bbbbbbbb-0000-0000-0000-000000000003'"); got != "Luis@Colegio.mx" {
		t.Errorf("a stopped migration rewrites nothing, got %q", got)
	}
	if hasConstraint(t, db, "users_email_normalized_ck") {
		t.Error("a stopped migration adds no CHECK")
	}
}
