package handler

import (
	"context"
	"time"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// EnrollmentHistoryReader reads a pupil's enrollments (every status) and the
// moves between groups. The Connect StudentsClient satisfies it.
type EnrollmentHistoryReader interface {
	ListEnrollments(ctx context.Context, studentID, cycleID, groupID string) ([]client.Enrollment, error)
	ListTransfers(ctx context.Context, studentID string) ([]client.Transfer, error)
}

// StudentGroupLabeler names the group a pupil was in on a given school day, as
// every screen names it: "1° PRIMARIA · A" (ISSUE-017). A receipt is reprinted
// long after it was issued — for a withdrawn pupil, or one moved since — so the
// group is the one of the day the money came, not today's. It resolves the
// group through the same path the pass lists use, so the two never disagree.
type StudentGroupLabeler struct {
	students EnrollmentHistoryReader
	groups   AttendanceGroupReader
	// zone is the school's time zone, whose calendar day each instant falls on.
	zone *time.Location
}

// NewStudentGroupLabeler wires a StudentGroupLabeler; a nil zone reads as UTC.
func NewStudentGroupLabeler(students EnrollmentHistoryReader, groups AttendanceGroupReader, zone *time.Location) StudentGroupLabeler {
	if zone == nil {
		zone = time.UTC
	}
	return StudentGroupLabeler{students: students, groups: groups, zone: zone}
}

// StudentGroupLabel is the pupil's group label on `day` (YYYY-MM-DD). The
// enrollment is the one in force that day; failing that, the most recent of any
// status. found is false for a pupil with no enrollment, or whose group the
// catalog no longer holds.
func (l StudentGroupLabeler) StudentGroupLabel(ctx context.Context, studentID, day string) (string, bool, error) {
	enrollments, err := l.students.ListEnrollments(ctx, studentID, "", "")
	if err != nil || len(enrollments) == 0 {
		return "", false, err
	}
	chosen := l.enrollmentOn(enrollments, day)
	transfers, err := l.students.ListTransfers(ctx, studentID)
	if err != nil {
		return "", false, err
	}
	group, found, err := resolveListGroup(ctx, l.groups, l.groupOn(chosen, transfers, day))
	if err != nil || !found {
		return "", false, err
	}
	return group.Label, true, nil
}

// enrollmentOn is the enrollment in force on day — begun by then and not ended
// before it — or, when none was, the most recent one. Among several, the latest
// begun wins. Linear in the enrollments: a pupil has a handful.
func (l StudentGroupLabeler) enrollmentOn(enrollments []client.Enrollment, day string) client.Enrollment {
	var inForce, latest *client.Enrollment
	for i := range enrollments {
		e := &enrollments[i]
		if latest == nil || e.EnrolledAt.After(latest.EnrolledAt) {
			latest = e
		}
		if !l.inForceOn(*e, day) {
			continue
		}
		if inForce == nil || e.EnrolledAt.After(inForce.EnrolledAt) {
			inForce = e
		}
	}
	if inForce != nil {
		return *inForce
	}
	return *latest
}

// inForceOn: begun on or before day, and not ended — or ended on or after it.
func (l StudentGroupLabeler) inForceOn(e client.Enrollment, day string) bool {
	if l.dayOf(e.EnrolledAt) > day {
		return false
	}
	ended := e.WithdrawnOn
	if ended == "" && !e.EndedAt.IsZero() {
		ended = l.dayOf(e.EndedAt)
	}
	return ended == "" || ended >= day
}

// groupOn undoes, newest first, the moves of the enrollment made after day: a
// transfer rewrites the enrollment's group in place, so the moves are the only
// record of the group it had then. A move on the day itself already counts.
func (l StudentGroupLabeler) groupOn(e client.Enrollment, transfers []client.Transfer, day string) string {
	group := e.GroupID
	for i := len(transfers) - 1; i >= 0; i-- {
		t := transfers[i]
		if t.EnrollmentID == e.ID && l.dayOf(t.MovedAt) > day {
			group = t.FromGroupID
		}
	}
	return group
}

// dayOf is the school's calendar day of an instant ("" for an unknown one,
// which then sorts before every day).
func (l StudentGroupLabeler) dayOf(instant time.Time) string {
	if instant.IsZero() {
		return ""
	}
	return instant.In(l.zone).Format(time.DateOnly)
}
