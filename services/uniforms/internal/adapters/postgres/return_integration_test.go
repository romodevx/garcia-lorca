//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/garcia-lorca/services/uniforms/internal/domain"
	"github.com/garcia-lorca/services/uniforms/internal/ports"
)

// returnOf builds the command the use case would: the sold delivery and its
// lines turned returned, with the record.
func returnOf(t *testing.T, sale ports.OrderAggregate, damaged map[string]bool) ports.ReturnCounterSaleCommand {
	t.Helper()
	f, err := sale.Fulfillments[0].ReturnAtCounter()
	if err != nil {
		t.Fatalf("ReturnAtCounter: %v", err)
	}
	lines := make([]domain.OrderLine, 0, len(sale.Lines))
	for _, l := range sale.Lines {
		back, err := l.Return()
		if err != nil {
			t.Fatalf("line Return: %v", err)
		}
		lines = append(lines, back)
	}
	return ports.ReturnCounterSaleCommand{
		Fulfillment: f, Lines: lines, Damaged: damaged, Folio: sale.Order.Folio(),
		Record: ports.CounterSaleReturn{
			FulfillmentID: f.ID(), OrderID: sale.Order.ID(), Reason: "Talla equivocada",
			RefundMode: ports.RefundVoid, RefundCents: f.AmountCents(), RefundStatus: ports.RefundDone,
			ActorName: "Director",
		},
	}
}

func onHandOf(t *testing.T, stockID string) int {
	t.Helper()
	var n int
	if err := superDB.Raw(`SELECT on_hand FROM uniform_stock WHERE id = ?`, stockID).Scan(&n).Error; err != nil {
		t.Fatalf("read on_hand: %v", err)
	}
	return n
}

// TestReturnCounterSalePutsTheGarmentsBack: C2-015 — the units come back to
// the shelf with a 'return' move, the delivery and lines read returned, the
// record is kept; a retry does not put them back twice.
func TestReturnCounterSalePutsTheGarmentsBack(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	item, stock := seedGarment(t, repo, "RET-OK", "8", 25000, 5)
	sale, _, err := repo.CounterSale(ctx, saleCmd(t, item, "8", 2, "ret-ok"))
	if err != nil {
		t.Fatalf("CounterSale: %v", err)
	}
	if got := onHandOf(t, stock.ID()); got != 3 {
		t.Fatalf("after the sale on_hand = %d, want 3", got)
	}

	cmd := returnOf(t, sale, nil)
	got, err := repo.ReturnCounterSale(ctx, cmd)
	if err != nil {
		t.Fatalf("ReturnCounterSale: %v", err)
	}

	if onHandOf(t, stock.ID()) != 5 {
		t.Errorf("on_hand = %d, want the 2 units back (5)", onHandOf(t, stock.ID()))
	}
	if got.Fulfillments[0].Status() != domain.StatusReturned || got.Lines[0].Status() != domain.LineReturned {
		t.Errorf("delivery %q line %q, want both returned", got.Fulfillments[0].Status(), got.Lines[0].Status())
	}
	rec, ok := got.Returns[cmd.Fulfillment.ID()]
	if !ok || rec.Reason != "Talla equivocada" || rec.RefundMode != ports.RefundVoid {
		t.Errorf("record = %+v, %v", rec, ok)
	}
	var moves int
	if err := superDB.Raw(`SELECT count(*) FROM uniform_stock_moves WHERE stock_id = ? AND kind = 'return' AND qty = 2`, stock.ID()).Scan(&moves).Error; err != nil || moves != 1 {
		t.Errorf("return moves = %d (%v), want one of +2", moves, err)
	}

	if _, err := repo.ReturnCounterSale(ctx, cmd); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if onHandOf(t, stock.ID()) != 5 {
		t.Errorf("a retry put the garments back twice: on_hand = %d", onHandOf(t, stock.ID()))
	}
}

// TestReturnCounterSaleWritesOffDamagedGarments: a garment that came back
// damaged is on the ledger as returned AND written off; it is not for sale.
func TestReturnCounterSaleWritesOffDamagedGarments(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	item, stock := seedGarment(t, repo, "RET-DMG", "8", 25000, 4)
	sale, _, err := repo.CounterSale(ctx, saleCmd(t, item, "8", 1, "ret-dmg"))
	if err != nil {
		t.Fatalf("CounterSale: %v", err)
	}

	cmd := returnOf(t, sale, map[string]bool{sale.Lines[0].ID(): true})
	cmd.Record.DamagedUnits = 1
	if _, err := repo.ReturnCounterSale(ctx, cmd); err != nil {
		t.Fatalf("ReturnCounterSale: %v", err)
	}

	if got := onHandOf(t, stock.ID()); got != 3 {
		t.Errorf("on_hand = %d, want 3: the damaged one is not back on sale", got)
	}
	var kinds []string
	if err := superDB.Raw(`SELECT kind FROM uniform_stock_moves WHERE stock_id = ? AND order_line_id = ? ORDER BY created_at, kind`,
		stock.ID(), sale.Lines[0].ID()).Scan(&kinds).Error; err != nil {
		t.Fatalf("read moves: %v", err)
	}
	want := map[string]bool{"sell": true, "return": true, "adjust": true}
	for _, k := range kinds {
		delete(want, k)
	}
	if len(want) != 0 {
		t.Errorf("ledger kinds %v, missing %v", kinds, want)
	}
}

// TestSettleReturnRefundRecordsTheExpense: a refund of a later day is pending
// until its expense exists, then done.
func TestSettleReturnRefundRecordsTheExpense(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	item, _ := seedGarment(t, repo, "RET-EXP", "8", 25000, 2)
	sale, _, err := repo.CounterSale(ctx, saleCmd(t, item, "8", 1, "ret-exp"))
	if err != nil {
		t.Fatalf("CounterSale: %v", err)
	}
	cmd := returnOf(t, sale, nil)
	cmd.Record.RefundMode, cmd.Record.RefundMethod, cmd.Record.RefundStatus = ports.RefundExpense, "cash", ports.RefundPending
	if _, err := repo.ReturnCounterSale(ctx, cmd); err != nil {
		t.Fatalf("ReturnCounterSale: %v", err)
	}

	const expenseID = "e0000000-0000-0000-0000-00000000c215"
	if err := repo.SettleReturnRefund(ctx, cmd.Fulfillment.ID(), expenseID); err != nil {
		t.Fatalf("SettleReturnRefund: %v", err)
	}
	got, err := repo.GetOrder(ctx, sale.Order.ID(), nil)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if rec := got.Returns[cmd.Fulfillment.ID()]; rec.RefundStatus != ports.RefundDone || rec.ExpenseID != expenseID {
		t.Errorf("record = %+v, want done with the expense", rec)
	}
}

// Review (PR #162): the decision is stored before money moves and the first
// one wins; applying the return afterwards keeps it; a decision nothing acted
// on can be dropped while the sale is still sold.
func TestBeginReturnStoresTheFirstDecision(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	item, _ := seedGarment(t, repo, "RET-INT", "8", 25000, 3)
	sale, _, err := repo.CounterSale(ctx, saleCmd(t, item, "8", 1, "ret-int"))
	if err != nil {
		t.Fatalf("CounterSale: %v", err)
	}
	cmd := returnOf(t, sale, nil)

	first, err := repo.BeginReturn(ctx, cmd.Record)
	if err != nil || first.RefundMode != ports.RefundVoid {
		t.Fatalf("BeginReturn = %+v, %v", first, err)
	}
	other := cmd.Record
	other.RefundMode, other.RefundMethod, other.RefundStatus = ports.RefundExpense, "cash", ports.RefundPending
	again, err := repo.BeginReturn(ctx, other)
	if err != nil || again.RefundMode != ports.RefundVoid {
		t.Fatalf("second BeginReturn = %+v, %v; the first decision must win", again, err)
	}

	if err := repo.DropReturnIntent(ctx, cmd.Fulfillment.ID()); err != nil {
		t.Fatalf("DropReturnIntent: %v", err)
	}
	if got, _ := repo.GetOrder(ctx, sale.Order.ID(), nil); len(got.Returns) != 0 {
		t.Fatalf("intent survived the drop: %+v", got.Returns)
	}

	if _, err := repo.BeginReturn(ctx, cmd.Record); err != nil {
		t.Fatalf("BeginReturn again: %v", err)
	}
	if _, err := repo.ReturnCounterSale(ctx, cmd); err != nil {
		t.Fatalf("ReturnCounterSale after the intent: %v", err)
	}
	if err := repo.DropReturnIntent(ctx, cmd.Fulfillment.ID()); err != nil {
		t.Fatalf("DropReturnIntent on a returned sale: %v", err)
	}
	got, _ := repo.GetOrder(ctx, sale.Order.ID(), nil)
	if _, kept := got.Returns[cmd.Fulfillment.ID()]; !kept {
		t.Error("a returned sale lost its record to a drop")
	}
}

// Settling a refund twice (two clicks both finishing it) is not an error and
// keeps the first expense.
func TestSettleReturnRefundTwiceKeepsTheFirstExpense(t *testing.T) {
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	item, _ := seedGarment(t, repo, "RET-SET2", "8", 25000, 2)
	sale, _, err := repo.CounterSale(ctx, saleCmd(t, item, "8", 1, "ret-set2"))
	if err != nil {
		t.Fatalf("CounterSale: %v", err)
	}
	cmd := returnOf(t, sale, nil)
	cmd.Record.RefundMode, cmd.Record.RefundMethod, cmd.Record.RefundStatus = ports.RefundExpense, "cash", ports.RefundPending
	if _, err := repo.ReturnCounterSale(ctx, cmd); err != nil {
		t.Fatalf("ReturnCounterSale: %v", err)
	}

	if err := repo.SettleReturnRefund(ctx, cmd.Fulfillment.ID(), "e0000000-0000-0000-0000-00000000c301"); err != nil {
		t.Fatalf("first settle: %v", err)
	}
	if err := repo.SettleReturnRefund(ctx, cmd.Fulfillment.ID(), "e0000000-0000-0000-0000-00000000c302"); err != nil {
		t.Fatalf("second settle: %v", err)
	}
	got, _ := repo.GetOrder(ctx, sale.Order.ID(), nil)
	if rec := got.Returns[cmd.Fulfillment.ID()]; rec.ExpenseID != "e0000000-0000-0000-0000-00000000c301" {
		t.Errorf("expense = %q, want the first", rec.ExpenseID)
	}
}
