//go:build integration

// Integration tests for the students postgres adapter against a REAL Postgres
// (testcontainers-go). They prove the repository round-trips, CURP uniqueness,
// keyset pagination, and — critically — that RLS isolates tenants when connected
// as the non-owner `app` role (same posture as iam, ADR 0001).
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
	pgadapter "github.com/garcia-lorca/services/students/internal/adapters/postgres"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

const (
	dbName  = "garcia_lorca"
	tenantA = "11111111-1111-1111-1111-111111111111"
	tenantB = "22222222-2222-2222-2222-222222222222"
)

// migrationFiles are applied in order before the app role is granted, so the
// non-owner role gets privileges on every table (students + enrollments +
// guardian links).
var migrationFiles = []string{
	"../../../../../migrations/students/0001_init.sql",
	"../../../../../migrations/students/0002_enrollments.sql",
	"../../../../../migrations/students/0003_guardians.sql",
	"../../../../../migrations/students/0004_maternal_level.sql",
	"../../../../../migrations/students/0005_contacts.sql",
}

var (
	superDB   *gorm.DB
	appDSN    string
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

func applicant(t *testing.T, id, tenant, curp, group string) *domain.Student {
	t.Helper()
	c, err := domain.NewCURP(curp)
	if err != nil {
		t.Fatalf("curp: %v", err)
	}
	s, err := domain.NewApplicant(domain.NewApplicantParams{
		ID: id, TenantID: tenant, CURP: c, FirstName: "Ana", LastName: "H",
		Level: domain.LevelPrimaria, GradeGroup: group,
	})
	if err != nil {
		t.Fatalf("applicant: %v", err)
	}
	return s
}

// --- tests -----------------------------------------------------------------

func TestCreateFindAndTransition(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	id := "aaaaaaaa-0000-0000-0000-000000000001"
	if err := repo.Create(ctx, applicant(t, id, tenantA, "HEGA050312MDFRNN08", "4° A")); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Status() != domain.AdmissionPending || got.GradeGroup() != "4° A" {
		t.Errorf("round-trip mismatch: %+v", got)
	}

	// Persist a transition (accept → enrolled) and re-read.
	_, _ = got.Accept()
	_, _, _ = got.Enroll("cycle-2026")
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	reread, _ := repo.FindByID(ctx, id)
	if reread.Status() != domain.AdmissionEnrolled {
		t.Errorf("status not persisted: %v", reread.Status())
	}
}

func TestDuplicateCURPIsConflict(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	_ = repo.Create(ctx, applicant(t, "aaaaaaaa-0000-0000-0000-000000000010", tenantA, "MEXA080101HDFRNN01", "1° A"))
	err := repo.Create(ctx, applicant(t, "aaaaaaaa-0000-0000-0000-000000000011", tenantA, "MEXA080101HDFRNN01", "1° B"))
	if glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("duplicate CURP: want KindConflict, got %v", err)
	}
}

func TestRLSIsolatesTenants(t *testing.T) {
	idB := "bbbbbbbb-0000-0000-0000-000000000001"
	if err := appRepo(t, tenantB).Create(context.Background(), applicant(t, idB, tenantB, "LOBA070202MDFRNN02", "2° A")); err != nil {
		t.Fatalf("seed B: %v", err)
	}
	// Tenant A must not see tenant B's student.
	if _, err := appRepo(t, tenantA).FindByID(context.Background(), idB); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("tenant A saw tenant B's student (RLS breach): %v", err)
	}
	// Tenant B sees its own.
	if _, err := appRepo(t, tenantB).FindByID(context.Background(), idB); err != nil {
		t.Errorf("tenant B should see its own student: %v", err)
	}
	// The same CURP may exist in a different tenant (uniqueness is per-tenant).
	if err := appRepo(t, tenantA).Create(context.Background(), applicant(t, "aaaaaaaa-0000-0000-0000-000000000099", tenantA, "LOBA070202MDFRNN02", "2° A")); err != nil {
		t.Errorf("same CURP in another tenant should be allowed: %v", err)
	}
}

func TestListFilterAndPaginate(t *testing.T) {
	repo := appRepo(t, tenantB)
	ctx := context.Background()
	// Seed 3 secundaria students in tenant B (distinct CURPs + ids ordered).
	curps := []string{"SEBA090303HDFRNN03", "SEBB090303HDFRNN04", "SEBC090303HDFRNN05"}
	for i, c := range curps {
		id := fmt.Sprintf("bbbbbbbb-1111-0000-0000-00000000000%d", i+1)
		s, _ := domain.NewApplicant(domain.NewApplicantParams{
			ID: id, TenantID: tenantB, CURP: mustCURP(t, c), FirstName: "X", LastName: "Y",
			Level: domain.LevelSecundaria, GradeGroup: "1° A",
		})
		if err := repo.Create(ctx, s); err != nil {
			t.Fatalf("seed list %d: %v", i, err)
		}
	}

	// Filter by level=secundaria, page size 2 → 2 rows + a next token.
	page1, next, err := repo.List(ctx, ports.ListFilter{Level: domain.LevelSecundaria, PageSize: 2})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(page1) != 2 || next == "" {
		t.Fatalf("page1 = %d rows, next=%q; want 2 + a token", len(page1), next)
	}
	page2, next2, _ := repo.List(ctx, ports.ListFilter{Level: domain.LevelSecundaria, PageSize: 2, PageToken: next})
	if len(page2) != 1 || next2 != "" {
		t.Errorf("page2 = %d rows, next=%q; want 1 + empty (last page)", len(page2), next2)
	}
}

func mustCURP(t *testing.T, s string) domain.CURP {
	t.Helper()
	c, err := domain.NewCURP(s)
	if err != nil {
		t.Fatalf("curp %q: %v", s, err)
	}
	return c
}

// --- enrollment tests ------------------------------------------------------

// Neutral cross-service references (config UUIDs) + a fixed clock so the
// enrollment tests are deterministic.
const (
	cycleUUID  = "c0000000-0000-0000-0000-000000000001"
	groupOne   = "40000000-0000-0000-0000-000000000001"
	groupTwo   = "40000000-0000-0000-0000-000000000002"
	groupThree = "40000000-0000-0000-0000-000000000003"
)

var enrollClock = time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

// seedStudent inserts a student (same service) to hang an enrollment off and
// returns its id.
func seedStudent(t *testing.T, tenant, id, curp string) string {
	t.Helper()
	if err := appRepo(t, tenant).Create(context.Background(), applicant(t, id, tenant, curp, "1° A")); err != nil {
		t.Fatalf("seed student: %v", err)
	}
	return id
}

func enrollment(t *testing.T, id, tenant, studentID, group string) domain.Enrollment {
	t.Helper()
	e, err := domain.NewEnrollment(id, tenant, studentID, cycleUUID, group, "", domain.EnrollmentStatusActive, enrollClock)
	if err != nil {
		t.Fatalf("new enrollment: %v", err)
	}
	return e
}

// countTransfers reads enrollment_transfers as the non-owner app role with the
// tenant GUC bound (mirrors the adapter's RLS binding), so it exercises RLS: a
// tenant sees only its own transfer rows.
func countTransfers(t *testing.T, tenant, enrollmentID string) int64 {
	t.Helper()
	db, err := open(appDSN)
	if err != nil {
		t.Fatalf("open app db: %v", err)
	}
	var n int64
	err = db.Transaction(func(tx *gorm.DB) error {
		// set_config(..., true) is the parameterizable equivalent of SET LOCAL,
		// exactly what pkg/authctx binds per transaction.
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenant).Error; err != nil {
			return err
		}
		return tx.Table("enrollment_transfers").Where("enrollment_id = ?", enrollmentID).Count(&n).Error
	})
	if err != nil {
		t.Fatalf("count transfers: %v", err)
	}
	return n
}

// TestEnrollmentRoundTripAndTransfer proves the enrollment round-trips and that a
// transfer is ATOMIC: after TransferEnrollment the enrollment's group changed AND
// exactly one transfer row exists with the right from/to/reason.
func TestEnrollmentRoundTripAndTransfer(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	studentID := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-00000000000a", "HEGB050312MDFRNN10")
	enrID := "e0000000-0000-0000-0000-00000000000a"

	if err := repo.CreateEnrollment(ctx, enrollment(t, enrID, tenantA, studentID, groupOne)); err != nil {
		t.Fatalf("CreateEnrollment: %v", err)
	}

	got, err := repo.GetEnrollment(ctx, enrID)
	if err != nil {
		t.Fatalf("GetEnrollment: %v", err)
	}
	if got.GroupID() != groupOne || got.CycleID() != cycleUUID || got.Status() != domain.EnrollmentStatusActive {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	// Transfer group1 → group2 with a reason.
	const reason = "grupo balanceado"
	transfer, err := domain.NewEnrollmentTransfer("70000000-0000-0000-0000-00000000000a", tenantA, enrID, groupOne, groupTwo, reason, enrollClock)
	if err != nil {
		t.Fatalf("new transfer: %v", err)
	}
	if err := repo.TransferEnrollment(ctx, enrID, groupTwo, transfer); err != nil {
		t.Fatalf("TransferEnrollment: %v", err)
	}

	// The group moved.
	moved, err := repo.GetEnrollment(ctx, enrID)
	if err != nil {
		t.Fatalf("GetEnrollment after transfer: %v", err)
	}
	if moved.GroupID() != groupTwo {
		t.Errorf("group not moved: got %q want %q", moved.GroupID(), groupTwo)
	}
	// Exactly one transfer row exists (atomic history write).
	if n := countTransfers(t, tenantA, enrID); n != 1 {
		t.Errorf("transfer rows = %d, want 1", n)
	}

	// ListEnrollments filters by group: the enrollment now shows under group2, not group1.
	byGroupTwo, err := repo.ListEnrollments(ctx, "", "", groupTwo)
	if err != nil {
		t.Fatalf("ListEnrollments: %v", err)
	}
	if len(byGroupTwo) != 1 || byGroupTwo[0].ID() != enrID {
		t.Errorf("list by group2 = %+v, want the transferred enrollment", byGroupTwo)
	}
}

// TestTransferAtomicRollsBackOnMissingEnrollment proves the two writes share one
// transaction: transferring an enrollment the tenant cannot see leaves NO
// transfer row behind (the group UPDATE matches zero rows → rollback).
func TestTransferAtomicRollsBackOnMissingEnrollment(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	const ghostEnrollment = "e0000000-0000-0000-0000-0000000000ff"
	transfer, err := domain.NewEnrollmentTransfer("70000000-0000-0000-0000-0000000000ff", tenantA, ghostEnrollment, groupOne, groupTwo, "n/a", enrollClock)
	if err != nil {
		t.Fatalf("new transfer: %v", err)
	}
	// The enrollment FK does not exist → the insert itself is rejected as Invalid.
	if err := repo.TransferEnrollment(ctx, ghostEnrollment, groupTwo, transfer); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("transfer of missing enrollment: want KindInvalid, got %v", err)
	}
	if n := countTransfers(t, tenantA, ghostEnrollment); n != 0 {
		t.Errorf("a transfer row leaked after a failed transfer: %d rows", n)
	}
}

// TestDuplicateEnrollmentIsConflict proves the UNIQUE(tenant, student, cycle)
// constraint surfaces as KindConflict.
func TestDuplicateEnrollmentIsConflict(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	studentID := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-00000000000c", "HEGC050312MDFRNN12")

	if err := repo.CreateEnrollment(ctx, enrollment(t, "e0000000-0000-0000-0000-00000000000c", tenantA, studentID, groupOne)); err != nil {
		t.Fatalf("first enrollment: %v", err)
	}
	// Same (student, cycle), different id/group → conflict.
	err := repo.CreateEnrollment(ctx, enrollment(t, "e0000000-0000-0000-0000-00000000000d", tenantA, studentID, groupTwo))
	if glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("duplicate enrollment: want KindConflict, got %v", err)
	}
}

// TestRLSIsolatesEnrollments proves read AND write isolation on `enrollments`
// for the non-owner app role: tenant A cannot read or mutate tenant B's
// enrollment.
func TestRLSIsolatesEnrollments(t *testing.T) {
	ctx := context.Background()
	studentB := seedStudent(t, tenantB, "dddddddd-0000-0000-0000-00000000000b", "HEGD050312HDFRNN11")
	enrB := "e0000000-0000-0000-0000-00000000000b"
	if err := appRepo(t, tenantB).CreateEnrollment(ctx, enrollment(t, enrB, tenantB, studentB, groupOne)); err != nil {
		t.Fatalf("seed enrollment B: %v", err)
	}

	// Read isolation: tenant A cannot see tenant B's enrollment; tenant B can.
	if _, err := appRepo(t, tenantA).GetEnrollment(ctx, enrB); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("tenant A saw tenant B's enrollment (RLS read breach): %v", err)
	}
	if _, err := appRepo(t, tenantB).GetEnrollment(ctx, enrB); err != nil {
		t.Errorf("tenant B should see its own enrollment: %v", err)
	}

	// Write isolation: tenant A cannot mutate tenant B's enrollment (no visible
	// row → NotFound), and the status stays active for tenant B.
	if err := appRepo(t, tenantA).SetEnrollmentStatus(ctx, enrB, domain.EnrollmentStatusWithdrawn); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("tenant A mutated tenant B's enrollment (RLS write breach): %v", err)
	}
	after, err := appRepo(t, tenantB).GetEnrollment(ctx, enrB)
	if err != nil {
		t.Fatalf("re-read enrollment B: %v", err)
	}
	if after.Status() != domain.EnrollmentStatusActive {
		t.Errorf("tenant A's write leaked into tenant B: status = %v", after.Status())
	}
}

// TestRLSIsolatesTransfers proves read AND write isolation on
// `enrollment_transfers`: a tenant reads only its own transfer rows, and cannot
// write a transfer against another tenant's enrollment.
func TestRLSIsolatesTransfers(t *testing.T) {
	ctx := context.Background()
	studentA := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-00000000000e", "HEGB050312HDFRNN13")
	enrA := "e0000000-0000-0000-0000-00000000000e"
	if err := appRepo(t, tenantA).CreateEnrollment(ctx, enrollment(t, enrA, tenantA, studentA, groupOne)); err != nil {
		t.Fatalf("seed enrollment A: %v", err)
	}
	transfer, err := domain.NewEnrollmentTransfer("70000000-0000-0000-0000-00000000000e", tenantA, enrA, groupOne, groupTwo, "cambio", enrollClock)
	if err != nil {
		t.Fatalf("new transfer: %v", err)
	}
	if err := appRepo(t, tenantA).TransferEnrollment(ctx, enrA, groupTwo, transfer); err != nil {
		t.Fatalf("seed transfer A: %v", err)
	}

	// Read isolation: tenant A sees its transfer row; tenant B sees none.
	if n := countTransfers(t, tenantA, enrA); n != 1 {
		t.Errorf("tenant A should see 1 transfer, saw %d", n)
	}
	if n := countTransfers(t, tenantB, enrA); n != 0 {
		t.Errorf("tenant B saw tenant A's transfer rows (RLS read breach): %d", n)
	}

	// Write isolation: tenant B cannot transfer tenant A's enrollment (the
	// enrollment is invisible → the group UPDATE matches zero rows → rollback),
	// and no new transfer row is created under tenant A.
	ghost, err := domain.NewEnrollmentTransfer("70000000-0000-0000-0000-00000000001e", tenantB, enrA, groupTwo, groupThree, "intruso", enrollClock)
	if err != nil {
		t.Fatalf("new ghost transfer: %v", err)
	}
	if err := appRepo(t, tenantB).TransferEnrollment(ctx, enrA, groupThree, ghost); err == nil {
		t.Error("tenant B transferred tenant A's enrollment (RLS write breach)")
	}
	if n := countTransfers(t, tenantA, enrA); n != 1 {
		t.Errorf("tenant B's write leaked a transfer row into tenant A: now %d (want 1)", n)
	}
}
