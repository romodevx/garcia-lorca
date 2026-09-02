package domain

import "time"

// Payable is one thing a deposit can be applied to: a charge with something
// still owed on it, or the surcharge that charge has earned.
//
// The planner does not decide the order. It is handed the items already sorted
// — by due date, then id — because the order has to be the same every time the
// same deposit is replayed, and a sort chosen here could not be seen by the
// caller writing the ledger.
type Payable struct {
	ChargeID  string
	DueOn     time.Time
	OwedCents int64
	// Surcharge marks an item that exists because a charge was paid late. The
	// planner treats it like any other debt; the caller needs the distinction to
	// know which rows it must materialize before writing the allocation.
	Surcharge bool
}

// Allocation is one item of a deposit landing on one charge.
type Allocation struct {
	ChargeID    string
	AmountCents int64
	Surcharge   bool
}

// Allocate splits a deposit across what a family owes, oldest first, filling
// each item completely before moving to the next.
//
// This is the school's rule stated plainly: a parent hands over an amount and
// says nothing else, and the money lands on the oldest thing they owe. Spreading
// it thinly across every open month instead would leave nothing settled, which
// means nothing stops accruing surcharge and no receipt can say what was paid.
//
// Whatever is left when the debts run out is returned rather than refused. A
// family that overpays keeps a credit; sending a parent away with cash in hand
// because the arithmetic came out even is not a thing a school does.
func Allocate(depositCents int64, items []Payable) ([]Allocation, int64) {
	remaining := depositCents
	var out []Allocation
	for _, item := range items {
		if remaining <= 0 {
			break
		}
		if item.OwedCents <= 0 {
			// Already covered. Allocating zero would put a line in the family's
			// statement that means nothing and has to be explained.
			continue
		}
		amount := item.OwedCents
		if amount > remaining {
			amount = remaining
		}
		out = append(out, Allocation{
			ChargeID:    item.ChargeID,
			AmountCents: amount,
			Surcharge:   item.Surcharge,
		})
		remaining -= amount
	}
	return out, remaining
}
