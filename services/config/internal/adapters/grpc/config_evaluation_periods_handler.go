package grpcadapter

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

// errPeriodsUnwired answers the period RPCs when the composition root did not
// wire the use case (it always does; this only guards a nil dereference).
var errPeriodsUnwired = connect.NewError(connect.CodeUnimplemented, errors.New("evaluation periods are not available"))

// WithEvaluationPeriods wires the evaluation-period use case (T14).
func (h *ConfigHandler) WithEvaluationPeriods(periods *usecase.EvaluationPeriodService) *ConfigHandler {
	h.periods = periods
	return h
}

// ListEvaluationPeriods lists a cycle's periods, optionally by level/grade.
func (h *ConfigHandler) ListEvaluationPeriods(ctx context.Context, req *connect.Request[configv1.ListEvaluationPeriodsRequest]) (*connect.Response[configv1.ListEvaluationPeriodsResponse], error) {
	if h.periods == nil {
		return nil, errPeriodsUnwired
	}
	ps, err := h.periods.ListEvaluationPeriods(ctx, req.Msg.GetCycleId(), req.Msg.GetLevelId(), req.Msg.GetGradeLevelId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.ListEvaluationPeriodsResponse{Periods: toProtoPeriods(ps)}), nil
}

// UpsertEvaluationPeriods replaces one grade's periods in a cycle.
func (h *ConfigHandler) UpsertEvaluationPeriods(ctx context.Context, req *connect.Request[configv1.UpsertEvaluationPeriodsRequest]) (*connect.Response[configv1.UpsertEvaluationPeriodsResponse], error) {
	if h.periods == nil {
		return nil, errPeriodsUnwired
	}
	inputs := make([]usecase.PeriodInput, 0, len(req.Msg.GetPeriods()))
	for _, in := range req.Msg.GetPeriods() {
		if in.GetStartsOn() == nil || in.GetEndsOn() == nil {
			return nil, connectErr(glerrors.New(glerrors.KindInvalid, "Cada periodo necesita fecha de inicio y de fin"))
		}
		inputs = append(inputs, usecase.PeriodInput{
			Ordinal: int(in.GetOrdinal()), Name: in.GetName(),
			StartsOn: asTime(in.GetStartsOn()), EndsOn: asTime(in.GetEndsOn()),
		})
	}
	ps, err := h.periods.UpsertEvaluationPeriods(ctx, req.Msg.GetCycleId(), req.Msg.GetGradeLevelId(), inputs)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.UpsertEvaluationPeriodsResponse{Periods: toProtoPeriods(ps)}), nil
}

// CopyEvaluationPeriods copies one grade's periods onto other grades.
func (h *ConfigHandler) CopyEvaluationPeriods(ctx context.Context, req *connect.Request[configv1.CopyEvaluationPeriodsRequest]) (*connect.Response[configv1.CopyEvaluationPeriodsResponse], error) {
	if h.periods == nil {
		return nil, errPeriodsUnwired
	}
	ps, err := h.periods.CopyEvaluationPeriods(ctx, req.Msg.GetCycleId(), req.Msg.GetFromGradeLevelId(), req.Msg.GetToGradeLevelIds())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.CopyEvaluationPeriodsResponse{Periods: toProtoPeriods(ps)}), nil
}

func toProtoPeriods(ps []domain.EvaluationPeriod) []*configv1.EvaluationPeriod {
	out := make([]*configv1.EvaluationPeriod, 0, len(ps))
	for _, p := range ps {
		out = append(out, &configv1.EvaluationPeriod{
			Id: p.ID(), TenantId: p.TenantID(), CycleId: p.CycleID(), GradeLevelId: p.GradeLevelID(),
			Ordinal: int32(p.Ordinal()), Name: p.Name(),
			StartsOn: timestamppb.New(p.StartsOn()), EndsOn: timestamppb.New(p.EndsOn()),
			TermId: p.TermID(),
		})
	}
	return out
}
