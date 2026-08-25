package usecase_test

import (
	"context"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/uniforms/internal/domain"
	"github.com/garcia-lorca/services/uniforms/internal/ports"
	"github.com/garcia-lorca/services/uniforms/internal/usecase"
)

func TestCreateItem(t *testing.T) {
	f := newFixture(t)
	got, err := f.svc.CreateItem(context.Background(), usecase.CreateItemInput{
		Code: "SUE-VER", Name: "Suéter verde", PriceCents: 65000,
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	if !got.Active() || got.Level() != domain.LevelAny {
		t.Errorf("a new garment is on sale and applies to every level: %+v", got)
	}
}

func TestCreateItemRejectsADuplicateCode(t *testing.T) {
	f := newFixture(t)
	in := usecase.CreateItemInput{Code: "PLA", Name: "Playera", PriceCents: 32000}
	if _, err := f.svc.CreateItem(context.Background(), in); err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err := f.svc.CreateItem(context.Background(), in)
	requireKind(t, err, glerrors.KindConflict)
}

func TestCreateItemValidation(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.CreateItem(context.Background(), usecase.CreateItemInput{Code: "", Name: "x", PriceCents: 1})
	requireKind(t, err, glerrors.KindInvalid)
}

// TestUpdateItemKeepsThePriceOfIssuedOrders: repricing changes what the NEXT
// order costs. The one already placed keeps what it was quoted, because its line
// snapshotted the amount.
func TestUpdateItemKeepsThePriceOfIssuedOrders(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item := f.seedItem(t, "SUE", "M", 65000, 5)

	sold, err := f.svc.CounterSale(ctx, saleFor(item, "M", 1))
	if err != nil {
		t.Fatalf("sale at the old price: %v", err)
	}

	if _, err := f.svc.UpdateItem(ctx, usecase.UpdateItemInput{
		ID: item.ID(), Name: item.Name(), PriceCents: 99000, Active: true,
	}); err != nil {
		t.Fatalf("reprice: %v", err)
	}

	if sold.Lines[0].UnitPriceCents() != 65000 {
		t.Errorf("the issued line now reads %d; it must keep the quoted 65000", sold.Lines[0].UnitPriceCents())
	}
}

func TestUpdateItemOnAMissingGarment(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.UpdateItem(context.Background(), usecase.UpdateItemInput{
		ID: "nope", Name: "x", PriceCents: 1, Active: true,
	})
	requireKind(t, err, glerrors.KindNotFound)
}

// TestListItemsForALevelIncludesTheAnyLevelGarments is what the family app asks
// for: everything a pupil of this level wears, which is the garments scoped to
// it PLUS the ones scoped to none.
func TestListItemsForALevelIncludesTheAnyLevelGarments(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	if _, err := f.svc.CreateItem(ctx, usecase.CreateItemInput{Code: "PLA", Name: "Playera", PriceCents: 32000}); err != nil {
		t.Fatalf("seed any-level: %v", err)
	}
	if _, err := f.svc.CreateItem(ctx, usecase.CreateItemInput{
		Code: "BAT", Name: "Batita", Level: domain.LevelKinder, PriceCents: 41000,
	}); err != nil {
		t.Fatalf("seed kinder: %v", err)
	}

	kinder, err := f.svc.ListItems(ctx, ports.ListItemsFilter{Level: domain.LevelKinder})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(kinder) != 2 {
		t.Errorf("a kinder pupil sees %d garments, want 2 (its own plus the universal one)", len(kinder))
	}

	primaria, err := f.svc.ListItems(ctx, ports.ListItemsFilter{Level: domain.LevelPrimaria})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(primaria) != 1 {
		t.Errorf("a primaria pupil sees %d garments, want 1", len(primaria))
	}
}

func TestListItemsHidesRetiredUnlessAsked(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item := f.seedItem(t, "EXC", "M", 50000, 0)
	if _, err := f.svc.UpdateItem(ctx, usecase.UpdateItemInput{
		ID: item.ID(), Name: item.Name(), PriceCents: item.PriceCents(), Active: false,
	}); err != nil {
		t.Fatalf("retire: %v", err)
	}

	visible, err := f.svc.ListItems(ctx, ports.ListItemsFilter{})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(visible) != 0 {
		t.Errorf("a retired garment is still on the list: %d", len(visible))
	}

	all, err := f.svc.ListItems(ctx, ports.ListItemsFilter{IncludeInactive: true})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("asking for retired ones returned %d", len(all))
	}
}

func TestListItemsRejectsAnUnknownLevel(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.ListItems(context.Background(), ports.ListItemsFilter{Level: domain.Level("universidad")})
	requireKind(t, err, glerrors.KindInvalid)
}

// TestUpsertSizesReportsCreatedAndUpdatedSeparately is what lets the panel say
// "8 sizes created, 2 already existed" instead of upserting in silence and
// leaving the user guessing what just happened.
func TestUpsertSizesReportsCreatedAndUpdatedSeparately(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item, err := f.svc.CreateItem(ctx, usecase.CreateItemInput{Code: "PAN", Name: "Pantalón", PriceCents: 48000})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}

	created, updated, _, err := f.svc.UpsertSizes(ctx, item.ID(), []ports.SizeSpec{
		{SizeLabel: "4", SizeOrder: 1}, {SizeLabel: "6", SizeOrder: 2}, {SizeLabel: "8", SizeOrder: 3},
	})
	if err != nil {
		t.Fatalf("UpsertSizes: %v", err)
	}
	if created != 3 || updated != 0 {
		t.Fatalf("created %d, updated %d; want 3 and 0", created, updated)
	}

	created, updated, sizes, err := f.svc.UpsertSizes(ctx, item.ID(), []ports.SizeSpec{
		{SizeLabel: "8", SizeOrder: 3, MinStock: 5}, {SizeLabel: "10", SizeOrder: 4},
	})
	if err != nil {
		t.Fatalf("second UpsertSizes: %v", err)
	}
	if created != 1 || updated != 1 {
		t.Errorf("created %d, updated %d; want 1 and 1", created, updated)
	}
	if len(sizes) != 4 {
		t.Errorf("the garment has %d sizes, want 4", len(sizes))
	}
}

func TestUpsertSizesValidation(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item := f.seedItem(t, "PAN", "8", 48000, 0)

	cases := []struct {
		name  string
		item  string
		specs []ports.SizeSpec
	}{
		{"no item", "", []ports.SizeSpec{{SizeLabel: "8"}}},
		{"no sizes", item.ID(), nil},
		{"blank label", item.ID(), []ports.SizeSpec{{SizeLabel: "  "}}},
		{"the same size twice", item.ID(), []ports.SizeSpec{{SizeLabel: "8"}, {SizeLabel: "8"}}},
		{"negative minimum", item.ID(), []ports.SizeSpec{{SizeLabel: "10", MinStock: -1}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := f.svc.UpsertSizes(ctx, tc.item, tc.specs)
			requireKind(t, err, glerrors.KindInvalid)
		})
	}
}

// --- inventory ---------------------------------------------------------------

func TestReceiveStock(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 0)
	stock, _ := f.repo.GetStockBySize(ctx, item.ID(), "8")

	got, _, err := f.svc.ReceiveStock(ctx, []ports.ReceiveLine{{StockID: stock.ID(), Qty: 12}}, "Factura 8871", testActor)
	if err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}
	if got[0].OnHand() != 12 {
		t.Errorf("on hand = %d, want 12", got[0].OnHand())
	}
}

func TestReceiveStockValidation(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 0)
	stock, _ := f.repo.GetStockBySize(ctx, item.ID(), "8")

	cases := []struct {
		name  string
		lines []ports.ReceiveLine
	}{
		{"nothing to receive", nil},
		{"no size named", []ports.ReceiveLine{{StockID: "", Qty: 1}}},
		{"zero quantity", []ports.ReceiveLine{{StockID: stock.ID(), Qty: 0}}},
		{"negative quantity", []ports.ReceiveLine{{StockID: stock.ID(), Qty: -3}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := f.svc.ReceiveStock(ctx, tc.lines, "nota", testActor)
			requireKind(t, err, glerrors.KindInvalid)
		})
	}
}

// TestAdjustStockDemandsAReason: this is the one write that can conceal a
// shortfall, and the ledger entry it leaves is worthless without one.
func TestAdjustStockDemandsAReason(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 10)
	stock, _ := f.repo.GetStockBySize(ctx, item.ID(), "8")

	if _, err := f.svc.AdjustStock(ctx, stock.ID(), -2, "", testActor); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid with no reason, got %v", err)
	}
	if _, err := f.svc.AdjustStock(ctx, stock.ID(), -2, "ok", testActor); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("two characters is not a reason, got %v", err)
	}

	got, err := f.svc.AdjustStock(ctx, stock.ID(), -2, "dos piezas dañadas en bodega", testActor)
	if err != nil {
		t.Fatalf("AdjustStock: %v", err)
	}
	if got.OnHand() != 8 {
		t.Errorf("on hand = %d, want 8", got.OnHand())
	}
}

func TestAdjustStockCannotStripAFamilysHold(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 5)
	stock, _ := f.repo.GetStockBySize(ctx, item.ID(), "8")
	held, err := stock.Reserve(4)
	if err != nil {
		t.Fatalf("seed a hold: %v", err)
	}
	f.repo.stock[stock.ID()] = held

	// Correcting down to two would leave four promised out of two on the shelf.
	_, err = f.svc.AdjustStock(ctx, stock.ID(), -3, "conteo físico", testActor)
	requireKind(t, err, glerrors.KindConflict)
}

func TestListStockBelowMinimum(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 1)
	if _, _, _, err := f.svc.UpsertSizes(ctx, item.ID(), []ports.SizeSpec{{SizeLabel: "8", SizeOrder: 1, MinStock: 5}}); err != nil {
		t.Fatalf("set a minimum: %v", err)
	}

	low, err := f.svc.ListStock(ctx, item.ID(), true)
	if err != nil {
		t.Fatalf("ListStock: %v", err)
	}
	if len(low) != 1 {
		t.Errorf("%d sizes below minimum, want 1", len(low))
	}
}

// --- settings ----------------------------------------------------------------

func TestSettingsRoundTrip(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)

	def, err := f.svc.GetSettings(ctx)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if def.PickupWindowBusinessDays() != 10 {
		t.Errorf("default window = %d, want 10", def.PickupWindowBusinessDays())
	}

	next, err := domain.NewSettings(testTenant, 15, false, 1, 3, 10, 3, 1)
	if err != nil {
		t.Fatalf("NewSettings: %v", err)
	}
	saved, err := f.svc.UpdateSettings(ctx, next)
	if err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if saved.PickupWindowBusinessDays() != 15 || saved.AutoCancelEnabled() {
		t.Errorf("settings did not round-trip: %+v", saved)
	}
}

// TestUpdatedCeilingsBindTheNextSale proves the settings are not decoration: a
// school that lowers the units-per-line ceiling has it enforced immediately.
func TestUpdatedCeilingsBindTheNextSale(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	item := f.seedItem(t, "PLA", "8", 32000, 50)

	tighter, err := domain.NewSettings(testTenant, 10, true, 2, 5, 20, 2, 2)
	if err != nil {
		t.Fatalf("NewSettings: %v", err)
	}
	if _, err := f.svc.UpdateSettings(ctx, tighter); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	_, err = f.svc.CounterSale(ctx, saleFor(item, "8", 3))
	requireKind(t, err, glerrors.KindInvalid)
}
