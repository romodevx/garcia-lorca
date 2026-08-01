//go:build integration

// Integration tests for the iam postgres adapter against a REAL Postgres
// (testcontainers-go). They prove the repository round-trips and — critically —
// that Row Level Security isolates tenants when connected as the non-owner `app`
// role (ADR 0001, risk R1).
//
// Run: go test -tags=integration ./internal/adapters/postgres/
// Requires Docker.
package postgres_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/adapters/bcrypt"
	pgadapter "github.com/garcia-lorca/services/iam/internal/adapters/postgres"
	"github.com/garcia-lorca/services/iam/internal/domain"
)

const (
	dbName        = "garcia_lorca"
	tenantA       = "11111111-1111-1111-1111-111111111111"
	tenantB       = "22222222-2222-2222-2222-222222222222"
	migrationsDir = "../../../../../migrations/iam"
)

var (
	superDB *gorm.DB // postgres superuser (bypasses RLS) — for seeding.
	appDSN  string   // connection string for the non-owner app role.
	// Ephemeral credentials for the throwaway test container, generated at
	// runtime so no password literal is ever committed.
	superPass = randToken()
	appPass   = randToken()
)

func randToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func TestMain(m *testing.M) {
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx,
		"postgres:16-bookworm",
		tcpostgres.WithDatabase(dbName),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword(superPass),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start container: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = container.Terminate(ctx) }()

	host, _ := container.Host(ctx)
	port, _ := container.MappedPort(ctx, "5432")
	superDSN := fmt.Sprintf("host=%s port=%s user=postgres password=%s dbname=%s sslmode=disable", host, port.Port(), superPass, dbName)
	appDSN = fmt.Sprintf("host=%s port=%s user=app password=%s dbname=%s sslmode=disable", host, port.Port(), appPass, dbName)

	superDB, err = open(superDSN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open super: %v\n", err)
		os.Exit(1)
	}
	if err := migrate(superDB); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func open(dsn string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
}

func migrate(db *gorm.DB) error {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files) // lexical order: 0001 before 0002.
	for _, name := range files {
		sql, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if err := db.Exec(string(sql)).Error; err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
	}
	// Non-owner, non-BYPASSRLS app role with DML only (mirrors setup_app_role.sql).
	setup := []string{
		"CREATE ROLE app LOGIN PASSWORD '" + appPass + "' NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE",
		"GRANT USAGE ON SCHEMA public TO app",
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO app",
	}
	for _, stmt := range setup {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("app role setup (%q): %w", stmt, err)
		}
	}
	return nil
}

// seedUser inserts a user as the superuser (bypasses RLS).
func seedUser(t *testing.T, id, tenant, email, hash, status string) {
	t.Helper()
	err := superDB.Exec(
		"INSERT INTO users (id, tenant_id, email, display_name, password_hash, status) VALUES (?, ?, ?, ?, ?, ?)",
		id, tenant, email, email, hash, status).Error
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func seedRole(t *testing.T, id, tenant, name string, perms []string) {
	t.Helper()
	arr := "{" // build a text[] literal
	for i, p := range perms {
		if i > 0 {
			arr += ","
		}
		arr += p
	}
	arr += "}"
	err := superDB.Exec(
		"INSERT INTO roles (id, tenant_id, name, permissions) VALUES (?, ?, ?, ?)",
		id, tenant, name, arr).Error
	if err != nil {
		t.Fatalf("seed role: %v", err)
	}
}

func assign(t *testing.T, userID, roleID, tenant string) {
	t.Helper()
	err := superDB.Exec(
		"INSERT INTO user_roles (user_id, role_id, tenant_id) VALUES (?, ?, ?)",
		userID, roleID, tenant).Error
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
}

func appDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := open(appDSN)
	if err != nil {
		t.Fatalf("open app db: %v", err)
	}
	return db
}

// --- tests -----------------------------------------------------------------

func TestFindAuthByEmailReturnsUserAndHash(t *testing.T) {
	uid := "aaaaaaaa-0000-0000-0000-000000000001"
	rid := "aaaaaaaa-0000-0000-0000-0000000000a1"
	hash, _ := bcrypt.NewHasher(4).Hash("secret")
	seedUser(t, uid, tenantA, "ana@garcialorca.mx", hash, "active")
	seedRole(t, rid, tenantA, "Docente", []string{"students:read", "grades:create"})
	assign(t, uid, rid, tenantA)

	repo := pgadapter.NewUserRepo(appDB(t), tenantA)
	email, _ := domain.NewEmail("ana@garcialorca.mx")
	user, gotHash, _, err := repo.FindAuthByEmail(context.Background(), tenantA, email)
	if err != nil {
		t.Fatalf("FindAuthByEmail: %v", err)
	}
	if user.ID() != uid || user.DisplayName() != "ana@garcialorca.mx" {
		t.Errorf("user mismatch: %+v", user)
	}
	if gotHash != hash {
		t.Errorf("hash not returned verbatim")
	}
	// The stored hash verifies against the original password (repo ↔ bcrypt).
	if err := bcrypt.NewHasher(4).Verify(gotHash, "secret"); err != nil {
		t.Errorf("stored hash should verify: %v", err)
	}
	perm, _ := domain.NewPermission("students:read")
	if !user.HasPermission(perm) {
		t.Errorf("roles not resolved: user should have students:read")
	}
}

func TestRLSIsolatesTenants(t *testing.T) {
	uB := "bbbbbbbb-0000-0000-0000-000000000001"
	seedUser(t, uB, tenantB, "beto@garcialorca.mx", "x", "active")

	// A repo scoped to tenant A must NOT see tenant B's user.
	repoA := pgadapter.NewUserRepo(appDB(t), tenantA)
	if _, err := repoA.FindByID(context.Background(), uB); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("tenant A saw tenant B's user (RLS breach): err=%v", err)
	}
	emailB, _ := domain.NewEmail("beto@garcialorca.mx")
	if _, _, _, err := repoA.FindAuthByEmail(context.Background(), tenantA, emailB); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("tenant A found tenant B by email (RLS breach): err=%v", err)
	}

	// A repo scoped to tenant B sees it.
	repoB := pgadapter.NewUserRepo(appDB(t), tenantB)
	if _, err := repoB.FindByID(context.Background(), uB); err != nil {
		t.Errorf("tenant B should see its own user: %v", err)
	}
}

func TestAddAndRemoveRole(t *testing.T) {
	uid := "cccccccc-0000-0000-0000-000000000001"
	rid := "cccccccc-0000-0000-0000-0000000000c1"
	seedUser(t, uid, tenantA, "caro@garcialorca.mx", "x", "active")
	seedRole(t, rid, tenantA, "Tutor", []string{"attendance:read"})

	repo := pgadapter.NewUserRepo(appDB(t), tenantA)
	ctx := context.Background()
	if err := repo.AddRole(ctx, uid, rid); err != nil {
		t.Fatalf("AddRole: %v", err)
	}
	// Idempotent: a second assignment is a no-op, not an error.
	if err := repo.AddRole(ctx, uid, rid); err != nil {
		t.Fatalf("AddRole (again): %v", err)
	}
	user, _ := repo.FindByID(ctx, uid)
	perm, _ := domain.NewPermission("attendance:read")
	if !user.HasPermission(perm) {
		t.Fatalf("role not assigned")
	}

	if err := repo.RemoveRole(ctx, uid, rid); err != nil {
		t.Fatalf("RemoveRole: %v", err)
	}
	user, _ = repo.FindByID(ctx, uid)
	if user.HasPermission(perm) {
		t.Errorf("role still present after RemoveRole")
	}
}

func TestOwnerSeedAndPasswordUpdate(t *testing.T) {
	const ownerTenant = "00000000-0000-0000-0000-000000000001"
	repo := pgadapter.NewUserRepo(appDB(t), ownerTenant)

	email, err := domain.NewEmail("director@garcialorca.edu.mx")
	if err != nil {
		t.Fatalf("NewEmail: %v", err)
	}
	user, hash, mustChange, err := repo.FindAuthByEmail(context.Background(), ownerTenant, email)
	if err != nil {
		t.Fatalf("FindAuthByEmail: %v", err)
	}
	if !mustChange {
		t.Error("owner must_change_password = false, want true")
	}
	if err := bcrypt.NewHasher(0).Verify(hash, "GarciaLorca2026!"); err != nil {
		t.Errorf("seeded temp password does not verify: %v", err)
	}

	if err := repo.UpdatePassword(context.Background(), user.ID(), "H-new"); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
	_, _, mustChangeAfter, err := repo.FindAuthByEmail(context.Background(), ownerTenant, email)
	if err != nil {
		t.Fatalf("FindAuthByEmail after update: %v", err)
	}
	if mustChangeAfter {
		t.Error("must_change_password still true after UpdatePassword")
	}
}

func TestRoleRepoFindByID(t *testing.T) {
	rid := "dddddddd-0000-0000-0000-0000000000d1"
	seedRole(t, rid, tenantA, "Direccion", []string{"students:read", "finance:read"})

	repo := pgadapter.NewRoleRepo(appDB(t), tenantA)
	role, err := repo.FindByID(context.Background(), rid)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if role.Name() != "Direccion" || len(role.Permissions()) != 2 {
		t.Errorf("role mismatch: %+v", role)
	}

	if _, err := repo.FindByID(context.Background(), "eeeeeeee-0000-0000-0000-0000000000e1"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("unknown role should be NotFound, got %v", err)
	}
}
