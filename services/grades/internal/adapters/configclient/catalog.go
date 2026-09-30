package configclient

import (
	"context"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

// Compile-time proof the adapter satisfies the catalog port too.
var _ ports.CatalogReader = (*Client)(nil)

// msgScaleNotConfigured is what a grading level whose scale cannot be read
// answers: config sent none (an older config, deployed after grades — config
// must be deployed first) or sent one that breaks the invariant.
const msgScaleNotConfigured = "La escala de calificación del nivel no está configurada"

// LevelOfGroup resolves group → grade level → level (three point reads) and
// projects the level's grading scale. A missing link is glerrors.ErrNotFound.
// A GRADING level whose scale is absent or inconsistent is a KindInvalid
// refusal the sheet can show (code LEVEL_SCALE_NOT_CONFIGURED): reading it as a
// zero scale would silently refuse every grade. A level that takes no numeric
// grades needs no usable scale.
func (c *Client) LevelOfGroup(ctx context.Context, groupID string) (ports.LevelRef, error) {
	l, err := c.levelOfGroup(ctx, groupID)
	if err != nil {
		return ports.LevelRef{}, err
	}
	return toLevelRef(l)
}

// toLevelRef projects a level, its grading scale and whether it is being
// deleted (see LevelOfGroup).
func toLevelRef(l *configv1.Level) (ports.LevelRef, error) {
	g := l.GetGrading()
	if g == nil {
		return ports.LevelRef{}, domain.Refusal(domain.CodeLevelScaleUnavailable, msgScaleNotConfigured)
	}
	deleting := isDeleting(l)
	if !g.GetEnabled() {
		return ports.LevelRef{ID: l.GetId(), GradingEnabled: false, Deleting: deleting}, nil
	}
	scale, err := domain.NewGradingScale(int(g.GetMinHundredths()), int(g.GetMaxHundredths()), int(g.GetPassingHundredths()), int(g.GetDecimals()))
	if err != nil {
		return ports.LevelRef{}, domain.Refusal(domain.CodeLevelScaleUnavailable, msgScaleNotConfigured)
	}
	return ports.LevelRef{ID: l.GetId(), GradingEnabled: true, Scale: scale, Deleting: deleting}, nil
}

// isDeleting reads config's level status (ADR 0021). An older config that
// sends none leaves the level writable.
func isDeleting(l *configv1.Level) bool {
	return l.GetStatus() == configv1.LevelStatus_LEVEL_STATUS_DELETING
}

// LevelIDOfGroup resolves group → grade level → level id and deletion state,
// whatever the level's scale says.
func (c *Client) LevelIDOfGroup(ctx context.Context, groupID string) (ports.LevelKey, error) {
	l, err := c.levelOfGroup(ctx, groupID)
	if err != nil {
		return ports.LevelKey{}, err
	}
	return ports.LevelKey{ID: l.GetId(), Deleting: isDeleting(l)}, nil
}

func (c *Client) levelOfGroup(ctx context.Context, groupID string) (*configv1.Level, error) {
	group, err := c.rpc.GetGroup(ctx, connect.NewRequest(&configv1.GetGroupRequest{Id: groupID}))
	if err != nil {
		return nil, mapConfigErr(err, "group")
	}
	grade, err := c.rpc.GetGradeLevel(ctx, connect.NewRequest(&configv1.GetGradeLevelRequest{Id: group.Msg.GetGroup().GetGradeLevelId()}))
	if err != nil {
		return nil, mapConfigErr(err, "grade level")
	}
	level, err := c.rpc.GetLevel(ctx, connect.NewRequest(&configv1.GetLevelRequest{Id: grade.Msg.GetGradeLevel().GetLevelId()}))
	if err != nil {
		return nil, mapConfigErr(err, "level")
	}
	return level.Msg.GetLevel(), nil
}

// ActiveCycle is the school's active cycle; false when none is active.
func (c *Client) ActiveCycle(ctx context.Context) (ports.CycleRef, bool, error) {
	res, err := c.rpc.ListCycles(ctx, connect.NewRequest(&configv1.ListCyclesRequest{}))
	if err != nil {
		return ports.CycleRef{}, false, mapConfigErr(err, "cycles")
	}
	for _, cy := range res.Msg.GetCycles() {
		if cy.GetStatus() == configv1.CycleStatus_CYCLE_STATUS_ACTIVE {
			return toCycleRef(cy), true, nil
		}
	}
	return ports.CycleRef{}, false, nil
}

// GroupsOfLevel lists the ids of a level's groups in a cycle: the level's
// grades, then the cycle's groups of those grades (two reads).
func (c *Client) GroupsOfLevel(ctx context.Context, cycleID, levelID string) ([]string, error) {
	grades, err := c.rpc.ListGradeLevels(ctx, connect.NewRequest(&configv1.ListGradeLevelsRequest{LevelId: levelID}))
	if err != nil {
		return nil, mapConfigErr(err, "grade levels")
	}
	ofLevel := map[string]bool{}
	for _, g := range grades.Msg.GetGradeLevels() {
		ofLevel[g.GetId()] = true
	}
	groups, err := c.rpc.ListGroups(ctx, connect.NewRequest(&configv1.ListGroupsRequest{CycleId: cycleID}))
	if err != nil {
		return nil, mapConfigErr(err, "groups")
	}
	var out []string
	for _, g := range groups.Msg.GetGroups() {
		if ofLevel[g.GetGradeLevelId()] {
			out = append(out, g.GetId())
		}
	}
	return out, nil
}

// CycleByID reads a cycle's name and dates. An unset date stays the zero time
// (never the Unix epoch). A missing cycle is glerrors.ErrNotFound.
func (c *Client) CycleByID(ctx context.Context, id string) (ports.CycleRef, error) {
	res, err := c.rpc.GetCycle(ctx, connect.NewRequest(&configv1.GetCycleRequest{Id: id}))
	if err != nil {
		return ports.CycleRef{}, mapConfigErr(err, "cycle")
	}
	return toCycleRef(res.Msg.GetCycle()), nil
}

func toCycleRef(cy *configv1.Cycle) ports.CycleRef {
	return ports.CycleRef{ID: cy.GetId(), Name: cy.GetName(), StartsOn: timeOrZero(cy.GetStartsOn()), EndsOn: timeOrZero(cy.GetEndsOn())}
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
