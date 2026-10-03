package client

import (
	"context"

	"connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

// T18: scholarships (becas) and who holds them. Value is basis points for a
// percent and cents otherwise; days are YYYY-MM-DD on the wire, always.

// ScholarshipInput is what create and update take.
type ScholarshipInput struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Value   int64    `json:"value"`
	Targets []string `json:"targets"`
	Active  bool     `json:"active"`
}

// Scholarship is a scholarship as the panel reads it.
type Scholarship struct {
	ID string `json:"id"`
	ScholarshipInput
	AssignedCount int32 `json:"assignedCount"`
}

// ScholarshipTarget is one thing a scholarship can apply to.
type ScholarshipTarget struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Group string `json:"group"`
}

// ScholarshipOverlap names another scholarship a holder has on a shared target.
type ScholarshipOverlap struct {
	ScholarshipID string `json:"scholarshipId"`
	Name          string `json:"name"`
}

// ScholarshipHolder is a pupil holding a scholarship.
type ScholarshipHolder struct {
	StudentID string               `json:"studentId"`
	StartsOn  string               `json:"startsOn"`
	Overlaps  []ScholarshipOverlap `json:"overlaps"`
}

// ListScholarships lists the scholarships.
func (c *FinanceClient) ListScholarships(ctx context.Context, includeInactive bool) ([]Scholarship, error) {
	res, err := c.rpc.ListScholarships(ctx, connect.NewRequest(&financev1.ListScholarshipsRequest{IncludeInactive: includeInactive}))
	if err != nil {
		return nil, err
	}
	out := make([]Scholarship, 0, len(res.Msg.GetScholarships()))
	for _, s := range res.Msg.GetScholarships() {
		out = append(out, toScholarship(s))
	}
	return out, nil
}

// GetScholarship reads one.
func (c *FinanceClient) GetScholarship(ctx context.Context, id string) (Scholarship, error) {
	res, err := c.rpc.GetScholarship(ctx, connect.NewRequest(&financev1.GetScholarshipRequest{Id: id}))
	if err != nil {
		return Scholarship{}, err
	}
	return toScholarship(res.Msg.GetScholarship()), nil
}

// CreateScholarship adds one.
func (c *FinanceClient) CreateScholarship(ctx context.Context, in ScholarshipInput) (Scholarship, error) {
	res, err := c.rpc.CreateScholarship(ctx, connect.NewRequest(&financev1.CreateScholarshipRequest{Scholarship: scholarshipInputToProto(in)}))
	if err != nil {
		return Scholarship{}, err
	}
	return toScholarship(res.Msg.GetScholarship()), nil
}

// UpdateScholarship replaces one's terms.
func (c *FinanceClient) UpdateScholarship(ctx context.Context, id string, in ScholarshipInput) (Scholarship, error) {
	res, err := c.rpc.UpdateScholarship(ctx, connect.NewRequest(&financev1.UpdateScholarshipRequest{Id: id, Scholarship: scholarshipInputToProto(in)}))
	if err != nil {
		return Scholarship{}, err
	}
	return toScholarship(res.Msg.GetScholarship()), nil
}

// ListScholarshipTargets lists what a scholarship can apply to today.
func (c *FinanceClient) ListScholarshipTargets(ctx context.Context) ([]ScholarshipTarget, error) {
	res, err := c.rpc.ListScholarshipTargets(ctx, connect.NewRequest(&financev1.ListScholarshipTargetsRequest{}))
	if err != nil {
		return nil, err
	}
	out := make([]ScholarshipTarget, 0, len(res.Msg.GetTargets()))
	for _, t := range res.Msg.GetTargets() {
		out = append(out, ScholarshipTarget{Key: t.GetKey(), Label: t.GetLabel(), Group: t.GetGroup()})
	}
	return out, nil
}

// ListScholarshipHolders lists who holds one now.
func (c *FinanceClient) ListScholarshipHolders(ctx context.Context, id string) ([]ScholarshipHolder, error) {
	res, err := c.rpc.ListScholarshipHolders(ctx, connect.NewRequest(&financev1.ListScholarshipHoldersRequest{ScholarshipId: id}))
	if err != nil {
		return nil, err
	}
	return toScholarshipHolders(res.Msg.GetHolders()), nil
}

// AssignScholarship gives one to pupils and returns the new holders.
func (c *FinanceClient) AssignScholarship(ctx context.Context, id string, studentIDs []string) ([]ScholarshipHolder, error) {
	res, err := c.rpc.AssignScholarship(ctx, connect.NewRequest(&financev1.AssignScholarshipRequest{ScholarshipId: id, StudentIds: studentIDs}))
	if err != nil {
		return nil, err
	}
	return toScholarshipHolders(res.Msg.GetHolders()), nil
}

// UnassignScholarship takes one away from pupils and reports how many.
func (c *FinanceClient) UnassignScholarship(ctx context.Context, id string, studentIDs []string) (int32, error) {
	res, err := c.rpc.UnassignScholarship(ctx, connect.NewRequest(&financev1.UnassignScholarshipRequest{ScholarshipId: id, StudentIds: studentIDs}))
	if err != nil {
		return 0, err
	}
	return res.Msg.GetRemoved(), nil
}

// --- mapping ---------------------------------------------------------------

func scholarshipInputToProto(in ScholarshipInput) *financev1.ScholarshipInput {
	return &financev1.ScholarshipInput{Name: in.Name, Kind: in.Kind, Value: in.Value, Targets: in.Targets, Active: in.Active}
}

func toScholarship(s *financev1.Scholarship) Scholarship {
	targets := append([]string{}, s.GetTargets()...)
	return Scholarship{
		ID: s.GetId(),
		ScholarshipInput: ScholarshipInput{
			Name: s.GetName(), Kind: s.GetKind(), Value: s.GetValue(), Targets: targets, Active: s.GetActive(),
		},
		AssignedCount: s.GetAssignedCount(),
	}
}

func toScholarshipHolders(rows []*financev1.ScholarshipHolder) []ScholarshipHolder {
	out := make([]ScholarshipHolder, 0, len(rows))
	for _, h := range rows {
		overlaps := make([]ScholarshipOverlap, 0, len(h.GetOverlaps()))
		for _, o := range h.GetOverlaps() {
			overlaps = append(overlaps, ScholarshipOverlap{ScholarshipID: o.GetId(), Name: o.GetName()})
		}
		out = append(out, ScholarshipHolder{StudentID: h.GetStudentId(), StartsOn: h.GetStartsOn(), Overlaps: overlaps})
	}
	return out
}
