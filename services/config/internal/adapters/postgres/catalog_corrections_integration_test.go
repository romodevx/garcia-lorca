//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	pgadapter "github.com/garcia-lorca/services/config/internal/adapters/postgres"
	"github.com/garcia-lorca/services/config/internal/domain"
)

// Catalog corrections (T8, ADR 0036): a grade or group captured wrong is fixed
// in place or deleted alone, without deleting its level; levels list in school
// order and can be reordered. Every fixture has its own id prefix (shared DB).

// tenantT8 holds this file's fixtures: the suite's shared tenants are counted
// by other tests (ListTeachers in TestTeacherRoundTrip), so nothing is added
// to them here.
const tenantT8 = "88888888-8888-4888-8888-888888888888"

func grade(t *testing.T, repo *pgadapter.Repository, tenantID, id, levelID, name string, ordinal int) domain.GradeLevel {
	t.Helper()
	g, err := domain.NewGradeLevel(id, tenantID, levelID, name, ordinal)
	require.NoError(t, err)
	require.NoError(t, repo.CreateGradeLevel(context.Background(), g))
	return g
}

// --- grades ---------------------------------------------------------------------

func TestRenameGradeLevelInPlace(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantT8)
	_, groupID, _, _ := teachingAssignmentFixture(t, tenantT8, "e1e1")
	gradeID := "e1e10000-0000-0000-0000-000000000003"

	got, err := repo.RenameGradeLevel(ctx, gradeID, "3°")

	require.NoError(t, err)
	assert.Equal(t, []any{gradeID, "3°", 1}, []any{got.ID(), got.Name(), got.Ordinal()})
	stored, err := repo.GetGradeLevel(ctx, gradeID)
	require.NoError(t, err)
	assert.Equal(t, "3°", stored.Name())
	g, err := repo.GetGroup(ctx, groupID)
	require.NoError(t, err)
	assert.Equal(t, gradeID, g.GradeLevelID(), "its group follows the id")
}

func TestRenameGradeLevelRefusals(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantT8)
	teachingAssignmentFixture(t, tenantT8, "e2e2")
	levelID := "e2e20000-0000-0000-0000-000000000002"
	gradeID := "e2e20000-0000-0000-0000-000000000003"
	grade(t, repo, tenantT8, "e2e20000-0000-0000-0000-000000000033", levelID, "Segundo", 2)

	_, err := repo.RenameGradeLevel(ctx, gradeID, "SEGUNDO")
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err), "a sibling's name in another case")
	assert.Equal(t, domain.CodeGradeNameTaken, domain.RefusalCodeOf(err))
	assert.Equal(t, "Ya existe un grado con ese nombre en este nivel.", glerrors.SafeMessage(err))

	_, err = repo.RenameGradeLevel(ctx, "e2e20000-0000-0000-0000-0000000000ff", "x")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err), "unknown")
	_, err = repo.RenameGradeLevel(ctx, "not-a-uuid", "x")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err), "malformed")

	other := pgadapter.NewRepository(appDB(t), tenantB)
	_, err = other.RenameGradeLevel(ctx, gradeID, "Robado")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err), "another tenant's grade is invisible (RLS)")
}

func TestDeleteGradeLevelRefusesWhileItHoldsSomethingAndNamesIt(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantT8)
	cycleID, _, _, _ := teachingAssignmentFixture(t, tenantT8, "e3e3")
	gradeID := "e3e30000-0000-0000-0000-000000000003"
	writeAsTenant(t, tenantT8, `INSERT INTO evaluation_periods (tenant_id, cycle_id, grade_level_id, ordinal, name, starts_on, ends_on)
		VALUES (?, ?, ?, 1, 'Trimestre 1', '2026-09-01', '2026-11-30')`, tenantT8, cycleID, gradeID)

	err := repo.DeleteGradeLevel(ctx, gradeID)

	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Equal(t, domain.CodeGradeLevelInUse, domain.RefusalCodeOf(err))
	assert.Equal(t, "No se puede borrar el grado Grade-e3e3: tiene 1 grupo, 1 materia y 1 periodo de evaluación. Bórralos primero; si alguna materia ya tiene calificaciones, puedes cambiarle el nombre al grado.",
		glerrors.SafeMessage(err))
	_, err = repo.GetGradeLevel(ctx, gradeID)
	assert.NoError(t, err, "nothing was deleted")
}

func TestDeleteGradeLevelRemovesAnEmptyGradeAlone(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantT8)
	teachingAssignmentFixture(t, tenantT8, "e4e4")
	levelID := "e4e40000-0000-0000-0000-000000000002"
	spare := grade(t, repo, tenantT8, "e4e40000-0000-0000-0000-000000000033", levelID, "Capturado mal", 2)

	other := pgadapter.NewRepository(appDB(t), tenantB)
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(other.DeleteGradeLevel(ctx, spare.ID())), "RLS")

	require.NoError(t, repo.DeleteGradeLevel(ctx, spare.ID()))
	_, err := repo.GetGradeLevel(ctx, spare.ID())
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
	_, err = repo.GetLevel(ctx, levelID)
	assert.NoError(t, err, "the level stays")
	left, err := repo.ListGradeLevels(ctx, levelID)
	require.NoError(t, err)
	assert.Len(t, left, 1, "the sibling grade stays")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(repo.DeleteGradeLevel(ctx, spare.ID())), "twice is NotFound")
}

// --- groups ---------------------------------------------------------------------

func TestRenameGroupInPlaceAndItsConflicts(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantT8)
	cycleID, groupID, _, _ := teachingAssignmentFixture(t, tenantT8, "e5e5")
	gradeID := "e5e50000-0000-0000-0000-000000000003"
	b, err := domain.NewGroup("e5e50000-0000-0000-0000-000000000055", tenantT8, cycleID, gradeID, "B", 30)
	require.NoError(t, err)
	require.NoError(t, repo.CreateGroup(ctx, b))

	got, err := repo.RenameGroup(ctx, groupID, "C")
	require.NoError(t, err)
	assert.Equal(t, []any{groupID, cycleID, gradeID, "C", 30}, []any{got.ID(), got.CycleID(), got.GradeLevelID(), got.Name(), got.Capacity()})

	_, err = repo.RenameGroup(ctx, groupID, "B")
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Equal(t, domain.CodeGroupNameTaken, domain.RefusalCodeOf(err))
	assert.Equal(t, "Ya existe un grupo con ese nombre en este grado y ciclo.", glerrors.SafeMessage(err))

	_, err = pgadapter.NewRepository(appDB(t), tenantB).RenameGroup(ctx, groupID, "Z")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err), "RLS")
	_, err = repo.RenameGroup(ctx, "nope", "Z")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}

func TestGroupUsageCountsAssignmentsAndTimetableBlocks(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantT8)
	cycleID, groupID, subjectID, teacherID := teachingAssignmentFixture(t, tenantT8, "e6e6")
	const assignID = "e6e67000-0000-0000-0000-000000000001"
	a, err := domain.NewTeachingAssignment(assignID, tenantT8, cycleID, groupID, subjectID, teacherID)
	require.NoError(t, err)
	require.NoError(t, repo.CreateTeachingAssignment(ctx, a))
	for _, day := range []int{1, 2} {
		writeAsTenant(t, tenantT8, `INSERT INTO schedule_blocks (tenant_id, cycle_id, assignment_id, group_id, subject_id, teacher_id, weekday, starts_at, ends_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 480, 530)`, tenantT8, cycleID, assignID, groupID, subjectID, teacherID, day)
	}

	usage, err := repo.GroupUsage(ctx, groupID)
	require.NoError(t, err)
	assert.Equal(t, domain.GroupUsage{TeachingAssignments: 1, ScheduleBlocks: 2}, usage)

	err = repo.DeleteGroup(ctx, groupID)
	assert.Equal(t, domain.CodeGroupInUse, domain.RefusalCodeOf(err))
	assert.Equal(t, "No se puede borrar el grupo A: tiene 1 clase asignada y 2 bloques de horario. Quita primero sus clases asignadas.",
		glerrors.SafeMessage(err))

	_, err = pgadapter.NewRepository(appDB(t), tenantB).GroupUsage(ctx, groupID)
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err), "RLS: another tenant's group is unknown, not empty")
}

func TestDeleteGroupRemovesAnUnusedGroupAlone(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantT8)
	_, groupID, _, _ := teachingAssignmentFixture(t, tenantT8, "e7e7")

	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(pgadapter.NewRepository(appDB(t), tenantB).DeleteGroup(ctx, groupID)), "RLS")
	require.NoError(t, repo.DeleteGroup(ctx, groupID))

	_, err := repo.GetGroup(ctx, groupID)
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
	_, err = repo.GetGradeLevel(ctx, "e7e70000-0000-0000-0000-000000000003")
	assert.NoError(t, err, "the grade stays")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(repo.DeleteGroup(ctx, groupID)))
}

// --- level order ----------------------------------------------------------------

// orderTenant gets a tenant of its own: ReorderLevels needs EXACTLY the
// tenant's levels, and the suite's shared tenants hold many.
func orderTenant(t *testing.T, tenantID string, names ...string) (*pgadapter.Repository, []string) {
	t.Helper()
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantID)
	ids := make([]string, 0, len(names))
	for i, name := range names {
		id := tenantID[:8] + "-0000-4000-8000-00000000000" + string(rune('1'+i))
		l, err := domain.NewLevel(id, tenantID, name, "", domain.GranularityCycle)
		require.NoError(t, err)
		require.NoError(t, repo.CreateLevel(ctx, l))
		ids = append(ids, id)
	}
	return repo, ids
}

func listedIDs(t *testing.T, repo *pgadapter.Repository) ([]string, []int) {
	t.Helper()
	levels, err := repo.ListLevels(context.Background())
	require.NoError(t, err)
	ids, orders := make([]string, 0, len(levels)), make([]int, 0, len(levels))
	for _, l := range levels {
		ids, orders = append(ids, l.ID()), append(orders, l.SortOrder())
	}
	return ids, orders
}

func TestNewLevelsGoLastAndListLevelsAnswersInSortOrder(t *testing.T) {
	repo, ids := orderTenant(t, "a1a1a1a1-0000-4000-8000-000000000000", "Secundaria", "Primaria", "Bachillerato")

	got, orders := listedIDs(t, repo)

	assert.Equal(t, ids, got, "creation order, not alphabetical")
	assert.Equal(t, []int{1, 2, 3}, orders)
	l, err := repo.GetLevel(context.Background(), ids[2])
	require.NoError(t, err)
	assert.Equal(t, 3, l.SortOrder())
}

// ISSUE-T14-205: Horarios' level chips came in alphabetical order because the
// schedule catalog read levels by name, not by the school's configured order.
func TestScheduleCatalogListsLevelsInSortOrder(t *testing.T) {
	repo, ids := orderTenant(t, "a6a6a6a6-0000-4000-8000-000000000000", "Secundaria", "Primaria", "Bachillerato")
	want := []string{ids[1], ids[0], ids[2]}
	_, err := repo.ReorderLevels(context.Background(), want)
	require.NoError(t, err)

	cycle, err := domain.NewCycle("a6a6a6a6-0000-4000-8000-0000000000c1", "a6a6a6a6-0000-4000-8000-000000000000", "2026-2027", domain.CycleStatusDraft,
		time.Date(2026, time.August, 24, 0, 0, 0, 0, time.UTC), time.Date(2027, time.July, 9, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NoError(t, repo.CreateCycle(context.Background(), cycle))

	cat, err := repo.LoadScheduleCatalog(context.Background(), cycle.ID())

	require.NoError(t, err)
	got := make([]string, 0, len(cat.Levels))
	for _, l := range cat.Levels {
		got = append(got, l.ID())
	}
	assert.Equal(t, want, got)
}

func TestReorderLevelsRewritesTheWholeOrder(t *testing.T) {
	repo, ids := orderTenant(t, "a2a2a2a2-0000-4000-8000-000000000000", "Secundaria", "Primaria", "Preescolar")
	want := []string{ids[2], ids[1], ids[0]}

	got, err := repo.ReorderLevels(context.Background(), want)

	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, want, []string{got[0].ID(), got[1].ID(), got[2].ID()})
	listed, orders := listedIDs(t, repo)
	assert.Equal(t, want, listed)
	assert.Equal(t, []int{1, 2, 3}, orders)
}

func TestReorderLevelsRefusesAnythingButExactlyTheTenantsLevels(t *testing.T) {
	repo, ids := orderTenant(t, "a3a3a3a3-0000-4000-8000-000000000000", "Primaria", "Secundaria")
	_, foreign := orderTenant(t, "a4a4a4a4-0000-4000-8000-000000000000", "Maternal")
	cases := map[string][]string{
		"one missing":            {ids[1]},
		"a repeat":               {ids[1], ids[1]},
		"another tenant's (RLS)": {ids[1], foreign[0]},
		"one extra":              {ids[1], ids[0], foreign[0]},
		"unknown":                {ids[1], "a3a3a3a3-0000-4000-8000-0000000000ff"},
		"malformed":              {ids[1], "not-a-uuid"},
	}
	for name, proposed := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := repo.ReorderLevels(context.Background(), proposed)

			assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
			assert.Equal(t, domain.CodeLevelOrderMismatch, domain.RefusalCodeOf(err))
			listed, _ := listedIDs(t, repo)
			assert.Equal(t, ids, listed, "nothing changed")
		})
	}
}

// Security review of T8: a level in the deletion saga is frozen, and its
// trash restores what it archived. Deleting one of its grades or groups by
// hand meanwhile would have that restore bring back a row deleted on purpose.
func TestDeletingAGradeOrGroupOfALevelBeingDeletedIsRefused(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantT8)
	_, groupID, _, _ := teachingAssignmentFixture(t, tenantT8, "e8e8")
	levelID := "e8e80000-0000-0000-0000-000000000002"
	spare := grade(t, repo, tenantT8, "e8e80000-0000-0000-0000-000000000033", levelID, "En papelera", 2)
	require.NoError(t, superDB(t).Exec(`UPDATE levels SET status = 'deleting' WHERE id = ?`, levelID).Error)

	err := repo.DeleteGradeLevel(ctx, spare.ID())
	assert.Equal(t, levelarchive.CodeLevelBeingDeleted, levelarchive.CodeOf(err), "grade: %v", err)
	err = repo.DeleteGroup(ctx, groupID)
	assert.Equal(t, levelarchive.CodeLevelBeingDeleted, levelarchive.CodeOf(err), "group: %v", err)

	_, err = repo.GetGradeLevel(ctx, spare.ID())
	assert.NoError(t, err, "the grade is still there")
	_, err = repo.GetGroup(ctx, groupID)
	assert.NoError(t, err, "the group is still there")
}
