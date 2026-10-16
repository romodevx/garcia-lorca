package grpcadapter_test

import (
	"context"
	"errors"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	grpcadapter "github.com/garcia-lorca/services/config/internal/adapters/grpc"
	"github.com/garcia-lorca/services/config/internal/domain"
)

// --- fakeCatalog: catalog corrections (T8) ------------------------------------

func (f *fakeCatalog) RenameGradeLevel(_ context.Context, id, name string) (domain.GradeLevel, error) {
	cur, ok := f.gradeLevels[id]
	if !ok {
		return domain.GradeLevel{}, glerrors.ErrNotFound
	}
	if name == "Taken" {
		return domain.GradeLevel{}, domain.GradeNameTaken()
	}
	renamed, err := cur.Renamed(name)
	if err != nil {
		return domain.GradeLevel{}, err
	}
	f.gradeLevels[id] = renamed
	return renamed, nil
}

func (f *fakeCatalog) DeleteGradeLevel(_ context.Context, id string) error {
	cur, ok := f.gradeLevels[id]
	if !ok {
		return glerrors.ErrNotFound
	}
	var usage domain.GradeLevelUsage
	for _, g := range f.groups {
		if g.GradeLevelID() == id {
			usage.Groups++
		}
	}
	if err := usage.Refusal(cur.Name()); err != nil {
		return err
	}
	delete(f.gradeLevels, id)
	return nil
}

func (f *fakeCatalog) RenameGroup(_ context.Context, id, name string) (domain.Group, error) {
	cur, ok := f.groups[id]
	if !ok {
		return domain.Group{}, glerrors.ErrNotFound
	}
	if name == "Taken" {
		return domain.Group{}, domain.GroupNameTaken()
	}
	renamed, err := cur.Renamed(name)
	if err != nil {
		return domain.Group{}, err
	}
	f.groups[id] = renamed
	return renamed, nil
}

func (f *fakeCatalog) GroupUsage(_ context.Context, id string) (domain.GroupUsage, error) {
	if _, ok := f.groups[id]; !ok {
		return domain.GroupUsage{}, glerrors.ErrNotFound
	}
	var usage domain.GroupUsage
	for _, a := range f.assignments {
		if a.GroupID() == id {
			usage.TeachingAssignments++
		}
	}
	return usage, nil
}

func (f *fakeCatalog) DeleteGroup(ctx context.Context, id string) error {
	usage, err := f.GroupUsage(ctx, id)
	if err != nil {
		return err
	}
	if err := usage.Refusal(f.groups[id].Name()); err != nil {
		return err
	}
	delete(f.groups, id)
	return nil
}

func (f *fakeCatalog) ReorderLevels(_ context.Context, ids []string) ([]domain.Level, error) {
	current := make([]string, 0, len(f.levels))
	for id := range f.levels {
		current = append(current, id)
	}
	if err := domain.CheckLevelOrder(current, ids); err != nil {
		return nil, err
	}
	out := make([]domain.Level, 0, len(ids))
	for i, id := range ids {
		f.levels[id] = f.levels[id].WithSortOrder(i + 1)
		out = append(out, f.levels[id])
	}
	return out, nil
}

// refusalCode is the X-Error-Code a refusal carries on the wire.
func refusalCode(t *testing.T, err error) string {
	t.Helper()
	var ce *connect.Error
	require.True(t, errors.As(err, &ce), "a connect error, got %v", err)
	return ce.Meta().Get(grpcadapter.ErrorCodeMeta)
}

// --- levels ---------------------------------------------------------------------

func TestLevelsCarryTheirSortOrderAndReorder(t *testing.T) {
	h, _ := newHandler(t)
	prim := seedLevel(t, h)
	sec, err := h.CreateLevel(ctx(), connect.NewRequest(&configv1.CreateLevelRequest{
		Name: "Secundaria", EnrollmentGranularity: configv1.EnrollmentGranularity_ENROLLMENT_GRANULARITY_CYCLE,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(2), sec.Msg.GetLevel().GetSortOrder(), "a new level answers its place")
	secID := sec.Msg.GetLevel().GetId()

	res, err := h.ReorderLevels(ctx(), connect.NewRequest(&configv1.ReorderLevelsRequest{LevelIds: []string{secID, prim}}))

	require.NoError(t, err)
	got := res.Msg.GetLevels()
	require.Len(t, got, 2)
	assert.Equal(t, []string{secID, prim}, []string{got[0].GetId(), got[1].GetId()})
	assert.Equal(t, []int32{1, 2}, []int32{got[0].GetSortOrder(), got[1].GetSortOrder()})
}

func TestReorderLevelsMismatchIsInvalidArgumentWithItsCode(t *testing.T) {
	h, _ := newHandler(t)
	seedLevel(t, h)

	_, err := h.ReorderLevels(ctx(), connect.NewRequest(&configv1.ReorderLevelsRequest{LevelIds: []string{"stranger"}}))

	assert.Equal(t, connect.CodeInvalidArgument, codeOf(err))
	assert.Equal(t, "LEVEL_ORDER_MISMATCH", refusalCode(t, err))
}

// --- grades ---------------------------------------------------------------------

func TestRenameGradeLevelHandler(t *testing.T) {
	h, _ := newHandler(t)
	levelID := seedLevel(t, h)
	gradeID := seedGradeLevel(t, h, levelID)

	res, err := h.RenameGradeLevel(ctx(), connect.NewRequest(&configv1.RenameGradeLevelRequest{Id: gradeID, Name: "3°"}))
	require.NoError(t, err)
	got := res.Msg.GetGradeLevel()
	assert.Equal(t, []any{gradeID, levelID, "3°"}, []any{got.GetId(), got.GetLevelId(), got.GetName()})

	_, err = h.RenameGradeLevel(ctx(), connect.NewRequest(&configv1.RenameGradeLevelRequest{Id: gradeID, Name: "Taken"}))
	assert.Equal(t, connect.CodeAlreadyExists, codeOf(err))
	assert.Equal(t, "GRADE_NAME_TAKEN", refusalCode(t, err))

	_, err = h.RenameGradeLevel(ctx(), connect.NewRequest(&configv1.RenameGradeLevelRequest{Id: gradeID, Name: " "}))
	assert.Equal(t, connect.CodeInvalidArgument, codeOf(err))
	_, err = h.RenameGradeLevel(ctx(), connect.NewRequest(&configv1.RenameGradeLevelRequest{Id: "nope", Name: "x"}))
	assert.Equal(t, connect.CodeNotFound, codeOf(err))
}

func TestDeleteGradeLevelHandler(t *testing.T) {
	h, _ := newHandler(t)
	gradeID := seedGradeLevel(t, h, seedLevel(t, h))
	groupID := seedGroup(t, h, seedCycle(t, h), gradeID)

	_, err := h.DeleteGradeLevel(ctx(), connect.NewRequest(&configv1.DeleteGradeLevelRequest{Id: gradeID}))
	assert.Equal(t, connect.CodeAlreadyExists, codeOf(err))
	assert.Equal(t, "GRADE_LEVEL_IN_USE", refusalCode(t, err))
	assert.Contains(t, err.Error(), "tiene 1 grupo")

	_, err = h.DeleteGroup(ctx(), connect.NewRequest(&configv1.DeleteGroupRequest{Id: groupID}))
	require.NoError(t, err)
	_, err = h.DeleteGradeLevel(ctx(), connect.NewRequest(&configv1.DeleteGradeLevelRequest{Id: gradeID}))
	require.NoError(t, err)
	_, err = h.DeleteGradeLevel(ctx(), connect.NewRequest(&configv1.DeleteGradeLevelRequest{Id: gradeID}))
	assert.Equal(t, connect.CodeNotFound, codeOf(err))
}

// --- groups ---------------------------------------------------------------------

func TestRenameGroupHandler(t *testing.T) {
	h, _ := newHandler(t)
	cycleID := seedCycle(t, h)
	gradeID := seedGradeLevel(t, h, seedLevel(t, h))
	groupID := seedGroup(t, h, cycleID, gradeID)

	res, err := h.RenameGroup(ctx(), connect.NewRequest(&configv1.RenameGroupRequest{Id: groupID, Name: "B"}))
	require.NoError(t, err)
	got := res.Msg.GetGroup()
	assert.Equal(t, []any{groupID, cycleID, gradeID, "B"}, []any{got.GetId(), got.GetCycleId(), got.GetGradeLevelId(), got.GetName()})

	_, err = h.RenameGroup(ctx(), connect.NewRequest(&configv1.RenameGroupRequest{Id: groupID, Name: "Taken"}))
	assert.Equal(t, connect.CodeAlreadyExists, codeOf(err))
	assert.Equal(t, "GROUP_NAME_TAKEN", refusalCode(t, err))
	_, err = h.RenameGroup(ctx(), connect.NewRequest(&configv1.RenameGroupRequest{Id: "nope", Name: "x"}))
	assert.Equal(t, connect.CodeNotFound, codeOf(err))
}

func TestGroupUsageAndDeleteGroupHandler(t *testing.T) {
	h, _ := newHandler(t)
	_, groupID, subjectID, teacherID := seedKnot(t, h)
	_, err := h.CreateTeachingAssignment(ctx(), connect.NewRequest(&configv1.CreateTeachingAssignmentRequest{
		GroupId: groupID, SubjectId: subjectID, TeacherId: teacherID,
	}))
	require.NoError(t, err)

	usage, err := h.GroupUsage(ctx(), connect.NewRequest(&configv1.GroupUsageRequest{GroupId: groupID}))
	require.NoError(t, err)
	assert.Equal(t, int64(1), usage.Msg.GetTeachingAssignments())

	_, err = h.DeleteGroup(ctx(), connect.NewRequest(&configv1.DeleteGroupRequest{Id: groupID}))
	assert.Equal(t, connect.CodeAlreadyExists, codeOf(err))
	assert.Equal(t, "GROUP_IN_USE", refusalCode(t, err))

	_, err = h.GroupUsage(ctx(), connect.NewRequest(&configv1.GroupUsageRequest{GroupId: "nope"}))
	assert.Equal(t, connect.CodeNotFound, codeOf(err))
}
