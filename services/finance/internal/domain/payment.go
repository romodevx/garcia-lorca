package domain

import (
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// How the money reached the school. A closed vocabulary: the CONTPAQi export
// and the future reconciliation report both switch on it.
const (
	// MethodCash is money taken at the front desk.
	MethodCash = "cash"
	// MethodCard is a card terminal charge.
	MethodCard = "card"
	// MethodTransfer is a bank transfer (SPEI and friends).
	MethodTransfer = "transfer"
)

// validateMethod enforces the closed payment-method vocabulary.
func validateMethod(method string) error {
	switch method {
	case MethodCash, MethodCard, MethodTransfer:
		return nil
	default:
		return glerrors.New(glerrors.KindInvalid, "invalid payment method")
	}
}

// Payment is one movement of money against a Charge. Payments are append-only:
// there is no mutator, and a mistake is corrected by a reversing entry so the
// audit trail survives. A charge's collected total is the sum of its payments,
// which is why Charge stores no paid field.
//
// paidAt is when the money was received, supplied by the caller — the domain
// never reads a clock, so every derivation stays deterministic.
//
// idempotencyKey is the caller's optional handle on the payment ATTEMPT (as
// opposed to reference, which identifies the money): two requests carrying the
// same key are the same attempt retried, and only one of them may become a
// payment. It is opaque to the domain — enforcing the dedup needs the whole
// ledger, so it belongs to the repository's payment transaction.
type Payment struct {
	id             string
	tenantID       string
	chargeID       string
	amount         Cents
	method         string
	reference      string
	paidAt         time.Time
	idempotencyKey string
	receivedBy     string
}

// WithReceivedBy returns the payment attributed to the account that took the
// money (task 7).
func (p Payment) WithReceivedBy(userID string) Payment {
	p.receivedBy = userID
	return p
}

// ReceivedBy is the account that took the money; empty when it was not kept.
func (p Payment) ReceivedBy() string { return p.receivedBy }

// NewPayment builds a validated Payment. Identity, tenant and charge are
// required, the amount must be strictly positive (a zero-money movement is not
// a payment), and the method must belong to the closed vocabulary above.
// reference is optional: cash at the desk carries none, and so is
// idempotencyKey — an empty key simply means the caller is not deduplicating
// retries. Every failure is KindInvalid.
func NewPayment(id, tenantID, chargeID string, amountCents int64, method, reference string, paidAt time.Time, idempotencyKey string) (Payment, error) {
	if strings.TrimSpace(id) == "" {
		return Payment{}, glerrors.New(glerrors.KindInvalid, "payment id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return Payment{}, glerrors.New(glerrors.KindInvalid, "tenant id is required")
	}
	if strings.TrimSpace(chargeID) == "" {
		return Payment{}, glerrors.New(glerrors.KindInvalid, "charge id is required")
	}
	// Stricter than Cents on the low end — a movement of no money is not a
	// payment — but it still goes through NewCents for the MaxCents upper bound,
	// which is what stops a near-MaxInt64 amount from overflowing the
	// currentPaid + amount sum in the payment knot and bypassing the overpay veto.
	if amountCents <= 0 {
		return Payment{}, glerrors.New(glerrors.KindInvalid, "payment amount must be positive")
	}
	amount, err := NewCents(amountCents)
	if err != nil {
		return Payment{}, err
	}
	if err := validateMethod(method); err != nil {
		return Payment{}, err
	}
	if paidAt.IsZero() {
		return Payment{}, glerrors.New(glerrors.KindInvalid, "payment date is required")
	}
	return Payment{
		id:             id,
		tenantID:       tenantID,
		chargeID:       chargeID,
		amount:         amount,
		method:         method,
		reference:      reference,
		paidAt:         paidAt,
		idempotencyKey: idempotencyKey,
	}, nil
}

// Accessors.
func (p Payment) ID() string             { return p.id }
func (p Payment) TenantID() string       { return p.tenantID }
func (p Payment) ChargeID() string       { return p.chargeID }
func (p Payment) Amount() Cents          { return p.amount }
func (p Payment) Method() string         { return p.method }
func (p Payment) Reference() string      { return p.reference }
func (p Payment) PaidAt() time.Time      { return p.paidAt }
func (p Payment) IdempotencyKey() string { return p.idempotencyKey }
