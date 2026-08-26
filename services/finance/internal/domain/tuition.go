package domain

import (
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// maxDueDay caps the day of the month a payment may fall due.
//
// 28 and not 31: February has no 29th in three years out of four, so a school
// that set the 31st would have its deadline moved by the calendar, silently, for
// one month a year. A deadline that moves on its own is worse than one the
// school cannot set, so this is refused at the boundary rather than adjusted.
const maxDueDay = 28

const monthsInYear = 12

// TuitionPlanSpec is what a school states about one level's cost in one cycle.
// It is a struct and not eight positional parameters because six of the eight
// are numbers, and a caller that swaps two of them would compile.
type TuitionPlanSpec struct {
	ID       string
	TenantID string
	CycleID  string
	LevelID  string

	EnrollmentCents int64
	MonthlyCents    int64

	// Months is how many monthly payments the year has; FirstBilledMonth is the
	// calendar month (1-12) the first one falls in. Ten from September is the
	// common Mexican shape — September through June, with August covered by the
	// inscription.
	Months           int
	FirstBilledMonth int

	DueDay       int
	LateFeeCents int64
	Active       bool
}

// TuitionPlan is what one level costs in one cycle: the inscription, the monthly
// fee, how many months there are, when each falls due, and what a late one costs.
//
// It hangs off (cycle, level) rather than level alone because a school raises
// tuition every year. Were the price attached to the level, raising it would
// rewrite the past: last year's charges would no longer reconcile against their
// own table and nobody could audit what was charged, or why.
type TuitionPlan struct {
	id       string
	tenantID string
	cycleID  string
	levelID  string

	enrollmentCents int64
	monthlyCents    int64

	months           int
	firstBilledMonth int

	dueDay       int
	lateFeeCents int64
	active       bool
}

// NewTuitionPlan validates a school's statement of what a level costs.
func NewTuitionPlan(spec TuitionPlanSpec) (TuitionPlan, error) {
	if strings.TrimSpace(spec.CycleID) == "" {
		return TuitionPlan{}, glerrors.New(glerrors.KindInvalid, "a tuition table belongs to a school year")
	}
	if strings.TrimSpace(spec.LevelID) == "" {
		return TuitionPlan{}, glerrors.New(glerrors.KindInvalid, "a tuition table belongs to a level")
	}
	if spec.EnrollmentCents < 0 || spec.MonthlyCents < 0 || spec.LateFeeCents < 0 {
		return TuitionPlan{}, glerrors.New(glerrors.KindInvalid, "an amount cannot be negative")
	}
	if spec.Months < 1 || spec.Months > monthsInYear {
		return TuitionPlan{}, glerrors.New(glerrors.KindInvalid, "a school year has between 1 and 12 monthly payments")
	}
	if spec.FirstBilledMonth < 1 || spec.FirstBilledMonth > monthsInYear {
		return TuitionPlan{}, glerrors.New(glerrors.KindInvalid, "the first billed month is a month of the year")
	}
	if spec.DueDay < 1 || spec.DueDay > maxDueDay {
		return TuitionPlan{}, glerrors.New(glerrors.KindInvalid,
			"the due day must be between 1 and 28, so it exists in February too")
	}
	return TuitionPlan{
		id: spec.ID, tenantID: spec.TenantID, cycleID: spec.CycleID, levelID: spec.LevelID,
		enrollmentCents: spec.EnrollmentCents, monthlyCents: spec.MonthlyCents,
		months: spec.Months, firstBilledMonth: spec.FirstBilledMonth,
		dueDay: spec.DueDay, lateFeeCents: spec.LateFeeCents, active: spec.Active,
	}, nil
}

// ID identifies the plan.
func (p TuitionPlan) ID() string { return p.id }

// TenantID is the school the plan belongs to.
func (p TuitionPlan) TenantID() string { return p.tenantID }

// CycleID is the school year the plan prices.
func (p TuitionPlan) CycleID() string { return p.cycleID }

// LevelID is the level the plan prices.
func (p TuitionPlan) LevelID() string { return p.levelID }

// EnrollmentCents is what joining the level costs, once per cycle.
func (p TuitionPlan) EnrollmentCents() int64 { return p.enrollmentCents }

// MonthlyCents is what one month of the level costs.
func (p TuitionPlan) MonthlyCents() int64 { return p.monthlyCents }

// Months is how many monthly payments the year has.
func (p TuitionPlan) Months() int { return p.months }

// FirstBilledMonth is the calendar month (1-12) of the first payment.
func (p TuitionPlan) FirstBilledMonth() int { return p.firstBilledMonth }

// DueDay is the day of the month a payment falls due.
func (p TuitionPlan) DueDay() int { return p.dueDay }

// LateFeeCents is the fixed surcharge added once per completed overdue month.
func (p TuitionPlan) LateFeeCents() int64 { return p.lateFeeCents }

// Active reports whether the run should use this plan.
func (p TuitionPlan) Active() bool { return p.active }

// BilledMonths turns "ten payments from September" into the ten actual due
// dates, walking forward from the first billed month on or after the cycle
// starts — which is how a year that opens in August reaches June of the NEXT
// calendar year without anybody writing the year down twice.
//
// It returns dates, not months, because a due date is what a charge needs and
// deriving it later would mean re-deciding the day-of-month rule in two places.
func (p TuitionPlan) BilledMonths(cycleStart time.Time) []time.Time {
	year := cycleStart.Year()
	if int(cycleStart.Month()) > p.firstBilledMonth {
		// The first billed month has already passed this calendar year, so the
		// school means the one that comes after the year turns.
		year++
	}
	first := time.Date(year, time.Month(p.firstBilledMonth), p.dueDay, 0, 0, 0, 0, cycleStart.Location())

	out := make([]time.Time, 0, p.months)
	for i := 0; i < p.months; i++ {
		out = append(out, first.AddDate(0, i, 0))
	}
	return out
}

// OverdueMonths counts COMPLETED months between a due date and a given day.
//
// Completed months and not days, because the surcharge rule is monthly: running
// the job on the 3rd or on the 27th has to produce the same answer, or a
// family's debt would depend on when somebody happened to press a button.
// Anything not yet due counts as zero.
func OverdueMonths(due, asOf time.Time) int {
	if !asOf.After(due) {
		return 0
	}
	months := (asOf.Year()-due.Year())*monthsInYear + int(asOf.Month()) - int(due.Month())
	if asOf.Day() < due.Day() {
		// The calendar month turned but the day of the month has not arrived, so
		// the latest month is not complete.
		months--
	}
	if months < 0 {
		return 0
	}
	return months
}
