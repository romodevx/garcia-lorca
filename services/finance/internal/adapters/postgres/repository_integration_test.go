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
	"os"
	"slices"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/finance/internal/adapters/postgres"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

const (
	dbName  = "garcia_lorca"
	tenantA = "11111111-1111-1111-1111-111111111111"
	tenantB = "22222222-2222-2222-2222-222222222222"
)

// migrationFiles are applied in order before the app role is granted, so the
// non-owner role gets privileges on every table.
var migrationFiles = []string{
	"../../../../../migrations/finance/0001_init.sql",
	"../../../../../migrations/finance/0002_payment_idempotency.sql",
}

var (
	superDB   *gorm.DB
	appDSN    string
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
	return p
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
	if _, _, _, err := appRepo(t, tenantA).RegisterPayment(ctx, newPayment(t, paymentID, tenantA, chargeID, 5000, paidAt), derive); err != nil {
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

	_, first, _, err := repo.RegisterPayment(ctx, newPayment(t, "f0000000-0000-0000-0000-0000000000b1", tenantA, chargeID, 4000, paidAt), derive)
	if err != nil {
		t.Fatalf("first payment: %v", err)
	}
	if first.PaidCents != 4000 || first.Charge.Status() != domain.StatusPartiallyPaid {
		t.Errorf("after 4000: paid = %d, status = %q; want 4000 / partially_paid", first.PaidCents, first.Charge.Status())
	}

	_, second, _, err := repo.RegisterPayment(ctx, newPayment(t, "f0000000-0000-0000-0000-0000000000b2", tenantA, chargeID, 6000, paidAt.Add(time.Hour)), derive)
	if err != nil {
		t.Fatalf("second payment: %v", err)
	}
	if second.PaidCents != 10000 || second.Charge.Status() != domain.StatusPaid {
		t.Errorf("after 6000: paid = %d, status = %q; want 10000 / paid", second.PaidCents, second.Charge.Status())
	}

	// One cent more than owed: derive vetoes it and the transaction rolls back.
	if _, _, _, err := repo.RegisterPayment(ctx, newPayment(t, "f0000000-0000-0000-0000-0000000000b3", tenantA, chargeID, 1, paidAt.Add(2*time.Hour)), derive); err == nil {
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

	if _, _, _, err := repo.RegisterPayment(ctx, newPayment(t, "f0000000-0000-0000-0000-0000000000b4", tenantA, chargeID, 1000, paidAt), derive); err == nil {
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
		newKeyedPayment(t, firstID, tenantA, chargeID, 4000, paidAt, "attempt-1"), derive)
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
		newKeyedPayment(t, retryID, tenantA, chargeID, 4000, paidAt.Add(time.Minute), "attempt-1"), derive)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !replayed {
		t.Error("retry did not report a replay")
	}
	if again.ID() != stored.ID() {
		t.Errorf("retry returned payment %q, want the original %q", again.ID(), stored.ID())
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
		newKeyedPayment(t, otherID, tenantA, chargeID, 3000, paidAt.Add(time.Hour), "attempt-2"), derive); err != nil {
		t.Fatalf("second attempt: %v", err)
	}
	if replayed {
		t.Error("a distinct key was treated as a replay")
	}
	_, third, _, err := repo.RegisterPayment(ctx,
		newPayment(t, keylessID, tenantA, chargeID, 3000, paidAt.Add(2*time.Hour)), derive)
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
	if _, _, _, err := repo.RegisterPayment(ctx, newPayment(t, "f0000000-0000-0000-0000-0000000000d1", tenantA, chargeOne, 3000, paidAt), derive); err != nil {
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
	if _, _, _, err := repo.RegisterPayment(ctx, newPayment(t, "f0000000-0000-0000-0000-0000000000e3", tenantA, settledID, 1000, paidAt), derive); err != nil {
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

	overdue, err := repo.ListOverdue(ctx, cycleOverdue, today)
	if err != nil {
		t.Fatalf("ListOverdue: %v", err)
	}
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
