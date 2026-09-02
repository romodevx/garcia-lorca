package domain_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/uniforms/internal/domain"
)

func TestNewItem(t *testing.T) {
	it, err := domain.NewItem("it-1", "t-1", " SUE-VER ", " Suéter verde ", "", domain.LevelAny, 65000)
	if err != nil {
		t.Fatalf("NewItem: %v", err)
	}
	if it.Code() != "SUE-VER" || it.Name() != "Suéter verde" {
		t.Errorf("code/name not trimmed: %q / %q", it.Code(), it.Name())
	}
	if !it.Active() {
		t.Error("a new garment starts on sale")
	}
	if it.Level() != domain.LevelAny {
		t.Errorf("level = %q, want the any-level default", it.Level())
	}
}

// TestItemLevelAnyAppliesEverywhere is the shape of the requirement: the school
// MAY have a different uniform per level, but most garments are the same from
// kinder to prepa and saying so four times would be busywork.
func TestItemLevelAnyAppliesEverywhere(t *testing.T) {
	anyLevel, err := domain.NewItem("it-1", "t-1", "PLA", "Playera", "", domain.LevelAny, 32000)
	if err != nil {
		t.Fatalf("NewItem: %v", err)
	}
	kinderOnly, err := domain.NewItem("it-2", "t-1", "BAT", "Batita", "", domain.LevelKinder, 41000)
	if err != nil {
		t.Fatalf("NewItem: %v", err)
	}

	cases := []struct {
		name  string
		item  domain.Item
		level domain.Level
		want  bool
	}{
		{"any-level garment, kinder pupil", anyLevel, domain.LevelKinder, true},
		{"any-level garment, prepa pupil", anyLevel, domain.LevelPreparatoria, true},
		{"kinder garment, kinder pupil", kinderOnly, domain.LevelKinder, true},
		{"kinder garment, primaria pupil", kinderOnly, domain.LevelPrimaria, false},
		{"kinder garment, unknown level", kinderOnly, domain.LevelAny, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.item.AppliesTo(tc.level); got != tc.want {
				t.Errorf("AppliesTo(%q) = %v, want %v", tc.level, got, tc.want)
			}
		})
	}
}

// TestItemRetireKeepsIssuedOrdersWorking: a retired garment leaves the catalogue
// a parent can order from, but orders already placed still point at it and must
// still be deliverable. Deleting it would orphan them.
func TestItemRetireKeepsIssuedOrdersWorking(t *testing.T) {
	it, err := domain.NewItem("it-1", "t-1", "PLA", "Playera", "", domain.LevelAny, 32000)
	if err != nil {
		t.Fatalf("NewItem: %v", err)
	}
	retired := it.Retire()
	if retired.Active() {
		t.Error("Retire did not take it off sale")
	}
	if it.Active() != true {
		t.Error("Retire must not mutate the receiver")
	}
	if retired.Name() == "" || retired.PriceCents() == 0 {
		t.Error("a retired garment keeps its identity, so issued orders still read")
	}
}

func TestNewItemValidation(t *testing.T) {
	cases := []struct {
		name              string
		id, code, garment string
		level             domain.Level
		price             int64
	}{
		{"blank id", "", "PLA", "Playera", domain.LevelAny, 100},
		{"blank code", "it-1", "  ", "Playera", domain.LevelAny, 100},
		{"blank name", "it-1", "PLA", "  ", domain.LevelAny, 100},
		{"negative price", "it-1", "PLA", "Playera", domain.LevelAny, -1},
		{"unknown level", "it-1", "PLA", "Playera", domain.Level("universidad"), 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewItem(tc.id, "t-1", tc.code, tc.garment, "", tc.level, tc.price)
			if glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("want KindInvalid, got %v", err)
			}
		})
	}
}

// TestItemPriceIsAlwaysTheServersGuards the rule the security review made
// explicit: whatever the client sends, the amount charged comes from here.
func TestItemPriceIsAlwaysTheServers(t *testing.T) {
	it, err := domain.NewItem("it-1", "t-1", "SUE", "Suéter", "", domain.LevelAny, 65000)
	if err != nil {
		t.Fatalf("NewItem: %v", err)
	}
	line, err := it.LineFor("ln-1", "ord-1", "M", 2, domain.LineReserved)
	if err != nil {
		t.Fatalf("LineFor: %v", err)
	}
	if line.UnitPriceCents() != 65000 || line.SubtotalCents() != 130000 {
		t.Errorf("line priced at %d (subtotal %d); the catalogue says 65000",
			line.UnitPriceCents(), line.SubtotalCents())
	}
	if line.ItemName() != "Suéter" {
		t.Errorf("line name = %q; it is snapshotted from the catalogue", line.ItemName())
	}
}

// TestRetiredItemCannotStartNewOrders: retired means retired from NEW billing,
// which is the same rule the finance concept catalogue already enforces.
func TestRetiredItemCannotStartNewOrders(t *testing.T) {
	it, err := domain.NewItem("it-1", "t-1", "EXC", "Excursión 2024", "", domain.LevelAny, 50000)
	if err != nil {
		t.Fatalf("NewItem: %v", err)
	}
	if _, err := it.Retire().LineFor("ln-1", "ord-1", "M", 1, domain.LineReserved); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid ordering a retired garment, got %v", err)
	}
}

// The school sells books and whatever else it decides to sell. The module
// already does the hard parts — stock that cannot go negative, an append-only
// ledger, a counter sale that bills finance — and none of that is about
// clothing.
func TestAnItemCarriesItsCategory(t *testing.T) {
	item, err := domain.NewItem("it-1", "t1", "LIB-1", "Libro de matemáticas", "",
		domain.LevelPrimaria, 45000)
	if err != nil {
		t.Fatalf("NewItem: %v", err)
	}
	// Everything stored before this is a uniform, which is the truth about it.
	if item.Category() != domain.CategoryUniform {
		t.Errorf("category = %q, want uniform by default", item.Category())
	}

	book, err := item.WithCategory(domain.CategoryBook)
	if err != nil {
		t.Fatalf("WithCategory: %v", err)
	}
	if book.Category() != domain.CategoryBook {
		t.Errorf("category = %q, want book", book.Category())
	}
}

func TestAnUnknownCategoryIsRefused(t *testing.T) {
	item, err := domain.NewItem("it-1", "t1", "LIB-1", "Libro", "", domain.LevelPrimaria, 45000)
	if err != nil {
		t.Fatalf("NewItem: %v", err)
	}
	if _, err := item.WithCategory("mascota"); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("unknown category: want KindInvalid, got %v", err)
	}
}
