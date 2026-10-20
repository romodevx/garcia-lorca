package grpcadapter_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
	grpcadapter "github.com/garcia-lorca/services/grades/internal/adapters/grpc"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
	"github.com/garcia-lorca/services/grades/internal/usecase"
)

// Through the RPCs a letters level captures "b", answers "B" in the sheet and
// in the enrollment's grades, and says it grades with letters.
func TestLetterGradeRPCsRoundTrip(t *testing.T) {
	catalog := newFakeCatalog()
	catalog.level = ports.LevelRef{ID: levelID, GradeLevelID: gradeID, GradingLetters: true}
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{enrollmentID: activeEnrollment(enrollmentID, "stu-1")}}
	svc := usecase.NewPeriodGradesService(usecase.PeriodGradesDeps{
		Grades: newFakePeriodGrades(), Assignments: baseAssignments(), Catalog: catalog, Periods: fakeEvalPeriods{}, Enrollments: enr,
		TenantID: testTenant, NewID: func() string { return "pg-1" }, Now: gradesNow,
	})
	h := grpcadapter.NewGradesHandler(nil, svc, &stubPeriods{}, usecase.NewHealthService("grades"))
	ctx := context.Background()

	put, err := h.UpsertPeriodGrade(ctx, connect.NewRequest(&gradesv1.UpsertPeriodGradeRequest{
		AssignmentId: assignmentID, PeriodId: period1, EnrollmentId: enrollmentID, Value: "b", CapturedBy: "u", SchoolWide: true,
	}))
	require.NoError(t, err)
	assert.Equal(t, "B", put.Msg.GetGrade().GetValue())

	sheet, err := h.GetPeriodSheet(ctx, connect.NewRequest(&gradesv1.GetPeriodSheetRequest{AssignmentId: assignmentID, PeriodId: period1}))
	require.NoError(t, err)
	assert.True(t, sheet.Msg.GetGradingLetters())
	require.Len(t, sheet.Msg.GetRows(), 1)
	assert.Equal(t, "B", sheet.Msg.GetRows()[0].GetValue())

	mine, err := h.GetEnrollmentPeriodGrades(ctx, connect.NewRequest(&gradesv1.GetEnrollmentPeriodGradesRequest{EnrollmentId: enrollmentID}))
	require.NoError(t, err)
	assert.True(t, mine.Msg.GetGradingLetters())
	require.Len(t, mine.Msg.GetGrades(), 1)
	assert.Equal(t, "B", mine.Msg.GetGrades()[0].GetValue())
}

// A letters report card says so on the wire, in full and in the list, with
// the letters as its period values and no average.
func TestLettersReportCardOnTheWire(t *testing.T) {
	g, err := domain.NewPeriodGrade(domain.PeriodGradeFields{
		ID: "g1", TenantID: testTenant, EnrollmentID: "enr-1", SubjectID: "len", PeriodID: "p1", CapturedBy: "u", Letter: domain.LetterA,
	})
	require.NoError(t, err)
	rc, err := domain.BuildReportCard(domain.ReportCardHeader{
		ID: "rc-1", TenantID: testTenant, TermID: "term-1", EnrollmentID: "enr-1", StudentID: "stu-1", Version: 1,
		Periods: []domain.ReportCardPeriod{{ID: "p1", Ordinal: 1, Name: "1.er trimestre"}}, Letters: true,
	}, []domain.ReportCardSubject{{ID: "len", Name: "Lenguajes"}}, []domain.PeriodGrade{g})
	require.NoError(t, err)
	h := grpcadapter.NewGradesHandler(nil, nil, &stubPeriods{card: rc, cards: []domain.ReportCard{rc}}, usecase.NewHealthService("grades"))

	full, err := h.GetReportCard(context.Background(), connect.NewRequest(&gradesv1.GetReportCardRequest{Id: "rc-1"}))
	require.NoError(t, err)
	assert.True(t, full.Msg.GetReportCard().GetLetters())
	assert.Equal(t, "", full.Msg.GetReportCard().GetGeneralAverage())
	assert.Equal(t, map[string]string{"p1": "A"}, full.Msg.GetReportCard().GetLines()[0].GetPeriodValues())

	list, err := h.ListReportCards(context.Background(), connect.NewRequest(&gradesv1.ListReportCardsRequest{TermId: "term-1"}))
	require.NoError(t, err)
	assert.True(t, list.Msg.GetReportCards()[0].GetLetters())
}
