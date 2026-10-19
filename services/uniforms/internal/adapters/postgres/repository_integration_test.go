//go:build integration

// Integration tests for the uniforms postgres adapter against a REAL Postgres.
// They prove the things only a database can: that the conditional deduction
// refuses to spend a hold, that FIFO allocation is a queue and not a scramble,
// that a replayed sale sells nothing twice, and that the ledger records every
// movement — all as the NON-OWNER app role, so RLS is in force throughout.
//
// Run: go test -tags=integration ./internal/adapters/postgres/
// Requires Docker.
package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/uniforms/internal/adapters/postgres"
	"github.com/garcia-lorca/services/uniforms/internal/domain"
	"github.com/garcia-lorca/services/uniforms/internal/ports"
)

var soldAt = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)

// appRepo builds the repository over a connection as the NON-OWNER app role, so
// every test runs under production's RLS conditions.
func appRepo(t *testing.T, tenant string) *pgadapter.Repository {
	t.Helper()
	db, err := open(appDSN)
	if err != nil {
		t.Fatalf("open app connection: %v", err)
	}
	t.Cleanup(func() {
		if sql, err := db.DB(); err == nil {
			_ = sql.Close()
		}
	})
	return pgadapter.NewRepository(db, tenant, func() time.Time { return soldAt })
}

// seedGarment puts a garment on sale with one size and a starting count.
func seedGarment(t *testing.T, repo *pgadapter.Repository, name, size string, price int64, onHand int) (domain.Item, domain.StockLevel) {
	t.Helper()
	ctx := context.Background()
	created, err := repo.CreateItem(ctx, mustItem(t, name, price))
	if err != nil {
		t.Fatalf("seed garment %q: %v", name, err)
	}
	if _, _, _, err := repo.UpsertSizes(ctx, created.ID(), []ports.SizeSpec{{SizeLabel: size, SizeOrder: 1}}); err != nil {
		t.Fatalf("seed size: %v", err)
	}
	stock, err := repo.GetStockBySize(ctx, created.ID(), size)
	if err != nil {
		t.Fatalf("read seeded size: %v", err)
	}
	if onHand > 0 {
		if _, _, err := repo.ReceiveStock(ctx, []ports.ReceiveLine{{StockID: stock.ID(), Qty: onHand}}, "seed", ""); err != nil {
			t.Fatalf("seed stock: %v", err)
		}
		stock, err = repo.GetStockBySize(ctx, created.ID(), size)
		if err != nil {
			t.Fatalf("re-read seeded size: %v", err)
		}
	}
	return created, stock
}

func mustItem(t *testing.T, name string, price int64) domain.Item {
	t.Helper()
	item, err := domain.NewItem(newUUID(t), tenantA, name, "", domain.LevelAny, price)
	if err != nil {
		t.Fatalf("NewItem: %v", err)
	}
	return item
}

func newUUID(t *testing.T) string {
	t.Helper()
	var id string
	if err := superDB.Raw("SELECT gen_random_uuid()").Scan(&id).Error; err != nil {
		t.Fatalf("mint an id: %v", err)
	}
	return id
}

// --- catalogue ---------------------------------------------------------------

// Task 3: an article has no clave. Two with the same name are two articles,
// told apart by id; nothing in the school's schema asks for a code any more.
func TestCreateItemNeedsNoCode(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	if _, err := repo.CreateItem(ctx, mustItem(t, "Playera sin clave", 32000)); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := repo.CreateItem(ctx, mustItem(t, "Playera sin clave", 32000)); err != nil {
		t.Errorf("a second article without a clave: %v", err)
	}
}

// TestUpsertSizesNeverTouchesTheCount is the rule that keeps the ledger able to
// explain itself: a size run is the SHAPE of the catalogue, and stock moves only
// through receipts, sales and adjustments, each of which leaves an entry.
func TestUpsertSizesNeverTouchesTheCount(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	item, stock := seedGarment(t, repo, "KEEP-1", "8", 32000, 7)

	created, updated, _, err := repo.UpsertSizes(ctx, item.ID(),
		[]ports.SizeSpec{{SizeLabel: "8", SizeOrder: 9, MinStock: 4}, {SizeLabel: "10", SizeOrder: 10}})
	if err != nil {
		t.Fatalf("UpsertSizes: %v", err)
	}
	if created != 1 || updated != 1 {
		t.Errorf("created %d, updated %d; want 1 and 1", created, updated)
	}
	after, err := repo.GetStockBySize(ctx, item.ID(), "8")
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if after.OnHand() != stock.OnHand() {
		t.Errorf("on hand changed from %d to %d; a size run must not move stock", stock.OnHand(), after.OnHand())
	}
	if after.MinStock() != 4 {
		t.Errorf("minimum = %d, want 4", after.MinStock())
	}
}

// --- the conditional deduction ----------------------------------------------

// TestCounterSaleCannotSpendAHold is the reason the deduction is a conditional
// UPDATE whose predicate says AVAILABLE rather than on hand. Four of five are
// promised; the till may sell one and no more.
func TestCounterSaleCannotSpendAHold(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	item, stock := seedGarment(t, repo, "HOLD-1", "8", 32000, 5)
	holdStock(t, stock.ID(), 4)

	if _, _, err := repo.CounterSale(ctx, saleCmd(t, item, "8", 1, "key-hold-1")); err != nil {
		t.Fatalf("selling the one free piece: %v", err)
	}
	_, _, err := repo.CounterSale(ctx, saleCmd(t, item, "8", 1, "key-hold-2"))
	if glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("want KindConflict selling into a hold, got %v", err)
	}
	// WHICH mechanism refused it matters. The predicate on the UPDATE returns
	// zero rows and yields this message; the table's CHECK, which also catches
	// this case, yields a different one and ABORTS the transaction — fine for a
	// single line, fatal for a multi-line sale where later lines still have work
	// to do. Asserting the message pins the refusal to the predicate and stops a
	// weakened WHERE clause from hiding behind the constraint.
	if !strings.Contains(err.Error(), "no quedan suficientes") {
		t.Errorf("refused by the constraint rather than the predicate: %v", err)
	}

	after, err := repo.GetStockBySize(ctx, item.ID(), "8")
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if after.OnHand() != 4 || after.Reserved() != 4 {
		t.Errorf("on hand %d, reserved %d; want 4 and 4 — the hold is intact", after.OnHand(), after.Reserved())
	}
}

// TestAdjustStockNamesWhyItRefused (C2-002): going below zero with nothing held
// and stripping a family's hold are different refusals, and say so.
func TestAdjustStockNamesWhyItRefused(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	_, bare := seedGarment(t, repo, "ADJ-ZERO", "8", 32000, 3)
	_, held := seedGarment(t, repo, "ADJ-HOLD", "8", 32000, 5)
	holdStock(t, held.ID(), 4)

	_, err := repo.AdjustStock(ctx, bare.ID(), -5, "conteo físico", "")
	if glerrors.KindOf(err) != glerrors.KindConflict || !strings.Contains(err.Error(), "below zero") {
		t.Errorf("below zero with nothing held: got %v", err)
	}
	_, err = repo.AdjustStock(ctx, held.ID(), -3, "conteo físico", "")
	if glerrors.KindOf(err) != glerrors.KindConflict || !strings.Contains(err.Error(), "below what is held") {
		t.Errorf("below the holds: got %v", err)
	}
}

// TestCounterSaleIsIdempotent: a double click at the till sells once.
func TestCounterSaleIsIdempotent(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	item, _ := seedGarment(t, repo, "IDEM-1", "8", 32000, 5)

	first, replayed, err := repo.CounterSale(ctx, saleCmd(t, item, "8", 1, "key-idem"))
	if err != nil {
		t.Fatalf("first sale: %v", err)
	}
	if replayed {
		t.Error("the first sale reported itself as a replay")
	}

	second, replayed, err := repo.CounterSale(ctx, saleCmd(t, item, "8", 1, "key-idem"))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !replayed {
		t.Error("the retry was not recognised as a replay")
	}
	if second.Order.ID() != first.Order.ID() {
		t.Errorf("the replay produced a different order: %q vs %q", second.Order.ID(), first.Order.ID())
	}
	if len(second.Lines) != 1 || len(second.Fulfillments) != 1 {
		t.Errorf("the replay did not return the original sale: %d lines, %d deliveries",
			len(second.Lines), len(second.Fulfillments))
	}

	after, _ := repo.GetStockBySize(ctx, item.ID(), "8")
	if after.OnHand() != 4 {
		t.Errorf("on hand = %d, want 4 — one sale, not two", after.OnHand())
	}
}

// TestCounterSaleIsAtomic: if one line of a multi-line sale cannot be covered,
// NOTHING is written. A partial sale would leave a folio, a delivery and a
// deduction for goods the parent did not get.
func TestCounterSaleIsAtomic(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	plenty, _ := seedGarment(t, repo, "ATOM-1", "8", 32000, 5)
	scarce, _ := seedGarment(t, repo, "ATOM-2", "8", 48000, 0)

	cmd := saleCmd(t, plenty, "8", 1, "key-atomic")
	cmd.Lines = append(cmd.Lines, mustLine(t, cmd.OrderID, scarce, "8", 1))

	if _, _, err := repo.CounterSale(ctx, cmd); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("want KindConflict, got %v", err)
	}

	// The line that COULD have been covered must not have been.
	after, _ := repo.GetStockBySize(ctx, plenty.ID(), "8")
	if after.OnHand() != 5 {
		t.Errorf("on hand = %d, want 5 — a refused sale writes nothing", after.OnHand())
	}
	orders, _, total, err := repo.ListOrders(ctx, ports.ListOrdersFilter{})
	if err != nil {
		t.Fatalf("ListOrders: %v", err)
	}
	for _, o := range orders {
		if o.Order.ID() == cmd.OrderID {
			t.Error("the refused sale left an order behind")
		}
	}
	_ = total
}

// TestCounterSaleWritesTheLedger: every movement of every unit is recorded, which
// is the only way a physical count months from now can be audited.
func TestCounterSaleWritesTheLedger(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	item, stock := seedGarment(t, repo, "LEDG-1", "8", 32000, 5)

	if _, _, err := repo.CounterSale(ctx, saleCmd(t, item, "8", 2, "key-ledger")); err != nil {
		t.Fatalf("CounterSale: %v", err)
	}

	var kinds []string
	var qtys []int
	rows, err := superDB.Raw(`SELECT kind, qty FROM uniform_stock_moves
	                          WHERE stock_id = ? ORDER BY created_at ASC, kind ASC`, stock.ID()).Rows()
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var qty int
		if err := rows.Scan(&kind, &qty); err != nil {
			t.Fatalf("scan: %v", err)
		}
		kinds = append(kinds, kind)
		qtys = append(qtys, qty)
	}
	// The receipt that seeded it, then the sale.
	if len(kinds) != 2 || kinds[0] != "receive" || kinds[1] != "sell" {
		t.Fatalf("ledger entries = %v, want receive then sell", kinds)
	}
	if qtys[0] != 5 || qtys[1] != -2 {
		t.Errorf("quantities = %v, want 5 then -2 — the sign is part of the record", qtys)
	}
}

// Task 10: a pupil registered but not yet placed in a group buys at the counter.
// The order names the pupil and no cycle, and reads back that way.
func TestCounterSaleToAPupilWithNoCycle(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	item, _ := seedGarment(t, repo, "NOCY-1", "8", 32000, 5)

	cmd := saleCmd(t, item, "8", 1, "key-nocycle")
	cmd.StudentID, cmd.StudentName = newUUID(t), "Zoe Villalobos"
	saved, _, err := repo.CounterSale(ctx, cmd)
	if err != nil {
		t.Fatalf("CounterSale to a pupil with no cycle: %v", err)
	}
	if saved.Order.StudentID() != cmd.StudentID || saved.Order.CycleID() != "" {
		t.Errorf("order = student %q, cycle %q; want the pupil and no cycle", saved.Order.StudentID(), saved.Order.CycleID())
	}

	// A family order still names its cycle: the table refuses a child's order
	// with none that did not come from the counter.
	err = superDB.Exec(`UPDATE uniform_orders SET source = 'family' WHERE id = ?`, saved.Order.ID()).Error
	if err == nil {
		t.Error("a family order with a pupil and no cycle was stored; want the table to refuse it")
	}
}

// --- FIFO allocation ---------------------------------------------------------

// TestReceiveStockAllocatesOldestFirst is the shop's fairness rule. A family who
// ordered three weeks ago gets the arriving piece before one who ordered
// yesterday — and before the counter can sell it.
func TestReceiveStockAllocatesOldestFirst(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	item, stock := seedGarment(t, repo, "FIFO-1", "8", 32000, 0)

	older := seedWaitingLine(t, item, "8", 1, soldAt.Add(-72*time.Hour))
	newer := seedWaitingLine(t, item, "8", 1, soldAt.Add(-1*time.Hour))

	_, allocated, err := repo.ReceiveStock(ctx, []ports.ReceiveLine{{StockID: stock.ID(), Qty: 1}}, "una pieza", "")
	if err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}
	if len(allocated) != 1 {
		t.Fatalf("allocated %d lines, want 1 — only one piece arrived", len(allocated))
	}
	if allocated[0] != older {
		t.Errorf("allocated the newer line; the queue is not a queue")
	}
	if lineStatus(t, newer) != string(domain.LineAwaitingStock) {
		t.Errorf("the newer line was reserved out of turn")
	}
	after, _ := repo.GetStockBySize(ctx, item.ID(), "8")
	if after.OnHand() != 1 || after.Reserved() != 1 {
		t.Errorf("on hand %d, reserved %d; the arriving piece is held for the older order",
			after.OnHand(), after.Reserved())
	}
}

// TestReceiveStockStopsAtTheFirstLineItCannotCover: skipping ahead to a smaller
// order is exactly how the oldest one never gets filled.
func TestReceiveStockStopsAtTheFirstLineItCannotCover(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	item, stock := seedGarment(t, repo, "FIFO-2", "8", 32000, 0)

	big := seedWaitingLine(t, item, "8", 3, soldAt.Add(-72*time.Hour))
	small := seedWaitingLine(t, item, "8", 1, soldAt.Add(-1*time.Hour))

	_, allocated, err := repo.ReceiveStock(ctx, []ports.ReceiveLine{{StockID: stock.ID(), Qty: 2}}, "dos piezas", "")
	if err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}
	if len(allocated) != 0 {
		t.Errorf("allocated %d lines; two pieces cannot cover the order of three at the head", len(allocated))
	}
	if lineStatus(t, small) != string(domain.LineAwaitingStock) {
		t.Error("the smaller, newer order jumped the queue")
	}
	_ = big
}

// --- reads -------------------------------------------------------------------

// TestListOrdersSearchTellsAFolioFromAName is what lets one search box take
// either without asking the user which they meant.
func TestListOrdersSearchTellsAFolioFromAName(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	item, _ := seedGarment(t, repo, "SRCH-1", "8", 32000, 5)
	sale, _, err := repo.CounterSale(ctx, saleCmd(t, item, "8", 1, "key-search"))
	if err != nil {
		t.Fatalf("CounterSale: %v", err)
	}

	byFolio, _, _, err := repo.ListOrders(ctx, ports.ListOrdersFilter{Query: sale.Order.Folio()})
	if err != nil {
		t.Fatalf("search by folio: %v", err)
	}
	if len(byFolio) != 1 || byFolio[0].Order.ID() != sale.Order.ID() {
		t.Errorf("searching the folio returned %d orders", len(byFolio))
	}

	// A delivery folio finds the order it belongs to.
	bySuffix, _, _, err := repo.ListOrders(ctx, ports.ListOrdersFilter{Query: sale.Order.Folio() + "-A"})
	if err != nil {
		t.Fatalf("search by delivery folio: %v", err)
	}
	if len(bySuffix) != 1 {
		t.Errorf("searching a delivery folio returned %d orders", len(bySuffix))
	}
}

// TestListOrdersEscapesWildcards: a parent called "100%" is a search term, not a
// pattern. Without escaping, a search for "%" returns the whole school.
func TestListOrdersEscapesWildcards(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	item, _ := seedGarment(t, repo, "ESC-1", "8", 32000, 5)
	cmd := saleCmd(t, item, "8", 1, "key-escape")
	cmd.StudentID = newUUID(t)
	cmd.StudentName = "Ana Ruiz"
	cmd.StudentLevel = domain.Level("c1000000-0000-0000-0000-000000000002")
	cmd.CycleID = newUUID(t)
	if _, _, err := repo.CounterSale(ctx, cmd); err != nil {
		t.Fatalf("CounterSale: %v", err)
	}

	got, _, _, err := repo.ListOrders(ctx, ports.ListOrdersFilter{Query: "%"})
	if err != nil {
		t.Fatalf("ListOrders: %v", err)
	}
	for _, o := range got {
		if o.Order.ID() == cmd.OrderID {
			t.Error(`searching for "%" matched a name; the wildcard was interpreted, not escaped`)
		}
	}
}

// TestGetOrderScopedToAFamilyReportsNotFound: an order outside the caller's
// children is NOT FOUND, never forbidden. A 403 on somebody else's id confirms
// that it exists, which is half of what an attacker wanted.
func TestGetOrderScopedToAFamilyReportsNotFound(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	item, _ := seedGarment(t, repo, "SCOPE-1", "8", 32000, 5)
	cmd := saleCmd(t, item, "8", 1, "key-scope")
	cmd.StudentID = newUUID(t)
	cmd.StudentName = "Ana Ruiz"
	cmd.StudentLevel = domain.Level("c1000000-0000-0000-0000-000000000002")
	cmd.CycleID = newUUID(t)
	sale, _, err := repo.CounterSale(ctx, cmd)
	if err != nil {
		t.Fatalf("CounterSale: %v", err)
	}

	// The rightful family sees it.
	if _, err := repo.GetOrder(ctx, sale.Order.ID(), []string{cmd.StudentID}); err != nil {
		t.Fatalf("the order's own family cannot read it: %v", err)
	}
	// Another family gets nothing, and learns nothing.
	_, err = repo.GetOrder(ctx, sale.Order.ID(), []string{newUUID(t)})
	if glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("want KindNotFound for another family's order, got %v", err)
	}
}

// TestSettingsDefaultBeforeTheyAreEverSaved: a school that has never opened the
// settings screen still gets a working shop.
func TestSettingsDefaultBeforeTheyAreEverSaved(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantB) // untouched tenant
	got, err := repo.GetSettings(ctx)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if got.PickupWindowBusinessDays() != 10 {
		t.Errorf("window = %d, want the default 10", got.PickupWindowBusinessDays())
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantB)
	next, err := domain.NewSettings(tenantB, 15, false, 1, 3, 10, 3, 1)
	if err != nil {
		t.Fatalf("NewSettings: %v", err)
	}
	if _, err := repo.UpdateSettings(ctx, next); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	got, err := repo.GetSettings(ctx)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if got.PickupWindowBusinessDays() != 15 || got.AutoCancelEnabled() {
		t.Errorf("settings did not round-trip: %+v", got)
	}
}

// --- helpers -----------------------------------------------------------------

func saleCmd(t *testing.T, item domain.Item, size string, qty int, key string) ports.CounterSaleCommand {
	t.Helper()
	orderID := newUUID(t)
	return ports.CounterSaleCommand{
		OrderID: orderID, FulfillmentID: newUUID(t), TenantID: tenantA,
		Lines:      []domain.OrderLine{mustLine(t, orderID, item, size, qty)},
		TotalCents: item.PriceCents() * int64(qty),
		Note:       "venta de prueba", IdempotencyKey: key, SoldAt: soldAt, FolioYear: soldAt.Year(),
	}
}

func mustLine(t *testing.T, orderID string, item domain.Item, size string, qty int) domain.OrderLine {
	t.Helper()
	line, err := item.LineFor(newUUID(t), orderID, size, qty, domain.LineSold)
	if err != nil {
		t.Fatalf("LineFor: %v", err)
	}
	return line
}

// holdStock reserves units directly, standing in for the family orders that
// stage two will create.
func holdStock(t *testing.T, stockID string, qty int) {
	t.Helper()
	if err := superDB.Exec(`UPDATE uniform_stock SET reserved = reserved + ? WHERE id = ?`,
		qty, stockID).Error; err != nil {
		t.Fatalf("seed a hold: %v", err)
	}
}

// seedWaitingLine plants a line that has been waiting for stock since `since`,
// so the FIFO tests have a queue with a real age order.
func seedWaitingLine(t *testing.T, item domain.Item, size string, qty int, since time.Time) string {
	t.Helper()
	orderID := newUUID(t)
	studentID := newUUID(t)
	if err := superDB.Exec(`INSERT INTO uniform_orders
	    (id, tenant_id, folio, student_id, student_name, student_level, cycle_id, source, created_at)
	    VALUES (?, ?, ?, ?, 'Familia en espera', 'primaria', ?, 'family', ?)`,
		orderID, tenantA, folioFor(t), studentID, newUUID(t), since).Error; err != nil {
		t.Fatalf("seed waiting order: %v", err)
	}
	lineID := newUUID(t)
	if err := superDB.Exec(`INSERT INTO uniform_order_lines
	    (id, tenant_id, order_id, item_id, item_name, size_label, qty, unit_price_cents, status, created_at)
	    VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'awaiting_stock', ?)`,
		lineID, tenantA, orderID, item.ID(), item.Name(), size, qty, item.PriceCents(), since).Error; err != nil {
		t.Fatalf("seed waiting line: %v", err)
	}
	return lineID
}

var folioSeq int

func folioFor(t *testing.T) string {
	t.Helper()
	folioSeq++
	return domain.FormatFolio(2025, folioSeq)
}

func lineStatus(t *testing.T, lineID string) string {
	t.Helper()
	var status string
	if err := superDB.Raw(`SELECT status FROM uniform_order_lines WHERE id = ?`, lineID).
		Scan(&status).Error; err != nil {
		t.Fatalf("read line status: %v", err)
	}
	return status
}

// T5b review: the folio numbers under the year the use case hands in (the
// school's year), not the UTC year of the instant. A sale at 19:00 on
// 31 December 2026 in Morelia is 01:00 UTC on 1 January 2027 and is U-2026-….
func TestCounterSaleFolioIsNumberedInTheGivenYear(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	item, _ := seedGarment(t, repo, "FOLIO-YEAR-1", "8", 32000, 5)

	cmd := saleCmd(t, item, "8", 1, "key-folio-year")
	cmd.SoldAt = time.Date(2027, time.January, 1, 1, 0, 0, 0, time.UTC)
	cmd.FolioYear = 2026
	got, _, err := repo.CounterSale(ctx, cmd)
	if err != nil {
		t.Fatalf("CounterSale: %v", err)
	}
	if !strings.HasPrefix(got.Order.Folio(), "U-2026-") {
		t.Errorf("folio = %q, want U-2026-…", got.Order.Folio())
	}
}
