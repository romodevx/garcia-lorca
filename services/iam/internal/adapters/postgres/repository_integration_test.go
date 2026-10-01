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
	// superDSNFor builds a superuser DSN for any database in the test container,
	// so a migration test can build a throwaway database of its own.
	superDSNFor func(db string) string
	superDB     *gorm.DB // postgres superuser (bypasses RLS) — for seeding.
	appDSN      string   // connection string for the non-owner app role.
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
	superDSNFor = func(db string) string {
		return fmt.Sprintf("host=%s port=%s user=postgres password=%s dbname=%s sslmode=disable", host, port.Port(), superPass, db)
	}
	superDSN := superDSNFor(dbName)
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

// sqlFiles lists the migration files in migrationsDir in lexical order (0001
// before 0002). It is the one list both the suite schema and the
// data-migration tests are built from.
func sqlFiles() ([]string, error) {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	return files, nil
}

func migrate(db *gorm.DB) error {
	files, err := sqlFiles()
	if err != nil {
		return err
	}
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
	user, gotHash, err := repo.FindAuthByEmail(context.Background(), tenantA, email)
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
	if _, _, err := repoA.FindAuthByEmail(context.Background(), tenantA, emailB); glerrors.KindOf(err) != glerrors.KindNotFound {
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

// TestOwnerUserAuthAndPasswordUpdate self-seeds the owner user (the migration
// seeds only the owner ROLE now — the owner USER is env-driven and never
// committed). It seeds a runtime-hashed temp password via superDB (bypasses
// RLS), then proves the app-role repo reads must_change_password=true, verifies
// the temp password, and clears the flag on UpdatePassword.
func TestOwnerUserAuthAndPasswordUpdate(t *testing.T) {
	const (
		ownerTenant = "00000000-0000-0000-0000-000000000001"
		ownerID     = "00000000-0000-0000-0000-0000000000d1"
		ownerRoleID = "00000000-0000-0000-0000-0000000000a1" // migration-seeded owner role.
		ownerEmail  = "director@garcialorca.edu.mx"
	)

	// bcrypt a neutral test constant at runtime — no committed credential.
	tempHash, err := bcrypt.NewHasher(0).Hash("test-owner-pw")
	if err != nil {
		t.Fatalf("hash temp password: %v", err)
	}

	// Seed the owner user + role assignment as the superuser (bypasses RLS).
	if err := superDB.Exec(
		"INSERT INTO users (id, tenant_id, email, display_name, password_hash, status, must_change_password) VALUES (?, ?, ?, ?, ?, ?, true)",
		ownerID, ownerTenant, ownerEmail, "Director", tempHash, "active").Error; err != nil {
		t.Fatalf("seed owner user: %v", err)
	}
	assign(t, ownerID, ownerRoleID, ownerTenant)
	t.Cleanup(func() {
		// Hermetic: remove the row (and its assignment) this test created.
		_ = superDB.Exec("DELETE FROM user_roles WHERE user_id = ?", ownerID).Error
		_ = superDB.Exec("DELETE FROM users WHERE id = ?", ownerID).Error
	})

	repo := pgadapter.NewUserRepo(appDB(t), ownerTenant)
	email, err := domain.NewEmail(ownerEmail)
	if err != nil {
		t.Fatalf("NewEmail: %v", err)
	}
	user, hash, err := repo.FindAuthByEmail(context.Background(), ownerTenant, email)
	if err != nil {
		t.Fatalf("FindAuthByEmail: %v", err)
	}
	if !user.MustChangePassword() {
		t.Error("owner must_change_password = false, want true")
	}
	if err := bcrypt.NewHasher(0).Verify(hash, "test-owner-pw"); err != nil {
		t.Errorf("seeded temp password does not verify: %v", err)
	}

	if err := repo.UpdatePassword(context.Background(), user.ID(), "H-new"); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
	userAfter, _, err := repo.FindAuthByEmail(context.Background(), ownerTenant, email)
	if err != nil {
		t.Fatalf("FindAuthByEmail after update: %v", err)
	}
	if userAfter.MustChangePassword() {
		t.Error("must_change_password still true after UpdatePassword")
	}
}

// TestCreateUserRoundTripWithRoles provisions a user AS THE NON-OWNER app role
// (through RLS), assigns two roles, then reads the principal back with its roles
// resolved — proving CreateUser + SetUserRoles + the role-resolving read all
// round-trip under RLS. A duplicate email is KindConflict.
func TestCreateUserRoundTripWithRoles(t *testing.T) {
	ctx := context.Background()
	r1 := "abababab-0000-0000-0000-0000000000a1"
	r2 := "abababab-0000-0000-0000-0000000000a2"
	seedRole(t, r1, tenantA, "Coordinador", []string{"users:read", "roles:read"})
	seedRole(t, r2, tenantA, "DocenteA", []string{"students:read"})

	repo := pgadapter.NewUserRepo(appDB(t), tenantA)
	uid := "abababab-0000-0000-0000-000000000001"
	email, _ := domain.NewEmail("nueva@garcialorca.mx")
	user, err := domain.NewUser(uid, tenantA, email, "Nueva", domain.StatusActive, true, nil)
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	hash, _ := bcrypt.NewHasher(4).Hash("test-temp-pw")
	if err := repo.CreateUser(ctx, user, hash); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := repo.SetUserRoles(ctx, uid, []string{r1, r2}); err != nil {
		t.Fatalf("SetUserRoles: %v", err)
	}

	got, gotHash, err := repo.FindAuthByEmail(ctx, tenantA, email)
	if err != nil {
		t.Fatalf("FindAuthByEmail: %v", err)
	}
	if !got.MustChangePassword() {
		t.Error("created user must_change_password = false, want true (temp password)")
	}
	if err := bcrypt.NewHasher(4).Verify(gotHash, "test-temp-pw"); err != nil {
		t.Errorf("stored hash should verify against the temp password: %v", err)
	}
	usersRead, _ := domain.NewPermission("users:read")
	studentsRead, _ := domain.NewPermission("students:read")
	if !got.HasPermission(usersRead) || !got.HasPermission(studentsRead) {
		t.Errorf("assigned roles not resolved on read: %+v", got.RoleIDs())
	}

	// A duplicate email (UNIQUE(tenant_id, email)) is KindConflict.
	dup, _ := domain.NewUser("abababab-0000-0000-0000-000000000002", tenantA, email, "Dup", domain.StatusActive, true, nil)
	if err := repo.CreateUser(ctx, dup, hash); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("duplicate email should be KindConflict, got %v", glerrors.KindOf(err))
	}
}

// TestListUsersResolvesRolesNoNPlusOne lists users with their roles resolved. The
// implementation loads roles for the whole page in ONE join query (loadRolesForUsers),
// so the listing costs two queries regardless of user count — no N+1 by construction.
func TestListUsersResolvesRoles(t *testing.T) {
	ctx := context.Background()
	uid := "acacacac-0000-0000-0000-000000000001"
	rid := "acacacac-0000-0000-0000-0000000000c1"
	seedUser(t, uid, tenantA, "listme@garcialorca.mx", "x", "active")
	seedRole(t, rid, tenantA, "ListRole", []string{"grades:read"})
	assign(t, uid, rid, tenantA)

	repo := pgadapter.NewUserRepo(appDB(t), tenantA)
	users, err := repo.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}

	var found domain.User
	for _, u := range users {
		if u.ID() == uid {
			found = u
		}
	}
	if found.ID() != uid {
		t.Fatalf("ListUsers did not return the seeded user %q", uid)
	}
	gradesRead, _ := domain.NewPermission("grades:read")
	if !found.HasPermission(gradesRead) || len(found.RoleIDs()) != 1 {
		t.Errorf("ListUsers did not resolve the user's roles: RoleIDs=%v", found.RoleIDs())
	}
}

// TestSetStatusSuspend flips a user to suspended (so it can no longer
// authenticate) and reports KindNotFound for an unknown id.
func TestSetStatusSuspend(t *testing.T) {
	ctx := context.Background()
	uid := "adadadad-0000-0000-0000-000000000001"
	seedUser(t, uid, tenantA, "suspendme@garcialorca.mx", "x", "active")

	repo := pgadapter.NewUserRepo(appDB(t), tenantA)
	if err := repo.SetStatus(ctx, uid, domain.StatusSuspended); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	got, err := repo.FindByID(ctx, uid)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.CanAuthenticate() {
		t.Error("suspended user should not be able to authenticate")
	}

	if err := repo.SetStatus(ctx, "adadadad-0000-0000-0000-0000000000ff", domain.StatusSuspended); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("SetStatus on an unknown id should be KindNotFound, got %v", glerrors.KindOf(err))
	}
}

// TestSetUserRolesReplaceIsAtomic proves SetUserRoles replaces the whole set and
// that a bad role id (FK violation) is KindInvalid AND rolls the swap back — the
// delete + inserts share one tenant-bound transaction.
func TestSetUserRolesReplaceIsAtomic(t *testing.T) {
	ctx := context.Background()
	uid := "aeaeaeae-0000-0000-0000-000000000001"
	r1 := "aeaeaeae-0000-0000-0000-0000000000e1"
	r2 := "aeaeaeae-0000-0000-0000-0000000000e2"
	seedUser(t, uid, tenantA, "swap@garcialorca.mx", "x", "active")
	seedRole(t, r1, tenantA, "SwapOne", []string{"students:read"})
	seedRole(t, r2, tenantA, "SwapTwo", []string{"grades:read"})

	repo := pgadapter.NewUserRepo(appDB(t), tenantA)
	if err := repo.SetUserRoles(ctx, uid, []string{r1}); err != nil {
		t.Fatalf("SetUserRoles(r1): %v", err)
	}
	if err := repo.SetUserRoles(ctx, uid, []string{r2}); err != nil {
		t.Fatalf("SetUserRoles(r2): %v", err)
	}
	got, _ := repo.FindByID(ctx, uid)
	studentsRead, _ := domain.NewPermission("students:read")
	gradesRead, _ := domain.NewPermission("grades:read")
	if got.HasPermission(studentsRead) || !got.HasPermission(gradesRead) {
		t.Errorf("replace did not swap roles: RoleIDs=%v", got.RoleIDs())
	}

	// A non-existent role id violates the FK → KindInvalid, and the prior set
	// (r2) survives because the failed swap rolls back.
	ghost := "aeaeaeae-0000-0000-0000-0000000000ff"
	if err := repo.SetUserRoles(ctx, uid, []string{ghost}); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("bad role id should be KindInvalid, got %v", glerrors.KindOf(err))
	}
	after, _ := repo.FindByID(ctx, uid)
	if !after.HasPermission(gradesRead) || len(after.RoleIDs()) != 1 {
		t.Errorf("failed SetUserRoles must roll back, leaving r2 intact: RoleIDs=%v", after.RoleIDs())
	}
}

// TestRoleRepoCRUD exercises CreateRole (+ duplicate-name conflict), ListRoles,
// SetPermissions, and NotFound on an unknown id — all as the app role under RLS.
func TestRoleRepoCRUD(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRoleRepo(appDB(t), tenantA)

	rid := "afafafaf-0000-0000-0000-0000000000f1"
	role, _ := domain.NewRole(rid, "Auditoria", []string{"users:read", "roles:read"})
	if err := repo.CreateRole(ctx, role); err != nil {
		t.Fatalf("CreateRole: %v", err)
	}

	// Duplicate name (UNIQUE(tenant_id, name)) → KindConflict.
	dup, _ := domain.NewRole("afafafaf-0000-0000-0000-0000000000f2", "Auditoria", []string{"users:read"})
	if err := repo.CreateRole(ctx, dup); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("duplicate role name should be KindConflict, got %v", glerrors.KindOf(err))
	}

	roles, err := repo.ListRoles(ctx)
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	var listed domain.Role
	for _, r := range roles {
		if r.ID() == rid {
			listed = r
		}
	}
	if listed.ID() != rid || len(listed.Permissions()) != 2 {
		t.Errorf("ListRoles did not return the created role with 2 permissions: %+v", listed)
	}

	if err := repo.SetPermissions(ctx, rid, []string{"users:write"}); err != nil {
		t.Fatalf("SetPermissions: %v", err)
	}
	updated, _ := repo.FindByID(ctx, rid)
	usersWrite, _ := domain.NewPermission("users:write")
	if len(updated.Permissions()) != 1 || !updated.HasPermission(usersWrite) {
		t.Errorf("SetPermissions did not replace the set: %+v", updated.Permissions())
	}

	if err := repo.SetPermissions(ctx, "afafafaf-0000-0000-0000-0000000000ff", []string{"users:read"}); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("SetPermissions on an unknown id should be KindNotFound, got %v", glerrors.KindOf(err))
	}
}

// TestRLSIsolatesAdminWrites confirms the admin writes stay tenant-isolated: a
// role created in tenant A is invisible to a tenant-B repo, and tenant B cannot
// suspend tenant A's user (RLS hides the row → NotFound).
func TestRLSIsolatesAdminWrites(t *testing.T) {
	ctx := context.Background()
	rid := "ba5eba11-0000-0000-0000-0000000000a1"
	uid := "ba5eba11-0000-0000-0000-000000000001"
	seedRole(t, rid, tenantA, "TenantAOnly", []string{"users:read"})
	seedUser(t, uid, tenantA, "onlya@garcialorca.mx", "x", "active")

	roleRepoB := pgadapter.NewRoleRepo(appDB(t), tenantB)
	if _, err := roleRepoB.FindByID(ctx, rid); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("tenant B saw tenant A's role (RLS breach): err=%v", err)
	}
	userRepoB := pgadapter.NewUserRepo(appDB(t), tenantB)
	if err := userRepoB.SetStatus(ctx, uid, domain.StatusSuspended); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("tenant B suspended tenant A's user (RLS breach): err=%v", glerrors.KindOf(err))
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

// --- seeded role permissions -------------------------------------------------

// TestSeededRolesHoldTheirPermissions closes a gap the permission migrations had
// left open since the first of them: every one is an UPDATE guarded by a NOT
// (permissions @> …) clause, so a migration that matched ZERO rows — a renamed
// role, a changed tenant literal, a typo in the WHERE — would apply cleanly and
// grant nothing. The next thing to notice would be a director who cannot open a
// module, in production.
//
// It asserts what the migrations are supposed to have produced, not how they did
// it, so it guards the ones already merged as well as the ones still to come.
func TestSeededRolesHoldTheirPermissions(t *testing.T) {
	const seedTenant = "00000000-0000-0000-0000-000000000001"

	cases := []struct {
		role string
		want []string
	}{
		{"owner", []string{
			// One per permission migration, in the order they landed.
			"config:read", "config:write",
			// students uses granular verbs, not a blanket :write.
			"students:read", "students:list", "students:create",
			"students:update", "students:enroll",
			"grades:read", "grades:write",
			"users:read", "users:write", "roles:read", "roles:write",
			"finance:read", "finance:write",
			"attendance:read", "attendance:write",
			"comms:read", "comms:write",
			"guardians:read", "guardians:write",
			"teaching:read", "teaching:write",
			"attendance_own:read", "attendance_own:write",
			// M12: the owner holds all four so it can delegate them.
			"uniforms:stock", "uniforms:orders", "uniforms:write",
			"uniforms_own:read", "uniforms_own:write",
			// M14: teachers left Configuración and became a module of its own,
			// so it gates on its own permission — the point being that a school
			// can hand somebody the staff roster without handing them the whole
			// academic catalogue.
			"teachers:read", "teachers:write",
			// The owner holds family:read so it can DELEGATE it: the edge's
			// anti-escalation guard only lets a caller assign roles whose
			// permissions it already holds, so without this nobody at all could
			// assign `guardian` and no family could be given the app. Safe
			// because for the owner it is strictly a subset of what it already
			// reads — profile←students:read, account←finance:read,
			// grades←grades:read, attendance←attendance:read,
			// notifications←comms:read. See ADR 0012.
			"family:read",
			// Task 5: a pupil's medical record has its own permissions — who
			// reads the class list is not thereby entitled to its allergies.
			"medical:read", "medical:write",
			// Task 6: notes on a pupil. notes_write can be delegated (a teacher
			// writes notes); notes_void — voiding anyone's note, at any time —
			// is the owner's alone.
			"students:notes_write", "students:notes_void",
			// Task 9: the owner holds the counter's two permissions so it can
			// delegate them (the caller-subset guard).
			"collections:read", "payments:write",
			// T7: closing a trimester is the owner's decision alone.
			"grades:close",
			// ADR 0021: deleting a level is the director's alone (0024).
			"levels:delete",
			// T2: deleting a user account is the director's alone (0025).
			"users:delete",
			// T9: correcting a registered payment (0026). The owner holds it
			// by default and may delegate it.
			"payments:edit",
		}},
		// Task 9: the cashier. Reads the counter, takes money, finds pupils.
		{"cobranza", []string{"students:read", "students:list", "collections:read", "payments:write"}},
		// T2: the teacher. Only their own grades and attendance, both scoped in
		// the gateway by principal→teacher→assignments of the active cycle.
		{"maestro", []string{"teaching:read", "teaching:write", "attendance_own:read", "attendance_own:write"}},
		{"guardian", []string{
			"family:read",
			"uniforms_own:read", "uniforms_own:write",
		}},
	}

	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			var got []string
			err := superDB.Raw(
				`SELECT unnest(permissions) FROM roles WHERE tenant_id = ? AND name = ?`,
				seedTenant, tc.role).Scan(&got).Error
			if err != nil {
				t.Fatalf("read %s permissions: %v", tc.role, err)
			}
			if len(got) == 0 {
				t.Fatalf("role %q has no permissions at all — did a migration match zero rows?", tc.role)
			}
			held := make(map[string]bool, len(got))
			for _, p := range got {
				held[p] = true
			}
			for _, want := range tc.want {
				if !held[want] {
					t.Errorf("role %q is missing %q", tc.role, want)
				}
			}
		})
	}

	// A role the school had already named "cobranza" is corrected, not skipped:
	// re-running the migration after it was widened brings it back to four.
	t.Run("an existing cobranza role is brought to the counter", func(t *testing.T) {
		if err := superDB.Exec(`UPDATE roles SET permissions = ARRAY['finance:write'] WHERE tenant_id = ? AND name = 'cobranza'`, seedTenant).Error; err != nil {
			t.Fatalf("widen: %v", err)
		}
		sql, err := os.ReadFile(filepath.Join(migrationsDir, "0021_cobranza_role.sql"))
		if err != nil {
			t.Fatalf("read migration: %v", err)
		}
		if err := superDB.Exec(string(sql)).Error; err != nil {
			t.Fatalf("re-run 0021: %v", err)
		}
		var got []string
		if err := superDB.Raw(`SELECT unnest(permissions) FROM roles WHERE tenant_id = ? AND name = 'cobranza'`, seedTenant).Scan(&got).Error; err != nil {
			t.Fatalf("read: %v", err)
		}
		if strings.Join(got, ",") != "students:read,students:list,collections:read,payments:write" {
			t.Errorf("cobranza = %v, want exactly the counter", got)
		}
	})

	// Cobranza holds exactly the counter: nothing more, and nothing owner-only
	// (finance:* stays the owner's).
	t.Run("cobranza holds exactly the counter", func(t *testing.T) {
		var got []string
		if err := superDB.Raw(
			`SELECT unnest(permissions) FROM roles WHERE tenant_id = ? AND name = 'cobranza'`,
			seedTenant).Scan(&got).Error; err != nil {
			t.Fatalf("read cobranza permissions: %v", err)
		}
		if len(got) != 4 {
			t.Errorf("cobranza holds %v, want exactly its four permissions", got)
		}
		for _, p := range got {
			if domain.IsOwnerOnly(p) {
				t.Errorf("cobranza holds the owner-only permission %q", p)
			}
		}
	})

	// T2: an existing "maestro" role (the dev seed made one by hand) is brought
	// to exactly the teacher's four, whatever it held before.
	t.Run("an existing maestro role is brought to the teacher's four", func(t *testing.T) {
		if err := superDB.Exec(`UPDATE roles SET permissions = ARRAY['grades:write','finance:read'] WHERE tenant_id = ? AND name = 'maestro'`, seedTenant).Error; err != nil {
			t.Fatalf("widen: %v", err)
		}
		sql, err := os.ReadFile(filepath.Join(migrationsDir, "0022_maestro_role.sql"))
		if err != nil {
			t.Fatalf("read migration: %v", err)
		}
		if err := superDB.Exec(string(sql)).Error; err != nil {
			t.Fatalf("re-run 0022: %v", err)
		}
		var got []string
		if err := superDB.Raw(`SELECT unnest(permissions) FROM roles WHERE tenant_id = ? AND name = 'maestro'`, seedTenant).Scan(&got).Error; err != nil {
			t.Fatalf("read: %v", err)
		}
		if strings.Join(got, ",") != "teaching:read,teaching:write,attendance_own:read,attendance_own:write" {
			t.Errorf("maestro = %v, want exactly the teacher's four", got)
		}
		for _, p := range got {
			if domain.IsOwnerOnly(p) {
				t.Errorf("maestro holds the owner-only permission %q", p)
			}
		}
	})

	// T7: 0023 is idempotent — re-running it never duplicates grades:close.
	t.Run("re-running 0023 grants grades:close exactly once", func(t *testing.T) {
		sql, err := os.ReadFile(filepath.Join(migrationsDir, "0023_grades_close_permission.sql"))
		if err != nil {
			t.Fatalf("read migration: %v", err)
		}
		if err := superDB.Exec(string(sql)).Error; err != nil {
			t.Fatalf("re-run 0023: %v", err)
		}
		var n int
		if err := superDB.Raw(
			`SELECT count(*) FROM roles, unnest(permissions) p WHERE tenant_id = ? AND id = '00000000-0000-0000-0000-0000000000a1' AND p = 'grades:close'`,
			seedTenant).Scan(&n).Error; err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 1 {
			t.Errorf("owner holds grades:close %d times, want exactly once", n)
		}
	})

	// The maestro role keeps its fixed id, so a deployment can tell it apart from
	// a school-made role of another name.
	t.Run("maestro has its fixed id", func(t *testing.T) {
		var id string
		if err := superDB.Raw(`SELECT id::text FROM roles WHERE tenant_id = ? AND name = 'maestro'`, seedTenant).Scan(&id).Error; err != nil {
			t.Fatalf("read: %v", err)
		}
		if id != "00000000-0000-0000-0000-0000000000a4" {
			t.Errorf("maestro id = %q, want the fixed ...00a4", id)
		}
	})

	// The guardian must NOT have picked up anything tenant-wide along the way.
	t.Run("guardian holds nothing owner-only", func(t *testing.T) {
		var got []string
		if err := superDB.Raw(
			`SELECT unnest(permissions) FROM roles WHERE tenant_id = ? AND name = 'guardian'`,
			seedTenant).Scan(&got).Error; err != nil {
			t.Fatalf("read guardian permissions: %v", err)
		}
		for _, p := range got {
			if domain.IsOwnerOnly(p) {
				t.Errorf("the guardian role holds the owner-only permission %q", p)
			}
		}
	})
}
