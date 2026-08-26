package usecase

import (
	"context"
	"fmt"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
)

// EnsureEnrollmentCharge issues a pupil's inscription for a cycle, if the school
// prices their level and they do not already owe it.
//
// This is what makes the school's rule true — "the inscription is charged when
// the pupil takes a place in a group" — without anybody pressing anything. It
// runs off the StudentEnrolled event, which students already publishes.
//
// It needs NO idempotency ledger, unlike the comms consumer that reads the same
// topic. Pub/Sub delivers at least once, and the answer here is the charge's own
// external_ref: `enrollment:<cycle>:<student>` is unique per school (migration
// 0003), so a redelivery inserts nothing and reports zero. The uniqueness that
// already had to exist for the monthly run to be re-runnable is the same
// uniqueness that makes this consumer safe.
//
// A pupil whose level has no tuition table is NOT charged zero — it is left for
// the monthly run to report as an unpriced level, which is the one place a
// person is looking.
func (s *FinanceService) EnsureEnrollmentCharge(ctx context.Context, cycleID, studentID string) (created bool, err error) {
	if cycleID == "" || studentID == "" {
		return false, glerrors.New(glerrors.KindInvalid, "the enrolment must name a student and a cycle")
	}
	cycle, err := s.cycles.ByID(ctx, cycleID)
	if err != nil {
		return false, err
	}

	enrollments, err := s.enrollments.ListByCycle(ctx, cycleID, "")
	if err != nil {
		return false, err
	}
	var groupID string
	for _, e := range enrollments {
		if e.StudentID == studentID && e.Status == enrolmentStatusActive {
			groupID = e.GroupID
			break
		}
	}
	if groupID == "" {
		// Enrolled and then withdrawn before the event was handled, or an event
		// for a cycle whose enrolment is gone. Nothing to bill, and nothing wrong.
		return false, nil
	}

	levelOfGroup, err := s.levelOfGroup(ctx, cycleID)
	if err != nil {
		return false, err
	}
	plans, err := s.repo.ListTuitionPlans(ctx, cycleID)
	if err != nil {
		return false, err
	}
	var plan domain.TuitionPlan
	var priced bool
	for _, p := range plans {
		if p.Active() && p.LevelID() == levelOfGroup[groupID] {
			plan, priced = p, true
			break
		}
	}
	if !priced {
		return false, nil
	}

	concept, err := s.repo.EnsureConceptOfKind(ctx, s.newID(), domain.KindEnrollment, enrollmentConceptName)
	if err != nil {
		return false, err
	}
	charge, err := s.buildCharge(chargeSpec{
		studentID:   studentID,
		cycleID:     cycleID,
		conceptID:   concept.ID(),
		description: enrollmentConceptName,
		amount:      plan.EnrollmentCents(),
		due:         enrollmentDueDate(cycle.StartsOn, plan),
		ref:         fmt.Sprintf("enrollment:%s:%s", cycleID, studentID),
	})
	if err != nil {
		return false, err
	}
	written, err := s.repo.CreateChargesBulk(ctx, []domain.Charge{charge})
	if err != nil {
		return false, err
	}
	return written == 1, nil
}
