package domain

import (
	"fmt"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Deposit is money as it arrived: one amount, one date, one method, one receipt.
//
// It exists because the school does not collect the way the old screen assumed.
// A parent hands over an amount and says nothing else; what it covers is worked
// out afterwards, oldest debt first. Recording the deposit and recording the
// allocation as separate facts is what lets the office correct a split without
// touching the money, and lets a receipt say "recibimos $6,700" even while the
// months it covers are still being decided.
//
// The allocations themselves are Payments carrying this deposit's id. There is
// one place that answers "what has been collected against this charge", and it
// is still the payments table.
//
// A Deposit is a value. Void returns a new one rather than mutating, so a caller
// holding the pre-void deposit keeps holding exactly what was true when it read
// it.
type Deposit struct {
	id              string
	tenantID        string
	studentID       string
	amount          Cents
	method          string
	reference       string
	paidAt          time.Time
	idempotencyKey  string
	calendarVersion string
	voidedAt        time.Time
	voidReason      string
	cardType        string
	receivedBy      Receiver
	capturedAt      time.Time
	// folio is the number the receipt prints (task 12): P-<year>-<six digits>,
	// one run per school and year, allocated when the deposit is stored.
	folio string
	// tenders are the methods the money came in by (T8). method, reference and
	// cardType above are their summary: the one tender's own, or "mixed" with
	// neither reference nor card type when there are several.
	tenders []Tender
}

// Tenders are the methods the money came in by, in the order given. A deposit
// built the old way is one tender with the deposit's own method.
func (d Deposit) Tenders() []Tender {
	if len(d.tenders) == 0 {
		return []Tender{{method: d.method, amount: d.amount, reference: d.reference, cardType: d.cardType}}
	}
	out := make([]Tender, len(d.tenders))
	copy(out, d.tenders)
	return out
}

// WithTenders returns the deposit paid by these tenders (T8). They must add up
// exactly to the deposit's amount — the rule the school approved: a payment
// whose methods do not cover the amount to pay is not registered. The summary
// fields follow: one tender lends the deposit its method, reference and card
// type; several make it "mixed", with those details on each tender.
func (d Deposit) WithTenders(tenders []Tender) (Deposit, error) {
	if err := ValidateTenders(tenders, d.amount.Int64()); err != nil {
		return Deposit{}, err
	}
	d.tenders = make([]Tender, len(tenders))
	copy(d.tenders, tenders)
	if len(tenders) == 1 {
		d.method, d.reference, d.cardType = tenders[0].method, tenders[0].reference, tenders[0].cardType
		return d, nil
	}
	d.method, d.reference, d.cardType = MethodMixed, "", ""
	return d, nil
}

// FormatDepositFolio renders a deposit's receipt number.
func FormatDepositFolio(year, seq int) string { return fmt.Sprintf("P-%04d-%06d", year, seq) }

// WithFolio returns the deposit carrying its receipt number.
func (d Deposit) WithFolio(folio string) Deposit {
	d.folio = folio
	return d
}

// Folio is the receipt number; empty until the deposit is stored.
func (d Deposit) Folio() string { return d.folio }

// Receiver is who took the money at the counter: the account, and its name as
// it read then (task 7). Empty on deposits captured before it was recorded.
type Receiver struct {
	UserID string
	Name   string
}

// WithReceiver returns the deposit attributed to whoever received it.
func (d Deposit) WithReceiver(r Receiver) Deposit {
	d.receivedBy = r
	return d
}

// ReceivedBy is who took the money; empty on deposits from before it was kept.
func (d Deposit) ReceivedBy() Receiver { return d.receivedBy }

// WithCapturedAt returns the deposit with the moment it was keyed, which is
// not the moment the family paid when a cashier captures a payment late.
func (d Deposit) WithCapturedAt(at time.Time) Deposit {
	d.capturedAt = at
	return d
}

// CapturedAt is when the deposit was keyed; zero before it is stored.
func (d Deposit) CapturedAt() time.Time { return d.capturedAt }

// Which kind of card the money came in on. Not a payment method — the money
// still arrives by card and the ledger treats it as one — but the terminal the
// office reconciles against at month end knows the difference, so the ledger
// has to as well.
const (
	// CardDebit is a debit card.
	CardDebit = "debit"
	// CardCredit is a credit card.
	CardCredit = "credit"
)

// WithCardType records debit or credit on a card payment.
//
// Only on a card payment: a card type on cash is a contradiction somebody would
// eventually have to explain to an auditor, and nobody could. An empty type
// clears it, which is the ordinary state of cash, a transfer, and every card
// payment taken before the school asked for this.
func (d Deposit) WithCardType(cardType string) (Deposit, error) {
	if err := validateCardType(d.method, cardType); err != nil {
		return Deposit{}, err
	}
	d.cardType = cardType
	if len(d.tenders) == 1 {
		d.tenders = []Tender{d.tenders[0]}
		d.tenders[0].cardType = cardType
	}
	return d, nil
}

// CardType is debit or credit on a card payment, and empty otherwise.
func (d Deposit) CardType() string { return d.cardType }

// NewDeposit builds a validated deposit.
//
// calendarVersion is the school calendar the allocation was worked out against.
// A surcharge or an early-payment discount is a number someone has to defend
// months later, and without this a recomputation cannot tell whether it is
// looking at the same calendar the cashier saw. It is opaque to the domain.
func NewDeposit(id, tenantID, studentID string, amountCents int64, method, reference string, paidAt time.Time, idempotencyKey, calendarVersion string) (Deposit, error) {
	if strings.TrimSpace(id) == "" {
		return Deposit{}, glerrors.New(glerrors.KindInvalid, "deposit id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return Deposit{}, glerrors.New(glerrors.KindInvalid, "tenant id is required")
	}
	if strings.TrimSpace(studentID) == "" {
		return Deposit{}, glerrors.New(glerrors.KindInvalid, "student id is required")
	}
	// Same reasoning as Payment: a movement of no money is not a deposit, and
	// NewCents still enforces the upper bound that keeps sums from overflowing.
	if amountCents <= 0 {
		return Deposit{}, glerrors.New(glerrors.KindInvalid, "deposit amount must be positive")
	}
	amount, err := NewCents(amountCents)
	if err != nil {
		return Deposit{}, err
	}
	if err := validateMethod(method); err != nil {
		return Deposit{}, err
	}
	if paidAt.IsZero() {
		return Deposit{}, glerrors.New(glerrors.KindInvalid, "deposit date is required")
	}
	return Deposit{
		id: id, tenantID: tenantID, studentID: studentID,
		amount: amount, method: method, reference: strings.TrimSpace(reference),
		paidAt: paidAt, idempotencyKey: idempotencyKey,
		calendarVersion: calendarVersion,
	}, nil
}

// Void cancels a deposit, keeping the row and recording why.
//
// A cashier who keys $67,000 instead of $6,700 has to be able to undo it, and a
// school that deletes money rows cannot explain its own ledger. The reason is
// required: without one, a cancelled deposit is an unexplained hole in the day's
// takings that someone will have to account for at the end of the month.
//
// Voiding an already-void deposit is a second cashier acting on a stale screen.
// The first void stands and the second is refused, rather than quietly
// overwriting who cancelled what, and when.
func (d Deposit) Void(reason string, at time.Time) (Deposit, error) {
	if d.Voided() {
		return Deposit{}, glerrors.New(glerrors.KindConflict, "this deposit is already cancelled")
	}
	if strings.TrimSpace(reason) == "" {
		return Deposit{}, glerrors.New(glerrors.KindInvalid, "cancelling a deposit needs a reason")
	}
	if at.IsZero() {
		return Deposit{}, glerrors.New(glerrors.KindInvalid, "cancellation date is required")
	}
	out := d
	out.voidedAt = at
	out.voidReason = strings.TrimSpace(reason)
	return out, nil
}

// Rehydrate rebuilds a stored deposit, void state included. Only the persistence
// mapper calls it: NewDeposit is the constructor for new money, and it
// deliberately cannot produce a voided deposit.
func (d Deposit) Rehydrate(voidedAt time.Time, voidReason string) Deposit {
	out := d
	out.voidedAt = voidedAt
	out.voidReason = strings.TrimSpace(voidReason)
	return out
}

// ID returns the deposit's identifier.
func (d Deposit) ID() string { return d.id }

// TenantID returns the owning tenant.
func (d Deposit) TenantID() string { return d.tenantID }

// StudentID is the pupil whose family handed over the money.
func (d Deposit) StudentID() string { return d.studentID }

// Amount is the money as it arrived, in cents.
func (d Deposit) Amount() Cents { return d.amount }

// Method is how the money reached the school.
func (d Deposit) Method() string { return d.method }

// Reference identifies the money itself: a folio, a transfer id, a card slip.
func (d Deposit) Reference() string { return d.reference }

// PaidAt is when the money changed hands, which is not always when it was keyed.
func (d Deposit) PaidAt() time.Time { return d.paidAt }

// IdempotencyKey is the caller's handle on the attempt.
func (d Deposit) IdempotencyKey() string { return d.idempotencyKey }

// CalendarVersion is the school calendar the allocation was worked out against.
func (d Deposit) CalendarVersion() string { return d.calendarVersion }

// Voided reports whether the deposit has been cancelled.
func (d Deposit) Voided() bool { return !d.voidedAt.IsZero() }

// VoidedAt is when it was cancelled (zero if it was not).
func (d Deposit) VoidedAt() time.Time { return d.voidedAt }

// VoidReason is why it was cancelled.
func (d Deposit) VoidReason() string { return d.voidReason }
