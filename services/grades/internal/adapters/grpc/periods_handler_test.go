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

	closeForce   bool
	closeActor   domain.Actor
	reopenActor  domain.Actor
	reopenReason string
	filter       ports.ReportCardFilter
	closed       []string
	closedAsked  []string
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

func (s *stubPeriods) Reopen(_ context.Context, _ string, actor domain.Actor, reason string) (domain.PeriodStatus, error) {
	s.reopenActor, s.reopenReason = actor, reason
	return domain.PeriodOpen, s.err
}

func (s *stubPeriods) ReportCards(_ context.Context, f ports.ReportCardFilter) ([]domain.ReportCard, error) {
	s.filter = f
	return s.cards, s.err
}

func (s *stubPeriods) ReportCard(context.Context, string) (domain.ReportCard, error) {
	return s.card, s.err
}

func (s *stubPeriods) ClosedTerms(_ context.Context, ids []string) ([]string, error) {
	s.closedAsked = ids
	return s.closed, s.err
}

func periodsHandler(p *stubPeriods) *grpcadapter.GradesHandler {
	return grpcadapter.NewGradesHandler(nil, nil, p, usecase.NewHealthService("grades"))
}

var issuedAt = time.Date(2026, 12, 1, 10, 0, 0, 0, time.UTC)

func sampleCard(t *testing.T) domain.ReportCard {
	t.Helper()
	scale, err := domain.NewGradingScale(500, 1000, 600, 1)
	require.NoError(t, err)
	var grades []domain.PeriodGrade
	for _, c := range []struct {
		period string
		whole  int
	}{{"p1", 9}, {"p2", 8}, {"p3", 9}} {
		g, err := domain.NewPeriodGrade(domain.PeriodGradeFields{ID: "g-" + c.period, TenantID: testTenant, EnrollmentID: enrollmentID,
			SubjectID: subjectID, PeriodID: c.period, CapturedBy: "u", Value: domain.GradeValueFromHundredths(c.whole * 100)})
		require.NoError(t, err)
		grades = append(grades, g)
	}
	rc, err := domain.BuildReportCard(domain.ReportCardHeader{
		ID: "rc-1", TenantID: testTenant, TermID: termID, CycleID: cycleID, LevelID: levelID, GroupID: groupID,
		EnrollmentID: enrollmentID, StudentID: "stu-1", Version: 2, IssuedAt: issuedAt, IssuedBy: "dir",
		StudentName: "Ana López", EnrollmentNumber: "2026-00001", GroupLabel: "1° · A", LevelName: "Primaria",
		CycleName: "2026-2027", TermName: "3.er trimestre", TermOrdinal: 3, Scale: scale,
		Periods: []domain.ReportCardPeriod{
			{ID: "p1", Ordinal: 1, Name: "1.er trimestre", StartsOn: issuedAt},
			{ID: "p2", Ordinal: 2, Name: "2.º trimestre", StartsOn: issuedAt},
			{ID: "p3", Ordinal: 3, Name: "3.er trimestre", StartsOn: issuedAt},
		},
	}, []domain.ReportCardSubject{{ID: subjectID, Name: "Matemáticas"}, {ID: "subj-2", Name: "Español"}}, grades)
	require.NoError(t, err)
	return rc
}

func TestListPeriodsRPC(t *testing.T) {
	closed, err := domain.NewOpenPeriod(termID, cycleID, levelID).Close(domain.Actor{ID: "dir", Name: "Directora"}, issuedAt, nil)
	require.NoError(t, err)
	p := &stubPeriods{periods: []usecase.Period{{
		Term:      ports.TermRef{ID: termID, CycleID: cycleID, LevelID: levelID, Name: "1.er trimestre", Ordinal: 1},
		LevelName: "Primaria", Closure: closed, Expected: 10, Captured: 7,
		StartsOn: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC), EndsOn: time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC),
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
	assert.Empty(t, first.GetMonths(), "a term has no months since T15")
	assert.Equal(t, "2026-08-24", first.GetStartsOn())
	assert.Equal(t, "2026-11-30", first.GetEndsOn())
	assert.Equal(t, "closed", first.GetStatus())
	assert.Equal(t, int32(1), first.GetVersion())
	assert.Equal(t, "Directora", first.GetClosedByName())
	assert.Equal(t, issuedAt, first.GetClosedAt().AsTime())
	assert.Equal(t, &gradesv1.PeriodProgress{Captured: 7, Expected: 10}, first.GetProgress())
	second := res.Msg.GetPeriods()[1]
	assert.Equal(t, "open", second.GetStatus())
	assert.Nil(t, second.GetClosedAt(), "an open, never-closed period has no closed_at")
	assert.Empty(t, second.GetStartsOn(), "no grade has the period")
}

func TestGetPeriodProgressRPC(t *testing.T) {
	p := &stubPeriods{progress: usecase.PeriodProgress{TermID: termID, Unfinished: true, EndsOn: time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC),
		GroupsWithoutPeriod: []string{"2° · A"}, Expected: 4, Captured: 1, ReportCards: 3, SkippedWithoutGrades: 1, WithdrawnReportCards: 2,
		Missing: []usecase.MissingGrades{{GroupID: groupID, GroupLabel: "1° · A", SubjectID: subjectID, SubjectName: "Matemáticas", TeacherName: "Gabriela", Missing: 3}}}}
	res, err := periodsHandler(p).GetPeriodProgress(context.Background(), connect.NewRequest(&gradesv1.GetPeriodProgressRequest{TermId: termID}))
	require.NoError(t, err)
	m := res.Msg
	assert.Equal(t, int32(4), m.GetExpected())
	assert.Equal(t, int32(1), m.GetCaptured())
	assert.Equal(t, int32(3), m.GetReportCards())
	assert.Equal(t, int32(1), m.GetSkippedWithoutGrades())
	assert.Equal(t, int32(2), m.GetWithdrawnReportCards())
	assert.True(t, m.GetUnfinished())
	assert.Equal(t, "2026-11-30", m.GetEndsOn())
	assert.Equal(t, []string{"2° · A"}, m.GetGroupsWithoutPeriod())
	assert.Empty(t, m.GetMonths())
	require.Len(t, m.GetMissing(), 1)
	assert.Equal(t, &gradesv1.MissingGrades{GroupId: groupID, GroupLabel: "1° · A", SubjectId: subjectID, SubjectName: "Matemáticas", TeacherName: "Gabriela", MissingCount: 3}, m.GetMissing()[0])
}

// A refusal to close is AlreadyExists (409 at the edge) carrying its code.
func TestClosePeriodRPC(t *testing.T) {
	p := &stubPeriods{closeRes: usecase.CloseResult{Status: domain.PeriodClosed, Version: 2, ReportCards: 30, SkippedWithoutGrades: 2, WithdrawnReportCards: 1}}
	res, err := periodsHandler(p).ClosePeriod(context.Background(), connect.NewRequest(&gradesv1.ClosePeriodRequest{
		TermId: termID, Force: true, ActorId: "dir", ActorName: "Directora",
	}))
	require.NoError(t, err)
	assert.Equal(t, &gradesv1.ClosePeriodResponse{Status: "closed", Version: 2, ReportCards: 30, SkippedWithoutGrades: 2, WithdrawnReportCards: 1}, res.Msg)
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
	res, err := periodsHandler(p).ReopenPeriod(context.Background(), connect.NewRequest(&gradesv1.ReopenPeriodRequest{TermId: termID, ActorId: "dir", ActorName: "Directora", Reason: "Corrección"}))
	require.NoError(t, err)
	assert.Equal(t, "open", res.Msg.GetStatus())
	assert.Equal(t, domain.Actor{ID: "dir", Name: "Directora"}, p.reopenActor, "CC-308: the actor reaches the use case")
	assert.Equal(t, "Corrección", p.reopenReason)

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
	assert.Equal(t, "3.er trimestre", sum.GetTermName())
	assert.Equal(t, int32(2), sum.GetVersion())
	assert.Equal(t, "8.66", sum.GetGeneralAverage(), "exact mean truncated to 2 decimals")
	assert.Equal(t, enrollmentID, sum.GetEnrollmentId())
	assert.Equal(t, issuedAt, sum.GetIssuedAt().AsTime())
	assert.Equal(t, int32(3), sum.GetTermOrdinal())
	assert.Equal(t, cycleID, sum.GetCycleId())
	assert.Equal(t, "2026-2027", sum.GetCycleName())

	one, err := periodsHandler(p).GetReportCard(context.Background(), connect.NewRequest(&gradesv1.GetReportCardRequest{Id: "rc-1"}))
	require.NoError(t, err)
	card := one.Msg.GetReportCard()
	assert.Equal(t, "published", card.GetStatus())
	assert.Equal(t, "2026-00001", card.GetEnrollmentNumber())
	assert.Equal(t, "Primaria", card.GetLevelName())
	assert.Equal(t, "2026-2027", card.GetCycleName())
	assert.Equal(t, int32(3), card.GetTermOrdinal())
	assert.Empty(t, card.GetMonths())
	require.Len(t, card.GetPeriods(), 3)
	assert.Equal(t, &gradesv1.ReportCardPeriod{Id: "p2", Ordinal: 2, Name: "2.º trimestre"}, card.GetPeriods()[1])
	assert.Equal(t, &gradesv1.GradingScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600}, card.GetScale())
	require.Len(t, card.GetLines(), 2)
	assert.Equal(t, map[string]string{"p1": "9", "p2": "8", "p3": "9"}, card.GetLines()[0].GetPeriodValues())
	assert.Empty(t, card.GetLines()[0].GetMonthValues())
	assert.Equal(t, "8.66", card.GetLines()[0].GetTermAverage())
	assert.Equal(t, "", card.GetLines()[1].GetTermAverage(), "no grade ⇒ empty, never 0")
	assert.Equal(t, int32(2), card.GetLines()[1].GetPosition())

	p.err = glerrors.ErrNotFound
	_, err = periodsHandler(p).GetReportCard(context.Background(), connect.NewRequest(&gradesv1.GetReportCardRequest{Id: "nope"}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

// ClosedTerms answers which requested terms are closed, in one call.
func TestClosedTermsRPC(t *testing.T) {
	p := &stubPeriods{closed: []string{"term-t1"}}
	res, err := periodsHandler(p).ClosedTerms(context.Background(), connect.NewRequest(&gradesv1.ClosedTermsRequest{TermIds: []string{"term-t1", "term-t2"}}))
	require.NoError(t, err)
	assert.Equal(t, []string{"term-t1"}, res.Msg.GetClosedTermIds())
	assert.Equal(t, []string{"term-t1", "term-t2"}, p.closedAsked)

	p = &stubPeriods{}
	res, err = periodsHandler(p).ClosedTerms(context.Background(), connect.NewRequest(&gradesv1.ClosedTermsRequest{}))
	require.NoError(t, err)
	assert.NotNil(t, res.Msg.GetClosedTermIds(), "an empty answer is a list")

	p.err = glerrors.New(glerrors.KindInternal, "db down")
	_, err = periodsHandler(p).ClosedTerms(context.Background(), connect.NewRequest(&gradesv1.ClosedTermsRequest{TermIds: []string{"x"}}))
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
}
