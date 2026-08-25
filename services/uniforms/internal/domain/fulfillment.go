package domain

import (
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// FulfillmentStatus is the state of one delivery.
//
// The state machine lives on the DELIVERY rather than the order because a
// delivery is what actually gets picked, charged, expires and is handed over. An
// order of three garments with two in stock becomes one delivery now and another
// when the third arrives, each with its own charge and its own deadline.
//
//	picking ──► ready_for_pickup ──► paid ──► dispatched
//	                   │
//	                   └──► expired          (nobody came; the hold is released)
//
//	Cancelled is reachable from picking and ready_for_pickup.
//	SoldAtCounter is terminal from birth and reachable from nothing.
type FulfillmentStatus string

const (
	// StatusPicking: the row exists, the charge does not yet. Deliberately a
	// state rather than a gap, so a crash between the two leaves something to
	// resume instead of a charge nobody owns.
	StatusPicking FulfillmentStatus = "picking"
	// StatusReadyForPickup: waiting for the family, and the clock is running.
	StatusReadyForPickup FulfillmentStatus = "ready_for_pickup"
	// StatusPaid: settled. The goods left the shelf at this transition.
	StatusPaid FulfillmentStatus = "paid"
	// StatusDispatched: handed over. Terminal.
	StatusDispatched FulfillmentStatus = "dispatched"
	// StatusExpired: nobody came, the hold was released. Terminal.
	StatusExpired FulfillmentStatus = "expired"
	// StatusCancelled: called off by staff. Terminal.
	StatusCancelled FulfillmentStatus = "cancelled"
	// StatusSoldAtCounter: sold and handed over in one act. Terminal from birth,
	// and the reason it is its own state rather than a shortcut through the
	// normal path: no sweep may ever "return" goods that physically left.
	StatusSoldAtCounter FulfillmentStatus = "sold_at_counter"
)

// allowedTransitions is the single source of truth for what may follow what.
// Every path NOT in this table is a way to lose money or lose goods.
var allowedTransitions = map[FulfillmentStatus]map[FulfillmentStatus]bool{
	StatusPicking: {
		StatusReadyForPickup: true,
		StatusCancelled:      true,
	},
	StatusReadyForPickup: {
		StatusPaid:      true,
		StatusExpired:   true,
		StatusCancelled: true,
	},
	StatusPaid: {
		StatusDispatched: true,
	},
	StatusDispatched:    {},
	StatusExpired:       {},
	StatusCancelled:     {},
	StatusSoldAtCounter: {},
}

// HoldsStock reports whether a delivery in this state is still keeping units
// reserved. The expiry sweep and the panel's counters both read it; getting it
// wrong either strands inventory or sells it twice.
//
// Paid is deliberately false: the goods came off the shelf at that transition,
// so there is nothing left to hold.
func (s FulfillmentStatus) HoldsStock() bool {
	return s == StatusPicking || s == StatusReadyForPickup
}

// IsTerminal reports whether nothing may follow.
func (s FulfillmentStatus) IsTerminal() bool {
	return len(allowedTransitions[s]) == 0
}

// Fulfillment is one delivery of an order. Immutable: every transition returns a
// copy, so a refused transition cannot leave a half-applied delivery behind.
type Fulfillment struct {
	id                string
	tenantID          string
	orderID           string
	seq               int
	status            FulfillmentStatus
	amountCents       int64
	financeChargeID   string
	readyAt           time.Time
	pickupDeadline    time.Time
	deadlineEstimated bool
	extensionSeq      int
	requiresAttention bool
	attentionReason   string
	paidAt            time.Time
	dispatchedAt      time.Time
	cancelledAt       time.Time
	cancelReason      string
}

const maxReasonLen = 200

// NewFulfillment opens a delivery in the picking state.
func NewFulfillment(id, tenantID, orderID string, seq int, amountCents int64) (Fulfillment, error) {
	f, err := newFulfillment(id, tenantID, orderID, seq, amountCents)
	if err != nil {
		return Fulfillment{}, err
	}
	f.status = StatusPicking
	return f, nil
}

// NewCounterSaleFulfillment opens a delivery that is already over: the parent
// walked in, paid and left with the goods. It is born terminal so that no expiry
// sweep, and no resume of a stuck pick, can ever treat it as a hold and put
// stock back that is no longer in the storeroom.
func NewCounterSaleFulfillment(id, tenantID, orderID string, seq int, amountCents int64, soldAt time.Time) (Fulfillment, error) {
	f, err := newFulfillment(id, tenantID, orderID, seq, amountCents)
	if err != nil {
		return Fulfillment{}, err
	}
	if soldAt.IsZero() {
		return Fulfillment{}, glerrors.New(glerrors.KindInvalid, "the time of sale is required")
	}
	f.status = StatusSoldAtCounter
	f.paidAt = soldAt
	f.dispatchedAt = soldAt
	return f, nil
}

func newFulfillment(id, tenantID, orderID string, seq int, amountCents int64) (Fulfillment, error) {
	if strings.TrimSpace(id) == "" {
		return Fulfillment{}, glerrors.New(glerrors.KindInvalid, "fulfillment id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return Fulfillment{}, glerrors.New(glerrors.KindInvalid, "tenant id is required")
	}
	if strings.TrimSpace(orderID) == "" {
		return Fulfillment{}, glerrors.New(glerrors.KindInvalid, "order id is required")
	}
	if seq < 1 {
		return Fulfillment{}, glerrors.New(glerrors.KindInvalid, "fulfillment sequence starts at 1")
	}
	if amountCents < 0 {
		return Fulfillment{}, glerrors.New(glerrors.KindInvalid, "amount cannot be negative")
	}
	return Fulfillment{id: id, tenantID: tenantID, orderID: orderID, seq: seq, amountCents: amountCents}, nil
}

// FulfillmentSnapshot is a stored delivery, as the persistence layer holds it.
// It exists so rehydration has ONE shape to fill rather than a constructor with
// sixteen positional arguments, half of them zero most of the time.
type FulfillmentSnapshot struct {
	ID                string
	TenantID          string
	OrderID           string
	Seq               int
	Status            FulfillmentStatus
	AmountCents       int64
	FinanceChargeID   string
	ReadyAt           time.Time
	PickupDeadline    time.Time
	DeadlineEstimated bool
	ExtensionSeq      int
	RequiresAttention bool
	AttentionReason   string
	PaidAt            time.Time
	DispatchedAt      time.Time
	CancelledAt       time.Time
	CancelReason      string
}

// RestoreFulfillment rebuilds a stored delivery.
//
// It deliberately does NOT replay the state machine to arrive at the stored
// status. A row that says "dispatched" is a fact about the past; asking the
// transition table to re-approve it would make LOADING that row fail for
// something that already happened, and no amount of validation at read time can
// un-hand-over a garment. The table governs what happens NEXT — the only place
// it can prevent anything — so the past is accepted as it stands and the guards
// apply from here on, exactly as strictly.
//
// The one thing it does refuse is a status this code does not know: that is data
// corruption or a half-finished migration, and loading it would hand the machine
// a state with no transitions and no meaning.
func RestoreFulfillment(s FulfillmentSnapshot) (Fulfillment, error) {
	f, err := newFulfillment(s.ID, s.TenantID, s.OrderID, s.Seq, s.AmountCents)
	if err != nil {
		return Fulfillment{}, err
	}
	if _, known := allowedTransitions[s.Status]; !known {
		return Fulfillment{}, glerrors.New(glerrors.KindInvalid,
			"unknown delivery status: "+string(s.Status))
	}
	f.status = s.Status
	f.financeChargeID = s.FinanceChargeID
	f.readyAt = s.ReadyAt
	f.pickupDeadline = s.PickupDeadline
	f.deadlineEstimated = s.DeadlineEstimated
	f.extensionSeq = s.ExtensionSeq
	f.requiresAttention = s.RequiresAttention
	f.attentionReason = s.AttentionReason
	f.paidAt = s.PaidAt
	f.dispatchedAt = s.DispatchedAt
	f.cancelledAt = s.CancelledAt
	f.cancelReason = s.CancelReason
	return f, nil
}

// Folio renders the delivery's own folio: the order's, plus a suffix. The suffix
// is derived from a NUMBER, so the twenty-seventh delivery is "-AA" rather than
// impossible.
func (f Fulfillment) Folio(orderFolio string) string {
	return orderFolio + "-" + seqSuffix(f.seq)
}

// seqSuffix turns 1,2,…,26,27 into A,B,…,Z,AA — spreadsheet columns, which
// everyone can already read.
func seqSuffix(seq int) string {
	var out []byte
	for n := seq; n > 0; n = (n - 1) / 26 {
		out = append([]byte{byte('A' + (n-1)%26)}, out...)
	}
	return string(out)
}

// transition applies a state change if the table allows it. Every mutator goes
// through here, so there is exactly one place that decides what is reachable.
func (f Fulfillment) transition(to FulfillmentStatus) (Fulfillment, error) {
	if !allowedTransitions[f.status][to] {
		return Fulfillment{}, glerrors.New(glerrors.KindConflict,
			"a delivery that is "+string(f.status)+" cannot become "+string(to))
	}
	f.status = to
	return f, nil
}

// MarkReady records that the charge was raised and starts the collection clock.
// deadlineEstimated says the school calendar was unreachable and the date was
// computed without it; such a date may later be extended, never shortened.
func (f Fulfillment) MarkReady(chargeID string, readyAt, deadline time.Time, deadlineEstimated bool) (Fulfillment, error) {
	if strings.TrimSpace(chargeID) == "" {
		return Fulfillment{}, glerrors.New(glerrors.KindInvalid, "a ready delivery must carry its charge")
	}
	if readyAt.IsZero() || deadline.IsZero() {
		return Fulfillment{}, glerrors.New(glerrors.KindInvalid, "a ready delivery needs a start and a deadline")
	}
	next, err := f.transition(StatusReadyForPickup)
	if err != nil {
		return Fulfillment{}, err
	}
	next.financeChargeID = chargeID
	next.readyAt = readyAt
	next.pickupDeadline = deadline
	next.deadlineEstimated = deadlineEstimated
	return next, nil
}

// MarkPaid records that the charge was settled. The caller deducts the goods in
// the same transaction; see StockLevel.SellReserved for why both counters move.
func (f Fulfillment) MarkPaid(at time.Time) (Fulfillment, error) {
	if at.IsZero() {
		return Fulfillment{}, glerrors.New(glerrors.KindInvalid, "the time of payment is required")
	}
	next, err := f.transition(StatusPaid)
	if err != nil {
		return Fulfillment{}, err
	}
	next.paidAt = at
	return next, nil
}

// Dispatch hands the goods over. Terminal.
func (f Fulfillment) Dispatch(at time.Time) (Fulfillment, error) {
	if at.IsZero() {
		return Fulfillment{}, glerrors.New(glerrors.KindInvalid, "the time of handover is required")
	}
	next, err := f.transition(StatusDispatched)
	if err != nil {
		return Fulfillment{}, err
	}
	next.dispatchedAt = at
	return next, nil
}

// Expire releases a hold nobody came to collect. Expiring twice is a conflict,
// which is what keeps the daily sweep and the lazy path on reads from releasing
// the same units twice.
func (f Fulfillment) Expire(at time.Time) (Fulfillment, error) {
	if at.IsZero() {
		return Fulfillment{}, glerrors.New(glerrors.KindInvalid, "the time of expiry is required")
	}
	next, err := f.transition(StatusExpired)
	if err != nil {
		return Fulfillment{}, err
	}
	next.cancelledAt = at
	return next, nil
}

// Cancel calls a delivery off. The reason is required because this is the state
// a parent will ask about.
func (f Fulfillment) Cancel(reason string, at time.Time) (Fulfillment, error) {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" || len(trimmed) > maxReasonLen {
		return Fulfillment{}, glerrors.New(glerrors.KindInvalid, "a cancellation needs a reason")
	}
	if at.IsZero() {
		return Fulfillment{}, glerrors.New(glerrors.KindInvalid, "the time of cancellation is required")
	}
	next, err := f.transition(StatusCancelled)
	if err != nil {
		return Fulfillment{}, err
	}
	next.cancelReason = trimmed
	next.cancelledAt = at
	return next, nil
}

// FlagForAttention marks a delivery no rule can settle on its own — money that
// arrived for a hold already released, say, where the goods may since have been
// sold to somebody else. It does NOT change the status: the point is to hand the
// case to a person, not to absorb it. Silence is the one outcome not allowed.
func (f Fulfillment) FlagForAttention(reason string) (Fulfillment, error) {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" || len(trimmed) > maxReasonLen {
		return Fulfillment{}, glerrors.New(glerrors.KindInvalid, "a flag needs a reason or it tells staff nothing")
	}
	f.requiresAttention = true
	f.attentionReason = trimmed
	return f, nil
}

// ExtendTo gives a family more time. It exists so that staff do not cancel and
// re-create orders to buy a few days, which would leave the stock ledger unable
// to explain itself.
//
// A deadline may only ever move LATER: the date was already told to the family,
// and a system that walks a promise back is worse than one that never made it.
// extensionSeq rises with each extension and forms part of the notification's
// idempotency key, without which the consumer would discard the second "your
// order is ready" and the family would never learn the new date.
func (f Fulfillment) ExtendTo(deadline time.Time, reason string, maxExtensions int) (Fulfillment, error) {
	if f.status != StatusReadyForPickup {
		return Fulfillment{}, glerrors.New(glerrors.KindConflict, "only a delivery waiting to be collected can be extended")
	}
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" || len(trimmed) > maxReasonLen {
		return Fulfillment{}, glerrors.New(glerrors.KindInvalid, "an extension needs a reason")
	}
	if !deadline.After(f.pickupDeadline) {
		return Fulfillment{}, glerrors.New(glerrors.KindInvalid, "a deadline may be extended, never shortened")
	}
	if f.extensionSeq >= maxExtensions {
		return Fulfillment{}, glerrors.New(glerrors.KindConflict, "this delivery has been extended as often as the school allows")
	}
	f.pickupDeadline = deadline
	f.extensionSeq++
	return f, nil
}

// HoldsStock reports whether this delivery is still keeping units reserved.
func (f Fulfillment) HoldsStock() bool { return f.status.HoldsStock() }

// IsTerminal reports whether nothing may follow.
func (f Fulfillment) IsTerminal() bool { return f.status.IsTerminal() }

// IsOverdue reports whether the collection window has closed as of now. Pure:
// the clock is the caller's, never the package's.
func (f Fulfillment) IsOverdue(now time.Time) bool {
	if f.status != StatusReadyForPickup || f.pickupDeadline.IsZero() {
		return false
	}
	return now.After(f.pickupDeadline)
}

// Accessors.
func (f Fulfillment) ID() string                { return f.id }
func (f Fulfillment) TenantID() string          { return f.tenantID }
func (f Fulfillment) OrderID() string           { return f.orderID }
func (f Fulfillment) Seq() int                  { return f.seq }
func (f Fulfillment) Status() FulfillmentStatus { return f.status }
func (f Fulfillment) AmountCents() int64        { return f.amountCents }
func (f Fulfillment) FinanceChargeID() string   { return f.financeChargeID }
func (f Fulfillment) ReadyAt() time.Time        { return f.readyAt }
func (f Fulfillment) PickupDeadline() time.Time { return f.pickupDeadline }
func (f Fulfillment) DeadlineEstimated() bool   { return f.deadlineEstimated }
func (f Fulfillment) ExtensionSeq() int         { return f.extensionSeq }
func (f Fulfillment) RequiresAttention() bool   { return f.requiresAttention }
func (f Fulfillment) AttentionReason() string   { return f.attentionReason }
func (f Fulfillment) PaidAt() time.Time         { return f.paidAt }
func (f Fulfillment) DispatchedAt() time.Time   { return f.dispatchedAt }
func (f Fulfillment) CancelledAt() time.Time    { return f.cancelledAt }
func (f Fulfillment) CancelReason() string      { return f.cancelReason }
