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
	tenantC       = "33333333-3333-3333-3333-333333333333"
	tenantD       = "44444444-4444-4444-4444-444444444444"
	tenantE       = "55555555-5555-5555-5555-555555555555"
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

// TestListGroupsEmptyFiltersReturnAll proves the empty-filter contract on
// ListGroups (empty cycleID / gradeLevelID = no filter). Before the fix,
// ListGroups(ctx, "", "") ran `cycle_id = ”` against a uuid column (22P02) —
// reachable from the teacher roster's group-name enrichment, which calls
// ListGroups with an empty cycleId to build a groupId→name map.
func TestListGroupsEmptyFiltersReturnAll(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantE)

	const (
		cycleID  = "e0e00001-0000-0000-0000-000000000001"
		levelID  = "e0e00002-0000-0000-0000-000000000001"
		gradeID1 = "e0e00003-0000-0000-0000-000000000001"
		gradeID2 = "e0e00003-0000-0000-0000-000000000002"
		groupA   = "e0e00004-0000-0000-0000-000000000001"
		groupB   = "e0e00004-0000-0000-0000-000000000002"
		groupC   = "e0e00004-0000-0000-0000-000000000003"
	)

	cycle, err := domain.NewCycle(cycleID, tenantE, "Cycle-Groups-E", domain.CycleStatusActive, startsOn, endsOn)
	if err != nil {
		t.Fatalf("NewCycle: %v", err)
	}
	if err := repo.CreateCycle(ctx, cycle); err != nil {
		t.Fatalf("CreateCycle: %v", err)
	}
	level, err := domain.NewLevel(levelID, tenantE, "Level-Groups-E", "", domain.GranularityTerm)
	if err != nil {
		t.Fatalf("NewLevel: %v", err)
	}
	if err := repo.CreateLevel(ctx, level); err != nil {
		t.Fatalf("CreateLevel: %v", err)
	}
	for id, ord := range map[string]int{gradeID1: 1, gradeID2: 2} {
		grade, err := domain.NewGradeLevel(id, tenantE, levelID, fmt.Sprintf("Grade-%d", ord), ord)
		if err != nil {
			t.Fatalf("NewGradeLevel: %v", err)
		}
		if err := repo.CreateGradeLevel(ctx, grade); err != nil {
			t.Fatalf("CreateGradeLevel: %v", err)
		}
	}
	// Two groups under grade 1, one under grade 2 → three across the tenant/cycle.
	seed := []struct {
		id, gradeID, name string
	}{
		{groupA, gradeID1, "A"},
		{groupB, gradeID1, "B"},
		{groupC, gradeID2, "A"},
	}
	for _, g := range seed {
		group, err := domain.NewGroup(g.id, tenantE, cycleID, g.gradeID, g.name, 30)
		if err != nil {
			t.Fatalf("NewGroup(%s): %v", g.id, err)
		}
		if err := repo.CreateGroup(ctx, group); err != nil {
			t.Fatalf("CreateGroup(%s): %v", g.id, err)
		}
	}

	// Fully-open filter returns all groups of the tenant (the red case: empty
	// cycle_id against a uuid column).
	all, err := repo.ListGroups(ctx, "", "")
	if err != nil {
		t.Fatalf("ListGroups(ctx, \"\", \"\") must not error (22P02 regression): %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListGroups(ctx, \"\", \"\"): got %d, want 3", len(all))
	}

	// Cycle only (grade level open) still works and finds all three.
	byCycle, err := repo.ListGroups(ctx, cycleID, "")
	if err != nil || len(byCycle) != 3 {
		t.Fatalf("ListGroups(ctx, cycleID, \"\"): len=%d err=%v", len(byCycle), err)
	}

	// Grade level only (cycle open) narrows to that grade level.
	byGrade, err := repo.ListGroups(ctx, "", gradeID1)
	if err != nil || len(byGrade) != 2 {
		t.Fatalf("ListGroups(ctx, \"\", gradeID1): len=%d err=%v", len(byGrade), err)
	}

	// Both filters set narrows to the cycle+grade intersection.
	both, err := repo.ListGroups(ctx, cycleID, gradeID1)
	if err != nil || len(both) != 2 {
		t.Fatalf("ListGroups(ctx, cycleID, gradeID1): len=%d err=%v", len(both), err)
	}
}

// TestListGradeLevelsEmptyFilterReturnsAll proves the empty-filter contract on
// ListGradeLevels (empty levelID = no filter). Before the fix,
// ListGradeLevels(ctx, "") ran `level_id = ”` against a uuid column and
// Postgres rejected it (22P02) — reachable from the gateway grade-levels
// handler, which forwards a missing levelId param verbatim. A dedicated tenant
// keeps the counts deterministic.
func TestListGradeLevelsEmptyFilterReturnsAll(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantD)

	const (
		levelID1 = "d0d00001-0000-0000-0000-000000000001"
		levelID2 = "d0d00001-0000-0000-0000-000000000002"
		gradeA   = "d0d00002-0000-0000-0000-000000000001"
		gradeB   = "d0d00002-0000-0000-0000-000000000002"
		gradeC   = "d0d00002-0000-0000-0000-000000000003"
	)

	levels := []struct{ id, name string }{
		{levelID1, "Primaria-D"},
		{levelID2, "Secundaria-D"},
	}
	for _, l := range levels {
		level, err := domain.NewLevel(l.id, tenantD, l.name, "", domain.GranularityTerm)
		if err != nil {
			t.Fatalf("NewLevel(%s): %v", l.name, err)
		}
		if err := repo.CreateLevel(ctx, level); err != nil {
			t.Fatalf("CreateLevel(%s): %v", l.name, err)
		}
	}
	// Two grade levels under level 1, one under level 2 → three across the tenant.
	seed := []struct {
		id, levelID, name string
		ordinal           int
	}{
		{gradeA, levelID1, "Grade-A", 1},
		{gradeB, levelID1, "Grade-B", 2},
		{gradeC, levelID2, "Grade-C", 1},
	}
	for _, g := range seed {
		grade, err := domain.NewGradeLevel(g.id, tenantD, g.levelID, g.name, g.ordinal)
		if err != nil {
			t.Fatalf("NewGradeLevel(%s): %v", g.id, err)
		}
		if err := repo.CreateGradeLevel(ctx, grade); err != nil {
			t.Fatalf("CreateGradeLevel(%s): %v", g.id, err)
		}
	}

	// Empty filter returns ALL grade levels across levels (the red case).
	all, err := repo.ListGradeLevels(ctx, "")
	if err != nil {
		t.Fatalf("ListGradeLevels(ctx, \"\") must not error (22P02 regression): %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListGradeLevels(ctx, \"\"): got %d, want 3", len(all))
	}

	// A concrete level still narrows to that level only.
	one, err := repo.ListGradeLevels(ctx, levelID1)
	if err != nil {
		t.Fatalf("ListGradeLevels(ctx, levelID1): %v", err)
	}
	if len(one) != 2 {
		t.Errorf("ListGradeLevels(ctx, levelID1): got %d, want 2", len(one))
	}
}

// TestListSubjectsEmptyFilterReturnsAll proves the empty-filter contract on
// ListSubjects (empty gradeLevelID = no filter). Before the fix,
// ListSubjects(ctx, "") ran `grade_level_id = ”` against a uuid column and
// Postgres rejected it (22P02). A dedicated tenant keeps the counts
// deterministic regardless of what other tests seed.
func TestListSubjectsEmptyFilterReturnsAll(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantC)

	const (
		levelID  = "c0c00001-0000-0000-0000-000000000001"
		gradeID1 = "c0c00002-0000-0000-0000-000000000001"
		gradeID2 = "c0c00002-0000-0000-0000-000000000002"
		subjA    = "c0c00003-0000-0000-0000-000000000001"
		subjB    = "c0c00003-0000-0000-0000-000000000002"
		subjC    = "c0c00003-0000-0000-0000-000000000003"
	)

	level, err := domain.NewLevel(levelID, tenantC, "Primaria-C", "", domain.GranularityTerm)
	if err != nil {
		t.Fatalf("NewLevel: %v", err)
	}
	if err := repo.CreateLevel(ctx, level); err != nil {
		t.Fatalf("CreateLevel: %v", err)
	}
	for id, ord := range map[string]int{gradeID1: 1, gradeID2: 2} {
		grade, err := domain.NewGradeLevel(id, tenantC, levelID, fmt.Sprintf("Grade-%d", ord), ord)
		if err != nil {
			t.Fatalf("NewGradeLevel: %v", err)
		}
		if err := repo.CreateGradeLevel(ctx, grade); err != nil {
			t.Fatalf("CreateGradeLevel: %v", err)
		}
	}
	// Two subjects under grade 1, one under grade 2 → three across the tenant.
	seed := []struct{ id, gradeID, name string }{
		{subjA, gradeID1, "Matematicas"},
		{subjB, gradeID1, "Espanol"},
		{subjC, gradeID2, "Ciencias"},
	}
	for _, s := range seed {
		subject, err := domain.NewSubject(s.id, tenantC, s.gradeID, s.name)
		if err != nil {
			t.Fatalf("NewSubject(%s): %v", s.name, err)
		}
		if err := repo.CreateSubject(ctx, subject); err != nil {
			t.Fatalf("CreateSubject(%s): %v", s.name, err)
		}
	}

	// Empty filter returns ALL subjects across grade levels (the red case).
	all, err := repo.ListSubjects(ctx, "")
	if err != nil {
		t.Fatalf("ListSubjects(ctx, \"\") must not error (22P02 regression): %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListSubjects(ctx, \"\"): got %d, want 3", len(all))
	}

	// A concrete grade level still narrows to that level only.
	one, err := repo.ListSubjects(ctx, gradeID1)
	if err != nil {
		t.Fatalf("ListSubjects(ctx, gradeID1): %v", err)
	}
	if len(one) != 2 {
		t.Errorf("ListSubjects(ctx, gradeID1): got %d, want 2", len(one))
	}
}

// TestListTermsEmptyFiltersReturnAll proves the empty-filter contract on
// ListTerms (empty cycleID / levelID = no filter). Before the fix,
// ListTerms(ctx, "", "") ran `cycle_id = ”` against a uuid column (22P02).
func TestListTermsEmptyFiltersReturnAll(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantC)

	const (
		cycleID = "c1c10001-0000-0000-0000-000000000001"
		levelID = "c1c10002-0000-0000-0000-000000000001"
		termT1  = "c1c10003-0000-0000-0000-000000000001"
		termT2  = "c1c10003-0000-0000-0000-000000000002"
	)

	cycle, err := domain.NewCycle(cycleID, tenantC, "Cycle-Terms-C", domain.CycleStatusActive, startsOn, endsOn)
	if err != nil {
		t.Fatalf("NewCycle: %v", err)
	}
	if err := repo.CreateCycle(ctx, cycle); err != nil {
		t.Fatalf("CreateCycle: %v", err)
	}
	level, err := domain.NewLevel(levelID, tenantC, "Level-Terms-C", "", domain.GranularityTerm)
	if err != nil {
		t.Fatalf("NewLevel: %v", err)
	}
	if err := repo.CreateLevel(ctx, level); err != nil {
		t.Fatalf("CreateLevel: %v", err)
	}
	for id, ord := range map[string]int{termT1: 1, termT2: 2} {
		term, err := domain.NewTerm(id, tenantC, cycleID, levelID, fmt.Sprintf("T%d", ord), ord, startsOn, endsOn)
		if err != nil {
			t.Fatalf("NewTerm: %v", err)
		}
		if err := repo.CreateTerm(ctx, term); err != nil {
			t.Fatalf("CreateTerm: %v", err)
		}
	}

	// Fully-open filter returns all terms of the tenant (the red case: empty
	// cycle_id against a uuid column).
	all, err := repo.ListTerms(ctx, "", "")
	if err != nil {
		t.Fatalf("ListTerms(ctx, \"\", \"\") must not error (22P02 regression): %v", err)
	}
	if len(all) != 2 {
		t.Errorf("ListTerms(ctx, \"\", \"\"): got %d, want 2", len(all))
	}

	// Partial filter: cycle only (level open) still works and finds both.
	byCycle, err := repo.ListTerms(ctx, cycleID, "")
	if err != nil || len(byCycle) != 2 {
		t.Fatalf("ListTerms(ctx, cycleID, \"\"): len=%d err=%v", len(byCycle), err)
	}

	// Partial filter: level only (cycle open) also works and finds both.
	byLevel, err := repo.ListTerms(ctx, "", levelID)
	if err != nil || len(byLevel) != 2 {
		t.Fatalf("ListTerms(ctx, \"\", levelID): len=%d err=%v", len(byLevel), err)
	}

	// Both filters set narrows to the cycle+level intersection.
	both, err := repo.ListTerms(ctx, cycleID, levelID)
	if err != nil || len(both) != 2 {
		t.Fatalf("ListTerms(ctx, cycleID, levelID): len=%d err=%v", len(both), err)
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

// TestTeacherRoundTrip persists a teacher WITHOUT an IAM link (user_id NULL) and
// one WITH a user_id set, then reads both back, asserting the "" ↔ NULL bridge
// maps in both directions.
func TestTeacherRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantA)

	const (
		noUserID  = "ffff0001-0000-0000-0000-000000000001"
		hasUserID = "ffff0002-0000-0000-0000-000000000001"
		linkedUID = "99990001-0000-0000-0000-000000000001"
	)

	// user_id "" must persist as SQL NULL and read back as "".
	unlinked, err := domain.NewTeacher(noUserID, tenantA, "Ada", "Lovelace", "ada@example.com", "")
	if err != nil {
		t.Fatalf("NewTeacher (unlinked): %v", err)
	}
	if err := repo.CreateTeacher(ctx, unlinked); err != nil {
		t.Fatalf("CreateTeacher (unlinked, NULL user_id): %v", err)
	}
	gotUnlinked, err := repo.GetTeacher(ctx, noUserID)
	if err != nil {
		t.Fatalf("GetTeacher (unlinked): %v", err)
	}
	if gotUnlinked.HasUser() || gotUnlinked.UserID() != "" {
		t.Errorf("NULL user_id should map to \"\": HasUser=%v UserID=%q", gotUnlinked.HasUser(), gotUnlinked.UserID())
	}
	if gotUnlinked.Email() != "ada@example.com" || gotUnlinked.FirstName() != "Ada" || gotUnlinked.LastName() != "Lovelace" {
		t.Errorf("teacher round-trip mismatch: %+v", gotUnlinked)
	}

	// A set user_id must persist and read back verbatim.
	linked, err := domain.NewTeacher(hasUserID, tenantA, "Alan", "Turing", "alan@example.com", linkedUID)
	if err != nil {
		t.Fatalf("NewTeacher (linked): %v", err)
	}
	if err := repo.CreateTeacher(ctx, linked); err != nil {
		t.Fatalf("CreateTeacher (linked): %v", err)
	}
	gotLinked, err := repo.GetTeacher(ctx, hasUserID)
	if err != nil {
		t.Fatalf("GetTeacher (linked): %v", err)
	}
	if !gotLinked.HasUser() || gotLinked.UserID() != linkedUID {
		t.Errorf("set user_id round-trip: HasUser=%v UserID=%q, want %q", gotLinked.HasUser(), gotLinked.UserID(), linkedUID)
	}

	// A duplicate email (per tenant) is KindConflict (UNIQUE(tenant_id, email)).
	dup, err := domain.NewTeacher("ffff0003-0000-0000-0000-000000000001", tenantA, "Ada", "Byron", "ada@example.com", "")
	if err != nil {
		t.Fatalf("NewTeacher (dup): %v", err)
	}
	if err := repo.CreateTeacher(ctx, dup); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("duplicate teacher email should be KindConflict, got %v", glerrors.KindOf(err))
	}

	teachers, err := repo.ListTeachers(ctx)
	if err != nil {
		t.Fatalf("ListTeachers: %v", err)
	}
	if len(teachers) != 2 {
		t.Errorf("ListTeachers: got %d, want 2", len(teachers))
	}
}

// TestSetTeacherUserIDLinkAndUnlink links a teacher to an IAM user then unlinks
// it (SQL NULL), round-tripping both directions as the non-owner app role under
// RLS. An unknown teacher id is KindNotFound.
func TestSetTeacherUserIDLinkAndUnlink(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantA)

	tid := "eeee0001-0000-0000-0000-000000000001"
	te, err := domain.NewTeacher(tid, tenantA, "Grace", "Hopper", "grace.link@example.com", "")
	if err != nil {
		t.Fatalf("NewTeacher: %v", err)
	}
	if err := repo.CreateTeacher(ctx, te); err != nil {
		t.Fatalf("CreateTeacher: %v", err)
	}

	// Link.
	link := "99990002-0000-0000-0000-000000000001"
	if err := repo.SetTeacherUserID(ctx, tid, &link); err != nil {
		t.Fatalf("SetTeacherUserID (link): %v", err)
	}
	got, err := repo.GetTeacher(ctx, tid)
	if err != nil {
		t.Fatalf("GetTeacher (linked): %v", err)
	}
	if !got.HasUser() || got.UserID() != link {
		t.Errorf("link round-trip: HasUser=%v UserID=%q, want %q", got.HasUser(), got.UserID(), link)
	}

	// Unlink (NULL).
	if err := repo.SetTeacherUserID(ctx, tid, nil); err != nil {
		t.Fatalf("SetTeacherUserID (unlink): %v", err)
	}
	got, err = repo.GetTeacher(ctx, tid)
	if err != nil {
		t.Fatalf("GetTeacher (unlinked): %v", err)
	}
	if got.HasUser() || got.UserID() != "" {
		t.Errorf("unlink round-trip: HasUser=%v UserID=%q, want \"\"", got.HasUser(), got.UserID())
	}

	// Unknown teacher id → KindNotFound.
	if err := repo.SetTeacherUserID(ctx, "eeee0001-0000-0000-0000-0000000000ff", &link); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("unknown teacher should be KindNotFound, got %v", glerrors.KindOf(err))
	}
}

// teachingAssignmentFixture builds a valid, mutually-consistent parent chain for
// a teaching assignment (a group and a subject under the SAME grade level, plus a
// teacher) under tenantID via the app role, returning the group/subject/teacher
// ids the knot needs. The prefix keeps ids unique across tests sharing the DB.
func teachingAssignmentFixture(t *testing.T, tenantID, prefix string) (cycleID, groupID, subjectID, teacherID string) {
	t.Helper()
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantID)

	cycleID = prefix + "0000-0000-0000-0000-000000000001"
	levelID := prefix + "0000-0000-0000-0000-000000000002"
	gradeID := prefix + "0000-0000-0000-0000-000000000003"
	subjectID = prefix + "0000-0000-0000-0000-000000000004"
	groupID = prefix + "0000-0000-0000-0000-000000000005"
	teacherID = prefix + "0000-0000-0000-0000-000000000006"

	cycle, err := domain.NewCycle(cycleID, tenantID, "Knot-Cycle-"+prefix, domain.CycleStatusActive, startsOn, endsOn)
	if err != nil {
		t.Fatalf("fixture NewCycle: %v", err)
	}
	if err := repo.CreateCycle(ctx, cycle); err != nil {
		t.Fatalf("fixture CreateCycle: %v", err)
	}
	level, err := domain.NewLevel(levelID, tenantID, "Level-"+prefix, "", domain.GranularityTerm)
	if err != nil {
		t.Fatalf("fixture NewLevel: %v", err)
	}
	if err := repo.CreateLevel(ctx, level); err != nil {
		t.Fatalf("fixture CreateLevel: %v", err)
	}
	grade, err := domain.NewGradeLevel(gradeID, tenantID, levelID, "Grade-"+prefix, 1)
	if err != nil {
		t.Fatalf("fixture NewGradeLevel: %v", err)
	}
	if err := repo.CreateGradeLevel(ctx, grade); err != nil {
		t.Fatalf("fixture CreateGradeLevel: %v", err)
	}
	// Subject and group share this grade level (the knot's teachability invariant).
	subject, err := domain.NewSubject(subjectID, tenantID, gradeID, "Subject-"+prefix)
	if err != nil {
		t.Fatalf("fixture NewSubject: %v", err)
	}
	if err := repo.CreateSubject(ctx, subject); err != nil {
		t.Fatalf("fixture CreateSubject: %v", err)
	}
	group, err := domain.NewGroup(groupID, tenantID, cycleID, gradeID, "A", 30)
	if err != nil {
		t.Fatalf("fixture NewGroup: %v", err)
	}
	if err := repo.CreateGroup(ctx, group); err != nil {
		t.Fatalf("fixture CreateGroup: %v", err)
	}
	teacher, err := domain.NewTeacher(teacherID, tenantID, "Grace", "Hopper", "grace-"+prefix+"@example.com", "")
	if err != nil {
		t.Fatalf("fixture NewTeacher: %v", err)
	}
	if err := repo.CreateTeacher(ctx, teacher); err != nil {
		t.Fatalf("fixture CreateTeacher: %v", err)
	}
	return cycleID, groupID, subjectID, teacherID
}

// TestTeachingAssignmentRoundTrip persists a teaching assignment against a valid
// (group, subject same grade level, teacher) chain, reads it back, and asserts
// the cycle_id matches the group's cycle. A duplicate (cycle, group, subject) is
// KindConflict (the UNIQUE backstop).
func TestTeachingAssignmentRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantA)

	cycleID, groupID, subjectID, teacherID := teachingAssignmentFixture(t, tenantA, "a0a0")
	const assignID = "a0a07000-0000-0000-0000-000000000001"

	assign, err := domain.NewTeachingAssignment(assignID, tenantA, cycleID, groupID, subjectID, teacherID)
	if err != nil {
		t.Fatalf("NewTeachingAssignment: %v", err)
	}
	if err := repo.CreateTeachingAssignment(ctx, assign); err != nil {
		t.Fatalf("CreateTeachingAssignment (valid knot insert): %v", err)
	}

	got, err := repo.GetTeachingAssignment(ctx, assignID)
	if err != nil {
		t.Fatalf("GetTeachingAssignment: %v", err)
	}
	if got.GroupID() != groupID || got.SubjectID() != subjectID || got.TeacherID() != teacherID {
		t.Errorf("assignment round-trip mismatch: %+v", got)
	}
	if got.CycleID() != cycleID {
		t.Errorf("assignment cycle_id = %q, want the group's cycle %q", got.CycleID(), cycleID)
	}

	// List filters: by teacher, by group, by cycle all find the one assignment.
	if rows, err := repo.ListTeachingAssignments(ctx, "", "", teacherID); err != nil || len(rows) != 1 {
		t.Fatalf("ListTeachingAssignments(teacher): len=%d err=%v", len(rows), err)
	}
	if rows, err := repo.ListTeachingAssignments(ctx, cycleID, groupID, ""); err != nil || len(rows) != 1 {
		t.Fatalf("ListTeachingAssignments(cycle, group): len=%d err=%v", len(rows), err)
	}

	// A duplicate (cycle, group, subject) is rejected by the UNIQUE constraint.
	dup, err := domain.NewTeachingAssignment("a0a07000-0000-0000-0000-000000000002", tenantA, cycleID, groupID, subjectID, teacherID)
	if err != nil {
		t.Fatalf("NewTeachingAssignment (dup): %v", err)
	}
	if err := repo.CreateTeachingAssignment(ctx, dup); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("duplicate (cycle, group, subject) should be KindConflict, got %v", glerrors.KindOf(err))
	}
}

// TestRLSIsolatesTeachers proves app.current_tenant isolates the teachers table:
// a teacher created by tenant A is invisible AND unwritable to a tenant-B
// connection (read + write isolation, as the non-owner app role).
func TestRLSIsolatesTeachers(t *testing.T) {
	ctx := context.Background()
	const teacherID = "b0b00001-0000-0000-0000-000000000001"

	repoA := pgadapter.NewRepository(appDB(t), tenantA)
	teacher, err := domain.NewTeacher(teacherID, tenantA, "Isolation", "Teacher-A", "iso-teacher-a@example.com", "")
	if err != nil {
		t.Fatalf("NewTeacher: %v", err)
	}
	if err := repoA.CreateTeacher(ctx, teacher); err != nil {
		t.Fatalf("tenant A CreateTeacher: %v", err)
	}

	// Read isolation: tenant B sees it via neither Get nor List.
	repoB := pgadapter.NewRepository(appDB(t), tenantB)
	if _, err := repoB.GetTeacher(ctx, teacherID); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("tenant B saw tenant A's teacher via Get (RLS breach): err=%v", err)
	}
	teachersB, err := repoB.ListTeachers(ctx)
	if err != nil {
		t.Fatalf("tenant B ListTeachers: %v", err)
	}
	for _, te := range teachersB {
		if te.ID() == teacherID {
			t.Errorf("tenant B listed tenant A's teacher (RLS breach)")
		}
	}

	// Write isolation: tenant B's UPDATE/DELETE affect zero rows.
	if n := writeAsTenant(t, tenantB, "UPDATE teachers SET last_name = 'hijacked' WHERE id = ?", teacherID); n != 0 {
		t.Errorf("tenant B UPDATE teachers affected %d rows (RLS breach)", n)
	}
	if n := writeAsTenant(t, tenantB, "DELETE FROM teachers WHERE id = ?", teacherID); n != 0 {
		t.Errorf("tenant B DELETE teachers affected %d rows (RLS breach)", n)
	}

	// Tenant A's row is untouched.
	gotA, err := repoA.GetTeacher(ctx, teacherID)
	if err != nil {
		t.Fatalf("tenant A GetTeacher after B's writes: %v", err)
	}
	if gotA.LastName() != "Teacher-A" {
		t.Errorf("tenant A's teacher was mutated across the tenant boundary: %q", gotA.LastName())
	}
}

// TestRLSIsolatesTeachingAssignments proves app.current_tenant isolates the
// teaching_assignments table: an assignment created by tenant A is invisible AND
// unwritable to a tenant-B connection (read + write isolation, app role).
func TestRLSIsolatesTeachingAssignments(t *testing.T) {
	ctx := context.Background()

	cycleID, groupID, subjectID, teacherID := teachingAssignmentFixture(t, tenantA, "b1b1")
	const assignID = "b1b17000-0000-0000-0000-000000000001"

	repoA := pgadapter.NewRepository(appDB(t), tenantA)
	assign, err := domain.NewTeachingAssignment(assignID, tenantA, cycleID, groupID, subjectID, teacherID)
	if err != nil {
		t.Fatalf("NewTeachingAssignment: %v", err)
	}
	if err := repoA.CreateTeachingAssignment(ctx, assign); err != nil {
		t.Fatalf("tenant A CreateTeachingAssignment: %v", err)
	}

	// Read isolation.
	repoB := pgadapter.NewRepository(appDB(t), tenantB)
	if _, err := repoB.GetTeachingAssignment(ctx, assignID); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("tenant B saw tenant A's assignment via Get (RLS breach): err=%v", err)
	}
	rowsB, err := repoB.ListTeachingAssignments(ctx, "", "", "")
	if err != nil {
		t.Fatalf("tenant B ListTeachingAssignments: %v", err)
	}
	for _, a := range rowsB {
		if a.ID() == assignID {
			t.Errorf("tenant B listed tenant A's assignment (RLS breach)")
		}
	}

	// Write isolation.
	if n := writeAsTenant(t, tenantB, "UPDATE teaching_assignments SET subject_id = subject_id WHERE id = ?", assignID); n != 0 {
		t.Errorf("tenant B UPDATE teaching_assignments affected %d rows (RLS breach)", n)
	}
	if n := writeAsTenant(t, tenantB, "DELETE FROM teaching_assignments WHERE id = ?", assignID); n != 0 {
		t.Errorf("tenant B DELETE teaching_assignments affected %d rows (RLS breach)", n)
	}

	// Tenant A's row survives.
	if _, err := repoA.GetTeachingAssignment(ctx, assignID); err != nil {
		t.Fatalf("tenant A GetTeachingAssignment after B's writes: %v", err)
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
