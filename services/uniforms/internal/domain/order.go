package domain

import (
	"fmt"
	"regexp"
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// --- folios ------------------------------------------------------------------

// folioPattern matches an order folio, optionally with a delivery suffix. It is
// what lets ONE search box take either a folio or a student's name without
// asking the user which they meant.
var folioPattern = regexp.MustCompile(`^[Uu]-\d{4}-\d{6}(-[A-Za-z]{1,3})?$`)

// FormatFolio renders the human-facing order number.
func FormatFolio(year, seq int) string { return fmt.Sprintf("U-%04d-%06d", year, seq) }

// LooksLikeFolio reports whether a search term is a folio rather than a name.
func LooksLikeFolio(s string) bool { return folioPattern.MatchString(strings.TrimSpace(s)) }

// --- line status -------------------------------------------------------------

// LineStatus is what one garment in one size is doing.
//
//	awaiting_stock ──► reserved ──► sold        (delivered)
//	                       │
//	                       └──► released        (the hold lapsed; terminal)
//	awaiting_stock ──► cancelled                (called off; terminal)
//
// released and cancelled are TERMINAL, and that is a deliberate answer to a real
// question: after a delivery expires, may its lines be picked again? No. The
// school reclaimed the goods and may already have sold them to another family;
// re-promising them would be the system betting on stock it does not know it has.
// The family orders again, which is honest.
type LineStatus string

const (
	LineAwaitingStock LineStatus = "awaiting_stock"
	LineReserved      LineStatus = "reserved"
	LineSold          LineStatus = "sold"
	LineReleased      LineStatus = "released"
	LineCancelled     LineStatus = "cancelled"
)

// HoldsStock reports whether this line is keeping a unit committed. Only
// reserved does: sold means it left, released and cancelled mean it went back.
func (s LineStatus) HoldsStock() bool { return s == LineReserved }

// IsTerminal reports whether the line's story is over.
func (s LineStatus) IsTerminal() bool {
	return s == LineSold || s == LineReleased || s == LineCancelled
}

// delivered reports whether the family actually received this line.
func (s LineStatus) delivered() bool { return s == LineSold }

// --- order lines -------------------------------------------------------------

// OrderLine is one garment in one size within an order.
//
// itemName and unitPriceCents are SNAPSHOTS. The catalogue may be renamed or
// repriced afterwards, and an order already placed must not change under the
// family. The price in particular never comes from the client: it is read from
// the catalogue when the order is made and frozen here.
type OrderLine struct {
	id             string
	tenantID       string
	orderID        string
	itemID         string
	itemName       string
	sizeLabel      string
	qty            int
	unitPriceCents int64
	status         LineStatus
	fulfillmentID  string
	expiryCount    int
}

const maxItemNameLen = 120

// NewOrderLine builds a validated line.
func NewOrderLine(id, tenantID, orderID, itemID, itemName, sizeLabel string, qty int, unitPriceCents int64, status LineStatus) (OrderLine, error) {
	if strings.TrimSpace(id) == "" {
		return OrderLine{}, glerrors.New(glerrors.KindInvalid, "line id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return OrderLine{}, glerrors.New(glerrors.KindInvalid, "tenant id is required")
	}
	if strings.TrimSpace(orderID) == "" {
		return OrderLine{}, glerrors.New(glerrors.KindInvalid, "order id is required")
	}
	if strings.TrimSpace(itemID) == "" {
		return OrderLine{}, glerrors.New(glerrors.KindInvalid, "item id is required")
	}
	name := strings.TrimSpace(itemName)
	if name == "" || len(name) > maxItemNameLen {
		return OrderLine{}, glerrors.New(glerrors.KindInvalid, "item name is required")
	}
	label := strings.TrimSpace(sizeLabel)
	if label == "" || len(label) > maxSizeLabelLen {
		return OrderLine{}, glerrors.New(glerrors.KindInvalid, "size label must be 1 to 12 characters")
	}
	if qty < 1 {
		return OrderLine{}, glerrors.New(glerrors.KindInvalid, "quantity must be at least 1")
	}
	if unitPriceCents < 0 {
		return OrderLine{}, glerrors.New(glerrors.KindInvalid, "price cannot be negative")
	}
	if status == "" {
		return OrderLine{}, glerrors.New(glerrors.KindInvalid, "line status is required")
	}
	return OrderLine{
		id: id, tenantID: tenantID, orderID: orderID, itemID: itemID, itemName: name,
		sizeLabel: label, qty: qty, unitPriceCents: unitPriceCents, status: status,
	}, nil
}

// SubtotalCents is what this line costs.
func (l OrderLine) SubtotalCents() int64 { return int64(l.qty) * l.unitPriceCents }

// Reserve commits stock to the line. Refused once the line is terminal — see the
// note on LineStatus for why a released line is not picked up again.
func (l OrderLine) Reserve() (OrderLine, error) {
	if l.status != LineAwaitingStock {
		return OrderLine{}, glerrors.New(glerrors.KindConflict,
			"a line that is "+string(l.status)+" cannot be reserved")
	}
	l.status = LineReserved
	return l, nil
}

// Release gives the hold back and counts the lapse. The count is what stops a
// family holding the last unit of a size indefinitely, for free, by reserving it
// and never coming to pay.
func (l OrderLine) Release() (OrderLine, error) {
	if l.status != LineReserved {
		return OrderLine{}, glerrors.New(glerrors.KindConflict,
			"a line that is "+string(l.status)+" holds nothing to release")
	}
	l.status = LineReleased
	l.fulfillmentID = ""
	l.expiryCount++
	return l, nil
}

// Cancel calls off a line that is still waiting for stock. A line that already
// holds goods is not cancelled but RELEASED: the difference is that letting go of
// real goods counts against the family's lapse budget, and calling off something
// nobody had does not.
func (l OrderLine) Cancel() (OrderLine, error) {
	if l.status != LineAwaitingStock {
		return OrderLine{}, glerrors.New(glerrors.KindConflict,
			"only a line still waiting for stock can be cancelled")
	}
	l.status = LineCancelled
	return l, nil
}

// Sell records that the line was handed over.
func (l OrderLine) Sell() (OrderLine, error) {
	if l.status != LineReserved {
		return OrderLine{}, glerrors.New(glerrors.KindConflict,
			"a line that is "+string(l.status)+" cannot be sold")
	}
	l.status = LineSold
	return l, nil
}

// AssignTo puts the line into a delivery.
func (l OrderLine) AssignTo(fulfillmentID string) (OrderLine, error) {
	if strings.TrimSpace(fulfillmentID) == "" {
		return OrderLine{}, glerrors.New(glerrors.KindInvalid, "fulfillment id is required")
	}
	l.fulfillmentID = fulfillmentID
	return l, nil
}

// CanBeReservedAgain reports whether the family's lapses have run out. A line
// that has been let go too often stops being reservable, which is the ceiling
// that makes a hold mean something.
func (l OrderLine) CanBeReservedAgain(maxExpiries int) bool { return l.expiryCount < maxExpiries }

// Accessors.
func (l OrderLine) ID() string            { return l.id }
func (l OrderLine) TenantID() string      { return l.tenantID }
func (l OrderLine) OrderID() string       { return l.orderID }
func (l OrderLine) ItemID() string        { return l.itemID }
func (l OrderLine) ItemName() string      { return l.itemName }
func (l OrderLine) SizeLabel() string     { return l.sizeLabel }
func (l OrderLine) Qty() int              { return l.qty }
func (l OrderLine) UnitPriceCents() int64 { return l.unitPriceCents }
func (l OrderLine) Status() LineStatus    { return l.status }
func (l OrderLine) FulfillmentID() string { return l.fulfillmentID }
func (l OrderLine) ExpiryCount() int      { return l.expiryCount }

// --- the order's derived label ----------------------------------------------

// OrderLabel is how an order reads in the panel and in the family app. It is
// DERIVED on every read and never stored, so it cannot drift from the lines and
// deliveries it describes.
type OrderLabel string

const (
	LabelAwaitingStock OrderLabel = "awaiting_stock"
	LabelPartial       OrderLabel = "partial"
	LabelInProgress    OrderLabel = "in_progress"
	LabelClosed        OrderLabel = "closed"
	// LabelClosedPartial is the honest ending for an order that finished having
	// delivered only some of what was asked for. Without it, the worst outcome
	// the module can produce — a family that ordered three garments, received
	// one, and had the rest lapse — reads exactly like the best one.
	LabelClosedPartial OrderLabel = "closed_partial"
)

// DeriveOrderLabel computes an order's state from its lines and whether any of
// its deliveries is still holding goods.
func DeriveOrderLabel(lines []LineStatus, hasLiveFulfillment bool) OrderLabel {
	var waiting, open, delivered, lost int
	for _, s := range lines {
		switch {
		case s == LineAwaitingStock:
			waiting++
		case !s.IsTerminal():
			open++
		case s.delivered():
			delivered++
		default:
			lost++
		}
	}

	if waiting > 0 {
		if hasLiveFulfillment || open > 0 {
			return LabelPartial
		}
		return LabelAwaitingStock
	}
	if open > 0 || hasLiveFulfillment {
		return LabelInProgress
	}
	if lost > 0 {
		return LabelClosedPartial
	}
	return LabelClosed
}
