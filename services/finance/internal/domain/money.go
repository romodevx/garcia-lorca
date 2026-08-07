package domain

import (
	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Cents is a money amount in integer minor units (MXN centavos). Money is never
// a float in this service: binary floating point cannot represent decimal money
// exactly, and a ledger that rounds is a ledger that is wrong.
//
// Cents is non-negative by construction — it models an amount, not a movement.
// Direction (charge vs. payment) is carried by the entity that holds it, and
// zero is legal (a fully discounted charge, a free concept). Amounts that must
// be strictly positive, such as a payment, enforce that at their own
// constructor.
type Cents int64

// NewCents builds a validated Cents from a raw minor-unit value, rejecting
// negatives as KindInvalid.
func NewCents(v int64) (Cents, error) {
	if v < 0 {
		return 0, glerrors.New(glerrors.KindInvalid, "amount cannot be negative")
	}
	return Cents(v), nil
}

// Int64 returns the raw minor-unit value, for adapters and arithmetic.
func (c Cents) Int64() int64 { return int64(c) }
