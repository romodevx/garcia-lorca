//go:build integration

// Schema tests for the uniforms migrations against a REAL Postgres
// (testcontainers-go). They prove the things a migration can get wrong in ways
// no unit test would notice: that RLS isolates tenants for reads AND writes when
// connected as the non-owner `app` role, that the stock ledger really is
// append-only rather than merely commented as such, that the counter invariant
// holds, and that folio allocation is per tenant.
//
// The harness creates the application role and its default privileges BEFORE
// running the migrations, mirroring production (setup_app_role.sql runs once,
// migrations follow). That order matters here: 0001 narrows the privileges it
// was granted at CREATE TABLE time, and granting afterwards would hand them back.
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
	"strings"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	dbName  = "garcia_lorca"
	tenantA = "11111111-1111-1111-1111-111111111111"
	tenantB = "22222222-2222-2222-2222-222222222222"
	// A tenant no other test in this package ever writes to. The isolation
	// assertions say "sees ZERO rows", which is only a statement about RLS if
	// the observer genuinely owns nothing — otherwise a sibling test seeding its
	// own legitimate row reads as a leak.
	tenantOutsider = "33333333-3333-3333-3333-333333333333"
)

var migrationFiles = []string{
	"../../../../../migrations/uniforms/0001_init.sql",
	"../../../../../migrations/uniforms/0002_orders.sql",
	"../../../../../migrations/uniforms/0003_settings.sql",
	"../../../../../migrations/uniforms/0004_sale_keys.sql",
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
		tcpostgres.WithUsername("owner"),
		tcpostgres.WithPassword(superPass),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2)),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start postgres: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = testcontainers.TerminateContainer(container) }()

	ownerDSN, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "connection string: %v\n", err)
		os.Exit(1)
	}
	superDB, err = open(ownerDSN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open owner: %v\n", err)
		os.Exit(1)
	}
	if err := setUp(ownerDSN); err != nil {
		fmt.Fprintf(os.Stderr, "set up schema: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func open(dsn string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
}

// setUp creates the non-owner application role with production's grants, THEN
// applies the migrations, so a migration that narrows its own privileges (the
// stock ledger) is not undone by a later blanket grant.
func setUp(ownerDSN string) error {
	for _, stmt := range []string{
		"CREATE ROLE app LOGIN PASSWORD '" + appPass + "' NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE",
		"GRANT USAGE ON SCHEMA public TO app",
		"ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO app",
		"ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO app",
	} {
		if err := superDB.Exec(stmt).Error; err != nil {
			return fmt.Errorf("%q: %w", stmt, err)
		}
	}
	for _, f := range migrationFiles {
		sqlBytes, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}
		if err := superDB.Exec(string(sqlBytes)).Error; err != nil {
			return fmt.Errorf("apply %s: %w", f, err)
		}
	}
	appDSN = strings.Replace(ownerDSN, "owner:"+superPass, "app:"+appPass, 1)
	return nil
}

// asApp opens a connection as the NON-OWNER app role with the tenant bound, so
// every assertion runs under the same RLS conditions as production.
func asApp(t *testing.T, tenant string) *gorm.DB {
	t.Helper()
	db, err := open(appDSN)
	if err != nil {
		t.Fatalf("open app connection: %v", err)
	}
	tx := db.Begin()
	t.Cleanup(func() { tx.Rollback() })
	if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenant).Error; err != nil {
		t.Fatalf("bind tenant: %v", err)
	}
	return tx
}

// seedItem inserts one garment for a tenant and returns its id.
func seedItem(t *testing.T, tx *gorm.DB, tenant, code string) string {
	t.Helper()
	var id string
	err := tx.Raw(`INSERT INTO uniform_items (tenant_id, code, name, price_cents)
	               VALUES (?, ?, ?, ?) RETURNING id`, tenant, code, "Playera", 32000).Scan(&id).Error
	if err != nil {
		t.Fatalf("seed item: %v", err)
	}
	return id
}

// --- RLS ---------------------------------------------------------------------

// TestRLSIsolatesEveryTable is the mandatory isolation test: seven tables, and a
// tenant may neither read nor write another's rows. A table missing from this
// list is a table nobody proved.
func TestRLSIsolatesEveryTable(t *testing.T) {
	tables := []string{
		"uniform_items", "uniform_stock", "uniform_stock_moves",
		"uniform_folio_counters", "uniform_orders", "uniform_fulfillments",
		"uniform_order_lines", "uniform_settings", "uniform_sale_keys",
	}

	// Seed one row per table as tenant A, using the owner connection so the seed
	// itself is not the thing under test.
	itemID := ownerSeed(t)

	for _, table := range tables {
		t.Run(table+" read", func(t *testing.T) {
			tx := asApp(t, tenantOutsider)
			var n int64
			if err := tx.Raw("SELECT count(*) FROM " + table).Scan(&n).Error; err != nil {
				t.Fatalf("count: %v", err)
			}
			if n != 0 {
				t.Errorf("an outsider sees %d rows of tenant A in %s", n, table)
			}
		})
	}

	t.Run("write across tenants is refused", func(t *testing.T) {
		tx := asApp(t, tenantOutsider)
		// The outsider, bound to its own tenant, tries to plant a row labelled A.
		err := tx.Exec(`INSERT INTO uniform_items (tenant_id, code, name, price_cents)
		                VALUES (?, 'SMUGGLED', 'x', 1)`, tenantA).Error
		if err == nil {
			t.Error("an outsider inserted a row belonging to tenant A")
		}
	})

	t.Run("update across tenants touches nothing", func(t *testing.T) {
		tx := asApp(t, tenantOutsider)
		res := tx.Exec("UPDATE uniform_items SET name = 'hijacked' WHERE id = ?", itemID)
		if res.Error != nil {
			t.Fatalf("update: %v", res.Error)
		}
		if res.RowsAffected != 0 {
			t.Errorf("an outsider updated %d of tenant A's rows", res.RowsAffected)
		}
	})
}

// ownerSeed plants one row in every table as tenant A, from the owner
// connection, and returns the item id the RLS assertions poke at.
func ownerSeed(t *testing.T) string {
	t.Helper()
	var itemID, stockID, orderID, fulfillmentID string
	exec := func(query string, args ...any) {
		t.Helper()
		if err := superDB.Exec(query, args...).Error; err != nil {
			t.Fatalf("seed %q: %v", query, err)
		}
	}
	scan := func(dst *string, query string, args ...any) {
		t.Helper()
		if err := superDB.Raw(query, args...).Scan(dst).Error; err != nil {
			t.Fatalf("seed %q: %v", query, err)
		}
	}
	scan(&itemID, `INSERT INTO uniform_items (tenant_id, code, name, price_cents)
	               VALUES (?, 'RLS-1', 'Playera', 32000) RETURNING id`, tenantA)
	scan(&stockID, `INSERT INTO uniform_stock (tenant_id, item_id, size_label, on_hand)
	                VALUES (?, ?, '8', 5) RETURNING id`, tenantA, itemID)
	exec(`INSERT INTO uniform_stock_moves (tenant_id, stock_id, kind, qty)
	      VALUES (?, ?, 'receive', 5)`, tenantA, stockID)
	// ON CONFLICT because the repository tests share this container and may have
	// already opened this year's run of folios. The seed is scaffolding, not the
	// thing under test.
	// Its own year, so it can never collide with the repository tests that share
	// this container and allocate real folios.
	exec(`INSERT INTO uniform_folio_counters (tenant_id, year, next_seq) VALUES (?, 2099, 1)
	      ON CONFLICT (tenant_id, year) DO NOTHING`, tenantA)
	scan(&orderID, `INSERT INTO uniform_orders (tenant_id, folio, source)
	                VALUES (?, 'U-2099-000001', 'counter') RETURNING id`, tenantA)
	scan(&fulfillmentID, `INSERT INTO uniform_fulfillments (tenant_id, order_id, seq, status)
	                      VALUES (?, ?, 1, 'picking') RETURNING id`, tenantA, orderID)
	exec(`INSERT INTO uniform_order_lines (tenant_id, order_id, item_id, item_name, size_label, qty, unit_price_cents, status, fulfillment_id)
	      VALUES (?, ?, ?, 'Playera', '8', 1, 32000, 'reserved', ?)`, tenantA, orderID, itemID, fulfillmentID)
	exec(`INSERT INTO uniform_settings (tenant_id) VALUES (?)`, tenantA)
	exec(`INSERT INTO uniform_sale_keys (tenant_id, idempotency_key, order_id)
	      VALUES (?, 'seed-key', ?)`, tenantA, orderID)
	return itemID
}

// --- the stock ledger is append-only ----------------------------------------

// TestStockMovesAreAppendOnly proves the ledger's promise is enforced, not
// merely written in a comment. Its whole value is that a physical count can be
// audited against it months later, which is worth nothing if a row can be
// rewritten. UPDATE and DELETE are revoked from the application role, so the
// attempt fails loudly instead of quietly matching no rows.
func TestStockMovesAreAppendOnly(t *testing.T) {
	itemID := ownerSeedForLedger(t)
	tx := asApp(t, tenantA)

	// Appending is the one thing that works.
	var stockID string
	if err := tx.Raw(`SELECT id FROM uniform_stock WHERE item_id = ?`, itemID).Scan(&stockID).Error; err != nil {
		t.Fatalf("read stock: %v", err)
	}
	if err := tx.Exec(`INSERT INTO uniform_stock_moves (tenant_id, stock_id, kind, qty)
	                   VALUES (?, ?, 'adjust', -1)`, tenantA, stockID).Error; err != nil {
		t.Fatalf("append should be allowed: %v", err)
	}

	// The error has to be a PRIVILEGE error. RLS alone would make these match no
	// rows and report success, which is the failure mode this test exists to
	// rule out: a ledger that silently ignores a rewrite looks identical to one
	// that refuses it, right up until an audit.
	// Each attempt runs inside its own savepoint. Without that the first failure
	// aborts the transaction and the second reports "current transaction is
	// aborted" — an incidental error that would let a permitted DELETE pass for
	// a refused one.
	for i, stmt := range []string{
		`UPDATE uniform_stock_moves SET qty = 999`,
		`DELETE FROM uniform_stock_moves`,
	} {
		point := fmt.Sprintf("sp%d", i)
		if err := tx.Exec("SAVEPOINT " + point).Error; err != nil {
			t.Fatalf("savepoint: %v", err)
		}
		err := tx.Exec(stmt).Error
		if err == nil {
			t.Errorf("%q was allowed against an append-only ledger", stmt)
		} else if !strings.Contains(err.Error(), "permission denied") {
			t.Errorf("%q failed with %v; want a permission error, not an incidental one", stmt, err)
		}
		if err := tx.Exec("ROLLBACK TO SAVEPOINT " + point).Error; err != nil {
			t.Fatalf("rollback to savepoint: %v", err)
		}
	}
}

func ownerSeedForLedger(t *testing.T) string {
	t.Helper()
	var itemID, stockID string
	if err := superDB.Raw(`INSERT INTO uniform_items (tenant_id, code, name, price_cents)
	                       VALUES (?, 'LEDGER-1', 'Sueter', 65000) RETURNING id`, tenantA).Scan(&itemID).Error; err != nil {
		t.Fatalf("seed item: %v", err)
	}
	if err := superDB.Raw(`INSERT INTO uniform_stock (tenant_id, item_id, size_label, on_hand)
	                       VALUES (?, ?, '10', 3) RETURNING id`, tenantA, itemID).Scan(&stockID).Error; err != nil {
		t.Fatalf("seed stock: %v", err)
	}
	return itemID
}

// --- the counter invariant ---------------------------------------------------

// TestStockCannotReserveMoreThanItHas is the invariant the module rests on. The
// last case is the one that matters most: selling the final unit must decrement
// BOTH counters in one statement. Dropping on_hand alone trips the check, and
// since that write happens inside the transaction that consumes the payment
// event, tripping it would send the event to the dead-letter queue with the
// goods paid for and still held.
func TestStockCannotReserveMoreThanItHas(t *testing.T) {
	tx := asApp(t, tenantA)
	itemID := seedItem(t, tx, tenantA, "INV-1")
	var stockID string
	if err := tx.Raw(`INSERT INTO uniform_stock (tenant_id, item_id, size_label, on_hand, reserved)
	                  VALUES (?, ?, '6', 1, 1) RETURNING id`, tenantA, itemID).Scan(&stockID).Error; err != nil {
		t.Fatalf("seed stock: %v", err)
	}

	// Savepoints again: a failed statement poisons the transaction, so without
	// them the second case would "pass" on the first case's error.
	refuses := func(t *testing.T, point, stmt string) {
		t.Helper()
		if err := tx.Exec("SAVEPOINT " + point).Error; err != nil {
			t.Fatalf("savepoint: %v", err)
		}
		err := tx.Exec(stmt, stockID).Error
		if err == nil {
			t.Errorf("%q was accepted; the counter invariant did not hold", stmt)
		} else if !strings.Contains(err.Error(), "uniform_stock_counters") {
			t.Errorf("%q failed with %v; want the counter check, not an incidental error", stmt, err)
		}
		if err := tx.Exec("ROLLBACK TO SAVEPOINT " + point).Error; err != nil {
			t.Fatalf("rollback to savepoint: %v", err)
		}
	}

	t.Run("reserving beyond stock is refused", func(t *testing.T) {
		refuses(t, "sp_reserve", `UPDATE uniform_stock SET reserved = reserved + 1 WHERE id = ?`)
	})

	t.Run("selling the last unit needs both counters", func(t *testing.T) {
		// Wrong on purpose: the sale drops the physical count and leaves the
		// hold behind. Right is on_hand and reserved falling together, in one
		// statement.
		refuses(t, "sp_sell", `UPDATE uniform_stock SET on_hand = on_hand - 1 WHERE id = ?`)
	})

	t.Run("selling the last unit with both counters is accepted", func(t *testing.T) {
		if err := tx.Exec("SAVEPOINT sp_ok").Error; err != nil {
			t.Fatalf("savepoint: %v", err)
		}
		if err := tx.Exec(`UPDATE uniform_stock SET on_hand = on_hand - 1, reserved = reserved - 1 WHERE id = ?`,
			stockID).Error; err != nil {
			t.Errorf("the correct pair was refused: %v", err)
		}
		if err := tx.Exec("ROLLBACK TO SAVEPOINT sp_ok").Error; err != nil {
			t.Fatalf("rollback to savepoint: %v", err)
		}
	})
}

// --- folios ------------------------------------------------------------------

// TestFolioCountersArePerTenant proves two schools do not share one run of
// numbers, which would leak each school's volume to the other and is the reason
// this is a counter table rather than a Postgres sequence.
func TestFolioCountersArePerTenant(t *testing.T) {
	next := func(tenant string) int {
		t.Helper()
		tx := asApp(t, tenant)
		var seq int
		err := tx.Raw(`INSERT INTO uniform_folio_counters (tenant_id, year, next_seq)
		               VALUES (?, 2027, 2)
		               ON CONFLICT (tenant_id, year) DO UPDATE SET next_seq = uniform_folio_counters.next_seq + 1
		               RETURNING next_seq - 1`, tenant).Scan(&seq).Error
		if err != nil {
			t.Fatalf("allocate folio for %s: %v", tenant, err)
		}
		return seq
	}

	if got := next(tenantA); got != 1 {
		t.Errorf("tenant A first folio = %d, want 1", got)
	}
	if got := next(tenantB); got != 1 {
		t.Errorf("tenant B first folio = %d, want 1 (counters must not be shared)", got)
	}
}

// --- shape checks ------------------------------------------------------------

// TestOrderRequiresCycleWhenItHasAStudent guards the rule that a counter sale to
// nobody cannot be charged to anybody: no student means no cycle, and a student
// always names the cycle the charge belongs to.
func TestOrderRequiresCycleWhenItHasAStudent(t *testing.T) {
	tx := asApp(t, tenantA)
	err := tx.Exec(`INSERT INTO uniform_orders (tenant_id, folio, student_id, source)
	                VALUES (?, 'U-2099-000900', gen_random_uuid(), 'family')`, tenantA).Error
	if err == nil {
		t.Error("an order naming a student was accepted with no cycle")
	}
}

// TestFolioFormatIsEnforced keeps the panel's search able to tell a folio from a
// student's name by shape alone.
func TestFolioFormatIsEnforced(t *testing.T) {
	tx := asApp(t, tenantA)
	if err := tx.Exec(`INSERT INTO uniform_orders (tenant_id, folio, source)
	                   VALUES (?, 'nonsense', 'counter')`, tenantA).Error; err == nil {
		t.Error("a malformed folio was accepted")
	}
}
