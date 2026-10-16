package usecase

import (
	"time"

	"github.com/garcia-lorca/services/finance/internal/domain"
)

// SchoolClock is finance's ONE source of time (ADR 0035). It answers two
// different questions and keeps them apart:
//
//   - Now is the instant, to stamp what happened (a payment's paid_at, an
//     event, a void).
//   - Today is the school's calendar date in Morelia (America/Mexico_City),
//     to decide what is due: surcharge days, overdue months, the early-payment
//     window, the overdue cutoff, the run's default date, the "Al {fecha}" of
//     an account.
//
// Every "today" in the service is Today. Truncating Now to a day anywhere else
// would bring back the UTC day this clock replaced, in one calculation only,
// six hours out of step with the rest of the same statement.
type SchoolClock struct {
	now  func() time.Time
	zone *time.Location
}

// NewSchoolClock pairs the instant source with the school's zone, resolved once
// at boot from SCHOOL_TIMEZONE. A missing piece is a wiring bug and panics:
// falling back to UTC would be the very bug this clock removes.
func NewSchoolClock(now func() time.Time, zone *time.Location) SchoolClock {
	if now == nil || zone == nil {
		panic("usecase: a SchoolClock needs both an instant source and the school's zone")
	}
	return SchoolClock{now: now, zone: zone}
}

// Now is the current instant.
func (c SchoolClock) Now() time.Time { return c.now() }

// Today is the school's calendar date now, as midnight UTC like every stored
// finance date.
func (c SchoolClock) Today() time.Time { return domain.SchoolDate(c.now(), c.zone) }

// DayOf is the school's calendar date of an instant, as midnight UTC. The zero
// time stays zero: "not recorded" is not 1 January of year 1 in Morelia.
func (c SchoolClock) DayOf(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return domain.SchoolDate(t, c.zone)
}
