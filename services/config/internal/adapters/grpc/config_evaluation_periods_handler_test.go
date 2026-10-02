package grpcadapter_test

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	grpcadapter "github.com/garcia-lorca/services/config/internal/adapters/grpc"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

// periodsRepo adds an in-memory period store to the catalog fake, which
// already answers GetCycle and GetGradeLevel.
type periodsRepo struct {
	*fakeCatalog
	periods []domain.EvaluationPeriod
}

func (r *periodsRepo) ListEvaluationPeriods(_ context.Context, cycleID, _, gradeID string) ([]domain.EvaluationPeriod, error) {
	var out []domain.EvaluationPeriod
	for _, p := range r.periods {
		if p.CycleID() == cycleID && (gradeID == "" || p.GradeLevelID() == gradeID) {
			out = append(out, p)
		}
	}
	return out, nil
}

func (r *periodsRepo) ReplaceEvaluationPeriods(_ context.Context, cycleID string, sets []ports.GradePeriodSet) error {
	for _, set := range sets {
		kept := r.periods[:0:0]
		for _, p := range r.periods {
			if p.CycleID() != cycleID || p.GradeLevelID() != set.GradeLevelID {
				kept = append(kept, p)
			}
		}
		r.periods = append(kept, set.Periods...)
	}
	return nil
}

func newPeriodsHandler(t *testing.T) (*grpcadapter.ConfigHandler, string, string, string) {
	t.Helper()
	h, fake := newHandler(t)
	ids := 0
	svc := usecase.NewEvaluationPeriodService(&periodsRepo{fakeCatalog: fake}, tenant, func() string { ids++; return "period-" + itoa(ids) })
	h.WithEvaluationPeriods(svc)
	cycleID, levelID := seedCycle(t, h), seedLevel(t, h)
	g1 := seedGradeLevel(t, h, levelID)
	g2, err := h.CreateGradeLevel(ctx(), connect.NewRequest(&configv1.CreateGradeLevelRequest{LevelId: levelID, Name: "2°", Ordinal: 2}))
	require.NoError(t, err)
	return h, cycleID, g1, g2.Msg.GetGradeLevel().GetId()
}

func periodIn(ordinal int32, name string, from, to time.Time) *configv1.EvaluationPeriodInput {
	return &configv1.EvaluationPeriodInput{Ordinal: ordinal, Name: name, StartsOn: tsOf(from), EndsOn: tsOf(to)}
}

func TestEvaluationPeriodsUpsertListAndCopy(t *testing.T) {
	h, cycleID, g1, g2 := newPeriodsHandler(t)
	first, second := someDate.AddDate(0, 1, 0), someDate.AddDate(0, 4, 0)

	up, err := h.UpsertEvaluationPeriods(ctx(), connect.NewRequest(&configv1.UpsertEvaluationPeriodsRequest{
		CycleId: cycleID, GradeLevelId: g1,
		Periods: []*configv1.EvaluationPeriodInput{
			periodIn(2, "T2", second, second.AddDate(0, 2, 0)),
			periodIn(1, "T1", first, second.AddDate(0, 0, -1)),
		},
	}))
	require.NoError(t, err)
	require.Len(t, up.Msg.GetPeriods(), 2)
	p1 := up.Msg.GetPeriods()[0]
	assert.Equal(t, int32(1), p1.GetOrdinal())
	assert.Equal(t, "T1", p1.GetName())
	assert.Equal(t, g1, p1.GetGradeLevelId())
	assert.True(t, p1.GetStartsOn().AsTime().Equal(first))

	// Sending the 2nd period alone, by id, keeps that period (not the 1st).
	p2 := up.Msg.GetPeriods()[1]
	again, err := h.UpsertEvaluationPeriods(ctx(), connect.NewRequest(&configv1.UpsertEvaluationPeriodsRequest{
		CycleId: cycleID, GradeLevelId: g1,
		Periods: []*configv1.EvaluationPeriodInput{{Id: p2.GetId(), Ordinal: 1, Name: "T2", StartsOn: p2.GetStartsOn(), EndsOn: p2.GetEndsOn()}},
	}))
	require.NoError(t, err)
	assert.Equal(t, p2.GetId(), again.Msg.GetPeriods()[0].GetId())
	_, err = h.UpsertEvaluationPeriods(ctx(), connect.NewRequest(&configv1.UpsertEvaluationPeriodsRequest{
		CycleId: cycleID, GradeLevelId: g1,
		Periods: []*configv1.EvaluationPeriodInput{
			periodIn(2, "T2", second, second.AddDate(0, 2, 0)),
			periodIn(1, "T1", first, second.AddDate(0, 0, -1)),
		},
	}))
	require.NoError(t, err)

	cp, err := h.CopyEvaluationPeriods(ctx(), connect.NewRequest(&configv1.CopyEvaluationPeriodsRequest{
		CycleId: cycleID, FromGradeLevelId: g1, ToGradeLevelIds: []string{g2},
	}))
	require.NoError(t, err)
	require.Len(t, cp.Msg.GetPeriods(), 2)
	assert.Equal(t, g2, cp.Msg.GetPeriods()[0].GetGradeLevelId())

	list, err := h.ListEvaluationPeriods(ctx(), connect.NewRequest(&configv1.ListEvaluationPeriodsRequest{CycleId: cycleID}))
	require.NoError(t, err)
	assert.Len(t, list.Msg.GetPeriods(), 4)
}

func TestEvaluationPeriodRefusalsCarryTheirCode(t *testing.T) {
	h, cycleID, g1, _ := newPeriodsHandler(t)
	first := someDate.AddDate(0, 1, 0)
	tests := []struct {
		name     string
		periods  []*configv1.EvaluationPeriodInput
		wantCode connect.Code
		wantMeta string
	}{
		{"overlap", []*configv1.EvaluationPeriodInput{
			periodIn(1, "T1", first, first.AddDate(0, 2, 0)),
			periodIn(2, "T2", first.AddDate(0, 2, 0), first.AddDate(0, 4, 0)),
		}, connect.CodeAlreadyExists, string(domain.CodePeriodOverlap)},
		{"outside the cycle", []*configv1.EvaluationPeriodInput{
			periodIn(1, "T1", laterDate, laterDate.AddDate(0, 1, 0)),
		}, connect.CodeInvalidArgument, string(domain.CodePeriodOutsideCycle)},
		{"ends before it starts", []*configv1.EvaluationPeriodInput{
			periodIn(1, "T1", first.AddDate(0, 1, 0), first),
		}, connect.CodeInvalidArgument, string(domain.CodePeriodEndsBeforeStart)},
		{"no dates", []*configv1.EvaluationPeriodInput{{Ordinal: 1, Name: "T1"}}, connect.CodeInvalidArgument, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := h.UpsertEvaluationPeriods(ctx(), connect.NewRequest(&configv1.UpsertEvaluationPeriodsRequest{
				CycleId: cycleID, GradeLevelId: g1, Periods: tt.periods,
			}))
			var ce *connect.Error
			require.ErrorAs(t, err, &ce)
			assert.Equal(t, tt.wantCode, ce.Code())
			assert.Equal(t, tt.wantMeta, ce.Meta().Get(grpcadapter.ErrorCodeMeta))
		})
	}
}

func TestEvaluationPeriodsUnwiredIsUnimplemented(t *testing.T) {
	h, _ := newHandler(t)
	_, err := h.ListEvaluationPeriods(ctx(), connect.NewRequest(&configv1.ListEvaluationPeriodsRequest{CycleId: "c"}))
	assert.Equal(t, connect.CodeUnimplemented, codeOf(err))
}
