package grpcadapter

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"

	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
	"github.com/garcia-lorca/services/grades/internal/usecase"
)

var errNoGroupUsage = connect.NewError(connect.CodeUnimplemented, errors.New("group usage not wired"))

// WithGroupUsage wires the group-usage count (T8, ADR 0036). The composition
// root always wires it; without it the RPC answers Unimplemented.
func (h *GradesHandler) WithGroupUsage(groups *usecase.GroupUsageService) *GradesHandler {
	h.groups = groups
	return h
}

// GroupUsage counts the report cards issued for one group.
func (h *GradesHandler) GroupUsage(ctx context.Context, req *connect.Request[gradesv1.GroupUsageRequest]) (*connect.Response[gradesv1.GroupUsageResponse], error) {
	if h.groups == nil {
		return nil, errNoGroupUsage
	}
	n, err := h.groups.ReportCards(ctx, req.Msg.GetGroupId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&gradesv1.GroupUsageResponse{ReportCards: n}), nil
}
