package usecase

import (
	"context"
	"time"

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
	var joinedOn time.Time
	for _, e := range enrollments {
		if e.StudentID == studentID && e.Status == enrolmentStatusActive {
			groupID, joinedOn = e.GroupID, e.EnrolledAt
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
	// Priced at the day THEY joined, not at today. A pupil enrolled in February
	// whose event is handled in March pays February's rung: the ladder answers
	// "when did you join", and a redelivery days later must not cost the family
	// more than the day they walked in.
	joined := joinedOn
	if joined.IsZero() {
		joined = s.now()
	}
	charges, err := s.enrollmentCharges(studentID, cycleID, concept.ID(),
		plan, joined, enrollmentDueDate(cycle.StartsOn, plan, joinedOn))
	if err != nil {
		return false, err
	}
	written, err := s.repo.CreateChargesBulk(ctx, charges)
	if err != nil {
		return false, err
	}
	return written > 0, nil
}
