package configclient

import (
	"context"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

// Compile-time proof the adapter satisfies the catalog port too.
var _ ports.CatalogReader = (*Client)(nil)

// LevelOfGroup resolves group → grade level → level (three point reads) and
// projects the level's grading scale. A missing link is glerrors.ErrNotFound.
// A level whose scale is absent or inconsistent is KindInternal: reading it as
// a zero scale would silently refuse every grade.
func (c *Client) LevelOfGroup(ctx context.Context, groupID string) (ports.LevelRef, error) {
	group, err := c.rpc.GetGroup(ctx, connect.NewRequest(&configv1.GetGroupRequest{Id: groupID}))
	if err != nil {
		return ports.LevelRef{}, mapConfigErr(err, "group")
	}
	grade, err := c.rpc.GetGradeLevel(ctx, connect.NewRequest(&configv1.GetGradeLevelRequest{Id: group.Msg.GetGroup().GetGradeLevelId()}))
	if err != nil {
		return ports.LevelRef{}, mapConfigErr(err, "grade level")
	}
	level, err := c.rpc.GetLevel(ctx, connect.NewRequest(&configv1.GetLevelRequest{Id: grade.Msg.GetGradeLevel().GetLevelId()}))
	if err != nil {
		return ports.LevelRef{}, mapConfigErr(err, "level")
	}
	l := level.Msg.GetLevel()
	g := l.GetGrading()
	if g == nil {
		return ports.LevelRef{}, glerrors.New(glerrors.KindInternal, "config level carries no grading scale")
	}
	scale, err := domain.NewGradingScale(int(g.GetMinHundredths()), int(g.GetMaxHundredths()), int(g.GetPassingHundredths()), int(g.GetDecimals()))
	if err != nil {
		return ports.LevelRef{}, glerrors.Wrap(glerrors.KindInternal, "config level grading scale is inconsistent", err)
	}
	return ports.LevelRef{ID: l.GetId(), GradingEnabled: g.GetEnabled(), Scale: scale}, nil
}

// CycleByID reads a cycle's name and dates. An unset date stays the zero time
// (never the Unix epoch). A missing cycle is glerrors.ErrNotFound.
func (c *Client) CycleByID(ctx context.Context, id string) (ports.CycleRef, error) {
	res, err := c.rpc.GetCycle(ctx, connect.NewRequest(&configv1.GetCycleRequest{Id: id}))
	if err != nil {
		return ports.CycleRef{}, mapConfigErr(err, "cycle")
	}
	cy := res.Msg.GetCycle()
	return ports.CycleRef{ID: cy.GetId(), Name: cy.GetName(), StartsOn: timeOrZero(cy.GetStartsOn()), EndsOn: timeOrZero(cy.GetEndsOn())}, nil
}

// AssignmentsByGroup lists the teaching assignments of a group in a cycle.
func (c *Client) AssignmentsByGroup(ctx context.Context, cycleID, groupID string) ([]ports.AssignmentRef, error) {
	res, err := c.rpc.ListTeachingAssignments(ctx, connect.NewRequest(&configv1.ListTeachingAssignmentsRequest{CycleId: cycleID, GroupId: groupID}))
	if err != nil {
		return nil, mapConfigErr(err, "teaching assignments")
	}
	src := res.Msg.GetTeachingAssignments()
	out := make([]ports.AssignmentRef, 0, len(src))
	for _, a := range src {
		out = append(out, ports.AssignmentRef{ID: a.GetId(), GroupID: a.GetGroupId(), SubjectID: a.GetSubjectId(), TeacherID: a.GetTeacherId(), CycleID: a.GetCycleId()})
	}
	return out, nil
}

func timeOrZero(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime().UTC()
}
