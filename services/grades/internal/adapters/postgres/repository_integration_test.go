//go:build integration

// Integration tests for the grades postgres adapter against a REAL Postgres
// (testcontainers-go). They prove the upsert round-trips (one row per
// student·subject·term, value edited in place, the reserved published_at column
// left NULL), that the DB value CHECK rejects out-of-range grades, and —
// critically — that RLS isolates tenants for both reads AND writes when
// connected as the non-owner `app` role (same posture as students, ADR 0001).
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
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/grades/internal/adapters/postgres"
	"github.com/garcia-lorca/services/grades/internal/domain"
)

const (
	dbName  = "garcia_lorca"
	tenantA = "11111111-1111-1111-1111-111111111111"
	tenantB = "22222222-2222-2222-2222-222222222222"
)

// migrationFiles are applied in order before the app role is granted, so the
// non-owner role gets privileges on every table.
var migrationFiles = []string{
	"../../../../../migrations/grades/0001_init.sql",
}

var (
	superDB   *gorm.DB
	appDSN    string
	superPass = randToken()
	appPass   = randToken()
)

// Neutral cross-service references (config/students UUIDs) + a fixed clock so the
// tests are deterministic.
const (
	enrollmentOne = "e0000000-0000-0000-0000-000000000001"
	enrollmentTwo = "e0000000-0000-0000-0000-000000000002"
	// enrollmentRLS is used only by the RLS isolation test so no other test
	// pre-populates a tenant-A row for its key (the container is shared and not
	// reset between tests).
	enrollmentRLS = "e0000000-0000-0000-0000-000000000003"
	subjectOne    = "50000000-0000-0000-0000-000000000001"
	termOne       = "70000000-0000-0000-0000-000000000001"
	teacherOne    = "60000000-0000-0000-0000-000000000001"
)

var captureClock = time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

func randToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func TestMain(m *testing.M) {
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:16-bookworm",
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

	if superDB, err = open(superDSN); err != nil {
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
	for _, f := range migrationFiles {
		sql, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if err := db.Exec(string(sql)).Error; err != nil {
			return fmt.Errorf("apply %s: %w", f, err)
		}
	}
	for _, stmt := range []string{
		"CREATE ROLE app LOGIN PASSWORD '" + appPass + "' NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE",
		"GRANT USAGE ON SCHEMA public TO app",
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO app",
	} {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("%q: %w", stmt, err)
		}
	}
	return nil
}

func appRepo(t *testing.T, tenant string) *pgadapter.Repository {
	t.Helper()
	db, err := open(appDSN)
	if err != nil {
		t.Fatalf("open app db: %v", err)
	}
	return pgadapter.NewRepository(db, tenant)
}

func gradeEntry(t *testing.T, id, tenant, enrollment string, value float64) domain.GradeEntry {
	t.Helper()
	e, err := domain.NewGradeEntry(id, tenant, enrollment, subjectOne, termOne, teacherOne, value, captureClock)
	if err != nil {
		t.Fatalf("new grade entry: %v", err)
	}
	return e
}

// countEntries reads grade_entries as the non-owner app role with the tenant GUC
// bound (mirrors the adapter's RLS binding), so it exercises RLS: a tenant sees
// only its own rows.
func countEntries(t *testing.T, tenant, enrollment string) int64 {
	t.Helper()
	db, err := open(appDSN)
	if err != nil {
		t.Fatalf("open app db: %v", err)
	}
	var n int64
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenant).Error; err != nil {
			return err
		}
		return tx.Table("grade_entries").Where("enrollment_id = ?", enrollment).Count(&n).Error
	})
	if err != nil {
		t.Fatalf("count entries: %v", err)
	}
	return n
}

// publishedAtIsNull reads the reserved published_at column of a stored entry as
// the app role with the tenant bound, proving capture never writes it.
func publishedAtIsNull(t *testing.T, tenant, enrollment string) bool {
	t.Helper()
	db, err := open(appDSN)
	if err != nil {
		t.Fatalf("open app db: %v", err)
	}
	var nulls int64
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenant).Error; err != nil {
			return err
		}
		return tx.Table("grade_entries").
			Where("enrollment_id = ? AND published_at IS NULL", enrollment).
			Count(&nulls).Error
	})
	if err != nil {
		t.Fatalf("check published_at: %v", err)
	}
	return nulls == 1
}

// --- tests -----------------------------------------------------------------

// TestUpsertRoundTripEditsInPlace proves the upsert is idempotent on its key:
// a second capture of the same student·subject·term updates the value on the SAME
// row (not a new insert), and the reserved published_at column stays NULL.
func TestUpsertRoundTripEditsInPlace(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	id := "aaaaaaaa-0000-0000-0000-000000000001"

	first, err := repo.UpsertGradeEntry(ctx, gradeEntry(t, id, tenantA, enrollmentOne, 6))
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if first.Value() != 6 || first.SubjectID() != subjectOne || first.TeacherID() != teacherOne {
		t.Fatalf("stored entry mismatch: %+v", first)
	}

	// Re-capture the same key with a different value + a different candidate id:
	// it must edit the existing row in place, keeping the original id.
	second, err := repo.UpsertGradeEntry(ctx, gradeEntry(t, "aaaaaaaa-0000-0000-0000-000000000099", tenantA, enrollmentOne, 9))
	if err != nil {
		t.Fatalf("re-capture: %v", err)
	}
	if second.Value() != 9 {
		t.Errorf("re-capture did not update value: %v", second.Value())
	}
	if second.ID() != first.ID() {
		t.Errorf("upsert changed the row id: first=%q second=%q", first.ID(), second.ID())
	}

	// Exactly one row for this key (edit, not insert).
	if n := countEntries(t, tenantA, enrollmentOne); n != 1 {
		t.Errorf("grade rows = %d, want 1 (upsert edited in place)", n)
	}
	// A read via the port reflects the edited value.
	got, err := repo.GetForKey(ctx, enrollmentOne, subjectOne, termOne)
	if err != nil {
		t.Fatalf("GetForKey: %v", err)
	}
	if got.Value() != 9 {
		t.Errorf("GetForKey value = %v, want 9", got.Value())
	}
	// The reserved boleta column is never written on capture.
	if !publishedAtIsNull(t, tenantA, enrollmentOne) {
		t.Error("published_at must stay NULL after a capture")
	}
}

// TestGetForKeyMissingIsNotFound proves an absent key surfaces as KindNotFound.
func TestGetForKeyMissingIsNotFound(t *testing.T) {
	repo := appRepo(t, tenantA)
	_, err := repo.GetForKey(context.Background(), "e0000000-0000-0000-0000-0000000000ff", subjectOne, termOne)
	if glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("missing key: want KindNotFound, got %v", err)
	}
}

// TestValueCheckRejectsOutOfRange proves the DB CHECK (value 0..10) is a
// defense-in-depth below the domain guard: a raw insert of 11 is rejected by
// Postgres even when it bypasses the domain constructor.
func TestValueCheckRejectsOutOfRange(t *testing.T) {
	db, err := open(appDSN)
	if err != nil {
		t.Fatalf("open app db: %v", err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantA).Error; err != nil {
			return err
		}
		return tx.Exec(
			"INSERT INTO grade_entries (tenant_id, enrollment_id, subject_id, term_id, teacher_id, value) VALUES (?, ?, ?, ?, ?, ?)",
			tenantA, enrollmentTwo, subjectOne, termOne, teacherOne, 11,
		).Error
	})
	if err == nil {
		t.Fatal("DB accepted an out-of-range value (11); the CHECK must reject it")
	}
}

// TestRLSIsolatesGradeEntries proves read AND write isolation on `grade_entries`
// for the non-owner app role: tenant A cannot read or mutate tenant B's grade.
func TestRLSIsolatesGradeEntries(t *testing.T) {
	ctx := context.Background()
	idB := "bbbbbbbb-0000-0000-0000-000000000001"
	if _, err := appRepo(t, tenantB).UpsertGradeEntry(ctx, gradeEntry(t, idB, tenantB, enrollmentRLS, 7)); err != nil {
		t.Fatalf("seed B: %v", err)
	}

	// Read isolation: tenant A cannot see tenant B's grade; tenant B can.
	if _, err := appRepo(t, tenantA).GetForKey(ctx, enrollmentRLS, subjectOne, termOne); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("tenant A saw tenant B's grade (RLS read breach): %v", err)
	}
	if _, err := appRepo(t, tenantB).GetForKey(ctx, enrollmentRLS, subjectOne, termOne); err != nil {
		t.Errorf("tenant B should see its own grade: %v", err)
	}

	// Write isolation: tenant A "upserting" the same (enrollment, subject, term)
	// key cannot touch tenant B's row — the tenant is part of the unique key and
	// RLS bounds the transaction, so tenant A inserts its OWN separate row and
	// tenant B's value stays 7.
	if _, err := appRepo(t, tenantA).UpsertGradeEntry(ctx, gradeEntry(t, "aaaaaaaa-0000-0000-0000-0000000000a1", tenantA, enrollmentRLS, 3)); err != nil {
		t.Fatalf("tenant A upsert: %v", err)
	}
	afterB, err := appRepo(t, tenantB).GetForKey(ctx, enrollmentRLS, subjectOne, termOne)
	if err != nil {
		t.Fatalf("re-read grade B: %v", err)
	}
	if afterB.Value() != 7 {
		t.Errorf("tenant A's write leaked into tenant B: value = %v, want 7", afterB.Value())
	}
	// Each tenant sees exactly its own single row for this enrollment.
	if n := countEntries(t, tenantA, enrollmentRLS); n != 1 {
		t.Errorf("tenant A rows = %d, want 1", n)
	}
	if n := countEntries(t, tenantB, enrollmentRLS); n != 1 {
		t.Errorf("tenant B rows = %d, want 1", n)
	}
}
