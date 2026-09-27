package usecase_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
	"github.com/garcia-lorca/services/grades/internal/usecase"
)

// --- fakes -------------------------------------------------------------------

const (
	levelPrimaria   = "lvl-prim"
	levelPreescolar = "lvl-pre"
	kinderGroupID   = "grp-kinder"
	kinderAsgID     = "asg-kinder"
	capturer        = "user-1"
)

// fakeCatalog resolves groups to levels, cycles and a group's assignments.
type fakeCatalog struct {
	levelByGroup map[string]ports.LevelRef
	cycles       map[string]ports.CycleRef
	assignments  []ports.AssignmentRef
}

func (f *fakeCatalog) LevelOfGroup(_ context.Context, groupID string) (ports.LevelRef, error) {
	l, ok := f.levelByGroup[groupID]
	if !ok {
		return ports.LevelRef{}, glerrors.ErrNotFound
	}
	return l, nil
}

func (f *fakeCatalog) CycleByID(_ context.Context, id string) (ports.CycleRef, error) {
	c, ok := f.cycles[id]
	if !ok {
		return ports.CycleRef{}, glerrors.ErrNotFound
	}
	return c, nil
}

func (f *fakeCatalog) AssignmentsByGroup(_ context.Context, cycle, group string) ([]ports.AssignmentRef, error) {
	var out []ports.AssignmentRef
	for _, a := range f.assignments {
		if a.CycleID == cycle && a.GroupID == group {
			out = append(out, a)
		}
	}
	return out, nil
}

func mustScale(t *testing.T, minH, maxH, passH, dec int) domain.GradingScale {
	t.Helper()
	s, err := domain.NewGradingScale(minH, maxH, passH, dec)
	require.NoError(t, err)
	return s
}

func baseCatalog(t *testing.T) *fakeCatalog {
	scale := mustScale(t, 500, 1000, 600, 1)
	return &fakeCatalog{
		levelByGroup: map[string]ports.LevelRef{
			groupID:       {ID: levelPrimaria, GradingEnabled: true, Scale: scale},
			kinderGroupID: {ID: levelPreescolar, GradingEnabled: false, Scale: scale},
		},
		cycles: map[string]ports.CycleRef{
			cycleID: {ID: cycleID, Name: "2026-2027",
				StartsOn: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC),
				EndsOn:   time.Date(2027, 7, 9, 0, 0, 0, 0, time.UTC)},
		},
		assignments: []ports.AssignmentRef{
			{ID: assignmentID, GroupID: groupID, SubjectID: subjectID, TeacherID: teacherID, CycleID: cycleID},
			{ID: "asg-2", GroupID: groupID, SubjectID: "subj-2", TeacherID: "teacher-2", CycleID: cycleID},
			{ID: "asg-3", GroupID: groupID, SubjectID: subjectID, TeacherID: "teacher-3", CycleID: cycleID},
		},
	}
}

// fakeMonthly is an in-memory monthly-grade store keyed by (enrollment,
// subject, month); it counts sheet reads to prove the sheet is one batched read.
type fakeMonthly struct {
	rows       map[string]domain.MonthlyGrade
	sheetCalls int
	deleted    []string
}

func newFakeMonthly() *fakeMonthly { return &fakeMonthly{rows: map[string]domain.MonthlyGrade{}} }

func monthlyKey(enrollment, subject string, m domain.Month) string {
	return enrollment + "|" + subject + "|" + m.String()
}

func (f *fakeMonthly) UpsertMonthlyGrade(_ context.Context, g domain.MonthlyGrade) (domain.MonthlyGrade, error) {
	f.rows[monthlyKey(g.EnrollmentID(), g.SubjectID(), g.Month())] = g
	return g, nil
}

func (f *fakeMonthly) DeleteMonthlyGrade(_ context.Context, enrollment, subject string, m domain.Month) error {
	k := monthlyKey(enrollment, subject, m)
	delete(f.rows, k)
	f.deleted = append(f.deleted, k)
	return nil
}

func (f *fakeMonthly) MonthlyGradesForSheet(_ context.Context, ids []string, subject string, m domain.Month) (map[string]domain.MonthlyGrade, error) {
	f.sheetCalls++
	out := map[string]domain.MonthlyGrade{}
	for _, id := range ids {
		if g, ok := f.rows[monthlyKey(id, subject, m)]; ok {
			out[id] = g
		}
	}
	return out, nil
}

func (f *fakeMonthly) MonthlyGradesForEnrollment(_ context.Context, enrollment string) ([]domain.MonthlyGrade, error) {
	var out []domain.MonthlyGrade
	for _, g := range f.rows {
		if g.EnrollmentID() == enrollment {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Month().String() < out[j].Month().String() })
	return out, nil
}

// --- fixtures ----------------------------------------------------------------

var sheetNow = time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)

type monthlyFixture struct {
	svc     *usecase.MonthlyGradesService
	repo    *fakeMonthly
	catalog *fakeCatalog
	asg     *fakeAssignments
	enr     *fakeEnrollments
}

func newMonthlyFixture(t *testing.T) monthlyFixture {
	t.Helper()
	asg := baseAssignments()
	asg.assignments[kinderAsgID] = ports.AssignmentRef{ID: kinderAsgID, GroupID: kinderGroupID, SubjectID: subjectID, TeacherID: teacherID, CycleID: cycleID}
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{
		enrollmentID: activeEnrollment(enrollmentID, "stu-1"),
		"enr-2":      activeEnrollment("enr-2", "stu-2"),
		"enr-gone":   {ID: "enr-gone", StudentID: "stu-3", GroupID: groupID, CycleID: cycleID, Status: "withdrawn"},
		"enr-other":  {ID: "enr-other", StudentID: "stu-4", GroupID: "grp-other", CycleID: cycleID, Status: "active"},
		"enr-kinder": {ID: "enr-kinder", StudentID: "stu-5", GroupID: kinderGroupID, CycleID: cycleID, Status: "active"},
	}}
	repo := newFakeMonthly()
	catalog := baseCatalog(t)
	seq := 0
	newID := func() string { seq++; return "mg-" + string(rune('0'+seq)) }
	svc := usecase.NewMonthlyGradesService(repo, asg, catalog, enr, testTenant, newID, func() time.Time { return sheetNow })
	return monthlyFixture{svc: svc, repo: repo, catalog: catalog, asg: asg, enr: enr}
}

func upsertInput(month, enrollment, value string) usecase.UpsertMonthlyInput {
	return usecase.UpsertMonthlyInput{AssignmentID: assignmentID, Month: month, EnrollmentID: enrollment, Value: value, CapturedBy: capturer}
}

// --- UpsertMonthlyGrade ------------------------------------------------------

func TestUpsertMonthlyGradeKnot(t *testing.T) {
	tests := []struct {
		name     string
		in       usecase.UpsertMonthlyInput
		wantKind glerrors.Kind
		wantMsg  string
	}{
		{name: "unknown assignment is not found", in: usecase.UpsertMonthlyInput{AssignmentID: "nope", Month: "2026-09", EnrollmentID: enrollmentID, Value: "9", CapturedBy: capturer}, wantKind: glerrors.KindNotFound},
		{name: "preescolar takes no numeric grade", in: usecase.UpsertMonthlyInput{AssignmentID: kinderAsgID, Month: "2026-09", EnrollmentID: "enr-kinder", Value: "9", CapturedBy: capturer}, wantKind: glerrors.KindInvalid, wantMsg: "Este nivel no lleva calificación numérica"},
		{name: "enrollment of another group", in: upsertInput("2026-09", "enr-other", "9"), wantKind: glerrors.KindInvalid, wantMsg: "El alumno no está inscrito en este grupo"},
		{name: "withdrawn enrollment", in: upsertInput("2026-09", "enr-gone", "9"), wantKind: glerrors.KindInvalid, wantMsg: "El alumno no está inscrito en este grupo"},
		{name: "unknown enrollment", in: upsertInput("2026-09", "enr-ghost", "9"), wantKind: glerrors.KindInvalid, wantMsg: "El alumno no está inscrito en este grupo"},
		{name: "month before the cycle", in: upsertInput("2026-07", enrollmentID, "9"), wantKind: glerrors.KindInvalid, wantMsg: "Ese mes no pertenece al ciclo"},
		{name: "month after the cycle", in: upsertInput("2027-08", enrollmentID, "9"), wantKind: glerrors.KindInvalid, wantMsg: "Ese mes no pertenece al ciclo"},
		{name: "malformed month", in: upsertInput("sept", enrollmentID, "9"), wantKind: glerrors.KindInvalid, wantMsg: "El mes debe tener el formato AAAA-MM"},
		{name: "two decimals on a one-decimal scale", in: upsertInput("2026-09", enrollmentID, "9.95"), wantKind: glerrors.KindInvalid, wantMsg: "Solo se permite 1 decimal"},
		{name: "above the scale", in: upsertInput("2026-09", enrollmentID, "11"), wantKind: glerrors.KindInvalid, wantMsg: "La calificación debe estar entre 5 y 10"},
		{name: "below the scale", in: upsertInput("2026-09", enrollmentID, "4"), wantKind: glerrors.KindInvalid, wantMsg: "La calificación debe estar entre 5 y 10"},
		{name: "missing captured by", in: usecase.UpsertMonthlyInput{AssignmentID: assignmentID, Month: "2026-09", EnrollmentID: enrollmentID, Value: "9"}, wantKind: glerrors.KindInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newMonthlyFixture(t)
			_, err := f.svc.Upsert(context.Background(), tt.in)
			require.Error(t, err)
			assert.Equal(t, tt.wantKind, glerrors.KindOf(err), "err = %v", err)
			if tt.wantMsg != "" {
				assert.Equal(t, tt.wantMsg, glerrors.SafeMessage(err))
			}
			assert.Empty(t, f.repo.rows, "a refused grade must not be stored")
		})
	}
}

func TestUpsertMonthlyGradeDerivesSubjectAndTeacher(t *testing.T) {
	f := newMonthlyFixture(t)
	g, err := f.svc.Upsert(context.Background(), upsertInput("2027-07", enrollmentID, "10.0"))
	require.NoError(t, err)
	assert.Equal(t, subjectID, g.SubjectID())
	assert.Equal(t, teacherID, g.TeacherID())
	assert.Equal(t, capturer, g.CapturedBy())
	assert.Equal(t, 1000, g.Value().Hundredths())
	assert.Equal(t, "2027-07", g.Month().String(), "the cycle's last month is capturable")

	// A correction of the same cell replaces the value (autosave).
	_, err = f.svc.Upsert(context.Background(), upsertInput("2027-07", enrollmentID, "8.5"))
	require.NoError(t, err)
	stored := f.repo.rows[monthlyKey(enrollmentID, subjectID, g.Month())]
	assert.Equal(t, 850, stored.Value().Hundredths())
	assert.Len(t, f.repo.rows, 1)
}

// --- ClearMonthlyGrade -------------------------------------------------------

func TestClearMonthlyGrade(t *testing.T) {
	f := newMonthlyFixture(t)
	_, err := f.svc.Upsert(context.Background(), upsertInput("2026-09", enrollmentID, "9"))
	require.NoError(t, err)

	require.NoError(t, f.svc.Clear(context.Background(), assignmentID, "2026-09", enrollmentID))
	assert.Empty(t, f.repo.rows)
	require.NoError(t, f.svc.Clear(context.Background(), assignmentID, "2026-09", enrollmentID), "clearing an empty cell succeeds")

	tests := []struct {
		name, asg, month, enrollment string
		wantKind                     glerrors.Kind
	}{
		{"unknown assignment", "nope", "2026-09", enrollmentID, glerrors.KindNotFound},
		{"month outside the cycle", assignmentID, "2027-09", enrollmentID, glerrors.KindInvalid},
		{"enrollment of another group", assignmentID, "2026-09", "enr-other", glerrors.KindInvalid},
		{"preescolar", kinderAsgID, "2026-09", "enr-kinder", glerrors.KindInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newMonthlyFixture(t)
			err := f.svc.Clear(context.Background(), tt.asg, tt.month, tt.enrollment)
			assert.Equal(t, tt.wantKind, glerrors.KindOf(err), "err = %v", err)
			assert.Empty(t, f.repo.deleted, "a refused clear must not reach the store")
		})
	}
}

// --- GetMonthlySheet ---------------------------------------------------------

func TestMonthlySheet(t *testing.T) {
	f := newMonthlyFixture(t)
	_, err := f.svc.Upsert(context.Background(), upsertInput("2026-10", "enr-2", "7.5"))
	require.NoError(t, err)

	sheet, err := f.svc.Sheet(context.Background(), assignmentID, "")
	require.NoError(t, err)
	assert.Equal(t, "2026-10", sheet.Month.String(), "month omitted ⇒ the current UTC month")
	require.Len(t, sheet.Months, 12)
	assert.Equal(t, "2026-08", sheet.Months[0].String())
	assert.True(t, sheet.GradingEnabled)
	assert.True(t, sheet.Editable)
	assert.Empty(t, sheet.ReadOnlyReason)
	assert.Equal(t, 1, f.repo.sheetCalls, "the sheet's grades are ONE batched read")
	require.Len(t, sheet.Rows, 2, "only active enrollments of the group")
	byEnrollment := map[string]usecase.MonthlySheetRow{}
	for _, r := range sheet.Rows {
		byEnrollment[r.EnrollmentID] = r
	}
	assert.False(t, byEnrollment[enrollmentID].HasValue)
	assert.True(t, byEnrollment["enr-2"].HasValue)
	assert.Equal(t, "7.5", byEnrollment["enr-2"].Value.String())
	assert.Equal(t, "stu-2", byEnrollment["enr-2"].StudentID)

	other, err := f.svc.Sheet(context.Background(), assignmentID, "2026-09")
	require.NoError(t, err)
	assert.False(t, other.Rows[0].HasValue || other.Rows[1].HasValue, "another month has its own cells")
}

func TestMonthlySheetClampsAndRefuses(t *testing.T) {
	f := newMonthlyFixture(t)
	f.catalog.cycles[cycleID] = ports.CycleRef{ID: cycleID, StartsOn: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), EndsOn: time.Date(2027, 6, 30, 0, 0, 0, 0, time.UTC)}
	sheet, err := f.svc.Sheet(context.Background(), assignmentID, "")
	require.NoError(t, err)
	assert.Equal(t, "2027-01", sheet.Month.String(), "before the cycle ⇒ its first month")

	_, err = f.svc.Sheet(context.Background(), assignmentID, "2026-12")
	assert.Equal(t, "Ese mes no pertenece al ciclo", glerrors.SafeMessage(err))
	_, err = f.svc.Sheet(context.Background(), "nope", "")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))

	f.catalog.cycles[cycleID] = ports.CycleRef{ID: cycleID}
	_, err = f.svc.Sheet(context.Background(), assignmentID, "")
	assert.Equal(t, "El ciclo escolar no tiene fechas de inicio y fin", glerrors.SafeMessage(err))
}

func TestMonthlySheetPreescolarIsReadOnly(t *testing.T) {
	f := newMonthlyFixture(t)
	sheet, err := f.svc.Sheet(context.Background(), kinderAsgID, "2026-09")
	require.NoError(t, err)
	assert.False(t, sheet.GradingEnabled)
	assert.False(t, sheet.Editable)
	assert.Equal(t, "Este nivel no lleva calificación numérica", sheet.ReadOnlyReason)
	assert.Len(t, sheet.Rows, 1)
}

// --- ForEnrollment -----------------------------------------------------------

func TestEnrollmentMonthlyGrades(t *testing.T) {
	f := newMonthlyFixture(t)
	for _, c := range []struct{ month, value string }{{"2026-09", "9.5"}, {"2026-10", "8"}} {
		_, err := f.svc.Upsert(context.Background(), upsertInput(c.month, enrollmentID, c.value))
		require.NoError(t, err)
	}
	got, err := f.svc.ForEnrollment(context.Background(), enrollmentID)
	require.NoError(t, err)
	assert.Equal(t, cycleID, got.CycleID)
	assert.Equal(t, "2026-2027", got.CycleName)
	assert.Len(t, got.Months, 12)
	assert.True(t, got.GradingEnabled)
	assert.Equal(t, []string{subjectID, "subj-2"}, got.SubjectIDs, "the group's subjects, each once")
	require.Len(t, got.Grades, 2)
	assert.Equal(t, "9.5", got.Grades[0].Value().String())

	_, err = f.svc.ForEnrollment(context.Background(), "enr-ghost")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))

	kinder, err := f.svc.ForEnrollment(context.Background(), "enr-kinder")
	require.NoError(t, err)
	assert.False(t, kinder.GradingEnabled)
	assert.Empty(t, kinder.SubjectIDs)
	assert.Empty(t, kinder.Grades)
}
