package grpcadapter

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

// PlacementService is what the handler needs of the placement use case.
type PlacementService interface {
	EnsurePlacements(ctx context.Context, cycleID string, reqs []usecase.PlacementRequest, dryRun bool) ([]usecase.Placement, error)
}

// WithPlacements wires the legacy import's find-or-create (ADR 0033). The
// composition root always wires it.
func (h *ConfigHandler) WithPlacements(p PlacementService) *ConfigHandler {
	h.placements = p
	return h
}

// EnsurePlacements finds or creates the level → grade → group of each
// placement in one transaction (ADR 0033). Permissions and feature flags are
// enforced by the gateway; per-placement failures travel in each answer's
// error code, while a refused call (too many placements, unknown cycle) maps
// to its Connect code.
func (h *ConfigHandler) EnsurePlacements(
	ctx context.Context, req *connect.Request[configv1.EnsurePlacementsRequest],
) (*connect.Response[configv1.EnsurePlacementsResponse], error) {
	if h.placements == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("placements are not wired"))
	}
	reqs := make([]usecase.PlacementRequest, 0, len(req.Msg.GetPlacements()))
	for _, p := range req.Msg.GetPlacements() {
		reqs = append(reqs, usecase.PlacementRequest{
			LevelID: p.GetLevelId(), NewLevelName: p.GetNewLevelName(),
			GradeOrdinal: int(p.GetGradeOrdinal()), GroupName: p.GetGroupName(), Capacity: int(p.GetCapacity()),
		})
	}
	placements, err := h.placements.EnsurePlacements(ctx, req.Msg.GetCycleId(), reqs, req.Msg.GetDryRun())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*configv1.Placement, 0, len(placements))
	for _, p := range placements {
		out = append(out, toProtoPlacement(p))
	}
	return connect.NewResponse(&configv1.EnsurePlacementsResponse{Placements: out}), nil
}

func toProtoPlacement(p usecase.Placement) *configv1.Placement {
	return &configv1.Placement{
		LevelId: p.LevelID, LevelName: p.LevelName, LevelCreated: p.LevelCreated,
		GradeLevelId: p.GradeLevelID, GradeName: p.GradeName, GradeCreated: p.GradeCreated,
		GroupId: p.GroupID, GroupName: p.GroupName, GroupCreated: p.GroupCreated,
		Capacity: int32(p.Capacity), Error: p.Error,
	}
}
