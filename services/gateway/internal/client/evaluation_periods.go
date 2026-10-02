package client

import (
	"context"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
)

// EvaluationPeriod is one evaluation period of a grade in a cycle (T14).
// StartsOn/EndsOn are calendar dates (midnight UTC), both inclusive. TermID is
// the level term a migrated period came from, or "".
type EvaluationPeriod struct {
	ID           string
	CycleID      string
	GradeLevelID string
	Ordinal      int
	Name         string
	StartsOn     time.Time
	EndsOn       time.Time
	TermID       string
}

// EvaluationPeriodInput is one period of a grade's new set. ID is the grade's
// existing period the row edits, or "" for a new one.
type EvaluationPeriodInput struct {
	ID       string
	Ordinal  int
	Name     string
	StartsOn time.Time
	EndsOn   time.Time
}

// ListEvaluationPeriods lists a cycle's periods, optionally narrowed to a
// level's grades and/or one grade.
func (c *ConfigClient) ListEvaluationPeriods(ctx context.Context, cycleID, levelID, gradeLevelID string) ([]EvaluationPeriod, error) {
	res, err := c.rpc.ListEvaluationPeriods(ctx, connect.NewRequest(&configv1.ListEvaluationPeriodsRequest{
		CycleId: cycleID, LevelId: levelID, GradeLevelId: gradeLevelID,
	}))
	if err != nil {
		return nil, err
	}
	return toEvaluationPeriods(res.Msg.GetPeriods()), nil
}

// UpsertEvaluationPeriods replaces one grade's periods in a cycle.
func (c *ConfigClient) UpsertEvaluationPeriods(ctx context.Context, cycleID, gradeLevelID string, in []EvaluationPeriodInput) ([]EvaluationPeriod, error) {
	periods := make([]*configv1.EvaluationPeriodInput, 0, len(in))
	for _, p := range in {
		periods = append(periods, &configv1.EvaluationPeriodInput{
			Id: p.ID, Ordinal: int32(p.Ordinal), Name: p.Name, //nolint:gosec // 1..12, checked at the edge
			StartsOn: timestamppb.New(p.StartsOn), EndsOn: timestamppb.New(p.EndsOn),
		})
	}
	res, err := c.rpc.UpsertEvaluationPeriods(ctx, connect.NewRequest(&configv1.UpsertEvaluationPeriodsRequest{
		CycleId: cycleID, GradeLevelId: gradeLevelID, Periods: periods,
	}))
	if err != nil {
		return nil, err
	}
	return toEvaluationPeriods(res.Msg.GetPeriods()), nil
}

// CopyEvaluationPeriods gives each target grade the source grade's periods.
func (c *ConfigClient) CopyEvaluationPeriods(ctx context.Context, cycleID, fromGradeLevelID string, toGradeLevelIDs []string) ([]EvaluationPeriod, error) {
	res, err := c.rpc.CopyEvaluationPeriods(ctx, connect.NewRequest(&configv1.CopyEvaluationPeriodsRequest{
		CycleId: cycleID, FromGradeLevelId: fromGradeLevelID, ToGradeLevelIds: toGradeLevelIDs,
	}))
	if err != nil {
		return nil, err
	}
	return toEvaluationPeriods(res.Msg.GetPeriods()), nil
}

func toEvaluationPeriods(ps []*configv1.EvaluationPeriod) []EvaluationPeriod {
	out := make([]EvaluationPeriod, 0, len(ps))
	for _, p := range ps {
		out = append(out, EvaluationPeriod{
			ID: p.GetId(), CycleID: p.GetCycleId(), GradeLevelID: p.GetGradeLevelId(),
			Ordinal: int(p.GetOrdinal()), Name: p.GetName(),
			StartsOn: p.GetStartsOn().AsTime().UTC(), EndsOn: p.GetEndsOn().AsTime().UTC(),
			TermID: p.GetTermId(),
		})
	}
	return out
}
