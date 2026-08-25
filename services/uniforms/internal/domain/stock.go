package domain

import (
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// StockLevel is one garment in one size, and the position of the whole module
// rests on its two counters.
//
//	onHand    what is physically in the storeroom
//	reserved  how much of that is already committed to a live order line
//	available onHand - reserved, DERIVED and never stored
//
// available is the only number a parent should ever see: it is what can actually
// be taken. Selling into someone else's hold is how a family that has waited
// three weeks arrives to nothing.
//
// The type is immutable — every operation returns a copy — so a failed operation
// cannot leave a half-applied position behind.
type StockLevel struct {
	id        string
	tenantID  string
	itemID    string
	sizeLabel string
	sizeOrder int
	onHand    int
	reserved  int
	minStock  int
}

// maxSizeLabelLen matches the column: sizes are "4" or "CH", never prose.
const maxSizeLabelLen = 12

// NewStockLevel rebuilds a position and refuses a broken one. A row that already
// violates the invariant must not become a domain object, or every operation
// after it inherits the corruption.
func NewStockLevel(id, tenantID, itemID, sizeLabel string, sizeOrder, onHand, reserved, minStock int) (StockLevel, error) {
	if strings.TrimSpace(id) == "" {
		return StockLevel{}, glerrors.New(glerrors.KindInvalid, "stock id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return StockLevel{}, glerrors.New(glerrors.KindInvalid, "tenant id is required")
	}
	if strings.TrimSpace(itemID) == "" {
		return StockLevel{}, glerrors.New(glerrors.KindInvalid, "item id is required")
	}
	label := strings.TrimSpace(sizeLabel)
	if label == "" || len(label) > maxSizeLabelLen {
		return StockLevel{}, glerrors.New(glerrors.KindInvalid, "size label must be 1 to 12 characters")
	}
	if onHand < 0 || reserved < 0 || minStock < 0 {
		return StockLevel{}, glerrors.New(glerrors.KindInvalid, "stock counters cannot be negative")
	}
	if reserved > onHand {
		return StockLevel{}, glerrors.New(glerrors.KindInvalid, "reserved cannot exceed what is on hand")
	}
	return StockLevel{
		id: id, tenantID: tenantID, itemID: itemID, sizeLabel: label,
		sizeOrder: sizeOrder, onHand: onHand, reserved: reserved, minStock: minStock,
	}, nil
}

// Available is what a parent may take: the shelf less what is already promised.
func (s StockLevel) Available() int { return s.onHand - s.reserved }

// BelowMinimum reports whether the size needs reordering. It measures what can
// still be sold, not what is on the shelf — five pieces all spoken for is an
// empty shelf as far as the next parent is concerned.
func (s StockLevel) BelowMinimum() bool { return s.Available() <= s.minStock }

// Reserve commits units to an order line. It refuses to commit more than is
// free, which is what stops two families being promised the same piece.
func (s StockLevel) Reserve(qty int) (StockLevel, error) {
	if err := requirePositive(qty); err != nil {
		return StockLevel{}, err
	}
	if qty > s.Available() {
		return StockLevel{}, glerrors.New(glerrors.KindConflict, "not enough stock available")
	}
	s.reserved += qty
	return s, nil
}

// Release gives a hold back. The goods never moved, so only the commitment does.
func (s StockLevel) Release(qty int) (StockLevel, error) {
	if err := requirePositive(qty); err != nil {
		return StockLevel{}, err
	}
	if qty > s.reserved {
		return StockLevel{}, glerrors.New(glerrors.KindInvalid, "cannot release more than is held")
	}
	s.reserved -= qty
	return s, nil
}

// SellReserved hands over units that were being held: the shelf AND the hold
// fall together, in one step.
//
// The pairing is the point. Dropping onHand alone leaves reserved above it,
// which the table's CHECK refuses — and that write happens inside the
// transaction that consumes the payment event, so getting it wrong sends the
// event to the dead-letter queue with the goods paid for and still held. On the
// last unit in stock, that is the common case rather than the rare one.
func (s StockLevel) SellReserved(qty int) (StockLevel, error) {
	if err := requirePositive(qty); err != nil {
		return StockLevel{}, err
	}
	if qty > s.reserved {
		return StockLevel{}, glerrors.New(glerrors.KindInvalid, "cannot sell more than was held")
	}
	s.onHand -= qty
	s.reserved -= qty
	return s, nil
}

// SellFromShelf hands over units that were never reserved — the counter sale,
// where the goods leave as they are sold. Only the physical count falls, and it
// may take only from what is FREE: another family's hold is not the till's to
// spend.
func (s StockLevel) SellFromShelf(qty int) (StockLevel, error) {
	if err := requirePositive(qty); err != nil {
		return StockLevel{}, err
	}
	if qty > s.Available() {
		return StockLevel{}, glerrors.New(glerrors.KindConflict, "not enough stock available")
	}
	s.onHand -= qty
	return s, nil
}

// Receive books a delivery from the supplier onto the shelf.
func (s StockLevel) Receive(qty int) (StockLevel, error) {
	if err := requirePositive(qty); err != nil {
		return StockLevel{}, err
	}
	s.onHand += qty
	return s, nil
}

// Adjust corrects the count after a breakage, a loss or a miscount. It may not
// take the shelf below what families are already holding: those goods are
// promised, and letting the holds go is a decision for a person, not a side
// effect of a stock count.
func (s StockLevel) Adjust(delta int) (StockLevel, error) {
	if delta == 0 {
		return StockLevel{}, glerrors.New(glerrors.KindInvalid, "an adjustment must move the count")
	}
	next := s.onHand + delta
	if next < 0 {
		return StockLevel{}, glerrors.New(glerrors.KindConflict, "an adjustment cannot take the count below zero")
	}
	if next < s.reserved {
		return StockLevel{}, glerrors.New(glerrors.KindConflict, "an adjustment cannot take the count below what is held")
	}
	s.onHand = next
	return s, nil
}

// requirePositive rejects the zero and negative quantities that would otherwise
// let a caller express "sell minus one" and quietly invert an operation.
func requirePositive(qty int) error {
	if qty <= 0 {
		return glerrors.New(glerrors.KindInvalid, "quantity must be positive")
	}
	return nil
}

// Accessors.
func (s StockLevel) ID() string        { return s.id }
func (s StockLevel) TenantID() string  { return s.tenantID }
func (s StockLevel) ItemID() string    { return s.itemID }
func (s StockLevel) SizeLabel() string { return s.sizeLabel }
func (s StockLevel) SizeOrder() int    { return s.sizeOrder }
func (s StockLevel) OnHand() int       { return s.onHand }
func (s StockLevel) Reserved() int     { return s.reserved }
func (s StockLevel) MinStock() int     { return s.minStock }
