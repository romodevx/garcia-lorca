package domain

import (
	"fmt"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/schoolcal"
)

// shortSpanishMonths are the abbreviations the office reads ("24 ago 2026").
var shortSpanishMonths = [...]string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"}

// SchoolDate is the calendar date of an instant in the school's zone,
// represented at midnight UTC like a parsed YYYY-MM-DD or a stored date
// column, so comparisons against those stay exact (ADR 0035).
func SchoolDate(t time.Time, zone *time.Location) time.Time {
	return schoolcal.Date(t, zone)
}

// ShortSpanishDate formats a calendar date as the office writes it: "24 ago 2026".
func ShortSpanishDate(d time.Time) string {
	return fmt.Sprintf("%d %s %d", d.Day(), shortSpanishMonths[d.Month()-1], d.Year())
}

// EntryInstant is when an enrolment with a chosen entry day starts.
//
// on, today and cycleStart are calendar dates (midnight UTC). The day must lie
// between the cycle's first day and the school's today, both inclusive: before
// the cycle there is nothing to join, and a future day would bill a month the
// pupil has not reached. The result is 00:00 of that day in the school's zone,
// so every reader that takes the school day of enrolled_at — finance's billing
// first of all — lands on the day the office typed, never the day before.
func EntryInstant(on, today, cycleStart time.Time, zone *time.Location) (time.Time, error) {
	start := calendarDay(cycleStart)
	if on.Before(start) || on.After(today) {
		return time.Time{}, glerrors.New(glerrors.KindInvalid, fmt.Sprintf(
			"La fecha de ingreso debe estar entre el inicio del ciclo (%s) y hoy.", ShortSpanishDate(start)))
	}
	return time.Date(on.Year(), on.Month(), on.Day(), 0, 0, 0, 0, zone), nil
}

// calendarDay is the UTC date of a stored date (config's starts_on travels as
// midnight UTC).
func calendarDay(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}
