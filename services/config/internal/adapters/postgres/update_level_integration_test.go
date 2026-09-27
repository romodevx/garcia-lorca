//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/config/internal/adapters/postgres"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
)

// UpdateLevel writes mode and scale together, leaves an absent field as it was,
// and cannot reach another tenant's level (RLS).
func TestUpdateLevelWritesPresentFieldsTogether(t *testing.T) {
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantA)
	l, err := domain.NewLevel("eeee0010-0000-0000-0000-000000000001", tenantA, "Primaria UL", "", domain.GranularityCycle)
	require.NoError(t, err)
	require.NoError(t, repo.CreateLevel(ctx, l))

	daily := domain.AttendanceDaily
	scale, err := domain.NewGradingScale(true, 0, 10000, 7000, 2)
	require.NoError(t, err)
	require.NoError(t, repo.UpdateLevel(ctx, l.ID(), ports.LevelPatch{AttendanceMode: &daily, Grading: &scale}))
	got, err := repo.GetLevel(ctx, l.ID())
	require.NoError(t, err)
	assert.Equal(t, daily, got.AttendanceMode())
	assert.Equal(t, scale, got.Grading())

	perClass := domain.AttendancePerClass
	require.NoError(t, repo.UpdateLevel(ctx, l.ID(), ports.LevelPatch{AttendanceMode: &perClass}))
	got, err = repo.GetLevel(ctx, l.ID())
	require.NoError(t, err)
	assert.Equal(t, perClass, got.AttendanceMode())
	assert.Equal(t, scale, got.Grading(), "an absent scale must keep its value")

	other := pgadapter.NewRepository(appDB(t), tenantB)
	err = other.UpdateLevel(ctx, l.ID(), ports.LevelPatch{AttendanceMode: &daily})
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err), "RLS: a foreign tenant cannot change the level")
	err = repo.UpdateLevel(ctx, "eeee0010-0000-0000-0000-00000000ffff", ports.LevelPatch{AttendanceMode: &daily})
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}
