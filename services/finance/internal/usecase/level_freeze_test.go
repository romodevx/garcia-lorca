package usecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/finance/internal/ports"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

// Confirmation review (gap 3): a tuition plan hangs off a level. While the
// level's deletion runs (config: DELETING) a plan write is refused with
// LEVEL_BEING_DELETED — a plan written after finance's archive step would
// otherwise survive the deletion, priced for a level that is gone.
func TestUpsertTuitionPlanRefusesALevelBeingDeleted(t *testing.T) {
	plan := usecase.TuitionPlanInput{
		CycleID: testCycle, LevelID: testLevel, MonthlyCents: 1, Months: 10,
		FirstBilledMonth: 9, DueDay: 10, Active: true,
	}

	f := newFixture(t)
	f.catalog.levels = []ports.LevelRef{{ID: testLevel, Name: "Primaria", Deleting: true}}
	_, err := f.svc.UpsertTuitionPlan(context.Background(), plan)
	assert.Equal(t, levelarchive.CodeLevelBeingDeleted, levelarchive.CodeOf(err), "err = %v", err)
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Empty(t, f.repo.plans, "no plan is stored")

	f.catalog.levels = []ports.LevelRef{{ID: testLevel, Name: "Primaria"}}
	_, err = f.svc.UpsertTuitionPlan(context.Background(), plan)
	require.NoError(t, err, "an active level is priced as before")
}
