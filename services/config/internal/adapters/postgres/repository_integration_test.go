//go:build integration

// Integration tests for the config postgres adapter against a REAL Postgres
// (testcontainers-go). They prove the repository round-trips each catalog entity
// and — critically — that Row Level Security isolates tenants when connected as
// the non-owner `app` role (ADR 0001). They also guard the enum-casing bridge
// (levels store UPPERCASE 'CYCLE'/'TERM') and the parent-FK error mapping.
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
	pgadapter "github.com/garcia-lorca/services/config/internal/adapters/postgres"
	"github.com/garcia-lorca/services/config/internal/domain"
)

const (
	dbName        = "garcia_lorca"
	tenantA       = "11111111-1111-1111-1111-111111111111"
	tenantB       = "22222222-2222-2222-2222-222222222222"
	migrationsDir = "../../../../../migrations/config"
)

var (
	appDSN string // connection string for the non-owner app role.
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

	superDB, err := open(superDSN)
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

func appDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := open(appDSN)
	if err != nil {
		t.Fatalf("open app db: %v", err)
	}
	return db
}

// dates used across tests (date-only; the domain stores the instant).
var (
	startsOn = time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	endsOn   = time.Date(2027, time.July, 15, 0, 0, 0, 0, time.UTC)
)

// --- tests -----------------------------------------------------------------

// TestCatalogRoundTrip persists one of each entity (with a valid parent chain)
// as tenant A through the app role, reads each back, and asserts list scoping.
func TestCatalogRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantA)

	const (
		cycleID = "aaaa0001-0000-0000-0000-000000000001"
		levelID = "aaaa0002-0000-0000-0000-000000000001"
		gradeID = "aaaa0003-0000-0000-0000-000000000001"
		subjID  = "aaaa0004-0000-0000-0000-000000000001"
		termID  = "aaaa0005-0000-0000-0000-000000000001"
		groupID = "aaaa0006-0000-0000-0000-000000000001"
	)

	cycle, err := domain.NewCycle(cycleID, tenantA, "2026-2027", domain.CycleStatusDraft, startsOn, endsOn)
	if err != nil {
		t.Fatalf("NewCycle: %v", err)
	}
	if err := repo.CreateCycle(ctx, cycle); err != nil {
		t.Fatalf("CreateCycle (clean insert, no CHECK violation expected): %v", err)
	}
	gotCycle, err := repo.GetCycle(ctx, cycleID)
	if err != nil {
		t.Fatalf("GetCycle: %v", err)
	}
	if gotCycle.Name() != "2026-2027" || gotCycle.Status() != domain.CycleStatusDraft {
		t.Errorf("cycle round-trip mismatch: %+v", gotCycle)
	}
	if !gotCycle.StartsOn().Equal(startsOn) || !gotCycle.EndsOn().Equal(endsOn) {
		t.Errorf("cycle date bridge mismatch: starts=%v ends=%v", gotCycle.StartsOn(), gotCycle.EndsOn())
	}

	level, err := domain.NewLevel(levelID, tenantA, "Primaria", "PRIM", domain.GranularityTerm)
	if err != nil {
		t.Fatalf("NewLevel: %v", err)
	}
	if err := repo.CreateLevel(ctx, level); err != nil {
		t.Fatalf("CreateLevel: %v", err)
	}
	gotLevel, err := repo.GetLevel(ctx, levelID)
	if err != nil {
		t.Fatalf("GetLevel: %v", err)
	}
	if gotLevel.Granularity() != domain.GranularityTerm {
		t.Errorf("level granularity round-trip: got %v", gotLevel.Granularity())
	}

	grade, err := domain.NewGradeLevel(gradeID, tenantA, levelID, "1 Primaria", 1)
	if err != nil {
		t.Fatalf("NewGradeLevel: %v", err)
	}
	if err := repo.CreateGradeLevel(ctx, grade); err != nil {
		t.Fatalf("CreateGradeLevel: %v", err)
	}
	if _, err := repo.GetGradeLevel(ctx, gradeID); err != nil {
		t.Fatalf("GetGradeLevel: %v", err)
	}
	grades, err := repo.ListGradeLevels(ctx, levelID)
	if err != nil || len(grades) != 1 {
		t.Fatalf("ListGradeLevels: len=%d err=%v", len(grades), err)
	}

	subject, err := domain.NewSubject(subjID, tenantA, gradeID, "Matematicas")
	if err != nil {
		t.Fatalf("NewSubject: %v", err)
	}
	if err := repo.CreateSubject(ctx, subject); err != nil {
		t.Fatalf("CreateSubject: %v", err)
	}
	subjects, err := repo.ListSubjects(ctx, gradeID)
	if err != nil || len(subjects) != 1 {
		t.Fatalf("ListSubjects: len=%d err=%v", len(subjects), err)
	}

	term, err := domain.NewTerm(termID, tenantA, cycleID, levelID, "T1", 1, startsOn, endsOn)
	if err != nil {
		t.Fatalf("NewTerm: %v", err)
	}
	if err := repo.CreateTerm(ctx, term); err != nil {
		t.Fatalf("CreateTerm: %v", err)
	}
	gotTerm, err := repo.GetTerm(ctx, termID)
	if err != nil {
		t.Fatalf("GetTerm: %v", err)
	}
	if !gotTerm.StartsOn().Equal(startsOn) || !gotTerm.EndsOn().Equal(endsOn) {
		t.Errorf("term date bridge mismatch: starts=%v ends=%v", gotTerm.StartsOn(), gotTerm.EndsOn())
	}
	// List scoping: the cycle's terms include this term; narrowing to the level works.
	if terms, err := repo.ListTerms(ctx, cycleID, ""); err != nil || len(terms) != 1 {
		t.Fatalf("ListTerms(cycle): len=%d err=%v", len(terms), err)
	}
	if terms, err := repo.ListTerms(ctx, cycleID, levelID); err != nil || len(terms) != 1 {
		t.Fatalf("ListTerms(cycle, level): len=%d err=%v", len(terms), err)
	}

	group, err := domain.NewGroup(groupID, tenantA, cycleID, gradeID, "A", 30)
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	if err := repo.CreateGroup(ctx, group); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if _, err := repo.GetGroup(ctx, groupID); err != nil {
		t.Fatalf("GetGroup: %v", err)
	}
	if groups, err := repo.ListGroups(ctx, cycleID, gradeID); err != nil || len(groups) != 1 {
		t.Fatalf("ListGroups(cycle, grade): len=%d err=%v", len(groups), err)
	}
}

// TestRLSIsolatesTenants proves app.current_tenant isolates tenants on the
// config tables (representative on cycles): a cycle created by tenant A is
// invisible and unwritable to a tenant-B-scoped connection.
func TestRLSIsolatesTenants(t *testing.T) {
	ctx := context.Background()
	const cycleID = "bbbb0001-0000-0000-0000-000000000001"

	repoA := pgadapter.NewRepository(appDB(t), tenantA)
	cycle, err := domain.NewCycle(cycleID, tenantA, "Isolation-Cycle-A", domain.CycleStatusActive, startsOn, endsOn)
	if err != nil {
		t.Fatalf("NewCycle: %v", err)
	}
	if err := repoA.CreateCycle(ctx, cycle); err != nil {
		t.Fatalf("tenant A CreateCycle: %v", err)
	}

	// Tenant B cannot see it via Get or List.
	repoB := pgadapter.NewRepository(appDB(t), tenantB)
	if _, err := repoB.GetCycle(ctx, cycleID); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("tenant B saw tenant A's cycle via Get (RLS breach): err=%v", err)
	}
	cyclesB, err := repoB.ListCycles(ctx)
	if err != nil {
		t.Fatalf("tenant B ListCycles: %v", err)
	}
	for _, c := range cyclesB {
		if c.ID() == cycleID {
			t.Errorf("tenant B listed tenant A's cycle (RLS breach)")
		}
	}

	// Tenant B cannot write it: RLS makes the row invisible, so UPDATE/DELETE
	// as a tenant-B-bound connection affect zero rows.
	if n := writeAsTenant(t, tenantB, "UPDATE cycles SET name = 'hijacked' WHERE id = ?", cycleID); n != 0 {
		t.Errorf("tenant B UPDATE affected %d rows (RLS breach)", n)
	}
	if n := writeAsTenant(t, tenantB, "DELETE FROM cycles WHERE id = ?", cycleID); n != 0 {
		t.Errorf("tenant B DELETE affected %d rows (RLS breach)", n)
	}

	// Tenant A still owns an untouched row.
	gotA, err := repoA.GetCycle(ctx, cycleID)
	if err != nil {
		t.Fatalf("tenant A GetCycle after B's writes: %v", err)
	}
	if gotA.Name() != "Isolation-Cycle-A" {
		t.Errorf("tenant A's cycle was mutated across the tenant boundary: %q", gotA.Name())
	}
}

// TestCreateGroupMissingGradeLevelIsInvalid proves the FK-violation mapping:
// creating a group whose grade_level_id has no row surfaces as KindInvalid.
func TestCreateGroupMissingGradeLevelIsInvalid(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantA)

	const cycleID = "cccc0001-0000-0000-0000-000000000001"
	cycle, err := domain.NewCycle(cycleID, tenantA, "FK-Test-Cycle", domain.CycleStatusDraft, startsOn, endsOn)
	if err != nil {
		t.Fatalf("NewCycle: %v", err)
	}
	if err := repo.CreateCycle(ctx, cycle); err != nil {
		t.Fatalf("CreateCycle: %v", err)
	}

	// grade_level_id points at a row that does not exist.
	group, err := domain.NewGroup(
		"cccc0002-0000-0000-0000-000000000001", tenantA, cycleID,
		"dddd9999-0000-0000-0000-000000000001", "A", 30)
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	if err := repo.CreateGroup(ctx, group); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("missing parent grade level should be KindInvalid, got %v", glerrors.KindOf(err))
	}
}

// TestEnrollmentGranularityRoundTrip guards the casing bridge: a CYCLE and a
// TERM level persist (satisfying the UPPERCASE DDL CHECK) and read back as the
// matching domain granularity. A wrong write casing would violate the CHECK.
func TestEnrollmentGranularityRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantA)

	tests := []struct {
		id   string
		name string
		gran domain.EnrollmentGranularity
	}{
		{"eeee0001-0000-0000-0000-000000000001", "Preescolar", domain.GranularityCycle},
		{"eeee0002-0000-0000-0000-000000000001", "Secundaria", domain.GranularityTerm},
	}
	for _, tt := range tests {
		level, err := domain.NewLevel(tt.id, tenantA, tt.name, "", tt.gran)
		if err != nil {
			t.Fatalf("NewLevel(%s): %v", tt.name, err)
		}
		if err := repo.CreateLevel(ctx, level); err != nil {
			t.Fatalf("CreateLevel(%s) (CHECK violation would mean wrong casing): %v", tt.name, err)
		}
		got, err := repo.GetLevel(ctx, tt.id)
		if err != nil {
			t.Fatalf("GetLevel(%s): %v", tt.name, err)
		}
		if got.Granularity() != tt.gran {
			t.Errorf("granularity round-trip for %s: got %v, want %v", tt.name, got.Granularity(), tt.gran)
		}
	}
}

// writeAsTenant runs a single write on the app connection inside a transaction
// bound to tenantID (same set_config primitive as pkg/authctx) and returns the
// affected row count. It lets the RLS test observe UPDATE/DELETE visibility
// without an Update/Delete port method.
func writeAsTenant(t *testing.T, tenantID, sql string, args ...any) int64 {
	t.Helper()
	var affected int64
	err := appDB(t).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantID).Error; err != nil {
			return err
		}
		res := tx.Exec(sql, args...)
		if res.Error != nil {
			return res.Error
		}
		affected = res.RowsAffected
		return nil
	})
	if err != nil {
		t.Fatalf("writeAsTenant(%q): %v", sql, err)
	}
	return affected
}
