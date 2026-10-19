//go:build integration

package repo_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

// CC-301 fixtures: their own groups and enrollments (the container is shared
// and never reset).
const (
	groupHistA = "90000000-0000-0000-0000-000000000301"
	groupHistB = "90000000-0000-0000-0000-000000000302"

	enrHistNew   = "e0000000-0000-0000-0000-000000000301" // enrolled on the 17th
	enrHistMoved = "e0000000-0000-0000-0000-000000000302" // A → B on the 17th
	enrHistStays = "e0000000-0000-0000-0000-000000000303" // in A all along
)

// historyEnrollments is the students side of the scenario: who was ever in each
// group, and the one move between them.
type historyEnrollments struct{ mx *time.Location }

func (h historyEnrollments) EverInGroup(_ context.Context, groupID string) ([]service.EnrollmentRef, error) {
	aug := time.Date(2026, 8, 24, 0, 0, 0, 0, h.mx)
	moved := service.EnrollmentRef{ID: enrHistMoved, StudentID: "stu-moved", GroupID: groupHistB, Status: "active", EnrolledAt: aug}
	switch groupID {
	case groupHistA:
		return []service.EnrollmentRef{
			{ID: enrHistNew, StudentID: "stu-new", GroupID: groupHistA, Status: "active", EnrolledAt: time.Date(2026, 10, 17, 0, 0, 0, 0, h.mx)},
			moved,
			{ID: enrHistStays, StudentID: "stu-stays", GroupID: groupHistA, Status: "active", EnrolledAt: aug},
		}, nil
	case groupHistB:
		return []service.EnrollmentRef{moved}, nil
	}
	return nil, glerrors.ErrNotFound
}

func (h historyEnrollments) MovesOf(context.Context, string) ([]service.Move, error) {
	return []service.Move{{EnrollmentID: enrHistMoved, FromGroupID: groupHistA, ToGroupID: groupHistB, MovedAt: time.Date(2026, 10, 17, 11, 0, 0, 0, h.mx)}}, nil
}

type everyGroupExists struct{}

func (everyGroupExists) ByID(context.Context, string) error         { return nil }
func (everyGroupExists) WritableByID(context.Context, string) error { return nil }

// The QA repro of CC-301, against the real store: a pupil enrolled on the 17th
// cannot be marked absent on the 16th (nothing stored, nobody notified), and a
// list of the 16th keeps the pupil who moved on the 17th — with the mark it
// stored — while the new group's list of the 16th does not show them.
func TestPastListIsTheGroupOfThatDay(t *testing.T) {
	mx, err := time.LoadLocation("America/Mexico_City")
	require.NoError(t, err)
	r := appRepo(t, tenantA)
	ids := []string{"a0000000-0000-0000-0000-00000000c301", "a0000000-0000-0000-0000-00000000c302"}
	next := 0
	svc := service.NewAttendanceService(r, historyEnrollments{mx: mx}, everyGroupExists{}, nil, tenantA,
		func() string { next++; return ids[next-1] }, slog.New(slog.DiscardHandler), service.WithSchoolLocation(mx))
	ctx := context.Background()
	save := func(group string, marks ...service.Mark) error {
		_, err := svc.SaveDailyAttendance(ctx, service.SaveRequest{GroupID: group, Date: "2026-10-16", Marks: marks,
			TakenBy: testTaker.UserID, TakenByName: testTaker.Name})
		return err
	}

	err = save(groupHistA, service.Mark{EnrollmentID: enrHistNew, Status: "absent"})
	require.Error(t, err)
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err), "a mark before enrolling is a client error")
	stored, err := r.ListByEnrollment(ctx, enrHistNew, time.Time{}, time.Time{})
	require.NoError(t, err)
	assert.Empty(t, stored, "nothing stored for the pupil not yet enrolled")

	require.NoError(t, save(groupHistA, service.Mark{EnrollmentID: enrHistMoved, Status: "absent"}))

	listA, err := svc.GetDailyRoster(ctx, groupHistA, "2026-10-16", "")
	require.NoError(t, err)
	require.Len(t, listA.Rows, 2, "the moved pupil and the one who stayed; not the one enrolled later")
	assert.Equal(t, enrHistMoved, listA.Rows[0].EnrollmentID)
	assert.Equal(t, "absent", listA.Rows[0].Status, "a later move does not rewrite a past list")
	assert.Equal(t, enrHistStays, listA.Rows[1].EnrollmentID)

	listB, err := svc.GetDailyRoster(ctx, groupHistB, "2026-10-16", "")
	require.NoError(t, err)
	assert.Empty(t, listB.Rows, "the new group's list of a day before the move does not show the pupil")
	assert.Error(t, save(groupHistB, service.Mark{EnrollmentID: enrHistMoved, Status: "late"}), "nor can it mark them")
}
