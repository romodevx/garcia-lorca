package domain_test

import (
	"errors"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/uniforms/internal/domain"
)

func stock(t *testing.T, onHand, reserved int) domain.StockLevel {
	t.Helper()
	s, err := domain.NewStockLevel("st-1", "t-1", "it-1", "8", 3, onHand, reserved, 2)
	if err != nil {
		t.Fatalf("NewStockLevel(%d, %d): %v", onHand, reserved, err)
	}
	return s
}

func TestStockAvailableIsWhatCanActuallyBeTaken(t *testing.T) {
	s := stock(t, 10, 4)
	if got := s.Available(); got != 6 {
		t.Errorf("Available() = %d, want 6 (on hand minus what is already committed)", got)
	}
}

func TestStockBelowMinimum(t *testing.T) {
	// The alert is about what can still be sold, not what is on the shelf: five
	// pieces all spoken for is an empty shelf as far as the next parent is
	// concerned.
	cases := []struct {
		name             string
		onHand, reserved int
		want             bool
	}{
		{"plenty free", 10, 0, false},
		{"exactly at the line", 2, 0, true},
		{"stocked but all committed", 5, 5, true},
		{"empty", 0, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stock(t, tc.onHand, tc.reserved).BelowMinimum(); got != tc.want {
				t.Errorf("BelowMinimum() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStockReserve(t *testing.T) {
	s := stock(t, 10, 4)

	got, err := s.Reserve(6)
	if err != nil {
		t.Fatalf("Reserve(6): %v", err)
	}
	if got.OnHand() != 10 || got.Reserved() != 10 || got.Available() != 0 {
		t.Errorf("after reserving everything free: on hand %d, reserved %d", got.OnHand(), got.Reserved())
	}
	if s.Reserved() != 4 {
		t.Error("Reserve must not mutate the receiver")
	}
}

func TestStockReserveBeyondAvailable(t *testing.T) {
	s := stock(t, 10, 4)
	// Seven is one more than free. The parent is told there is none, not sold
	// something another family is already holding.
	if _, err := s.Reserve(7); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("want KindConflict reserving beyond available, got %v", err)
	}
}

func TestStockRelease(t *testing.T) {
	s := stock(t, 10, 4)
	got, err := s.Release(4)
	if err != nil {
		t.Fatalf("Release(4): %v", err)
	}
	if got.OnHand() != 10 || got.Reserved() != 0 {
		t.Errorf("releasing returns the hold, not the goods: on hand %d, reserved %d", got.OnHand(), got.Reserved())
	}
}

func TestStockReleaseMoreThanHeld(t *testing.T) {
	s := stock(t, 10, 4)
	if _, err := s.Release(5); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid releasing more than is held, got %v", err)
	}
}

// TestStockSellReservedMovesBothCounters is the invariant that, got wrong, sends
// a payment event to the dead-letter queue with the goods paid for and still
// held. Selling a piece that was reserved must drop the physical count AND the
// hold; dropping only the first leaves reserved above on_hand, which the table's
// CHECK refuses — inside the transaction that consumes the payment.
func TestStockSellReservedMovesBothCounters(t *testing.T) {
	// The case that matters most: the last unit in stock, all of it spoken for.
	s := stock(t, 1, 1)

	got, err := s.SellReserved(1)
	if err != nil {
		t.Fatalf("SellReserved(1): %v", err)
	}
	if got.OnHand() != 0 || got.Reserved() != 0 {
		t.Errorf("on hand %d, reserved %d; both must fall together", got.OnHand(), got.Reserved())
	}
}

func TestStockSellReservedNeedsTheHold(t *testing.T) {
	s := stock(t, 10, 1)
	if _, err := s.SellReserved(2); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid selling more than was held, got %v", err)
	}
}

// TestStockSellFromShelfRespectsOtherFamiliesHolds is the counter sale. The
// goods leave immediately and were never reserved, so only the physical count
// falls — but it may only take from what is FREE. Selling into another family's
// hold is how a parent who has been waiting three weeks arrives to nothing.
func TestStockSellFromShelfRespectsOtherFamiliesHolds(t *testing.T) {
	s := stock(t, 5, 4)

	got, err := s.SellFromShelf(1)
	if err != nil {
		t.Fatalf("SellFromShelf(1): %v", err)
	}
	if got.OnHand() != 4 || got.Reserved() != 4 {
		t.Errorf("on hand %d, reserved %d; the hold is untouched", got.OnHand(), got.Reserved())
	}

	if _, err := got.SellFromShelf(1); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("want KindConflict selling into a hold, got %v", err)
	}
}

func TestStockReceive(t *testing.T) {
	s := stock(t, 1, 1)
	got, err := s.Receive(20)
	if err != nil {
		t.Fatalf("Receive(20): %v", err)
	}
	if got.OnHand() != 21 || got.Reserved() != 1 {
		t.Errorf("on hand %d, reserved %d", got.OnHand(), got.Reserved())
	}
}

func TestStockAdjust(t *testing.T) {
	cases := []struct {
		name             string
		onHand, reserved int
		delta            int
		wantOnHand       int
		wantKind         glerrors.Kind
	}{
		{"count up", 10, 2, 3, 13, glerrors.KindUnknown},
		{"count down", 10, 2, -3, 7, glerrors.KindUnknown},
		// A correction may not take the shelf below what families are holding:
		// the goods would already be promised. Releasing those holds is a
		// decision for a person, not a side effect of a stock count.
		{"below what is held", 10, 8, -5, 0, glerrors.KindConflict},
		{"below zero", 3, 0, -4, 0, glerrors.KindConflict},
		{"no movement", 10, 0, 0, 0, glerrors.KindInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := stock(t, tc.onHand, tc.reserved).Adjust(tc.delta)
			if tc.wantKind != glerrors.KindUnknown {
				if glerrors.KindOf(err) != tc.wantKind {
					t.Fatalf("want %v, got %v", tc.wantKind, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Adjust(%d): %v", tc.delta, err)
			}
			if got.OnHand() != tc.wantOnHand {
				t.Errorf("on hand = %d, want %d", got.OnHand(), tc.wantOnHand)
			}
		})
	}
}

func TestStockRejectsNonPositiveQuantities(t *testing.T) {
	s := stock(t, 10, 2)
	for _, tc := range []struct {
		name string
		call func(int) (domain.StockLevel, error)
	}{
		{"reserve", s.Reserve},
		{"release", s.Release},
		{"sell reserved", s.SellReserved},
		{"sell from shelf", s.SellFromShelf},
		{"receive", s.Receive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, qty := range []int{0, -1} {
				if _, err := tc.call(qty); glerrors.KindOf(err) != glerrors.KindInvalid {
					t.Errorf("%s(%d): want KindInvalid, got %v", tc.name, qty, err)
				}
			}
		})
	}
}

// TestNewStockLevelRefusesABrokenPosition guards the way in: a row that already
// breaks the invariant must never become a domain object, or every operation
// after it inherits the corruption.
func TestNewStockLevelRefusesABrokenPosition(t *testing.T) {
	if _, err := domain.NewStockLevel("st-1", "t-1", "it-1", "8", 1, 2, 5, 0); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid for reserved above on hand, got %v", err)
	}
	if _, err := domain.NewStockLevel("st-1", "t-1", "it-1", "", 1, 2, 0, 0); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid for a blank size, got %v", err)
	}
}

// TestInsufficientStockIsIdentifiable is what lets the edge tell a cashier
// "esa talla ya no está disponible" instead of a generic conflict.
//
// Running out of stock and a duplicate garment code are both conflicts, and
// without a way to tell them apart the panel has to show the same sentence for
// both — one of which is the cashier's fault and one of which is not. Matching on
// the message text would work until somebody rewords it.
func TestInsufficientStockIsIdentifiable(t *testing.T) {
	s := stock(t, 5, 4)

	_, err := s.Reserve(2)
	if !errors.Is(err, domain.ErrInsufficientStock) {
		t.Errorf("a refused reservation is not identifiable as an out-of-stock: %v", err)
	}
	_, err = s.SellFromShelf(2)
	if !errors.Is(err, domain.ErrInsufficientStock) {
		t.Errorf("a refused counter sale is not identifiable as an out-of-stock: %v", err)
	}
	// It is still a conflict, so every caller that only branches on the kind
	// keeps working.
	if glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("kind = %v, want KindConflict", glerrors.KindOf(err))
	}
}

// TestOtherConflictsAreNotOutOfStock: releasing more than is held is a caller
// bug, not an empty shelf, and must not be reported to a parent as one.
func TestOtherConflictsAreNotOutOfStock(t *testing.T) {
	s := stock(t, 5, 1)
	if _, err := s.Release(3); errors.Is(err, domain.ErrInsufficientStock) {
		t.Error("releasing too much was reported as an out-of-stock")
	}
}
