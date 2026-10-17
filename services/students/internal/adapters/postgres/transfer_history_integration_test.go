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
	require.NoError(t, createRoomy(ctx, repo, enrollment(t, enrPupil, tenantA, pupil, groupOne)))
	require.NoError(t, createRoomy(ctx, repo, enrollment(t, enrOther, tenantA, other, groupOne)))

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
	require.NoError(t, createRoomy(ctx, repo, enrollment(t, enrID, tenantA, pupil, groupOne)))

	active, err := repo.GetEnrollment(ctx, enrID)
	require.NoError(t, err)
	assert.True(t, active.EndedAt().IsZero(), "an active enrollment has not ended")

	require.NoError(t, repo.SetEnrollmentStatus(ctx, enrID, domain.EnrollmentStatusWithdrawn))
	withdrawn, err := repo.GetEnrollment(ctx, enrID)
	require.NoError(t, err)
	assert.False(t, withdrawn.EndedAt().IsZero(), "a withdrawn enrollment says when")
}

// ListTransfersIntoGroup answers every move INTO one group (whoever made it),
// oldest first, and nothing from another tenant.
func TestListTransfersIntoGroupIsTheClassListHistory(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	a := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-000000000611", "HEGB050312HDFRNN71")
	b := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-000000000612", "HEGB050312HDFRNN72")
	const enrA, enrB = "e0000000-0000-0000-0000-000000000611", "e0000000-0000-0000-0000-000000000612"
	require.NoError(t, createRoomy(ctx, repo, enrollment(t, enrA, tenantA, a, groupOne)))
	require.NoError(t, createRoomy(ctx, repo, enrollment(t, enrB, tenantA, b, groupOne)))

	intoTwoLate, err := domain.NewEnrollmentTransfer("70000000-0000-0000-0000-000000000611", tenantA, enrA, groupOne, groupTwo, "balanceo", enrollClock.AddDate(0, 0, 30))
	require.NoError(t, err)
	intoTwoEarly, err := domain.NewEnrollmentTransfer("70000000-0000-0000-0000-000000000612", tenantA, enrB, groupOne, groupTwo, "balanceo", enrollClock.AddDate(0, 0, 3))
	require.NoError(t, err)
	require.NoError(t, repo.TransferEnrollment(ctx, transferInto(enrA, groupTwo, intoTwoLate)))
	require.NoError(t, repo.TransferEnrollment(ctx, transferInto(enrB, groupTwo, intoTwoEarly)))

	got, err := repo.ListTransfersIntoGroup(ctx, groupTwo)
	require.NoError(t, err)
	ids := make([]string, 0, len(got))
	for _, m := range got {
		assert.Equal(t, groupTwo, m.ToGroupID())
		ids = append(ids, m.ID())
	}
	assert.Subset(t, ids, []string{"70000000-0000-0000-0000-000000000611", "70000000-0000-0000-0000-000000000612"})
	assert.Less(t, indexOf(ids, "70000000-0000-0000-0000-000000000612"), indexOf(ids, "70000000-0000-0000-0000-000000000611"), "oldest move first")

	none, err := appRepo(t, tenantB).ListTransfersIntoGroup(ctx, groupTwo)
	require.NoError(t, err)
	assert.Empty(t, none, "RLS hides another tenant's moves")
}

// CC-301: a past day's class list needs every enrollment that was EVER in the
// group — including one moved out since (it now carries another group_id) —
// and every move into or out of it, so attendance can tell who sat in the
// group on that day. A pupil who never touched the group is not part of it.
func TestEverInGroupReadsMovedOutPupilsAndBothSidesOfTheMoves(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	stayed := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-000000000621", "HEGB050312HDFRNN81")
	left := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-000000000622", "HEGB050312HDFRNN82")
	elsewhere := seedStudent(t, tenantA, "dddddddd-0000-0000-0000-000000000623", "HEGB050312HDFRNN83")
	const enrStayed, enrLeft, enrElsewhere = "e0000000-0000-0000-0000-000000000621", "e0000000-0000-0000-0000-000000000622", "e0000000-0000-0000-0000-000000000623"
	require.NoError(t, createRoomy(ctx, repo, enrollment(t, enrStayed, tenantA, stayed, groupOne)))
	require.NoError(t, createRoomy(ctx, repo, enrollment(t, enrLeft, tenantA, left, groupOne)))
	require.NoError(t, createRoomy(ctx, repo, enrollment(t, enrElsewhere, tenantA, elsewhere, groupTwo)))

	out, err := domain.NewEnrollmentTransfer("70000000-0000-0000-0000-000000000621", tenantA, enrLeft, groupOne, groupTwo, "balanceo", enrollClock.AddDate(0, 0, 4))
	require.NoError(t, err)
	require.NoError(t, repo.TransferEnrollment(ctx, transferInto(enrLeft, groupTwo, out)))

	ever, err := repo.ListEnrollmentsEverInGroup(ctx, groupOne)
	require.NoError(t, err)
	ids := make([]string, 0, len(ever))
	for _, e := range ever {
		ids = append(ids, e.ID())
	}
	assert.Contains(t, ids, enrStayed)
	assert.Contains(t, ids, enrLeft, "moved out since, still part of the group's past")
	assert.NotContains(t, ids, enrElsewhere)

	moves, err := repo.ListTransfersTouchingGroup(ctx, groupOne)
	require.NoError(t, err)
	var found bool
	for _, m := range moves {
		assert.True(t, m.FromGroupID() == groupOne || m.ToGroupID() == groupOne)
		found = found || m.ID() == "70000000-0000-0000-0000-000000000621"
	}
	assert.True(t, found, "the move OUT of the group is part of its history")

	none, err := appRepo(t, tenantB).ListEnrollmentsEverInGroup(ctx, groupOne)
	require.NoError(t, err)
	assert.Empty(t, none, "RLS hides another tenant's enrollments")
	noMoves, err := appRepo(t, tenantB).ListTransfersTouchingGroup(ctx, groupOne)
	require.NoError(t, err)
	assert.Empty(t, noMoves, "RLS hides another tenant's moves")
}

func indexOf(ids []string, id string) int {
	for i, v := range ids {
		if v == id {
			return i
		}
	}
	return -1
}
