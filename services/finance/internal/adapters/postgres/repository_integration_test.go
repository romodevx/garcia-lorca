//go:build integration

// Integration tests for the finance postgres adapter against a REAL Postgres
// (testcontainers-go). They prove the ledger math (paid totals, remaining
// balance, overdue selection), that RegisterPayment is atomic — a payment the
// derive rule vetoes writes NOTHING — that bulk charge generation is idempotent,
// and, critically, that RLS isolates tenants for both reads AND writes when
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
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/pubsub"
	pgadapter "github.com/garcia-lorca/services/finance/internal/adapters/postgres"
	financepubsub "github.com/garcia-lorca/services/finance/internal/adapters/pubsub"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
	"google.golang.org/protobuf/proto"
)

const (
	dbName  = "garcia_lorca"
	tenantA = "11111111-1111-1111-1111-111111111111"
	tenantB = "22222222-2222-2222-2222-222222222222"
)

// migrationDir holds the migrations applied, in filename order, before the app
// role is granted — so the non-owner role gets privileges on every table.
//
// Globbed rather than listed. The list used to be written out by hand, and a
// migration added without remembering to append it here failed every test that
// touched the new column, in a way that read as a code bug rather than a
// forgotten line.
const migrationDir = "../../../../../migrations/finance"

func migrationFiles() ([]string, error) {
	files, err := filepath.Glob(filepath.Join(migrationDir, "*.sql"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no migrations found in %s", migrationDir)
	}
	// Glob sorts already; sorting again says the order is load-bearing rather
	// than incidental.
	slices.Sort(files)
	return files, nil
}

var (
	superDB   *gorm.DB
	appDSN    string
	superDSN  string
	superPass = randToken()
	appPass   = randToken()
)

// Neutral cross-service references (students/config UUIDs). The container is
// shared and never reset between tests, so each test owns its own concept,
// student and — where the cycle is a filter — its own cycle, and no test can
// pre-populate a row another one counts on.
const (
	cycleOne     = "70000000-0000-0000-0000-000000000001"
	cycleBulk    = "70000000-0000-0000-0000-000000000002"
	cycleOverdue = "70000000-0000-0000-0000-000000000003"
	cycleOrder   = "70000000-0000-0000-0000-000000000004"
	cycleExtRef  = "70000000-0000-0000-0000-000000000005"
)

// today is the fixed clock the overdue query is evaluated against; the domain
// never reads a clock, so the tests never do either.
var today = time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)

func yesterday() time.Time { return today.AddDate(0, 0, -1) }
func tomorrow() time.Time  { return today.AddDate(0, 0, 1) }

// paidAt is a fixed reception instant; individual payments offset from it so
// their order is deterministic.
var paidAt = time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

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
	superDSN = fmt.Sprintf("host=%s port=%s user=postgres password=%s dbname=%s sslmode=disable", host, port.Port(), superPass, dbName)
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
	files, err := migrationFiles()
	if err != nil {
		return err
	}
	for _, f := range files {
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
	// Closed with the test. Every call opens a pool, and the suite used to keep
	// them all until the process ended — one test more ran the container out of
	// connection slots ("remaining connection slots are reserved").
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("app db handle: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close app db: %v", err)
		}
	})
	return pgadapter.NewRepository(db, tenant)
}

// --- builders ---------------------------------------------------------------

func newConcept(t *testing.T, id, tenant, name string, amountCents int64) domain.ChargeConcept {
	t.Helper()
	c, err := domain.NewChargeConcept(id, tenant, name, amountCents, domain.RecurrenceOnce, true)
	if err != nil {
		t.Fatalf("new charge concept: %v", err)
	}
	return c
}

func newCharge(t *testing.T, id, tenant, student, cycle, conceptID string, amount, discount int64, due time.Time) domain.Charge {
	t.Helper()
	c, err := domain.NewCharge(id, tenant, student, cycle, conceptID, "colegiatura", amount, discount, due)
	if err != nil {
		t.Fatalf("new charge: %v", err)
	}
	return c
}

// one is a payment of a single method, as RegisterPayment takes it (T8).
func one(p domain.Payment) []domain.Payment { return []domain.Payment{p} }

// newPayment builds a payment that opts out of deduplication — the historical
// behaviour, and what every test that is not about idempotency wants.
func newPayment(t *testing.T, id, tenant, chargeID string, amount int64, at time.Time) domain.Payment {
	t.Helper()
	return newKeyedPayment(t, id, tenant, chargeID, amount, at, "")
}

// newKeyedPayment builds a payment carrying an idempotency key: two attempts
// under the same key are the same payment retried.
func newKeyedPayment(t *testing.T, id, tenant, chargeID string, amount int64, at time.Time, key string) domain.Payment {
	t.Helper()
	p, err := domain.NewPayment(id, tenant, chargeID, amount, domain.MethodCash, "", at, key)
	if err != nil {
		t.Fatalf("new payment: %v", err)
	}
	// Dated on the UTC date of at, like newDeposit.
	return p.WithPaidOn(at)
}

// derive mirrors the use case's rule (usecase.derivePaidStatus): a cancelled
// charge and an overpay are vetoed inside the payment transaction, so the
// adapter's atomicity is exercised exactly as production does it.
func derive(charge domain.Charge, newPaidCents int64) (string, error) {
	if charge.Status() == domain.StatusCancelled {
		return "", glerrors.New(glerrors.KindInvalid, "cannot pay a cancelled charge")
	}
	if newPaidCents > charge.Effective().Int64() {
		return "", glerrors.New(glerrors.KindInvalid, "payment exceeds the remaining balance")
	}
	return charge.DeriveStatus(newPaidCents), nil
}

// seedConceptAndCharge creates one concept plus one charge for a tenant, and
// returns the stored charge.
func seedConceptAndCharge(t *testing.T, tenant, conceptID, conceptName, chargeID, student, cycle string, amount, discount int64, due time.Time) domain.Charge {
	t.Helper()
	repo := appRepo(t, tenant)
	ctx := context.Background()
	if _, err := repo.CreateConcept(ctx, newConcept(t, conceptID, tenant, conceptName, amount)); err != nil {
		t.Fatalf("seed concept %s: %v", conceptName, err)
	}
	charge, err := repo.CreateCharge(ctx, newCharge(t, chargeID, tenant, student, cycle, conceptID, amount, discount, due))
	if err != nil {
		t.Fatalf("seed charge %s: %v", chargeID, err)
	}
	return charge
}

// countPayments reads `payments` as the non-owner app role with the tenant GUC
// bound (mirrors the adapter's RLS binding), so it exercises RLS: a tenant sees
// only its own rows.
func countPayments(t *testing.T, tenant, chargeID string) int64 {
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
		return tx.Table("payments").Where("charge_id = ?", chargeID).Count(&n).Error
	})
	if err != nil {
		t.Fatalf("count payments: %v", err)
	}
	return n
}

func isNotFound(err error) bool { return glerrors.KindOf(err) == glerrors.KindNotFound }

// --- RLS --------------------------------------------------------------------

// TestRLSIsolatesConcepts proves read AND write isolation on `charge_concepts`
// for the non-owner app role: tenant B cannot list, read or update tenant A's
// catalog entry.
func TestRLSIsolatesConcepts(t *testing.T) {
	ctx := context.Background()
	const conceptID = "c0000000-0000-0000-0000-0000000000a1"
	if _, err := appRepo(t, tenantA).CreateConcept(ctx, newConcept(t, conceptID, tenantA, "rls-concept", 10000)); err != nil {
		t.Fatalf("seed A: %v", err)
	}

	// Read isolation: the entry is absent from tenant B's catalog and unreadable
	// by id.
	listB, err := appRepo(t, tenantB).ListConcepts(ctx, true)
	if err != nil {
		t.Fatalf("tenant B ListConcepts: %v", err)
	}
	for _, c := range listB {
		if c.ID() == conceptID {
			t.Fatal("tenant B saw tenant A's concept (RLS read breach)")
		}
	}
	if _, err := appRepo(t, tenantB).GetConcept(ctx, conceptID); !isNotFound(err) {
		t.Errorf("tenant B GetConcept: want NotFound, got %v", err)
	}

	// Write isolation: an update aimed at tenant A's row touches nothing.
	hijack := newConcept(t, conceptID, tenantB, "hijacked", 1)
	if _, err := appRepo(t, tenantB).UpdateConcept(ctx, hijack); !isNotFound(err) {
		t.Errorf("tenant B UpdateConcept: want NotFound, got %v", err)
	}
	after, err := appRepo(t, tenantA).GetConcept(ctx, conceptID)
	if err != nil {
		t.Fatalf("tenant A GetConcept: %v", err)
	}
	if after.Name() != "rls-concept" || after.DefaultAmount().Int64() != 10000 {
		t.Errorf("tenant B's write leaked into tenant A: %+v", after)
	}
}

// TestRLSIsolatesCharges proves read AND write isolation on `charges`: tenant B
// can neither read nor mutate a charge issued by tenant A.
func TestRLSIsolatesCharges(t *testing.T) {
	ctx := context.Background()
	const (
		conceptID = "c0000000-0000-0000-0000-0000000000a2"
		chargeID  = "d0000000-0000-0000-0000-0000000000a2"
		student   = "50000000-0000-0000-0000-0000000000a2"
	)
	seedConceptAndCharge(t, tenantA, conceptID, "rls-charge", chargeID, student, cycleOne, 10000, 0, today)

	// Read isolation.
	if _, err := appRepo(t, tenantB).GetCharge(ctx, chargeID); !isNotFound(err) {
		t.Errorf("tenant B GetCharge: want NotFound, got %v", err)
	}
	listB, err := appRepo(t, tenantB).ListCharges(ctx, student, "")
	if err != nil {
		t.Fatalf("tenant B ListCharges: %v", err)
	}
	if len(listB) != 0 {
		t.Errorf("tenant B saw %d of tenant A's charges (RLS read breach)", len(listB))
	}

	// Write isolation: cancelling tenant A's charge from tenant B touches nothing.
	hijack, err := newCharge(t, chargeID, tenantB, student, cycleOne, conceptID, 10000, 0, today).Cancel()
	if err != nil {
		t.Fatalf("build cancelled charge: %v", err)
	}
	if _, err := appRepo(t, tenantB).SaveCharge(ctx, hijack); !isNotFound(err) {
		t.Errorf("tenant B SaveCharge: want NotFound, got %v", err)
	}
	afterA, err := appRepo(t, tenantA).GetCharge(ctx, chargeID)
	if err != nil {
		t.Fatalf("tenant A GetCharge: %v", err)
	}
	if afterA.Charge.Status() != domain.StatusPending {
		t.Errorf("tenant B's write leaked into tenant A: status = %q, want pending", afterA.Charge.Status())
	}
}

// TestRLSIsolatesPayments proves a payment collected by tenant A is invisible to
// tenant B, both by charge and through the student's statement.
func TestRLSIsolatesPayments(t *testing.T) {
	ctx := context.Background()
	const (
		conceptID = "c0000000-0000-0000-0000-0000000000a3"
		chargeID  = "d0000000-0000-0000-0000-0000000000a3"
		paymentID = "f0000000-0000-0000-0000-0000000000a3"
		student   = "50000000-0000-0000-0000-0000000000a3"
	)
	seedConceptAndCharge(t, tenantA, conceptID, "rls-payment", chargeID, student, cycleOne, 10000, 0, today)
	if _, _, _, err := appRepo(t, tenantA).RegisterPayment(ctx, one(newPayment(t, paymentID, tenantA, chargeID, 5000, paidAt)), derive, nil); err != nil {
		t.Fatalf("seed payment: %v", err)
	}

	paymentsB, err := appRepo(t, tenantB).ListPayments(ctx, student)
	if err != nil {
		t.Fatalf("tenant B ListPayments: %v", err)
	}
	if len(paymentsB) != 0 {
		t.Errorf("tenant B saw %d of tenant A's payments (RLS read breach)", len(paymentsB))
	}
	if n := countPayments(t, tenantB, chargeID); n != 0 {
		t.Errorf("tenant B payment rows = %d, want 0", n)
	}

	paymentsA, err := appRepo(t, tenantA).ListPayments(ctx, student)
	if err != nil {
		t.Fatalf("tenant A ListPayments: %v", err)
	}
	if len(paymentsA) != 1 || paymentsA[0].Amount().Int64() != 5000 {
		t.Errorf("tenant A should see its own payment, got %+v", paymentsA)
	}
}

// --- the atomic payment knot ------------------------------------------------

// TestRegisterPaymentIsAtomicAndDerivesStatus walks a charge from pending
// through partially_paid to paid, and then proves a vetoed payment (overpay)
// rolls the whole transaction back: no payment row, unchanged status.
func TestRegisterPaymentIsAtomicAndDerivesStatus(t *testing.T) {
	ctx := context.Background()
	const (
		conceptID = "c0000000-0000-0000-0000-0000000000b1"
		chargeID  = "d0000000-0000-0000-0000-0000000000b1"
		student   = "50000000-0000-0000-0000-0000000000b1"
	)
	seedConceptAndCharge(t, tenantA, conceptID, "atomic-payment", chargeID, student, cycleOne, 10000, 0, today)
	repo := appRepo(t, tenantA)

	_, first, _, err := repo.RegisterPayment(ctx, one(newPayment(t, "f0000000-0000-0000-0000-0000000000b1", tenantA, chargeID, 4000, paidAt)), derive, nil)
	if err != nil {
		t.Fatalf("first payment: %v", err)
	}
	if first.PaidCents != 4000 || first.Charge.Status() != domain.StatusPartiallyPaid {
		t.Errorf("after 4000: paid = %d, status = %q; want 4000 / partially_paid", first.PaidCents, first.Charge.Status())
	}

	_, second, _, err := repo.RegisterPayment(ctx, one(newPayment(t, "f0000000-0000-0000-0000-0000000000b2", tenantA, chargeID, 6000, paidAt.Add(time.Hour))), derive, nil)
	if err != nil {
		t.Fatalf("second payment: %v", err)
	}
	if second.PaidCents != 10000 || second.Charge.Status() != domain.StatusPaid {
		t.Errorf("after 6000: paid = %d, status = %q; want 10000 / paid", second.PaidCents, second.Charge.Status())
	}

	// One cent more than owed: derive vetoes it and the transaction rolls back.
	if _, _, _, err := repo.RegisterPayment(ctx, one(newPayment(t, "f0000000-0000-0000-0000-0000000000b3", tenantA, chargeID, 1, paidAt.Add(2*time.Hour))), derive, nil); err == nil {
		t.Fatal("overpay was accepted; derive must veto it")
	}
	if n := countPayments(t, tenantA, chargeID); n != 2 {
		t.Errorf("payment rows = %d, want 2 (the vetoed payment must not be written)", n)
	}
	after, err := repo.GetCharge(ctx, chargeID)
	if err != nil {
		t.Fatalf("GetCharge: %v", err)
	}
	if after.PaidCents != 10000 || after.Charge.Status() != domain.StatusPaid {
		t.Errorf("after veto: paid = %d, status = %q; want 10000 / paid", after.PaidCents, after.Charge.Status())
	}
}

// CC-203: a charge withdrawn from the screen keeps its reason on the row.
func TestSaveChargePersistsCancellationReason(t *testing.T) {
	ctx := context.Background()
	const (
		conceptID = "c0000000-0000-0000-0000-00000000c203"
		chargeID  = "d0000000-0000-0000-0000-00000000c203"
		student   = "50000000-0000-0000-0000-00000000c203"
	)
	charge := seedConceptAndCharge(t, tenantA, conceptID, "cancel-reason", chargeID, student, cycleOne, 10000, 0, today)
	repo := appRepo(t, tenantA)

	cancelled, err := charge.Cancel()
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := repo.SaveCharge(ctx, cancelled.WithCancellationReason("Cargo duplicado")); err != nil {
		t.Fatalf("SaveCharge: %v", err)
	}

	after, err := repo.GetCharge(ctx, chargeID)
	if err != nil {
		t.Fatalf("GetCharge: %v", err)
	}
	if got := after.Charge.CancellationReason(); got != "Cargo duplicado" {
		t.Errorf("stored reason = %q, want %q", got, "Cargo duplicado")
	}
}

// TestRegisterPaymentRejectsCancelled proves a cancelled charge — a status the
// mapper reconstructs from the stored row — is vetoed under the row lock, and
// that nothing is written.
func TestRegisterPaymentRejectsCancelled(t *testing.T) {
	ctx := context.Background()
	const (
		conceptID = "c0000000-0000-0000-0000-0000000000b4"
		chargeID  = "d0000000-0000-0000-0000-0000000000b4"
		student   = "50000000-0000-0000-0000-0000000000b4"
	)
	charge := seedConceptAndCharge(t, tenantA, conceptID, "cancelled-payment", chargeID, student, cycleOne, 10000, 0, today)
	repo := appRepo(t, tenantA)

	cancelled, err := charge.Cancel()
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	saved, err := repo.SaveCharge(ctx, cancelled)
	if err != nil {
		t.Fatalf("SaveCharge: %v", err)
	}
	if saved.Status() != domain.StatusCancelled {
		t.Fatalf("stored status = %q, want cancelled", saved.Status())
	}

	if _, _, _, err := repo.RegisterPayment(ctx, one(newPayment(t, "f0000000-0000-0000-0000-0000000000b4", tenantA, chargeID, 1000, paidAt)), derive, nil); err == nil {
		t.Fatal("payment against a cancelled charge was accepted")
	}
	if n := countPayments(t, tenantA, chargeID); n != 0 {
		t.Errorf("payment rows = %d, want 0", n)
	}
	after, err := repo.GetCharge(ctx, chargeID)
	if err != nil {
		t.Fatalf("GetCharge: %v", err)
	}
	if after.Charge.Status() != domain.StatusCancelled || after.PaidCents != 0 {
		t.Errorf("after veto: status = %q, paid = %d; want cancelled / 0", after.Charge.Status(), after.PaidCents)
	}
}

// TestRegisterPaymentIsIdempotent proves the retry story end to end against a
// real Postgres: a second attempt under the same idempotency key returns the
// ORIGINAL payment and writes nothing — no second row, no extra cents — while a
// fresh key (and no key at all) still collects normally.
func TestRegisterPaymentIsIdempotent(t *testing.T) {
	ctx := context.Background()
	const (
		conceptID = "c0000000-0000-0000-0000-0000000000b5"
		chargeID  = "d0000000-0000-0000-0000-0000000000b5"
		student   = "50000000-0000-0000-0000-0000000000b5"
		firstID   = "f0000000-0000-0000-0000-0000000000b5"
		retryID   = "f0000000-0000-0000-0000-0000000000b6"
		otherID   = "f0000000-0000-0000-0000-0000000000b7"
		keylessID = "f0000000-0000-0000-0000-0000000000b8"
	)
	seedConceptAndCharge(t, tenantA, conceptID, "idempotent-payment", chargeID, student, cycleOne, 10000, 0, today)
	repo := appRepo(t, tenantA)

	stored, first, replayed, err := repo.RegisterPayment(ctx,
		one(newKeyedPayment(t, firstID, tenantA, chargeID, 4000, paidAt, "attempt-1")), derive, nil)
	if err != nil {
		t.Fatalf("first payment: %v", err)
	}
	if replayed {
		t.Fatal("the first attempt reported a replay")
	}
	if first.PaidCents != 4000 || first.Charge.Status() != domain.StatusPartiallyPaid {
		t.Fatalf("after 4000: paid = %d, status = %q; want 4000 / partially_paid", first.PaidCents, first.Charge.Status())
	}

	// The same attempt again — a retry after a lost response. It must return the
	// payment already stored, NOT a new one.
	again, second, replayed, err := repo.RegisterPayment(ctx,
		one(newKeyedPayment(t, retryID, tenantA, chargeID, 4000, paidAt.Add(time.Minute), "attempt-1")), derive, nil)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !replayed {
		t.Error("retry did not report a replay")
	}
	if len(again) != 1 || again[0].ID() != stored[0].ID() {
		t.Errorf("retry returned %+v, want the original %q", again, stored[0].ID())
	}
	if second.PaidCents != 4000 || second.Charge.Status() != domain.StatusPartiallyPaid {
		t.Errorf("after retry: paid = %d, status = %q; want the charge untouched at 4000 / partially_paid",
			second.PaidCents, second.Charge.Status())
	}
	if n := countPayments(t, tenantA, chargeID); n != 1 {
		t.Fatalf("payment rows = %d, want 1 (the retry must not double-charge)", n)
	}

	// A different key is a different attempt, and an absent key opts out of
	// deduplication entirely — both still collect.
	if _, _, replayed, err = repo.RegisterPayment(ctx,
		one(newKeyedPayment(t, otherID, tenantA, chargeID, 3000, paidAt.Add(time.Hour), "attempt-2")), derive, nil); err != nil {
		t.Fatalf("second attempt: %v", err)
	}
	if replayed {
		t.Error("a distinct key was treated as a replay")
	}
	_, third, _, err := repo.RegisterPayment(ctx,
		one(newPayment(t, keylessID, tenantA, chargeID, 3000, paidAt.Add(2*time.Hour))), derive, nil)
	if err != nil {
		t.Fatalf("keyless payment: %v", err)
	}
	if third.PaidCents != 10000 || third.Charge.Status() != domain.StatusPaid {
		t.Errorf("after 3 collections: paid = %d, status = %q; want 10000 / paid", third.PaidCents, third.Charge.Status())
	}
	if n := countPayments(t, tenantA, chargeID); n != 3 {
		t.Errorf("payment rows = %d, want 3", n)
	}
}

// --- bulk generation & ledger math ------------------------------------------

// TestCreateChargesBulkIdempotent proves re-running a generation writes nothing:
// the (tenant, student, cycle, concept) unique key makes every row a no-op the
// second time, and the reported count is the rows actually inserted.
func TestCreateChargesBulkIdempotent(t *testing.T) {
	ctx := context.Background()
	const conceptID = "c0000000-0000-0000-0000-0000000000c1"
	repo := appRepo(t, tenantA)
	if _, err := repo.CreateConcept(ctx, newConcept(t, conceptID, tenantA, "bulk-concept", 8000)); err != nil {
		t.Fatalf("seed concept: %v", err)
	}

	charges := []domain.Charge{
		newCharge(t, "d0000000-0000-0000-0000-0000000000c1", tenantA, "50000000-0000-0000-0000-0000000000c1", cycleBulk, conceptID, 8000, 0, today),
		newCharge(t, "d0000000-0000-0000-0000-0000000000c2", tenantA, "50000000-0000-0000-0000-0000000000c2", cycleBulk, conceptID, 8000, 0, today),
		newCharge(t, "d0000000-0000-0000-0000-0000000000c3", tenantA, "50000000-0000-0000-0000-0000000000c3", cycleBulk, conceptID, 8000, 0, today),
	}

	created, err := repo.CreateChargesBulk(ctx, charges)
	if err != nil {
		t.Fatalf("first bulk: %v", err)
	}
	if created != 3 {
		t.Errorf("first bulk created = %d, want 3", created)
	}

	again, err := repo.CreateChargesBulk(ctx, charges)
	if err != nil {
		t.Fatalf("second bulk: %v", err)
	}
	if again != 0 {
		t.Errorf("second bulk created = %d, want 0 (re-running must be idempotent)", again)
	}
}

// TestCreateChargesBulkIsIdempotentForREFERENCEDCharges is the case the older
// test above does not reach, and the one the monthly billing run depends on
// entirely.
//
// A charge with no external ref is deduplicated by (student, cycle, concept). A
// charge WITH one is deduplicated by the ref instead — that is the whole point
// of migration 0003, because a school year has ten tuition charges for the same
// concept and the keyless index forbids the second.
//
// The bulk insert names its arbiter explicitly, so it named ONE of the two
// partial indexes. Referenced charges therefore collided against the index that
// was not the arbiter, and Postgres raised a duplicate-key error instead of
// skipping: the second billing run of any month returned a 500 and generated
// nothing. Found by running the thing twice against a real database.
func TestCreateChargesBulkIsIdempotentForReferencedCharges(t *testing.T) {
	ctx := context.Background()
	const conceptID = "c0000000-0000-0000-0000-0000000000f7"
	repo := appRepo(t, tenantA)
	if _, err := repo.CreateConcept(ctx, newConcept(t, conceptID, tenantA, "bulk-referenced", 8000)); err != nil {
		t.Fatalf("seed concept: %v", err)
	}

	// Two charges for the SAME student, cycle and concept — September's tuition
	// and October's. Only the ref tells them apart, which is exactly the shape
	// the billing run produces.
	student := "50000000-0000-0000-0000-0000000000f7"
	// The ids differ between attempts, exactly as they do in life: the billing
	// run mints a fresh uuid every time it builds a charge, so the ONLY thing
	// telling a re-run from a first run is the external ref.
	build := func(idSuffix string) []domain.Charge {
		var out []domain.Charge
		for i, month := range []string{"2026-09", "2026-10"} {
			c := newCharge(t, fmt.Sprintf("d0000000-0000-0000-0000-00000000%s%d", idSuffix, i+1),
				tenantA, student, cycleBulk, conceptID, 8000, 0, today)
			withRef, err := c.WithExternalRef("tuition:" + cycleBulk + ":" + student + ":" + month)
			if err != nil {
				t.Fatalf("external ref: %v", err)
			}
			out = append(out, withRef)
		}
		return out
	}

	created, err := repo.CreateChargesBulk(ctx, build("f7a"))
	if err != nil {
		t.Fatalf("first bulk: %v", err)
	}
	if created != 2 {
		t.Fatalf("first bulk created = %d, want 2 — two months of the same concept", created)
	}

	again, err := repo.CreateChargesBulk(ctx, build("f7b"))
	if err != nil {
		t.Fatalf("second bulk must skip, not fail: %v", err)
	}
	if again != 0 {
		t.Errorf("second bulk created = %d, want 0", again)
	}
}

// TestCreateChargesBulkWithASingleReferencedCharge is the case the two-row test
// above does not reach, and the one the panel actually hits.
//
// GORM renders a one-row insert differently from a batch, and the ON CONFLICT
// predicate came out as a bind parameter rather than a literal. Postgres cannot
// prove a parameterised predicate implies a partial index's, so it answered
// "there is no unique or exclusion constraint matching the ON CONFLICT
// specification" — and every billing run of a school with ONE pupil failed,
// which is exactly the shape of a demo.
func TestCreateChargesBulkWithASingleReferencedCharge(t *testing.T) {
	ctx := context.Background()
	const conceptID = "c0000000-0000-0000-0000-0000000000f8"
	repo := appRepo(t, tenantA)
	if _, err := repo.CreateConcept(ctx, newConcept(t, conceptID, tenantA, "single-referenced", 8000)); err != nil {
		t.Fatalf("seed concept: %v", err)
	}
	student := "50000000-0000-0000-0000-0000000000f8"
	one := func(id string) []domain.Charge {
		c := newCharge(t, id, tenantA, student, cycleBulk, conceptID, 8000, 0, today)
		withRef, err := c.WithExternalRef("enrollment:" + cycleBulk + ":" + student)
		if err != nil {
			t.Fatalf("external ref: %v", err)
		}
		return []domain.Charge{withRef}
	}

	created, err := repo.CreateChargesBulk(ctx, one("d0000000-0000-0000-0000-0000000000f8"))
	if err != nil {
		t.Fatalf("single referenced charge: %v", err)
	}
	if created != 1 {
		t.Fatalf("created = %d, want 1", created)
	}

	again, err := repo.CreateChargesBulk(ctx, one("d0000000-0000-0000-0000-0000000000f9"))
	if err != nil {
		t.Fatalf("re-running must skip, not fail: %v", err)
	}
	if again != 0 {
		t.Errorf("created = %d, want 0", again)
	}
}

// TestGetAccountBalanceMath proves the statement's arithmetic: each charge comes
// back with its own collected total (resolved in one grouped query), and what is
// still owed is Effective() - PaidCents.
func TestGetAccountBalanceMath(t *testing.T) {
	ctx := context.Background()
	const (
		conceptOne = "c0000000-0000-0000-0000-0000000000d1"
		conceptTwo = "c0000000-0000-0000-0000-0000000000d2"
		chargeOne  = "d0000000-0000-0000-0000-0000000000d1"
		chargeTwo  = "d0000000-0000-0000-0000-0000000000d2"
		student    = "50000000-0000-0000-0000-0000000000d1"
	)
	seedConceptAndCharge(t, tenantA, conceptOne, "balance-tuition", chargeOne, student, cycleOne, 10000, 0, yesterday())
	seedConceptAndCharge(t, tenantA, conceptTwo, "balance-uniform", chargeTwo, student, cycleOne, 5000, 0, tomorrow())

	repo := appRepo(t, tenantA)
	if _, _, _, err := repo.RegisterPayment(ctx, one(newPayment(t, "f0000000-0000-0000-0000-0000000000d1", tenantA, chargeOne, 3000, paidAt)), derive, nil); err != nil {
		t.Fatalf("partial payment: %v", err)
	}

	list, err := repo.ListCharges(ctx, student, "")
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListCharges returned %d charges, want 2", len(list))
	}
	// Ordered by due_date ASC: the partially paid tuition first.
	if list[0].Charge.ID() != chargeOne || list[1].Charge.ID() != chargeTwo {
		t.Fatalf("charges out of due-date order: %q then %q", list[0].Charge.ID(), list[1].Charge.ID())
	}
	if list[0].PaidCents != 3000 || list[1].PaidCents != 0 {
		t.Errorf("paid totals = %d / %d, want 3000 / 0", list[0].PaidCents, list[1].PaidCents)
	}
	if remaining := list[0].Charge.Effective().Int64() - list[0].PaidCents; remaining != 7000 {
		t.Errorf("tuition remaining = %d, want 7000", remaining)
	}
	if remaining := list[1].Charge.Effective().Int64() - list[1].PaidCents; remaining != 5000 {
		t.Errorf("uniform remaining = %d, want 5000", remaining)
	}
	if list[0].Charge.Status() != domain.StatusPartiallyPaid {
		t.Errorf("tuition status = %q, want partially_paid", list[0].Charge.Status())
	}
}

// TestListOverdueSelectsPastDueWithBalance proves the overdue query keeps only
// what is genuinely owed: past its due date, still carrying a balance, and not
// cancelled.
func TestListOverdueSelectsPastDueWithBalance(t *testing.T) {
	ctx := context.Background()
	const (
		conceptID   = "c0000000-0000-0000-0000-0000000000e1"
		overdueID   = "d0000000-0000-0000-0000-0000000000e1"
		futureID    = "d0000000-0000-0000-0000-0000000000e2"
		settledID   = "d0000000-0000-0000-0000-0000000000e3"
		cancelledID = "d0000000-0000-0000-0000-0000000000e4"
	)
	repo := appRepo(t, tenantA)
	if _, err := repo.CreateConcept(ctx, newConcept(t, conceptID, tenantA, "overdue-concept", 1000)); err != nil {
		t.Fatalf("seed concept: %v", err)
	}

	seed := func(id, student string, due time.Time) {
		t.Helper()
		if _, err := repo.CreateCharge(ctx, newCharge(t, id, tenantA, student, cycleOverdue, conceptID, 1000, 0, due)); err != nil {
			t.Fatalf("seed charge %s: %v", id, err)
		}
	}
	seed(overdueID, "50000000-0000-0000-0000-0000000000e1", yesterday())
	seed(futureID, "50000000-0000-0000-0000-0000000000e2", tomorrow())
	seed(settledID, "50000000-0000-0000-0000-0000000000e3", yesterday())
	seed(cancelledID, "50000000-0000-0000-0000-0000000000e4", yesterday())

	// Past due but fully collected: no balance, so not overdue.
	if _, _, _, err := repo.RegisterPayment(ctx, one(newPayment(t, "f0000000-0000-0000-0000-0000000000e3", tenantA, settledID, 1000, paidAt)), derive, nil); err != nil {
		t.Fatalf("settle charge: %v", err)
	}
	// Past due but withdrawn by the school: cancelled is never overdue.
	stored, err := repo.GetCharge(ctx, cancelledID)
	if err != nil {
		t.Fatalf("GetCharge(cancelled): %v", err)
	}
	cancelled, err := stored.Charge.Cancel()
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := repo.SaveCharge(ctx, cancelled); err != nil {
		t.Fatalf("SaveCharge(cancelled): %v", err)
	}

	page, err := repo.ListOverduePage(ctx, cycleOverdue, today, ports.ChargeCursor{}, 0)
	if err != nil {
		t.Fatalf("ListOverduePage: %v", err)
	}
	if !page.Next.IsZero() {
		t.Errorf("Next = %+v, want zero: four candidates fit one page", page.Next)
	}
	overdue := page.Charges
	if len(overdue) != 1 {
		ids := make([]string, 0, len(overdue))
		for _, c := range overdue {
			ids = append(ids, c.Charge.ID())
		}
		t.Fatalf("ListOverdue returned %v, want exactly [%s]", ids, overdueID)
	}
	if overdue[0].Charge.ID() != overdueID {
		t.Errorf("ListOverdue returned %q, want %q", overdue[0].Charge.ID(), overdueID)
	}
	if overdue[0].PaidCents != 0 {
		t.Errorf("overdue paid = %d, want 0", overdue[0].PaidCents)
	}
}

// TestListChargesIsEarliestDueFirst proves the ledger read has a DETERMINISTIC
// order — earliest due date first — rather than whatever the planner emits. That
// order is what makes the read's documented cap safe to apply: the rows a capped
// read drops are always the ones furthest from being collectable.
func TestListChargesIsEarliestDueFirst(t *testing.T) {
	ctx := context.Background()
	const (
		conceptID = "c0000000-0000-0000-0000-0000000000f1"
		lastID    = "d0000000-0000-0000-0000-0000000000f1"
		firstID   = "d0000000-0000-0000-0000-0000000000f2"
		middleID  = "d0000000-0000-0000-0000-0000000000f3"
	)
	repo := appRepo(t, tenantA)
	if _, err := repo.CreateConcept(ctx, newConcept(t, conceptID, tenantA, "ordered-concept", 1000)); err != nil {
		t.Fatalf("seed concept: %v", err)
	}

	// Seeded out of due-date order, one student each (a student owes a concept
	// once per cycle), so insertion order cannot masquerade as the ordering.
	seeds := []struct {
		id, student string
		due         time.Time
	}{
		{lastID, "50000000-0000-0000-0000-0000000000f1", today.AddDate(0, 0, 30)},
		{firstID, "50000000-0000-0000-0000-0000000000f2", today.AddDate(0, 0, -30)},
		{middleID, "50000000-0000-0000-0000-0000000000f3", today},
	}
	for _, s := range seeds {
		if _, err := repo.CreateCharge(ctx, newCharge(t, s.id, tenantA, s.student, cycleOrder, conceptID, 1000, 0, s.due)); err != nil {
			t.Fatalf("seed charge %s: %v", s.id, err)
		}
	}

	list, err := repo.ListCharges(ctx, "", cycleOrder)
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}

	want := []string{firstID, middleID, lastID}
	got := make([]string, 0, len(list))
	for _, c := range list {
		got = append(got, c.Charge.ID())
	}
	if !slices.Equal(got, want) {
		t.Fatalf("ListCharges order = %v, want %v (earliest due first)", got, want)
	}
}

// --- external references (M12 uniforms) --------------------------------------

// TestChargeExternalRefAllowsRepeatConcept is the test that would have caught the
// blocker: `charges` carries UNIQUE (tenant, student, cycle, concept) so a
// student could only ever be charged a concept ONCE per cycle. A uniforms order
// needs several — one per delivery, and one per purchase across the school year.
// Migration 0003 makes that key PARTIAL (charges with no external ref only), so
// referenced charges are free to repeat while tuition keeps its old rule.
func TestChargeExternalRefAllowsRepeatConcept(t *testing.T) {
	ctx := context.Background()
	const (
		conceptID = "c0000000-0000-0000-0000-00000000ab01"
		studentID = "50000000-0000-0000-0000-00000000ab01"
	)
	repo := appRepo(t, tenantA)
	if _, err := repo.CreateConcept(ctx, newConcept(t, conceptID, tenantA, "uniformes", 145000)); err != nil {
		t.Fatalf("seed concept: %v", err)
	}

	first := withExternalRef(t,
		newCharge(t, "d0000000-0000-0000-0000-00000000ab01", tenantA, studentID, cycleExtRef, conceptID, 89000, 0, today),
		"uniforms:ful-a")
	if _, err := repo.CreateCharge(ctx, first); err != nil {
		t.Fatalf("first referenced charge: %v", err)
	}

	// Same student, same cycle, same concept — and it must go through, because
	// this is delivery -B of the same order.
	second := withExternalRef(t,
		newCharge(t, "d0000000-0000-0000-0000-00000000ab02", tenantA, studentID, cycleExtRef, conceptID, 65000, 0, today),
		"uniforms:ful-b")
	saved, err := repo.CreateCharge(ctx, second)
	if err != nil {
		t.Fatalf("second referenced charge must be allowed: %v", err)
	}
	if saved.ExternalRef() != "uniforms:ful-b" {
		t.Errorf("external ref not persisted, got %q", saved.ExternalRef())
	}
}

// TestChargeExternalRefIsUnique proves the reference itself is deduplicated: a
// retried "create the charge for this fulfillment" must not bill the family
// twice. Same guarantee RegisterPayment's idempotency key gives, same partial
// index shape (migration 0002).
func TestChargeExternalRefIsUnique(t *testing.T) {
	ctx := context.Background()
	const (
		conceptID = "c0000000-0000-0000-0000-00000000ab02"
		studentID = "50000000-0000-0000-0000-00000000ab02"
	)
	repo := appRepo(t, tenantA)
	if _, err := repo.CreateConcept(ctx, newConcept(t, conceptID, tenantA, "uniformes-dup", 145000)); err != nil {
		t.Fatalf("seed concept: %v", err)
	}

	one := withExternalRef(t,
		newCharge(t, "d0000000-0000-0000-0000-00000000ab03", tenantA, studentID, cycleExtRef, conceptID, 89000, 0, today),
		"uniforms:ful-dup")
	if _, err := repo.CreateCharge(ctx, one); err != nil {
		t.Fatalf("first: %v", err)
	}

	// A different charge id, but the same origin row: the retry of a call whose
	// response was lost.
	retry := withExternalRef(t,
		newCharge(t, "d0000000-0000-0000-0000-00000000ab04", tenantA, "50000000-0000-0000-0000-00000000ab09", cycleExtRef, conceptID, 89000, 0, today),
		"uniforms:ful-dup")
	if _, err := repo.CreateCharge(ctx, retry); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("a repeated external ref must conflict, got %v", err)
	}
}

// TestChargeKeylessStillOnePerCycle is the regression guard for the dangerous
// half of migration 0003. CreateChargesBulk relies on ON CONFLICT DO NOTHING to
// make re-running a generation a no-op; if the unique key stopped covering
// keyless charges, GenerateCharges would silently re-bill the whole school.
func TestChargeKeylessStillOnePerCycle(t *testing.T) {
	ctx := context.Background()
	const (
		conceptID = "c0000000-0000-0000-0000-00000000ab03"
		studentID = "50000000-0000-0000-0000-00000000ab03"
	)
	repo := appRepo(t, tenantA)
	if _, err := repo.CreateConcept(ctx, newConcept(t, conceptID, tenantA, "colegiatura-guard", 8000)); err != nil {
		t.Fatalf("seed concept: %v", err)
	}

	tuition := newCharge(t, "d0000000-0000-0000-0000-00000000ab05", tenantA, studentID, cycleExtRef, conceptID, 8000, 0, today)
	if _, err := repo.CreateCharge(ctx, tuition); err != nil {
		t.Fatalf("first tuition charge: %v", err)
	}

	dup := newCharge(t, "d0000000-0000-0000-0000-00000000ab06", tenantA, studentID, cycleExtRef, conceptID, 8000, 0, today)
	if _, err := repo.CreateCharge(ctx, dup); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("a keyless charge must still be one per concept per cycle, got %v", err)
	}

	// And the bulk path — the one that bills the whole school — stays a no-op.
	again, err := repo.CreateChargesBulk(ctx, []domain.Charge{tuition, dup})
	if err != nil {
		t.Fatalf("bulk re-run: %v", err)
	}
	if again != 0 {
		t.Fatalf("re-running a generation created %d rows; it must create 0", again)
	}
}

// TestCancelChargeIfUnpaidRefusesCollectedMoney proves the guard the uniforms
// expiry sweep stands on, end to end: with money posted, the cancel is refused
// and the stored charge is left untouched.
func TestCancelChargeIfUnpaidRefusesCollectedMoney(t *testing.T) {
	ctx := context.Background()
	const (
		conceptID = "c0000000-0000-0000-0000-00000000ab04"
		studentID = "50000000-0000-0000-0000-00000000ab04"
		chargeID  = "d0000000-0000-0000-0000-00000000ab07"
	)
	repo := appRepo(t, tenantA)
	if _, err := repo.CreateConcept(ctx, newConcept(t, conceptID, tenantA, "uniformes-race", 145000)); err != nil {
		t.Fatalf("seed concept: %v", err)
	}
	c := withExternalRef(t,
		newCharge(t, chargeID, tenantA, studentID, cycleExtRef, conceptID, 89000, 0, today),
		"uniforms:ful-race")
	if _, err := repo.CreateCharge(ctx, c); err != nil {
		t.Fatalf("seed charge: %v", err)
	}
	if _, _, _, err := repo.RegisterPayment(ctx,
		one(newPayment(t, "e0000000-0000-0000-0000-00000000ab07", tenantA, chargeID, 89000, paidAt)), derive, nil); err != nil {
		t.Fatalf("cashier collects: %v", err)
	}

	// The sweep fires a moment later. It must back off.
	if _, err := repo.CancelChargeIfUnpaid(ctx, chargeID); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("want KindConflict cancelling a paid charge, got %v", err)
	}
	after, err := repo.GetCharge(ctx, chargeID)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if after.Charge.Status() == domain.StatusCancelled {
		t.Error("the charge was cancelled despite carrying a payment")
	}
}

// TestCancelChargeIfUnpaidCancelsCleanCharge is the other half: nothing
// collected, so the hold really did lapse and the charge is voided.
func TestCancelChargeIfUnpaidCancelsCleanCharge(t *testing.T) {
	ctx := context.Background()
	const (
		conceptID = "c0000000-0000-0000-0000-00000000ab05"
		studentID = "50000000-0000-0000-0000-00000000ab05"
		chargeID  = "d0000000-0000-0000-0000-00000000ab08"
	)
	repo := appRepo(t, tenantA)
	if _, err := repo.CreateConcept(ctx, newConcept(t, conceptID, tenantA, "uniformes-lapse", 145000)); err != nil {
		t.Fatalf("seed concept: %v", err)
	}
	c := withExternalRef(t,
		newCharge(t, chargeID, tenantA, studentID, cycleExtRef, conceptID, 89000, 0, today),
		"uniforms:ful-lapse")
	if _, err := repo.CreateCharge(ctx, c); err != nil {
		t.Fatalf("seed charge: %v", err)
	}

	cancelled, err := repo.CancelChargeIfUnpaid(ctx, chargeID)
	if err != nil {
		t.Fatalf("CancelChargeIfUnpaid: %v", err)
	}
	if cancelled.Charge.Status() != domain.StatusCancelled {
		t.Errorf("want cancelled, got %q", cancelled.Charge.Status())
	}
}

// withExternalRef tags a charge for the tests that need one.
func withExternalRef(t *testing.T, c domain.Charge, ref string) domain.Charge {
	t.Helper()
	tagged, err := c.WithExternalRef(ref)
	if err != nil {
		t.Fatalf("WithExternalRef(%q): %v", ref, err)
	}
	return tagged
}

// RescheduleUnpaid is what makes a corrected deadline reach the pupils already
// billed: the run never re-issues a charge it has written, so without this a
// school could set the inscription's date and see nothing happen to anybody.
//
// The guards are in SQL, so they are tested here and not against a fake.
func TestRescheduleUnpaidMovesOnlyWhatItShould(t *testing.T) {
	ctx := context.Background()
	const conceptID = "c0000000-0000-0000-0000-0000000000fc"
	const cycle = "a0000000-0000-0000-0000-0000000000fc"
	repo := appRepo(t, tenantA)
	if _, err := repo.CreateConcept(ctx, newConcept(t, conceptID, tenantA, "reschedule", 8000)); err != nil {
		t.Fatalf("seed concept: %v", err)
	}

	seed := func(idSuffix, student, ref string) domain.Charge {
		c := newCharge(t, "e0000000-0000-0000-0000-0000000000"+idSuffix,
			tenantA, student, cycle, conceptID, 8000, 0, today)
		withRef, err := c.WithExternalRef(ref)
		if err != nil {
			t.Fatalf("external ref: %v", err)
		}
		if _, err := repo.CreateCharge(ctx, withRef); err != nil {
			t.Fatalf("seed charge %s: %v", ref, err)
		}
		return withRef
	}

	unpaid := seed("c1", "50000000-0000-0000-0000-0000000000c1", "enrollment:"+cycle+":c1")
	paid := seed("c2", "50000000-0000-0000-0000-0000000000c2", "enrollment:"+cycle+":c2")
	sameDay := seed("c3", "50000000-0000-0000-0000-0000000000c3", "enrollment:"+cycle+":c3")
	partly := seed("c4", "50000000-0000-0000-0000-0000000000c4", "enrollment:"+cycle+":c4")
	startOnly := seed("c5", "50000000-0000-0000-0000-0000000000c5", "enrollment:"+cycle+":c5")

	// The second one is settled, so it is history. The fourth carries an abono
	// and is still owing, so it moves (T5).
	if _, _, _, err := repo.RegisterPayment(ctx, one(newPayment(t, "f0000000-0000-0000-0000-0000000000c2",
		tenantA, paid.ID(), 8000, today)), derive, nil); err != nil {
		t.Fatalf("seed payment: %v", err)
	}
	if _, _, _, err := repo.RegisterPayment(ctx, one(newPayment(t, "f0000000-0000-0000-0000-0000000000c4",
		tenantA, partly.ID(), 1000, today)), derive, nil); err != nil {
		t.Fatalf("seed abono: %v", err)
	}

	moveTo := today.AddDate(0, 2, 0)
	opensOn := today.AddDate(0, 1, 0)
	moved, err := repo.RescheduleUnpaid(ctx, []ports.ChargeReschedule{
		{ExternalRef: unpaid.ExternalRef(), StartsOn: opensOn, DueOn: moveTo},
		{ExternalRef: paid.ExternalRef(), StartsOn: opensOn, DueOn: moveTo},
		// Already on the day asked for: not a move, and must not be counted as
		// one, or the run would report four hundred moves every month.
		{ExternalRef: sameDay.ExternalRef(), DueOn: today},
		{ExternalRef: partly.ExternalRef(), StartsOn: opensOn, DueOn: moveTo},
		// Only the start differs: still a move.
		{ExternalRef: startOnly.ExternalRef(), StartsOn: today.AddDate(0, 0, -3), DueOn: today},
	})
	if err != nil {
		t.Fatalf("RescheduleUnpaid: %v", err)
	}
	if moved != 3 {
		t.Fatalf("moved = %d, want 3 — the unpaid, the part-paid and the start-only charge", moved)
	}

	for _, id := range []string{unpaid.ID(), partly.ID()} {
		got, err := repo.GetCharge(ctx, id)
		if err != nil {
			t.Fatalf("GetCharge: %v", err)
		}
		if !got.Charge.DueDate().Equal(moveTo) || !got.Charge.StartsOn().Equal(opensOn) {
			t.Errorf("%s: window %s..%s, want %s..%s", id,
				got.Charge.StartsOn().Format("2006-01-02"), got.Charge.DueDate().Format("2006-01-02"),
				opensOn.Format("2006-01-02"), moveTo.Format("2006-01-02"))
		}
	}
	settled, err := repo.GetCharge(ctx, paid.ID())
	if err != nil {
		t.Fatalf("GetCharge: %v", err)
	}
	if !settled.Charge.DueDate().Equal(today) || !settled.Charge.StartsOn().IsZero() {
		t.Errorf("a settled charge was re-dated to %s..%s",
			settled.Charge.StartsOn().Format("2006-01-02"), settled.Charge.DueDate().Format("2006-01-02"))
	}
	opened, err := repo.GetCharge(ctx, startOnly.ID())
	if err != nil {
		t.Fatalf("GetCharge: %v", err)
	}
	if !opened.Charge.StartsOn().Equal(today.AddDate(0, 0, -3)) {
		t.Errorf("start-only move: starts %s", opened.Charge.StartsOn().Format("2006-01-02"))
	}
}

// The window a charge is issued with is the window it is read back with (T5),
// and the database refuses one that opens after its deadline.
func TestAChargeKeepsItsWindow(t *testing.T) {
	ctx := context.Background()
	const conceptID = "c0000000-0000-0000-0000-0000000000fe"
	const cycle = "a0000000-0000-0000-0000-0000000000fe"
	repo := appRepo(t, tenantA)
	if _, err := repo.CreateConcept(ctx, newConcept(t, conceptID, tenantA, "window", 8000)); err != nil {
		t.Fatalf("seed concept: %v", err)
	}
	c := withExternalRef(t, newCharge(t, "e0000000-0000-0000-0000-0000000000fe",
		tenantA, "50000000-0000-0000-0000-0000000000fe", cycle, conceptID, 8000, 0, today),
		"enrollment:"+cycle+":fe")
	opens := today.AddDate(0, 0, -10)
	c, err := c.WithStartsOn(opens)
	if err != nil {
		t.Fatalf("WithStartsOn: %v", err)
	}
	if n, err := repo.CreateChargesBulk(ctx, []domain.Charge{c}); err != nil || n != 1 {
		t.Fatalf("CreateChargesBulk = %d, %v", n, err)
	}
	got, err := repo.GetCharge(ctx, c.ID())
	if err != nil {
		t.Fatalf("GetCharge: %v", err)
	}
	if !got.Charge.StartsOn().Equal(opens) {
		t.Errorf("starts on %s, want %s", got.Charge.StartsOn().Format("2006-01-02"), opens.Format("2006-01-02"))
	}

	// The CHECK is the backstop for a writer that skipped the domain.
	moved, err := repo.RescheduleUnpaid(ctx, []ports.ChargeReschedule{
		{ExternalRef: c.ExternalRef(), StartsOn: today.AddDate(0, 0, 5), DueOn: today},
	})
	if err == nil {
		t.Fatalf("a start after the deadline was stored (moved %d)", moved)
	}
}

// One tenant cannot move another's deadlines. The update runs under RLS as the
// non-owner `app` role, so this is the write side of the same posture the
// isolation tests above pin for reads.
func TestRescheduleUnpaidCannotReachAnotherTenant(t *testing.T) {
	ctx := context.Background()
	const conceptID = "c0000000-0000-0000-0000-0000000000fd"
	const cycle = "a0000000-0000-0000-0000-0000000000fd"
	ownerRepo := appRepo(t, tenantA)
	if _, err := ownerRepo.CreateConcept(ctx, newConcept(t, conceptID, tenantA, "reschedule-rls", 8000)); err != nil {
		t.Fatalf("seed concept: %v", err)
	}
	c := newCharge(t, "e0000000-0000-0000-0000-0000000000fd",
		tenantA, "50000000-0000-0000-0000-0000000000fd", cycle, conceptID, 8000, 0, today)
	withRef, err := c.WithExternalRef("enrollment:" + cycle + ":fd")
	if err != nil {
		t.Fatalf("external ref: %v", err)
	}
	if _, err := ownerRepo.CreateCharge(ctx, withRef); err != nil {
		t.Fatalf("seed charge: %v", err)
	}

	intruder := appRepo(t, tenantB)
	moved, err := intruder.RescheduleUnpaid(ctx, []ports.ChargeReschedule{
		{ExternalRef: withRef.ExternalRef(), DueOn: today.AddDate(0, -2, 0)},
	})
	if err != nil {
		t.Fatalf("RescheduleUnpaid: %v", err)
	}
	if moved != 0 {
		t.Fatalf("tenant B moved %d of tenant A's charges", moved)
	}

	got, err := ownerRepo.GetCharge(ctx, withRef.ID())
	if err != nil {
		t.Fatalf("GetCharge: %v", err)
	}
	if !got.Charge.DueDate().Equal(today) {
		t.Errorf("the deadline moved to %s", got.Charge.DueDate().Format("2006-01-02"))
	}
}

// Task 10: a uniform sold to a pupil not yet placed in a group is charged with
// no cycle. It is stored, read back and listed with the pupil's charges; a row
// with no cycle AND no ref is refused by the table itself.
func TestASaleChargeWithoutACycle(t *testing.T) {
	ctx := context.Background()
	const (
		conceptID = "c0000000-0000-0000-0000-00000000ab10"
		studentID = "50000000-0000-0000-0000-00000000ab10"
		chargeID  = "d0000000-0000-0000-0000-00000000ab10"
	)
	repo := appRepo(t, tenantA)
	if _, err := repo.CreateConcept(ctx, newConcept(t, conceptID, tenantA, "uniformes sin ciclo", 45000)); err != nil {
		t.Fatalf("seed concept: %v", err)
	}
	sale, err := domain.NewSaleCharge(chargeID, tenantA, studentID, conceptID, "Uniformes · U-2026-000010", 45000, today, "uniforms:ful-nocycle")
	if err != nil {
		t.Fatalf("NewSaleCharge: %v", err)
	}
	if _, err := repo.CreateCharge(ctx, sale); err != nil {
		t.Fatalf("CreateCharge: %v", err)
	}

	got, err := repo.GetCharge(ctx, chargeID)
	if err != nil {
		t.Fatalf("GetCharge: %v", err)
	}
	if got.Charge.CycleID() != "" || got.Charge.ExternalRef() != "uniforms:ful-nocycle" {
		t.Errorf("read back: cycle %q, ref %q; want none and the sale's ref", got.Charge.CycleID(), got.Charge.ExternalRef())
	}
	listed, err := repo.ListCharges(ctx, studentID, "")
	if err != nil {
		t.Fatalf("ListCharges: %v", err)
	}
	if len(listed) != 1 {
		t.Errorf("the pupil's charges = %d, want the sale", len(listed))
	}

	err = superDB.Exec(`INSERT INTO charges (id, tenant_id, student_id, cycle_id, concept_id, description, amount_cents, due_date, status)
		VALUES ('d0000000-0000-0000-0000-00000000ab11', ?, ?, NULL, ?, 'sin ciclo ni ref', 100, ?, 'pending')`,
		tenantA, studentID, conceptID, today).Error
	if err == nil {
		t.Error("a charge with no cycle and no ref was stored; want the table to refuse it")
	}
}

func TestStudentWithdrawnConsumerCancelsFutureChargeInPostgres(t *testing.T) {
	ctx := context.Background()
	const (
		conceptID = "c0000000-0000-0000-0000-00000000ab21"
		studentID = "50000000-0000-0000-0000-00000000ab21"
		chargeID  = "d0000000-0000-0000-0000-00000000ab21"
	)
	repo := appRepo(t, tenantA)
	if _, err := repo.CreateConcept(ctx, newConcept(t, conceptID, tenantA, "withdrawal-consumer", 24000)); err != nil {
		t.Fatalf("seed concept: %v", err)
	}
	// A billing-run charge (tuition ref): only those may be cancelled by a baja.
	charge, err := newCharge(t, chargeID, tenantA, studentID, cycleOne, conceptID, 24000, 0, tomorrow()).
		WithExternalRef(domain.TuitionRefPrefix + cycleOne + ":" + studentID + ":2026-11")
	if err != nil {
		t.Fatalf("tuition ref: %v", err)
	}
	if _, err := repo.CreateCharge(ctx, charge); err != nil {
		t.Fatalf("seed charge: %v", err)
	}
	// UTC zone: `today` is a stored-style date (midnight UTC) and this test is
	// about the consumer's writes, not the school's zone (ADR 0035).
	service := usecase.NewFinanceService(repo, nil, nil, nil, nil, nil, nil, tenantA,
		func() string { return "generated-id" }, usecase.NewSchoolClock(func() time.Time { return today }, time.UTC))
	service.WithActivities(repo)
	consumer := financepubsub.NewConsumer(service, slog.New(slog.NewTextHandler(io.Discard, nil)))
	data, err := proto.Marshal(&studentsv1.StudentWithdrawn{
		TenantId: tenantA, StudentId: studentID, CycleId: cycleOne,
		EnrollmentId: "60000000-0000-0000-0000-00000000ab21", WithdrawnOn: today.Format("2006-01-02"),
	})
	if err != nil {
		t.Fatalf("marshal withdrawal: %v", err)
	}
	msg := pubsub.Message{Attributes: map[string]string{"type": "StudentWithdrawn", "tenant_id": tenantA}, Data: data}
	if err := consumer.Handle(ctx, msg); err != nil {
		t.Fatalf("consumer Handle: %v", err)
	}
	stored, err := repo.GetCharge(ctx, chargeID)
	if err != nil {
		t.Fatalf("read charge: %v", err)
	}
	if stored.Charge.Status() != domain.StatusCancelled || stored.Charge.CancellationReason() != "baja" {
		t.Fatalf("stored cancellation = %q (%q), want cancelled (baja)", stored.Charge.Status(), stored.Charge.CancellationReason())
	}
	if err := consumer.Handle(ctx, msg); err != nil {
		t.Fatalf("redelivery: %v", err)
	}
}

func TestWithdrawalCancellationWaitsForLockedPaymentDecision(t *testing.T) {
	ctx := context.Background()
	const (
		conceptID = "c0000000-0000-0000-0000-00000000ab22"
		studentID = "50000000-0000-0000-0000-00000000ab22"
		chargeID  = "d0000000-0000-0000-0000-00000000ab22"
		paymentID = "e0000000-0000-0000-0000-00000000ab22"
	)
	repo := appRepo(t, tenantA)
	if _, err := repo.CreateConcept(ctx, newConcept(t, conceptID, tenantA, "withdrawal-lock", 24000)); err != nil {
		t.Fatalf("seed concept: %v", err)
	}
	charge := newCharge(t, chargeID, tenantA, studentID, cycleOne, conceptID, 24000, 0, tomorrow())
	if _, err := repo.CreateCharge(ctx, charge); err != nil {
		t.Fatalf("seed charge: %v", err)
	}

	locked := superDB.Begin()
	if locked.Error != nil {
		t.Fatal(locked.Error)
	}
	var lockedID string
	if err := locked.Raw("SELECT id FROM charges WHERE id = ? FOR UPDATE", chargeID).Scan(&lockedID).Error; err != nil {
		_ = locked.Rollback().Error
		t.Fatalf("lock charge: %v", err)
	}
	cancelStarted := make(chan struct{})
	cancelResult := make(chan error, 1)
	go func() {
		close(cancelStarted)
		_, err := repo.CancelChargeIfUnpaidWithReason(ctx, chargeID, "baja", "")
		cancelResult <- err
	}()
	<-cancelStarted

	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiters int64
		err := superDB.Raw(`SELECT count(*) FROM pg_stat_activity
			WHERE wait_event_type = 'Lock' AND query ILIKE '%charges%'`).Scan(&waiters).Error
		if err != nil {
			_ = locked.Rollback().Error
			t.Fatalf("read lock waiters: %v", err)
		}
		if waiters > 0 {
			break
		}
		if time.Now().After(deadline) {
			_ = locked.Rollback().Error
			t.Fatal("cancellation did not wait for the charge row lock")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := locked.Exec(`INSERT INTO payments (id, tenant_id, charge_id, amount_cents, method, paid_at, paid_on)
		VALUES (?, ?, ?, 1000, 'cash', ?, ?)`, paymentID, tenantA, chargeID, paidAt, domain.CalendarDay(paidAt).Format("2006-01-02")).Error; err != nil {
		_ = locked.Rollback().Error
		t.Fatalf("post payment while holding row lock: %v", err)
	}
	if err := locked.Commit().Error; err != nil {
		t.Fatalf("commit payment: %v", err)
	}
	if err := <-cancelResult; glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("cancel after concurrent payment = %v, want KindConflict", err)
	}
	stored, err := repo.GetCharge(ctx, chargeID)
	if err != nil {
		t.Fatalf("read charge: %v", err)
	}
	if stored.Charge.Status() == domain.StatusCancelled || stored.PaidCents != 1000 {
		t.Errorf("charge after race = %q, paid %d; want partially paid and not cancelled", stored.Charge.Status(), stored.PaidCents)
	}
}

// TestGuardedCancellationRecordsWhenAndWho: C2-005 — the cancellation keeps
// its reason, the moment and the account that did it.
func TestGuardedCancellationRecordsWhenAndWho(t *testing.T) {
	ctx := context.Background()
	const (
		conceptID = "c0000000-0000-0000-0000-00000000c205"
		studentID = "50000000-0000-0000-0000-00000000c205"
		chargeID  = "d0000000-0000-0000-0000-00000000c205"
		actorID   = "a0000000-0000-0000-0000-00000000c205"
	)
	repo := appRepo(t, tenantA)
	if _, err := repo.CreateConcept(ctx, newConcept(t, conceptID, tenantA, "cancel-audit", 24000)); err != nil {
		t.Fatalf("seed concept: %v", err)
	}
	if _, err := repo.CreateCharge(ctx, newCharge(t, chargeID, tenantA, studentID, cycleOne, conceptID, 24000, 0, tomorrow())); err != nil {
		t.Fatalf("seed charge: %v", err)
	}

	if _, err := repo.CancelChargeIfUnpaidWithReason(ctx, chargeID, "Cargo duplicado", actorID); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	var row struct {
		CancelledBy *string
		Stamped     bool
	}
	if err := superDB.Raw(`SELECT cancelled_by::text AS cancelled_by, cancelled_at IS NOT NULL AS stamped
		FROM charges WHERE id = ?`, chargeID).Scan(&row).Error; err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if row.CancelledBy == nil || *row.CancelledBy != actorID || !row.Stamped {
		t.Fatalf("audit = by %v, stamped %v; want by %s and a time", row.CancelledBy, row.Stamped, actorID)
	}
}
