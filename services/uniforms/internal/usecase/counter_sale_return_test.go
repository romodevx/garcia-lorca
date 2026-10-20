package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/uniforms/internal/domain"
	"github.com/garcia-lorca/services/uniforms/internal/ports"
	"github.com/garcia-lorca/services/uniforms/internal/usecase"
)

// soldToday sells one garment at fixedNow and returns the sale and its stock.
func soldToday(t *testing.T, f *fixture) (ports.OrderAggregate, domain.StockLevel) {
	t.Helper()
	item := f.seedItem(t, "PLA", "8", 25000, 5)
	sale, err := f.svc.CounterSale(context.Background(), saleFor(item, "8", 1))
	if err != nil {
		t.Fatalf("CounterSale: %v", err)
	}
	stock, err := f.repo.GetStockBySize(context.Background(), item.ID(), "8")
	if err != nil {
		t.Fatalf("stock: %v", err)
	}
	return sale, stock
}

func returnInput(orderID string) usecase.ReturnCounterSaleInput {
	return usecase.ReturnCounterSaleInput{
		OrderID: orderID, Reason: "Talla equivocada", ActorUserID: testActor, ActorName: "Director",
	}
}

// laterDay rebuilds the service one day after the sale, as tomorrow's till.
func laterDay(f *fixture) {
	f.svc = usecase.NewService(f.repo, f.students, f.finance, f.pub, testTenant,
		seqID("ret"), func() time.Time { return fixedNow.Add(24 * time.Hour) }, time.UTC)
}

// C2-015: returned the day it was sold, the sale's payments are voided in
// finance and the garment is back on the shelf.
func TestReturnTheSameDayVoidsThePayments(t *testing.T) {
	f := newFixture(t)
	sale, stock := soldToday(t, f)

	got, err := f.svc.ReturnCounterSale(context.Background(), returnInput(sale.Order.ID()))
	if err != nil {
		t.Fatalf("ReturnCounterSale: %v", err)
	}

	if len(f.finance.voided) != 1 || f.finance.voided[0] != f.repo.chargesRecorded[sale.Fulfillments[0].ID()] {
		t.Errorf("voided %v, want the sale's charge", f.finance.voided)
	}
	if len(f.finance.refunds) != 0 {
		t.Errorf("a same-day return also recorded an expense: %+v", f.finance.refunds)
	}
	rec := got.Returns[sale.Fulfillments[0].ID()]
	if rec.RefundMode != ports.RefundVoid || rec.RefundStatus != ports.RefundDone || rec.Reason != "Talla equivocada" {
		t.Errorf("record = %+v", rec)
	}
	if got.Fulfillments[0].Status() != domain.StatusReturned {
		t.Errorf("status = %q, want returned", got.Fulfillments[0].Status())
	}
	if back := f.repo.stock[stock.ID()]; back.OnHand() != stock.OnHand()+1 {
		t.Errorf("on hand %d, want %d", back.OnHand(), stock.OnHand()+1)
	}
}

// Another day the sale's day is not rewritten: the refund is an expense of
// today, by the method the money goes back.
func TestReturnAnotherDayIsAnExpenseOfThatDay(t *testing.T) {
	f := newFixture(t)
	sale, _ := soldToday(t, f)
	laterDay(f)
	in := returnInput(sale.Order.ID())
	in.RefundMethod = "transfer"

	got, err := f.svc.ReturnCounterSale(context.Background(), in)
	if err != nil {
		t.Fatalf("ReturnCounterSale: %v", err)
	}

	if len(f.finance.voided) != 0 {
		t.Errorf("an earlier day's sale was voided: %v", f.finance.voided)
	}
	if len(f.finance.refunds) != 1 {
		t.Fatalf("refunds = %+v, want one expense", f.finance.refunds)
	}
	r := f.finance.refunds[0]
	if r.AmountCents != 25000 || r.Method != "transfer" || r.SpentOn != fixedNow.Add(24*time.Hour).Format("2006-01-02") || r.ActorUserID != testActor {
		t.Errorf("refund = %+v", r)
	}
	if rec := got.Returns[sale.Fulfillments[0].ID()]; rec.RefundMode != ports.RefundExpense || rec.RefundStatus != ports.RefundDone || rec.ExpenseID != "exp-1" {
		t.Errorf("record = %+v", rec)
	}
}

func TestReturnAnotherDayNeedsHowTheMoneyGoesBack(t *testing.T) {
	f := newFixture(t)
	sale, _ := soldToday(t, f)
	laterDay(f)

	_, err := f.svc.ReturnCounterSale(context.Background(), returnInput(sale.Order.ID()))

	requireKind(t, err, glerrors.KindInvalid)
}

func TestReturnNeedsAReason(t *testing.T) {
	f := newFixture(t)
	sale, _ := soldToday(t, f)
	in := returnInput(sale.Order.ID())
	in.Reason = "  "

	_, err := f.svc.ReturnCounterSale(context.Background(), in)

	requireKind(t, err, glerrors.KindInvalid)
	if len(f.finance.voided) != 0 || len(f.repo.returned) != 0 {
		t.Error("a refused return moved money or goods")
	}
}

// A garment that came back damaged is written off; the others go back on sale.
func TestReturnWritesOffWhatCameBackDamaged(t *testing.T) {
	f := newFixture(t)
	sale, stock := soldToday(t, f)
	in := returnInput(sale.Order.ID())
	in.DamagedLineIDs = []string{sale.Lines[0].ID()}

	if _, err := f.svc.ReturnCounterSale(context.Background(), in); err != nil {
		t.Fatalf("ReturnCounterSale: %v", err)
	}

	cmd := f.repo.returned[sale.Fulfillments[0].ID()]
	if !cmd.Damaged[sale.Lines[0].ID()] || cmd.Record.DamagedUnits != 1 {
		t.Errorf("damaged = %v units %d", cmd.Damaged, cmd.Record.DamagedUnits)
	}
	if back := f.repo.stock[stock.ID()]; back.OnHand() != stock.OnHand() {
		t.Errorf("a damaged garment went back on sale: on hand %d", back.OnHand())
	}
}

func TestReturnRefusesALineOfAnotherSale(t *testing.T) {
	f := newFixture(t)
	sale, _ := soldToday(t, f)
	in := returnInput(sale.Order.ID())
	in.DamagedLineIDs = []string{"not-a-line-of-this-sale"}

	_, err := f.svc.ReturnCounterSale(context.Background(), in)

	requireKind(t, err, glerrors.KindInvalid)
}

// If finance refuses the void the garments stay sold: nothing half-happens.
func TestReturnWhoseVoidFailsChangesNothing(t *testing.T) {
	f := newFixture(t)
	sale, _ := soldToday(t, f)
	f.finance.failVoid = glerrors.New(glerrors.KindInternal, "finance down")

	_, err := f.svc.ReturnCounterSale(context.Background(), returnInput(sale.Order.ID()))

	if err == nil {
		t.Fatal("want the failure surfaced")
	}
	if len(f.repo.returned) != 0 {
		t.Error("the garments came back although the money did not")
	}
}

// The goods are back but the expense failed: the delivery is flagged and the
// refund pending; pressing "Devolver" again records it.
func TestReturnWithAFailedExpenseIsPendingAndARetryFinishesIt(t *testing.T) {
	f := newFixture(t)
	sale, _ := soldToday(t, f)
	laterDay(f)
	f.finance.failRefund = errors.New("finance down")
	in := returnInput(sale.Order.ID())
	in.RefundMethod = "cash"

	got, err := f.svc.ReturnCounterSale(context.Background(), in)
	if err != nil {
		t.Fatalf("ReturnCounterSale: %v", err)
	}
	fid := sale.Fulfillments[0].ID()
	if got.Returns[fid].RefundStatus != ports.RefundPending || f.repo.flagged[fid] == "" {
		t.Fatalf("record %+v flagged %q, want pending and flagged", got.Returns[fid], f.repo.flagged[fid])
	}

	f.finance.failRefund = nil
	again, err := f.svc.ReturnCounterSale(context.Background(), in)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if again.Returns[fid].RefundStatus != ports.RefundDone || len(f.finance.refunds) != 1 {
		t.Errorf("after retry %+v refunds %d, want done with one expense", again.Returns[fid], len(f.finance.refunds))
	}
}
