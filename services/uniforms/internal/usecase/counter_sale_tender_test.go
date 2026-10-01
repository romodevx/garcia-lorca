package usecase_test

import (
	"context"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
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
