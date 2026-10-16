package client

import (
	"context"

	connect "connectrpc.com/connect"

	attendancev1 "github.com/garcia-lorca/gen/go/garcialorca/attendance/v1"
	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
)

// Catalog corrections (T8, ADR 0036): fixing a grade or group captured wrong
// without deleting its level, the school order of levels, and what each
// service keeps of a group before it may be deleted.

// ReorderLevels puts the levels in the given school order and answers them.
func (c *ConfigClient) ReorderLevels(ctx context.Context, levelIDs []string) ([]LevelResult, error) {
	res, err := c.rpc.ReorderLevels(ctx, connect.NewRequest(&configv1.ReorderLevelsRequest{LevelIds: levelIDs}))
	if err != nil {
		return nil, err
	}
	out := make([]LevelResult, 0, len(res.Msg.GetLevels()))
	for _, l := range res.Msg.GetLevels() {
		out = append(out, toLevelResult(l))
	}
	return out, nil
}

// RenameGradeLevel renames a grade in place.
func (c *ConfigClient) RenameGradeLevel(ctx context.Context, id, name string) (GradeLevelResult, error) {
	res, err := c.rpc.RenameGradeLevel(ctx, connect.NewRequest(&configv1.RenameGradeLevelRequest{Id: id, Name: name}))
	if err != nil {
		return GradeLevelResult{}, err
	}
	return toGradeLevelResult(res.Msg.GetGradeLevel()), nil
}

// DeleteGradeLevel removes a grade that holds nothing.
func (c *ConfigClient) DeleteGradeLevel(ctx context.Context, id string) error {
	_, err := c.rpc.DeleteGradeLevel(ctx, connect.NewRequest(&configv1.DeleteGradeLevelRequest{Id: id}))
	return err
}

// RenameGroup renames a group in place.
func (c *ConfigClient) RenameGroup(ctx context.Context, id, name string) (GroupResult, error) {
	res, err := c.rpc.RenameGroup(ctx, connect.NewRequest(&configv1.RenameGroupRequest{Id: id, Name: name}))
	if err != nil {
		return GroupResult{}, err
	}
	return toGroupResult(res.Msg.GetGroup()), nil
}

// ConfigGroupUsage is what config keeps of a group.
type ConfigGroupUsage struct {
	TeachingAssignments int64
	ScheduleBlocks      int64
}

// GroupUsage counts config's references to a group (NotFound when missing).
func (c *ConfigClient) GroupUsage(ctx context.Context, groupID string) (ConfigGroupUsage, error) {
	res, err := c.rpc.GroupUsage(ctx, connect.NewRequest(&configv1.GroupUsageRequest{GroupId: groupID}))
	if err != nil {
		return ConfigGroupUsage{}, err
	}
	return ConfigGroupUsage{TeachingAssignments: res.Msg.GetTeachingAssignments(), ScheduleBlocks: res.Msg.GetScheduleBlocks()}, nil
}

// DeleteGroup removes a group config keeps nothing of.
func (c *ConfigClient) DeleteGroup(ctx context.Context, id string) error {
	_, err := c.rpc.DeleteGroup(ctx, connect.NewRequest(&configv1.DeleteGroupRequest{Id: id}))
	return err
}

// StudentsGroupUsage is what students keeps of a group.
type StudentsGroupUsage struct {
	Enrollments int64
	Transfers   int64
}

// GroupUsage counts the enrollments (any status) and transfers of a group.
func (c *StudentsClient) GroupUsage(ctx context.Context, groupID string) (StudentsGroupUsage, error) {
	res, err := c.rpc.GroupUsage(ctx, connect.NewRequest(&studentsv1.GroupUsageRequest{GroupId: groupID}))
	if err != nil {
		return StudentsGroupUsage{}, err
	}
	return StudentsGroupUsage{Enrollments: res.Msg.GetEnrollments(), Transfers: res.Msg.GetTransfers()}, nil
}

// RefreshGroupName relabels the active pupils' files with the group's new
// name, answering how many changed.
func (c *StudentsClient) RefreshGroupName(ctx context.Context, groupID, name string) (int64, error) {
	res, err := c.rpc.RefreshGroupName(ctx, connect.NewRequest(&studentsv1.RefreshGroupNameRequest{GroupId: groupID, Name: name}))
	if err != nil {
		return 0, err
	}
	return res.Msg.GetStudents(), nil
}

// AttendanceGroupUsage is what attendance keeps of a group.
type AttendanceGroupUsage struct {
	Sessions int64
	Marks    int64
}

// GroupUsage counts the pass lists and marks of a group.
func (c *AttendanceClient) GroupUsage(ctx context.Context, groupID string) (AttendanceGroupUsage, error) {
	res, err := c.rpc.GroupUsage(ctx, connect.NewRequest(&attendancev1.GroupUsageRequest{GroupId: groupID}))
	if err != nil {
		return AttendanceGroupUsage{}, err
	}
	return AttendanceGroupUsage{Sessions: res.Msg.GetSessions(), Marks: res.Msg.GetMarks()}, nil
}

// GroupReportCards counts the report cards issued for a group.
func (c *GradesClient) GroupReportCards(ctx context.Context, groupID string) (int64, error) {
	res, err := c.rpc.GroupUsage(ctx, connect.NewRequest(&gradesv1.GroupUsageRequest{GroupId: groupID}))
	if err != nil {
		return 0, err
	}
	return res.Msg.GetReportCards(), nil
}
