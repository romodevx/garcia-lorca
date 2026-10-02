package usecase

import (
	"context"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// WithScholarships turns on T18 (ADR 0022): every path that prices a debt asks
// the pupil's scholarships. Without it no charge carries one.
func (s *FinanceService) WithScholarships(repo ports.ScholarshipRepository) *FinanceService {
	s.scholarships = repo
	return s
}

// scholarshipPricer is domain.ApplyScholarship with what it needs around it:
// the scholarships each pupil holds and the system concepts that say what a
// charge IS (its target). A nil pricer prices nothing, so a service wired
// without scholarships, or a test, reads charges exactly as stored.
type scholarshipPricer struct {
	held  map[string][]domain.HeldScholarship
	names map[string]string
	ids   systemConceptIDs
}

// systemConceptIDs are the catalog's system concepts by kind.
type systemConceptIDs struct {
	tuition, enrolment, lateFee string
}

// pricerFor reads the scholarships these pupils hold now (nil = every pupil).
func (s *FinanceService) pricerFor(ctx context.Context, studentIDs []string, ids systemConceptIDs) (*scholarshipPricer, error) {
	if s.scholarships == nil {
		return nil, nil
	}
	held, err := s.scholarships.HeldBy(ctx, studentIDs)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, list := range held {
		for _, h := range list {
			names[h.Scholarship.ID()] = h.Scholarship.Name()
		}
	}
	return &scholarshipPricer{held: held, names: names, ids: ids}, nil
}

// conceptIDs reads the system concepts' ids WITHOUT ensuring them: a read must
// not write, and a school with no such concept has no charge pointing at it.
func (s *FinanceService) conceptIDs(ctx context.Context) (systemConceptIDs, error) {
	concepts, err := s.repo.ListConcepts(ctx, true)
	if err != nil {
		return systemConceptIDs{}, err
	}
	var ids systemConceptIDs
	for _, c := range concepts {
		switch c.Kind() {
		case domain.KindTuition:
			ids.tuition = c.ID()
		case domain.KindEnrollment:
			ids.enrolment = c.ID()
		case domain.KindLateFee:
			ids.lateFee = c.ID()
		}
	}
	return ids, nil
}

// target is what a charge is, in a scholarship's words; "" for a surcharge,
// which no scholarship touches.
func (p *scholarshipPricer) target(c domain.Charge) string {
	switch {
	case c.ConceptID() == p.ids.lateFee || domain.IsFeeRef(c.ExternalRef()):
		return ""
	case !ownLedgerRef(c.ExternalRef()):
		// Another module's sale (a uniforms fulfillment): its price is that
		// module's, and it is paid by the single-charge path, which does not
		// price scholarships.
		return ""
	case c.ConceptID() == p.ids.tuition:
		return domain.TargetTuition
	case c.ConceptID() == p.ids.enrolment:
		return domain.TargetEnrollment
	}
	if activity, ok := domain.ActivityOfRef(c.ExternalRef()); ok {
		return domain.ActivityTarget(activity)
	}
	return domain.ConceptTarget(c.ConceptID())
}

// ownLedgerRef reports whether a charge is the school's own: typed by hand
// (no ref) or issued by finance's run (tuition, inscription, fee, activity).
func ownLedgerRef(ref string) bool {
	if ref == "" {
		return true
	}
	for _, prefix := range []string{tuitionRefPrefix, domain.EnrollmentRefPrefix, domain.FeeRefPrefix, domain.ActivityRefPrefix} {
		if strings.HasPrefix(ref, prefix) {
			return true
		}
	}
	return false
}

// tuitionRefPrefix names a month the run issued, "tuition:<cycle>:<student>:<YYYY-MM>".
const tuitionRefPrefix = "tuition:"

// price is the charge as a payment (or a read) on `on` finds it: with the
// scholarship's part, when one applies. Nothing is written.
func (p *scholarshipPricer) price(c ports.ChargeWithPaid, on time.Time) (ports.ChargeWithPaid, error) {
	if p == nil {
		return c, nil
	}
	priced, err := domain.ApplyScholarship(c.Charge, c.PaidCents, c.PaidByDueCents,
		p.held[c.Charge.StudentID()], p.target(c.Charge), on)
	if err != nil {
		return ports.ChargeWithPaid{}, err
	}
	c.Charge = priced
	return c, nil
}

// priceAll prices a list, leaving the input untouched.
func (p *scholarshipPricer) priceAll(in []ports.ChargeWithPaid, on time.Time) ([]ports.ChargeWithPaid, error) {
	if p == nil {
		return in, nil
	}
	out := make([]ports.ChargeWithPaid, 0, len(in))
	for _, c := range in {
		priced, err := p.price(c, on)
		if err != nil {
			return nil, err
		}
		out = append(out, priced)
	}
	return out, nil
}

// name is a scholarship's name for a statement line; "" when unknown.
func (p *scholarshipPricer) name(id string) string {
	if p == nil {
		return ""
	}
	return p.names[id]
}

// learnNames adds the names of scholarships no longer held — a charge settled
// under one keeps naming it — when any charge needs one.
func (s *FinanceService) learnNames(ctx context.Context, p *scholarshipPricer, charges []ports.ChargeWithPaid) error {
	if p == nil {
		return nil
	}
	for _, c := range charges {
		if id := c.Charge.ScholarshipID(); id != "" && p.names[id] == "" {
			all, err := s.scholarships.ListScholarships(ctx, true)
			if err != nil {
				return err
			}
			for _, sch := range all {
				p.names[sch.ID()] = sch.Name()
			}
			return nil
		}
	}
	return nil
}

// stillOwed keeps the charges that, priced on `on`, still owe money: what the
// overdue views and the monthly late fee must see (a month a scholarship
// settled is not a debt).
func stillOwed(p *scholarshipPricer, in []ports.ChargeWithPaid, on time.Time) ([]ports.ChargeWithPaid, error) {
	priced, err := p.priceAll(in, on)
	if err != nil {
		return nil, err
	}
	out := priced[:0:0]
	for _, c := range priced {
		if owed(c) > 0 {
			out = append(out, c)
		}
	}
	return out, nil
}

// FreezeSettledScholarships writes, on these pupils' charges, the scholarship
// part of every charge the scholarship has SETTLED — paid in time, or owing
// nothing by its due date (a free month). It runs before a scholarship is taken
// away or changed, so what it already settled is history the change never
// rewrites; what is still pending goes back to its price, by the rule.
func (s *FinanceService) FreezeSettledScholarships(ctx context.Context, studentIDs []string) error {
	if s.scholarships == nil || len(studentIDs) == 0 {
		return nil
	}
	ids, err := s.conceptIDs(ctx)
	if err != nil {
		return err
	}
	pricer, err := s.pricerFor(ctx, studentIDs, ids)
	if err != nil {
		return err
	}
	now := s.now()
	today := dayOf(now)
	var settled []domain.Charge
	for _, student := range studentIDs {
		charges, err := s.repo.ListCharges(ctx, student, "")
		if err != nil {
			return err
		}
		for _, c := range charges {
			if c.Charge.ScholarshipID() != "" {
				continue
			}
			priced, err := pricer.price(c, now)
			if err != nil {
				return err
			}
			if priced.Charge.ScholarshipID() == "" || owed(priced) > 0 {
				continue
			}
			// Settled by money at any time, or by the scholarship alone once
			// its due date has come: a free month not yet due is still pending.
			if c.PaidCents > 0 || !dayOf(c.Charge.DueDate()).After(today) {
				settled = append(settled, priced.Charge)
			}
		}
	}
	if len(settled) == 0 {
		return nil
	}
	_, err = s.scholarships.FreezeScholarships(ctx, settled)
	return err
}

// dayOf is the UTC calendar day of an instant, as every finance date.
func dayOf(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// refuseScholarshipCharge stops the single-charge payment on a charge a
// scholarship applies to on `when` (T18): that path vetoes over-payment and
// derives the status against the stored price, so it would collect the full
// price of a discounted month. Registrar pago (a deposit) prices it.
func (s *FinanceService) refuseScholarshipCharge(ctx context.Context, chargeID string, when time.Time) error {
	if s.scholarships == nil {
		return nil
	}
	charge, err := s.repo.GetCharge(ctx, chargeID)
	if glerrors.KindOf(err) == glerrors.KindNotFound {
		// The payment path reports a missing charge in its own words.
		return nil
	}
	if err != nil {
		return err
	}
	ids, err := s.conceptIDs(ctx)
	if err != nil {
		return err
	}
	pricer, err := s.pricerFor(ctx, []string{charge.Charge.StudentID()}, ids)
	if err != nil {
		return err
	}
	all, err := s.repo.ListCharges(ctx, charge.Charge.StudentID(), "")
	if err != nil {
		return err
	}
	for _, c := range all {
		if c.Charge.ID() != chargeID || c.Charge.ScholarshipID() != "" {
			continue
		}
		priced, err := pricer.price(c, when)
		if err != nil {
			return err
		}
		if priced.Charge.ScholarshipID() != "" {
			return glerrors.New(glerrors.KindConflict, "este cargo tiene beca: cóbralo en Registrar pago")
		}
	}
	return nil
}
