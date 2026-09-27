package domain

import (
	"errors"
	"sort"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// monthLayout is the wire form of a calendar month: "2026-09".
const monthLayout = "2006-01"

// ParseMonth reads a "YYYY-MM" month into its first day, in UTC. Anything else
// (a day, a one-digit month, month 13) is KindInvalid.
func ParseMonth(raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	m, err := time.Parse(monthLayout, value)
	if err != nil || m.Format(monthLayout) != value {
		return time.Time{}, glerrors.New(glerrors.KindInvalid, "Mes inválido (usa AAAA-MM): "+value)
	}
	return m, nil
}

// FormatMonth renders a month as "YYYY-MM".
func FormatMonth(m time.Time) string { return m.Format(monthLayout) }

// isFirstOfMonth reports whether m is midnight UTC on day 1 — the only shape a
// month takes in the domain.
func isFirstOfMonth(m time.Time) bool {
	return m.Equal(time.Date(m.Year(), m.Month(), 1, 0, 0, 0, 0, time.UTC))
}

// Months returns the calendar months the term covers (first day of each, UTC),
// sorted ascending. The slice is a copy.
func (t Term) Months() []time.Time {
	return append([]time.Time(nil), t.months...)
}

// WithMonths returns a copy of t covering exactly months (first day of each),
// sorted and de-duplicated. An empty set clears the term. Whether a month lies
// in the cycle, or already belongs to a sibling term, needs the cycle and the
// siblings, so the use case and the repository decide those.
func (t Term) WithMonths(months []time.Time) (Term, error) {
	seen := make(map[time.Time]struct{}, len(months))
	out := make([]time.Time, 0, len(months))
	for _, m := range months {
		if !isFirstOfMonth(m) {
			return Term{}, glerrors.New(glerrors.KindInvalid, "a term month must be the first day of the month")
		}
		key := m.UTC()
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	t.months = out
	return t, nil
}

// ContainsMonth reports whether any day of month m falls inside the cycle, so a
// cycle starting on 24 August still owns August.
func (c Cycle) ContainsMonth(m time.Time) bool {
	firstOfStart := time.Date(c.startsOn.Year(), c.startsOn.Month(), 1, 0, 0, 0, 0, time.UTC)
	return !m.Before(firstOfStart) && !m.After(c.endsOn)
}

// RefusalCode is the machine-readable name of a refusal: the sentence is for
// the person, the code for the screen (it must mark the offending month without
// parsing Spanish). The gRPC edge sends it as X-Error-Code error metadata.
type RefusalCode string

// The refusal codes of the term-months surface.
const (
	CodeMonthInOtherTerm  RefusalCode = "MONTH_IN_OTHER_TERM"
	CodeMonthOutsideCycle RefusalCode = "MONTH_OUTSIDE_CYCLE"
)

// RefusalError is a typed domain error that also names its RefusalCode. It
// unwraps to the *glerrors.Error, so KindOf and SafeMessage read it as any
// other typed error.
type RefusalError struct {
	Code RefusalCode
	err  *glerrors.Error
}

func refusal(kind glerrors.Kind, code RefusalCode, msg string) error {
	return &RefusalError{Code: code, err: glerrors.New(kind, msg)}
}

// Error implements error.
func (e *RefusalError) Error() string { return e.err.Error() }

// Unwrap exposes the typed error for errors.As / KindOf / SafeMessage.
func (e *RefusalError) Unwrap() error { return e.err }

// RefusalCodeOf is err's refusal code, or "" when it carries none.
func RefusalCodeOf(err error) RefusalCode {
	var r *RefusalError
	if errors.As(err, &r) {
		return r.Code
	}
	return ""
}

// MonthInOtherTerm is the KindConflict refusal for a month another term of the
// same cycle and level already holds. A zero month (the adapter lost the race
// to a concurrent writer and cannot name it) is left out of the sentence.
func MonthInOtherTerm(m time.Time) error {
	msg := "Ese mes ya pertenece a otro periodo del nivel"
	if !m.IsZero() {
		msg += ": " + FormatMonth(m)
	}
	return refusal(glerrors.KindConflict, CodeMonthInOtherTerm, msg)
}

// MonthOutsideCycle is the KindInvalid refusal for a month no day of which lies
// in the term's cycle.
func MonthOutsideCycle(m time.Time) error {
	return refusal(glerrors.KindInvalid, CodeMonthOutsideCycle, "El mes está fuera del ciclo: "+FormatMonth(m))
}

// FirstMonthInOtherTerm is the earliest month of t that another term of the
// same cycle and level already holds, if any: a month belongs to at most one
// term per (cycle, level). Terms of other cycles or levels, and t itself, are
// ignored.
func FirstMonthInOtherTerm(t Term, siblings []Term) (time.Time, bool) {
	taken := make(map[time.Time]struct{})
	for _, s := range siblings {
		if s.id == t.id || s.cycleID != t.cycleID || s.levelID != t.levelID {
			continue
		}
		for _, m := range s.months {
			taken[m] = struct{}{}
		}
	}
	for _, m := range t.months {
		if _, clash := taken[m]; clash {
			return m, true
		}
	}
	return time.Time{}, false
}
