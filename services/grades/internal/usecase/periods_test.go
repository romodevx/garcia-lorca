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
	grades    *fakeMonthly
	cards     []domain.ReportCard
	lockCalls int
}

type fakePeriodTx struct {
	p        *fakePeriods
	termID   string
	closure  *domain.PeriodClosure
	issued   []domain.ReportCard
	lockedAt int
}

func (f *fakePeriods) WithPeriodLock(ctx context.Context, termID string, fn func(context.Context, ports.PeriodTx) error) error {
	f.lockCalls++
	tx := &fakePeriodTx{p: f, termID: termID}
	if err := fn(ctx, tx); err != nil {
		return err
	}
	if tx.closure != nil {
		f.grades.closures[termID] = *tx.closure
	}
	for _, rc := range tx.issued {
		for i, old := range f.cards {
			if old.TermID() == rc.TermID() && old.EnrollmentID() == rc.EnrollmentID() && old.Status() == domain.ReportCardPublished {
				f.cards[i] = old.WithStatus(domain.ReportCardSuperseded)
			}
		}
		f.cards = append(f.cards, rc)
	}
	return nil
}

func (t *fakePeriodTx) Closure(context.Context) (domain.PeriodClosure, bool, error) {
	c, ok := t.p.grades.closures[t.termID]
	return c, ok, nil
}

func (t *fakePeriodTx) MonthlyGrades(_ context.Context, ids []string, months []domain.Month) ([]domain.MonthlyGrade, error) {
	return t.p.grades.gradesIn(ids, months), nil
}

func (t *fakePeriodTx) SaveClosure(_ context.Context, c domain.PeriodClosure) error {
	t.closure = &c
	return nil
}

func (t *fakePeriodTx) IssueReportCards(_ context.Context, cards []domain.ReportCard) error {
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
	published []domain.ReportCard
	err       error
}

func (f *fakePublisher) PublishReportCardIssued(_ context.Context, rc domain.ReportCard) error {
	f.published = append(f.published, rc)
	return f.err
}

// --- fixture -----------------------------------------------------------------

const (
	group2ID  = "grp-2"
	subject2  = "subj-2"
	director  = "user-dir"
	levelName = "Primaria"
)

// Two groups of the primary level: grp-1 (enr-1, enr-2; Matemáticas by
// teacher-1, Español by teacher-2) and grp-2 (enr-3; Matemáticas by
// teacher-3). The first trimester is Sep–Nov; "now" is late November, so
// every month counts: expected = 2×2×3 + 1×1×3 = 15.
type periodsFixture struct {
	svc       *usecase.PeriodsService
	grades    *fakeMonthly
	periods   *fakePeriods
	directory *fakeDirectory
	publisher *fakePublisher
	catalog   *fakeCatalog
	asg       *fakeAssignments
	now       *time.Time
}

var periodsNow = time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)

func newPeriodsFixture(t *testing.T) periodsFixture {
	t.Helper()
	scale := mustScale(t, 500, 1000, 600, 1)
	catalog := baseCatalog(t)
	catalog.levelCatalogs = map[string]ports.LevelCatalog{levelPrimaria: {
		LevelName: levelName, GradingEnabled: true, Scale: scale,
		Groups: []ports.GroupRef{{ID: groupID, Label: "1° · A"}, {ID: group2ID, Label: "2° · A"}},
		Assignments: []ports.AssignmentRef{
			{ID: assignmentID, GroupID: groupID, SubjectID: subjectID, TeacherID: teacherID, CycleID: cycleID},
			{ID: "asg-2", GroupID: groupID, SubjectID: subject2, TeacherID: "teacher-2", CycleID: cycleID},
			{ID: "asg-3", GroupID: group2ID, SubjectID: subjectID, TeacherID: "teacher-3", CycleID: cycleID},
		},
		SubjectNames: map[string]string{subjectID: "Matemáticas", subject2: "Español"},
		TeacherNames: map[string]string{teacherID: "Gabriela Rojas", "teacher-2": "Luis Pérez", "teacher-3": "Ana Ruiz"},
	}, levelPreescolar: {LevelName: "Preescolar", GradingEnabled: false}}
	asg := baseAssignments()
	asg.terms = map[string]ports.TermRef{
		firstTermID:   {ID: firstTermID, CycleID: cycleID, LevelID: levelPrimaria, Name: "1.er trimestre", Ordinal: 1, Months: months(t, "2026-09", "2026-10", "2026-11")},
		secondTermID:  {ID: secondTermID, CycleID: cycleID, LevelID: levelPrimaria, Name: "2.º trimestre", Ordinal: 2, Months: months(t, "2026-12", "2027-01", "2027-02")},
		"term-empty":  {ID: "term-empty", CycleID: cycleID, LevelID: levelPrimaria, Name: "3.er trimestre", Ordinal: 3},
		"term-kinder": {ID: "term-kinder", CycleID: cycleID, LevelID: levelPreescolar, Name: "1.er trimestre", Ordinal: 1, Months: months(t, "2026-09")},
	}
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{
		"enr-1":    {ID: "enr-1", StudentID: "stu-1", GroupID: groupID, CycleID: cycleID, Status: "active"},
		"enr-2":    {ID: "enr-2", StudentID: "stu-2", GroupID: groupID, CycleID: cycleID, Status: "active"},
		"enr-3":    {ID: "enr-3", StudentID: "stu-3", GroupID: group2ID, CycleID: cycleID, Status: "active"},
		"enr-gone": {ID: "enr-gone", StudentID: "stu-4", GroupID: groupID, CycleID: cycleID, Status: "withdrawn"},
		"enr-kid":  {ID: "enr-kid", StudentID: "stu-5", GroupID: kinderGroupID, CycleID: cycleID, Status: "active"},
	}}
	grades := newFakeMonthly()
	periods := &fakePeriods{grades: grades}
	directory := &fakeDirectory{names: map[string]ports.StudentRef{
		"stu-1": {ID: "stu-1", Name: "Ana López", EnrollmentNumber: "2026-00001"},
		"stu-2": {ID: "stu-2", Name: "Beto Díaz", EnrollmentNumber: "2026-00002"},
		"stu-3": {ID: "stu-3", Name: "Carla Ñúñez", EnrollmentNumber: "2026-00003"},
	}}
	publisher := &fakePublisher{}
	seq := 0
	now := periodsNow
	svc := usecase.NewPeriodsService(usecase.PeriodsDeps{
		Periods: periods, Grades: grades, Terms: asg, Catalog: catalog, Enrollments: enr,
		Students: directory, Publisher: publisher, TenantID: testTenant,
		NewID: func() string { seq++; return "rc-" + string(rune('a'+seq-1)) },
		Now:   func() time.Time { return now },
	})
	return periodsFixture{svc: svc, grades: grades, periods: periods, directory: directory, publisher: publisher, catalog: catalog, asg: asg, now: &now}
}

// capture stores a grade directly (the monthly knot is tested elsewhere).
func (f periodsFixture) capture(t *testing.T, enrollment, subject, month string, hundredths int) {
	t.Helper()
	m, err := domain.ParseMonth(month)
	require.NoError(t, err)
	g, err := domain.NewMonthlyGrade(domain.MonthlyGradeFields{
		ID: "mg-" + enrollment + subject + month, TenantID: testTenant, EnrollmentID: enrollment, SubjectID: subject,
		TeacherID: teacherID, CapturedBy: capturer, Month: m, Value: domain.GradeValueFromHundredths(hundredths),
	})
	require.NoError(t, err)
	f.grades.rows[monthlyKey(enrollment, subject, m)] = g
}

// captureAll fills every expected cell of the first trimester.
func (f periodsFixture) captureAll(t *testing.T) {
	for _, month := range []string{"2026-09", "2026-10", "2026-11"} {
		f.capture(t, "enr-1", subjectID, month, 900)
		f.capture(t, "enr-1", subject2, month, 800)
		f.capture(t, "enr-2", subjectID, month, 700)
		f.capture(t, "enr-2", subject2, month, 1000)
		f.capture(t, "enr-3", subjectID, month, 850)
	}
}

var dirActor = domain.Actor{ID: director, Name: "Directora Ruiz"}

// --- progress ----------------------------------------------------------------

func TestPeriodProgressCountsAndGroupsWhatIsMissing(t *testing.T) {
	f := newPeriodsFixture(t)
	f.capture(t, "enr-1", subjectID, "2026-09", 900)
	f.capture(t, "enr-1", subjectID, "2026-10", 900)
	f.capture(t, "enr-2", subjectID, "2026-09", 900)
	f.capture(t, "enr-3", subjectID, "2026-11", 900)
	f.capture(t, "enr-gone", subjectID, "2026-09", 900)   // withdrawn: not expected, not counted
	f.capture(t, "enr-1", subjectID, "2026-12", 900)      // next term
	f.capture(t, "enr-1", "subj-dropped", "2026-09", 900) // no longer taught: not expected

	got, err := f.svc.Progress(context.Background(), firstTermID)
	require.NoError(t, err)
	assert.Equal(t, 15, got.Expected)
	assert.Equal(t, 4, got.Captured)
	assert.Equal(t, []string{"2026-09", "2026-10", "2026-11"}, monthStrings(got.Months))
	assert.Equal(t, 1, f.grades.batchCalls, "every grade of the level in ONE read")
	assert.Equal(t, 1, f.catalog.catalogCalls)
	assert.Equal(t, []usecase.MissingGrades{
		{GroupID: groupID, GroupLabel: "1° · A", SubjectID: subject2, SubjectName: "Español", TeacherName: "Luis Pérez", Missing: 6},
		{GroupID: groupID, GroupLabel: "1° · A", SubjectID: subjectID, SubjectName: "Matemáticas", TeacherName: "Gabriela Rojas", Missing: 3},
		{GroupID: group2ID, GroupLabel: "2° · A", SubjectID: subjectID, SubjectName: "Matemáticas", TeacherName: "Ana Ruiz", Missing: 2},
	}, got.Missing)
}

// Months after the current one are not owed yet.
func TestPeriodProgressOnlyCountsMonthsUpToNow(t *testing.T) {
	f := newPeriodsFixture(t)
	*f.now = time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	got, err := f.svc.Progress(context.Background(), firstTermID)
	require.NoError(t, err)
	assert.Equal(t, 10, got.Expected, "September and October only")

	empty, err := f.svc.Progress(context.Background(), "term-empty")
	require.NoError(t, err)
	assert.Zero(t, empty.Expected)

	_, err = f.svc.Progress(context.Background(), "ghost")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}

// --- close -------------------------------------------------------------------

func TestCloseRefusesMissingGradesWithoutForce(t *testing.T) {
	f := newPeriodsFixture(t)
	f.captureAll(t)
	delete(f.grades.rows, monthlyKey("enr-3", subjectID, months(t, "2026-11")[0]))
	delete(f.grades.rows, monthlyKey("enr-2", subject2, months(t, "2026-10")[0]))

	_, err := f.svc.Close(context.Background(), firstTermID, false, dirActor)
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Equal(t, domain.CodeMissingGrades, domain.RefusalCodeOf(err))
	assert.Equal(t, "Faltan 2 calificaciones por capturar", glerrors.SafeMessage(err))
	assert.Empty(t, f.grades.closures, "nothing closed")
	assert.Empty(t, f.periods.cards, "no report card issued")
	assert.Empty(t, f.publisher.published)

	delete(f.grades.rows, monthlyKey("enr-1", subject2, months(t, "2026-10")[0]))
	f.capture(t, "enr-3", subjectID, "2026-11", 900)
	f.capture(t, "enr-2", subject2, "2026-10", 900)
	_, err = f.svc.Close(context.Background(), firstTermID, false, dirActor)
	assert.Equal(t, "Falta 1 calificación por capturar", glerrors.SafeMessage(err))
}

// Closing with force issues a report card per ACTIVE enrollment of the level,
// version 1, with the names frozen, then announces each one.
func TestCloseWithForceIssuesReportCards(t *testing.T) {
	f := newPeriodsFixture(t)
	f.capture(t, "enr-1", subjectID, "2026-09", 840)
	f.capture(t, "enr-1", subjectID, "2026-10", 850)
	f.capture(t, "enr-1", subject2, "2026-11", 800)

	res, err := f.svc.Close(context.Background(), firstTermID, true, dirActor)
	require.NoError(t, err)
	assert.Equal(t, usecase.CloseResult{Status: domain.PeriodClosed, Version: 1, ReportCards: 3}, res)

	closure := f.grades.closures[firstTermID]
	assert.True(t, closure.IsClosed())
	assert.Equal(t, "Directora Ruiz", closure.ClosedByName())
	assert.Equal(t, periodsNow, closure.ClosedAt())
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
	assert.Equal(t, "1.er trimestre", h.TermName)
	assert.Equal(t, director, h.IssuedBy)
	assert.Equal(t, periodsNow, h.IssuedAt)
	assert.Equal(t, 600, h.Scale.PassingHundredths())
	require.Len(t, ana.Lines(), 2)
	assert.Equal(t, "Español", ana.Lines()[0].SubjectName, "subjects in Spanish name order")
	assert.Equal(t, "8.0", ana.Lines()[0].TermAverage.String())
	assert.Equal(t, "Matemáticas", ana.Lines()[1].SubjectName)
	assert.Equal(t, "8.5", ana.Lines()[1].TermAverage.String(), "8.45 half-up")
	assert.Equal(t, "8.3", ana.GeneralAverage().String(), "mean of 8.0 and 8.5 = 8.25 → 8.3")
	assert.Len(t, byEnrollment["enr-3"].Lines(), 1, "grp-2 is taught one subject")
	_, withdrawn := byEnrollment["enr-gone"]
	assert.False(t, withdrawn, "a withdrawn enrollment gets no report card")

	assert.Len(t, f.publisher.published, 3, "one GradesPublished per report card, after the commit")
	assert.Equal(t, 1, f.directory.calls, "names read once, before the transaction")
}

func TestCloseRefusals(t *testing.T) {
	tests := []struct {
		name     string
		term     string
		prepare  func(t *testing.T, f periodsFixture)
		wantKind glerrors.Kind
		wantCode domain.RefusalCode
	}{
		{name: "unknown term", term: "ghost", wantKind: glerrors.KindNotFound},
		{name: "term without months", term: "term-empty", wantKind: glerrors.KindConflict, wantCode: domain.CodeTermWithoutMonths},
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

	f.capture(t, "enr-1", subjectID, "2026-10", 1000)
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
	for _, rc := range f.publisher.published {
		versions = append(versions, rc.Version())
	}
	sort.Ints(versions)
	assert.Equal(t, []int{1, 1, 1, 2, 2, 2}, versions)
}

func TestReopenRefusals(t *testing.T) {
	f := newPeriodsFixture(t)
	_, err := f.svc.Reopen(context.Background(), firstTermID, director)
	assert.Equal(t, domain.CodeAlreadyOpen, domain.RefusalCodeOf(err), "a period never closed is open")
	_, err = f.svc.Reopen(context.Background(), "ghost", director)
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}

// The announcement is best-effort: the closure and the report cards are
// already committed, so a failed publish never fails the close.
func TestClosePublishFailureDoesNotFailTheClose(t *testing.T) {
	f := newPeriodsFixture(t)
	f.publisher.err = errors.New("broker down")
	res, err := f.svc.Close(context.Background(), firstTermID, true, dirActor)
	require.NoError(t, err)
	assert.Equal(t, 3, res.ReportCards)
	assert.True(t, f.grades.closures[firstTermID].IsClosed())
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
	require.Len(t, got, 4)
	byTerm := map[string]usecase.Period{}
	for _, p := range got {
		byTerm[p.Term.ID] = p
	}
	first := byTerm[firstTermID]
	assert.Equal(t, levelName, first.LevelName)
	assert.True(t, first.Closure.IsClosed())
	assert.Equal(t, 1, first.Closure.Version())
	assert.Equal(t, 15, first.Expected)
	assert.Equal(t, 15, first.Captured)
	second := byTerm[secondTermID]
	assert.False(t, second.Closure.IsClosed())
	assert.Zero(t, second.Expected, "December has not arrived")
	assert.Equal(t, "Preescolar", byTerm["term-kinder"].LevelName)
	assert.Zero(t, byTerm["term-kinder"].Expected, "a level without numeric grades owes nothing")
	assert.Equal(t, 1, f.grades.batchCalls, "every grade of the cycle in ONE read")
}

func TestReportCardsNeedAFilter(t *testing.T) {
	f := newPeriodsFixture(t)
	_, err := f.svc.ReportCards(context.Background(), ports.ReportCardFilter{})
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
	_, err = f.svc.ReportCard(context.Background(), "ghost")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}
