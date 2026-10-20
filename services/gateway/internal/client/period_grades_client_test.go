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
	got *configv1.UpdateLevelRequest
}

func (s *stubGrading) GetLevel(_ context.Context, req *connect.Request[configv1.GetLevelRequest]) (*connect.Response[configv1.GetLevelResponse], error) {
	return connect.NewResponse(&configv1.GetLevelResponse{Level: &configv1.Level{Id: req.Msg.GetId(), Name: "Preescolar",
		Grading: &configv1.GradingScale{Enabled: false, MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600}}}), nil
}

func (s *stubGrading) UpdateLevel(_ context.Context, req *connect.Request[configv1.UpdateLevelRequest]) (*connect.Response[configv1.UpdateLevelResponse], error) {
	s.got = req.Msg
	return connect.NewResponse(&configv1.UpdateLevelResponse{Level: &configv1.Level{Id: req.Msg.GetId(), Grading: req.Msg.GetGrading(),
		AttendanceMode: req.Msg.GetAttendanceMode()}}), nil
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
	got, err := c.UpdateLevel(context.Background(), "lvl-1", client.LevelPatch{Grading: &want})
	require.NoError(t, err)
	assert.Equal(t, want, got.Grading, "the response level carries the new scale back")
	assert.Equal(t, "lvl-1", stub.got.GetId())
	assert.Equal(t, int32(7000), stub.got.GetGrading().GetPassingHundredths())
	assert.True(t, stub.got.GetGrading().GetEnabled())
	assert.Nil(t, stub.got.AttendanceMode, "an absent mode stays absent on the wire")

	letters := client.LevelGrading{Letters: true, Scale: client.GradeScale{MinHundredths: 500, MaxHundredths: 1000, PassingHundredths: 600}}
	got, err = c.UpdateLevel(context.Background(), "lvl-1", client.LevelPatch{Grading: &letters})
	require.NoError(t, err)
	assert.True(t, stub.got.GetGrading().GetLetters(), "letters travel to config")
	assert.True(t, got.Grading.Letters, "and come back on the level")

	daily := "daily"
	got, err = c.UpdateLevel(context.Background(), "lvl-1", client.LevelPatch{AttendanceMode: &daily, Grading: &want})
	require.NoError(t, err)
	assert.Equal(t, configv1.AttendanceMode_ATTENDANCE_MODE_DAILY, stub.got.GetAttendanceMode(), "both fields travel in ONE request")
	assert.Equal(t, "daily", got.AttendanceMode)

	asg, err := c.GetTeachingAssignment(context.Background(), "asg-1")
	require.NoError(t, err)
	assert.Equal(t, client.TeachingAssignment{ID: "asg-1", CycleID: "cyc-1", GroupID: "grp-1", SubjectID: "sub-1", TeacherID: "tch-1"}, asg)
	_, err = c.GetTeachingAssignment(context.Background(), "ghost")
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

// --- grades: by evaluation period (T15) ----------------------------------------

var updated = time.Date(2026, 10, 3, 15, 4, 5, 0, time.UTC)

type stubPeriodGrades struct {
	gradesv1connect.UnimplementedGradesServiceHandler
	gotUpsert *gradesv1.UpsertPeriodGradeRequest
	gotClear  *gradesv1.ClearPeriodGradeRequest
	gotSheet  *gradesv1.GetPeriodSheetRequest
	gotCount  *gradesv1.CountScaleViolationsRequest
	gotWith   *gradesv1.PeriodsWithGradesRequest
	// letters makes the sheet and the enrollment's grades a letters level's.
	letters bool
}

func (s *stubPeriodGrades) CountScaleViolations(_ context.Context, req *connect.Request[gradesv1.CountScaleViolationsRequest]) (*connect.Response[gradesv1.CountScaleViolationsResponse], error) {
	s.gotCount = req.Msg
	return connect.NewResponse(&gradesv1.CountScaleViolationsResponse{Violations: 3}), nil
}

func (s *stubPeriodGrades) UpsertPeriodGrade(_ context.Context, req *connect.Request[gradesv1.UpsertPeriodGradeRequest]) (*connect.Response[gradesv1.UpsertPeriodGradeResponse], error) {
	s.gotUpsert = req.Msg
	return connect.NewResponse(&gradesv1.UpsertPeriodGradeResponse{Grade: &gradesv1.PeriodGrade{
		EnrollmentId: req.Msg.GetEnrollmentId(), SubjectId: "sub-1", PeriodId: req.Msg.GetPeriodId(), Value: "9",
		UpdatedAt: timestamppb.New(updated), CapturedByName: "Gabriela Rojas"}}), nil
}

func (s *stubPeriodGrades) ClearPeriodGrade(_ context.Context, req *connect.Request[gradesv1.ClearPeriodGradeRequest]) (*connect.Response[gradesv1.ClearPeriodGradeResponse], error) {
	s.gotClear = req.Msg
	return connect.NewResponse(&gradesv1.ClearPeriodGradeResponse{}), nil
}

func (s *stubPeriodGrades) PeriodsWithGrades(_ context.Context, req *connect.Request[gradesv1.PeriodsWithGradesRequest]) (*connect.Response[gradesv1.PeriodsWithGradesResponse], error) {
	s.gotWith = req.Msg
	return connect.NewResponse(&gradesv1.PeriodsWithGradesResponse{PeriodIds: []string{"per-1"}}), nil
}

var stubPeriodRefs = []*gradesv1.EvaluationPeriodRef{
	{Id: "per-1", Ordinal: 1, Name: "1.er trimestre", StartsOn: "2026-08-24", EndsOn: "2026-11-30"},
}

var wantPeriodRefs = []client.EvaluationPeriodRef{
	{ID: "per-1", Ordinal: 1, Name: "1.er trimestre", StartsOn: "2026-08-24", EndsOn: "2026-11-30"},
}

func (s *stubPeriodGrades) GetPeriodSheet(_ context.Context, req *connect.Request[gradesv1.GetPeriodSheetRequest]) (*connect.Response[gradesv1.GetPeriodSheetResponse], error) {
	s.gotSheet = req.Msg
	return connect.NewResponse(&gradesv1.GetPeriodSheetResponse{
		AssignmentId: "asg-1", PeriodId: "per-1", Periods: stubPeriodRefs, CurrentPeriodId: "per-1", Today: "2026-10-01",
		Scale:          &gradesv1.GradingScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600},
		GradingEnabled: !s.letters, GradingLetters: s.letters, Editable: false, ReadOnlyReason: "El periodo está cerrado", ClosedPeriodIds: []string{"per-1"},
		Rows: []*gradesv1.PeriodSheetRow{
			{EnrollmentId: "enr-1", StudentId: "stu-1", Value: "8", UpdatedAt: timestamppb.New(updated), Editable: true},
			{EnrollmentId: "enr-2", StudentId: "stu-2", ReadOnlyReason: "Ese periodo corresponde al grupo anterior del alumno"},
		},
	}), nil
}

func (s *stubPeriodGrades) GetEnrollmentPeriodGrades(_ context.Context, req *connect.Request[gradesv1.GetEnrollmentPeriodGradesRequest]) (*connect.Response[gradesv1.GetEnrollmentPeriodGradesResponse], error) {
	return connect.NewResponse(&gradesv1.GetEnrollmentPeriodGradesResponse{
		CycleId: "cyc-1", CycleName: "2026-2027", Periods: stubPeriodRefs,
		Scale:          &gradesv1.GradingScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600},
		GradingEnabled: !s.letters, GradingLetters: s.letters, SubjectIds: []string{"sub-1"},
		Grades: []*gradesv1.PeriodGrade{{EnrollmentId: req.Msg.GetEnrollmentId(), SubjectId: "sub-1", PeriodId: "per-1", Value: "9"}},
	}), nil
}

func TestGradesClientPeriodGradesRoundTrip(t *testing.T) {
	stub := &stubPeriodGrades{}
	mux := http.NewServeMux()
	path, h := gradesv1connect.NewGradesServiceHandler(stub)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := client.NewGradesClient(srv.URL, srv.Client())
	ctx := context.Background()

	g, err := c.UpsertPeriodGrade(ctx, client.PeriodGradeInput{AssignmentID: "asg-1", PeriodID: "per-1", EnrollmentID: "enr-1",
		Value: "9", CapturedBy: "usr-1", CapturedByName: "Gabriela Rojas", SchoolWide: true})
	require.NoError(t, err)
	assert.Equal(t, client.PeriodGrade{EnrollmentID: "enr-1", SubjectID: "sub-1", PeriodID: "per-1", Value: "9", UpdatedAt: updated}, g)
	assert.Equal(t, "usr-1", stub.gotUpsert.GetCapturedBy())
	assert.Equal(t, "Gabriela Rojas", stub.gotUpsert.GetCapturedByName())
	assert.Equal(t, "9", stub.gotUpsert.GetValue())
	assert.True(t, stub.gotUpsert.GetSchoolWide())

	require.NoError(t, c.ClearPeriodGrade(ctx, client.PeriodCell{AssignmentID: "asg-1", PeriodID: "per-1", EnrollmentID: "enr-1", SchoolWide: true}))
	assert.Equal(t, "enr-1", stub.gotClear.GetEnrollmentId())
	assert.Equal(t, "per-1", stub.gotClear.GetPeriodId())
	assert.True(t, stub.gotClear.GetSchoolWide())

	sheet, err := c.GetPeriodSheet(ctx, client.PeriodSheetQuery{AssignmentID: "asg-1"})
	require.NoError(t, err)
	assert.Equal(t, "", stub.gotSheet.GetPeriodId())
	assert.False(t, stub.gotSheet.GetSchoolWide())
	assert.Equal(t, client.PeriodSheet{
		AssignmentID: "asg-1", PeriodID: "per-1", Periods: wantPeriodRefs, CurrentPeriodID: "per-1", Today: "2026-10-01",
		Scale:          client.GradeScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600},
		GradingEnabled: true, Editable: false, ReadOnlyReason: "El periodo está cerrado", ClosedPeriodIDs: []string{"per-1"},
		Rows: []client.PeriodSheetRow{
			{EnrollmentID: "enr-1", StudentID: "stu-1", Value: "8", UpdatedAt: updated, Editable: true},
			{EnrollmentID: "enr-2", StudentID: "stu-2", ReadOnlyReason: "Ese periodo corresponde al grupo anterior del alumno"},
		},
	}, sheet)

	n, err := c.CountScaleViolations(ctx, "lvl-1", client.LevelGrading{Enabled: true, Scale: client.GradeScale{MinHundredths: 500, MaxHundredths: 950, Decimals: 1, PassingHundredths: 600}})
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	assert.Equal(t, "lvl-1", stub.gotCount.GetLevelId())
	assert.Equal(t, int32(950), stub.gotCount.GetScale().GetMaxHundredths())
	_, err = c.CountScaleViolations(ctx, "lvl-1", client.LevelGrading{Letters: true})
	require.NoError(t, err)
	assert.True(t, stub.gotCount.GetGradingLetters(), "a switch to letters counts every number")

	fam, err := c.GetEnrollmentPeriodGrades(ctx, "enr-1")
	require.NoError(t, err)
	assert.Equal(t, client.EnrollmentPeriodGrades{
		CycleID: "cyc-1", CycleName: "2026-2027", Periods: wantPeriodRefs,
		Scale:          client.GradeScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600},
		GradingEnabled: true, SubjectIDs: []string{"sub-1"},
		Grades: []client.PeriodGrade{{EnrollmentID: "enr-1", SubjectID: "sub-1", PeriodID: "per-1", Value: "9"}},
	}, fam)

	with, err := c.PeriodsWithGrades(ctx, []string{"per-1", "per-2"})
	require.NoError(t, err)
	assert.Equal(t, []string{"per-1"}, with)
	assert.Equal(t, []string{"per-1", "per-2"}, stub.gotWith.GetPeriodIds())
}

// gradingLetters travels through the client's explicit mapping.
func TestGradesClientCarriesGradingLetters(t *testing.T) {
	stub := &stubPeriodGrades{letters: true}
	mux := http.NewServeMux()
	path, h := gradesv1connect.NewGradesServiceHandler(stub)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := client.NewGradesClient(srv.URL, srv.Client())

	fam, err := c.GetEnrollmentPeriodGrades(context.Background(), "enr-1")
	require.NoError(t, err)
	assert.True(t, fam.GradingLetters)
	assert.False(t, fam.GradingEnabled)
	sheet, err := c.GetPeriodSheet(context.Background(), client.PeriodSheetQuery{AssignmentID: "asg-1"})
	require.NoError(t, err)
	assert.True(t, sheet.GradingLetters)
}
