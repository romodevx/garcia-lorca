package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func mustCycle(t *testing.T, id, name string, from, to time.Time) domain.Cycle {
	t.Helper()
	c, err := domain.NewCycle(id, "t1", name, domain.CycleStatusDraft, from, to)
	require.NoError(t, err)
	return c
}

func TestRefuseCycleOverlap(t *testing.T) {
	current := mustCycle(t, "c-26", "2026-2027", day(2026, time.August, 24), day(2027, time.July, 9))

	cases := map[string]struct {
		from, to time.Time
		refused  bool
	}{
		"the QA case: starts inside the live year": {day(2027, time.January, 10), day(2027, time.December, 20), true},
		"contains the live year":                   {day(2026, time.August, 1), day(2027, time.August, 1), true},
		"inside the live year":                     {day(2026, time.October, 1), day(2026, time.November, 1), true},
		"ends on the live year's first day":        {day(2025, time.August, 1), day(2026, time.August, 24), true},
		"starts on the live year's last day":       {day(2027, time.July, 9), day(2028, time.July, 1), true},
		"the day after the live year ends":         {day(2027, time.July, 10), day(2028, time.July, 7), false},
		"ends the day before the live year starts": {day(2025, time.August, 25), day(2026, time.August, 23), false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			candidate := mustCycle(t, "c-new", "nuevo", tc.from, tc.to)
			err := domain.RefuseCycleOverlap(candidate, []domain.Cycle{current})
			if !tc.refused {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
			assert.Equal(t, domain.CodeCycleOverlap, domain.RefusalCodeOf(err))
			assert.Equal(t,
				`Las fechas se traslapan con el ciclo "2026-2027" (del 24 de agosto de 2026 al 9 de julio de 2027)`,
				glerrors.SafeMessage(err))
		})
	}
}

func TestRefuseCycleOverlapIgnoresTheCycleItself(t *testing.T) {
	c := mustCycle(t, "c-26", "2026-2027", day(2026, time.August, 24), day(2027, time.July, 9))
	assert.NoError(t, domain.RefuseCycleOverlap(c, []domain.Cycle{c}))
}

func TestRefuseCycleOverlapNamesTheEarliestClash(t *testing.T) {
	later := mustCycle(t, "c-27", "2027-2028", day(2027, time.August, 23), day(2028, time.July, 7))
	earlier := mustCycle(t, "c-26", "2026-2027", day(2026, time.August, 24), day(2027, time.July, 9))
	wide := mustCycle(t, "c-new", "largo", day(2026, time.September, 1), day(2028, time.June, 1))

	err := domain.RefuseCycleOverlap(wide, []domain.Cycle{later, earlier})

	assert.Contains(t, glerrors.SafeMessage(err), `"2026-2027"`)
}

func TestCycleRename(t *testing.T) {
	c := mustCycle(t, "c-26", "2026-2027", day(2026, time.August, 24), day(2027, time.July, 9))

	renamed, err := c.Rename("  Ciclo 2026-2027  ")
	require.NoError(t, err)
	assert.Equal(t, "Ciclo 2026-2027", renamed.Name())
	assert.Equal(t, c.ID(), renamed.ID())
	assert.Equal(t, c.StartsOn(), renamed.StartsOn())

	_, err = c.Rename("   ")
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
	assert.Equal(t, domain.CodeCycleNameRequired, domain.RefusalCodeOf(err))
}

func mustClosure(t *testing.T, id, label string, from, to time.Time) domain.NonWorkingPeriod {
	t.Helper()
	p, err := domain.NewNonWorkingPeriod(id, "t1", "c-26", label, domain.KindHoliday, from, to)
	require.NoError(t, err)
	return p
}

func TestRefuseNonWorkingOverlap(t *testing.T) {
	revolution := mustClosure(t, "p-1", "Revolución", day(2026, time.November, 16), day(2026, time.November, 16))
	winter := mustClosure(t, "p-2", "Vacaciones de invierno", day(2026, time.December, 21), day(2027, time.January, 6))
	existing := []domain.NonWorkingPeriod{winter, revolution}

	cases := map[string]struct {
		from, to time.Time
		clash    string
	}{
		"the QA case: the same holiday twice": {day(2026, time.November, 16), day(2026, time.November, 16), `"Revolución" (16 de noviembre de 2026)`},
		"a range covering the holiday":        {day(2026, time.November, 13), day(2026, time.November, 17), `"Revolución" (16 de noviembre de 2026)`},
		"inside the vacation":                 {day(2027, time.January, 6), day(2027, time.January, 6), `"Vacaciones de invierno" (del 21 de diciembre de 2026 al 6 de enero de 2027)`},
		"the day after the holiday":           {day(2026, time.November, 17), day(2026, time.November, 17), ""},
		"between the two":                     {day(2026, time.November, 17), day(2026, time.December, 20), ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			candidate := mustClosure(t, "p-new", "nuevo", tc.from, tc.to)
			err := domain.RefuseNonWorkingOverlap(candidate, existing)
			if tc.clash == "" {
				assert.NoError(t, err)
				return
			}
			assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
			assert.Equal(t, domain.CodeNonWorkingOverlap, domain.RefusalCodeOf(err))
			assert.Equal(t, "Ese día ya está marcado sin clases: "+tc.clash, glerrors.SafeMessage(err))
		})
	}
}
