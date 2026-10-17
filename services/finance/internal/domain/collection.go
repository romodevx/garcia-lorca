package domain

import (
	"time"

	"github.com/garcia-lorca/pkg/schoolcal"
)

// CollectionPolicy is what a school charges for paying late and forgives for
// paying early. One policy per (cycle, level), alongside the tuition table.
//
// It is a value, not an entity: it has no identity of its own and is always read
// with the plan it belongs to. Keeping it separate from TuitionPlan is what lets
// the rules change for a new cycle without rewriting what the old one charged.
type CollectionPolicy struct {
	// DailyMode selects which surcharge rule applies. false keeps the original
	// rule (TuitionPlan.LateFeeCents, once per completed overdue month), which is
	// what every plan written before M16 means. true is per-day at
	// LateFeeDailyCents.
	//
	// An explicit mode rather than "daily if a daily rate is set": a school that
	// half-configured the new rule would otherwise silently switch, and the
	// difference between $150 a month and $10 a day is money on every statement.
	DailyMode bool

	// LateFeeDailyCents is charged for each day past the due date. Zero disables
	// the surcharge entirely — a school that has not set a rate charges nothing,
	// rather than charging zero and looking like the feature worked.
	LateFeeDailyCents int64

	// SurchargeCountsAllDays decides whether weekends and holidays are billable.
	//
	// This school said yes: "el recargo sí sábados domingos y feriados". Another
	// may not, and the difference is real money, so it is configuration rather
	// than an assumption compiled into the arithmetic.
	SurchargeCountsAllDays bool

	// EarlyPaymentCents is forgiven per month settled inside the window. A FIXED
	// amount, not a percentage: "el pronto pago es una cantidad fija".
	EarlyPaymentCents int64

	// EarlyPaymentDays is the width of that window in WORKING days, counted from
	// the start of the month being paid. Working days and not calendar days so a
	// window ending on a Sunday does not quietly cost a family the discount.
	EarlyPaymentDays int
	// surchargeFreeBefore is the day a legacy-imported pupil arrived (T14):
	// a charge that fell due before it accrues no surcharge. Zero for everybody
	// else. Per pupil, never stored with the plan — see WithSurchargeFreeBefore.
	surchargeFreeBefore time.Time
}

// WithSurchargeFreeBefore is the policy as it applies to a pupil the legacy
// import brought in on `day` (owner decision 2026-10-17): the months that fell
// due before they arrived are owed at the normal price, with no surcharge of
// either rule. From that day on the policy is unchanged.
func (p CollectionPolicy) WithSurchargeFreeBefore(day time.Time) CollectionPolicy {
	p.surchargeFreeBefore = day
	return p
}

// SurchargeFree reports whether a charge due on dueOn is spared every
// surcharge: it fell due strictly before the pupil was imported. The ONE rule
// the monthly run (accrueLateFees) and the daily derivation (Surcharge) read.
func (p CollectionPolicy) SurchargeFree(dueOn time.Time) bool {
	return !p.surchargeFreeBefore.IsZero() && dueOn.Before(p.surchargeFreeBefore)
}

// Surcharge is what a charge has accrued for being late, as of paidOn.
//
// The rule the school gave, in their own numbers: tuition falls due on the 10th,
// and the count starts the day AFTER. Pay on the 11th and you owe one day; on
// the 20th, ten; on the 30th, twenty.
//
// It does NOT stop at the end of the month it fell due in. Asked directly, the
// owner confirmed it keeps running until THAT month is settled — a surcharge
// frozen at month end costs a family the same whether they pay on 1 October or
// in March, which removes every reason to pay. There is deliberately no ceiling
// either; if one is ever wanted it belongs in the policy, not here.
//
// Paying a different month does not pause it. Each month answers for itself,
// which is what makes a statement explainable to a parent.
//
// Derived, never stored. A daily surcharge written as charge rows would be ~30
// rows per pupil per month — 12,000 a month for this school — and would still be
// wrong the moment the rate changed. The ledger materialises it as one real
// charge at the moment money is applied to it, so rows exist where money moved.
func Surcharge(p CollectionPolicy, cal *schoolcal.Calendar, dueOn, paidOn time.Time) int64 {
	if p.LateFeeDailyCents <= 0 || p.SurchargeFree(dueOn) {
		return 0
	}
	var days int
	if p.SurchargeCountsAllDays {
		days = cal.CalendarDaysBetween(dueOn, paidOn)
	} else {
		days = cal.WorkingDaysBetween(dueOn, paidOn)
	}
	if days <= 0 {
		return 0
	}
	return int64(days) * p.LateFeeDailyCents
}

// EarlyPaymentDiscount is what a family is forgiven for settling a month inside
// its early window, counted from monthStart.
//
// Applied PER MONTH SETTLED, not per payment: "si paga 6 meses se le aplica
// durante los 6 meses la cuota de pronto pago". A family clearing six months on
// time earns it six times, so the caller asks once per month it settles.
//
// The window is working days, which is the one place the school calendar still
// matters now that the surcharge runs every day. A holiday inside the window
// pushes the deadline out and the family keeps the benefit — the rule the
// director gave for deadlines landing on closed days.
func EarlyPaymentDiscount(p CollectionPolicy, cal *schoolcal.Calendar, monthStart, paidOn time.Time) int64 {
	if p.EarlyPaymentCents <= 0 || p.EarlyPaymentDays <= 0 {
		return 0
	}
	// AddWorkingDays(start, n-1): the first working day is day one of the window,
	// so five days from a Tuesday ends on the fifth working day, not the sixth.
	deadline := cal.AddWorkingDays(monthStart, p.EarlyPaymentDays-1)
	if paidOn.After(deadline) {
		return 0
	}
	return p.EarlyPaymentCents
}
