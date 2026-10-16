package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	attendancev1 "github.com/garcia-lorca/gen/go/garcialorca/attendance/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/attendance/v1/attendancev1connect"
	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/grades/v1/gradesv1connect"
	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
	"github.com/garcia-lorca/services/gateway/internal/client"
)

// --- config: catalog corrections (T8) -------------------------------------------

type stubCorrectionsConfig struct {
	configv1connect.UnimplementedConfigServiceHandler
	reordered []string
	deleted   []string
}

func (s *stubCorrectionsConfig) ReorderLevels(_ context.Context, req *connect.Request[configv1.ReorderLevelsRequest]) (*connect.Response[configv1.ReorderLevelsResponse], error) {
	s.reordered = req.Msg.GetLevelIds()
	out := make([]*configv1.Level, 0, len(s.reordered))
	for i, id := range s.reordered {
		out = append(out, &configv1.Level{Id: id, SortOrder: int32(i + 1)})
	}
	return connect.NewResponse(&configv1.ReorderLevelsResponse{Levels: out}), nil
}

func (s *stubCorrectionsConfig) RenameGradeLevel(_ context.Context, req *connect.Request[configv1.RenameGradeLevelRequest]) (*connect.Response[configv1.RenameGradeLevelResponse], error) {
	return connect.NewResponse(&configv1.RenameGradeLevelResponse{GradeLevel: &configv1.GradeLevel{
		Id: req.Msg.GetId(), LevelId: "lvl", Name: req.Msg.GetName(), Ordinal: 3,
	}}), nil
}

func (s *stubCorrectionsConfig) DeleteGradeLevel(_ context.Context, req *connect.Request[configv1.DeleteGradeLevelRequest]) (*connect.Response[configv1.DeleteGradeLevelResponse], error) {
	s.deleted = append(s.deleted, "grade:"+req.Msg.GetId())
	return connect.NewResponse(&configv1.DeleteGradeLevelResponse{}), nil
}

func (s *stubCorrectionsConfig) RenameGroup(_ context.Context, req *connect.Request[configv1.RenameGroupRequest]) (*connect.Response[configv1.RenameGroupResponse], error) {
	return connect.NewResponse(&configv1.RenameGroupResponse{Group: &configv1.Group{
		Id: req.Msg.GetId(), CycleId: "cyc", GradeLevelId: "gr", Name: req.Msg.GetName(), Capacity: 25,
	}}), nil
}

func (s *stubCorrectionsConfig) GroupUsage(context.Context, *connect.Request[configv1.GroupUsageRequest]) (*connect.Response[configv1.GroupUsageResponse], error) {
	return connect.NewResponse(&configv1.GroupUsageResponse{TeachingAssignments: 4, ScheduleBlocks: 6}), nil
}

func (s *stubCorrectionsConfig) DeleteGroup(_ context.Context, req *connect.Request[configv1.DeleteGroupRequest]) (*connect.Response[configv1.DeleteGroupResponse], error) {
	s.deleted = append(s.deleted, "group:"+req.Msg.GetId())
	return connect.NewResponse(&configv1.DeleteGroupResponse{}), nil
}

func TestConfigClientCatalogCorrections(t *testing.T) {
	stub := &stubCorrectionsConfig{}
	mux := http.NewServeMux()
	mux.Handle(configv1connect.NewConfigServiceHandler(stub))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := client.NewConfigClient(srv.URL, srv.Client())
	ctx := context.Background()

	levels, err := c.ReorderLevels(ctx, []string{"b", "a"})
	require.NoError(t, err)
	assert.Equal(t, []string{"b", "a"}, stub.reordered)
	assert.Equal(t, []client.LevelResult{{ID: "b", SortOrder: 1}, {ID: "a", SortOrder: 2}}, []client.LevelResult{
		{ID: levels[0].ID, SortOrder: levels[0].SortOrder}, {ID: levels[1].ID, SortOrder: levels[1].SortOrder}})

	g, err := c.RenameGradeLevel(ctx, "g1", "3°")
	require.NoError(t, err)
	assert.Equal(t, client.GradeLevelResult{ID: "g1", LevelID: "lvl", Name: "3°", Ordinal: 3}, g)

	grp, err := c.RenameGroup(ctx, "gr1", "B")
	require.NoError(t, err)
	assert.Equal(t, client.GroupResult{ID: "gr1", CycleID: "cyc", GradeLevelID: "gr", Name: "B", Capacity: 25}, grp)

	usage, err := c.GroupUsage(ctx, "gr1")
	require.NoError(t, err)
	assert.Equal(t, client.ConfigGroupUsage{TeachingAssignments: 4, ScheduleBlocks: 6}, usage)

	require.NoError(t, c.DeleteGradeLevel(ctx, "g1"))
	require.NoError(t, c.DeleteGroup(ctx, "gr1"))
	assert.Equal(t, []string{"grade:g1", "group:gr1"}, stub.deleted)
}

// --- students / attendance / grades: what each keeps of a group -------------------

type stubGroupStudents struct {
	studentsv1connect.UnimplementedStudentsServiceHandler
	relabelled string
}

func (s *stubGroupStudents) GroupUsage(context.Context, *connect.Request[studentsv1.GroupUsageRequest]) (*connect.Response[studentsv1.GroupUsageResponse], error) {
	return connect.NewResponse(&studentsv1.GroupUsageResponse{Enrollments: 25, Transfers: 3}), nil
}

func (s *stubGroupStudents) RefreshGroupName(_ context.Context, req *connect.Request[studentsv1.RefreshGroupNameRequest]) (*connect.Response[studentsv1.RefreshGroupNameResponse], error) {
	s.relabelled = req.Msg.GetGroupId() + "=" + req.Msg.GetName()
	return connect.NewResponse(&studentsv1.RefreshGroupNameResponse{Students: 24}), nil
}

type stubGroupAttendance struct {
	attendancev1connect.UnimplementedAttendanceServiceHandler
}

func (stubGroupAttendance) GroupUsage(context.Context, *connect.Request[attendancev1.GroupUsageRequest]) (*connect.Response[attendancev1.GroupUsageResponse], error) {
	return connect.NewResponse(&attendancev1.GroupUsageResponse{Sessions: 40, Marks: 900}), nil
}

type stubGroupGrades struct {
	gradesv1connect.UnimplementedGradesServiceHandler
}

func (stubGroupGrades) GroupUsage(context.Context, *connect.Request[gradesv1.GroupUsageRequest]) (*connect.Response[gradesv1.GroupUsageResponse], error) {
	return connect.NewResponse(&gradesv1.GroupUsageResponse{ReportCards: 2}), nil
}

func TestGroupUsageClients(t *testing.T) {
	ctx := context.Background()
	students := &stubGroupStudents{}
	sp, sh := studentsv1connect.NewStudentsServiceHandler(students)
	ap, ah := attendancev1connect.NewAttendanceServiceHandler(stubGroupAttendance{})
	gp, gh := gradesv1connect.NewGradesServiceHandler(stubGroupGrades{})
	ss, as, gs := serveAt(t, sp, sh), serveAt(t, ap, ah), serveAt(t, gp, gh)

	su, err := client.NewStudentsClient(ss, http.DefaultClient).GroupUsage(ctx, "g")
	require.NoError(t, err)
	assert.Equal(t, client.StudentsGroupUsage{Enrollments: 25, Transfers: 3}, su)

	n, err := client.NewStudentsClient(ss, http.DefaultClient).RefreshGroupName(ctx, "g", "B")
	require.NoError(t, err)
	assert.Equal(t, int64(24), n)
	assert.Equal(t, "g=B", students.relabelled)

	au, err := client.NewAttendanceClient(as, http.DefaultClient).GroupUsage(ctx, "g")
	require.NoError(t, err)
	assert.Equal(t, client.AttendanceGroupUsage{Sessions: 40, Marks: 900}, au)

	cards, err := client.NewGradesClient(gs, http.DefaultClient).GroupReportCards(ctx, "g")
	require.NoError(t, err)
	assert.Equal(t, int64(2), cards)
}
