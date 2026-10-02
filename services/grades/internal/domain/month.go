package domain

import (
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// monthLayout is how a month travels on the wire and in URLs ("2026-09").
const monthLayout = "2006-01"

// Month is a calendar month, held as its first day at midnight UTC. Since T15
// it only names the columns of report cards issued before period capture
// (report_cards.months), which read back exactly as issued.
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

// FirstDay is the month's first day at midnight UTC.
func (m Month) FirstDay() time.Time { return m.first }

// IsZero reports whether m is the zero Month (no month at all).
func (m Month) IsZero() bool { return m.first.IsZero() }

// String is "YYYY-MM".
func (m Month) String() string { return m.first.Format(monthLayout) }
