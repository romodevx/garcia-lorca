//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// Owner decision after T9: choosing a group by hand respects its seats like
// the automatic placement and a transfer do — a full group, or one whose
// size was never set, refuses the pupil and says why.
func TestEnrollingIntoAChosenGroupRespectsItsSeats(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	at := time.Date(2026, time.October, 16, 12, 0, 0, 0, time.UTC)
	enrol := func(studentID, enrID string, seats ports.GroupSeats) error {
		e, err := domain.NewEnrollment(enrID, tenantA, studentID, cycleUUID, seats.ID, "", domain.EnrollmentStatusActive, at)
		require.NoError(t, err)
		return repo.CreateEnrollment(ctx, e, seats)
	}
	first := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-000000000951", "HEGB050312HDFRNS51")
	second := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-000000000952", "HEGB050312HDFRNS52")
	third := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-000000000953", "HEGB050312HDFRNS53")

	// Groups of its own: other tests in this database fill the shared ones,
	// and a group here is only an id (config owns the catalog).
	const ownGroup, ownUnsized = "6a000000-0000-4000-8000-000000000951", "6a000000-0000-4000-8000-000000000952"
	oneSeat := ports.GroupSeats{ID: ownGroup, Name: "1° Z", CycleID: cycleUUID, Capacity: 1}
	require.NoError(t, enrol(first, "e0000000-0000-0000-0000-000000000951", oneSeat))

	err := enrol(second, "e0000000-0000-0000-0000-000000000952", oneSeat)
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err), "full: %v", err)
	assert.Contains(t, err.Error(), "ya no tiene lugares")

	unsized := ports.GroupSeats{ID: ownUnsized, Name: "1° Y", CycleID: cycleUUID, Capacity: 0}
	err = enrol(third, "e0000000-0000-0000-0000-000000000953", unsized)
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err), "unsized: %v", err)
	assert.Contains(t, err.Error(), "no tiene cupo definido")
}

// roomFor is a roomy group for e: tests about something other than seats
// place pupils without counting them.
func roomFor(e domain.Enrollment) ports.GroupSeats {
	return ports.GroupSeats{ID: e.GroupID(), CycleID: e.CycleID(), Capacity: 1000}
}

// createRoomy places e without counting seats, for tests about other things.
func createRoomy(ctx context.Context, repo interface {
	CreateEnrollment(context.Context, domain.Enrollment, ports.GroupSeats) error
}, e domain.Enrollment) error {
	return repo.CreateEnrollment(ctx, e, roomFor(e))
}
