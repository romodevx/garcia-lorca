package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/domain"
)

var closeAt = time.Date(2026, 12, 1, 10, 0, 0, 0, time.UTC)

// A period that was never closed is open at version 0; closing moves it to
// closed and bumps the version; closing again is refused until it reopens.
func TestPeriodClosureLifecycle(t *testing.T) {
	p := domain.NewOpenPeriod("term-1", "cyc-1", "lvl-1")
	assert.Equal(t, domain.PeriodOpen, p.Status())
	assert.Equal(t, 0, p.Version())

	closed, err := p.Close(domain.Actor{ID: "u-1", Name: "Directora"}, closeAt)
	require.NoError(t, err)
	assert.Equal(t, domain.PeriodClosed, closed.Status())
	assert.Equal(t, 1, closed.Version())
	assert.Equal(t, closeAt, closed.ClosedAt())
	assert.Equal(t, "u-1", closed.ClosedBy())
	assert.Equal(t, "Directora", closed.ClosedByName())
	assert.Equal(t, domain.PeriodOpen, p.Status(), "Close returns a new value")

	_, err = closed.Close(domain.Actor{ID: "u-1"}, closeAt)
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Equal(t, domain.CodeAlreadyClosed, domain.RefusalCodeOf(err))

	reopenAt := closeAt.Add(time.Hour)
	reopened, err := closed.Reopen(reopenAt)
	require.NoError(t, err)
	assert.Equal(t, domain.PeriodOpen, reopened.Status())
	assert.Equal(t, 1, reopened.Version(), "reopening keeps the version: the report cards stay")
	assert.Equal(t, reopenAt, reopened.ReopenedAt())

	_, err = reopened.Reopen(reopenAt)
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Equal(t, domain.CodeAlreadyOpen, domain.RefusalCodeOf(err))

	again, err := reopened.Close(domain.Actor{ID: "u-2", Name: "Otra"}, reopenAt)
	require.NoError(t, err)
	assert.Equal(t, 2, again.Version(), "the second close issues version 2")
}

func TestRestorePeriodClosureValidates(t *testing.T) {
	_, err := domain.RestorePeriodClosure(domain.PeriodClosureFields{TermID: "t", CycleID: "c", LevelID: "l", Status: "weird"})
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
	_, err = domain.RestorePeriodClosure(domain.PeriodClosureFields{TermID: "t", CycleID: "c", LevelID: "l", Status: domain.PeriodClosed, Version: -1})
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
	got, err := domain.RestorePeriodClosure(domain.PeriodClosureFields{TermID: "t", CycleID: "c", LevelID: "l", Status: domain.PeriodClosed, Version: 3})
	require.NoError(t, err)
	assert.True(t, got.IsClosed())
}

// Conflicts carry a machine code like the invalid refusals do.
func TestConflictRefusal(t *testing.T) {
	err := domain.Conflict(domain.CodeMonthLocked, "El periodo está cerrado")
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Equal(t, domain.CodeMonthLocked, domain.RefusalCodeOf(err))
	assert.Equal(t, "El periodo está cerrado", glerrors.SafeMessage(err))
}
