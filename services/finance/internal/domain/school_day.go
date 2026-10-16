package domain

import (
	"time"

	"github.com/garcia-lorca/pkg/schoolcal"
)

// SchoolDate is the school's calendar date at instant t: the wall-clock day in
// zone (America/Mexico_City, resolved once at boot from SCHOOL_TIMEZONE),
// written as midnight UTC.
//
// Midnight UTC because that is how every date finance stores already reads —
// due_date, starts_on, a counter-dated paid_at, a parsed "2026-10-15" — so the
// answer compares exactly with them. Only the question "which day is it NOW"
// moves to the school's zone (ADR 0035); a stored date is never re-zoned.
//
// Before ADR 0035 the day was the UTC one, so after 18:00 in Morelia the
// service already charged tomorrow's surcharge and closed today's early-payment
// window. Every "today" in finance goes through here, never through a local
// truncation of its own.
func SchoolDate(t time.Time, zone *time.Location) time.Time {
	return schoolcal.Date(t, zone)
}
