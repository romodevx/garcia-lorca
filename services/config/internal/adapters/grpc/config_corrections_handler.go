package grpcadapter

import (
	"context"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
)

// Catalog corrections (T8, ADR 0036): rename or delete a grade or group
// captured wrong without deleting its level, and the school order of levels.

// ReorderLevels puts the levels in the school order the request gives.
func (h *ConfigHandler) ReorderLevels(ctx context.Context, req *connect.Request[configv1.ReorderLevelsRequest]) (*connect.Response[configv1.ReorderLevelsResponse], error) {
	levels, err := h.catalog.ReorderLevels(ctx, req.Msg.GetLevelIds())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*configv1.Level, 0, len(levels))
	for _, l := range levels {
		out = append(out, toProtoLevel(l))
	}
	return connect.NewResponse(&configv1.ReorderLevelsResponse{Levels: out}), nil
}

// RenameGradeLevel renames a grade in place (same id, level and ordinal).
func (h *ConfigHandler) RenameGradeLevel(ctx context.Context, req *connect.Request[configv1.RenameGradeLevelRequest]) (*connect.Response[configv1.RenameGradeLevelResponse], error) {
	g, err := h.catalog.RenameGradeLevel(ctx, req.Msg.GetId(), req.Msg.GetName())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.RenameGradeLevelResponse{GradeLevel: toProtoGradeLevel(g)}), nil
}

// DeleteGradeLevel removes a grade that holds nothing.
func (h *ConfigHandler) DeleteGradeLevel(ctx context.Context, req *connect.Request[configv1.DeleteGradeLevelRequest]) (*connect.Response[configv1.DeleteGradeLevelResponse], error) {
	if err := h.catalog.DeleteGradeLevel(ctx, req.Msg.GetId()); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.DeleteGradeLevelResponse{}), nil
}

// RenameGroup renames a group in place (same id, cycle and grade).
func (h *ConfigHandler) RenameGroup(ctx context.Context, req *connect.Request[configv1.RenameGroupRequest]) (*connect.Response[configv1.RenameGroupResponse], error) {
	g, err := h.catalog.RenameGroup(ctx, req.Msg.GetId(), req.Msg.GetName())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.RenameGroupResponse{Group: toProtoGroup(g)}), nil
}

// GroupUsage counts what config keeps of a group.
func (h *ConfigHandler) GroupUsage(ctx context.Context, req *connect.Request[configv1.GroupUsageRequest]) (*connect.Response[configv1.GroupUsageResponse], error) {
	u, err := h.catalog.GroupUsage(ctx, req.Msg.GetGroupId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.GroupUsageResponse{
		TeachingAssignments: u.TeachingAssignments,
		ScheduleBlocks:      u.ScheduleBlocks,
	}), nil
}

// DeleteGroup removes a group config keeps nothing of.
func (h *ConfigHandler) DeleteGroup(ctx context.Context, req *connect.Request[configv1.DeleteGroupRequest]) (*connect.Response[configv1.DeleteGroupResponse], error) {
	if err := h.catalog.DeleteGroup(ctx, req.Msg.GetId()); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.DeleteGroupResponse{}), nil
}
