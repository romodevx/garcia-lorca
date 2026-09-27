//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/students/internal/domain"
)

// ListTransfers returns ONE student's moves (never a sibling's), oldest first,
// with the groups and the instant of each move.
func TestListTransfersIsThePupilsHistory(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	pupil := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-000000000601", "HEGB050312HDFRNN61")
	other := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-000000000602", "HEGB050312HDFRNN62")
	const enrPupil, enrOther = "e0000000-0000-0000-0000-000000000601", "e0000000-0000-0000-0000-000000000602"
	require.NoError(t, repo.CreateEnrollment(ctx, enrollment(t, enrPupil, tenantA, pupil, groupOne)))
	require.NoError(t, repo.CreateEnrollment(ctx, enrollment(t, enrOther, tenantA, other, groupOne)))

	first, err := domain.NewEnrollmentTransfer("70000000-0000-0000-0000-000000000601", tenantA, enrPupil, groupOne, groupTwo, "balanceo", enrollClock.AddDate(0, 0, 10))
	require.NoError(t, err)
	second, err := domain.NewEnrollmentTransfer("70000000-0000-0000-0000-000000000602", tenantA, enrPupil, groupTwo, groupOne, "regresa", enrollClock.AddDate(0, 0, 20))
	require.NoError(t, err)
	sibling, err := domain.NewEnrollmentTransfer("70000000-0000-0000-0000-000000000603", tenantA, enrOther, groupOne, groupTwo, "otro", enrollClock.AddDate(0, 0, 5))
	require.NoError(t, err)
	require.NoError(t, repo.TransferEnrollment(ctx, transferInto(enrPupil, groupTwo, second)))
	require.NoError(t, repo.TransferEnrollment(ctx, transferInto(enrPupil, groupOne, first)))
	require.NoError(t, repo.TransferEnrollment(ctx, transferInto(enrOther, groupTwo, sibling)))

	got, err := repo.ListTransfers(ctx, pupil)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "70000000-0000-0000-0000-000000000601", got[0].ID(), "oldest move first")
	assert.Equal(t, groupOne, got[0].FromGroupID())
	assert.Equal(t, groupTwo, got[0].ToGroupID())
	assert.True(t, got[0].MovedAt().Equal(enrollClock.AddDate(0, 0, 10)))
	assert.Equal(t, "70000000-0000-0000-0000-000000000602", got[1].ID())

	none, err := appRepo(t, tenantB).ListTransfers(ctx, pupil)
	require.NoError(t, err)
	assert.Empty(t, none, "RLS hides another tenant's history")
}

// A withdrawn enrollment carries the instant it stopped being active; an active
// one carries none.
func TestAnEndedEnrollmentCarriesItsEndInstant(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	pupil := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-000000000603", "HEGB050312HDFRNN63")
	const enrID = "e0000000-0000-0000-0000-000000000603"
	require.NoError(t, repo.CreateEnrollment(ctx, enrollment(t, enrID, tenantA, pupil, groupOne)))

	active, err := repo.GetEnrollment(ctx, enrID)
	require.NoError(t, err)
	assert.True(t, active.EndedAt().IsZero(), "an active enrollment has not ended")

	require.NoError(t, repo.SetEnrollmentStatus(ctx, enrID, domain.EnrollmentStatusWithdrawn))
	withdrawn, err := repo.GetEnrollment(ctx, enrID)
	require.NoError(t, err)
	assert.False(t, withdrawn.EndedAt().IsZero(), "a withdrawn enrollment says when")
}
