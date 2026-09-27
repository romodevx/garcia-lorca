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

const msgPreviousGroup = "Ese mes corresponde al grupo anterior del alumno"

// transferredFixture moves "enr-2" into the test group on 2026-10-20: months
// before October belong to the previous group.
func transferredFixture(t *testing.T) monthlyFixture {
	t.Helper()
	f := newMonthlyFixture(t)
	f.enr.transfers = []ports.TransferRef{
		{EnrollmentID: "enr-2", ToGroupID: "grp-older", MovedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		{EnrollmentID: "enr-2", ToGroupID: groupID, MovedAt: time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)},
		{EnrollmentID: "enr-2", ToGroupID: groupID, MovedAt: time.Date(2026, 10, 20, 17, 0, 0, 0, time.UTC)},
	}
	return f
}

func teacherInput(month, enrollment, value string) usecase.UpsertMonthlyInput {
	in := upsertInput(month, enrollment, value)
	in.Scope = usecase.ScopeTeacher
	return in
}

// After a transfer the new group's teacher writes only from the transfer month
// on (the LATEST move into the group counts); the director writes any month.
func TestMonthsBeforeATransferBelongToThePreviousGroup(t *testing.T) {
	tests := []struct {
		name     string
		month    string
		scope    usecase.CaptureScope
		enroll   string
		wantCode domain.RefusalCode
	}{
		{name: "teacher, month before the move", month: "2026-09", scope: usecase.ScopeTeacher, enroll: "enr-2", wantCode: domain.CodeMonthPreviousGroup},
		{name: "teacher, month of the move", month: "2026-10", scope: usecase.ScopeTeacher, enroll: "enr-2"},
		{name: "teacher, month after the move", month: "2026-11", scope: usecase.ScopeTeacher, enroll: "enr-2"},
		{name: "teacher, never-moved pupil", month: "2026-09", scope: usecase.ScopeTeacher, enroll: enrollmentID},
		{name: "director, month before the move", month: "2026-09", scope: usecase.ScopeSchool, enroll: "enr-2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := transferredFixture(t)
			*f.now = time.Date(2026, 11, 30, 23, 0, 0, 0, time.UTC)
			in := upsertInput(tt.month, tt.enroll, "9")
			in.Scope = tt.scope
			_, err := f.svc.Upsert(context.Background(), in)
			clearErr := f.svc.Clear(context.Background(), tt.scope, assignmentID, tt.month, tt.enroll)
			if tt.wantCode == "" {
				require.NoError(t, err)
				require.NoError(t, clearErr)
				return
			}
			for _, e := range []error{err, clearErr} {
				assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(e))
				assert.Equal(t, msgPreviousGroup, glerrors.SafeMessage(e))
				assert.Equal(t, tt.wantCode, domain.RefusalCodeOf(e))
			}
			assert.Empty(t, f.repo.rows)
			assert.Empty(t, f.repo.deleted)
		})
	}
}

// The sheet marks each row: a transferred pupil's earlier months are read-only
// for the teacher (with the reason) while the rest of the sheet stays editable;
// the director sees every row editable.
func TestMonthlySheetMarksPreviousGroupRowsReadOnly(t *testing.T) {
	f := transferredFixture(t)
	sheet, err := f.svc.Sheet(context.Background(), usecase.ScopeTeacher, assignmentID, "2026-09")
	require.NoError(t, err)
	assert.True(t, sheet.Editable)
	rows := map[string]usecase.MonthlySheetRow{}
	for _, r := range sheet.Rows {
		rows[r.EnrollmentID] = r
	}
	assert.False(t, rows["enr-2"].Editable)
	assert.Equal(t, msgPreviousGroup, rows["enr-2"].ReadOnlyReason)
	assert.True(t, rows[enrollmentID].Editable)
	assert.Empty(t, rows[enrollmentID].ReadOnlyReason)

	october, err := f.svc.Sheet(context.Background(), usecase.ScopeTeacher, assignmentID, "2026-10")
	require.NoError(t, err)
	for _, r := range october.Rows {
		assert.True(t, r.Editable, "the transfer month is the new group's")
	}

	director, err := f.svc.Sheet(context.Background(), usecase.ScopeSchool, assignmentID, "2026-09")
	require.NoError(t, err)
	for _, r := range director.Rows {
		assert.True(t, r.Editable, "the director edits any month")
	}

	kinder, err := f.svc.Sheet(context.Background(), usecase.ScopeTeacher, kinderAsgID, "2026-09")
	require.NoError(t, err)
	for _, r := range kinder.Rows {
		assert.False(t, r.Editable, "a read-only sheet has no editable row")
		assert.Equal(t, "Este nivel no lleva calificación numérica", r.ReadOnlyReason)
	}
}

// Every knot refusal names its machine code.
func TestMonthlyKnotRefusalsCarryCodes(t *testing.T) {
	tests := []struct {
		name string
		in   usecase.UpsertMonthlyInput
		want domain.RefusalCode
	}{
		{"level not graded", usecase.UpsertMonthlyInput{AssignmentID: kinderAsgID, Month: "2026-09", EnrollmentID: "enr-kinder", Value: "9", CapturedBy: capturer}, domain.CodeLevelNotGraded},
		{"enrollment of another group", upsertInput("2026-09", "enr-other", "9"), domain.CodeEnrollmentNotInGroup},
		{"month outside the cycle", upsertInput("2027-08", enrollmentID, "9"), domain.CodeMonthOutsideCycle},
		{"out of range", upsertInput("2026-09", enrollmentID, "11"), domain.CodeGradeOutOfRange},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newMonthlyFixture(t)
			_, err := f.svc.Upsert(context.Background(), tt.in)
			assert.Equal(t, tt.want, domain.RefusalCodeOf(err), "err = %v", err)
		})
	}
}

// A level that stopped grading numerically still shows the grades already
// captured (never hidden); with nothing captured it lists no subjects.
func TestEnrollmentMonthlyGradesKeepsGradesAfterGradingIsDisabled(t *testing.T) {
	f := newMonthlyFixture(t)
	_, err := f.svc.Upsert(context.Background(), upsertInput("2026-09", enrollmentID, "9.5"))
	require.NoError(t, err)
	prim := f.catalog.levelByGroup[groupID]
	prim.GradingEnabled = false
	f.catalog.levelByGroup[groupID] = prim

	got, err := f.svc.ForEnrollment(context.Background(), enrollmentID)
	require.NoError(t, err)
	assert.False(t, got.GradingEnabled)
	require.Len(t, got.Grades, 1, "captured grades are never hidden")
	assert.Equal(t, "9.5", got.Grades[0].Value().String())
	assert.Equal(t, []string{subjectID}, got.SubjectIDs, "only the subjects that carry a grade")
}

// CountScaleViolations counts the grades of the level's active cycle that a
// proposed scale would refuse; loosening never counts, disabling counts all.
func TestCountScaleViolations(t *testing.T) {
	f := newMonthlyFixture(t)
	for _, c := range []struct{ enr, month, value string }{
		{enrollmentID, "2026-09", "9.5"}, {enrollmentID, "2026-10", "6"}, {"enr-2", "2026-09", "10"},
	} {
		_, err := f.svc.Upsert(context.Background(), upsertInput(c.month, c.enr, c.value))
		require.NoError(t, err)
	}
	active := f.catalog.cycles[cycleID]
	f.catalog.activeCycle = &active
	f.catalog.groupsByLevel = map[string][]string{levelPrimaria: {groupID}}

	tests := []struct {
		name    string
		enabled bool
		scale   domain.GradingScale
		want    int
	}{
		{"loosen: 0–10, 2 decimals", true, mustScale(t, 0, 1000, 600, 2), 0},
		{"tighten: max 9.5", true, mustScale(t, 500, 950, 600, 1), 1},
		{"tighten: min 7", true, mustScale(t, 700, 1000, 800, 1), 1},
		{"tighten: whole numbers", true, mustScale(t, 500, 1000, 600, 0), 1},
		{"disable", false, domain.GradingScale{}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := f.svc.CountScaleViolations(context.Background(), levelPrimaria, ports.ScaleCheck{GradingEnabled: tt.enabled, Scale: tt.scale})
			require.NoError(t, err)
			assert.Equal(t, tt.want, n)
		})
	}

	n, err := f.svc.CountScaleViolations(context.Background(), levelPreescolar, ports.ScaleCheck{})
	require.NoError(t, err)
	assert.Zero(t, n, "a level without groups in the cycle has nothing to violate")

	f.catalog.activeCycle = nil
	n, err = f.svc.CountScaleViolations(context.Background(), levelPrimaria, ports.ScaleCheck{})
	require.NoError(t, err)
	assert.Zero(t, n, "no active cycle, nothing to violate")
}

// A level whose scale config cannot be read refuses writes with a sentence the
// sheet can show.
func TestMonthlyWritesSurfaceAnUnconfiguredScale(t *testing.T) {
	f := newMonthlyFixture(t)
	f.catalog.scaleErr = domain.Refusal(domain.CodeLevelScaleUnavailable, "La escala de calificación del nivel no está configurada")
	_, err := f.svc.Upsert(context.Background(), upsertInput("2026-09", enrollmentID, "9"))
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
	assert.Equal(t, "La escala de calificación del nivel no está configurada", glerrors.SafeMessage(err))
}

const msgMonthInFuture = "Aún no llega ese mes"

// A month after the current UTC month cannot be graded yet, by the teacher or
// the director: writes and clears refuse with MONTH_IN_FUTURE and the sheet
// still answers (so the screen can show it) but read-only, for that reason.
func TestFutureMonthsAreReadOnly(t *testing.T) {
	tests := []struct {
		name     string
		month    string
		scope    usecase.CaptureScope
		wantCode domain.RefusalCode
	}{
		{name: "teacher, current month", month: "2026-10", scope: usecase.ScopeTeacher},
		{name: "teacher, past month", month: "2026-09", scope: usecase.ScopeTeacher},
		{name: "teacher, next month", month: "2026-11", scope: usecase.ScopeTeacher, wantCode: domain.CodeMonthInFuture},
		{name: "director, next month", month: "2026-11", scope: usecase.ScopeSchool, wantCode: domain.CodeMonthInFuture},
		{name: "director, last month of the cycle", month: "2027-07", scope: usecase.ScopeSchool, wantCode: domain.CodeMonthInFuture},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newMonthlyFixture(t) // now = 2026-10-15
			in := upsertInput(tt.month, enrollmentID, "9")
			in.Scope = tt.scope
			_, err := f.svc.Upsert(context.Background(), in)
			clearErr := f.svc.Clear(context.Background(), tt.scope, assignmentID, tt.month, enrollmentID)
			sheet, sheetErr := f.svc.Sheet(context.Background(), tt.scope, assignmentID, tt.month)
			require.NoError(t, sheetErr)
			require.Len(t, sheet.Months, 12, "the sheet still lists the whole cycle")
			if tt.wantCode == "" {
				require.NoError(t, err)
				require.NoError(t, clearErr)
				assert.True(t, sheet.Editable)
				return
			}
			for _, e := range []error{err, clearErr} {
				assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(e))
				assert.Equal(t, msgMonthInFuture, glerrors.SafeMessage(e))
				assert.Equal(t, tt.wantCode, domain.RefusalCodeOf(e))
			}
			assert.Empty(t, f.repo.rows)
			assert.Empty(t, f.repo.deleted)
			assert.False(t, sheet.Editable)
			assert.Equal(t, msgMonthInFuture, sheet.ReadOnlyReason)
			for _, r := range sheet.Rows {
				assert.False(t, r.Editable)
				assert.Equal(t, msgMonthInFuture, r.ReadOnlyReason)
			}
		})
	}
}

// "Now" is the UTC month: the last instant of October still refuses November,
// and the first instant of November accepts it.
func TestFutureMonthBoundaryIsUTC(t *testing.T) {
	f := newMonthlyFixture(t)
	*f.now = time.Date(2026, 10, 31, 23, 59, 59, 0, time.UTC)
	_, err := f.svc.Upsert(context.Background(), upsertInput("2026-11", enrollmentID, "9"))
	assert.Equal(t, domain.CodeMonthInFuture, domain.RefusalCodeOf(err))

	*f.now = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	_, err = f.svc.Upsert(context.Background(), upsertInput("2026-11", enrollmentID, "9"))
	require.NoError(t, err)
}
