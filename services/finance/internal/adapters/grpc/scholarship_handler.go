package grpcadapter

import (
	"context"
	"time"

	connect "connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// WithScholarships wires T18's scholarships. Without it those RPCs answer
// Unimplemented.
func (h *FinanceHandler) WithScholarships(svc *usecase.ScholarshipService) *FinanceHandler {
	h.scholarships = svc
	return h
}

var errScholarshipsUnwired = connect.NewError(connect.CodeUnimplemented, glerrors.New(glerrors.KindInternal, "scholarships are not enabled"))

// ListScholarships lists the scholarships with their holder counts.
func (h *FinanceHandler) ListScholarships(ctx context.Context, req *connect.Request[financev1.ListScholarshipsRequest]) (*connect.Response[financev1.ListScholarshipsResponse], error) {
	if h.scholarships == nil {
		return nil, errScholarshipsUnwired
	}
	views, err := h.scholarships.ListScholarships(ctx, req.Msg.GetIncludeInactive())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*financev1.Scholarship, len(views))
	for i, v := range views {
		out[i] = toProtoScholarship(v.Scholarship, v.AssignedCount)
	}
	return connect.NewResponse(&financev1.ListScholarshipsResponse{Scholarships: out}), nil
}

// GetScholarship reads one.
func (h *FinanceHandler) GetScholarship(ctx context.Context, req *connect.Request[financev1.GetScholarshipRequest]) (*connect.Response[financev1.GetScholarshipResponse], error) {
	if h.scholarships == nil {
		return nil, errScholarshipsUnwired
	}
	v, err := h.scholarships.GetScholarship(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.GetScholarshipResponse{Scholarship: toProtoScholarship(v.Scholarship, v.AssignedCount)}), nil
}

// CreateScholarship adds one.
func (h *FinanceHandler) CreateScholarship(ctx context.Context, req *connect.Request[financev1.CreateScholarshipRequest]) (*connect.Response[financev1.CreateScholarshipResponse], error) {
	if h.scholarships == nil {
		return nil, errScholarshipsUnwired
	}
	s, err := h.scholarships.CreateScholarship(ctx, fromProtoScholarshipInput(req.Msg.GetScholarship()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.CreateScholarshipResponse{Scholarship: toProtoScholarship(s, 0)}), nil
}

// UpdateScholarship replaces one's terms and reads it back with its count.
func (h *FinanceHandler) UpdateScholarship(ctx context.Context, req *connect.Request[financev1.UpdateScholarshipRequest]) (*connect.Response[financev1.UpdateScholarshipResponse], error) {
	if h.scholarships == nil {
		return nil, errScholarshipsUnwired
	}
	if _, err := h.scholarships.UpdateScholarship(ctx, req.Msg.GetId(), fromProtoScholarshipInput(req.Msg.GetScholarship())); err != nil {
		return nil, connectErr(err)
	}
	v, err := h.scholarships.GetScholarship(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.UpdateScholarshipResponse{Scholarship: toProtoScholarship(v.Scholarship, v.AssignedCount)}), nil
}

// ListScholarshipTargets lists what a scholarship can apply to today.
func (h *FinanceHandler) ListScholarshipTargets(ctx context.Context, _ *connect.Request[financev1.ListScholarshipTargetsRequest]) (*connect.Response[financev1.ListScholarshipTargetsResponse], error) {
	if h.scholarships == nil {
		return nil, errScholarshipsUnwired
	}
	targets, err := h.scholarships.ListTargets(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*financev1.ScholarshipTarget, len(targets))
	for i, t := range targets {
		out[i] = &financev1.ScholarshipTarget{Key: t.Key, Label: t.Label, Group: t.Group}
	}
	return connect.NewResponse(&financev1.ListScholarshipTargetsResponse{Targets: out}), nil
}

// ListScholarshipHolders lists who holds one now.
func (h *FinanceHandler) ListScholarshipHolders(ctx context.Context, req *connect.Request[financev1.ListScholarshipHoldersRequest]) (*connect.Response[financev1.ListScholarshipHoldersResponse], error) {
	if h.scholarships == nil {
		return nil, errScholarshipsUnwired
	}
	holders, err := h.scholarships.ListHolders(ctx, req.Msg.GetScholarshipId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.ListScholarshipHoldersResponse{Holders: toProtoHolders(holders)}), nil
}

// AssignScholarship gives one to pupils.
func (h *FinanceHandler) AssignScholarship(ctx context.Context, req *connect.Request[financev1.AssignScholarshipRequest]) (*connect.Response[financev1.AssignScholarshipResponse], error) {
	if h.scholarships == nil {
		return nil, errScholarshipsUnwired
	}
	holders, err := h.scholarships.Assign(ctx, req.Msg.GetScholarshipId(), req.Msg.GetStudentIds())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.AssignScholarshipResponse{Holders: toProtoHolders(holders)}), nil
}

// UnassignScholarship takes one away from pupils.
func (h *FinanceHandler) UnassignScholarship(ctx context.Context, req *connect.Request[financev1.UnassignScholarshipRequest]) (*connect.Response[financev1.UnassignScholarshipResponse], error) {
	if h.scholarships == nil {
		return nil, errScholarshipsUnwired
	}
	n, err := h.scholarships.Unassign(ctx, req.Msg.GetScholarshipId(), req.Msg.GetStudentIds())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&financev1.UnassignScholarshipResponse{Removed: int32(n)}), nil
}

func fromProtoScholarshipInput(m *financev1.ScholarshipInput) usecase.ScholarshipInput {
	return usecase.ScholarshipInput{
		Name: m.GetName(), Kind: m.GetKind(), Value: m.GetValue(), Targets: m.GetTargets(), Active: m.GetActive(),
	}
}

func toProtoScholarship(s domain.Scholarship, assigned int) *financev1.Scholarship {
	return &financev1.Scholarship{
		Id: s.ID(), Name: s.Name(), Kind: s.Kind(), Value: s.Value(), Targets: s.Targets(),
		Active: s.Active(), AssignedCount: int32(assigned),
	}
}

func toProtoHolders(hs []usecase.ScholarshipHolder) []*financev1.ScholarshipHolder {
	out := make([]*financev1.ScholarshipHolder, len(hs))
	for i, h := range hs {
		overlaps := make([]*financev1.ScholarshipRef, len(h.Overlaps))
		for j, o := range h.Overlaps {
			overlaps[j] = &financev1.ScholarshipRef{Id: o.ID, Name: o.Name}
		}
		out[i] = &financev1.ScholarshipHolder{
			StudentId: h.StudentID, StartsOn: h.StartsOn.Format(time.DateOnly), Overlaps: overlaps,
		}
	}
	return out
}
