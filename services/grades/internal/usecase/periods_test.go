package usecase_test

import (
	"context"
	"errors"
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

// fakePeriods stores closures and report cards in memory. WithPeriodLock
// stages every write and applies it only when fn succeeds (a rollback keeps
// nothing), like the real transaction.
type fakePeriods struct {
	grades    *fakePeriodGrades
	cards     []domain.ReportCard
	lockCalls int
	// outbox holds the queued events, committed with the transaction.
	outbox []fakeOutboxRow
	// cancelAfterLock, when set, runs once the transaction returns (the
	// request's client hangs up right after the commit).
	cancelAfterLock func()
}

type fakeOutboxRow struct {
	event     ports.OutboxEvent
	published bool
	attempts  int
}

// DrainOutbox publishes the unpublished rows in order, marking the ones that
// went out and counting every attempt, like the real drain.
func (f *fakePeriods) DrainOutbox(_ context.Context, limit int, publish func(ports.OutboxEvent) error) (int, error) {
	published := 0
	for i := range f.outbox {
		if published >= limit {
			break
		}
		row := &f.outbox[i]
		if row.published {
			continue
		}
		row.attempts++
		if err := publish(row.event); err != nil {
			continue
		}
		row.published = true
		published++
	}
	return published, nil
}

func (f *fakePeriods) pending() int {
	n := 0
	for _, r := range f.outbox {
		if !r.published {
			n++
		}
	}
	return n
}

type fakePeriodTx struct {
	p        *fakePeriods
	termID   string
	closure  *domain.PeriodClosure
	issued   []domain.ReportCard
	issuing  bool
	queued   []ports.OutboxEvent
	lockedAt int
}

func (f *fakePeriods) WithPeriodLock(ctx context.Context, termID string, fn func(context.Context, ports.PeriodTx) error) error {
	f.lockCalls++
	if f.cancelAfterLock != nil {
		defer f.cancelAfterLock()
	}
	tx := &fakePeriodTx{p: f, termID: termID}
	if err := fn(ctx, tx); err != nil {
		return err
	}
	if tx.closure != nil {
		f.grades.closures[termID] = *tx.closure
	}
	if tx.issuing {
		for i, old := range f.cards {
			if old.TermID() == termID && old.Status() == domain.ReportCardPublished {
				f.cards[i] = old.WithStatus(domain.ReportCardSuperseded)
			}
		}
	}
	f.cards = append(f.cards, tx.issued...)
	for _, e := range tx.queued {
		f.outbox = append(f.outbox, fakeOutboxRow{event: e})
	}
	return nil
}

func (t *fakePeriodTx) QueueEvents(_ context.Context, events []ports.OutboxEvent) error {
	t.queued = append(t.queued, events...)
	return nil
}

func (t *fakePeriodTx) Closure(context.Context) (domain.PeriodClosure, bool, error) {
	c, ok := t.p.grades.closures[t.termID]
	return c, ok, nil
}

func (t *fakePeriodTx) PeriodGrades(_ context.Context, ids, periods []string) ([]domain.PeriodGrade, error) {
	return t.p.grades.gradesIn(ids, periods), nil
}

func (t *fakePeriodTx) SaveClosure(_ context.Context, c domain.PeriodClosure) error {
	t.closure = &c
	return nil
}

// PublishedReportCards is the term's published cards, lines included, as the
// transaction reads them before superseding them.
func (t *fakePeriodTx) PublishedReportCards(context.Context) ([]domain.ReportCard, error) {
	var out []domain.ReportCard
	for _, rc := range t.p.cards {
		if rc.TermID() == t.termID && rc.Status() == domain.ReportCardPublished {
			out = append(out, rc)
		}
	}
	return out, nil
}

func (t *fakePeriodTx) IssueReportCards(_ context.Context, cards []domain.ReportCard) error {
	t.issuing = true
	t.issued = append(t.issued, cards...)
	return nil
}

func (f *fakePeriods) LatestReportCards(_ context.Context, filter ports.ReportCardFilter) ([]domain.ReportCard, error) {
	out := []domain.ReportCard{}
	for _, rc := range f.cards {
		h := rc.Header()
		if rc.Status() != domain.ReportCardPublished ||
			(filter.TermID != "" && h.TermID != filter.TermID) ||
			(filter.GroupID != "" && h.GroupID != filter.GroupID) ||
			(filter.StudentID != "" && h.StudentID != filter.StudentID) {
			continue
		}
		out = append(out, rc)
	}
	return out, nil
}

func (f *fakePeriods) ReportCard(_ context.Context, id string) (domain.ReportCard, error) {
	for _, rc := range f.cards {
		if rc.ID() == id {
			return rc, nil
		}
	}
	return domain.ReportCard{}, glerrors.ErrNotFound
}

type fakeDirectory struct {
	names map[string]ports.StudentRef
	err   error
	calls int
}

func (f *fakeDirectory) StudentsByIDs(_ context.Context, ids []string) (map[string]ports.StudentRef, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]ports.StudentRef{}
	for _, id := range ids {
		if s, ok := f.names[id]; ok {
			out[id] = s
		}
	}
	return out, nil
}

type fakePublisher struct {
	published []ports.ReportCardIssuedEvent
	err       error
	// ctxErr records whether a publish saw an already-cancelled context.
	ctxErr error
}

func (f *fakePublisher) PublishReportCardIssued(ctx context.Context, e ports.ReportCardIssuedEvent) error {
	if ctx.Err() != nil {
		f.ctxErr = ctx.Err()
	}
	if f.err != nil {
		return f.err
	}
	f.published = append(f.published, e)
	return nil
}

// --- fixture -----------------------------------------------------------------

const (
	group2ID  = "grp-2"
	subject2  = "subj-2"
	director  = "user-dir"
	levelName = "Primaria"
)

// Two groups of the primary level: grp-1, of grade gl-1 (enr-1, enr-2;
// Matemáticas by teacher-1, Español by teacher-2) and grp-2, of grade gl-2
// (enr-3; Matemáticas by teacher-3). Each grade has three trimesters; the 1.er
// trimestre term stands for period 1 of each grade (ADR 0030). "Now" is late
// November, inside period 1: expected = 2×2 + 1×1 = 5.
type periodsFixture struct {
	svc       *usecase.PeriodsService
	grades    *fakePeriodGrades
	periods   *fakePeriods
	directory *fakeDirectory
	publisher *fakePublisher
	catalog   *fakeCatalog
	asg       *fakeAssignments
	enr       *fakeEnrollments
	evals     *fakeEvalPeriods
	now       *time.Time
}

var periodsNow = time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC)

func newPeriodsFixture(t *testing.T) periodsFixture {
	t.Helper()
	scale := mustScale(t, 500, 1000, 600, 1)
	catalog := baseCatalog(t)
	catalog.levelCatalogs = map[string]ports.LevelCatalog{levelPrimaria: {
		LevelName: levelName, GradingEnabled: true, Scale: scale,
		Groups: []ports.GroupRef{{ID: groupID, Label: "1° · A", GradeLevelID: gradeOne}, {ID: group2ID, Label: "2° · A", GradeLevelID: gradeTwo}},
		Assignments: []ports.AssignmentRef{
			{ID: assignmentID, GroupID: groupID, SubjectID: subjectID, TeacherID: teacherID, CycleID: cycleID},
			{ID: "asg-2", GroupID: groupID, SubjectID: subject2, TeacherID: "teacher-2", CycleID: cycleID},
			{ID: "asg-3", GroupID: group2ID, SubjectID: subjectID, TeacherID: "teacher-3", CycleID: cycleID},
		},
		SubjectNames: map[string]string{subjectID: "Matemáticas", subject2: "Español"},
		TeacherNames: map[string]string{teacherID: "Gabriela Rojas", "teacher-2": "Luis Pérez", "teacher-3": "Ana Ruiz"},
	}, levelPreescolar: {LevelName: "Preescolar", GradingEnabled: false,
		Groups: []ports.GroupRef{{ID: kinderGroupID, Label: "K · A", GradeLevelID: gradeKinder}}}}
	asg := baseAssignments()
	asg.terms = map[string]ports.TermRef{
		firstTermID:   {ID: firstTermID, CycleID: cycleID, LevelID: levelPrimaria, Name: "1.er trimestre", Ordinal: 1},
		secondTermID:  {ID: secondTermID, CycleID: cycleID, LevelID: levelPrimaria, Name: "2.º trimestre", Ordinal: 2},
		"term-fourth": {ID: "term-fourth", CycleID: cycleID, LevelID: levelPrimaria, Name: "4.º bloque", Ordinal: 4},
		"term-kinder": {ID: "term-kinder", CycleID: cycleID, LevelID: levelPreescolar, Name: "1.er trimestre", Ordinal: 1},
	}
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{
		"enr-1":    {ID: "enr-1", StudentID: "stu-1", GroupID: groupID, CycleID: cycleID, Status: "active"},
		"enr-2":    {ID: "enr-2", StudentID: "stu-2", GroupID: groupID, CycleID: cycleID, Status: "active"},
		"enr-3":    {ID: "enr-3", StudentID: "stu-3", GroupID: group2ID, CycleID: cycleID, Status: "active"},
		"enr-gone": {ID: "enr-gone", StudentID: "stu-4", GroupID: groupID, CycleID: cycleID, Status: "withdrawn"},
		"enr-kid":  {ID: "enr-kid", StudentID: "stu-5", GroupID: kinderGroupID, CycleID: cycleID, Status: "active"},
	}}
	grades := newFakePeriodGrades()
	periods := &fakePeriods{grades: grades}
	directory := &fakeDirectory{names: map[string]ports.StudentRef{
		"stu-1": {ID: "stu-1", Name: "Ana López", EnrollmentNumber: "2026-00001"},
		"stu-2": {ID: "stu-2", Name: "Beto Díaz", EnrollmentNumber: "2026-00002"},
		"stu-3": {ID: "stu-3", Name: "Carla Ñúñez", EnrollmentNumber: "2026-00003"},
	}}
	publisher := &fakePublisher{}
	evals := baseEvalPeriods()
	seq := 0
	now := periodsNow
	svc := usecase.NewPeriodsService(usecase.PeriodsDeps{
		Periods: periods, Grades: grades, EvalPeriods: evals, Terms: asg, Catalog: catalog, Enrollments: enr,
		Students: directory, Publisher: publisher, TenantID: testTenant,
		NewID:    func() string { seq++; return "rc-" + string(rune('a'+seq-1)) },
		Now:      func() time.Time { return now },
		Location: mexicoCity(t),
	})
	return periodsFixture{svc: svc, grades: grades, periods: periods, directory: directory, publisher: publisher,
		catalog: catalog, asg: asg, enr: enr, evals: evals, now: &now}
}

// capture stores a whole-number grade directly (the capture knot is tested
// elsewhere).
func (f periodsFixture) capture(t *testing.T, enrollment, subject, period string, whole int) {
	t.Helper()
	g, err := domain.NewPeriodGrade(domain.PeriodGradeFields{
		ID: "pg-" + enrollment + subject + period, TenantID: testTenant, EnrollmentID: enrollment, SubjectID: subject,
		PeriodID: period, CapturedBy: capturer, Value: domain.GradeValueFromHundredths(whole * 100),
	})
	require.NoError(t, err)
	f.grades.rows[gradeKey(enrollment, subject, period)] = g
}

// captureAll fills every expected cell of the first trimester.
func (f periodsFixture) captureAll(t *testing.T) {
	f.capture(t, "enr-1", subjectID, period1, 9)
	f.capture(t, "enr-1", subject2, period1, 8)
	f.capture(t, "enr-2", subjectID, period1, 7)
	f.capture(t, "enr-2", subject2, period1, 10)
	f.capture(t, "enr-3", subjectID, "gl-2-"+period1, 8)
}

var dirActor = domain.Actor{ID: director, Name: "Directora Ruiz"}

// --- progress ----------------------------------------------------------------

func TestPeriodProgressCountsAndGroupsWhatIsMissing(t *testing.T) {
	f := newPeriodsFixture(t)
	f.capture(t, "enr-1", subjectID, period1, 9)
	f.capture(t, "enr-2", subjectID, period1, 9)
	f.capture(t, "enr-gone", subjectID, period1, 9)   // withdrawn: not expected, not counted
	f.capture(t, "enr-1", subjectID, period2, 9)      // next term
	f.capture(t, "enr-1", "subj-dropped", period1, 9) // no longer taught: not expected
	f.capture(t, "enr-3", subjectID, period1, 9)      // a period of ANOTHER grade: not grp-2's

	got, err := f.svc.Progress(context.Background(), firstTermID)
	require.NoError(t, err)
	assert.Equal(t, 5, got.Expected)
	assert.Equal(t, 2, got.Captured)
	assert.True(t, got.Unfinished, "period 1 ends Nov 30")
	assert.Equal(t, day(2026, 11, 30), got.EndsOn)
	assert.Empty(t, got.GroupsWithoutPeriod)
	assert.Equal(t, 1, f.grades.batchCalls, "every grade of the level in ONE read")
	assert.Equal(t, 1, f.catalog.catalogCalls)
	assert.Equal(t, []usecase.MissingGrades{
		{GroupID: groupID, GroupLabel: "1° · A", SubjectID: subject2, SubjectName: "Español", TeacherName: "Luis Pérez", Missing: 2},
		{GroupID: group2ID, GroupLabel: "2° · A", SubjectID: subjectID, SubjectName: "Matemáticas", TeacherName: "Ana Ruiz", Missing: 1},
	}, got.Missing)
}

// A term is finished once every grade's period of it ended on the school's
// LOCAL day; a group whose grade lacks the period is named.
func TestPeriodProgressUnfinishedAndGroupsWithoutPeriod(t *testing.T) {
	f := newPeriodsFixture(t)
	*f.now = time.Date(2026, 12, 1, 3, 0, 0, 0, time.UTC) // 21:00 Nov 30 in Mexico City
	got, err := f.svc.Progress(context.Background(), firstTermID)
	require.NoError(t, err)
	assert.True(t, got.Unfinished, "still Nov 30 at school")

	*f.now = time.Date(2026, 12, 1, 15, 0, 0, 0, time.UTC)
	got, err = f.svc.Progress(context.Background(), firstTermID)
	require.NoError(t, err)
	assert.False(t, got.Unfinished)

	f.evals.byGrade[gradeTwo] = trimesters(gradeTwo)[1:] // gl-2 has no period 1
	got, err = f.svc.Progress(context.Background(), firstTermID)
	require.NoError(t, err)
	assert.Equal(t, []string{"2° · A"}, got.GroupsWithoutPeriod)
	assert.Equal(t, 4, got.Expected, "grp-2 owes nothing it cannot capture")

	fourth, err := f.svc.Progress(context.Background(), "term-fourth")
	require.NoError(t, err)
	assert.Zero(t, fourth.Expected)
	assert.Equal(t, []string{"1° · A", "2° · A"}, fourth.GroupsWithoutPeriod)
	assert.True(t, fourth.EndsOn.IsZero())

	_, err = f.svc.Progress(context.Background(), "ghost")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}

// A pupil who joined the group after the term's period ended owes nothing in
// it (it belongs to before, or to the previous group).
func TestPeriodProgressSkipsPupilsWhoJoinedAfterThePeriod(t *testing.T) {
	f := newPeriodsFixture(t)
	*f.now = time.Date(2027, 1, 20, 15, 0, 0, 0, time.UTC)
	late := f.enr.byID["enr-3"]
	late.EnrolledAt = time.Date(2026, 12, 10, 15, 0, 0, 0, time.UTC)
	f.enr.byID["enr-3"] = late
	f.enr.transfers = []ports.TransferRef{
		{EnrollmentID: "enr-2", ToGroupID: groupID, MovedAt: time.Date(2026, 11, 2, 15, 0, 0, 0, time.UTC)}, // during period 1
	}

	got, err := f.svc.Progress(context.Background(), firstTermID)
	require.NoError(t, err)
	assert.Equal(t, 4, got.Expected, "enr-3 joined after period 1 ended; enr-2 joined during it")

	f.captureAll(t)
	delete(f.grades.rows, gradeKey("enr-3", subjectID, "gl-2-"+period1))
	res, err := f.svc.Close(context.Background(), firstTermID, false, dirActor)
	require.NoError(t, err, "complete for every pupil who owes it: no force needed")
	assert.Equal(t, 2, res.ReportCards)
	assert.Equal(t, 1, res.SkippedWithoutGrades, "enr-3 has no grade in period 1: no card (T10)")
}

// --- close -------------------------------------------------------------------

func TestCloseRefusesMissingGradesWithoutForce(t *testing.T) {
	f := newPeriodsFixture(t)
	f.captureAll(t)
	delete(f.grades.rows, gradeKey("enr-3", subjectID, "gl-2-"+period1))
	delete(f.grades.rows, gradeKey("enr-2", subject2, period1))

	_, err := f.svc.Close(context.Background(), firstTermID, false, dirActor)
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Equal(t, domain.CodeMissingGrades, domain.RefusalCodeOf(err))
	assert.Equal(t, "Faltan 2 calificaciones por capturar", glerrors.SafeMessage(err))
	assert.Empty(t, f.grades.closures, "nothing closed")
	assert.Empty(t, f.periods.cards, "no report card issued")
	assert.Empty(t, f.publisher.published)

	f.capture(t, "enr-3", subjectID, "gl-2-"+period1, 9)
	_, err = f.svc.Close(context.Background(), firstTermID, false, dirActor)
	assert.Equal(t, "Falta 1 calificación por capturar", glerrors.SafeMessage(err))
}

// Closing with force issues a report card per ACTIVE enrollment of the level,
// version 1, with the names frozen, then announces each one. Its columns are
// the pupil's grade's periods up to the term's ordinal; the averages are
// exact, truncated to 2 decimals.
func TestCloseWithForceIssuesReportCards(t *testing.T) {
	f := newPeriodsFixture(t)
	*f.now = time.Date(2027, 3, 30, 15, 0, 0, 0, time.UTC)
	f.capture(t, "enr-1", subjectID, period1, 9)
	f.capture(t, "enr-1", subjectID, period2, 8)
	f.capture(t, "enr-1", subject2, period2, 7)
	f.capture(t, "enr-1", subjectID, period3, 5) // a later period: not on this card
	f.capture(t, "enr-2", subjectID, period2, 6)
	f.capture(t, "enr-3", subjectID, "gl-2-"+period2, 8)

	res, err := f.svc.Close(context.Background(), secondTermID, true, dirActor)
	require.NoError(t, err)
	assert.Equal(t, usecase.CloseResult{Status: domain.PeriodClosed, Version: 1, ReportCards: 3}, res)

	closure := f.grades.closures[secondTermID]
	assert.True(t, closure.IsClosed())
	assert.Equal(t, "Directora Ruiz", closure.ClosedByName())
	assert.Equal(t, levelPrimaria, closure.LevelID())

	require.Len(t, f.periods.cards, 3)
	byEnrollment := map[string]domain.ReportCard{}
	for _, rc := range f.periods.cards {
		byEnrollment[rc.EnrollmentID()] = rc
	}
	ana := byEnrollment["enr-1"]
	h := ana.Header()
	assert.Equal(t, 1, h.Version)
	assert.Equal(t, "Ana López", h.StudentName)
	assert.Equal(t, "2026-00001", h.EnrollmentNumber)
	assert.Equal(t, "1° · A", h.GroupLabel)
	assert.Equal(t, levelName, h.LevelName)
	assert.Equal(t, "2026-2027", h.CycleName)
	assert.Equal(t, "2.º trimestre", h.TermName, "named after the pupil's grade's period")
	assert.Equal(t, 2, h.TermOrdinal)
	assert.Empty(t, h.Months, "a T15 card has period columns, no months")
	require.Len(t, h.Periods, 2)
	assert.Equal(t, period1, h.Periods[0].ID)
	assert.Equal(t, period2, h.Periods[1].ID)
	assert.Equal(t, director, h.IssuedBy)
	require.Len(t, ana.Lines(), 2)
	assert.Equal(t, "Español", ana.Lines()[0].SubjectName, "subjects in Spanish name order")
	assert.Equal(t, "7", ana.Lines()[0].Average.String())
	assert.Equal(t, "Matemáticas", ana.Lines()[1].SubjectName)
	assert.Equal(t, map[string]string{period1: "9", period2: "8"}, ana.Lines()[1].PeriodValueStrings())
	assert.Equal(t, "8.5", ana.Lines()[1].Average.String())
	assert.Equal(t, "7.75", ana.GeneralAverage().String(), "exact mean of 7 and 8.5")
	carla := byEnrollment["enr-3"].Header()
	assert.Equal(t, "gl-2-"+period1, carla.Periods[0].ID, "grp-2's card has its own grade's periods")
	_, withdrawn := byEnrollment["enr-gone"]
	assert.False(t, withdrawn, "a withdrawn enrollment gets no report card")

	assert.Len(t, f.publisher.published, 3, "one GradesPublished per report card, after the commit")
	assert.Equal(t, 1, f.directory.calls, "names read once, before the transaction")
}

// T10 (QA ISSUE-005): a pupil with NO grade in the term's own period gets no
// report card and no notification — a group without subjects, or a pupil
// graded only in another period. The close says how many it published and how
// many it skipped; the preview (Progress) says the same beforehand.
func TestCloseSkipsPupilsWithoutGradesInThePeriod(t *testing.T) {
	f := newPeriodsFixture(t)
	*f.now = time.Date(2026, 12, 5, 15, 0, 0, 0, time.UTC) // period 1 ended
	f.capture(t, "enr-1", subjectID, period1, 9)
	f.capture(t, "enr-2", subjectID, period2, 8) // another period: not this one's

	preview, err := f.svc.Progress(context.Background(), firstTermID)
	require.NoError(t, err)
	assert.Equal(t, 1, preview.ReportCards, "the dialog can say it beforehand")
	assert.Equal(t, 2, preview.SkippedWithoutGrades)

	res, err := f.svc.Close(context.Background(), firstTermID, true, dirActor)
	require.NoError(t, err)
	assert.Equal(t, usecase.CloseResult{Status: domain.PeriodClosed, Version: 1, ReportCards: 1, SkippedWithoutGrades: 2}, res)
	require.Len(t, f.periods.cards, 1)
	assert.Equal(t, "enr-1", f.periods.cards[0].EnrollmentID())
	require.Len(t, f.publisher.published, 1, "nobody without grades is notified")
	assert.Equal(t, "stu-1", f.publisher.published[0].StudentID)
}

// A level where nobody has a grade still closes (force), publishing nothing.
func TestCloseWithNoGradesAtAllPublishesNothing(t *testing.T) {
	f := newPeriodsFixture(t)
	res, err := f.svc.Close(context.Background(), firstTermID, true, dirActor)
	require.NoError(t, err)
	assert.Equal(t, usecase.CloseResult{Status: domain.PeriodClosed, Version: 1, ReportCards: 0, SkippedWithoutGrades: 3}, res)
	assert.True(t, f.grades.closures[firstTermID].IsClosed())
	assert.Empty(t, f.periods.cards)
	assert.Empty(t, f.periods.outbox)
	assert.Empty(t, f.publisher.published)
}

func TestCloseRefusals(t *testing.T) {
	tests := []struct {
		name     string
		term     string
		prepare  func(t *testing.T, f periodsFixture)
		wantKind glerrors.Kind
		wantCode domain.RefusalCode
		wantMsg  string
	}{
		{name: "unknown term", term: "ghost", wantKind: glerrors.KindNotFound},
		{name: "no grade has the term's period", term: "term-fourth", wantKind: glerrors.KindConflict,
			wantCode: domain.CodePeriodNotConfigured, wantMsg: "Los grupos 1° · A, 2° · A no tienen el periodo 4 configurado"},
		{name: "a grade lacks the term's period", term: firstTermID, prepare: func(_ *testing.T, f periodsFixture) {
			f.evals.byGrade[gradeTwo] = trimesters(gradeTwo)[1:]
		}, wantKind: glerrors.KindConflict, wantCode: domain.CodePeriodNotConfigured, wantMsg: "El grupo 2° · A no tiene el periodo 1 configurado"},
		{name: "level without numeric grades", term: "term-kinder", wantKind: glerrors.KindConflict, wantCode: domain.CodeLevelNotGraded},
		{name: "already closed", term: firstTermID, prepare: func(t *testing.T, f periodsFixture) {
			_, err := f.svc.Close(context.Background(), firstTermID, true, dirActor)
			require.NoError(t, err)
			f.publisher.published = nil
		}, wantKind: glerrors.KindConflict, wantCode: domain.CodeAlreadyClosed},
		{name: "names unavailable aborts before writing", term: firstTermID, prepare: func(_ *testing.T, f periodsFixture) {
			f.directory.err = glerrors.New(glerrors.KindInternal, "students down")
		}, wantKind: glerrors.KindInternal},
		{name: "catalog unavailable aborts before writing", term: firstTermID, prepare: func(_ *testing.T, f periodsFixture) {
			f.catalog.catalogErr = glerrors.New(glerrors.KindInternal, "config down")
		}, wantKind: glerrors.KindInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPeriodsFixture(t)
			if tt.prepare != nil {
				tt.prepare(t, f)
			}
			cardsBefore := len(f.periods.cards)
			_, err := f.svc.Close(context.Background(), tt.term, true, dirActor)
			require.Error(t, err)
			assert.Equal(t, tt.wantKind, glerrors.KindOf(err), "err = %v", err)
			assert.Equal(t, tt.wantCode, domain.RefusalCodeOf(err))
			if tt.wantMsg != "" {
				assert.Equal(t, tt.wantMsg, glerrors.SafeMessage(err))
			}
			assert.Len(t, f.periods.cards, cardsBefore, "a refused close issues nothing")
			assert.Empty(t, f.publisher.published)
		})
	}

	f := newPeriodsFixture(t)
	_, err := f.svc.Close(context.Background(), firstTermID, true, domain.Actor{})
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err), "the closing director is required")
}

// Closing a complete period needs no force.
func TestCloseCompletePeriodWithoutForce(t *testing.T) {
	f := newPeriodsFixture(t)
	f.captureAll(t)
	res, err := f.svc.Close(context.Background(), firstTermID, false, dirActor)
	require.NoError(t, err)
	assert.Equal(t, 3, res.ReportCards)
}

// Reopen → correct → close again issues version 2, which supersedes version
// 1; parents see only the latest.
func TestReopenAndCloseAgainIssuesVersion2(t *testing.T) {
	f := newPeriodsFixture(t)
	f.captureAll(t)
	_, err := f.svc.Close(context.Background(), firstTermID, false, dirActor)
	require.NoError(t, err)

	status, err := f.svc.Reopen(context.Background(), firstTermID, director)
	require.NoError(t, err)
	assert.Equal(t, domain.PeriodOpen, status)
	assert.False(t, f.grades.closures[firstTermID].IsClosed())
	latest, err := f.svc.ReportCards(context.Background(), ports.ReportCardFilter{TermID: firstTermID})
	require.NoError(t, err)
	assert.Len(t, latest, 3, "reopening keeps the report cards published")

	f.capture(t, "enr-1", subjectID, period1, 10)
	res, err := f.svc.Close(context.Background(), firstTermID, false, dirActor)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Version)

	latest, err = f.svc.ReportCards(context.Background(), ports.ReportCardFilter{StudentID: "stu-1"})
	require.NoError(t, err)
	require.Len(t, latest, 1)
	assert.Equal(t, 2, latest[0].Version())
	assert.Len(t, f.periods.cards, 6)
	superseded := 0
	for _, rc := range f.periods.cards {
		if rc.Status() == domain.ReportCardSuperseded {
			superseded++
			assert.Equal(t, 1, rc.Version())
		}
	}
	assert.Equal(t, 3, superseded)
	versions := []int{}
	for _, e := range f.publisher.published {
		versions = append(versions, e.Version)
	}
	sort.Ints(versions)
	assert.Equal(t, []int{1, 1, 1, 2}, versions, "v1 notifies everyone; v2 only the pupil whose card changed")
	assert.Equal(t, "stu-1", f.publisher.published[3].StudentID)
}

// Owner decision: a re-close whose cards match the previous ones still issues
// the new version (parents see the latest), but notifies nobody.
func TestReCloseWithoutChangesNotifiesNobody(t *testing.T) {
	f := newPeriodsFixture(t)
	f.captureAll(t)
	_, err := f.svc.Close(context.Background(), firstTermID, false, dirActor)
	require.NoError(t, err)
	require.Len(t, f.publisher.published, 3, "v1 always notifies")
	_, err = f.svc.Reopen(context.Background(), firstTermID, director)
	require.NoError(t, err)

	res, err := f.svc.Close(context.Background(), firstTermID, false, dirActor)
	require.NoError(t, err)

	assert.Equal(t, usecase.CloseResult{Status: domain.PeriodClosed, Version: 2, ReportCards: 3}, res)
	latest, err := f.svc.ReportCards(context.Background(), ports.ReportCardFilter{TermID: firstTermID})
	require.NoError(t, err)
	for _, rc := range latest {
		assert.Equal(t, 2, rc.Version(), "unchanged pupils still get the new version")
	}
	assert.Len(t, f.publisher.published, 3, "nothing changed: no GradesPublished for v2")
	assert.Len(t, f.periods.outbox, 3, "and nothing queued")
}

// A pupil with no previous card (enrolled since the last close) is news.
func TestReCloseNotifiesAPupilWithoutAPreviousCard(t *testing.T) {
	f := newPeriodsFixture(t)
	f.captureAll(t)
	_, err := f.svc.Close(context.Background(), firstTermID, false, dirActor)
	require.NoError(t, err)
	_, err = f.svc.Reopen(context.Background(), firstTermID, director)
	require.NoError(t, err)
	kept := f.periods.cards[:0]
	for _, rc := range f.periods.cards {
		if rc.StudentID() != "stu-3" {
			kept = append(kept, rc)
		}
	}
	f.periods.cards = kept
	f.publisher.published = nil

	_, err = f.svc.Close(context.Background(), firstTermID, false, dirActor)
	require.NoError(t, err)

	require.Len(t, f.publisher.published, 1)
	assert.Equal(t, "stu-3", f.publisher.published[0].StudentID)
}

func TestReopenRefusals(t *testing.T) {
	f := newPeriodsFixture(t)
	_, err := f.svc.Reopen(context.Background(), firstTermID, director)
	assert.Equal(t, domain.CodeAlreadyOpen, domain.RefusalCodeOf(err), "a period never closed is open")
	_, err = f.svc.Reopen(context.Background(), "ghost", director)
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}

// GradesPublished rides a transactional outbox: the events are queued in the
// close's transaction (one per report card) and drained right after the
// commit. A broker that is down never fails the close — the events stay
// queued, and the next drain (the sweep) publishes them.
func TestCloseQueuesGradesPublishedInItsTransaction(t *testing.T) {
	f := newPeriodsFixture(t)
	f.publisher.err = errors.New("broker down")
	f.captureAll(t)
	res, err := f.svc.Close(context.Background(), firstTermID, true, dirActor)
	require.NoError(t, err)
	assert.Equal(t, 3, res.ReportCards)
	assert.True(t, f.grades.closures[firstTermID].IsClosed())
	require.Len(t, f.periods.outbox, 3, "one queued event per report card, with the commit")
	assert.Equal(t, 3, f.periods.pending(), "nothing went out: still queued")
	assert.Equal(t, ports.OutboxGradesPublished, f.periods.outbox[0].event.Type)

	f.publisher.err = nil
	n, err := f.svc.DrainOutbox(context.Background(), 0)
	require.NoError(t, err)
	assert.Equal(t, 3, n, "the sweep publishes what the broker refused")
	assert.Zero(t, f.periods.pending())
	require.Len(t, f.publisher.published, 3)
	ev := f.publisher.published[0]
	assert.Equal(t, testTenant, ev.TenantID)
	assert.Equal(t, firstTermID, ev.TermID)
	assert.Equal(t, "1.er trimestre", ev.TermName)
	assert.Equal(t, cycleID, ev.CycleID)
	assert.Equal(t, 1, ev.Version)
	assert.NotEmpty(t, ev.ReportCardID)
	assert.NotEmpty(t, ev.EnrollmentID)
	assert.NotEmpty(t, ev.StudentID)
}

// A refused close queues nothing (the events roll back with it).
func TestRefusedCloseQueuesNothing(t *testing.T) {
	f := newPeriodsFixture(t)
	_, err := f.svc.Close(context.Background(), firstTermID, false, dirActor)
	require.Error(t, err)
	assert.Empty(t, f.periods.outbox)
}

// The drain after a close runs detached from the request: a client that hung
// up (cancelled context) does not cancel the publication.
func TestCloseDrainsDetachedFromTheRequest(t *testing.T) {
	f := newPeriodsFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	f.periods.cancelAfterLock = cancel
	f.captureAll(t)
	_, err := f.svc.Close(ctx, firstTermID, true, dirActor)
	require.NoError(t, err)
	assert.NoError(t, f.publisher.ctxErr, "the drain must not inherit the request's cancellation")
	assert.Len(t, f.publisher.published, 3)
	assert.Zero(t, f.periods.pending())
}

// --- lists -------------------------------------------------------------------

func TestListPeriods(t *testing.T) {
	f := newPeriodsFixture(t)
	f.captureAll(t)
	_, err := f.svc.Close(context.Background(), firstTermID, false, dirActor)
	require.NoError(t, err)
	f.grades.batchCalls = 0

	got, err := f.svc.ListPeriods(context.Background(), cycleID)
	require.NoError(t, err)
	require.Len(t, got, 3, "T10 ISSUE-019: a level without numeric grades has no period to close")
	byTerm := map[string]usecase.Period{}
	for _, p := range got {
		byTerm[p.Term.ID] = p
	}
	first := byTerm[firstTermID]
	assert.Equal(t, levelName, first.LevelName)
	assert.True(t, first.Closure.IsClosed())
	assert.Equal(t, 1, first.Closure.Version())
	assert.Equal(t, 5, first.Expected)
	assert.Equal(t, 5, first.Captured)
	assert.Equal(t, day(2026, 8, 24), first.StartsOn)
	assert.Equal(t, day(2026, 11, 30), first.EndsOn)
	second := byTerm[secondTermID]
	assert.False(t, second.Closure.IsClosed())
	assert.Equal(t, 5, second.Expected, "every pupil owes the period, reached or not")
	assert.Zero(t, second.Captured)
	assert.True(t, byTerm["term-fourth"].StartsOn.IsZero(), "no grade has a 4th period")
	_, listed := byTerm["term-kinder"]
	assert.False(t, listed, "Preescolar («Lleva calificación numérica» off) is not listed")
	assert.Equal(t, 1, f.grades.batchCalls, "every grade of the cycle in ONE read")
}

// A pupil's list is newest period first (term ordinal, not issue date): T1
// re-closed AFTER T2 still lists below it, and only its latest version shows.
func TestStudentReportCardsNewestTermFirst(t *testing.T) {
	f := newPeriodsFixture(t)
	f.captureAll(t)
	_, err := f.svc.Close(context.Background(), firstTermID, false, dirActor)
	require.NoError(t, err)
	f.capture(t, "enr-1", subjectID, period2, 8)
	_, err = f.svc.Close(context.Background(), secondTermID, true, dirActor)
	require.NoError(t, err)
	_, err = f.svc.Reopen(context.Background(), firstTermID, director)
	require.NoError(t, err)
	_, err = f.svc.Close(context.Background(), firstTermID, false, dirActor) // v2 of T1, issued last
	require.NoError(t, err)

	latest, err := f.svc.ReportCards(context.Background(), ports.ReportCardFilter{StudentID: "stu-1"})
	require.NoError(t, err)

	require.Len(t, latest, 2)
	assert.Equal(t, secondTermID, latest[0].TermID())
	assert.Equal(t, 2, latest[0].Header().TermOrdinal)
	assert.Equal(t, firstTermID, latest[1].TermID())
	assert.Equal(t, 2, latest[1].Version(), "only the latest published version of T1")
}

func TestReportCardsNeedAFilter(t *testing.T) {
	f := newPeriodsFixture(t)
	_, err := f.svc.ReportCards(context.Background(), ports.ReportCardFilter{})
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
	_, err = f.svc.ReportCard(context.Background(), "ghost")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}

// ClosedTerms lists the requested terms that are closed now, in one read; a
// reopened term is open.
func TestClosedTerms(t *testing.T) {
	f := newPeriodsFixture(t)
	_, err := f.svc.Close(context.Background(), firstTermID, true, dirActor)
	require.NoError(t, err)
	_, err = f.svc.Close(context.Background(), secondTermID, true, dirActor)
	require.NoError(t, err)
	_, err = f.svc.Reopen(context.Background(), secondTermID, director)
	require.NoError(t, err)

	got, err := f.svc.ClosedTerms(context.Background(), []string{secondTermID, firstTermID, "ghost"})
	require.NoError(t, err)
	assert.Equal(t, []string{firstTermID}, got)

	none, err := f.svc.ClosedTerms(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, none)
}
