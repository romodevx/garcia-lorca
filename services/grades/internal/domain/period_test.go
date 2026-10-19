package domain_test

import (
	"strings"
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

	sep, err := domain.ParseMonth("2026-09")
	require.NoError(t, err)
	oct, err := domain.ParseMonth("2026-10")
	require.NoError(t, err)
	covered := []domain.Month{sep, oct}
	closed, err := p.Close(domain.Actor{ID: "u-1", Name: "Directora"}, closeAt, covered)
	require.NoError(t, err)
	covered[0] = oct
	assert.Equal(t, []domain.Month{sep, oct}, closed.Months(), "the close snapshots the term's months (a copy)")
	assert.Equal(t, domain.PeriodClosed, closed.Status())
	assert.Equal(t, 1, closed.Version())
	assert.Equal(t, closeAt, closed.ClosedAt())
	assert.Equal(t, "u-1", closed.ClosedBy())
	assert.Equal(t, "Directora", closed.ClosedByName())
	assert.Equal(t, domain.PeriodOpen, p.Status(), "Close returns a new value")

	_, err = closed.Close(domain.Actor{ID: "u-1"}, closeAt, nil)
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Equal(t, domain.CodeAlreadyClosed, domain.RefusalCodeOf(err))

	reopenAt := closeAt.Add(time.Hour)
	reopener := domain.Actor{ID: "u-3", Name: "Subdirectora"}
	reopened, err := closed.Reopen(reopener, "  Corrección de la nota de Ana  ", reopenAt)
	require.NoError(t, err)
	assert.Equal(t, "u-3", reopened.ReopenedBy(), "CC-308: who reopened it")
	assert.Equal(t, "Subdirectora", reopened.ReopenedByName())
	assert.Equal(t, "Corrección de la nota de Ana", reopened.ReopenReason(), "the reason, trimmed")
	assert.Equal(t, domain.PeriodOpen, reopened.Status())
	assert.Equal(t, 1, reopened.Version(), "reopening keeps the version: the report cards stay")
	assert.Equal(t, []domain.Month{sep, oct}, reopened.Months(), "reopening keeps the last snapshot")
	assert.Equal(t, reopenAt, reopened.ReopenedAt())

	_, err = reopened.Reopen(reopener, "otra vez", reopenAt)
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Equal(t, domain.CodeAlreadyOpen, domain.RefusalCodeOf(err))

	again, err := reopened.Close(domain.Actor{ID: "u-2", Name: "Otra"}, reopenAt, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, again.Version(), "the second close issues version 2")
}

// CC-308: a reopen needs a reason — blank or over 500 characters is refused
// and the period stays closed.
func TestReopenRequiresAReason(t *testing.T) {
	closed, err := domain.NewOpenPeriod("term-1", "cyc-1", "lvl-1").Close(domain.Actor{ID: "u-1", Name: "Directora"}, closeAt, nil)
	require.NoError(t, err)
	for name, reason := range map[string]string{
		"empty":    "",
		"blank":    "  \t ",
		"too long": strings.Repeat("ñ", 501),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := closed.Reopen(domain.Actor{ID: "u-1", Name: "Directora"}, reason, closeAt)
			assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
			assert.True(t, closed.IsClosed())
		})
	}
	ok, err := closed.Reopen(domain.Actor{ID: "u-1", Name: "Directora"}, strings.Repeat("ñ", 500), closeAt)
	require.NoError(t, err)
	assert.False(t, ok.IsClosed(), "500 characters is the limit, inclusive")
}

func TestRestorePeriodClosureValidates(t *testing.T) {
	_, err := domain.RestorePeriodClosure(domain.PeriodClosureFields{TermID: "t", CycleID: "c", LevelID: "l", Status: "weird"})
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
	_, err = domain.RestorePeriodClosure(domain.PeriodClosureFields{TermID: "t", CycleID: "c", LevelID: "l", Status: domain.PeriodClosed, Version: -1})
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
	sep, err := domain.ParseMonth("2026-09")
	require.NoError(t, err)
	got, err := domain.RestorePeriodClosure(domain.PeriodClosureFields{TermID: "t", CycleID: "c", LevelID: "l", Status: domain.PeriodClosed, Version: 3, Months: []domain.Month{sep}})
	require.NoError(t, err)
	assert.True(t, got.IsClosed())
	assert.Equal(t, []domain.Month{sep}, got.Months())
}

// Conflicts carry a machine code like the invalid refusals do.
func TestConflictRefusal(t *testing.T) {
	err := domain.Conflict(domain.CodeMonthLocked, "El periodo está cerrado")
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Equal(t, domain.CodeMonthLocked, domain.RefusalCodeOf(err))
	assert.Equal(t, "El periodo está cerrado", glerrors.SafeMessage(err))
}
