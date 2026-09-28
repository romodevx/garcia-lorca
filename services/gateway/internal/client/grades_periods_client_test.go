package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/grades/v1/gradesv1connect"
	"github.com/garcia-lorca/services/gateway/internal/client"
)

var periodClock = time.Date(2026, 12, 1, 10, 0, 0, 0, time.UTC)

// stubPeriodsGrades answers the T7 RPCs and records every request, so the
// test proves each field crosses the wire in BOTH directions.
type stubPeriodsGrades struct {
	gradesv1connect.UnimplementedGradesServiceHandler
	listReq    *gradesv1.ListPeriodsRequest
	progReq    *gradesv1.GetPeriodProgressRequest
	closeReq   *gradesv1.ClosePeriodRequest
	reopenReq  *gradesv1.ReopenPeriodRequest
	cardsReq   *gradesv1.ListReportCardsRequest
	cardReq    *gradesv1.GetReportCardRequest
	sheetClose []string
	closedReq  *gradesv1.ClosedTermsRequest
}

func (s *stubPeriodsGrades) ListPeriods(_ context.Context, r *connect.Request[gradesv1.ListPeriodsRequest]) (*connect.Response[gradesv1.ListPeriodsResponse], error) {
	s.listReq = r.Msg
	return connect.NewResponse(&gradesv1.ListPeriodsResponse{Periods: []*gradesv1.Period{{
		TermId: "term-1", TermName: "1.er trimestre", Ordinal: 1, LevelId: "lvl-1", LevelName: "Primaria",
		Months: []string{"2026-09", "2026-10"}, Status: "closed", ClosedAt: timestamppb.New(periodClock),
		ClosedByName: "Directora", Version: 2, Progress: &gradesv1.PeriodProgress{Captured: 7, Expected: 9},
	}}}), nil
}

func (s *stubPeriodsGrades) GetPeriodProgress(_ context.Context, r *connect.Request[gradesv1.GetPeriodProgressRequest]) (*connect.Response[gradesv1.GetPeriodProgressResponse], error) {
	s.progReq = r.Msg
	return connect.NewResponse(&gradesv1.GetPeriodProgressResponse{TermId: "term-1", Months: []string{"2026-09"}, UnfinishedMonths: []string{"2026-09"}, Expected: 9, Captured: 7,
		Missing: []*gradesv1.MissingGrades{{GroupId: "grp-1", GroupLabel: "1° · A", SubjectId: "sub-1", SubjectName: "Matemáticas", TeacherName: "Gabriela", MissingCount: 2}}}), nil
}

func (s *stubPeriodsGrades) ClosePeriod(_ context.Context, r *connect.Request[gradesv1.ClosePeriodRequest]) (*connect.Response[gradesv1.ClosePeriodResponse], error) {
	s.closeReq = r.Msg
	return connect.NewResponse(&gradesv1.ClosePeriodResponse{Status: "closed", Version: 2, ReportCards: 31}), nil
}

func (s *stubPeriodsGrades) ReopenPeriod(_ context.Context, r *connect.Request[gradesv1.ReopenPeriodRequest]) (*connect.Response[gradesv1.ReopenPeriodResponse], error) {
	s.reopenReq = r.Msg
	return connect.NewResponse(&gradesv1.ReopenPeriodResponse{Status: "open"}), nil
}

func (s *stubPeriodsGrades) ListReportCards(_ context.Context, r *connect.Request[gradesv1.ListReportCardsRequest]) (*connect.Response[gradesv1.ListReportCardsResponse], error) {
	s.cardsReq = r.Msg
	return connect.NewResponse(&gradesv1.ListReportCardsResponse{ReportCards: []*gradesv1.ReportCardSummary{{
		Id: "rc-1", TermId: "term-1", TermName: "1.er trimestre", StudentId: "stu-1", StudentName: "Ana López",
		GroupId: "grp-1", GroupLabel: "1° · A", Version: 2, IssuedAt: timestamppb.New(periodClock), GeneralAverage: "8.5", EnrollmentId: "enr-1",
		TermOrdinal: 1, CycleId: "cyc-1", CycleName: "2026-2027",
	}}}), nil
}

func (s *stubPeriodsGrades) GetReportCard(_ context.Context, r *connect.Request[gradesv1.GetReportCardRequest]) (*connect.Response[gradesv1.GetReportCardResponse], error) {
	s.cardReq = r.Msg
	return connect.NewResponse(&gradesv1.GetReportCardResponse{ReportCard: &gradesv1.ReportCard{
		Id: "rc-1", TermId: "term-1", CycleId: "cyc-1", LevelId: "lvl-1", GroupId: "grp-1", EnrollmentId: "enr-1", StudentId: "stu-1",
		Version: 2, Status: "published", IssuedAt: timestamppb.New(periodClock), IssuedBy: "usr-dir", StudentName: "Ana López",
		EnrollmentNumber: "2026-00001", GroupLabel: "1° · A", LevelName: "Primaria", CycleName: "2026-2027", TermName: "1.er trimestre",
		Months: []string{"2026-09", "2026-10"}, GeneralAverage: "8.5", TermOrdinal: 1,
		Scale: &gradesv1.GradingScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600},
		Lines: []*gradesv1.ReportCardLine{{SubjectId: "sub-1", SubjectName: "Matemáticas", Position: 1, MonthValues: map[string]string{"2026-09": "8.4"}, TermAverage: "8.4"}},
	}}), nil
}

func (s *stubPeriodsGrades) GetMonthlySheet(_ context.Context, _ *connect.Request[gradesv1.GetMonthlySheetRequest]) (*connect.Response[gradesv1.GetMonthlySheetResponse], error) {
	return connect.NewResponse(&gradesv1.GetMonthlySheetResponse{AssignmentId: "asg-1", Month: "2026-09", ClosedMonths: s.sheetClose, UnmappedMonths: []string{"2026-08"}}), nil
}

func (s *stubPeriodsGrades) ClosedTerms(_ context.Context, r *connect.Request[gradesv1.ClosedTermsRequest]) (*connect.Response[gradesv1.ClosedTermsResponse], error) {
	s.closedReq = r.Msg
	return connect.NewResponse(&gradesv1.ClosedTermsResponse{ClosedTermIds: []string{"term-1"}}), nil
}

func newPeriodsGradesClient(t *testing.T, stub *stubPeriodsGrades) *client.GradesClient {
	t.Helper()
	mux := http.NewServeMux()
	path, h := gradesv1connect.NewGradesServiceHandler(stub)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return client.NewGradesClient(srv.URL, srv.Client())
}

func TestGradesClientPeriodsRoundTrip(t *testing.T) {
	stub := &stubPeriodsGrades{sheetClose: []string{"2026-09"}}
	c := newPeriodsGradesClient(t, stub)
	ctx := context.Background()

	periods, err := c.ListPeriods(ctx, "cyc-1")
	require.NoError(t, err)
	assert.Equal(t, "cyc-1", stub.listReq.GetCycleId())
	assert.Equal(t, []client.Period{{
		TermID: "term-1", TermName: "1.er trimestre", Ordinal: 1, LevelID: "lvl-1", LevelName: "Primaria",
		Months: []string{"2026-09", "2026-10"}, Status: "closed", ClosedAt: periodClock, ClosedByName: "Directora",
		Version: 2, Captured: 7, Expected: 9,
	}}, periods)

	prog, err := c.GetPeriodProgress(ctx, "term-1")
	require.NoError(t, err)
	assert.Equal(t, "term-1", stub.progReq.GetTermId())
	assert.Equal(t, client.PeriodProgress{TermID: "term-1", Months: []string{"2026-09"}, UnfinishedMonths: []string{"2026-09"}, Expected: 9, Captured: 7,
		Missing: []client.MissingGrades{{GroupID: "grp-1", GroupLabel: "1° · A", SubjectID: "sub-1", SubjectName: "Matemáticas", TeacherName: "Gabriela", MissingCount: 2}}}, prog)

	closed, err := c.ClosePeriod(ctx, client.ClosePeriodInput{TermID: "term-1", Force: true, ActorID: "usr-dir", ActorName: "Directora"})
	require.NoError(t, err)
	assert.True(t, proto.Equal(&gradesv1.ClosePeriodRequest{TermId: "term-1", Force: true, ActorId: "usr-dir", ActorName: "Directora"}, stub.closeReq), "request = %v", stub.closeReq)
	assert.Equal(t, client.ClosePeriodResult{Status: "closed", Version: 2, ReportCards: 31}, closed)

	status, err := c.ReopenPeriod(ctx, "term-1", "usr-dir")
	require.NoError(t, err)
	assert.Equal(t, "open", status)
	assert.True(t, proto.Equal(&gradesv1.ReopenPeriodRequest{TermId: "term-1", ActorId: "usr-dir"}, stub.reopenReq), "request = %v", stub.reopenReq)

	cards, err := c.ListReportCards(ctx, client.ReportCardFilter{TermID: "term-1", GroupID: "grp-1", StudentID: "stu-1"})
	require.NoError(t, err)
	assert.True(t, proto.Equal(&gradesv1.ListReportCardsRequest{TermId: "term-1", GroupId: "grp-1", StudentId: "stu-1"}, stub.cardsReq), "request = %v", stub.cardsReq)
	assert.Equal(t, []client.ReportCardSummary{{ID: "rc-1", TermID: "term-1", TermName: "1.er trimestre", StudentID: "stu-1", StudentName: "Ana López",
		GroupID: "grp-1", GroupLabel: "1° · A", Version: 2, IssuedAt: periodClock, GeneralAverage: "8.5", EnrollmentID: "enr-1",
		TermOrdinal: 1, CycleID: "cyc-1", CycleName: "2026-2027"}}, cards)

	card, err := c.GetReportCard(ctx, "rc-1")
	require.NoError(t, err)
	assert.Equal(t, "rc-1", stub.cardReq.GetId())
	assert.Equal(t, client.ReportCard{
		ID: "rc-1", TermID: "term-1", CycleID: "cyc-1", LevelID: "lvl-1", GroupID: "grp-1", EnrollmentID: "enr-1", StudentID: "stu-1",
		Version: 2, Status: "published", IssuedAt: periodClock, IssuedBy: "usr-dir", StudentName: "Ana López",
		EnrollmentNumber: "2026-00001", GroupLabel: "1° · A", LevelName: "Primaria", CycleName: "2026-2027", TermName: "1.er trimestre",
		Months: []string{"2026-09", "2026-10"}, GeneralAverage: "8.5", TermOrdinal: 1,
		Scale: client.GradeScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600},
		Lines: []client.ReportCardLine{{SubjectID: "sub-1", SubjectName: "Matemáticas", Position: 1, MonthValues: map[string]string{"2026-09": "8.4"}, TermAverage: "8.4"}},
	}, card)

	sheet, err := c.GetMonthlySheet(ctx, client.MonthlySheetQuery{AssignmentID: "asg-1"})
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-09"}, sheet.ClosedMonths)
	assert.Equal(t, []string{"2026-08"}, sheet.UnmappedMonths)

	closedTerms, err := c.ClosedTerms(ctx, []string{"term-1", "term-2"})
	require.NoError(t, err)
	assert.Equal(t, []string{"term-1", "term-2"}, stub.closedReq.GetTermIds())
	assert.Equal(t, []string{"term-1"}, closedTerms)
}
