package client

import (
	"context"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
)

// TeachingLoadItem is one (group, subject) slot of a teaching load.
type TeachingLoadItem struct {
	GroupID   string
	SubjectID string
}

// TeachingLoadInput gives one teacher a batch of slots ("carga académica").
type TeachingLoadInput struct {
	TeacherID        string
	Items            []TeachingLoadItem
	ReplaceConflicts bool
}

// TeachingLoadConflict is a requested slot another teacher already holds.
type TeachingLoadConflict struct {
	GroupID      string
	SubjectID    string
	AssignmentID string
	TeacherID    string
	TeacherName  string
}

// TeachingLoadResult says what happened to every item of a load.
type TeachingLoadResult struct {
	Created    []TeachingAssignment
	Reassigned []TeachingAssignment
	Skipped    []TeachingLoadItem
	Conflicts  []TeachingLoadConflict
}

// AssignTeachingLoad forwards a teaching load to the config service, which
// applies it in one transaction.
func (c *ConfigClient) AssignTeachingLoad(ctx context.Context, in TeachingLoadInput) (TeachingLoadResult, error) {
	items := make([]*configv1.TeachingLoadItem, 0, len(in.Items))
	for _, it := range in.Items {
		items = append(items, &configv1.TeachingLoadItem{GroupId: it.GroupID, SubjectId: it.SubjectID})
	}
	res, err := c.rpc.AssignTeachingLoad(ctx, connect.NewRequest(&configv1.AssignTeachingLoadRequest{
		TeacherId:        in.TeacherID,
		Items:            items,
		ReplaceConflicts: in.ReplaceConflicts,
	}))
	if err != nil {
		return TeachingLoadResult{}, err
	}
	return toTeachingLoadResult(res.Msg), nil
}

// DeleteTeachingAssignment frees a slot.
func (c *ConfigClient) DeleteTeachingAssignment(ctx context.Context, id string) error {
	_, err := c.rpc.DeleteTeachingAssignment(ctx, connect.NewRequest(&configv1.DeleteTeachingAssignmentRequest{Id: id}))
	return err
}

// ReassignTeachingAssignment hands an existing slot to another teacher.
func (c *ConfigClient) ReassignTeachingAssignment(ctx context.Context, id, teacherID string) (TeachingAssignment, error) {
	res, err := c.rpc.ReassignTeachingAssignment(ctx, connect.NewRequest(&configv1.ReassignTeachingAssignmentRequest{
		Id:        id,
		TeacherId: teacherID,
	}))
	if err != nil {
		return TeachingAssignment{}, err
	}
	return toTeachingAssignment(res.Msg.GetTeachingAssignment()), nil
}

func toTeachingLoadResult(msg *configv1.AssignTeachingLoadResponse) TeachingLoadResult {
	out := TeachingLoadResult{
		Created:    make([]TeachingAssignment, 0, len(msg.GetCreated())),
		Reassigned: make([]TeachingAssignment, 0, len(msg.GetReassigned())),
		Skipped:    make([]TeachingLoadItem, 0, len(msg.GetSkipped())),
		Conflicts:  make([]TeachingLoadConflict, 0, len(msg.GetConflicts())),
	}
	for _, a := range msg.GetCreated() {
		out.Created = append(out.Created, toTeachingAssignment(a))
	}
	for _, a := range msg.GetReassigned() {
		out.Reassigned = append(out.Reassigned, toTeachingAssignment(a))
	}
	for _, s := range msg.GetSkipped() {
		out.Skipped = append(out.Skipped, TeachingLoadItem{GroupID: s.GetGroupId(), SubjectID: s.GetSubjectId()})
	}
	for _, c := range msg.GetConflicts() {
		out.Conflicts = append(out.Conflicts, TeachingLoadConflict{
			GroupID:      c.GetGroupId(),
			SubjectID:    c.GetSubjectId(),
			AssignmentID: c.GetAssignmentId(),
			TeacherID:    c.GetTeacherId(),
			TeacherName:  c.GetTeacherName(),
		})
	}
	return out
}
