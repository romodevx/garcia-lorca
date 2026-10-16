package usecase

import (
	"context"

	"github.com/garcia-lorca/services/finance/internal/ports"
)

// schoolDayEnrollments reads enrolments with EnrolledAt turned into the school
// DAY the pupil joined (ADR 0035): the date in Morelia, at midnight UTC like
// every stored finance date.
//
// Students hands over an instant. A chosen entry date is 00:00 in Morelia
// (06:00 UTC, the same day), but a default one is the instant of the alta, and
// one at 19:00 on 31 October is 1 November in UTC — a month the pupil did not
// join in. Converting once here means every reader (the tuition months, the
// fees, the inscription's due date) sees the same day.
type schoolDayEnrollments struct {
	inner ports.EnrollmentReader
	clock SchoolClock
}

func (r schoolDayEnrollments) ListByCycle(ctx context.Context, cycleID, groupID string) ([]ports.EnrollmentRef, error) {
	rows, err := r.inner.ListByCycle(ctx, cycleID, groupID)
	return r.inSchoolDays(rows), err
}

func (r schoolDayEnrollments) ListByStudent(ctx context.Context, studentID string) ([]ports.EnrollmentRef, error) {
	rows, err := r.inner.ListByStudent(ctx, studentID)
	return r.inSchoolDays(rows), err
}

func (r schoolDayEnrollments) inSchoolDays(rows []ports.EnrollmentRef) []ports.EnrollmentRef {
	for i := range rows {
		rows[i].EnrolledAt = r.clock.DayOf(rows[i].EnrolledAt)
	}
	return rows
}
