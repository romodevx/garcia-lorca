package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
	"github.com/garcia-lorca/services/grades/internal/usecase"
)

const (
	firstTermID  = "term-t1"
	secondTermID = "term-t2"
)

func months(t *testing.T, raw ...string) []domain.Month {
	t.Helper()
	out := make([]domain.Month, 0, len(raw))
	for _, r := range raw {
		m, err := domain.ParseMonth(r)
		require.NoError(t, err)
		out = append(out, m)
	}
	return out
}

// withTrimesters maps the primary level's months into two terms: Sep–Nov and
// Dec–Jan (the rest of the cycle belongs to no term).
func withTrimesters(t *testing.T, f monthlyFixture) {
	t.Helper()
	f.asg.terms[firstTermID] = ports.TermRef{ID: firstTermID, CycleID: cycleID, LevelID: levelPrimaria, Name: "1.er trimestre", Ordinal: 1,
		Months: months(t, "2026-09", "2026-10", "2026-11")}
	f.asg.terms[secondTermID] = ports.TermRef{ID: secondTermID, CycleID: cycleID, LevelID: levelPrimaria, Name: "2.º trimestre", Ordinal: 2,
		Months: months(t, "2026-12", "2027-01")}
}

func closeTerm(t *testing.T, f monthlyFixture, termID string) {
	t.Helper()
	c, err := domain.NewOpenPeriod(termID, cycleID, levelPrimaria).Close(domain.Actor{ID: "dir"}, sheetNow)
	require.NoError(t, err)
	f.repo.closures[termID] = c
}

func monthStrings(ms []domain.Month) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.String())
	}
	return out
}

// T7: once a level maps months into terms, the sheet lists only those months;
// a level without any mapping keeps every month of the cycle (as before).
func TestSheetListsOnlyMappedMonths(t *testing.T) {
	f := newMonthlyFixture(t)
	sheet, err := f.svc.Sheet(context.Background(), usecase.ScopeTeacher, assignmentID, "2026-09")
	require.NoError(t, err)
	assert.Len(t, sheet.Months, 12, "no mapping yet: the whole cycle")
	assert.Empty(t, sheet.ClosedMonths)

	withTrimesters(t, f)
	sheet, err = f.svc.Sheet(context.Background(), usecase.ScopeTeacher, assignmentID, "2026-09")
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-09", "2026-10", "2026-11", "2026-12", "2027-01"}, monthStrings(sheet.Months))

	_, err = f.svc.Sheet(context.Background(), usecase.ScopeTeacher, assignmentID, "2026-08")
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err), "a month outside every term is not on the sheet")
	_, err = f.svc.Upsert(context.Background(), upsertInput("2026-08", enrollmentID, "9"))
	assert.Equal(t, domain.CodeMonthOutsideCycle, domain.RefusalCodeOf(err), "nor writable")
}

// A closed term's months are read-only for EVERYONE (teacher and director),
// flagged on the sheet with the reason, and refused on write with MONTH_LOCKED.
func TestClosedPeriodLocksItsMonths(t *testing.T) {
	f := newMonthlyFixture(t)
	withTrimesters(t, f)
	_, err := f.svc.Upsert(context.Background(), upsertInput("2026-09", enrollmentID, "9"))
	require.NoError(t, err)
	closeTerm(t, f, firstTermID)

	for _, scope := range []usecase.CaptureScope{usecase.ScopeTeacher, usecase.ScopeSchool} {
		in := upsertInput("2026-10", enrollmentID, "8")
		in.Scope = scope
		_, err := f.svc.Upsert(context.Background(), in)
		assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err), "scope %d", scope)
		assert.Equal(t, domain.CodeMonthLocked, domain.RefusalCodeOf(err))
		assert.Equal(t, "El periodo está cerrado", glerrors.SafeMessage(err))

		err = f.svc.Clear(context.Background(), scope, assignmentID, "2026-09", enrollmentID)
		assert.Equal(t, domain.CodeMonthLocked, domain.RefusalCodeOf(err), "clearing is a write too")
	}
	assert.Len(t, f.repo.rows, 1, "nothing changed in the closed term")

	sheet, err := f.svc.Sheet(context.Background(), usecase.ScopeSchool, assignmentID, "2026-10")
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-09", "2026-10", "2026-11"}, monthStrings(sheet.ClosedMonths))
	assert.False(t, sheet.Editable)
	assert.Equal(t, "El periodo está cerrado", sheet.ReadOnlyReason)
	for _, r := range sheet.Rows {
		assert.False(t, r.Editable)
		assert.Equal(t, "El periodo está cerrado", r.ReadOnlyReason)
	}

	// The next term stays open.
	*f.now = time.Date(2026, 12, 10, 0, 0, 0, 0, time.UTC)
	_, err = f.svc.Upsert(context.Background(), upsertInput("2026-12", enrollmentID, "8"))
	require.NoError(t, err)
}

// Reopening unlocks the months: the director (or the teacher) corrects.
func TestReopenedPeriodUnlocks(t *testing.T) {
	f := newMonthlyFixture(t)
	withTrimesters(t, f)
	closeTerm(t, f, firstTermID)
	reopened, err := f.repo.closures[firstTermID].Reopen(sheetNow)
	require.NoError(t, err)
	f.repo.closures[firstTermID] = reopened

	_, err = f.svc.Upsert(context.Background(), upsertInput("2026-10", enrollmentID, "8"))
	require.NoError(t, err)
	sheet, err := f.svc.Sheet(context.Background(), usecase.ScopeTeacher, assignmentID, "2026-10")
	require.NoError(t, err)
	assert.Empty(t, sheet.ClosedMonths)
	assert.True(t, sheet.Editable)
}

// Every write names the term that covers its month, so the store re-checks
// the closure inside its own transaction; a month no term covers names none.
func TestWritesCarryTheirPeriodLock(t *testing.T) {
	f := newMonthlyFixture(t)
	_, err := f.svc.Upsert(context.Background(), upsertInput("2026-09", enrollmentID, "9"))
	require.NoError(t, err)
	withTrimesters(t, f)
	_, err = f.svc.Upsert(context.Background(), upsertInput("2026-10", enrollmentID, "9"))
	require.NoError(t, err)
	require.NoError(t, f.svc.Clear(context.Background(), usecase.ScopeTeacher, assignmentID, "2026-10", enrollmentID))
	assert.Equal(t, []ports.PeriodLock{{}, {TermID: firstTermID}, {TermID: firstTermID}}, f.repo.locks)
}

// Another level's closed term never locks this level's months.
func TestOtherLevelsClosureDoesNotLock(t *testing.T) {
	f := newMonthlyFixture(t)
	withTrimesters(t, f)
	f.asg.terms["term-kinder"] = ports.TermRef{ID: "term-kinder", CycleID: cycleID, LevelID: levelPreescolar, Ordinal: 1, Months: months(t, "2026-09", "2026-10")}
	closeTerm(t, f, "term-kinder")
	_, err := f.svc.Upsert(context.Background(), upsertInput("2026-10", enrollmentID, "9"))
	require.NoError(t, err)
}
