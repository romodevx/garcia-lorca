package domain

import (
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// RequireFullyApplied is the rule a deposit is taken by (CC-202, QA
// 2026-10-17): every peso lands on a charge, or the deposit is not taken.
//
// The school used to keep what was left as the family's credit, but nothing
// ever applied it: the cut counted it, the statement read "a favor" and still
// asked "a pagar hoy". The realistic way in was two cashiers taking the same
// month at once — the second one, planned under the lock after the first
// committed, found it paid and kept the money unapplied. Paying AHEAD is not
// credit: months not yet open are charges, and money lands on them.
//
// depositCents is what was handed over, unappliedCents what no charge took;
// chosen says the cashier restricted the money to some charges. A
// KindConflict, worded for the cashier: it is the state of the account, not
// the request, that refuses the money.
func RequireFullyApplied(depositCents, unappliedCents int64, chosen bool) error {
	if unappliedCents <= 0 {
		return nil
	}
	applicable := Cents(depositCents - unappliedCents).Pesos()
	switch {
	case unappliedCents == depositCents && chosen:
		return glerrors.New(glerrors.KindConflict,
			"lo elegido ya está pagado: actualiza la cuenta del alumno antes de cobrar")
	case unappliedCents == depositCents:
		return glerrors.New(glerrors.KindConflict, "el alumno no tiene adeudos que cobrar")
	case chosen:
		return glerrors.New(glerrors.KindConflict,
			"el monto excede lo que deben los conceptos elegidos (máximo "+applicable+")")
	default:
		return glerrors.New(glerrors.KindConflict,
			"el monto excede lo que el alumno debe (máximo "+applicable+"), incluido lo que puede pagar por adelantado")
	}
}

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
// Whatever is left when the debts run out is returned, not refused: the
// caller decides. A deposit refuses it (RequireFullyApplied, CC-202) — money no
// charge claimed was stored as a credit nothing ever applied.
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
