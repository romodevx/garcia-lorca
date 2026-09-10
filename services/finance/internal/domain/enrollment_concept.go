package domain

import (
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// The bounds on a plan's inscription. Neither is a business rule the school
// asked for; they are what stops a malformed payload turning one billing run
// into four hundred pupils times an unbounded list.
const (
	maxEnrollmentConcepts = 12
	maxTiersPerConcept    = 12
	// maxConceptName bounds what the family reads. The name travels to
	// charges.description and to the plan's JSON, and the run multiplies it by
	// twelve lines times every pupil of the level — the same amplification the
	// two counts above exist to stop. 120 runes is what a statement can print.
	maxConceptName = 120
)

// PriceTier is what a concept costs from a date onward.
//
// "Vigente desde" and not a set of months: "de abril a septiembre" is one row
// rather than six checkboxes, and nothing has to verify that no month was left
// out or listed twice — a ladder covers the whole axis by construction.
type PriceTier struct {
	EffectiveFrom time.Time
	AmountCents   int64
}

// EnrollmentConcept is one line of the inscription: a name the family reads and
// what it costs depending on WHEN it is settled.
//
// The school's price list does not charge one amount to join. It charges three —
// inscription, administration and insurance, parents' association — and the
// first of them doubles between January and April. The ladder is the whole
// incentive: joining late costs more, and it goes on costing more until the
// money arrives.
//
// Deliberately NOT a Fee. A Fee is one amount divided between months; this is
// one amount that CHANGES with the month. Folding either into the other would
// leave neither able to explain itself, and both live on the same plan.
//
// See ADR 0011.
type EnrollmentConcept struct {
	// Name is what the family reads on the statement, and what the charge's
	// external reference is derived from. Unique within the plan.
	Name string
	// Tiers is the price ladder, ordered by EffectiveFrom. At least one step.
	Tiers []PriceTier
}

// AmountOn is what this concept costs on a given day: the LAST step whose date
// has arrived.
//
// Before the first step, the first step's amount. Zero would price the year free
// for anybody who enquired in December, and a concept nobody was charged for is
// one the school discovers in June.
//
// `on` must be the SCHOOL'S calendar day, and this service's convention is that
// the school's day is the UTC day: resolveAsOf truncates now().UTC() to UTC
// midnight, and a tier's effective_from parses from YYYY-MM-DD to the same. The
// school is UTC-6, so that convention runs ~6 hours ahead of the office clock —
// which it already does for due dates, for the daily surcharge and for the
// early-payment window. Correcting it here alone would put the ladder 6 hours
// out of step with the surcharge on the same statement, which is worse than a
// system that is consistently offset. It belongs in one place, where now()
// becomes a date, against a school timezone this service does not yet hold.
func (c EnrollmentConcept) AmountOn(on time.Time) int64 {
	if len(c.Tiers) == 0 {
		// validate() refuses a ladder with no steps, so this cannot happen on a
		// concept that reached a plan. It can happen on one a handler is holding
		// straight off a request payload, and a price quote must not be able to
		// take the service down.
		return 0
	}
	// The step with the LATEST date that has arrived — found by scanning, not by
	// stopping at the first future one. validate() checks the order, but this
	// method is reached before it (see the guard above), and a ladder written
	// backwards would otherwise return March's price for a January date: wrong
	// money, no error, no panic.
	amount, best := c.Tiers[0].AmountCents, time.Time{}
	for _, tier := range c.Tiers {
		if tier.EffectiveFrom.After(on) {
			continue
		}
		if best.IsZero() || tier.EffectiveFrom.After(best) {
			amount, best = tier.AmountCents, tier.EffectiveFrom
		}
	}
	if best.IsZero() {
		// Nothing has arrived yet: the earliest step, which on a validated ladder
		// is the first one.
		earliest := c.Tiers[0]
		for _, tier := range c.Tiers {
			if tier.EffectiveFrom.Before(earliest.EffectiveFrom) {
				earliest = tier
			}
		}
		return earliest.AmountCents
	}
	return amount
}

// clone returns a concept that shares nothing with this one.
func (c EnrollmentConcept) clone() EnrollmentConcept {
	c.Tiers = append([]PriceTier(nil), c.Tiers...)
	return c
}

// cloneConcepts copies a whole inscription, ladders and all.
//
// TuitionPlan presents itself as an immutable value, and AmountOn depends
// ABSOLUTELY on an order that is checked exactly once, in the constructor. The
// grpc handler builds this slice and hands it straight over; without a copy it
// still owns the plan's prices, and any later sort or reslice there would move
// the ladder behind every guard that already ran. For money that is a stronger
// reason to copy than it was for fees.
func cloneConcepts(in []EnrollmentConcept) []EnrollmentConcept {
	if len(in) == 0 {
		return nil
	}
	out := make([]EnrollmentConcept, 0, len(in))
	for _, c := range in {
		out = append(out, c.clone())
	}
	return out
}

// validate reports what a school cannot mean by one line of its inscription.
func (c EnrollmentConcept) validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return glerrors.New(glerrors.KindInvalid,
			"a line of the inscription needs a name, or nobody can say what it is for")
	}
	if len([]rune(c.Name)) > maxConceptName {
		return glerrors.New(glerrors.KindInvalid, "that name is longer than a statement can print")
	}
	if len(c.Tiers) == 0 {
		return glerrors.New(glerrors.KindInvalid, "set what the concept costs, from when")
	}
	if len(c.Tiers) > maxTiersPerConcept {
		return glerrors.New(glerrors.KindInvalid, "that is more price steps than a concept can carry")
	}
	var previous time.Time
	var previousAmount int64
	for _, tier := range c.Tiers {
		if tier.EffectiveFrom.IsZero() {
			return glerrors.New(glerrors.KindInvalid, "a price step needs the date it starts applying")
		}
		if tier.AmountCents <= 0 {
			// A step for nothing is a charge for nothing: it would sit in a
			// family's statement forever, already settled and never explained.
			return glerrors.New(glerrors.KindInvalid, "a price step must be for some money")
		}
		if tier.AmountCents > MaxCents {
			// Refused HERE, where the person typing it can be told. NewCents only
			// catches it when the charge is built — which is the billing run, at
			// night, where the error aborts the WHOLE cycle rather than one line,
			// and now also the deposit path, where it would stop the counter
			// taking money from anybody in that level.
			return glerrors.New(glerrors.KindInvalid, "that is more money than a price step can hold")
		}
		if !previous.IsZero() && tier.AmountCents < previousAmount {
			// Everything else leans on the ladder being a one-way ratchet: ADR
			// 0011, the freeze rule, and the re-price that walks issued charges
			// forward. A transposed digit — 33,000 for 3,300 — would otherwise
			// take four hundred inscriptions DOWN, below what the families were
			// already quoted, with nothing recording that it happened.
			return glerrors.New(glerrors.KindInvalid,
				"a price step cannot cost less than the one before it")
		}
		if !previous.IsZero() && !tier.EffectiveFrom.After(previous) {
			// Out of order, or two steps on one day. Sorting it here would be
			// guessing which price the school meant on the day they collide, and
			// the guess is worth a thousand pesos per pupil.
			return glerrors.New(glerrors.KindInvalid,
				"the price steps must run forwards in time, one date each")
		}
		previous, previousAmount = tier.EffectiveFrom, tier.AmountCents
	}
	return nil
}

// Slugify reduces a name to the ASCII letters and digits in it, lowercased.
//
// It exists because a concept's NAME is what its charge reference is built from
// (ADR 0008 + 0011), and a reference is an identity: two names that reduce to
// one slug are one charge, and the second line the school meant to print would
// be silently dropped by ON CONFLICT and reported as skipped.
//
// So the uniqueness guard and the reference builder must reduce names exactly
// as hard as each other, which is only guaranteed by there being one function.
// An empty result is a legal answer here; callers that need a reference of their
// own supply the fallback, because the fallback differs per kind of charge and
// changing an existing one would re-issue every charge that carries it.
func Slugify(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// EnrollmentLine is one line of the inscription, priced for a day.
type EnrollmentLine struct {
	// Name is what the family reads. EMPTY on the single line of a plan that has
	// no concepts: that inscription is described by the school's own catalog
	// entry, which the school may have renamed, and the domain does not hold the
	// catalog.
	Name        string
	AmountCents int64
}

// EnrollmentLinesOn is what joining this level costs on a given day, one line
// per concept the school prints, in the order it wrote them.
//
// A plan with no concepts has ONE unnamed line at EnrollmentCents — which is
// what every plan written before ADR 0011 means, and what they go on meaning.
func (p TuitionPlan) EnrollmentLinesOn(on time.Time) []EnrollmentLine {
	if len(p.enrollmentConcepts) == 0 {
		return []EnrollmentLine{{AmountCents: p.enrollmentCents}}
	}
	out := make([]EnrollmentLine, 0, len(p.enrollmentConcepts))
	for _, c := range p.enrollmentConcepts {
		out = append(out, EnrollmentLine{
			Name:        strings.TrimSpace(c.Name),
			AmountCents: c.AmountOn(on),
		})
	}
	return out
}

// EnrollmentConcepts are the lines this level's inscription is broken into,
// empty when the plan carries the single amount it always did.
func (p TuitionPlan) EnrollmentConcepts() []EnrollmentConcept {
	return cloneConcepts(p.enrollmentConcepts)
}
