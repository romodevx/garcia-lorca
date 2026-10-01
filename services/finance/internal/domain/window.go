package domain

import (
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// ValidateChargeWindow is the one rule about the days a charge is open in (T5):
// it opens on startsOn — the first day the family is shown it and asked for it —
// and falls due on dueOn.
//
// A zero startsOn is a charge that is open from the day it exists, which is
// every charge written before the window existed. A start with no deadline is
// refused rather than completed: the run would have to derive the deadline, and
// a derived date that lands before the start the school typed is a contradiction
// nobody chose.
//
// Written once and read by the plan (where the school types it) and by the
// charge (where the run writes it), so the editor and the ledger cannot drift.
func ValidateChargeWindow(startsOn, dueOn time.Time) error {
	if startsOn.IsZero() {
		return nil
	}
	if dueOn.IsZero() {
		return glerrors.New(glerrors.KindInvalid,
			"una fecha de inicio necesita su fecha límite")
	}
	if startsOn.After(dueOn) {
		return glerrors.New(glerrors.KindInvalid,
			"la fecha de inicio no puede ser posterior a la fecha límite")
	}
	return nil
}

// WithStartsOn returns a copy that opens to the family on startsOn, validated
// against its own deadline. The zero time clears it.
func (c Charge) WithStartsOn(startsOn time.Time) (Charge, error) {
	if err := ValidateChargeWindow(startsOn, c.dueDate); err != nil {
		return Charge{}, err
	}
	c.startsOn = startsOn
	return c, nil
}

// StartsOn is the first day the charge is shown to the family and asked for,
// or the zero time when it is open from the day it was issued.
func (c Charge) StartsOn() time.Time { return c.startsOn }

// OpenOn reports whether the charge has opened by the instant `on`.
//
// Compared as instants against a start stored at UTC midnight, which is this
// service's convention for the school's day (see school-day-is-utc-day): the
// same convention the due date, the surcharge and the price ladder follow.
func (c Charge) OpenOn(on time.Time) bool {
	return c.startsOn.IsZero() || !c.startsOn.After(on)
}
