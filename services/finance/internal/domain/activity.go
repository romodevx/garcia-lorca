package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// How often an activity is charged (T17). The school picks one per activity.
const (
	// PeriodicityMonthly charges once a month, on the same day of the month
	// as the first billing date. The default.
	PeriodicityMonthly = "monthly"
	// PeriodicityWeekly charges once a week, on the same weekday.
	PeriodicityWeekly = "weekly"
	// PeriodicityPerCycle is a single charge, on the first billing date.
	PeriodicityPerCycle = "per_cycle"
)

const (
	// MaxActivityName bounds an activity's name, in characters.
	MaxActivityName = 80
	// MaxActivitySlots bounds the weekly schedule: three slots a day, every day.
	MaxActivitySlots = 21
	// MaxActivityOptions bounds the price list: one per number of days plus
	// the flat fee.
	MaxActivityOptions = 8
	// minutesPerDay is the end of a day on the schedule's clock.
	minutesPerDay = 24 * 60
	// maxActivityPeriods stops a period walk that a bad date could make endless:
	// a weekly activity billed for eleven years.
	maxActivityPeriods = 600
	// daysPerWeek is a week, for weekly periods and the weekday range.
	daysPerWeek = 7
)

// ActivitySlot is one entry of an activity's weekly schedule: a weekday
// (1 = Monday … 7 = Sunday) and a time range, in minutes from midnight, end
// exclusive. The schedule is descriptive — what the family is told — and
// never part of the charge.
type ActivitySlot struct {
	Weekday  int
	StartMin int
	EndMin   int
}

// PriceOption is one charging scheme of an activity: a price per period for
// attending DaysPerWeek days a week, or — DaysPerWeek 0 — a flat fee whatever
// the days. Chosen per pupil when enrolling.
type PriceOption struct {
	ID          string
	DaysPerWeek int
	AmountCents int64
}

// Flat reports whether the option is the flat fee.
func (o PriceOption) Flat() bool { return o.DaysPerWeek == 0 }

// ActivitySpec is everything a school types about an activity.
type ActivitySpec struct {
	ID              string
	Name            string
	Schedule        []ActivitySlot
	Periodicity     string
	BillingStartsOn time.Time
	FirstDueOn      time.Time
	LateFeeCents    int64
	Active          bool
	Options         []PriceOption
	// ResumedOn is the day the activity was last switched back on; zero when
	// it never was. Periods due before it are never billed: the months an
	// activity was off are months it did not run.
	ResumedOn time.Time
}

// Activity is an extracurricular the school charges for (T17, ADR 0031): its
// schedule, its price options and when it is billed. Periods repeat the first
// billing date and the first due date at the activity's periodicity.
//
// Immutable; built only through NewActivity, so an Activity in hand is valid.
type Activity struct {
	id              string
	name            string
	schedule        []ActivitySlot
	periodicity     string
	billingStartsOn time.Time
	firstDueOn      time.Time
	lateFee         Cents
	active          bool
	options         []PriceOption
	resumedOn       time.Time
}

// NewActivity validates a spec. Every failure is KindInvalid, in Spanish,
// because the office reads it.
func NewActivity(s ActivitySpec) (Activity, error) {
	if strings.TrimSpace(s.ID) == "" {
		return Activity{}, invalid("activity id is required")
	}
	name := strings.TrimSpace(s.Name)
	if name == "" {
		return Activity{}, invalid("la actividad necesita un nombre")
	}
	if len([]rune(name)) > MaxActivityName {
		return Activity{}, invalid(fmt.Sprintf("el nombre admite hasta %d caracteres", MaxActivityName))
	}
	schedule, err := validSchedule(s.Schedule)
	if err != nil {
		return Activity{}, err
	}
	if err := validBillingDates(s.Periodicity, s.BillingStartsOn, s.FirstDueOn); err != nil {
		return Activity{}, err
	}
	lateFee, err := NewCents(s.LateFeeCents)
	if err != nil {
		return Activity{}, invalid("el recargo no puede ser negativo")
	}
	options, err := validOptions(s.Options)
	if err != nil {
		return Activity{}, err
	}
	return Activity{
		id: s.ID, name: name, schedule: schedule, periodicity: s.Periodicity,
		billingStartsOn: s.BillingStartsOn, firstDueOn: s.FirstDueOn,
		lateFee: lateFee, active: s.Active, options: options, resumedOn: s.ResumedOn,
	}, nil
}

func invalid(msg string) error { return glerrors.New(glerrors.KindInvalid, msg) }

// validSchedule checks every slot and that no two of one day overlap; it
// returns the slots sorted by day and start.
func validSchedule(slots []ActivitySlot) ([]ActivitySlot, error) {
	if len(slots) == 0 {
		return nil, invalid("indica al menos un día y horario")
	}
	if len(slots) > MaxActivitySlots {
		return nil, invalid(fmt.Sprintf("el horario admite hasta %d renglones", MaxActivitySlots))
	}
	out := append([]ActivitySlot(nil), slots...)
	for _, sl := range out {
		if sl.Weekday < 1 || sl.Weekday > daysPerWeek {
			return nil, invalid("el día debe ser de lunes (1) a domingo (7)")
		}
		if sl.StartMin < 0 || sl.EndMin > minutesPerDay-1 || sl.EndMin <= sl.StartMin {
			return nil, invalid("la hora de fin debe ser posterior a la de inicio")
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Weekday != out[j].Weekday {
			return out[i].Weekday < out[j].Weekday
		}
		return out[i].StartMin < out[j].StartMin
	})
	for i := 1; i < len(out); i++ {
		if out[i].Weekday == out[i-1].Weekday && out[i].StartMin < out[i-1].EndMin {
			return nil, invalid("dos horarios del mismo día se enciman")
		}
	}
	return out, nil
}

// validBillingDates checks the periodicity and that the first due date falls
// inside the first period: on or after the billing date, before the next one.
func validBillingDates(periodicity string, startsOn, dueOn time.Time) error {
	switch periodicity {
	case PeriodicityMonthly, PeriodicityWeekly, PeriodicityPerCycle:
	default:
		return invalid("periodicidad inválida")
	}
	if startsOn.IsZero() {
		return invalid("indica la fecha de inicio de cobro")
	}
	if dueOn.IsZero() {
		return invalid("indica la fecha límite de pago")
	}
	if dueOn.Before(startsOn) {
		return invalid("la fecha límite no puede ser anterior al inicio de cobro")
	}
	if periodicity != PeriodicityPerCycle && !dueOn.Before(shift(periodicity, startsOn, 1)) {
		return invalid("la fecha límite debe caer antes del siguiente periodo de cobro")
	}
	return nil
}

// validOptions checks the price list: amounts above zero, days 0–7, no two
// options for the same days (so at most one flat fee), every option with id.
func validOptions(options []PriceOption) ([]PriceOption, error) {
	if len(options) == 0 {
		return nil, invalid("indica al menos un esquema de cobro")
	}
	if len(options) > MaxActivityOptions {
		return nil, invalid(fmt.Sprintf("se admiten hasta %d esquemas de cobro", MaxActivityOptions))
	}
	seen := map[int]bool{}
	out := append([]PriceOption(nil), options...)
	for _, o := range out {
		if strings.TrimSpace(o.ID) == "" {
			return nil, invalid("price option id is required")
		}
		if o.DaysPerWeek < 0 || o.DaysPerWeek > daysPerWeek {
			return nil, invalid("los días por semana van de 1 a 7")
		}
		if o.AmountCents <= 0 {
			return nil, invalid("el monto de cada esquema debe ser mayor a cero")
		}
		if _, err := NewCents(o.AmountCents); err != nil {
			return nil, err
		}
		if seen[o.DaysPerWeek] {
			if o.Flat() {
				return nil, invalid("sólo puede haber una cuota fija")
			}
			return nil, invalid(fmt.Sprintf("hay dos esquemas de %d día(s) por semana", o.DaysPerWeek))
		}
		seen[o.DaysPerWeek] = true
	}
	// Per-day options in order, the flat fee last: the order the office reads.
	sort.SliceStable(out, func(i, j int) bool {
		return optionOrder(out[i]) < optionOrder(out[j])
	})
	return out, nil
}

func optionOrder(o PriceOption) int {
	if o.Flat() {
		return daysPerWeek + 1
	}
	return o.DaysPerWeek
}

// ActivityPeriod is one billable period: its identity inside the charge ref,
// the day it is billed (and opens to the family) and the day it falls due.
// Single marks the one period of a once-a-cycle activity, which has no "next
// period" to start a late joiner on.
type ActivityPeriod struct {
	Key         string
	BillsOn     time.Time
	DueOn       time.Time
	NextBillsOn time.Time // the next period's billing date; zero when Single
	Single      bool
}

// perCycleKey is the one period of a once-a-cycle activity: fixed, so moving
// its date never re-issues it.
const perCycleKey = "ciclo"

// PeriodsUpTo lists the periods whose billing date has arrived by asOf, oldest
// first. A period is the first billing and due dates moved n periods along:
// the same day of the month (clamped to the month's end) or the same weekday.
func (a Activity) PeriodsUpTo(asOf time.Time) []ActivityPeriod {
	var out []ActivityPeriod
	for n := 0; n < maxActivityPeriods; n++ {
		bills := shift(a.periodicity, a.billingStartsOn, n)
		if bills.After(asOf) {
			break
		}
		p := ActivityPeriod{
			Key:     a.periodKey(bills),
			BillsOn: bills,
			DueOn:   shift(a.periodicity, a.firstDueOn, n),
			Single:  a.periodicity == PeriodicityPerCycle,
		}
		if !p.Single {
			p.NextBillsOn = shift(a.periodicity, a.billingStartsOn, n+1)
		}
		// The months it was off are skipped; the period in course is billed
		// when it came back on or before its due date.
		if p.Single || a.resumedOn.IsZero() || !a.resumedOn.After(p.DueOn) {
			out = append(out, p)
		}
		if p.Single {
			break
		}
	}
	return out
}

// periodKey is the period's identity in the charge ref. Coarse on purpose —
// the month, the ISO week — so moving the billing day inside its period does
// not re-issue it.
func (a Activity) periodKey(bills time.Time) string {
	switch a.periodicity {
	case PeriodicityMonthly:
		return bills.Format("2006-01")
	case PeriodicityWeekly:
		y, w := bills.ISOWeek()
		return fmt.Sprintf("%d-W%02d", y, w)
	default:
		return perCycleKey
	}
}

// SameBillingCalendar reports whether another version of this activity mints
// the same period keys: same periodicity and a first billing date in the same
// period. Changing either once pupils are enrolled would re-issue periods
// already billed under new refs.
func (a Activity) SameBillingCalendar(other Activity) bool {
	return a.periodicity == other.periodicity &&
		a.periodKey(a.billingStartsOn) == other.periodKey(other.billingStartsOn)
}

// shift moves a date n periods along.
func shift(periodicity string, t time.Time, n int) time.Time {
	switch periodicity {
	case PeriodicityWeekly:
		return t.AddDate(0, 0, daysPerWeek*n)
	case PeriodicityMonthly:
		return addMonthsClamped(t, n)
	default:
		return t
	}
}

// addMonthsClamped is t moved n months, on the same day or the month's last:
// the 31st of October is followed by the 30th of November, never the 1st of
// December (which is what time.AddDate would say).
func addMonthsClamped(t time.Time, n int) time.Time {
	first := time.Date(t.Year(), t.Month()+time.Month(n), 1, 0, 0, 0, 0, t.Location())
	last := first.AddDate(0, 1, -1).Day()
	return time.Date(first.Year(), first.Month(), min(t.Day(), last), 0, 0, 0, 0, t.Location())
}

// Option finds a price option by id.
func (a Activity) Option(id string) (PriceOption, bool) {
	for _, o := range a.options {
		if o.ID == id {
			return o, true
		}
	}
	return PriceOption{}, false
}

// Accessors. Slices are copies: the activity is immutable.
func (a Activity) ID() string                 { return a.id }
func (a Activity) Name() string               { return a.name }
func (a Activity) Schedule() []ActivitySlot   { return append([]ActivitySlot(nil), a.schedule...) }
func (a Activity) Periodicity() string        { return a.periodicity }
func (a Activity) BillingStartsOn() time.Time { return a.billingStartsOn }
func (a Activity) FirstDueOn() time.Time      { return a.firstDueOn }
func (a Activity) LateFee() Cents             { return a.lateFee }
func (a Activity) Active() bool               { return a.active }
func (a Activity) Options() []PriceOption     { return append([]PriceOption(nil), a.options...) }
func (a Activity) ResumedOn() time.Time       { return a.resumedOn }

// ParseClock reads "HH:MM" (24 h) as minutes from midnight.
func ParseClock(raw string) (int, error) {
	var h, m int
	if len(raw) != len("15:04") || raw[2] != ':' {
		return 0, invalid("las horas van como HH:MM")
	}
	if _, err := fmt.Sscanf(raw, "%02d:%02d", &h, &m); err != nil {
		return 0, invalid("las horas van como HH:MM")
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, invalid("las horas van como HH:MM")
	}
	return h*60 + m, nil
}

// FormatClock writes minutes from midnight as "HH:MM".
func FormatClock(minutes int) string {
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}

// ActivityRefPrefix names an activity charge the billing run issued,
// "activity:<activityID>:<studentID>:<period>". Ids, never names, so renaming
// an activity re-issues nothing.
const ActivityRefPrefix = "activity:"

// ActivityChargeRef is the external ref of one pupil's charge for one period.
func ActivityChargeRef(activityID, studentID, periodKey string) string {
	return ActivityRefPrefix + activityID + ":" + studentID + ":" + periodKey
}

// IsActivityRef reports whether a charge was issued for an activity.
func IsActivityRef(ref string) bool { return strings.HasPrefix(ref, ActivityRefPrefix) }

// ActivityOfRef reads back the activity an activity charge belongs to.
func ActivityOfRef(ref string) (string, bool) {
	if !IsActivityRef(ref) {
		return "", false
	}
	id, _, _ := strings.Cut(strings.TrimPrefix(ref, ActivityRefPrefix), ":")
	return id, id != ""
}

// ActivityEnrollment is a pupil enrolled in an activity with the price option
// chosen for them, from startsOn until endsOn (zero: still enrolled). endsOn is
// the first day the pupil is no longer enrolled.
type ActivityEnrollment struct {
	id         string
	activityID string
	studentID  string
	optionID   string
	startsOn   time.Time
	endsOn     time.Time
}

// NewActivityEnrollment builds a validated, open enrolment.
func NewActivityEnrollment(id, activityID, studentID, optionID string, startsOn time.Time) (ActivityEnrollment, error) {
	for _, part := range []string{id, activityID, studentID, optionID} {
		if strings.TrimSpace(part) == "" {
			return ActivityEnrollment{}, invalid("enrollment id, activity, student and option are required")
		}
	}
	if startsOn.IsZero() {
		return ActivityEnrollment{}, invalid("indica la fecha de inscripción")
	}
	return ActivityEnrollment{id: id, activityID: activityID, studentID: studentID, optionID: optionID, startsOn: startsOn}, nil
}

// RestoreActivityEnrollment rebuilds a stored enrolment, end date included.
func RestoreActivityEnrollment(id, activityID, studentID, optionID string, startsOn, endsOn time.Time) (ActivityEnrollment, error) {
	e, err := NewActivityEnrollment(id, activityID, studentID, optionID, startsOn)
	if err != nil {
		return ActivityEnrollment{}, err
	}
	e.endsOn = endsOn
	return e, nil
}

// End returns a copy that stops on `on`: no period billed on or after it is
// charged. Charges already issued are the ledger's, never touched here.
func (e ActivityEnrollment) End(on time.Time) ActivityEnrollment {
	e.endsOn = on
	return e
}

// WithOption returns a copy charged under another price option from the next
// period billed.
func (e ActivityEnrollment) WithOption(optionID string) (ActivityEnrollment, error) {
	if strings.TrimSpace(optionID) == "" {
		return ActivityEnrollment{}, invalid("elige un esquema de cobro")
	}
	e.optionID = optionID
	return e, nil
}

// Bills reports whether the period is charged to this enrolment: it had not
// ended when the pupil joined (owner decision D3: the period in course is
// charged in full, even after its due date; no proration) — the one period of a
// once-a-cycle activity has no end and is always charged — and it is billed
// before the enrolment ended.
func (e ActivityEnrollment) Bills(p ActivityPeriod) bool {
	if !p.Single && !e.startsOn.Before(p.NextBillsOn) {
		return false
	}
	if e.endsOn.IsZero() {
		return true
	}
	return e.endsOn.After(e.startsOn) && p.BillsOn.Before(e.endsOn)
}

// DueOn is the day the period falls due for this pupil: its own due date, or,
// for one who joined after it, the day they joined (D3) — for a once-a-cycle
// period, the same window everyone had, counted from that day.
func (e ActivityEnrollment) DueOn(p ActivityPeriod) time.Time {
	if !e.startsOn.After(p.DueOn) {
		return p.DueOn
	}
	if p.Single {
		return e.startsOn.Add(p.DueOn.Sub(p.BillsOn))
	}
	return e.startsOn
}

// ActiveOn reports whether the pupil is still enrolled on that day: not ended,
// or ending later.
func (e ActivityEnrollment) ActiveOn(day time.Time) bool {
	return e.endsOn.IsZero() || e.endsOn.After(day)
}

// Accessors.
func (e ActivityEnrollment) ID() string          { return e.id }
func (e ActivityEnrollment) ActivityID() string  { return e.activityID }
func (e ActivityEnrollment) StudentID() string   { return e.studentID }
func (e ActivityEnrollment) OptionID() string    { return e.optionID }
func (e ActivityEnrollment) StartsOn() time.Time { return e.startsOn }
func (e ActivityEnrollment) EndsOn() time.Time   { return e.endsOn }
