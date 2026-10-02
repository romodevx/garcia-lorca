package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// MaxEvaluationPeriods is the most periods one grade may have in a cycle. A
// school evaluates by trimester (3), bimester (5) or at most month by month.
const MaxEvaluationPeriods = 12

// MaxPeriodNameLen bounds a period's name ("1.er trimestre"), in characters.
const MaxPeriodNameLen = 60

// The refusal codes of the evaluation-period surface (T14). The screen marks
// the offending row by code, never by parsing Spanish.
const (
	CodePeriodNameRequired    RefusalCode = "PERIOD_NAME_REQUIRED"
	CodePeriodEndsBeforeStart RefusalCode = "PERIOD_ENDS_BEFORE_START"
	CodePeriodOutsideCycle    RefusalCode = "PERIOD_OUTSIDE_CYCLE"
	CodePeriodOverlap         RefusalCode = "PERIOD_OVERLAP"
	CodePeriodOrdinalTaken    RefusalCode = "PERIOD_ORDINAL_TAKEN"
	CodeTooManyPeriods        RefusalCode = "TOO_MANY_PERIODS"
	CodePeriodsChanged        RefusalCode = "PERIODS_CHANGED"
)

// PeriodsChangedMeanwhile is the KindConflict refusal for a save that lost a
// race with another save of the same grade: reload and try again.
func PeriodsChangedMeanwhile() error {
	return refusal(glerrors.KindConflict, CodePeriodsChanged,
		"Alguien más guardó los periodos de este grado al mismo tiempo; recarga y vuelve a intentarlo")
}

// EvaluationPeriod is one evaluation period (a trimester, a bimester) of ONE
// grade in ONE cycle, with the calendar dates it spans, both inclusive. Grades
// of a level may evaluate on different calendars, so the period belongs to the
// grade, not to the level (T14).
//
// termID links a period migrated from the level's term (0016) to that term;
// it is provenance only. Grades still close periods and issue report cards per
// term; T15 maps a period to its term by (cycle, level of the grade, ordinal).
type EvaluationPeriod struct {
	id           string
	tenantID     string
	cycleID      string
	gradeLevelID string
	name         string
	ordinal      int
	startsOn     time.Time
	endsOn       time.Time
	termID       string
}

// NewEvaluationPeriod constructs a period. Ids are required; the name is
// trimmed and required (at most MaxPeriodNameLen characters); ordinal is
// 1..MaxEvaluationPeriods; only the calendar date of startsOn/endsOn counts
// (in the zone they carry) and endsOn must not be before startsOn — a one-day
// period is valid.
func NewEvaluationPeriod(id, tenantID, cycleID, gradeLevelID, name string, ordinal int, startsOn, endsOn time.Time) (EvaluationPeriod, error) {
	for _, required := range []struct{ value, what string }{
		{id, "period id"}, {tenantID, "period tenant id"}, {cycleID, "period cycle id"}, {gradeLevelID, "period grade level id"},
	} {
		if strings.TrimSpace(required.value) == "" {
			return EvaluationPeriod{}, glerrors.New(glerrors.KindInvalid, required.what+" is required")
		}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return EvaluationPeriod{}, refusal(glerrors.KindInvalid, CodePeriodNameRequired, "El periodo necesita un nombre")
	}
	if utf8.RuneCountInString(name) > MaxPeriodNameLen {
		return EvaluationPeriod{}, glerrors.New(glerrors.KindInvalid, fmt.Sprintf("El nombre del periodo admite hasta %d caracteres", MaxPeriodNameLen))
	}
	if ordinal < 1 || ordinal > MaxEvaluationPeriods {
		return EvaluationPeriod{}, glerrors.New(glerrors.KindInvalid, fmt.Sprintf("El número de periodo va de 1 a %d", MaxEvaluationPeriods))
	}
	from, to := DateOnly(startsOn), DateOnly(endsOn)
	if to.Before(from) {
		return EvaluationPeriod{}, refusal(glerrors.KindInvalid, CodePeriodEndsBeforeStart,
			fmt.Sprintf("«%s» termina antes de empezar", name))
	}
	return EvaluationPeriod{
		id: id, tenantID: tenantID, cycleID: cycleID, gradeLevelID: gradeLevelID,
		name: name, ordinal: ordinal, startsOn: from, endsOn: to,
	}, nil
}

// DateOnly is t's calendar date (in t's own zone) as midnight UTC, the one
// shape a date takes in the domain.
func DateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// WithTermID returns a copy of p linked to the term it was derived from.
func (p EvaluationPeriod) WithTermID(termID string) EvaluationPeriod {
	p.termID = termID
	return p
}

// ID returns the stable period identifier.
func (p EvaluationPeriod) ID() string { return p.id }

// TenantID returns the tenant the period belongs to.
func (p EvaluationPeriod) TenantID() string { return p.tenantID }

// CycleID returns the cycle the period belongs to.
func (p EvaluationPeriod) CycleID() string { return p.cycleID }

// GradeLevelID returns the grade the period belongs to.
func (p EvaluationPeriod) GradeLevelID() string { return p.gradeLevelID }

// Name returns the period's name ("1.er trimestre").
func (p EvaluationPeriod) Name() string { return p.name }

// Ordinal returns the 1-based position of the period within its grade.
func (p EvaluationPeriod) Ordinal() int { return p.ordinal }

// StartsOn returns the first day of the period (midnight UTC).
func (p EvaluationPeriod) StartsOn() time.Time { return p.startsOn }

// EndsOn returns the last day of the period, inclusive (midnight UTC).
func (p EvaluationPeriod) EndsOn() time.Time { return p.endsOn }

// TermID returns the term the period was migrated from, or "".
func (p EvaluationPeriod) TermID() string { return p.termID }

// Contains reports whether the calendar date of on lies in the period.
func (p EvaluationPeriod) Contains(on time.Time) bool {
	d := DateOnly(on)
	return !d.Before(p.startsOn) && !d.After(p.endsOn)
}

// CheckPeriodSet validates the complete set of periods of ONE grade in cycle:
// at most MaxEvaluationPeriods, every ordinal once, every period inside the
// cycle, and no two periods sharing a day. Sorting by start and comparing
// neighbours finds any overlap in O(n log n).
func CheckPeriodSet(cycle Cycle, periods []EvaluationPeriod) error {
	if len(periods) > MaxEvaluationPeriods {
		return refusal(glerrors.KindInvalid, CodeTooManyPeriods,
			fmt.Sprintf("Un grado puede tener hasta %d periodos", MaxEvaluationPeriods))
	}
	seen := make(map[int]struct{}, len(periods))
	for _, p := range periods {
		if _, dup := seen[p.ordinal]; dup {
			return refusal(glerrors.KindInvalid, CodePeriodOrdinalTaken,
				fmt.Sprintf("El periodo %d está repetido", p.ordinal))
		}
		seen[p.ordinal] = struct{}{}
		if p.startsOn.Before(DateOnly(cycle.StartsOn())) || p.endsOn.After(DateOnly(cycle.EndsOn())) {
			return refusal(glerrors.KindInvalid, CodePeriodOutsideCycle,
				fmt.Sprintf("«%s» queda fuera del ciclo (%s a %s)", p.name,
					cycle.StartsOn().Format(time.DateOnly), cycle.EndsOn().Format(time.DateOnly)))
		}
	}
	byStart := append([]EvaluationPeriod(nil), periods...)
	sort.Slice(byStart, func(i, j int) bool { return byStart[i].startsOn.Before(byStart[j].startsOn) })
	for i := 1; i < len(byStart); i++ {
		prev, next := byStart[i-1], byStart[i]
		if !next.startsOn.After(prev.endsOn) {
			return PeriodOverlap(prev, next)
		}
	}
	return nil
}

// PeriodOverlap is the KindConflict refusal for two periods of one grade that
// share at least one day. With a zero b (the adapter lost a race and cannot
// name the other period) only a is named.
func PeriodOverlap(a, b EvaluationPeriod) error {
	msg := "Los periodos de un grado no pueden encimarse"
	if a.name != "" && b.name != "" {
		msg = fmt.Sprintf("El periodo %d (%s) se encima con el periodo %d (%s)", a.ordinal, a.name, b.ordinal, b.name)
	}
	return refusal(glerrors.KindConflict, CodePeriodOverlap, msg)
}

// PeriodOn is the period of gradeLevelID that contains the calendar date of on
// (in on's own zone: pass the school's local date). Periods of other grades are
// ignored; a date in no period (a gap, a vacation, outside the cycle) answers
// false. A grade has a handful of periods, so a linear scan is the right tool.
func PeriodOn(periods []EvaluationPeriod, gradeLevelID string, on time.Time) (EvaluationPeriod, bool) {
	for _, p := range periods {
		if p.gradeLevelID == gradeLevelID && p.Contains(on) {
			return p, true
		}
	}
	return EvaluationPeriod{}, false
}
