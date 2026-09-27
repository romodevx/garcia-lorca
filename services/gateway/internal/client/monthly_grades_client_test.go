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
	"google.golang.org/protobuf/types/known/timestamppb"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/grades/v1/gradesv1connect"
	"github.com/garcia-lorca/services/gateway/internal/client"
)

// --- config: grading scale -----------------------------------------------------

type stubGrading struct {
	configv1connect.UnimplementedConfigServiceHandler
	got *configv1.SetLevelGradingScaleRequest
}

func (s *stubGrading) GetLevel(_ context.Context, req *connect.Request[configv1.GetLevelRequest]) (*connect.Response[configv1.GetLevelResponse], error) {
	return connect.NewResponse(&configv1.GetLevelResponse{Level: &configv1.Level{Id: req.Msg.GetId(), Name: "Preescolar",
		Grading: &configv1.GradingScale{Enabled: false, MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600}}}), nil
}

func (s *stubGrading) SetLevelGradingScale(_ context.Context, req *connect.Request[configv1.SetLevelGradingScaleRequest]) (*connect.Response[configv1.SetLevelGradingScaleResponse], error) {
	s.got = req.Msg
	return connect.NewResponse(&configv1.SetLevelGradingScaleResponse{Level: &configv1.Level{Id: req.Msg.GetId(), Grading: req.Msg.GetGrading()}}), nil
}

func (s *stubGrading) GetTeachingAssignment(_ context.Context, req *connect.Request[configv1.GetTeachingAssignmentRequest]) (*connect.Response[configv1.GetTeachingAssignmentResponse], error) {
	if req.Msg.GetId() != "asg-1" {
		return nil, connect.NewError(connect.CodeNotFound, nil)
	}
	return connect.NewResponse(&configv1.GetTeachingAssignmentResponse{TeachingAssignment: &configv1.TeachingAssignment{
		Id: "asg-1", CycleId: "cyc-1", GroupId: "grp-1", SubjectId: "sub-1", TeacherId: "tch-1"}}), nil
}

func TestConfigClientGradingScaleRoundTrip(t *testing.T) {
	stub := &stubGrading{}
	mux := http.NewServeMux()
	path, h := configv1connect.NewConfigServiceHandler(stub)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := client.NewConfigClient(srv.URL, srv.Client())

	lvl, err := c.GetLevel(context.Background(), "lvl-1")
	require.NoError(t, err)
	assert.Equal(t, client.LevelGrading{Enabled: false, Scale: client.GradeScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600}}, lvl.Grading)

	want := client.LevelGrading{Enabled: true, Scale: client.GradeScale{MinHundredths: 0, MaxHundredths: 10000, Decimals: 2, PassingHundredths: 7000}}
	got, err := c.SetLevelGradingScale(context.Background(), "lvl-1", want)
	require.NoError(t, err)
	assert.Equal(t, want, got.Grading, "the response level carries the new scale back")
	assert.Equal(t, "lvl-1", stub.got.GetId())
	assert.Equal(t, int32(7000), stub.got.GetGrading().GetPassingHundredths())
	assert.True(t, stub.got.GetGrading().GetEnabled())

	asg, err := c.GetTeachingAssignment(context.Background(), "asg-1")
	require.NoError(t, err)
	assert.Equal(t, client.TeachingAssignment{ID: "asg-1", CycleID: "cyc-1", GroupID: "grp-1", SubjectID: "sub-1", TeacherID: "tch-1"}, asg)
	_, err = c.GetTeachingAssignment(context.Background(), "ghost")
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

// --- grades: monthly -----------------------------------------------------------

var updated = time.Date(2026, 9, 3, 15, 4, 5, 0, time.UTC)

type stubMonthly struct {
	gradesv1connect.UnimplementedGradesServiceHandler
	gotUpsert *gradesv1.UpsertMonthlyGradeRequest
	gotClear  *gradesv1.ClearMonthlyGradeRequest
	gotSheet  *gradesv1.GetMonthlySheetRequest
}

func (s *stubMonthly) UpsertMonthlyGrade(_ context.Context, req *connect.Request[gradesv1.UpsertMonthlyGradeRequest]) (*connect.Response[gradesv1.UpsertMonthlyGradeResponse], error) {
	s.gotUpsert = req.Msg
	return connect.NewResponse(&gradesv1.UpsertMonthlyGradeResponse{Grade: &gradesv1.MonthlyGrade{
		EnrollmentId: req.Msg.GetEnrollmentId(), SubjectId: "sub-1", Month: req.Msg.GetMonth(), Value: "9.5", UpdatedAt: timestamppb.New(updated)}}), nil
}

func (s *stubMonthly) ClearMonthlyGrade(_ context.Context, req *connect.Request[gradesv1.ClearMonthlyGradeRequest]) (*connect.Response[gradesv1.ClearMonthlyGradeResponse], error) {
	s.gotClear = req.Msg
	return connect.NewResponse(&gradesv1.ClearMonthlyGradeResponse{}), nil
}

func (s *stubMonthly) GetMonthlySheet(_ context.Context, req *connect.Request[gradesv1.GetMonthlySheetRequest]) (*connect.Response[gradesv1.GetMonthlySheetResponse], error) {
	s.gotSheet = req.Msg
	return connect.NewResponse(&gradesv1.GetMonthlySheetResponse{
		AssignmentId: "asg-1", Month: "2026-09", Months: []string{"2026-08", "2026-09"},
		Scale:          &gradesv1.GradingScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600},
		GradingEnabled: true, Editable: false, ReadOnlyReason: "Periodo cerrado",
		Rows: []*gradesv1.MonthlySheetRow{
			{EnrollmentId: "enr-1", StudentId: "stu-1", Value: "8", UpdatedAt: timestamppb.New(updated)},
			{EnrollmentId: "enr-2", StudentId: "stu-2"},
		},
	}), nil
}

func (s *stubMonthly) GetEnrollmentMonthlyGrades(_ context.Context, req *connect.Request[gradesv1.GetEnrollmentMonthlyGradesRequest]) (*connect.Response[gradesv1.GetEnrollmentMonthlyGradesResponse], error) {
	return connect.NewResponse(&gradesv1.GetEnrollmentMonthlyGradesResponse{
		CycleId: "cyc-1", CycleName: "2026-2027", Months: []string{"2026-09"},
		Scale:          &gradesv1.GradingScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600},
		GradingEnabled: true, SubjectIds: []string{"sub-1"},
		Grades: []*gradesv1.MonthlyGrade{{EnrollmentId: req.Msg.GetEnrollmentId(), SubjectId: "sub-1", Month: "2026-09", Value: "9.5"}},
	}), nil
}

func TestGradesClientMonthlyRoundTrip(t *testing.T) {
	stub := &stubMonthly{}
	mux := http.NewServeMux()
	path, h := gradesv1connect.NewGradesServiceHandler(stub)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := client.NewGradesClient(srv.URL, srv.Client())
	ctx := context.Background()

	g, err := c.UpsertMonthlyGrade(ctx, client.MonthlyGradeInput{AssignmentID: "asg-1", Month: "2026-09", EnrollmentID: "enr-1", Value: "9.5", CapturedBy: "usr-1"})
	require.NoError(t, err)
	assert.Equal(t, client.MonthlyGrade{EnrollmentID: "enr-1", SubjectID: "sub-1", Month: "2026-09", Value: "9.5", UpdatedAt: updated}, g)
	assert.Equal(t, "usr-1", stub.gotUpsert.GetCapturedBy())
	assert.Equal(t, "9.5", stub.gotUpsert.GetValue())
	assert.Equal(t, "asg-1", stub.gotUpsert.GetAssignmentId())

	require.NoError(t, c.ClearMonthlyGrade(ctx, "asg-1", "2026-09", "enr-1"))
	assert.Equal(t, "enr-1", stub.gotClear.GetEnrollmentId())
	assert.Equal(t, "2026-09", stub.gotClear.GetMonth())

	sheet, err := c.GetMonthlySheet(ctx, "asg-1", "")
	require.NoError(t, err)
	assert.Equal(t, "", stub.gotSheet.GetMonth())
	assert.Equal(t, client.MonthlySheet{
		AssignmentID: "asg-1", Month: "2026-09", Months: []string{"2026-08", "2026-09"},
		Scale:          client.GradeScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600},
		GradingEnabled: true, Editable: false, ReadOnlyReason: "Periodo cerrado",
		Rows: []client.MonthlySheetRow{
			{EnrollmentID: "enr-1", StudentID: "stu-1", Value: "8", UpdatedAt: updated},
			{EnrollmentID: "enr-2", StudentID: "stu-2"},
		},
	}, sheet)

	fam, err := c.GetEnrollmentMonthlyGrades(ctx, "enr-1")
	require.NoError(t, err)
	assert.Equal(t, client.EnrollmentMonthlyGrades{
		CycleID: "cyc-1", CycleName: "2026-2027", Months: []string{"2026-09"},
		Scale:          client.GradeScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600},
		GradingEnabled: true, SubjectIDs: []string{"sub-1"},
		Grades: []client.MonthlyGrade{{EnrollmentID: "enr-1", SubjectID: "sub-1", Month: "2026-09", Value: "9.5"}},
	}, fam)
}
