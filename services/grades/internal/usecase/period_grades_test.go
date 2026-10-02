package usecase_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
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
	capturerName    = "Gabriela Rojas"
	gradeOne        = "gl-1"
	gradeTwo        = "gl-2"
	gradeKinder     = "gl-k"
	period1         = "p1"
	period2         = "p2"
	period3         = "p3"
	firstTermID     = "term-1"
	secondTermID    = "term-2"
)

// fakeCatalog resolves groups to levels, cycles and a group's assignments.
type fakeCatalog struct {
	levelByGroup  map[string]ports.LevelRef
	cycles        map[string]ports.CycleRef
	assignments   []ports.AssignmentRef
	scaleErr      error // LevelOfGroup fails with it (a level whose scale is unusable)
	activeCycle   *ports.CycleRef
	groupsByLevel map[string][]string
	// levelCatalogs answers LevelCatalog per level; catalogErr fails it.
	levelCatalogs map[string]ports.LevelCatalog
	catalogErr    error
	catalogCalls  int
}

func (f *fakeCatalog) LevelCatalog(_ context.Context, _, levelID string) (ports.LevelCatalog, error) {
	f.catalogCalls++
	if f.catalogErr != nil {
		return ports.LevelCatalog{}, f.catalogErr
	}
	c, ok := f.levelCatalogs[levelID]
	if !ok {
		return ports.LevelCatalog{}, glerrors.ErrNotFound
	}
	return c, nil
}

func (f *fakeCatalog) LevelIDOfGroup(_ context.Context, groupID string) (ports.LevelKey, error) {
	l, ok := f.levelByGroup[groupID]
	if !ok {
		return ports.LevelKey{}, glerrors.ErrNotFound
	}
	return ports.LevelKey{ID: l.ID, Deleting: l.Deleting}, nil
}

func (f *fakeCatalog) ActiveCycle(_ context.Context) (ports.CycleRef, bool, error) {
	if f.activeCycle == nil {
		return ports.CycleRef{}, false, nil
	}
	return *f.activeCycle, true, nil
}

func (f *fakeCatalog) GroupsOfLevel(_ context.Context, _, levelID string) ([]string, error) {
	return f.groupsByLevel[levelID], nil
}

func (f *fakeCatalog) LevelOfGroup(_ context.Context, groupID string) (ports.LevelRef, error) {
	if f.scaleErr != nil {
		return ports.LevelRef{}, f.scaleErr
	}
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
			groupID:       {ID: levelPrimaria, GradeLevelID: gradeOne, GradingEnabled: true, Scale: scale},
			kinderGroupID: {ID: levelPreescolar, GradeLevelID: gradeKinder, GradingEnabled: false, Scale: scale},
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

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// trimesters are the three periods of a grade (ids prefixed when not gl-1).
func trimesters(grade string) []domain.EvaluationPeriod {
	prefix := ""
	if grade != gradeOne {
		prefix = grade + "-"
	}
	return []domain.EvaluationPeriod{
		{ID: prefix + period1, GradeLevelID: grade, Ordinal: 1, Name: "1.er trimestre", StartsOn: day(2026, 8, 24), EndsOn: day(2026, 11, 30)},
		{ID: prefix + period2, GradeLevelID: grade, Ordinal: 2, Name: "2.º trimestre", StartsOn: day(2026, 12, 7), EndsOn: day(2027, 3, 26)},
		{ID: prefix + period3, GradeLevelID: grade, Ordinal: 3, Name: "3.er trimestre", StartsOn: day(2027, 4, 5), EndsOn: day(2027, 7, 9)},
	}
}

// fakeEvalPeriods is config's evaluation periods, per grade; levelOf maps a
// grade to its level for the level filter.
type fakeEvalPeriods struct {
	byGrade map[string][]domain.EvaluationPeriod
	levelOf map[string]string
	calls   int
}

func (f *fakeEvalPeriods) EvaluationPeriods(_ context.Context, _, levelID, gradeLevelID string) ([]domain.EvaluationPeriod, error) {
	f.calls++
	grades := make([]string, 0, len(f.byGrade))
	for g := range f.byGrade {
		grades = append(grades, g)
	}
	sort.Strings(grades)
	out := []domain.EvaluationPeriod{}
	for _, g := range grades {
		if (gradeLevelID != "" && g != gradeLevelID) || (levelID != "" && f.levelOf[g] != levelID) {
			continue
		}
		out = append(out, f.byGrade[g]...)
	}
	return out, nil
}

func baseEvalPeriods() *fakeEvalPeriods {
	return &fakeEvalPeriods{
		byGrade: map[string][]domain.EvaluationPeriod{gradeOne: trimesters(gradeOne), gradeTwo: trimesters(gradeTwo)},
		levelOf: map[string]string{gradeOne: levelPrimaria, gradeTwo: levelPrimaria, gradeKinder: levelPreescolar},
	}
}

// fakePeriodGrades is an in-memory period-grade store keyed by (enrollment,
// subject, period); it counts reads to prove they are batched.
type fakePeriodGrades struct {
	rows       map[string]domain.PeriodGrade
	sheetCalls int
	batchCalls int
	deleted    []string
	// locks records the period lock every write was guarded by.
	locks []ports.PeriodLock
	// closures are the stored period closures, keyed by term id.
	closures map[string]domain.PeriodClosure
}

func newFakePeriodGrades() *fakePeriodGrades {
	return &fakePeriodGrades{rows: map[string]domain.PeriodGrade{}, closures: map[string]domain.PeriodClosure{}}
}

func gradeKey(enrollment, subject, period string) string {
	return enrollment + "|" + subject + "|" + period
}

func (f *fakePeriodGrades) UpsertPeriodGrade(_ context.Context, g domain.PeriodGrade, lock ports.PeriodLock) (domain.PeriodGrade, error) {
	f.locks = append(f.locks, lock)
	if c, ok := f.closures[lock.TermID]; ok && c.IsClosed() {
		return domain.PeriodGrade{}, domain.Conflict(domain.CodePeriodLocked, "El periodo está cerrado")
	}
	f.rows[gradeKey(g.EnrollmentID(), g.SubjectID(), g.PeriodID())] = g
	return g, nil
}

func (f *fakePeriodGrades) DeletePeriodGrade(_ context.Context, enrollment, subject, period string, lock ports.PeriodLock) error {
	f.locks = append(f.locks, lock)
	k := gradeKey(enrollment, subject, period)
	delete(f.rows, k)
	f.deleted = append(f.deleted, k)
	return nil
}

func (f *fakePeriodGrades) PeriodClosures(_ context.Context, termIDs []string) (map[string]domain.PeriodClosure, error) {
	out := map[string]domain.PeriodClosure{}
	for _, id := range termIDs {
		if c, ok := f.closures[id]; ok {
			out[id] = c
		}
	}
	return out, nil
}

func (f *fakePeriodGrades) PeriodGradesForSheet(_ context.Context, ids []string, subject, period string) (map[string]domain.PeriodGrade, error) {
	f.sheetCalls++
	out := map[string]domain.PeriodGrade{}
	for _, id := range ids {
		if g, ok := f.rows[gradeKey(id, subject, period)]; ok {
			out[id] = g
		}
	}
	return out, nil
}

func (f *fakePeriodGrades) PeriodGradesForEnrollment(_ context.Context, enrollment string) ([]domain.PeriodGrade, error) {
	var out []domain.PeriodGrade
	for _, g := range f.rows {
		if g.EnrollmentID() == enrollment {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out, nil
}

func (f *fakePeriodGrades) PeriodGradesForEnrollments(_ context.Context, ids, periods []string) ([]domain.PeriodGrade, error) {
	f.batchCalls++
	return f.gradesIn(ids, periods), nil
}

func (f *fakePeriodGrades) gradesIn(ids, periods []string) []domain.PeriodGrade {
	inIDs, inPeriods := idSet(ids), idSet(periods)
	out := []domain.PeriodGrade{}
	for _, g := range f.rows {
		if inIDs[g.EnrollmentID()] && inPeriods[g.PeriodID()] {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

func (f *fakePeriodGrades) PeriodsWithGrades(_ context.Context, periodIDs []string) ([]string, error) {
	out := []string{}
	for _, id := range periodIDs {
		for _, g := range f.rows {
			if g.PeriodID() == id {
				out = append(out, id)
				break
			}
		}
	}
	return out, nil
}

func (f *fakePeriodGrades) CountScaleViolations(_ context.Context, ids []string, check ports.ScaleCheck) (int, error) {
	in := idSet(ids)
	n := 0
	for _, g := range f.rows {
		if in[g.EnrollmentID()] && (!check.GradingEnabled || !check.Scale.Admits(g.Value())) {
			n++
		}
	}
	return n, nil
}

// --- fixture -----------------------------------------------------------------

// sheetNow is mid-October 2026: inside the first trimester (Aug 24–Nov 30).
var sheetNow = time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC)

func mexicoCity(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Mexico_City")
	require.NoError(t, err)
	return loc
}

type gradesFixture struct {
	svc     *usecase.PeriodGradesService
	repo    *fakePeriodGrades
	catalog *fakeCatalog
	asg     *fakeAssignments
	enr     *fakeEnrollments
	evals   *fakeEvalPeriods
	// now is the fixture's clock; a test moves it to reach later periods.
	now *time.Time
}

func newGradesFixture(t *testing.T) gradesFixture {
	t.Helper()
	asg := baseAssignments()
	asg.assignments[kinderAsgID] = ports.AssignmentRef{ID: kinderAsgID, GroupID: kinderGroupID, SubjectID: subjectID, TeacherID: teacherID, CycleID: cycleID}
	asg.terms = map[string]ports.TermRef{
		firstTermID:  {ID: firstTermID, CycleID: cycleID, LevelID: levelPrimaria, Name: "1.er trimestre", Ordinal: 1},
		secondTermID: {ID: secondTermID, CycleID: cycleID, LevelID: levelPrimaria, Name: "2.º trimestre", Ordinal: 2},
	}
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{
		enrollmentID: activeEnrollment(enrollmentID, "stu-1"),
		"enr-2":      activeEnrollment("enr-2", "stu-2"),
		"enr-gone":   {ID: "enr-gone", StudentID: "stu-3", GroupID: groupID, CycleID: cycleID, Status: "withdrawn"},
		"enr-other":  {ID: "enr-other", StudentID: "stu-4", GroupID: "grp-other", CycleID: cycleID, Status: "active"},
		"enr-kinder": {ID: "enr-kinder", StudentID: "stu-5", GroupID: kinderGroupID, CycleID: cycleID, Status: "active"},
	}}
	repo := newFakePeriodGrades()
	catalog := baseCatalog(t)
	evals := baseEvalPeriods()
	seq := 0
	newID := func() string { seq++; return "pg-" + string(rune('0'+seq)) }
	now := sheetNow
	svc := usecase.NewPeriodGradesService(usecase.PeriodGradesDeps{
		Grades: repo, Assignments: asg, Catalog: catalog, Periods: evals, Enrollments: enr,
		TenantID: testTenant, NewID: newID, Now: func() time.Time { return now }, Location: mexicoCity(t),
	})
	return gradesFixture{svc: svc, repo: repo, catalog: catalog, asg: asg, enr: enr, evals: evals, now: &now}
}

func upsertInput(period, enrollment, value string) usecase.UpsertPeriodGradeInput {
	return usecase.UpsertPeriodGradeInput{
		AssignmentID: assignmentID, PeriodID: period, EnrollmentID: enrollment, Value: value,
		CapturedBy: capturer, CapturedByName: capturerName,
	}
}

func closeTerm(t *testing.T, f gradesFixture, termID string) {
	t.Helper()
	c, err := domain.NewOpenPeriod(termID, cycleID, levelPrimaria).Close(domain.Actor{ID: "dir"}, sheetNow, nil)
	require.NoError(t, err)
	f.repo.closures[termID] = c
}

// --- UpsertPeriodGrade -------------------------------------------------------

func TestUpsertPeriodGradeKnot(t *testing.T) {
	tests := []struct {
		name     string
		in       usecase.UpsertPeriodGradeInput
		wantKind glerrors.Kind
		wantCode domain.RefusalCode
		wantMsg  string
	}{
		{name: "unknown assignment is not found", in: usecase.UpsertPeriodGradeInput{AssignmentID: "nope", PeriodID: period1, EnrollmentID: enrollmentID, Value: "9", CapturedBy: capturer}, wantKind: glerrors.KindNotFound},
		{name: "preescolar takes no numeric grade", in: usecase.UpsertPeriodGradeInput{AssignmentID: kinderAsgID, PeriodID: period1, EnrollmentID: "enr-kinder", Value: "9", CapturedBy: capturer}, wantKind: glerrors.KindInvalid, wantCode: domain.CodeLevelNotGraded, wantMsg: "Este nivel no lleva calificación numérica"},
		{name: "enrollment of another group", in: upsertInput(period1, "enr-other", "9"), wantKind: glerrors.KindInvalid, wantCode: domain.CodeEnrollmentNotInGroup, wantMsg: "El alumno no está inscrito en este grupo"},
		{name: "withdrawn enrollment", in: upsertInput(period1, "enr-gone", "9"), wantKind: glerrors.KindInvalid, wantCode: domain.CodeEnrollmentNotInGroup},
		{name: "unknown enrollment", in: upsertInput(period1, "enr-ghost", "9"), wantKind: glerrors.KindInvalid, wantCode: domain.CodeEnrollmentNotInGroup},
		{name: "a period of another grade", in: upsertInput("gl-2-"+period1, enrollmentID, "9"), wantKind: glerrors.KindInvalid, wantCode: domain.CodePeriodNotInGrade, wantMsg: "Ese periodo no pertenece al grado del grupo"},
		{name: "an unknown period", in: upsertInput("p-ghost", enrollmentID, "9"), wantKind: glerrors.KindInvalid, wantCode: domain.CodePeriodNotInGrade},
		{name: "a period that has not started", in: upsertInput(period2, enrollmentID, "9"), wantKind: glerrors.KindInvalid, wantCode: domain.CodePeriodNotStarted, wantMsg: "Aún no empieza ese periodo"},
		{name: "a decimal grade", in: upsertInput(period1, enrollmentID, "9.5"), wantKind: glerrors.KindInvalid, wantCode: domain.CodeGradeNotInteger, wantMsg: "Solo se permiten calificaciones enteras"},
		{name: "above the scale", in: upsertInput(period1, enrollmentID, "11"), wantKind: glerrors.KindInvalid, wantCode: domain.CodeGradeOutOfRange, wantMsg: "La calificación debe estar entre 5 y 10"},
		{name: "below the scale", in: upsertInput(period1, enrollmentID, "4"), wantKind: glerrors.KindInvalid, wantCode: domain.CodeGradeOutOfRange},
		{name: "not a number", in: upsertInput(period1, enrollmentID, "nueve"), wantKind: glerrors.KindInvalid, wantCode: domain.CodeGradeNotNumber},
		{name: "missing captured by", in: usecase.UpsertPeriodGradeInput{AssignmentID: assignmentID, PeriodID: period1, EnrollmentID: enrollmentID, Value: "9"}, wantKind: glerrors.KindInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newGradesFixture(t)
			_, err := f.svc.Upsert(context.Background(), tt.in)
			require.Error(t, err)
			assert.Equal(t, tt.wantKind, glerrors.KindOf(err), "err = %v", err)
			if tt.wantCode != "" {
				assert.Equal(t, tt.wantCode, domain.RefusalCodeOf(err))
			}
			if tt.wantMsg != "" {
				assert.Equal(t, tt.wantMsg, glerrors.SafeMessage(err))
			}
			assert.Empty(t, f.repo.rows, "a refused grade must not be stored")
		})
	}
}

func TestUpsertPeriodGradeDerivesSubjectAndStoresAWholeNumber(t *testing.T) {
	f := newGradesFixture(t)
	g, err := f.svc.Upsert(context.Background(), upsertInput(period1, enrollmentID, "10"))
	require.NoError(t, err)
	assert.Equal(t, subjectID, g.SubjectID())
	assert.Equal(t, period1, g.PeriodID())
	assert.Equal(t, capturer, g.CapturedBy())
	assert.Equal(t, capturerName, g.CapturedByName())
	assert.Equal(t, "10", g.Value().String())

	// A correction of the same cell replaces the value (autosave).
	_, err = f.svc.Upsert(context.Background(), upsertInput(period1, enrollmentID, "8"))
	require.NoError(t, err)
	stored := f.repo.rows[gradeKey(enrollmentID, subjectID, period1)]
	assert.Equal(t, "8", stored.Value().String())
	assert.Len(t, f.repo.rows, 1)
}

// The period has started on the school's LOCAL day: on Dec 7 at 01:00 UTC it
// is still Dec 6 in Mexico City, so the 2nd trimester has not started there.
func TestPeriodStartIsTheSchoolsLocalDay(t *testing.T) {
	f := newGradesFixture(t)
	*f.now = time.Date(2026, 12, 7, 1, 0, 0, 0, time.UTC) // 19:00 Dec 6 in Mexico City
	_, err := f.svc.Upsert(context.Background(), upsertInput(period2, enrollmentID, "9"))
	assert.Equal(t, domain.CodePeriodNotStarted, domain.RefusalCodeOf(err))

	*f.now = time.Date(2026, 12, 7, 7, 0, 0, 0, time.UTC) // 01:00 Dec 7 in Mexico City
	_, err = f.svc.Upsert(context.Background(), upsertInput(period2, enrollmentID, "9"))
	require.NoError(t, err)
}

// A finished period stays writable (corrections) until its term is closed.
func TestFinishedPeriodStaysWritable(t *testing.T) {
	f := newGradesFixture(t)
	*f.now = time.Date(2027, 1, 20, 15, 0, 0, 0, time.UTC)
	_, err := f.svc.Upsert(context.Background(), upsertInput(period1, enrollmentID, "9"))
	require.NoError(t, err)
}

// --- closed terms lock their periods -----------------------------------------

// A period maps to the level term of the same ordinal (ADR 0030): every write
// carries that term's lock, and a closed term refuses the write for everyone.
func TestPeriodWritesCarryTheirTermLock(t *testing.T) {
	f := newGradesFixture(t)
	_, err := f.svc.Upsert(context.Background(), upsertInput(period1, enrollmentID, "9"))
	require.NoError(t, err)
	require.NoError(t, f.svc.Clear(context.Background(), usecase.ScopeTeacher, assignmentID, period1, enrollmentID))
	assert.Equal(t, []ports.PeriodLock{{TermID: firstTermID}, {TermID: firstTermID}}, f.repo.locks)
}

func TestClosedTermLocksItsPeriodForEveryone(t *testing.T) {
	for _, scope := range []usecase.CaptureScope{usecase.ScopeTeacher, usecase.ScopeSchool} {
		f := newGradesFixture(t)
		closeTerm(t, f, firstTermID)
		in := upsertInput(period1, enrollmentID, "9")
		in.Scope = scope
		_, err := f.svc.Upsert(context.Background(), in)
		assert.Equal(t, domain.CodePeriodLocked, domain.RefusalCodeOf(err))
		assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
		err = f.svc.Clear(context.Background(), scope, assignmentID, period1, enrollmentID)
		assert.Equal(t, domain.CodePeriodLocked, domain.RefusalCodeOf(err))
		assert.Empty(t, f.repo.rows)
		assert.Empty(t, f.repo.deleted)

		sheet, err := f.svc.Sheet(context.Background(), scope, assignmentID, period1)
		require.NoError(t, err)
		assert.False(t, sheet.Editable)
		assert.Equal(t, "El periodo está cerrado", sheet.ReadOnlyReason)
		assert.Equal(t, []string{period1}, sheet.ClosedPeriodIDs)
	}
}

// A reopened term unlocks; another level's closure never locks this grade.
func TestReopenedOrForeignTermDoesNotLock(t *testing.T) {
	f := newGradesFixture(t)
	closeTerm(t, f, firstTermID)
	reopened, err := f.repo.closures[firstTermID].Reopen(sheetNow)
	require.NoError(t, err)
	f.repo.closures[firstTermID] = reopened
	_, err = f.svc.Upsert(context.Background(), upsertInput(period1, enrollmentID, "9"))
	require.NoError(t, err)

	g := newGradesFixture(t)
	g.asg.terms["term-kinder"] = ports.TermRef{ID: "term-kinder", CycleID: cycleID, LevelID: levelPreescolar, Ordinal: 1}
	closeTerm(t, g, "term-kinder")
	_, err = g.svc.Upsert(context.Background(), upsertInput(period1, enrollmentID, "9"))
	require.NoError(t, err)
}

// A period with no term of its ordinal guards nothing: still capturable.
func TestPeriodWithoutATermIsUnguarded(t *testing.T) {
	f := newGradesFixture(t)
	delete(f.asg.terms, firstTermID)
	_, err := f.svc.Upsert(context.Background(), upsertInput(period1, enrollmentID, "9"))
	require.NoError(t, err)
	assert.Equal(t, []ports.PeriodLock{{}}, f.repo.locks)
}

// --- transfers ---------------------------------------------------------------

// A pupil who moved into the group: for a teacher, the periods that ENDED
// before the move belong to the previous group (read-only); the period the
// move falls in, and later ones, are the new group's. The director may write
// any of them.
func TestPeriodsBeforeATransferBelongToThePreviousGroup(t *testing.T) {
	f := newGradesFixture(t)
	*f.now = time.Date(2027, 1, 20, 15, 0, 0, 0, time.UTC) // inside the 2nd trimester
	f.enr.transfers = []ports.TransferRef{{EnrollmentID: enrollmentID, ToGroupID: groupID, MovedAt: time.Date(2026, 12, 10, 18, 0, 0, 0, time.UTC)}}

	_, err := f.svc.Upsert(context.Background(), upsertInput(period1, enrollmentID, "9"))
	assert.Equal(t, domain.CodePeriodPreviousGroup, domain.RefusalCodeOf(err))
	assert.Equal(t, "Ese periodo corresponde al grupo anterior del alumno", glerrors.SafeMessage(err))

	_, err = f.svc.Upsert(context.Background(), upsertInput(period2, enrollmentID, "9"))
	require.NoError(t, err, "the period of the move is the new group's")

	director := upsertInput(period1, enrollmentID, "9")
	director.Scope = usecase.ScopeSchool
	_, err = f.svc.Upsert(context.Background(), director)
	require.NoError(t, err, "the director acts for the whole school")

	sheet, err := f.svc.Sheet(context.Background(), usecase.ScopeTeacher, assignmentID, period1)
	require.NoError(t, err)
	rows := map[string]usecase.PeriodSheetRow{}
	for _, r := range sheet.Rows {
		rows[r.EnrollmentID] = r
	}
	assert.False(t, rows[enrollmentID].Editable)
	assert.Equal(t, "Ese periodo corresponde al grupo anterior del alumno", rows[enrollmentID].ReadOnlyReason)
	assert.True(t, rows["enr-2"].Editable)
}

// --- ClearPeriodGrade --------------------------------------------------------

func TestClearPeriodGrade(t *testing.T) {
	f := newGradesFixture(t)
	_, err := f.svc.Upsert(context.Background(), upsertInput(period1, enrollmentID, "9"))
	require.NoError(t, err)

	require.NoError(t, f.svc.Clear(context.Background(), usecase.ScopeTeacher, assignmentID, period1, enrollmentID))
	assert.Empty(t, f.repo.rows)
	require.NoError(t, f.svc.Clear(context.Background(), usecase.ScopeTeacher, assignmentID, period1, enrollmentID), "clearing an empty cell succeeds")

	tests := []struct {
		name, asg, period, enrollment string
		wantKind                      glerrors.Kind
	}{
		{"unknown assignment", "nope", period1, enrollmentID, glerrors.KindNotFound},
		{"period of another grade", assignmentID, "gl-2-" + period1, enrollmentID, glerrors.KindInvalid},
		{"period not started", assignmentID, period3, enrollmentID, glerrors.KindInvalid},
		{"enrollment of another group", assignmentID, period1, "enr-other", glerrors.KindInvalid},
		{"preescolar", kinderAsgID, period1, "enr-kinder", glerrors.KindInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newGradesFixture(t)
			err := f.svc.Clear(context.Background(), usecase.ScopeTeacher, tt.asg, tt.period, tt.enrollment)
			assert.Equal(t, tt.wantKind, glerrors.KindOf(err), "err = %v", err)
			assert.Empty(t, f.repo.deleted, "a refused clear must not reach the store")
		})
	}
}

// Review finding 4 (ADR 0021): while a level's deletion runs, every write that
// resolves it is refused with LEVEL_BEING_DELETED. Reads keep working.
func TestPeriodGradeWritesRefuseALevelBeingDeleted(t *testing.T) {
	writes := map[string]func(f gradesFixture) error{
		"upsert": func(f gradesFixture) error {
			_, err := f.svc.Upsert(context.Background(), upsertInput(period1, enrollmentID, "9"))
			return err
		},
		"clear": func(f gradesFixture) error {
			return f.svc.Clear(context.Background(), usecase.ScopeSchool, assignmentID, period1, enrollmentID)
		},
	}
	for name, write := range writes {
		t.Run(name, func(t *testing.T) {
			f := newGradesFixture(t)
			lvl := f.catalog.levelByGroup[groupID]
			lvl.Deleting = true
			f.catalog.levelByGroup[groupID] = lvl
			err := write(f)
			assert.Equal(t, levelarchive.CodeLevelBeingDeleted, levelarchive.CodeOf(err), "err = %v", err)
			assert.Empty(t, f.repo.rows)
			_, err = f.svc.Sheet(context.Background(), usecase.ScopeSchool, assignmentID, period1)
			require.NoError(t, err, "the grid still reads")
		})
	}
}

// --- GetPeriodSheet ----------------------------------------------------------

func TestPeriodSheet(t *testing.T) {
	f := newGradesFixture(t)
	_, err := f.svc.Upsert(context.Background(), upsertInput(period1, "enr-2", "7"))
	require.NoError(t, err)

	sheet, err := f.svc.Sheet(context.Background(), usecase.ScopeTeacher, assignmentID, "")
	require.NoError(t, err)
	assert.Equal(t, period1, sheet.PeriodID, "period omitted ⇒ the period of the school's day")
	assert.Equal(t, period1, sheet.CurrentPeriodID)
	assert.Equal(t, day(2026, 10, 15), sheet.Today)
	require.Len(t, sheet.Periods, 3)
	assert.Equal(t, "2.º trimestre", sheet.Periods[1].Name)
	assert.True(t, sheet.GradingEnabled)
	assert.True(t, sheet.Editable)
	assert.Empty(t, sheet.ReadOnlyReason)
	assert.Empty(t, sheet.ClosedPeriodIDs)
	assert.Equal(t, 1, f.repo.sheetCalls, "the sheet's grades are ONE batched read")
	require.Len(t, sheet.Rows, 2, "only active enrollments of the group")
	byEnrollment := map[string]usecase.PeriodSheetRow{}
	for _, r := range sheet.Rows {
		byEnrollment[r.EnrollmentID] = r
	}
	assert.False(t, byEnrollment[enrollmentID].HasValue)
	assert.True(t, byEnrollment["enr-2"].HasValue)
	assert.Equal(t, "7", byEnrollment["enr-2"].Value.String())
	assert.Equal(t, "stu-2", byEnrollment["enr-2"].StudentID)

	later, err := f.svc.Sheet(context.Background(), usecase.ScopeTeacher, assignmentID, period2)
	require.NoError(t, err)
	assert.False(t, later.Editable, "a period that has not started is read-only")
	assert.Equal(t, "Aún no empieza ese periodo", later.ReadOnlyReason)
	assert.False(t, later.Rows[0].HasValue || later.Rows[1].HasValue, "another period has its own cells")

	_, err = f.svc.Sheet(context.Background(), usecase.ScopeTeacher, assignmentID, "gl-2-"+period1)
	assert.Equal(t, domain.CodePeriodNotInGrade, domain.RefusalCodeOf(err))
	_, err = f.svc.Sheet(context.Background(), usecase.ScopeTeacher, "nope", "")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}

// Between two periods the sheet opens on the one that just ended (its grades
// are the ones being finished), and says no period is current.
func TestPeriodSheetBetweenPeriods(t *testing.T) {
	f := newGradesFixture(t)
	*f.now = time.Date(2026, 12, 3, 15, 0, 0, 0, time.UTC)
	sheet, err := f.svc.Sheet(context.Background(), usecase.ScopeTeacher, assignmentID, "")
	require.NoError(t, err)
	assert.Equal(t, period1, sheet.PeriodID)
	assert.Empty(t, sheet.CurrentPeriodID)
	assert.True(t, sheet.Editable)
}

func TestPeriodSheetOfAGradeWithoutPeriods(t *testing.T) {
	f := newGradesFixture(t)
	delete(f.evals.byGrade, gradeOne)
	sheet, err := f.svc.Sheet(context.Background(), usecase.ScopeTeacher, assignmentID, "")
	require.NoError(t, err)
	assert.Empty(t, sheet.Periods)
	assert.Empty(t, sheet.PeriodID)
	assert.False(t, sheet.Editable)
	assert.Equal(t, "Este grado no tiene periodos de evaluación configurados", sheet.ReadOnlyReason)
	assert.Len(t, sheet.Rows, 2, "the pupils still show")
	for _, r := range sheet.Rows {
		assert.False(t, r.Editable)
	}
}

func TestPeriodSheetPreescolarIsReadOnly(t *testing.T) {
	f := newGradesFixture(t)
	f.evals.byGrade[gradeKinder] = trimesters(gradeKinder)
	sheet, err := f.svc.Sheet(context.Background(), usecase.ScopeTeacher, kinderAsgID, "")
	require.NoError(t, err)
	assert.False(t, sheet.GradingEnabled)
	assert.False(t, sheet.Editable)
	assert.Equal(t, "Este nivel no lleva calificación numérica", sheet.ReadOnlyReason)
	assert.Len(t, sheet.Rows, 1)
}

// --- ForEnrollment -----------------------------------------------------------

func TestEnrollmentPeriodGrades(t *testing.T) {
	f := newGradesFixture(t)
	_, err := f.svc.Upsert(context.Background(), upsertInput(period1, enrollmentID, "9"))
	require.NoError(t, err)
	got, err := f.svc.ForEnrollment(context.Background(), enrollmentID)
	require.NoError(t, err)
	assert.Equal(t, cycleID, got.CycleID)
	assert.Equal(t, "2026-2027", got.CycleName)
	require.Len(t, got.Periods, 3)
	assert.Equal(t, period1, got.Periods[0].ID)
	assert.True(t, got.GradingEnabled)
	assert.Equal(t, []string{subjectID, "subj-2"}, got.SubjectIDs, "the group's subjects, each once")
	require.Len(t, got.Grades, 1)
	assert.Equal(t, "9", got.Grades[0].Value().String())

	_, err = f.svc.ForEnrollment(context.Background(), "enr-ghost")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))

	kinder, err := f.svc.ForEnrollment(context.Background(), "enr-kinder")
	require.NoError(t, err)
	assert.False(t, kinder.GradingEnabled)
	assert.Empty(t, kinder.SubjectIDs)
	assert.Empty(t, kinder.Grades)
}

// A level that stops grading numerically still shows what was captured: the
// grades are never hidden, and only the graded subjects are listed.
func TestEnrollmentPeriodGradesKeepsGradesAfterGradingIsDisabled(t *testing.T) {
	f := newGradesFixture(t)
	_, err := f.svc.Upsert(context.Background(), upsertInput(period1, enrollmentID, "9"))
	require.NoError(t, err)
	lvl := f.catalog.levelByGroup[groupID]
	lvl.GradingEnabled = false
	f.catalog.levelByGroup[groupID] = lvl

	got, err := f.svc.ForEnrollment(context.Background(), enrollmentID)
	require.NoError(t, err)
	assert.False(t, got.GradingEnabled)
	assert.Equal(t, []string{subjectID}, got.SubjectIDs)
	assert.Len(t, got.Grades, 1)
}

// --- scale violations and the config guard -----------------------------------

func TestCountScaleViolations(t *testing.T) {
	f := newGradesFixture(t)
	cycle := f.catalog.cycles[cycleID]
	f.catalog.activeCycle = &cycle
	f.catalog.groupsByLevel = map[string][]string{levelPrimaria: {groupID}}
	f.enr.byID["enr-3"] = activeEnrollment("enr-3", "stu-3")
	for enr, v := range map[string]string{enrollmentID: "5", "enr-2": "9", "enr-3": "10"} {
		_, err := f.svc.Upsert(context.Background(), upsertInput(period1, enr, v))
		require.NoError(t, err)
	}
	tighter := ports.ScaleCheck{GradingEnabled: true, Scale: mustScale(t, 600, 1000, 700, 0)}
	n, err := f.svc.CountScaleViolations(context.Background(), levelPrimaria, tighter)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "only the 5 falls outside 6–10")

	n, err = f.svc.CountScaleViolations(context.Background(), levelPrimaria, ports.ScaleCheck{GradingEnabled: false})
	require.NoError(t, err)
	assert.Equal(t, 3, n, "disabling grading counts every grade")

	f.catalog.activeCycle = nil
	n, err = f.svc.CountScaleViolations(context.Background(), levelPrimaria, ports.ScaleCheck{})
	require.NoError(t, err)
	assert.Zero(t, n, "no active cycle counts 0")
}

// T14's guard: config must not remove a period that already holds grades;
// the gateway asks which of the periods it would remove hold any.
func TestPeriodsWithGrades(t *testing.T) {
	f := newGradesFixture(t)
	_, err := f.svc.Upsert(context.Background(), upsertInput(period1, enrollmentID, "9"))
	require.NoError(t, err)
	got, err := f.svc.PeriodsWithGrades(context.Background(), []string{period1, period2})
	require.NoError(t, err)
	assert.Equal(t, []string{period1}, got)

	got, err = f.svc.PeriodsWithGrades(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}
