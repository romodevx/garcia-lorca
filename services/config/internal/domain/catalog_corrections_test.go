package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func TestGradeLevelRenamedKeepsIdentityLevelAndOrdinal(t *testing.T) {
	g, err := domain.NewGradeLevel("g1", "t1", "l1", "3o", 3)
	require.NoError(t, err)

	got, err := g.Renamed("  3°  ")

	require.NoError(t, err)
	assert.Equal(t, []any{"g1", "t1", "l1", "3°", 3}, []any{got.ID(), got.TenantID(), got.LevelID(), got.Name(), got.Ordinal()})
	assert.Equal(t, "3o", g.Name(), "the original is untouched")
}

func TestGradeLevelRenamedRefusesABlankName(t *testing.T) {
	g, _ := domain.NewGradeLevel("g1", "t1", "l1", "3°", 3)

	_, err := g.Renamed("   ")

	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
	assert.Equal(t, "El nombre del grado no puede quedar vacío.", glerrors.SafeMessage(err))
}

func TestGroupRenamedKeepsIdentityCycleGradeAndCapacity(t *testing.T) {
	g, err := domain.NewGroup("gr1", "t1", "c1", "g1", "a", 25)
	require.NoError(t, err)

	got, err := g.Renamed(" A ")

	require.NoError(t, err)
	assert.Equal(t, []any{"gr1", "t1", "c1", "g1", "A", 25},
		[]any{got.ID(), got.TenantID(), got.CycleID(), got.GradeLevelID(), got.Name(), got.Capacity()})
}

func TestGroupRenamedRefusesABlankName(t *testing.T) {
	g, _ := domain.NewGroup("gr1", "t1", "c1", "g1", "A", 25)

	_, err := g.Renamed("")

	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
	assert.Equal(t, "El nombre del grupo no puede quedar vacío.", glerrors.SafeMessage(err))
}

func TestGradeLevelUsageRefusal(t *testing.T) {
	cases := map[string]struct {
		usage domain.GradeLevelUsage
		want  string
	}{
		"empty grade deletes": {domain.GradeLevelUsage{}, ""},
		"groups and subjects": {domain.GradeLevelUsage{Groups: 2, Subjects: 8},
			"No se puede borrar el grado 3°: tiene 2 grupos y 8 materias. Bórralos primero."},
		"one period": {domain.GradeLevelUsage{EvaluationPeriods: 1},
			"No se puede borrar el grado 3°: tiene 1 periodo de evaluación. Bórralos primero."},
		"all three": {domain.GradeLevelUsage{Groups: 1, Subjects: 1, EvaluationPeriods: 4},
			"No se puede borrar el grado 3°: tiene 1 grupo, 1 materia y 4 periodos de evaluación. Bórralos primero."},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := tc.usage.Refusal("3°")
			if tc.want == "" {
				assert.NoError(t, err)
				return
			}
			assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
			assert.Equal(t, domain.CodeGradeLevelInUse, domain.RefusalCodeOf(err))
			assert.Equal(t, tc.want, glerrors.SafeMessage(err))
		})
	}
}

func TestGroupUsageRefusal(t *testing.T) {
	assert.NoError(t, domain.GroupUsage{}.Refusal("A"))

	err := domain.GroupUsage{TeachingAssignments: 4, ScheduleBlocks: 6}.Refusal("A")

	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Equal(t, domain.CodeGroupInUse, domain.RefusalCodeOf(err))
	assert.Equal(t, "No se puede borrar el grupo A: tiene 4 clases asignadas y 6 bloques de horario. Quita primero sus clases asignadas.",
		glerrors.SafeMessage(err))
}

func TestCheckLevelOrder(t *testing.T) {
	current := []string{"a", "b", "c"}
	cases := map[string]struct {
		proposed []string
		ok       bool
	}{
		"same set, new order": {[]string{"c", "a", "b"}, true},
		"same order":          {[]string{"a", "b", "c"}, true},
		"one missing":         {[]string{"a", "b"}, false},
		"one extra":           {[]string{"a", "b", "c", "d"}, false},
		"a stranger":          {[]string{"a", "b", "d"}, false},
		"a repeat":            {[]string{"a", "b", "b"}, false},
		"empty":               {nil, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := domain.CheckLevelOrder(current, tc.proposed)
			if tc.ok {
				assert.NoError(t, err)
				return
			}
			assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
			assert.Equal(t, domain.CodeLevelOrderMismatch, domain.RefusalCodeOf(err))
			assert.Equal(t, "La lista de niveles no coincide con los niveles del colegio. Recarga la página e inténtalo de nuevo.",
				glerrors.SafeMessage(err))
		})
	}
}

func TestLevelCarriesItsSortOrder(t *testing.T) {
	l, err := domain.NewLevel("l1", "t1", "Primaria", "", domain.GranularityCycle)
	require.NoError(t, err)

	assert.Equal(t, 0, l.SortOrder())
	assert.Equal(t, 3, l.WithSortOrder(3).SortOrder())
}
