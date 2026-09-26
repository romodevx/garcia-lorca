package grpcadapter_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	grpcadapter "github.com/garcia-lorca/services/config/internal/adapters/grpc"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

// --- fake: the teaching-load seam (mirrors the adapter's snapshot/plan/write) --

func (f *fakeCatalog) ApplyTeachingLoad(_ context.Context, teacherID string, slots []domain.TeachingSlot, plan func(ports.TeachingLoadSnapshot) (ports.TeachingLoadWrite, error)) error {
	snap := ports.TeachingLoadSnapshot{
		Groups:   map[string]domain.Group{},
		Subjects: map[string]domain.Subject{},
		Occupied: map[domain.TeachingSlot]domain.TeachingAssignment{},
		Holders:  map[string]domain.Teacher{},
	}
	snap.Teacher, snap.TeacherFound = f.teachers[teacherID]
	wanted := map[domain.TeachingSlot]bool{}
	for _, s := range slots {
		wanted[s] = true
		if g, ok := f.groups[s.GroupID]; ok {
			snap.Groups[s.GroupID] = g
		}
		if su, ok := f.subjects[s.SubjectID]; ok {
			snap.Subjects[s.SubjectID] = su
		}
	}
	for _, a := range f.assignments {
		if wanted[a.Slot()] {
			snap.Occupied[a.Slot()] = a
			snap.Holders[a.TeacherID()] = f.teachers[a.TeacherID()]
		}
	}
	w, err := plan(snap)
	if err != nil {
		return err
	}
	for _, a := range w.Create {
		if err := f.CreateTeachingAssignment(context.Background(), a); err != nil {
			return err
		}
	}
	for _, a := range w.Reassign {
		f.assignments[a.ID()] = a
	}
	return nil
}

func (f *fakeCatalog) DeleteTeachingAssignment(_ context.Context, id string) error {
	a, ok := f.assignments[id]
	if !ok {
		return glerrors.ErrNotFound
	}
	delete(f.assignments, id)
	delete(f.unique, "assignment|"+a.CycleID()+"|"+a.GroupID()+"|"+a.SubjectID())
	return nil
}

func (f *fakeCatalog) UpdateTeachingAssignmentTeacher(_ context.Context, id, teacherID string) error {
	a, ok := f.assignments[id]
	if !ok {
		return glerrors.ErrNotFound
	}
	moved, err := a.ReassignTo(teacherID)
	if err != nil {
		return err
	}
	f.assignments[id] = moved
	return nil
}

// newUUIDHandler is newHandler with a mint of well-formed uuids: the teaching
// load RPCs reject malformed ids at the boundary, so the fixtures must be real
// uuids. They carry hex letters so an upper-cased copy is a different string.
func newUUIDHandler(t *testing.T) (*grpcadapter.ConfigHandler, *fakeCatalog) {
	t.Helper()
	repo := newFake()
	ids := 0
	svc := usecase.NewCatalogService(repo, tenant, func() string {
		ids++
		return fmt.Sprintf("abcdef00-0000-4000-8000-%012d", ids)
	})
	return grpcadapter.NewConfigHandler(svc, usecase.NewHealthService("config")), repo
}

const unknownUUID = "99999999-9999-4999-8999-999999999999"

func TestAssignTeachingLoadHandlerMapsTheResult(t *testing.T) {
	h, _ := newUUIDHandler(t)
	_, groupID, subjectID, teacherID := seedKnot(t, h)
	otherTeacher := seedTeacher(t, h, "ada@example.com")
	gradeID := func() string {
		g, err := h.GetGroup(ctx(), connect.NewRequest(&configv1.GetGroupRequest{Id: groupID}))
		require.NoError(t, err)
		return g.Msg.GetGroup().GetGradeLevelId()
	}()
	secondSubject, err := h.CreateSubject(ctx(), connect.NewRequest(&configv1.CreateSubjectRequest{GradeLevelId: gradeID, Name: "Español"}))
	require.NoError(t, err)
	held, err := h.CreateTeachingAssignment(ctx(), connect.NewRequest(&configv1.CreateTeachingAssignmentRequest{
		GroupId: groupID, SubjectId: secondSubject.Msg.GetSubject().GetId(), TeacherId: otherTeacher,
	}))
	require.NoError(t, err)

	res, err := h.AssignTeachingLoad(ctx(), connect.NewRequest(&configv1.AssignTeachingLoadRequest{
		TeacherId: teacherID,
		Items: []*configv1.TeachingLoadItem{
			{GroupId: groupID, SubjectId: subjectID},
			{GroupId: groupID, SubjectId: secondSubject.Msg.GetSubject().GetId()},
		},
	}))

	require.NoError(t, err)
	require.Len(t, res.Msg.GetCreated(), 1)
	assert.Equal(t, subjectID, res.Msg.GetCreated()[0].GetSubjectId())
	assert.Equal(t, teacherID, res.Msg.GetCreated()[0].GetTeacherId())
	require.Len(t, res.Msg.GetConflicts(), 1)
	c := res.Msg.GetConflicts()[0]
	assert.Equal(t, groupID, c.GetGroupId())
	assert.Equal(t, secondSubject.Msg.GetSubject().GetId(), c.GetSubjectId())
	assert.Equal(t, held.Msg.GetTeachingAssignment().GetId(), c.GetAssignmentId())
	assert.Equal(t, otherTeacher, c.GetTeacherId())
	assert.NotEmpty(t, c.GetTeacherName())
	assert.Empty(t, res.Msg.GetReassigned())

	again, err := h.AssignTeachingLoad(ctx(), connect.NewRequest(&configv1.AssignTeachingLoadRequest{
		TeacherId:        teacherID,
		Items:            []*configv1.TeachingLoadItem{{GroupId: groupID, SubjectId: subjectID}, {GroupId: groupID, SubjectId: secondSubject.Msg.GetSubject().GetId()}},
		ReplaceConflicts: true,
	}))
	require.NoError(t, err)
	require.Len(t, again.Msg.GetSkipped(), 1)
	assert.Equal(t, subjectID, again.Msg.GetSkipped()[0].GetSubjectId())
	require.Len(t, again.Msg.GetReassigned(), 1)
	assert.Equal(t, held.Msg.GetTeachingAssignment().GetId(), again.Msg.GetReassigned()[0].GetId())
	assert.Equal(t, teacherID, again.Msg.GetReassigned()[0].GetTeacherId())
}

func TestAssignTeachingLoadHandlerRejectsBadRequests(t *testing.T) {
	tests := []struct {
		name     string
		req      func(groupID, subjectID, teacherID string) *configv1.AssignTeachingLoadRequest
		wantCode connect.Code
		wantMsg  string
	}{
		{"malformed teacher id", func(g, s, _ string) *configv1.AssignTeachingLoadRequest {
			return &configv1.AssignTeachingLoadRequest{TeacherId: "nope", Items: []*configv1.TeachingLoadItem{{GroupId: g, SubjectId: s}}}
		}, connect.CodeInvalidArgument, "teacher_id"},
		{"malformed group id names the item", func(g, s, te string) *configv1.AssignTeachingLoadRequest {
			return &configv1.AssignTeachingLoadRequest{TeacherId: te, Items: []*configv1.TeachingLoadItem{{GroupId: g, SubjectId: s}, {GroupId: "x", SubjectId: s}}}
		}, connect.CodeInvalidArgument, "items[1]"},
		{"malformed subject id names the item", func(g, _, te string) *configv1.AssignTeachingLoadRequest {
			return &configv1.AssignTeachingLoadRequest{TeacherId: te, Items: []*configv1.TeachingLoadItem{{GroupId: g, SubjectId: "x"}}}
		}, connect.CodeInvalidArgument, "items[0]"},
		{"unknown group", func(_, s, te string) *configv1.AssignTeachingLoadRequest {
			return &configv1.AssignTeachingLoadRequest{TeacherId: te, Items: []*configv1.TeachingLoadItem{{GroupId: unknownUUID, SubjectId: s}}}
		}, connect.CodeInvalidArgument, "items[0]: group not found"},
		{"no items", func(_, _, te string) *configv1.AssignTeachingLoadRequest {
			return &configv1.AssignTeachingLoadRequest{TeacherId: te}
		}, connect.CodeInvalidArgument, "items"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, repo := newUUIDHandler(t)
			_, groupID, subjectID, teacherID := seedKnot(t, h)

			_, err := h.AssignTeachingLoad(ctx(), connect.NewRequest(tt.req(groupID, subjectID, teacherID)))

			require.Error(t, err)
			assert.Equal(t, tt.wantCode, codeOf(err))
			assert.Contains(t, err.Error(), tt.wantMsg)
			assert.Empty(t, repo.assignments)
		})
	}
}

func TestDeleteTeachingAssignmentHandler(t *testing.T) {
	h, repo := newUUIDHandler(t)
	_, groupID, subjectID, teacherID := seedKnot(t, h)
	a, err := h.CreateTeachingAssignment(ctx(), connect.NewRequest(&configv1.CreateTeachingAssignmentRequest{GroupId: groupID, SubjectId: subjectID, TeacherId: teacherID}))
	require.NoError(t, err)
	id := a.Msg.GetTeachingAssignment().GetId()

	_, err = h.DeleteTeachingAssignment(ctx(), connect.NewRequest(&configv1.DeleteTeachingAssignmentRequest{Id: id}))
	require.NoError(t, err)
	assert.Empty(t, repo.assignments)

	tests := []struct {
		name     string
		id       string
		wantCode connect.Code
	}{
		{"already gone", id, connect.CodeNotFound},
		{"malformed id", "nope", connect.CodeInvalidArgument},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := h.DeleteTeachingAssignment(ctx(), connect.NewRequest(&configv1.DeleteTeachingAssignmentRequest{Id: tt.id}))
			assert.Equal(t, tt.wantCode, codeOf(err))
		})
	}
}

func TestReassignTeachingAssignmentHandler(t *testing.T) {
	h, _ := newUUIDHandler(t)
	_, groupID, subjectID, teacherID := seedKnot(t, h)
	other := seedTeacher(t, h, "ada@example.com")
	a, err := h.CreateTeachingAssignment(ctx(), connect.NewRequest(&configv1.CreateTeachingAssignmentRequest{GroupId: groupID, SubjectId: subjectID, TeacherId: teacherID}))
	require.NoError(t, err)
	id := a.Msg.GetTeachingAssignment().GetId()

	res, err := h.ReassignTeachingAssignment(ctx(), connect.NewRequest(&configv1.ReassignTeachingAssignmentRequest{Id: id, TeacherId: other}))
	require.NoError(t, err)
	assert.Equal(t, id, res.Msg.GetTeachingAssignment().GetId())
	assert.Equal(t, other, res.Msg.GetTeachingAssignment().GetTeacherId())

	tests := []struct {
		name      string
		id        string
		teacherID string
		wantCode  connect.Code
	}{
		{"unknown assignment", unknownUUID, other, connect.CodeNotFound},
		{"unknown teacher", id, unknownUUID, connect.CodeInvalidArgument},
		{"malformed id", "nope", other, connect.CodeInvalidArgument},
		{"malformed teacher id", id, "nope", connect.CodeInvalidArgument},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := h.ReassignTeachingAssignment(ctx(), connect.NewRequest(&configv1.ReassignTeachingAssignmentRequest{Id: tt.id, TeacherId: tt.teacherID}))
			assert.Equal(t, tt.wantCode, codeOf(err))
		})
	}
}

// Uuids are case-insensitive: an edge may send upper-case hex while Postgres
// hands back lower case. The handler canonicalizes every id, so duplicates,
// slot lookups and "already yours" compare equal whatever the case.
func TestTeachingLoadHandlersCanonicalizeIDCase(t *testing.T) {
	up := strings.ToUpper
	h, repo := newUUIDHandler(t)
	_, groupID, subjectID, teacherID := seedKnot(t, h)
	other := seedTeacher(t, h, "ada@example.com")

	res, err := h.AssignTeachingLoad(ctx(), connect.NewRequest(&configv1.AssignTeachingLoadRequest{
		TeacherId: up(teacherID),
		Items:     []*configv1.TeachingLoadItem{{GroupId: up(groupID), SubjectId: up(subjectID)}},
	}))
	require.NoError(t, err, "upper-case ids are accepted")
	require.Len(t, res.Msg.GetCreated(), 1)
	created := res.Msg.GetCreated()[0]
	assert.Equal(t, teacherID, created.GetTeacherId(), "stored in canonical lower case")
	assert.Equal(t, groupID, created.GetGroupId())
	assert.Equal(t, subjectID, created.GetSubjectId())

	again, err := h.AssignTeachingLoad(ctx(), connect.NewRequest(&configv1.AssignTeachingLoadRequest{
		TeacherId: up(teacherID),
		Items:     []*configv1.TeachingLoadItem{{GroupId: up(groupID), SubjectId: up(subjectID)}},
	}))
	require.NoError(t, err)
	assert.Len(t, again.Msg.GetSkipped(), 1, "the teacher's own slot sent upper-case is skipped")
	assert.Empty(t, again.Msg.GetConflicts(), "not reported as a conflict with itself")

	_, err = h.AssignTeachingLoad(ctx(), connect.NewRequest(&configv1.AssignTeachingLoadRequest{
		TeacherId: other,
		Items: []*configv1.TeachingLoadItem{
			{GroupId: up(groupID), SubjectId: up(subjectID)},
			{GroupId: groupID, SubjectId: subjectID},
		},
	}))
	require.Error(t, err, "the same slot in two cases is one slot twice")
	assert.Equal(t, connect.CodeInvalidArgument, codeOf(err))
	assert.Contains(t, err.Error(), "items[1]")

	id := created.GetId()
	moved, err := h.ReassignTeachingAssignment(ctx(), connect.NewRequest(&configv1.ReassignTeachingAssignmentRequest{Id: up(id), TeacherId: up(other)}))
	require.NoError(t, err)
	assert.Equal(t, id, moved.Msg.GetTeachingAssignment().GetId())
	assert.Equal(t, other, moved.Msg.GetTeachingAssignment().GetTeacherId())

	_, err = h.DeleteTeachingAssignment(ctx(), connect.NewRequest(&configv1.DeleteTeachingAssignmentRequest{Id: up(id)}))
	require.NoError(t, err)
	assert.Empty(t, repo.assignments)
}
