package grpcadapter_test

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	grpcadapter "github.com/garcia-lorca/services/grades/internal/adapters/grpc"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
	"github.com/garcia-lorca/services/grades/internal/usecase"
)

// stubPeriods answers the period use case with canned values and records what
// it was asked.
type stubPeriods struct {
	periods  []usecase.Period
	progress usecase.PeriodProgress
	closeRes usecase.CloseResult
	cards    []domain.ReportCard
	card     domain.ReportCard
	err      error

	closeForce  bool
	closeActor  domain.Actor
	reopenActor string
	filter      ports.ReportCardFilter
}

func (s *stubPeriods) ListPeriods(context.Context, string) ([]usecase.Period, error) {
	return s.periods, s.err
}

func (s *stubPeriods) Progress(context.Context, string) (usecase.PeriodProgress, error) {
	return s.progress, s.err
}

func (s *stubPeriods) Close(_ context.Context, _ string, force bool, actor domain.Actor) (usecase.CloseResult, error) {
	s.closeForce, s.closeActor = force, actor
	return s.closeRes, s.err
}

func (s *stubPeriods) Reopen(_ context.Context, _ string, actorID string) (domain.PeriodStatus, error) {
	s.reopenActor = actorID
	return domain.PeriodOpen, s.err
}

func (s *stubPeriods) ReportCards(_ context.Context, f ports.ReportCardFilter) ([]domain.ReportCard, error) {
	s.filter = f
	return s.cards, s.err
}

func (s *stubPeriods) ReportCard(context.Context, string) (domain.ReportCard, error) {
	return s.card, s.err
}

func periodsHandler(p *stubPeriods) *grpcadapter.GradesHandler {
	return grpcadapter.NewGradesHandler(nil, nil, p, usecase.NewHealthService("grades"))
}

var issuedAt = time.Date(2026, 12, 1, 10, 0, 0, 0, time.UTC)

func sampleCard(t *testing.T) domain.ReportCard {
	t.Helper()
	sep, err := domain.ParseMonth("2026-09")
	require.NoError(t, err)
	oct, err := domain.ParseMonth("2026-10")
	require.NoError(t, err)
	scale, err := domain.NewGradingScale(500, 1000, 600, 1)
	require.NoError(t, err)
	g, err := domain.NewMonthlyGrade(domain.MonthlyGradeFields{ID: "g1", TenantID: testTenant, EnrollmentID: enrollmentID, SubjectID: subjectID,
		TeacherID: teacherID, CapturedBy: "u", Month: sep, Value: domain.GradeValueFromHundredths(950)})
	require.NoError(t, err)
	rc, err := domain.BuildReportCard(domain.ReportCardHeader{
		ID: "rc-1", TenantID: testTenant, TermID: termID, CycleID: cycleID, LevelID: levelID, GroupID: groupID,
		EnrollmentID: enrollmentID, StudentID: "stu-1", Version: 2, IssuedAt: issuedAt, IssuedBy: "dir",
		StudentName: "Ana López", EnrollmentNumber: "2026-00001", GroupLabel: "1° · A", LevelName: "Primaria",
		CycleName: "2026-2027", TermName: "1.er trimestre", Months: []domain.Month{sep, oct}, Scale: scale,
	}, []domain.ReportCardSubject{{ID: subjectID, Name: "Matemáticas"}, {ID: "subj-2", Name: "Español"}}, []domain.MonthlyGrade{g})
	require.NoError(t, err)
	return rc
}

func TestListPeriodsRPC(t *testing.T) {
	closed, err := domain.NewOpenPeriod(termID, cycleID, levelID).Close(domain.Actor{ID: "dir", Name: "Directora"}, issuedAt)
	require.NoError(t, err)
	sep, err := domain.ParseMonth("2026-09")
	require.NoError(t, err)
	p := &stubPeriods{periods: []usecase.Period{{
		Term:      ports.TermRef{ID: termID, CycleID: cycleID, LevelID: levelID, Name: "1.er trimestre", Ordinal: 1, Months: []domain.Month{sep}},
		LevelName: "Primaria", Closure: closed, Expected: 10, Captured: 7,
	}, {
		Term: ports.TermRef{ID: "term-2", Name: "2.º trimestre", Ordinal: 2}, Closure: domain.NewOpenPeriod("term-2", cycleID, levelID),
	}}}
	res, err := periodsHandler(p).ListPeriods(context.Background(), connect.NewRequest(&gradesv1.ListPeriodsRequest{CycleId: cycleID}))
	require.NoError(t, err)
	require.Len(t, res.Msg.GetPeriods(), 2)
	first := res.Msg.GetPeriods()[0]
	assert.Equal(t, termID, first.GetTermId())
	assert.Equal(t, "1.er trimestre", first.GetTermName())
	assert.Equal(t, int32(1), first.GetOrdinal())
	assert.Equal(t, "Primaria", first.GetLevelName())
	assert.Equal(t, []string{"2026-09"}, first.GetMonths())
	assert.Equal(t, "closed", first.GetStatus())
	assert.Equal(t, int32(1), first.GetVersion())
	assert.Equal(t, "Directora", first.GetClosedByName())
	assert.Equal(t, issuedAt, first.GetClosedAt().AsTime())
	assert.Equal(t, &gradesv1.PeriodProgress{Captured: 7, Expected: 10}, first.GetProgress())
	second := res.Msg.GetPeriods()[1]
	assert.Equal(t, "open", second.GetStatus())
	assert.Nil(t, second.GetClosedAt(), "an open, never-closed period has no closed_at")
}

func TestGetPeriodProgressRPC(t *testing.T) {
	sep, err := domain.ParseMonth("2026-09")
	require.NoError(t, err)
	p := &stubPeriods{progress: usecase.PeriodProgress{TermID: termID, Months: []domain.Month{sep}, Expected: 4, Captured: 1,
		Missing: []usecase.MissingGrades{{GroupID: groupID, GroupLabel: "1° · A", SubjectID: subjectID, SubjectName: "Matemáticas", TeacherName: "Gabriela", Missing: 3}}}}
	res, err := periodsHandler(p).GetPeriodProgress(context.Background(), connect.NewRequest(&gradesv1.GetPeriodProgressRequest{TermId: termID}))
	require.NoError(t, err)
	m := res.Msg
	assert.Equal(t, int32(4), m.GetExpected())
	assert.Equal(t, int32(1), m.GetCaptured())
	assert.Equal(t, []string{"2026-09"}, m.GetMonths())
	require.Len(t, m.GetMissing(), 1)
	assert.Equal(t, &gradesv1.MissingGrades{GroupId: groupID, GroupLabel: "1° · A", SubjectId: subjectID, SubjectName: "Matemáticas", TeacherName: "Gabriela", MissingCount: 3}, m.GetMissing()[0])
}

// A refusal to close is AlreadyExists (409 at the edge) carrying its code.
func TestClosePeriodRPC(t *testing.T) {
	p := &stubPeriods{closeRes: usecase.CloseResult{Status: domain.PeriodClosed, Version: 2, ReportCards: 30}}
	res, err := periodsHandler(p).ClosePeriod(context.Background(), connect.NewRequest(&gradesv1.ClosePeriodRequest{
		TermId: termID, Force: true, ActorId: "dir", ActorName: "Directora",
	}))
	require.NoError(t, err)
	assert.Equal(t, &gradesv1.ClosePeriodResponse{Status: "closed", Version: 2, ReportCards: 30}, res.Msg)
	assert.True(t, p.closeForce)
	assert.Equal(t, domain.Actor{ID: "dir", Name: "Directora"}, p.closeActor)

	p.err = domain.Conflict(domain.CodeMissingGrades, "Faltan 3 calificaciones por capturar")
	_, err = periodsHandler(p).ClosePeriod(context.Background(), connect.NewRequest(&gradesv1.ClosePeriodRequest{TermId: termID, ActorId: "dir"}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(err))
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, "MISSING_GRADES", ce.Meta().Get(grpcadapter.ErrorCodeMeta))
	assert.Equal(t, "Faltan 3 calificaciones por capturar", ce.Message())
}

func TestReopenPeriodRPC(t *testing.T) {
	p := &stubPeriods{}
	res, err := periodsHandler(p).ReopenPeriod(context.Background(), connect.NewRequest(&gradesv1.ReopenPeriodRequest{TermId: termID, ActorId: "dir"}))
	require.NoError(t, err)
	assert.Equal(t, "open", res.Msg.GetStatus())
	assert.Equal(t, "dir", p.reopenActor)

	p.err = domain.Conflict(domain.CodeAlreadyOpen, "El periodo ya está abierto")
	_, err = periodsHandler(p).ReopenPeriod(context.Background(), connect.NewRequest(&gradesv1.ReopenPeriodRequest{TermId: termID}))
	assert.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(err))
}

func TestReportCardRPCs(t *testing.T) {
	rc := sampleCard(t)
	p := &stubPeriods{cards: []domain.ReportCard{rc}, card: rc}
	list, err := periodsHandler(p).ListReportCards(context.Background(), connect.NewRequest(&gradesv1.ListReportCardsRequest{TermId: termID, GroupId: groupID, StudentId: "stu-1"}))
	require.NoError(t, err)
	assert.Equal(t, ports.ReportCardFilter{TermID: termID, GroupID: groupID, StudentID: "stu-1"}, p.filter)
	require.Len(t, list.Msg.GetReportCards(), 1)
	sum := list.Msg.GetReportCards()[0]
	assert.Equal(t, "rc-1", sum.GetId())
	assert.Equal(t, "Ana López", sum.GetStudentName())
	assert.Equal(t, "1° · A", sum.GetGroupLabel())
	assert.Equal(t, "1.er trimestre", sum.GetTermName())
	assert.Equal(t, int32(2), sum.GetVersion())
	assert.Equal(t, "9.5", sum.GetGeneralAverage())
	assert.Equal(t, enrollmentID, sum.GetEnrollmentId())
	assert.Equal(t, issuedAt, sum.GetIssuedAt().AsTime())

	one, err := periodsHandler(p).GetReportCard(context.Background(), connect.NewRequest(&gradesv1.GetReportCardRequest{Id: "rc-1"}))
	require.NoError(t, err)
	card := one.Msg.GetReportCard()
	assert.Equal(t, "published", card.GetStatus())
	assert.Equal(t, "2026-00001", card.GetEnrollmentNumber())
	assert.Equal(t, "Primaria", card.GetLevelName())
	assert.Equal(t, "2026-2027", card.GetCycleName())
	assert.Equal(t, []string{"2026-09", "2026-10"}, card.GetMonths())
	assert.Equal(t, &gradesv1.GradingScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600}, card.GetScale())
	require.Len(t, card.GetLines(), 2)
	assert.Equal(t, map[string]string{"2026-09": "9.5"}, card.GetLines()[0].GetMonthValues())
	assert.Equal(t, "9.5", card.GetLines()[0].GetTermAverage())
	assert.Equal(t, "", card.GetLines()[1].GetTermAverage(), "no grade ⇒ empty, never 0")
	assert.Equal(t, int32(2), card.GetLines()[1].GetPosition())

	p.err = glerrors.ErrNotFound
	_, err = periodsHandler(p).GetReportCard(context.Background(), connect.NewRequest(&gradesv1.GetReportCardRequest{Id: "nope"}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

// The sheet carries the months a closed period locks.
func TestMonthlySheetCarriesClosedMonths(t *testing.T) {
	sep, err := domain.ParseMonth("2026-09")
	require.NoError(t, err)
	asg := baseAssignments()
	asg.terms["term-t1"] = ports.TermRef{ID: "term-t1", CycleID: cycleID, LevelID: levelID, Months: []domain.Month{sep}}
	repo := newFakeMonthly()
	closed, err := domain.NewOpenPeriod("term-t1", cycleID, levelID).Close(domain.Actor{ID: "dir"}, issuedAt)
	require.NoError(t, err)
	repo.closures["term-t1"] = closed
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{enrollmentID: activeEnrollment(enrollmentID, "stu-1")}}
	monthly := usecase.NewMonthlyGradesService(repo, asg, newFakeCatalog(), enr, testTenant, func() string { return "id" }, monthlyNow)
	h := grpcadapter.NewGradesHandler(nil, monthly, &stubPeriods{}, usecase.NewHealthService("grades"))

	res, err := h.GetMonthlySheet(context.Background(), connect.NewRequest(&gradesv1.GetMonthlySheetRequest{AssignmentId: assignmentID, Month: "2026-09"}))
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-09"}, res.Msg.GetClosedMonths())
	assert.Equal(t, []string{"2026-09"}, res.Msg.GetMonths(), "only mapped months")
	assert.False(t, res.Msg.GetEditable())
	assert.Equal(t, "El periodo está cerrado", res.Msg.GetReadOnlyReason())

	_, err = h.UpsertMonthlyGrade(context.Background(), connect.NewRequest(&gradesv1.UpsertMonthlyGradeRequest{
		AssignmentId: assignmentID, Month: "2026-09", EnrollmentId: enrollmentID, Value: "9", CapturedBy: "u", SchoolWide: true,
	}))
	assert.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(err))
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, "MONTH_LOCKED", ce.Meta().Get(grpcadapter.ErrorCodeMeta))
}
