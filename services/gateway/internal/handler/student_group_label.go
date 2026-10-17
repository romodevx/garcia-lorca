package handler

import (
	"context"
	"fmt"
	"sort"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/garcia-lorca/pkg/schoolcal"
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
	var (
		enrollments []client.Enrollment
		transfers   []client.Transfer
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) {
		if enrollments, err = l.students.ListEnrollments(gctx, studentID, "", ""); err != nil {
			return fmt.Errorf("listing the enrollments of pupil %s: %w", studentID, err)
		}
		return nil
	})
	g.Go(func() (err error) {
		if transfers, err = l.students.ListTransfers(gctx, studentID); err != nil {
			return fmt.Errorf("listing the transfers of pupil %s: %w", studentID, err)
		}
		return nil
	})
	if err := g.Wait(); err != nil || len(enrollments) == 0 {
		return "", false, err
	}
	chosen := l.enrollmentOn(enrollments, day)
	group, found, err := resolveListGroup(ctx, l.groups, l.groupOn(chosen, transfers, day))
	if err != nil {
		return "", false, fmt.Errorf("resolving the group of pupil %s on %s: %w", studentID, day, err)
	}
	if !found {
		return "", false, nil
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
// Without withdrawn_on the end is ended_at's day in the school's zone.
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
// The moves are ordered here, not trusted to arrive in order.
func (l StudentGroupLabeler) groupOn(e client.Enrollment, transfers []client.Transfer, day string) string {
	moves := make([]client.Transfer, 0, len(transfers))
	for _, t := range transfers {
		if t.EnrollmentID == e.ID {
			moves = append(moves, t)
		}
	}
	sort.SliceStable(moves, func(i, j int) bool { return moves[i].MovedAt.After(moves[j].MovedAt) })
	group := e.GroupID
	for _, t := range moves {
		if l.dayOf(t.MovedAt) <= day {
			break
		}
		group = t.FromGroupID
	}
	return group
}

// dayOf is the school's calendar day of an instant (schoolcal.Date, ADR
// 0035), as YYYY-MM-DD; "" for an unknown one, which sorts before every day.
func (l StudentGroupLabeler) dayOf(instant time.Time) string {
	if instant.IsZero() {
		return ""
	}
	return schoolcal.Date(instant, l.zone).Format(time.DateOnly)
}
