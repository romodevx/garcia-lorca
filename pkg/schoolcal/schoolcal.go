// Package schoolcal answers "is this a working day" and the arithmetic that
// depends on it.
//
// WHY THIS EXISTS. Nothing in this repository could compute a working day. The
// uniforms service carries a pickup_window_business_days setting and a
// deadlineEstimated flag whose whole purpose is to record "the calendar was
// unreachable" — for a calendar that was never built. Its migration says the
// school calendar "deliberately lives in the config service", and config had no
// calendar either. The seam was designed and left empty.
//
// Everything the school asked for rests on this: a late fee counted in days, an
// early-payment window of "the first five working days", a due date that moves
// off a holiday keeping its benefit or its cost.
//
// It is deliberately PURE — a set of dates in, answers out. No database, no
// clock, no service calls. Whoever owns the days (config) loads them; this only
// does the arithmetic, so both finance and uniforms can share one implementation
// instead of drifting apart with two.
package schoolcal

import "time"

// Calendar answers working-day questions against a fixed set of non-working
// days. The zero value is not useful; build one with New.
//
// Not safe for concurrent modification — it is immutable after New, which is
// what makes it safe to share and to cache.
type Calendar struct {
	holidays map[string]struct{}
}

// New builds a calendar from the school's non-working days. Times of day are
// discarded: a holiday is a day, not an instant. Duplicates are harmless.
//
// A nil or empty set is valid and means "weekends only", which is the correct
// floor for a school that has not loaded its calendar yet.
func New(holidays []time.Time) *Calendar {
	set := make(map[string]struct{}, len(holidays))
	for _, h := range holidays {
		set[key(h)] = struct{}{}
	}
	return &Calendar{holidays: set}
}

// IsWorkingDay reports whether the school operates on that date: not a weekend,
// not a loaded holiday. The time of day is ignored.
func (c *Calendar) IsWorkingDay(t time.Time) bool {
	switch t.Weekday() {
	case time.Saturday, time.Sunday:
		return false
	}
	_, isHoliday := c.holidays[key(t)]
	return !isHoliday
}

// NextWorkingDay returns t itself when it is a working day, otherwise the first
// working day after it.
//
// This is the rule the school gave for deadlines: one that lands on a holiday
// moves forward, keeping whatever benefit or cost was attached to it. It walks
// past a RUN of non-working days — a Friday holiday rolls to Monday, not into
// Saturday.
func (c *Calendar) NextWorkingDay(t time.Time) time.Time {
	d := midnight(t)
	// Bounded so a pathological calendar (every day loaded as a holiday) cannot
	// spin forever inside a request. A year of closure is not a calendar problem.
	for i := 0; i < 366; i++ {
		if c.IsWorkingDay(d) {
			return d
		}
		d = d.AddDate(0, 0, 1)
	}
	return d
}

// AddWorkingDays returns the date n working days after t, counting only days the
// school operates. n == 0 returns t rolled forward to a working day, so "the
// deadline is today" is never a day the office is shut.
//
// This is what makes an early-payment window survivable: "the first five working
// days" is a promise the family can keep even when the 5th is a Sunday.
func (c *Calendar) AddWorkingDays(t time.Time, n int) time.Time {
	d := c.NextWorkingDay(t)
	for i := 0; i < n; i++ {
		d = c.NextWorkingDay(d.AddDate(0, 0, 1))
	}
	return d
}

// CalendarDaysBetween counts every day from..to, weekends and holidays included.
// Negative spans are zero: a payment made before its due date is not "minus
// three days late", and a negative count would become a refund nobody approved.
func (c *Calendar) CalendarDaysBetween(from, to time.Time) int {
	days := int(midnight(to).Sub(midnight(from)).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

// WorkingDaysBetween counts only the working days in (from, to].
//
// Both this and CalendarDaysBetween exist because whether a school charges a
// late fee for a Sunday is a POLICY, not a fact. The calendar answers both
// questions and refuses to decide which one a school means.
func (c *Calendar) WorkingDaysBetween(from, to time.Time) int {
	a, b := midnight(from), midnight(to)
	if !b.After(a) {
		return 0
	}
	count := 0
	for d := a.AddDate(0, 0, 1); !d.After(b); d = d.AddDate(0, 0, 1) {
		if c.IsWorkingDay(d) {
			count++
		}
	}
	return count
}

// midnight strips the time of day, keeping the location so a date does not shift
// across a boundary on its way through.
func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// key is the day-granular identity used to match holidays.
func key(t time.Time) string { return t.Format("2006-01-02") }
