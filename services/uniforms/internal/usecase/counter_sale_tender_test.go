package usecase_test

import (
	"context"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/uniforms/internal/domain"
	"github.com/garcia-lorca/services/uniforms/internal/ports"
	"github.com/garcia-lorca/services/uniforms/internal/usecase"
)

// T8: a garment paid with a credit note and cash. The tenders reach finance as
// they were given, against the sale's charge.
func TestCounterSalePaysWithACreditNoteAndCash(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 5)
	sale := saleFor(item, "8", 2) // 64000
	sale.Method = ""
	sale.Tenders = []usecase.Tender{
		{Method: "credit_note", AmountCents: 20000, Reference: "NC-12"},
		{Method: "cash", AmountCents: 44000},
	}

	if _, err := f.svc.CounterSale(ctx, sale); err != nil {
		t.Fatalf("CounterSale: %v", err)
	}
	if len(f.finance.tenders) != 1 {
		t.Fatalf("payments recorded = %d, want 1", len(f.finance.tenders))
	}
	got := f.finance.tenders[0]
	want := []ports.Tender{
		{Method: "credit_note", AmountCents: 20000, Reference: "NC-12"},
		{Method: "cash", AmountCents: 44000},
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("tenders = %+v, want %+v", got, want)
	}
}

// The single method of before T8 is one tender for the whole total.
func TestCounterSaleSingleMethodIsOneTender(t *testing.T) {
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 5)
	if _, err := f.svc.CounterSale(context.Background(), saleFor(item, "8", 1)); err != nil {
		t.Fatalf("CounterSale: %v", err)
	}
	if got := f.finance.tenders[0]; len(got) != 1 || got[0] != (ports.Tender{Method: "cash", AmountCents: 32000}) {
		t.Errorf("tenders = %+v", got)
	}
}

// The strict rule, checked BEFORE anything leaves the shelf: tenders that do
// not add up to the sale's total (as the shop prices it) refuse the sale.
func TestCounterSaleRefusesTendersThatDoNotAddUp(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 5)
	cases := map[string][]usecase.Tender{
		"short":          {{Method: "credit_note", AmountCents: 20000}, {Method: "cash", AmountCents: 10000}},
		"unknown method": {{Method: "vale", AmountCents: 64000}},
		"zero":           {{Method: "cash", AmountCents: 0}, {Method: "cash", AmountCents: 64000}},
		"too many":       {{Method: "cash", AmountCents: 1}, {Method: "cash", AmountCents: 1}, {Method: "cash", AmountCents: 1}, {Method: "cash", AmountCents: 1}, {Method: "cash", AmountCents: 63996}},
	}
	for name, tenders := range cases {
		t.Run(name, func(t *testing.T) {
			sale := saleFor(item, "8", 2)
			sale.IdempotencyKey = "key-" + name
			sale.Tenders = tenders
			if _, err := f.svc.CounterSale(ctx, sale); glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Fatalf("want KindInvalid, got %v", err)
			}
			stock, err := f.repo.GetStockBySize(ctx, item.ID(), "8")
			if err != nil {
				t.Fatalf("read stock: %v", err)
			}
			if stock.OnHand() != 5 {
				t.Errorf("a refused sale moved stock: on hand %d", stock.OnHand())
			}
		})
	}
}

// Review of T8: a card type that finance would refuse (on cash, or not
// debit/credit) is refused HERE, before the garments leave the shelf.
func TestCounterSaleRefusesACardTypeFinanceWouldRefuse(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 5)
	for name, tender := range map[string]usecase.Tender{
		"on cash":      {Method: "cash", AmountCents: 32000, CardType: "debit"},
		"unknown kind": {Method: "card", AmountCents: 32000, CardType: "despensa"},
	} {
		t.Run(name, func(t *testing.T) {
			sale := saleFor(item, "8", 1)
			sale.IdempotencyKey = "ct-" + name
			sale.Tenders = []usecase.Tender{tender}
			if _, err := f.svc.CounterSale(ctx, sale); glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Fatalf("want KindInvalid, got %v", err)
			}
		})
	}
}

// Review of T8: a credit note belongs to a family. A sale to nobody in
// particular cannot be paid with one.
func TestCounterSaleWithoutAStudentRefusesACreditNote(t *testing.T) {
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 5)
	sale := saleFor(item, "8", 1)
	sale.StudentID, sale.Note = "", "Tío de Zoe"
	sale.Tenders = []usecase.Tender{{Method: "credit_note", AmountCents: 32000}}
	if _, err := f.svc.CounterSale(context.Background(), sale); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("want KindInvalid, got %v", err)
	}
}

// Review of T8: a retry of a sale that already went through returns the
// original order even if the price changed in between — the tender check
// does not stand in front of the replay.
func TestARetriedSaleReplaysAfterAPriceChange(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 5)
	sale := saleFor(item, "8", 1)
	sale.Tenders = []usecase.Tender{{Method: "cash", AmountCents: 32000}}
	first, err := f.svc.CounterSale(ctx, sale)
	if err != nil {
		t.Fatalf("first sale: %v", err)
	}
	f.repricing(t, item, 35000)
	again, err := f.svc.CounterSale(ctx, sale)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if again.Order.ID() != first.Order.ID() {
		t.Errorf("retry made a new order %s, want the original %s", again.Order.ID(), first.Order.ID())
	}
}

// repricing changes a garment's catalogue price.
func (f *fixture) repricing(t *testing.T, item domain.Item, priceCents int64) {
	t.Helper()
	if _, err := f.svc.UpdateItem(context.Background(), usecase.UpdateItemInput{
		ID: item.ID(), Name: item.Name(), PriceCents: priceCents, Active: true, Level: item.Level(),
	}); err != nil {
		t.Fatalf("reprice: %v", err)
	}
}
