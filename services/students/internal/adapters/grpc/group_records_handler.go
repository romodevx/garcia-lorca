package grpcadapter

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

var errGroupRecordsNotServed = connect.NewError(connect.CodeUnimplemented, errors.New("group records are not served"))

// WithGroupRecords returns the handler answering for students' records of a
// config group (T8, ADR 0036). Without it those RPCs answer Unimplemented.
func (h *StudentsHandler) WithGroupRecords(groups *usecase.GroupRecordService) *StudentsHandler {
	cp := *h
	cp.groups = groups
	return &cp
}

// GroupUsage counts the enrollments and transfers of one group.
func (h *StudentsHandler) GroupUsage(ctx context.Context, req *connect.Request[studentsv1.GroupUsageRequest]) (*connect.Response[studentsv1.GroupUsageResponse], error) {
	if h.groups == nil {
		return nil, errGroupRecordsNotServed
	}
	u, err := h.groups.GroupUsage(ctx, req.Msg.GetGroupId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.GroupUsageResponse{Enrollments: u.Enrollments, Transfers: u.Transfers}), nil
}

// RefreshGroupName relabels the active pupils' files after a group rename.
func (h *StudentsHandler) RefreshGroupName(ctx context.Context, req *connect.Request[studentsv1.RefreshGroupNameRequest]) (*connect.Response[studentsv1.RefreshGroupNameResponse], error) {
	if h.groups == nil {
		return nil, errGroupRecordsNotServed
	}
	n, err := h.groups.RefreshGroupName(ctx, req.Msg.GetGroupId(), req.Msg.GetName())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.RefreshGroupNameResponse{Students: n}), nil
}
