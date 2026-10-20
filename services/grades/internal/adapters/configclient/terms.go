package configclient

import (
	"context"
	"sort"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

// groupLabelSeparator joins a grade and its group the way every screen names
// them ("1° PRIMARIA · A").
const groupLabelSeparator = " · "

// ListTerms lists a cycle's terms (of one level when levelID is set) in ONE
// read, in ordinal order, each with its months.
func (c *Client) ListTerms(ctx context.Context, cycleID, levelID string) ([]ports.TermRef, error) {
	res, err := c.rpc.ListTerms(ctx, connect.NewRequest(&configv1.ListTermsRequest{CycleId: cycleID, LevelId: levelID}))
	if err != nil {
		return nil, mapConfigErr(err, "terms")
	}
	out := make([]ports.TermRef, 0, len(res.Msg.GetTerms()))
	for _, t := range res.Msg.GetTerms() {
		ref, err := toTermRef(t)
		if err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Ordinal < out[j].Ordinal })
	return out, nil
}

// toTermRef projects a config term. A month config sent malformed is an
// internal error: dropping it would silently unlock or hide a month.
func toTermRef(t *configv1.Term) (ports.TermRef, error) {
	months := make([]domain.Month, 0, len(t.GetMonths()))
	for _, raw := range t.GetMonths() {
		m, err := domain.ParseMonth(raw)
		if err != nil {
			return ports.TermRef{}, glerrors.Wrap(glerrors.KindInternal, "config sent a malformed term month", err)
		}
		months = append(months, m)
	}
	return ports.TermRef{
		ID: t.GetId(), CycleID: t.GetCycleId(), LevelID: t.GetLevelId(),
		Name: t.GetName(), Ordinal: int(t.GetOrdinal()), Months: months,
	}, nil
}

// LevelCatalog reads a level's catalog in one cycle in six calls whatever the
// school's size: the level, its grades, the cycle's groups, the cycle's
// assignments, every subject and every teacher.
func (c *Client) LevelCatalog(ctx context.Context, cycleID, levelID string) (ports.LevelCatalog, error) {
	res, err := c.rpc.GetLevel(ctx, connect.NewRequest(&configv1.GetLevelRequest{Id: levelID}))
	if err != nil {
		return ports.LevelCatalog{}, mapConfigErr(err, "level")
	}
	level, err := toLevelRef(res.Msg.GetLevel())
	if err != nil {
		return ports.LevelCatalog{}, err
	}
	groups, err := c.levelGroups(ctx, cycleID, levelID)
	if err != nil {
		return ports.LevelCatalog{}, err
	}
	assignments, err := c.cycleAssignments(ctx, cycleID, groups)
	if err != nil {
		return ports.LevelCatalog{}, err
	}
	subjects, err := c.subjectNames(ctx)
	if err != nil {
		return ports.LevelCatalog{}, err
	}
	teachers, err := c.teacherNames(ctx)
	if err != nil {
		return ports.LevelCatalog{}, err
	}
	return ports.LevelCatalog{
		LevelName: res.Msg.GetLevel().GetName(), GradingEnabled: level.GradingEnabled, GradingLetters: level.GradingLetters, Scale: level.Scale,
		Groups: groups, Assignments: assignments, SubjectNames: subjects, TeacherNames: teachers,
		Deleting: isDeleting(res.Msg.GetLevel()),
	}, nil
}

// levelGroups lists the level's groups in the cycle, labelled by their grade.
func (c *Client) levelGroups(ctx context.Context, cycleID, levelID string) ([]ports.GroupRef, error) {
	grades, err := c.rpc.ListGradeLevels(ctx, connect.NewRequest(&configv1.ListGradeLevelsRequest{LevelId: levelID}))
	if err != nil {
		return nil, mapConfigErr(err, "grade levels")
	}
	gradeNames := map[string]string{}
	for _, g := range grades.Msg.GetGradeLevels() {
		if g.GetLevelId() == levelID {
			gradeNames[g.GetId()] = g.GetName()
		}
	}
	groups, err := c.rpc.ListGroups(ctx, connect.NewRequest(&configv1.ListGroupsRequest{CycleId: cycleID}))
	if err != nil {
		return nil, mapConfigErr(err, "groups")
	}
	out := []ports.GroupRef{}
	for _, g := range groups.Msg.GetGroups() {
		grade, ok := gradeNames[g.GetGradeLevelId()]
		if !ok {
			continue
		}
		out = append(out, ports.GroupRef{ID: g.GetId(), Label: groupLabel(grade, g.GetName()), GradeLevelID: g.GetGradeLevelId()})
	}
	return out, nil
}

func groupLabel(grade, group string) string {
	if grade == "" {
		return group
	}
	return grade + groupLabelSeparator + group
}

// cycleAssignments lists the cycle's assignments in ONE read and keeps those
// of the given groups.
func (c *Client) cycleAssignments(ctx context.Context, cycleID string, groups []ports.GroupRef) ([]ports.AssignmentRef, error) {
	res, err := c.rpc.ListTeachingAssignments(ctx, connect.NewRequest(&configv1.ListTeachingAssignmentsRequest{CycleId: cycleID}))
	if err != nil {
		return nil, mapConfigErr(err, "teaching assignments")
	}
	inLevel := make(map[string]bool, len(groups))
	for _, g := range groups {
		inLevel[g.ID] = true
	}
	out := []ports.AssignmentRef{}
	for _, a := range res.Msg.GetTeachingAssignments() {
		if inLevel[a.GetGroupId()] {
			out = append(out, ports.AssignmentRef{ID: a.GetId(), GroupID: a.GetGroupId(), SubjectID: a.GetSubjectId(), TeacherID: a.GetTeacherId(), CycleID: a.GetCycleId()})
		}
	}
	return out, nil
}

func (c *Client) subjectNames(ctx context.Context) (map[string]string, error) {
	res, err := c.rpc.ListSubjects(ctx, connect.NewRequest(&configv1.ListSubjectsRequest{}))
	if err != nil {
		return nil, mapConfigErr(err, "subjects")
	}
	out := make(map[string]string, len(res.Msg.GetSubjects()))
	for _, s := range res.Msg.GetSubjects() {
		out[s.GetId()] = s.GetName()
	}
	return out, nil
}

func (c *Client) teacherNames(ctx context.Context) (map[string]string, error) {
	res, err := c.rpc.ListTeachers(ctx, connect.NewRequest(&configv1.ListTeachersRequest{}))
	if err != nil {
		return nil, mapConfigErr(err, "teachers")
	}
	out := make(map[string]string, len(res.Msg.GetTeachers()))
	for _, t := range res.Msg.GetTeachers() {
		out[t.GetId()] = joinName(t.GetFirstName(), t.GetLastName())
	}
	return out, nil
}

func joinName(first, last string) string {
	switch {
	case first == "":
		return last
	case last == "":
		return first
	default:
		return first + " " + last
	}
}
