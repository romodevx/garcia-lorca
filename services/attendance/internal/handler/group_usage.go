package handler

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"

	attendancev1 "github.com/garcia-lorca/gen/go/garcialorca/attendance/v1"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

// WithGroupUsage wires the group-usage count (T8, ADR 0036). The composition
// root always wires it; without it the RPC answers Unimplemented.
func (h *AttendanceHandler) WithGroupUsage(g *service.GroupUsageService) *AttendanceHandler {
	h.groups = g
	return h
}

var errNoGroupUsage = connect.NewError(connect.CodeUnimplemented, errors.New("group usage not wired"))

// GroupUsage counts the pass lists and marks of one group.
func (h *AttendanceHandler) GroupUsage(ctx context.Context, req *connect.Request[attendancev1.GroupUsageRequest]) (*connect.Response[attendancev1.GroupUsageResponse], error) {
	if h.groups == nil {
		return nil, errNoGroupUsage
	}
	u, err := h.groups.GroupUsage(ctx, req.Msg.GetGroupId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&attendancev1.GroupUsageResponse{Sessions: u.Sessions, Marks: u.Marks}), nil
}
