//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	pgadapter "github.com/garcia-lorca/services/config/internal/adapters/postgres"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
)

func scAbsence(t *testing.T, id, teacher string, from, to time.Time, span *domain.ClassTime) domain.TeacherAbsence {
	t.Helper()
	a, err := domain.NewTeacherAbsence(domain.AbsenceFields{
		ID: id, TenantID: tenantG, CycleID: scCycle, TeacherID: teacher, Kind: domain.AbsenceSick,
		StartsOn: from, EndsOn: to, Span: span, Note: "gripe",
	}, domain.Actor{UserID: "u-1", Name: "Directora"}, time.Now().UTC().Truncate(time.Microsecond))
	require.NoError(t, err)
	return a
}

// T19: absences round-trip (full day and partial), are cancelled in place, are
// read by dates, carry their id in the history, and stay inside their school.
func TestTeacherAbsencesRoundTripCancelAndRLS(t *testing.T) {
	seedSchedule(t)
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantG)
	mon := time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC)
	ct, err := domain.NewClassTime(480, 600)
	require.NoError(t, err)

	full := scAbsence(t, newID(30), scAna, mon, mon.AddDate(0, 0, 2), nil)
	part := scAbsence(t, newID(31), scLuis, mon, mon, &ct)
	require.NoError(t, repo.RunScheduleTx(ctx, scCycle, func(tx ports.ScheduleTx) error {
		if err := tx.InsertAbsence(full); err != nil {
			return err
		}
		if err := tx.InsertAbsence(part); err != nil {
			return err
		}
		return tx.AppendEvent(domain.ScheduleEvent{ID: newID(40), TenantID: tenantG, CycleID: scCycle, At: time.Now().UTC(),
			By: full.CreatedBy(), Action: domain.ActionAbsenceCreated, AbsenceID: full.ID(), Date: mon, Summary: "Ausencia registrada"})
	}))

	got, err := repo.GetTeacherAbsence(ctx, full.ID())
	require.NoError(t, err)
	assert.True(t, got.FullDay())
	assert.Equal(t, mon.AddDate(0, 0, 2), got.EndsOn())
	assert.Equal(t, "gripe", got.Note())
	assert.Equal(t, full.CreatedBy(), got.CreatedBy())
	gotPart, err := repo.GetTeacherAbsence(ctx, part.ID())
	require.NoError(t, err)
	assert.Equal(t, ct, gotPart.Span())

	evs, err := repo.ListScheduleEvents(ctx, scCycle, 10)
	require.NoError(t, err)
	require.Len(t, evs, 1)
	assert.Equal(t, full.ID(), evs[0].AbsenceID)
	assert.Equal(t, domain.ActionAbsenceCreated, evs[0].Action)

	// Wednesday only touches Ana's span.
	wed, err := repo.ListTeacherAbsences(ctx, scCycle, mon.AddDate(0, 0, 2), mon.AddDate(0, 0, 2))
	require.NoError(t, err)
	require.Len(t, wed, 1)
	assert.Equal(t, full.ID(), wed[0].ID())

	cancelled, err := full.Cancel(domain.Actor{UserID: "u-2", Name: "Subdirector"}, time.Now().UTC().Truncate(time.Microsecond))
	require.NoError(t, err)
	var active []domain.TeacherAbsence
	require.NoError(t, repo.RunScheduleTx(ctx, scCycle, func(tx ports.ScheduleTx) error {
		if err := tx.UpdateAbsence(cancelled); err != nil {
			return err
		}
		var err error
		active, err = tx.ActiveAbsences(scCycle, mon, mon)
		return err
	}))
	require.Len(t, active, 1, "the cancelled one is not active")
	assert.Equal(t, part.ID(), active[0].ID())
	stored, err := repo.GetTeacherAbsence(ctx, full.ID())
	require.NoError(t, err)
	assert.False(t, stored.Active())
	assert.Equal(t, "Subdirector", stored.CancelledBy().Name)

	// Tenant H sees none of it, and cannot write into tenant G.
	other := pgadapter.NewRepository(appDB(t), tenantH)
	hidden, err := other.ListTeacherAbsences(ctx, scCycle, mon, mon)
	require.NoError(t, err)
	assert.Empty(t, hidden)
	_, err = other.GetTeacherAbsence(ctx, full.ID())
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
	err = other.RunScheduleTx(ctx, scCycle, func(tx ports.ScheduleTx) error {
		return tx.InsertAbsence(scAbsence(t, newID(32), scAna, mon, mon, nil))
	})
	require.Error(t, err)
}
