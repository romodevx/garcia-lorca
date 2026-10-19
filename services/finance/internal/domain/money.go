package domain

import (
	"fmt"
	"strconv"

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

// MaxCents is the inclusive upper bound of a single money amount: 10^15 centavos
// (ten trillion pesos), far beyond any real charge or payment yet small enough
// that summing a realistic number of them stays well inside int64. The bound is
// what makes the payment knot safe: without it a near-MaxInt64 payment would
// overflow `currentPaid + amount` into a negative, slipping past the
// "payment exceeds the remaining balance" veto and both corrupting the ledger
// and poisoning every later SUM(amount_cents) with a bigint-out-of-range error.
const MaxCents int64 = 1_000_000_000_000_000

// NewCents builds a validated Cents from a raw minor-unit value, rejecting
// negatives and amounts above MaxCents as KindInvalid.
func NewCents(v int64) (Cents, error) {
	if v < 0 {
		return 0, glerrors.New(glerrors.KindInvalid, "amount cannot be negative")
	}
	if v > MaxCents {
		return 0, glerrors.New(glerrors.KindInvalid, "amount exceeds the maximum allowed")
	}
	return Cents(v), nil
}

// Int64 returns the raw minor-unit value, for adapters and arithmetic.
func (c Cents) Int64() int64 { return int64(c) }

// centsPerPeso is how many minor units make a peso.
const centsPerPeso = 100

// thousandsGroup is how many digits a thousands separator groups.
const thousandsGroup = 3

// Pesos is the amount as a person reads it in a message: "$32,300.00". Money
// in a sentence the cashier reads must look like the money on the screen.
func (c Cents) Pesos() string {
	digits := strconv.FormatInt(int64(c)/centsPerPeso, 10)
	grouped := make([]byte, 0, len(digits)+len(digits)/thousandsGroup)
	for i := range len(digits) {
		if i > 0 && (len(digits)-i)%thousandsGroup == 0 {
			grouped = append(grouped, ',')
		}
		grouped = append(grouped, digits[i])
	}
	return fmt.Sprintf("$%s.%02d", grouped, int64(c)%centsPerPeso)
}
