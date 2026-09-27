package domain

import (
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// monthLayout is how a month travels on the wire and in URLs ("2026-09").
const monthLayout = "2006-01"

// Month is a calendar month of the school year, held as its first day at
// midnight UTC — the same instant the monthly_grades.month date column stores.
type Month struct {
	first time.Time
}

// ParseMonth reads "YYYY-MM"; anything else is KindInvalid.
func ParseMonth(raw string) (Month, error) {
	t, err := time.Parse(monthLayout, raw)
	if err != nil || len(raw) != len(monthLayout) {
		return Month{}, glerrors.New(glerrors.KindInvalid, "El mes debe tener el formato AAAA-MM")
	}
	return Month{first: t}, nil
}

// MonthOf is the month an instant falls in, judged on the UTC calendar (the
// school day is the UTC day across the platform).
func MonthOf(t time.Time) Month {
	u := t.UTC()
	return Month{first: time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)}
}

// FirstDay is the month's first day at midnight UTC.
func (m Month) FirstDay() time.Time { return m.first }

// IsZero reports whether m is the zero Month (no month at all).
func (m Month) IsZero() bool { return m.first.IsZero() }

// String is "YYYY-MM".
func (m Month) String() string { return m.first.Format(monthLayout) }

// CycleMonths lists every month a cycle touches, from the month of start to
// the month of end inclusive. A cycle without dates (either zero) or with an
// inverted range has no months.
func CycleMonths(start, end time.Time) []Month {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return nil
	}
	var out []Month
	for m, last := MonthOf(start), MonthOf(end); !m.first.After(last.first); m = m.next() {
		out = append(out, m)
	}
	return out
}

func (m Month) next() Month { return Month{first: m.first.AddDate(0, 1, 0)} }

// ContainsMonth reports whether m is one of months.
func ContainsMonth(months []Month, m Month) bool {
	for _, c := range months {
		if c.first.Equal(m.first) {
			return true
		}
	}
	return false
}

// ClampMonth is the month of now pulled into months: the first month before the
// cycle starts, the last one after it ends. ok is false when months is empty.
func ClampMonth(months []Month, now time.Time) (Month, bool) {
	if len(months) == 0 {
		return Month{}, false
	}
	current := MonthOf(now)
	first, last := months[0], months[len(months)-1]
	switch {
	case current.first.Before(first.first):
		return first, true
	case current.first.After(last.first):
		return last, true
	default:
		return current, true
	}
}
