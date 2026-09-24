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

// Migration 0018 stores every login email trimmed and lowercase. Emails that
// would collide once lowercased are NOT touched and NOT deleted: they are
// recorded in user_email_conflicts for a person to resolve, and the CHECK that
// forbids unnormalized emails is only added when there is nothing to resolve.
func TestMigration0018NormalizesUserEmails(t *testing.T) {
	db := migrateUpTo(t, "mig_iam_email_clean", "0018")
	seed := func(id, email string) {
		t.Helper()
		if err := db.Exec(
			"INSERT INTO users (id, tenant_id, email, display_name, password_hash, status) VALUES (?, ?, ?, 'x', 'x', 'active')",
			id, schoolTenant, email).Error; err != nil {
			t.Fatalf("seed %s: %v", email, err)
		}
	}
	seed("aaaaaaaa-0000-0000-0000-000000000001", "  Ana.Cruz@Colegio.MX ")
	seed("aaaaaaaa-0000-0000-0000-000000000002", "beto@colegio.mx")

	applyMigration(t, db, "0018_email_lowercase.sql")

	var emails []string
	db.Raw("SELECT email FROM users ORDER BY id").Scan(&emails)
	if len(emails) != 2 || emails[0] != "ana.cruz@colegio.mx" || emails[1] != "beto@colegio.mx" {
		t.Fatalf("emails after migration = %q", emails)
	}
	if !hasConstraint(t, db, "users_email_normalized_ck") {
		t.Fatal("with no conflicts the normalization CHECK must be in place")
	}
	err := db.Exec("INSERT INTO users (id, tenant_id, email, display_name, password_hash, status) VALUES ('aaaaaaaa-0000-0000-0000-000000000003', ?, 'Mixed@Case.mx', 'x', 'x', 'active')", schoolTenant).Error
	if err == nil {
		t.Fatal("an unnormalized email must be refused once the CHECK is in place")
	}
}

func TestMigration0018RecordsCollisionsInsteadOfDeleting(t *testing.T) {
	db := migrateUpTo(t, "mig_iam_email_dup", "0018")
	for id, email := range map[string]string{
		"bbbbbbbb-0000-0000-0000-000000000001": "Ana@Colegio.mx",
		"bbbbbbbb-0000-0000-0000-000000000002": "ana@colegio.mx",
		"bbbbbbbb-0000-0000-0000-000000000003": "Luis@Colegio.mx",
	} {
		if err := db.Exec(
			"INSERT INTO users (id, tenant_id, email, display_name, password_hash, status) VALUES (?, ?, ?, 'x', 'x', 'active')",
			id, schoolTenant, email).Error; err != nil {
			t.Fatalf("seed %s: %v", email, err)
		}
	}

	applyMigration(t, db, "0018_email_lowercase.sql")

	var users int64
	db.Raw("SELECT count(*) FROM users").Scan(&users)
	if users != 3 {
		t.Fatalf("no user may be deleted, have %d", users)
	}
	var conflicts []string
	db.Raw("SELECT original_email FROM user_email_conflicts ORDER BY original_email COLLATE \"C\"").Scan(&conflicts)
	if len(conflicts) != 2 || conflicts[0] != "Ana@Colegio.mx" || conflicts[1] != "ana@colegio.mx" {
		t.Fatalf("conflicts = %q, want both spellings of ana", conflicts)
	}
	var luis string
	db.Raw("SELECT email FROM users WHERE id = 'bbbbbbbb-0000-0000-0000-000000000003'").Scan(&luis)
	if luis != "luis@colegio.mx" {
		t.Errorf("a non-colliding email is still normalized, got %q", luis)
	}
	var ana string
	db.Raw("SELECT email FROM users WHERE id = 'bbbbbbbb-0000-0000-0000-000000000001'").Scan(&ana)
	if ana != "Ana@Colegio.mx" {
		t.Errorf("a colliding email is left as it was, got %q", ana)
	}
	if hasConstraint(t, db, "users_email_normalized_ck") {
		t.Error("the CHECK must wait until the collisions are resolved")
	}
}
