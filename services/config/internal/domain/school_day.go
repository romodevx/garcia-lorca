package domain

import (
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Non-working period kinds. Two, because a school distinguishes a public holiday
// it does not choose from a vacation it does, and the difference shows up in the
// activity calendar even though the billing rules treat them identically.
const (
	KindHoliday  = "holiday"
	KindVacation = "vacation"
)

// maxPeriodDays bounds one entry. A vacation is weeks; a school year of closure
// is a typo, and expanding it would build an unbounded slice inside a request.
const maxPeriodDays = 120

// NonWorkingPeriod is a day, or a run of days, on which the school does not
// operate: a public holiday, a vacation, a suspension.
//
// It lives in config rather than finance on purpose. The school asked for one
// calendar serving both the collection rules and its own activities, and
// activities are not a money concern — putting it in finance would have forced
// every other module to ask the billing service what day it is.
//
// Stored as a RANGE and not one row per day: "vacaciones de invierno" is one
// decision a person makes once, and sixteen rows would be sixteen chances for
// half of them to be edited.
type NonWorkingPeriod struct {
	id       string
	tenantID string
	cycleID  string
	label    string
	kind     string
	from     time.Time
	to       time.Time
}

// NewNonWorkingPeriod constructs a period. Both ends are inclusive, and a
// single-day holiday has from == to.
func NewNonWorkingPeriod(id, tenantID, cycleID, label, kind string, from, to time.Time) (NonWorkingPeriod, error) {
	if strings.TrimSpace(id) == "" {
		return NonWorkingPeriod{}, glerrors.New(glerrors.KindInvalid, "non-working period id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return NonWorkingPeriod{}, glerrors.New(glerrors.KindInvalid, "non-working period tenant id is required")
	}
	if strings.TrimSpace(cycleID) == "" {
		return NonWorkingPeriod{}, glerrors.New(glerrors.KindInvalid, "non-working period cycle id is required")
	}
	if strings.TrimSpace(label) == "" {
		// The label is what a person reads on the calendar and in the explanation
		// of why a due date moved. An unnamed closed day cannot be explained.
		return NonWorkingPeriod{}, glerrors.New(glerrors.KindInvalid, "non-working period label is required")
	}
	if kind != KindHoliday && kind != KindVacation {
		return NonWorkingPeriod{}, glerrors.New(glerrors.KindInvalid,
			"non-working period kind must be holiday or vacation")
	}

	f, t := midnight(from), midnight(to)
	if t.Before(f) {
		return NonWorkingPeriod{}, glerrors.New(glerrors.KindInvalid,
			"non-working period end may not precede its start")
	}
	if int(t.Sub(f).Hours()/24)+1 > maxPeriodDays {
		return NonWorkingPeriod{}, glerrors.New(glerrors.KindInvalid,
			"non-working period is longer than a school ever closes; check the dates")
	}

	return NonWorkingPeriod{
		id: id, tenantID: tenantID, cycleID: cycleID,
		label: strings.TrimSpace(label), kind: kind, from: f, to: t,
	}, nil
}

// ID returns the period's identifier.
func (p NonWorkingPeriod) ID() string { return p.id }

// TenantID returns the owning tenant.
func (p NonWorkingPeriod) TenantID() string { return p.tenantID }

// CycleID returns the school cycle the period belongs to.
func (p NonWorkingPeriod) CycleID() string { return p.cycleID }

// Label is the human name shown on the calendar and in the explanation of a
// moved due date ("se recorrió por Independencia").
func (p NonWorkingPeriod) Label() string { return p.label }

// Kind is holiday or vacation.
func (p NonWorkingPeriod) Kind() string { return p.kind }

// From is the first non-working day, inclusive.
func (p NonWorkingPeriod) From() time.Time { return p.from }

// To is the last non-working day, inclusive.
func (p NonWorkingPeriod) To() time.Time { return p.to }

// Days expands the period into the individual days it covers, both ends
// included. This is what pkg/schoolcal consumes: the storage shape is a range
// because that is how a person thinks about it, and the arithmetic wants days.
func (p NonWorkingPeriod) Days() []time.Time {
	out := make([]time.Time, 0, int(p.to.Sub(p.from).Hours()/24)+1)
	for d := p.from; !d.After(p.to); d = d.AddDate(0, 0, 1) {
		out = append(out, d)
	}
	return out
}

func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
