package usecase_test

import (
	"context"
	"errors"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/uniforms/internal/domain"
	"github.com/garcia-lorca/services/uniforms/internal/usecase"
)

func saleFor(item domain.Item, size string, qty int) usecase.CounterSaleInput {
	return usecase.CounterSaleInput{
		StudentID:      testStudent,
		Method:         "cash",
		ActorUserID:    testActor,
		IdempotencyKey: "key-1",
		Lines:          []usecase.SaleLineInput{{ItemID: item.ID(), SizeLabel: size, Qty: qty}},
	}
}

func TestCounterSaleHappyPath(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 5)

	got, err := f.svc.CounterSale(ctx, saleFor(item, "8", 2))
	if err != nil {
		t.Fatalf("CounterSale: %v", err)
	}

	if len(got.Fulfillments) != 1 || got.Fulfillments[0].Status() != domain.StatusSoldAtCounter {
		t.Fatalf("a counter sale must produce one terminal delivery: %+v", got.Fulfillments)
	}
	if got.Order.StudentName() != "Zoe Villalobos" || got.Order.StudentLevel() != domain.LevelPrimaria {
		t.Errorf("the student was not snapshotted onto the order: %+v", got.Order)
	}
	// Two pieces left the shelf and nothing is being held.
	stock, err := f.repo.GetStockBySize(ctx, item.ID(), "8")
	if err != nil {
		t.Fatalf("read stock: %v", err)
	}
	if stock.OnHand() != 3 || stock.Reserved() != 0 {
		t.Errorf("on hand %d, reserved %d; want 3 and 0", stock.OnHand(), stock.Reserved())
	}
	// And the money was recorded against the family, once.
	if len(f.finance.charges) != 1 || len(f.finance.payments) != 1 {
		t.Fatalf("charges %d, payments %d; want one of each", len(f.finance.charges), len(f.finance.payments))
	}
	if f.finance.charges[0].AmountCents != 64000 {
		t.Errorf("charged %d, want 64000 — two at the catalogue price", f.finance.charges[0].AmountCents)
	}
}

// TestCounterSalePriceComesFromTheCatalogue is the regression test for the
// security finding: whatever a caller sends, the amount is the shop's.
func TestCounterSalePriceComesFromTheCatalogue(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item := f.seedItem(t, "SUE", "M", 65000, 3)

	got, err := f.svc.CounterSale(ctx, saleFor(item, "M", 1))
	if err != nil {
		t.Fatalf("CounterSale: %v", err)
	}
	if got.Lines[0].UnitPriceCents() != 65000 {
		t.Errorf("line priced at %d; the catalogue says 65000", got.Lines[0].UnitPriceCents())
	}
	// The input type has no price field at all — that is the point, and this
	// asserts the shape rather than trusting a comment.
	if f.finance.charges[0].AmountCents != 65000 {
		t.Errorf("charged %d, want 65000", f.finance.charges[0].AmountCents)
	}
}

// TestCounterSaleIsIdempotent: a double click at the till, or a retried timeout,
// must not sell the same garment twice. This is the only call that moves
// physical goods and money in one act.
func TestCounterSaleIsIdempotent(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 5)

	first, err := f.svc.CounterSale(ctx, saleFor(item, "8", 1))
	if err != nil {
		t.Fatalf("first sale: %v", err)
	}
	second, err := f.svc.CounterSale(ctx, saleFor(item, "8", 1))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}

	if second.Order.ID() != first.Order.ID() {
		t.Errorf("the replay produced a different order: %q vs %q", second.Order.ID(), first.Order.ID())
	}
	stock, _ := f.repo.GetStockBySize(ctx, item.ID(), "8")
	if stock.OnHand() != 4 {
		t.Errorf("on hand = %d, want 4 — one sale, not two", stock.OnHand())
	}
	if len(f.finance.charges) != 1 {
		t.Errorf("%d charges; the family must be billed once", len(f.finance.charges))
	}
}

func TestCounterSaleRequiresAnIdempotencyKey(t *testing.T) {
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 5)
	in := saleFor(item, "8", 1)
	in.IdempotencyKey = ""

	_, err := f.svc.CounterSale(context.Background(), in)
	requireKind(t, err, glerrors.KindInvalid)
}

// TestCounterSaleCannotTakeAnotherFamilysHold: four of the five pieces are
// promised. The till may sell one, and no more — the family that has been
// waiting is not the counter's to disappoint.
func TestCounterSaleCannotTakeAnotherFamilysHold(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 5)
	stock, _ := f.repo.GetStockBySize(ctx, item.ID(), "8")
	held, err := stock.Reserve(4)
	if err != nil {
		t.Fatalf("seed a hold: %v", err)
	}
	f.repo.stock[stock.ID()] = held

	if _, err := f.svc.CounterSale(ctx, saleFor(item, "8", 1)); err != nil {
		t.Fatalf("selling the one free piece: %v", err)
	}
	in := saleFor(item, "8", 1)
	in.IdempotencyKey = "key-2"
	_, err = f.svc.CounterSale(ctx, in)
	requireKind(t, err, glerrors.KindConflict)
}

// TestCounterSaleWithoutAStudentNeedsANote is the uncle at the window. The sale
// is allowed; leaving no trace of who bought it is not, or the till reconciles
// against nothing.
func TestCounterSaleWithoutAStudentNeedsANote(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 5)

	anon := saleFor(item, "8", 1)
	anon.StudentID = ""
	if _, err := f.svc.CounterSale(ctx, anon); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("want KindInvalid with no student and no note, got %v", err)
	}

	anon.Note = "Tío de Zoe Villalobos, 3 B"
	got, err := f.svc.CounterSale(ctx, anon)
	if err != nil {
		t.Fatalf("an anonymous sale with a note must be allowed: %v", err)
	}
	if !got.Order.IsAnonymous() {
		t.Error("the order should be anonymous")
	}
	// It still moves stock, and it still records money — against the school's
	// own counter account, so the day's till adds up.
	if len(f.finance.charges) != 1 {
		t.Errorf("%d charges; an anonymous sale must still be recorded", len(f.finance.charges))
	}
	if f.finance.charges[0].StudentID != "" {
		t.Errorf("the charge named a student: %q", f.finance.charges[0].StudentID)
	}
}

func TestCounterSaleRejectsAnUnknownStudent(t *testing.T) {
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 5)
	in := saleFor(item, "8", 1)
	in.StudentID = "99999999-9999-9999-9999-999999999999"

	_, err := f.svc.CounterSale(context.Background(), in)
	requireKind(t, err, glerrors.KindNotFound)
}

func TestCounterSaleRejectsARetiredGarment(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item := f.seedItem(t, "EXC", "M", 50000, 5)
	if _, err := f.svc.UpdateItem(ctx, usecase.UpdateItemInput{
		ID: item.ID(), Name: item.Name(), PriceCents: item.PriceCents(), Active: false,
	}); err != nil {
		t.Fatalf("retire: %v", err)
	}

	_, err := f.svc.CounterSale(ctx, saleFor(item, "M", 1))
	requireKind(t, err, glerrors.KindInvalid)
}

func TestCounterSaleRejectsAnUnknownSize(t *testing.T) {
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 5)
	_, err := f.svc.CounterSale(context.Background(), saleFor(item, "14", 1))
	requireKind(t, err, glerrors.KindNotFound)
}

func TestCounterSaleRespectsTheOrderShapeCeilings(t *testing.T) {
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 50)
	// Default ceiling is five of one size; six is an argument, not a purchase.
	_, err := f.svc.CounterSale(context.Background(), saleFor(item, "8", 6))
	requireKind(t, err, glerrors.KindInvalid)
}

func TestCounterSaleRejectsAnEmptySale(t *testing.T) {
	f := newFixture(t)
	in := usecase.CounterSaleInput{StudentID: testStudent, Method: "cash", IdempotencyKey: "key-1"}
	_, err := f.svc.CounterSale(context.Background(), in)
	requireKind(t, err, glerrors.KindInvalid)
}

func TestCounterSaleRejectsAnUnknownMethod(t *testing.T) {
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 5)
	in := saleFor(item, "8", 1)
	in.Method = "trueque"
	_, err := f.svc.CounterSale(context.Background(), in)
	requireKind(t, err, glerrors.KindInvalid)
}

// TestCounterSaleFlagsWhenTheMoneyCannotBeRecorded is the crash the review
// walked through. The goods left the storeroom in the first step; finance fails
// in the second. Unwinding would be a lie about the shelf, and pretending would
// be a lie about the till, so the delivery is handed to a person — loudly, with
// the reason on it.
func TestCounterSaleFlagsWhenTheMoneyCannotBeRecorded(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 5)
	f.finance.failCharge = errors.New("finance unreachable")

	got, err := f.svc.CounterSale(ctx, saleFor(item, "8", 1))
	if err != nil {
		t.Fatalf("the sale itself must still succeed: the goods are gone: %v", err)
	}

	stock, _ := f.repo.GetStockBySize(ctx, item.ID(), "8")
	if stock.OnHand() != 4 {
		t.Errorf("on hand = %d; the deduction must stand, the garment left", stock.OnHand())
	}
	fulfillmentID := got.Fulfillments[0].ID()
	reason, flagged := f.repo.flagged[fulfillmentID]
	if !flagged {
		t.Fatal("the delivery was not flagged; a sale finance never heard about cannot be silent")
	}
	if reason == "" {
		t.Error("the flag carries no reason, which tells the staff nothing")
	}
}

// TestCounterSaleFlagsWhenThePaymentCannotBeRecorded is the same crash one step
// later: the charge exists, so the family now owes for something they already
// paid for and carried out. That is worse than the previous case, not better,
// and it must be just as loud.
func TestCounterSaleFlagsWhenThePaymentCannotBeRecorded(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 5)
	f.finance.failPayment = errors.New("finance unreachable")

	got, err := f.svc.CounterSale(ctx, saleFor(item, "8", 1))
	if err != nil {
		t.Fatalf("the sale itself must still succeed: %v", err)
	}
	if _, flagged := f.repo.flagged[got.Fulfillments[0].ID()]; !flagged {
		t.Error("a charge raised but never paid must be flagged")
	}
	if len(f.finance.charges) != 1 {
		t.Errorf("%d charges; the charge was raised and should be recorded as such", len(f.finance.charges))
	}
}

// TestCounterSaleRecordsTheChargeOnTheDelivery closes the loop: on the happy
// path the delivery carries its charge, which is what later reconciliation and
// any refund would need to find it.
func TestCounterSaleRecordsTheChargeOnTheDelivery(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 5)

	got, err := f.svc.CounterSale(ctx, saleFor(item, "8", 1))
	if err != nil {
		t.Fatalf("CounterSale: %v", err)
	}
	if f.repo.chargesRecorded[got.Fulfillments[0].ID()] == "" {
		t.Error("the delivery does not carry its charge id")
	}
}

// TestCounterSaleUsesTheDeliveryAsTheChargesExternalRef ties the two ledgers
// together: it is what makes raising the charge idempotent (ADR 0008) and what
// lets reconciliation ask finance about a delivery by name.
func TestCounterSaleUsesTheDeliveryAsTheChargesExternalRef(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 5)

	got, err := f.svc.CounterSale(ctx, saleFor(item, "8", 1))
	if err != nil {
		t.Fatalf("CounterSale: %v", err)
	}
	want := "uniforms:" + got.Fulfillments[0].ID()
	if f.finance.charges[0].ExternalRef != want {
		t.Errorf("external ref = %q, want %q", f.finance.charges[0].ExternalRef, want)
	}
}
