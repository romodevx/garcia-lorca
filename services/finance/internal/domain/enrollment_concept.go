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
func (c EnrollmentConcept) AmountOn(on time.Time) int64 {
	amount := c.Tiers[0].AmountCents
	for _, tier := range c.Tiers {
		if tier.EffectiveFrom.After(on) {
			break
		}
		amount = tier.AmountCents
	}
	return amount
}

// validate reports what a school cannot mean by one line of its inscription.
func (c EnrollmentConcept) validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return glerrors.New(glerrors.KindInvalid,
			"a line of the inscription needs a name, or nobody can say what it is for")
	}
	if len(c.Tiers) == 0 {
		return glerrors.New(glerrors.KindInvalid, "set what the concept costs, from when")
	}
	if len(c.Tiers) > maxTiersPerConcept {
		return glerrors.New(glerrors.KindInvalid, "that is more price steps than a concept can carry")
	}
	var previous time.Time
	for _, tier := range c.Tiers {
		if tier.EffectiveFrom.IsZero() {
			return glerrors.New(glerrors.KindInvalid, "a price step needs the date it starts applying")
		}
		if tier.AmountCents <= 0 {
			// A step for nothing is a charge for nothing: it would sit in a
			// family's statement forever, already settled and never explained.
			return glerrors.New(glerrors.KindInvalid, "a price step must be for some money")
		}
		if !previous.IsZero() && !tier.EffectiveFrom.After(previous) {
			// Out of order, or two steps on one day. Sorting it here would be
			// guessing which price the school meant on the day they collide, and
			// the guess is worth a thousand pesos per pupil.
			return glerrors.New(glerrors.KindInvalid,
				"the price steps must run forwards in time, one date each")
		}
		previous = tier.EffectiveFrom
	}
	return nil
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
func (p TuitionPlan) EnrollmentConcepts() []EnrollmentConcept { return p.enrollmentConcepts }
