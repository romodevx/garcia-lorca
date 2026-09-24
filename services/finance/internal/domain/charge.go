package domain

import (
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Lifecycle of a charge. Every status except cancelled is DERIVED from the
// payments posted against the charge (see DeriveStatus); it is a cached
// projection of the ledger, never an independently mutable field.
const (
	// StatusPending: nothing has been paid yet.
	StatusPending = "pending"
	// StatusPartiallyPaid: paid, but less than the effective amount.
	StatusPartiallyPaid = "partially_paid"
	// StatusPaid: the effective amount is fully covered.
	StatusPaid = "paid"
	// StatusCancelled: withdrawn by the school; terminal and never derived.
	StatusCancelled = "cancelled"
)

// Charge is one debt a student owes for a cycle, created from a ChargeConcept.
// studentID, cycleID and conceptID are cross-service UUIDs — plain columns
// validated by RPC at the use-case boundary, never cross-service FKs.
//
// amount is the gross charge and discount the scholarship or courtesy applied
// to it; what the student actually owes is Effective() = amount - discount.
// Both are snapshots taken at creation time, so later edits to the catalog or
// the scholarship never rewrite an issued charge.
//
// The entity is immutable: WithDiscount, Cancel and WithStatus each return a
// copy. Paid amounts are deliberately NOT a field — the ledger owns them, and
// the use case passes the summed total in whenever the status must be
// re-derived.
type Charge struct {
	id          string
	tenantID    string
	studentID   string
	cycleID     string
	conceptID   string
	description string
	amount      Cents
	discount    Cents
	// discountReason is why the discount exists. It matters most for the ones
	// the system gives by itself: a family asking why a month is cheaper than
	// the price list deserves an answer that outlives whoever applied it.
	discountReason string
	dueDate        time.Time
	status         string
	// externalRef ties this charge to the row that caused it in another module
	// (e.g. a uniforms fulfillment). Empty for the school's own charges —
	// tuition and the like — which is what keeps the "one concept per cycle"
	// rule applying to them and only to them. See migrations/finance/0003.
	externalRef string
}

// NewCharge builds a validated Charge in the pending status. Every identity
// reference and the description are required, amount and discount must be
// non-negative, and the discount may not exceed the amount. Every failure is
// KindInvalid.
func NewCharge(id, tenantID, studentID, cycleID, conceptID, description string, amountCents, discountCents int64, dueDate time.Time) (Charge, error) {
	if strings.TrimSpace(id) == "" {
		return Charge{}, glerrors.New(glerrors.KindInvalid, "charge id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return Charge{}, glerrors.New(glerrors.KindInvalid, "tenant id is required")
	}
	if strings.TrimSpace(studentID) == "" {
		return Charge{}, glerrors.New(glerrors.KindInvalid, "student id is required")
	}
	if strings.TrimSpace(cycleID) == "" {
		return Charge{}, glerrors.New(glerrors.KindInvalid, "cycle id is required")
	}
	if strings.TrimSpace(conceptID) == "" {
		return Charge{}, glerrors.New(glerrors.KindInvalid, "charge concept id is required")
	}
	if strings.TrimSpace(description) == "" {
		return Charge{}, glerrors.New(glerrors.KindInvalid, "charge description is required")
	}
	amount, err := NewCents(amountCents)
	if err != nil {
		return Charge{}, err
	}
	discount, err := validateDiscount(amount, discountCents)
	if err != nil {
		return Charge{}, err
	}
	if dueDate.IsZero() {
		return Charge{}, glerrors.New(glerrors.KindInvalid, "charge due date is required")
	}
	return Charge{
		id:          id,
		tenantID:    tenantID,
		studentID:   studentID,
		cycleID:     cycleID,
		conceptID:   conceptID,
		description: description,
		amount:      amount,
		discount:    discount,
		dueDate:     dueDate,
		status:      StatusPending,
	}, nil
}

// validateDiscount checks a discount against the gross amount it applies to. A
// discount larger than the amount would make the charge owe negative money, so
// it is rejected; equal is legal and means a full scholarship.
func validateDiscount(amount Cents, discountCents int64) (Cents, error) {
	discount, err := NewCents(discountCents)
	if err != nil {
		return 0, err
	}
	if discount > amount {
		return 0, glerrors.New(glerrors.KindInvalid, "discount cannot exceed amount")
	}
	return discount, nil
}

// Effective is what the student actually owes: the gross amount less the
// discount. Non-negative by construction, since the discount is capped at the
// amount.
func (c Charge) Effective() Cents { return c.amount - c.discount }

// DeriveStatus computes the status this charge would have given the total paid
// against it. Pure: it neither mutates the receiver nor reads a clock.
//
// Cancelled is terminal and wins over any payment. Otherwise nothing paid means
// pending — including the fully-discounted case, which stays pending until a
// payment actually lands rather than reporting itself as collected.
func (c Charge) DeriveStatus(paidCents int64) string {
	if c.status == StatusCancelled {
		return StatusCancelled
	}
	if paidCents <= 0 {
		return StatusPending
	}
	if paidCents >= c.Effective().Int64() {
		return StatusPaid
	}
	return StatusPartiallyPaid
}

// WithDiscount returns a copy carrying a new discount, with the status
// re-derived from paidCents because changing the discount changes what is owed
// (granting a scholarship can settle a charge outright). The new discount is
// re-validated against the amount; over-amount is KindInvalid.
func (c Charge) WithDiscount(discountCents int64, paidCents int64, reason string) (Charge, error) {
	discount, err := validateDiscount(c.amount, discountCents)
	if err != nil {
		return Charge{}, err
	}
	c.discount = discount
	c.discountReason = strings.TrimSpace(reason)
	if discount == 0 {
		// A zero discount that still claimed to be an early payment would be a
		// row that contradicts itself.
		c.discountReason = ""
	}
	c.status = c.DeriveStatus(paidCents)
	return c, nil
}

// Reasons a discount exists. Open-ended — the school types its own for a
// negotiated arrangement — but the ones the system gives by itself are named
// here, so a report can find them without matching on free text.
const (
	// ReasonEarlyPayment is the pronto pago: a fixed amount off a month settled
	// inside the early window.
	ReasonEarlyPayment = "pronto_pago"
	// ReasonEarlyPaymentGranted is the same reward given OUTSIDE the window,
	// because the school decided to honour it — the parent who always pays on
	// the 5th and missed by a day.
	//
	// It is a separate reason on purpose. Recording it as an ordinary pronto
	// pago would make the rule look like it fired when it did not, and a month
	// later nobody could tell an automatic discount from a decision somebody
	// made at the counter.
	ReasonEarlyPaymentGranted = "pronto_pago_autorizado"
	// ReasonLevelDiscount is the school's standing arrangement for a whole
	// level — maternal, most often. Set once on the tuition table and applied by
	// the billing run, never typed per pupil.
	ReasonLevelDiscount = "descuento_nivel"
	// ReasonPriceAtPaymentDate is an inscription settled at the price of the
	// day the family paid, lower than the one it was raised to afterwards
	// (task 7): the cashier captured late a payment made in time. Only a payment
	// that settles the inscription outright earns it, and only for the climb
	// the ladder shows between that day and today.
	ReasonPriceAtPaymentDate = "precio_fecha_de_pago"
)

// DiscountReason is why the charge carries the discount it does. Empty on the
// discounts given before there was anywhere to record it, which is the truth
// about them.
func (c Charge) DiscountReason() string { return c.discountReason }

// Cancel returns a cancelled copy of the charge. Cancelling twice is a
// KindConflict.
//
// For the MVP a charge may be cancelled regardless of the payments posted
// against it: the school sometimes voids a charge it has already partially
// collected, and reconciling that money (refund vs. credit toward another
// charge) is a later phase, handled by the ledger rather than by blocking here.
func (c Charge) Cancel() (Charge, error) {
	if c.status == StatusCancelled {
		return Charge{}, glerrors.New(glerrors.KindConflict, "charge already cancelled")
	}
	c.status = StatusCancelled
	return c, nil
}

// WithStatus returns a copy whose status is re-derived from the total paid.
// Used after a payment is posted. A cancelled charge stays cancelled.
func (c Charge) WithStatus(paidCents int64) Charge {
	c.status = c.DeriveStatus(paidCents)
	return c
}

// maxExternalRefLen bounds the reference so a caller cannot push an unbounded
// string into the ledger's unique index.
const maxExternalRefLen = 128

// WithExternalRef returns a copy tagged with the reference of the row that
// caused this charge in another module. It is what lets a student be charged
// the same concept more than once in a cycle: the "one per cycle" unique index
// is partial and only covers charges with no reference, so tuition keeps its
// idempotency while a uniforms fulfillment gets its own identity.
//
// The reference is trimmed; blank or over-long is KindInvalid.
func (c Charge) WithExternalRef(ref string) (Charge, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return Charge{}, glerrors.New(glerrors.KindInvalid, "external ref is required")
	}
	if len(trimmed) > maxExternalRefLen {
		return Charge{}, glerrors.New(glerrors.KindInvalid, "external ref is too long")
	}
	c.externalRef = trimmed
	return c, nil
}

// WithDueDate returns a copy falling due on another day.
//
// It carries no validation because a due date has none to apply: any day is a
// day a school may ask to be paid on. The rule about WHICH charges may be moved
// — unpaid ones only — belongs to the caller that knows what has been collected,
// which is the repository holding the row lock.
func (c Charge) WithDueDate(due time.Time) Charge {
	c.dueDate = due
	return c
}

// WithAmount returns a copy priced at a new amount, validated.
//
// The twin of WithDueDate, and it exists for the same kind of reason: the price
// ladder of an inscription (ADR 0011) keeps rising until the money arrives, so a
// charge already issued has to be able to say a new number.
//
// It does NOT decide WHICH charges may be re-priced. The rule — nothing
// collected against them, because the first peso freezes the price — belongs to
// the caller that knows what has been paid, which is the repository writing it
// in the same statement.
//
// A copy made here reaches the database through RepriceUnpaid or through a
// deposit's Repriced set, and through NOTHING else. SaveCharge deliberately does
// not write the amount: its callers read and write outside a transaction, so it
// is the one path where a stale price could be put back.
func (c Charge) WithAmount(amountCents int64) (Charge, error) {
	amount, err := NewCents(amountCents)
	if err != nil {
		return Charge{}, err
	}
	// The same guard WithDiscount uses, through the same function: a charge that
	// owed negative money would pay for itself and then some. Reachable now that
	// the amount moves — a scholarship written against January's $2,200 outlives
	// a re-price only if it still fits.
	if _, err := validateDiscount(amount, c.discount.Int64()); err != nil {
		return Charge{}, err
	}
	c.amount = amount
	return c, nil
}

// CancelIfUnpaid returns a cancelled copy ONLY if nothing has been collected
// against the charge. Cancel deliberately voids a charge regardless of its
// payments (the school does that on purpose, see above); this is the guarded
// variant an automated caller needs.
//
// The uniforms expiry sweep is the reason it exists: it voids the charge for a
// hold nobody came to pay for, and it runs while the cashier may be taking that
// exact payment. Refusing to cancel a charge with money on it turns that race
// into an observable outcome — the sweep backs off and leaves the goods
// reserved — instead of a silently voided charge with collected money behind it.
// paidCents is read by the caller under the charge's row lock.
func (c Charge) CancelIfUnpaid(paidCents int64) (Charge, error) {
	if paidCents > 0 {
		return Charge{}, glerrors.New(glerrors.KindConflict, "charge has payments and cannot be cancelled")
	}
	return c.Cancel()
}

// Accessors.
func (c Charge) ID() string          { return c.id }
func (c Charge) TenantID() string    { return c.tenantID }
func (c Charge) StudentID() string   { return c.studentID }
func (c Charge) CycleID() string     { return c.cycleID }
func (c Charge) ConceptID() string   { return c.conceptID }
func (c Charge) Description() string { return c.description }
func (c Charge) Amount() Cents       { return c.amount }
func (c Charge) Discount() Cents     { return c.discount }
func (c Charge) DueDate() time.Time  { return c.dueDate }
func (c Charge) Status() string      { return c.status }
func (c Charge) ExternalRef() string { return c.externalRef }
