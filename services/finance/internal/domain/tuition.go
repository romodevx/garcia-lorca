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

	// Policy is what this level costs for paying late and saves for paying early.
	// It travels with the plan because a school raises tuition and changes its
	// late-fee rule in the same breath, and last year's charges must still
	// reconcile against last year's rules.
	Policy CollectionPolicy
	// Schedule replaces the uniform monthly payment with named amounts per
	// month. Empty keeps the uniform schedule.
	Schedule []Installment
	// MonthlyDiscountCents comes off every month of this level. Zero is no
	// discount, which is what most levels have.
	MonthlyDiscountCents int64
	// Fees are the extra amounts the school collects on top of the tuition,
	// each spread across the months it chose. Empty is the ordinary state.
	Fees []Fee
	// EnrollmentDueOn is the day the inscription falls due. Zero keeps the rule
	// the run has always used — the plan's due day in the cycle's first month —
	// so a plan written before this field goes on meaning what it meant.
	EnrollmentDueOn time.Time
	// EnrollmentStartsOn is the first day the inscription is shown to families
	// and asked for (T5). Zero is open from the day it is issued. It needs
	// EnrollmentDueOn and may not come after it (ValidateChargeWindow).
	EnrollmentStartsOn time.Time
	// EnrollmentConcepts breaks the inscription into the lines the school prints,
	// each priced by when the family settles it. Empty keeps EnrollmentCents,
	// which is what every plan written before ADR 0011 means.
	EnrollmentConcepts []EnrollmentConcept
}

// Installment is one payment of the year: which calendar month it falls in and
// how much of the year's tuition it collects.
//
// It exists because a school does not always divide the year into equal parts.
// "Once meses, pero diciembre a la mitad y la otra mitad en marzo" is a real
// arrangement — families are short in December — and a plan that can only
// express N equal payments cannot say it.
type Installment struct {
	// Month is 1-12. The year is worked out from the cycle, so a schedule that
	// runs past December continues into the next one.
	Month int
	// AmountCents is what this payment collects. It need not equal any other.
	AmountCents int64
}

// DueInstallment is one installment placed on the calendar.
type DueInstallment struct {
	DueOn       time.Time
	AmountCents int64
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

	dueDay               int
	lateFeeCents         int64
	policy               CollectionPolicy
	schedule             []Installment
	monthlyDiscountCents int64
	fees                 []Fee
	enrollmentDueOn      time.Time
	enrollmentStartsOn   time.Time
	enrollmentConcepts   []EnrollmentConcept
	active               bool
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
	if spec.Policy.DailyMode && spec.Policy.LateFeeDailyCents <= 0 {
		// The same rule the database enforces (tuition_plans_daily_rate_ck).
		// Caught here too so a half-configured plan is refused where the message
		// can say what is missing, not as a constraint violation.
		return TuitionPlan{}, glerrors.New(glerrors.KindInvalid,
			"a daily late-fee rule needs a daily rate, or it charges nothing")
	}
	if spec.Policy.LateFeeDailyCents < 0 || spec.Policy.EarlyPaymentCents < 0 || spec.Policy.EarlyPaymentDays < 0 {
		return TuitionPlan{}, glerrors.New(glerrors.KindInvalid, "an amount cannot be negative")
	}
	if spec.MonthlyDiscountCents < 0 {
		return TuitionPlan{}, glerrors.New(glerrors.KindInvalid, "a discount cannot be negative")
	}
	if spec.MonthlyDiscountCents > spec.MonthlyCents {
		// A charge that owes negative money would pay for itself and then some.
		return TuitionPlan{}, glerrors.New(glerrors.KindInvalid,
			"the discount cannot be bigger than the month it comes off")
	}
	if err := ValidateChargeWindow(spec.EnrollmentStartsOn, spec.EnrollmentDueOn); err != nil {
		return TuitionPlan{}, err
	}
	if len(spec.Fees) > maxFeesPerPlan {
		return TuitionPlan{}, glerrors.New(glerrors.KindInvalid, "that is more fees than a level can carry")
	}
	for _, f := range spec.Fees {
		if err := f.validate(); err != nil {
			return TuitionPlan{}, err
		}
	}
	if len(spec.EnrollmentConcepts) > maxEnrollmentConcepts {
		return TuitionPlan{}, glerrors.New(glerrors.KindInvalid,
			"that is more lines than an inscription can carry")
	}
	seenConcept := make(map[string]bool, len(spec.EnrollmentConcepts))
	for _, c := range spec.EnrollmentConcepts {
		if err := c.validate(); err != nil {
			return TuitionPlan{}, err
		}
		// Compared as the SLUG, not as the name, because the slug is what the
		// charge's reference is built from (ADR 0008 + 0011). "Cuota APF" and
		// "Cuota A.P.F." are two names to a human and one reference to the
		// ledger: ON CONFLICT would drop the second line, report it as skipped,
		// and under-bill the family for the whole year without saying so.
		name := Slugify(c.Name)
		if seenConcept[name] {
			return TuitionPlan{}, glerrors.New(glerrors.KindInvalid,
				"two lines of the inscription share a name")
		}
		seenConcept[name] = true
	}
	if len(spec.EnrollmentConcepts) > 0 && spec.EnrollmentCents != 0 {
		// Two answers to one question. EnrollmentCents is still read by the
		// billing run, the enrolment consumer, the grpc handler and the gateway
		// client, so a plan carrying both would price the inscription at one
		// number in the editor and another on the statement, with nothing saying
		// they disagree. Refused rather than documented.
		return TuitionPlan{}, glerrors.New(glerrors.KindInvalid,
			"an inscription is either one amount or a list of concepts, not both")
	}
	for _, i := range spec.Schedule {
		if i.Month < 1 || i.Month > 12 {
			return TuitionPlan{}, glerrors.New(glerrors.KindInvalid, "a payment must fall in a month from 1 to 12")
		}
		if i.AmountCents <= 0 {
			// A payment of nothing is a charge for nothing: it would sit in a
			// family's statement forever, already settled and never explained.
			return TuitionPlan{}, glerrors.New(glerrors.KindInvalid, "a payment must be for some money")
		}
	}

	return TuitionPlan{
		id: spec.ID, tenantID: spec.TenantID, cycleID: spec.CycleID, levelID: spec.LevelID,
		enrollmentCents: spec.EnrollmentCents, monthlyCents: spec.MonthlyCents,
		months: spec.Months, firstBilledMonth: spec.FirstBilledMonth,
		dueDay: spec.DueDay, lateFeeCents: spec.LateFeeCents, active: spec.Active,
		policy:               spec.Policy,
		schedule:             spec.Schedule,
		monthlyDiscountCents: spec.MonthlyDiscountCents,
		fees:                 spec.Fees,
		enrollmentDueOn:      spec.EnrollmentDueOn,
		enrollmentStartsOn:   spec.EnrollmentStartsOn,
		enrollmentConcepts:   cloneConcepts(spec.EnrollmentConcepts),
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

// Policy is the collection rule that applies to this plan's charges: what a late
// payment costs per day and what an early one saves.
func (p TuitionPlan) Policy() CollectionPolicy { return p.policy }

// WithSurchargeFreeBefore is the table as it prices one legacy-imported pupil
// (CollectionPolicy.WithSurchargeFreeBefore). Read-side only: the plan is never
// saved from it.
func (p TuitionPlan) WithSurchargeFreeBefore(day time.Time) TuitionPlan {
	p.policy = p.policy.WithSurchargeFreeBefore(day)
	return p
}

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

// MonthlyDiscountCents is what comes off every month of this level — the
// school's arrangement for maternal, or for any level it decides to price
// differently. It is set once on the table rather than typed per pupil, which
// is the difference between a rule and four hundred chances to mistype it.
func (p TuitionPlan) MonthlyDiscountCents() int64 { return p.monthlyDiscountCents }

// Schedule is the plan's explicit payment plan, empty when the year is divided
// into equal monthly parts.
func (p TuitionPlan) Schedule() []Installment { return p.schedule }

// Installments places the year's payments on the calendar: what falls due, when,
// and for how much.
//
// Without a schedule this is the uniform year the plan has always billed — N
// equal payments, one a month. With one, it is exactly what the school wrote
// down, which is how "diciembre a la mitad y la otra mitad en marzo" becomes
// something the billing run can issue.
func (p TuitionPlan) Installments(cycleStart time.Time) []DueInstallment {
	if len(p.schedule) == 0 {
		months := p.BilledMonths(cycleStart)
		out := make([]DueInstallment, 0, len(months))
		for _, due := range months {
			out = append(out, DueInstallment{DueOn: due, AmountCents: p.monthlyCents})
		}
		return out
	}

	year := cycleStart.Year()
	if int(cycleStart.Month()) > p.schedule[0].Month {
		year++
	}
	out := make([]DueInstallment, 0, len(p.schedule))
	previous := 0
	for _, i := range p.schedule {
		// The schedule is read in the order the school wrote it, so a month
		// number that goes backwards means the year turned: September to July
		// runs across two calendar years, and nobody writing it down thinks of
		// that as two lists.
		if previous != 0 && i.Month < previous {
			year++
		}
		previous = i.Month
		out = append(out, DueInstallment{
			DueOn:       time.Date(year, time.Month(i.Month), p.dueDay, 0, 0, 0, 0, cycleStart.Location()),
			AmountCents: i.AmountCents,
		})
	}
	return out
}

// InstallmentsFrom is the part of the year a pupil who arrived on joinedOn owes.
//
// The run bills the WHOLE year up front, which is right for everybody who was
// there when it opened and wrong for anybody who was not: a pupil who arrives in
// March would be billed for September, for months the school never taught them.
//
// The month they arrive in IS theirs — the school's rule is that the first
// payment is the inscription plus the current month's tuition — but they are not
// late for it. March fell due on the 10th and they enrolled on the 20th;
// charging them a surcharge for ten days they were not yet a pupil is money the
// school cannot defend. So that first deadline moves to the day they joined, and
// only that one. It stays inside the same calendar month, which is what keeps
// the charge's reference (and therefore its identity) unchanged.
//
// A zero joinedOn is every pupil enrolled before the join date was recorded at
// all. They get the whole year, exactly as they do today.
func (p TuitionPlan) InstallmentsFrom(cycleStart, joinedOn time.Time) []DueInstallment {
	return fromJoin(p.Installments(cycleStart), cycleStart, joinedOn,
		func(i DueInstallment) time.Time { return i.DueOn },
		func(i *DueInstallment, due time.Time) { i.DueOn = due })
}

// fromJoin keeps the items a pupil who joined on joinedOn owes: none from the
// months before, and every one of their own month — EVERY one, not just the
// first: a year split "diciembre a la mitad" has two payments in one month, and
// the second left on the plan's day was overdue the moment they enrolled — due
// no earlier than the day they arrived. A zero joinedOn, or one not after the
// cycle start, is the whole list. Shared by tuition and fees so the two can
// never drift apart.
func fromJoin[T any](all []T, cycleStart, joinedOn time.Time, due func(T) time.Time, setDue func(*T, time.Time)) []T {
	if joinedOn.IsZero() || !joinedOn.After(cycleStart) {
		return all
	}
	joinedMonth := monthIndex(joinedOn)
	out := make([]T, 0, len(all))
	for _, item := range all {
		on := due(item)
		if monthIndex(on) < joinedMonth {
			continue
		}
		if monthIndex(on) == joinedMonth && on.Before(joinedOn) {
			setDue(&item, joinedOn)
		}
		out = append(out, item)
	}
	return out
}

// monthIndex orders a date by month across year boundaries, so a school year
// running September to June compares without anybody writing the year down
// twice.
func monthIndex(t time.Time) int {
	return t.Year()*monthsInYear + int(t.Month())
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
	months := monthIndex(asOf) - monthIndex(due)
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

// Fees are the extra amounts this level collects on top of the tuition.
func (p TuitionPlan) Fees() []Fee { return p.fees }

// EnrollmentDueOn is the day the inscription falls due, or the zero time when
// the school did not set one and the run should derive it.
func (p TuitionPlan) EnrollmentDueOn() time.Time { return p.enrollmentDueOn }

// EnrollmentStartsOn is the first day the inscription opens to families, or the
// zero time when it is open from the day it is issued.
func (p TuitionPlan) EnrollmentStartsOn() time.Time { return p.enrollmentStartsOn }

// FeeInstallments places every fee's shares on the calendar: what falls due,
// when, for how much, and which part of which fee it is.
//
// The year is worked out the same way the tuition's is — forward from the cycle,
// turning when a month number goes backwards — because a school that writes
// "septiembre y febrero" is not thinking about two calendar years, and should
// not have to.
func (p TuitionPlan) FeeInstallments(cycleStart time.Time) []FeeInstallment {
	var out []FeeInstallment
	for _, f := range p.fees {
		shares := f.shares()
		year := cycleStart.Year()
		if int(cycleStart.Month()) > f.Months[0] {
			year++
		}
		previous := 0
		for i, month := range f.Months {
			if previous != 0 && month < previous {
				year++
			}
			previous = month
			out = append(out, FeeInstallment{
				Name:        strings.TrimSpace(f.Name),
				Part:        i + 1,
				Of:          len(f.Months),
				DueOn:       time.Date(year, time.Month(month), p.dueDay, 0, 0, 0, 0, cycleStart.Location()),
				AmountCents: shares[i],
			})
		}
	}
	return out
}

// FeeInstallmentsFrom is the part of the fees a pupil who joined on joinedOn
// owes, under the same rule InstallmentsFrom applies to the tuition: the months
// before they joined are not theirs, and the month they join is, due no earlier
// than the day they arrived. A zero joinedOn, or one not after the cycle start,
// is the whole list.
func (p TuitionPlan) FeeInstallmentsFrom(cycleStart, joinedOn time.Time) []FeeInstallment {
	return fromJoin(p.FeeInstallments(cycleStart), cycleStart, joinedOn,
		func(i FeeInstallment) time.Time { return i.DueOn },
		func(i *FeeInstallment, due time.Time) { i.DueOn = due })
}
