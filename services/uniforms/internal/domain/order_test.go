package domain_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/uniforms/internal/domain"
)

func TestFolioFormat(t *testing.T) {
	got := domain.FormatFolio(2026, 123)
	if got != "U-2026-000123" {
		t.Errorf("FormatFolio(2026, 123) = %q, want U-2026-000123", got)
	}
}

// TestLooksLikeFolio is what lets one search box take either a folio or a name
// without asking the user which they meant.
func TestLooksLikeFolio(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"U-2026-000123", true},
		{"U-2026-000123-A", true},
		{"U-2026-000123-AA", true},
		{"u-2026-000123", true}, // typed in a hurry, still a folio
		{"Zoe Villalobos", false},
		{"U-26-123", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := domain.LooksLikeFolio(tc.in); got != tc.want {
			t.Errorf("LooksLikeFolio(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func line(t *testing.T, status domain.LineStatus) domain.OrderLine {
	t.Helper()
	l, err := domain.NewOrderLine("ln-1", "t-1", "ord-1", "it-1", "Playera", "8", 1, 32000, status)
	if err != nil {
		t.Fatalf("NewOrderLine(%q): %v", status, err)
	}
	return l
}

// TestLineHoldsStockOnlyWhenReserved: reserved is the only status that keeps a
// unit committed. sold means it left; released and cancelled mean it went back.
func TestLineHoldsStockOnlyWhenReserved(t *testing.T) {
	cases := []struct {
		status domain.LineStatus
		holds  bool
	}{
		{domain.LineAwaitingStock, false},
		{domain.LineReserved, true},
		{domain.LineSold, false},
		{domain.LineReleased, false},
		{domain.LineCancelled, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			if got := tc.status.HoldsStock(); got != tc.holds {
				t.Errorf("HoldsStock() = %v, want %v", got, tc.holds)
			}
		})
	}
}

// TestReleasedLineIsNotReReserved is the rule the review forced into the open. A
// line the school reclaimed is dead: the goods went back on the shelf and may
// already have been sold to somebody else. The family orders again — which is
// honest — rather than the system quietly re-promising something it may not have.
func TestReleasedLineIsNotReReserved(t *testing.T) {
	released := line(t, domain.LineReleased)
	if _, err := released.Reserve(); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("want KindConflict re-reserving a released line, got %v", err)
	}
	cancelled := line(t, domain.LineCancelled)
	if _, err := cancelled.Reserve(); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("want KindConflict reserving a cancelled line, got %v", err)
	}
}

func TestAwaitingLineIsReservedWhenStockArrives(t *testing.T) {
	waiting := line(t, domain.LineAwaitingStock)
	got, err := waiting.Reserve()
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if got.Status() != domain.LineReserved {
		t.Errorf("status = %q, want reserved", got.Status())
	}
}

// TestLineExpiryCountRises: without a ceiling, a family can hold the last unit
// of a size for ten working days, let it lapse, order it again and repeat, for
// free and forever.
func TestLineExpiryCountRises(t *testing.T) {
	reserved := line(t, domain.LineReserved)
	released, err := reserved.Release()
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if released.ExpiryCount() != 1 {
		t.Errorf("expiry count = %d, want 1", released.ExpiryCount())
	}
	if released.Status() != domain.LineReleased {
		t.Errorf("status = %q, want released", released.Status())
	}
}

func TestLineCancelOnlyBeforeItHoldsAnything(t *testing.T) {
	waiting := line(t, domain.LineAwaitingStock)
	got, err := waiting.Cancel()
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if got.Status() != domain.LineCancelled {
		t.Errorf("status = %q, want cancelled", got.Status())
	}

	// A reserved line holds goods; letting it go is a release, not a
	// cancellation, and it counts against the family's lapse budget.
	reserved := line(t, domain.LineReserved)
	if _, err := reserved.Cancel(); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("want KindConflict cancelling a reserved line, got %v", err)
	}
}

func TestLineSellNeedsAReservation(t *testing.T) {
	if _, err := line(t, domain.LineAwaitingStock).Sell(); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Error("a line with nothing held cannot be sold")
	}
	sold, err := line(t, domain.LineReserved).Sell()
	if err != nil {
		t.Fatalf("Sell: %v", err)
	}
	if sold.Status() != domain.LineSold {
		t.Errorf("status = %q, want sold", sold.Status())
	}
}

// TestDeriveOrderLabel is the table that stops a panel from lying. The case that
// matters is the last one: an order that ended having delivered only part of what
// was asked for must NOT read as plain "Cerrado", or the worst outcome the module
// can produce looks exactly like the best one.
func TestDeriveOrderLabel(t *testing.T) {
	cases := []struct {
		name  string
		lines []domain.LineStatus
		live  bool // at least one delivery still holding
		want  domain.OrderLabel
	}{
		{"everything waiting on the supplier", []domain.LineStatus{domain.LineAwaitingStock, domain.LineAwaitingStock}, false, domain.LabelAwaitingStock},
		{"some picked, some still waiting", []domain.LineStatus{domain.LineReserved, domain.LineAwaitingStock}, true, domain.LabelPartial},
		{"all picked, none collected yet", []domain.LineStatus{domain.LineReserved, domain.LineReserved}, true, domain.LabelInProgress},
		{"everything delivered", []domain.LineStatus{domain.LineSold, domain.LineSold}, false, domain.LabelClosed},
		{"nothing delivered, all called off", []domain.LineStatus{domain.LineCancelled, domain.LineCancelled}, false, domain.LabelClosedPartial},
		{"delivered some, lost the rest", []domain.LineStatus{domain.LineSold, domain.LineReleased}, false, domain.LabelClosedPartial},
		{"delivered some, cancelled the rest", []domain.LineStatus{domain.LineSold, domain.LineCancelled}, false, domain.LabelClosedPartial},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := domain.DeriveOrderLabel(tc.lines, tc.live); got != tc.want {
				t.Errorf("DeriveOrderLabel = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNewOrderLineValidation(t *testing.T) {
	cases := []struct {
		name                     string
		id, item, itemName, size string
		qty                      int
		price                    int64
	}{
		{"blank id", "", "it-1", "Playera", "8", 1, 100},
		{"blank item", "ln-1", "", "Playera", "8", 1, 100},
		{"blank item name", "ln-1", "it-1", "", "8", 1, 100},
		{"blank size", "ln-1", "it-1", "Playera", "", 1, 100},
		{"zero quantity", "ln-1", "it-1", "Playera", "8", 0, 100},
		{"negative price", "ln-1", "it-1", "Playera", "8", 1, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewOrderLine(tc.id, "t-1", "ord-1", tc.item, tc.itemName, tc.size,
				tc.qty, tc.price, domain.LineAwaitingStock)
			if glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("want KindInvalid, got %v", err)
			}
		})
	}
}

func TestOrderLineSubtotal(t *testing.T) {
	l, err := domain.NewOrderLine("ln-1", "t-1", "ord-1", "it-1", "Playera", "8", 3, 32000, domain.LineReserved)
	if err != nil {
		t.Fatalf("NewOrderLine: %v", err)
	}
	if got := l.SubtotalCents(); got != 96000 {
		t.Errorf("subtotal = %d, want 96000", got)
	}
}
