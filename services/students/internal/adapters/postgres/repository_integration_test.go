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
	dbName        = "garcia_lorca"
	tenantA       = "11111111-1111-1111-1111-111111111111"
	tenantB       = "22222222-2222-2222-2222-222222222222"
	migrationFile = "../../../../../migrations/students/0001_init.sql"
)

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
	sql, err := os.ReadFile(migrationFile)
	if err != nil {
		return err
	}
	if err := db.Exec(string(sql)).Error; err != nil {
		return err
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
