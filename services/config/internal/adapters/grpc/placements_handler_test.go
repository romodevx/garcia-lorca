package grpcadapter_test

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

// stubPlacements records the call and answers what it is told.
type stubPlacements struct {
	cycleID string
	reqs    []usecase.PlacementRequest
	dryRun  bool
	answer  []usecase.Placement
	err     error
}

func (s *stubPlacements) EnsurePlacements(_ context.Context, cycleID string, reqs []usecase.PlacementRequest, dryRun bool) ([]usecase.Placement, error) {
	s.cycleID, s.reqs, s.dryRun = cycleID, reqs, dryRun
	return s.answer, s.err
}

func TestEnsurePlacementsTranslatesBothWays(t *testing.T) {
	h, _ := newHandler(t)
	stub := &stubPlacements{answer: []usecase.Placement{
		{
			LevelID: "lvl", LevelName: "Primaria", GradeLevelID: "gr", GradeName: "1°", GradeCreated: true,
			GroupID: "grp", GroupName: "A", GroupCreated: true, Capacity: 30,
		},
		{Error: usecase.PlacementLevelBeingDeleted},
	}}
	h.WithPlacements(stub)

	res, err := h.EnsurePlacements(ctx(), connect.NewRequest(&configv1.EnsurePlacementsRequest{
		CycleId: "cyc", DryRun: true,
		Placements: []*configv1.PlacementRequest{
			{LevelId: "lvl", GradeOrdinal: 1, GroupName: "a", Capacity: 30},
			{NewLevelName: "Kinder", GradeOrdinal: 2, GroupName: "B", Capacity: 20},
		},
	}))
	require.NoError(t, err)

	assert.Equal(t, "cyc", stub.cycleID)
	assert.True(t, stub.dryRun)
	assert.Equal(t, []usecase.PlacementRequest{
		{LevelID: "lvl", GradeOrdinal: 1, GroupName: "a", Capacity: 30},
		{NewLevelName: "Kinder", GradeOrdinal: 2, GroupName: "B", Capacity: 20},
	}, stub.reqs)

	got := res.Msg.GetPlacements()
	require.Len(t, got, 2)
	assert.Equal(t, "lvl", got[0].GetLevelId())
	assert.Equal(t, "Primaria", got[0].GetLevelName())
	assert.False(t, got[0].GetLevelCreated())
	assert.Equal(t, "gr", got[0].GetGradeLevelId())
	assert.Equal(t, "1°", got[0].GetGradeName())
	assert.True(t, got[0].GetGradeCreated())
	assert.Equal(t, "grp", got[0].GetGroupId())
	assert.Equal(t, "A", got[0].GetGroupName())
	assert.True(t, got[0].GetGroupCreated())
	assert.Equal(t, int32(30), got[0].GetCapacity())
	assert.Empty(t, got[0].GetError())
	assert.Equal(t, "level_being_deleted", got[1].GetError())
}

func TestEnsurePlacementsMapsTheCallsError(t *testing.T) {
	h, _ := newHandler(t)
	h.WithPlacements(&stubPlacements{err: glerrors.New(glerrors.KindInvalid, "the cycle does not exist")})
	_, err := h.EnsurePlacements(ctx(), connect.NewRequest(&configv1.EnsurePlacementsRequest{CycleId: "nope"}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestEnsurePlacementsUnwiredIsUnimplemented(t *testing.T) {
	h, _ := newHandler(t)
	_, err := h.EnsurePlacements(ctx(), connect.NewRequest(&configv1.EnsurePlacementsRequest{CycleId: "cyc"}))
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}
