package domain

import (
	"time"

	"github.com/garcia-lorca/pkg/schoolcal"
)

// EvaluationPeriod is grades' view of one evaluation period of a GRADE in a
// cycle (T14, owned by config — ADR 0029): a trimester, a bimester… with real
// dates. Grades are captured per period (T15). StartsOn and EndsOn are
// inclusive calendar days held at midnight UTC.
type EvaluationPeriod struct {
	ID           string
	GradeLevelID string
	Ordinal      int
	Name         string
	StartsOn     time.Time
	EndsOn       time.Time
}

// SchoolDay is the calendar day `now` falls on in the school's time zone,
// held at midnight UTC so it compares with a period's dates. The school's day
// is America/Mexico_City's (SCHOOL_TIMEZONE), never the UTC day: at 20:00 on
// Nov 30 the school is still on Nov 30.
func SchoolDay(now time.Time, loc *time.Location) time.Time {
	return schoolcal.Date(now, loc)
}

// Contains reports whether day (a SchoolDay) is inside the period.
func (p EvaluationPeriod) Contains(day time.Time) bool {
	return p.StartedBy(day) && !p.EndedBefore(day)
}

// StartedBy reports whether the period has begun on day.
func (p EvaluationPeriod) StartedBy(day time.Time) bool { return !day.Before(p.StartsOn) }

// EndedBefore reports whether the period was over before day.
func (p EvaluationPeriod) EndedBefore(day time.Time) bool { return p.EndsOn.Before(day) }

// PeriodOn is the period containing day; false when day falls in none.
func PeriodOn(periods []EvaluationPeriod, day time.Time) (EvaluationPeriod, bool) {
	for _, p := range periods {
		if p.Contains(day) {
			return p, true
		}
	}
	return EvaluationPeriod{}, false
}

// DefaultPeriod is the period a capture sheet opens on: the one containing
// day; between two periods, the latest one that already started; before the
// first, the first. false when there are no periods.
func DefaultPeriod(periods []EvaluationPeriod, day time.Time) (EvaluationPeriod, bool) {
	if p, ok := PeriodOn(periods, day); ok {
		return p, true
	}
	var best EvaluationPeriod
	found := false
	for _, p := range periods {
		if p.StartedBy(day) && (!found || p.StartsOn.After(best.StartsOn)) {
			best, found = p, true
		}
	}
	if found {
		return best, true
	}
	for _, p := range periods {
		if !found || p.StartsOn.Before(best.StartsOn) {
			best, found = p, true
		}
	}
	return best, found
}

// FindPeriod is the period with this id among periods.
func FindPeriod(periods []EvaluationPeriod, id string) (EvaluationPeriod, bool) {
	for _, p := range periods {
		if p.ID == id {
			return p, true
		}
	}
	return EvaluationPeriod{}, false
}
