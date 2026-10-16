package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/garcia-lorca/pkg/schoolcal"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// activityConceptName is the system concept activity charges hang from (T17).
const activityConceptName = "Actividad"

// activityLateFeeSuffix closes the ref of an activity period's fixed late fee,
// "latefee:<chargeID>:actividad": one per charge, so the run never issues it
// twice, and readable by feeParent like any monthly surcharge.
const activityLateFeeSuffix = "actividad"

// WithActivities wires T17's activities into the billing run. Without it the
// run bills no activity, exactly as before.
func (s *FinanceService) WithActivities(repo ports.ActivityRepository) *FinanceService {
	s.activities = repo
	return s
}

// activityBilling is what the run needs to bill activities: the active ones
// by id, their enrolments, and every activity's late fee (an activity switched
// off still charges the late fee on what it already issued).
type activityBilling struct {
	byID        map[string]domain.Activity
	enrollments []domain.ActivityEnrollment
	lateFees    map[string]int64
}

// loadActivityBilling reads the active activities and their enrolments; empty
// when activities are not wired.
func (s *FinanceService) loadActivityBilling(ctx context.Context) (activityBilling, error) {
	if s.activities == nil {
		return activityBilling{}, nil
	}
	all, err := s.activities.ListActivities(ctx, true)
	if err != nil {
		return activityBilling{}, err
	}
	enrollments, err := s.activities.ListBillableEnrollments(ctx)
	if err != nil {
		return activityBilling{}, err
	}
	b := activityBilling{
		byID:        make(map[string]domain.Activity, len(all)),
		enrollments: enrollments,
		lateFees:    make(map[string]int64, len(all)),
	}
	for _, a := range all {
		b.lateFees[a.ID()] = a.LateFee().Int64()
		if a.Active() {
			b.byID[a.ID()] = a
		}
	}
	return b, nil
}

// wantedActivityCharges is every activity period billable by asOf for the
// pupils active in the cycle. A pupil enrolled in an activity but not placed in
// the cycle is that cycle's to skip: a charge needs a cycle.
func (s *FinanceService) wantedActivityCharges(
	ctx context.Context, b activityBilling, cycleID string, active map[string]bool,
	asOf time.Time, calendar *schoolcal.Calendar,
) ([]domain.Charge, error) {
	if len(b.enrollments) == 0 {
		return nil, nil
	}
	var out []domain.Charge
	conceptID := ""
	for _, e := range b.enrollments {
		a, ok := b.byID[e.ActivityID()]
		if !ok || !active[e.StudentID()] {
			continue
		}
		if conceptID == "" {
			concept, err := s.repo.EnsureConceptOfKind(ctx, s.newID(), domain.KindActivity, activityConceptName)
			if err != nil {
				return nil, err
			}
			conceptID = concept.ID()
		}
		charges, err := s.activityCharges(a, e, cycleID, conceptID, asOf, calendar)
		if err != nil {
			return nil, err
		}
		out = append(out, charges...)
	}
	return out, nil
}

// activityCharges is one enrolment's billable periods as charges: the option's
// price, open on the billing date, due on the due date (moved off a closed
// day; the ref keeps the period), under activity:<a>:<student>:<period>.
func (s *FinanceService) activityCharges(
	a domain.Activity, e domain.ActivityEnrollment, cycleID, conceptID string,
	asOf time.Time, calendar *schoolcal.Calendar,
) ([]domain.Charge, error) {
	option, ok := a.Option(e.OptionID())
	if !ok {
		// A retired option is only ever left on an ended enrolment; nothing
		// open points at one (the repository refuses it).
		return nil, nil
	}
	var out []domain.Charge
	for _, p := range a.PeriodsUpTo(asOf) {
		if !e.Bills(p) {
			continue
		}
		due := p.DueOn
		if e.StartsOn().After(due) {
			// Only a once-a-cycle period bills a pupil who joined after its due
			// date (domain Bills). They get the same window everyone had, from
			// the day they joined — not a debt already late.
			due = e.StartsOn().Add(p.DueOn.Sub(p.BillsOn))
		}
		charge, err := s.buildCharge(chargeSpec{
			studentID:   e.StudentID(),
			cycleID:     cycleID,
			conceptID:   conceptID,
			description: activityDescription(a, p),
			amount:      option.AmountCents,
			due:         calendar.NextWorkingDay(due),
			ref:         domain.ActivityChargeRef(a.ID(), e.StudentID(), p.Key),
		})
		if err != nil {
			return nil, err
		}
		if charge, err = charge.WithStartsOn(p.BillsOn); err != nil {
			return nil, err
		}
		out = append(out, charge)
	}
	return out, nil
}

// activityDescription is what the family reads: "Natación octubre 2026",
// "Natación semana del 5 octubre 2026", or just the name for a once-a-cycle
// charge.
func activityDescription(a domain.Activity, p domain.ActivityPeriod) string {
	switch a.Periodicity() {
	case domain.PeriodicityMonthly:
		return fmt.Sprintf("%s %s", a.Name(), monthInWords(p.BillsOn))
	case domain.PeriodicityWeekly:
		return fmt.Sprintf("%s semana del %d %s", a.Name(), p.BillsOn.Day(), monthInWords(p.BillsOn))
	default:
		return a.Name()
	}
}

// BillActivityEnrollments issues, at once, the periods already billable for
// enrolments just made — the same charges, under the same refs, the run would
// issue tomorrow. Each pupil is billed in the cycle they are placed in; one
// with no cycle is left for the run of the cycle that takes them.
func (s *FinanceService) BillActivityEnrollments(ctx context.Context, a domain.Activity, es []domain.ActivityEnrollment) (int, error) {
	asOf, err := resolveAsOf("", s.clock.Today)
	if err != nil {
		return 0, err
	}
	concept, err := s.repo.EnsureConceptOfKind(ctx, s.newID(), domain.KindActivity, activityConceptName)
	if err != nil {
		return 0, err
	}
	calendars := map[string]*schoolcal.Calendar{}
	var wanted []domain.Charge
	for _, e := range es {
		cycleID, err := s.activeCycleOf(ctx, e.StudentID())
		if err != nil {
			return 0, err
		}
		if cycleID == "" {
			continue
		}
		if calendars[cycleID] == nil {
			calendars[cycleID] = s.closedDays(ctx, cycleID)
		}
		charges, err := s.activityCharges(a, e, cycleID, concept.ID(), asOf, calendars[cycleID])
		if err != nil {
			return 0, err
		}
		wanted = append(wanted, charges...)
	}
	return s.repo.CreateChargesBulk(ctx, wanted)
}

// activityLateFee is the fixed late fee an overdue activity period earns, once
// (T17: optional per activity, $0 = none). ok=false when the charge is not an
// activity's.
func (s *FinanceService) activityLateFee(c domain.Charge, cycleID, lateFeeConceptID string, fees map[string]int64) (domain.Charge, bool, error) {
	activityID, isActivity := domain.ActivityOfRef(c.ExternalRef())
	if !isActivity || fees[activityID] <= 0 {
		return domain.Charge{}, false, nil
	}
	fee, err := s.buildCharge(chargeSpec{
		studentID:   c.StudentID(),
		cycleID:     cycleID,
		conceptID:   lateFeeConceptID,
		description: fmt.Sprintf("%s · %s", lateFeeConceptName, c.Description()),
		amount:      fees[activityID],
		due:         c.DueDate(),
		ref:         fmt.Sprintf("%s%s:%s", domain.LateFeeRefPrefix, c.ID(), activityLateFeeSuffix),
	})
	return fee, err == nil, err
}
