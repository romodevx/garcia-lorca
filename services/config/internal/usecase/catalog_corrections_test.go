package usecase_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

// --- fakeCatalog: catalog corrections (T8) ------------------------------------

func (f *fakeCatalog) RenameGradeLevel(_ context.Context, id, name string) (domain.GradeLevel, error) {
	cur, ok := f.gradeLevels[id]
	if !ok {
		return domain.GradeLevel{}, glerrors.ErrNotFound
	}
	for _, g := range f.gradeLevels {
		if g.ID() != id && g.LevelID() == cur.LevelID() && strings.EqualFold(g.Name(), name) {
			return domain.GradeLevel{}, domain.GradeNameTaken()
		}
	}
	renamed, err := cur.Renamed(name)
	if err != nil {
		return domain.GradeLevel{}, err
	}
	f.renames++
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
	for _, s := range f.subjects {
		if s.GradeLevelID() == id {
			usage.Subjects++
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
	renamed, err := cur.Renamed(name)
	if err != nil {
		return domain.Group{}, err
	}
	f.renames++
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

// --- grades ---------------------------------------------------------------------

func TestRenameGradeLevelPersistsTheTrimmedName(t *testing.T) {
	svc, repo := newService(t)
	g := seedGradeLevel(t, svc, seedLevel(t, svc).ID())

	got, err := svc.RenameGradeLevel(ctx(), g.ID(), "  Tercero ")

	require.NoError(t, err)
	assert.Equal(t, g.ID(), got.ID())
	assert.Equal(t, "Tercero", repo.gradeLevels[g.ID()].Name())
}

func TestRenameGradeLevelRefusals(t *testing.T) {
	svc, repo := newService(t)
	g := seedGradeLevel(t, svc, seedLevel(t, svc).ID())

	_, err := svc.RenameGradeLevel(ctx(), g.ID(), "   ")
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err), "blank")
	assert.Zero(t, repo.renames, "a blank name never reaches the repository")

	_, err = svc.RenameGradeLevel(ctx(), "nope", "Tercero")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err), "unknown")
}

func TestDeleteGradeLevel(t *testing.T) {
	svc, repo := newService(t)
	l := seedLevel(t, svc)
	empty := seedGradeLevel(t, svc, l.ID())

	require.NoError(t, svc.DeleteGradeLevel(ctx(), empty.ID()))
	assert.NotContains(t, repo.gradeLevels, empty.ID())
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(svc.DeleteGradeLevel(ctx(), empty.ID())))
}

func TestDeleteGradeLevelWithSubjectsSaysWhatBlocksIt(t *testing.T) {
	svc, repo := newService(t)
	g := seedGradeLevel(t, svc, seedLevel(t, svc).ID())
	_, err := svc.CreateSubject(ctx(), g.ID(), "Matemáticas")
	require.NoError(t, err)

	err = svc.DeleteGradeLevel(ctx(), g.ID())

	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Equal(t, domain.CodeGradeLevelInUse, domain.RefusalCodeOf(err))
	assert.Contains(t, glerrors.SafeMessage(err), "1 materia")
	assert.Contains(t, repo.gradeLevels, g.ID(), "nothing was deleted")
}

// --- groups ---------------------------------------------------------------------

func seedGroup(t *testing.T, svc *usecase.CatalogService, cycleID, gradeID string) domain.Group {
	t.Helper()
	g, err := svc.CreateGroup(ctx(), cycleID, gradeID, "A", 25)
	require.NoError(t, err)
	return g
}

func TestRenameGroupPersistsTheTrimmedName(t *testing.T) {
	svc, repo := newService(t)
	g := seedGroup(t, svc, seedCycle(t, svc).ID(), seedGradeLevel(t, svc, seedLevel(t, svc).ID()).ID())

	got, err := svc.RenameGroup(ctx(), g.ID(), " B ")

	require.NoError(t, err)
	assert.Equal(t, "B", got.Name())
	assert.Equal(t, "B", repo.groups[g.ID()].Name())
}

func TestRenameGroupRefusals(t *testing.T) {
	svc, repo := newService(t)
	g := seedGroup(t, svc, seedCycle(t, svc).ID(), seedGradeLevel(t, svc, seedLevel(t, svc).ID()).ID())

	_, err := svc.RenameGroup(ctx(), g.ID(), "")
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
	assert.Zero(t, repo.renames)

	_, err = svc.RenameGroup(ctx(), "nope", "B")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}

func TestGroupUsageAndDeleteGroup(t *testing.T) {
	svc, repo := newService(t)
	g := seedGroup(t, svc, seedCycle(t, svc).ID(), seedGradeLevel(t, svc, seedLevel(t, svc).ID()).ID())

	usage, err := svc.GroupUsage(ctx(), g.ID())
	require.NoError(t, err)
	assert.Equal(t, domain.GroupUsage{}, usage)

	require.NoError(t, svc.DeleteGroup(ctx(), g.ID()))
	assert.NotContains(t, repo.groups, g.ID())

	_, err = svc.GroupUsage(ctx(), g.ID())
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}

// --- level order ----------------------------------------------------------------

func TestReorderLevelsAnswersTheNewOrder(t *testing.T) {
	svc, _ := newService(t)
	prim := seedLevel(t, svc)
	pre, err := svc.CreateLevel(ctx(), "Preescolar", "", domain.GranularityCycle)
	require.NoError(t, err)

	got, err := svc.ReorderLevels(ctx(), []string{pre.ID(), prim.ID()})

	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, []string{pre.ID(), prim.ID()}, []string{got[0].ID(), got[1].ID()})
	assert.Equal(t, []int{1, 2}, []int{got[0].SortOrder(), got[1].SortOrder()})
}

func TestReorderLevelsRefusesAPartialList(t *testing.T) {
	svc, _ := newService(t)
	prim := seedLevel(t, svc)
	_, err := svc.CreateLevel(ctx(), "Preescolar", "", domain.GranularityCycle)
	require.NoError(t, err)

	_, err = svc.ReorderLevels(ctx(), []string{prim.ID()})

	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
	assert.Equal(t, domain.CodeLevelOrderMismatch, domain.RefusalCodeOf(err))
}

func TestCreateLevelAnswersThePlaceTheRepositoryGaveIt(t *testing.T) {
	svc, _ := newService(t)
	seedLevel(t, svc)

	l, err := svc.CreateLevel(ctx(), "Secundaria", "", domain.GranularityCycle)

	require.NoError(t, err)
	assert.Equal(t, 2, l.SortOrder())
}
