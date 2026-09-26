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

	attendancev1 "github.com/garcia-lorca/gen/go/garcialorca/attendance/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/attendance/v1/attendancev1connect"
	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	"github.com/garcia-lorca/services/gateway/internal/client"
)

// --- config: attendance mode ---------------------------------------------------

type stubLevels struct {
	configv1connect.UnimplementedConfigServiceHandler
	gotMode configv1.AttendanceMode
}

func (s *stubLevels) ListLevels(context.Context, *connect.Request[configv1.ListLevelsRequest]) (*connect.Response[configv1.ListLevelsResponse], error) {
	return connect.NewResponse(&configv1.ListLevelsResponse{Levels: []*configv1.Level{
		{Id: "lvl-1", Name: "Primaria", AttendanceMode: configv1.AttendanceMode_ATTENDANCE_MODE_DAILY},
		{Id: "lvl-2", Name: "Secundaria", AttendanceMode: configv1.AttendanceMode_ATTENDANCE_MODE_PER_CLASS},
	}}), nil
}

func (s *stubLevels) SetLevelAttendanceMode(_ context.Context, req *connect.Request[configv1.SetLevelAttendanceModeRequest]) (*connect.Response[configv1.SetLevelAttendanceModeResponse], error) {
	s.gotMode = req.Msg.GetAttendanceMode()
	return connect.NewResponse(&configv1.SetLevelAttendanceModeResponse{Level: &configv1.Level{
		Id: req.Msg.GetId(), Name: "Primaria", AttendanceMode: req.Msg.GetAttendanceMode(),
	}}), nil
}

func TestConfigClientLevelAttendanceModeRoundTrip(t *testing.T) {
	stub := &stubLevels{}
	mux := http.NewServeMux()
	path, h := configv1connect.NewConfigServiceHandler(stub)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := client.NewConfigClient(srv.URL, srv.Client())

	levels, err := c.ListLevels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "daily", levels[0].AttendanceMode)
	assert.Equal(t, "per_class", levels[1].AttendanceMode)

	for _, tt := range []struct {
		mode string
		wire configv1.AttendanceMode
	}{
		{"per_class", configv1.AttendanceMode_ATTENDANCE_MODE_PER_CLASS},
		{"daily", configv1.AttendanceMode_ATTENDANCE_MODE_DAILY},
		{"weekly", configv1.AttendanceMode_ATTENDANCE_MODE_UNSPECIFIED},
	} {
		got, err := c.SetLevelAttendanceMode(context.Background(), "lvl-1", tt.mode)
		require.NoError(t, err)
		assert.Equal(t, tt.wire, stub.gotMode, "mode %q on the wire", tt.mode)
		if tt.wire != configv1.AttendanceMode_ATTENDANCE_MODE_UNSPECIFIED {
			assert.Equal(t, tt.mode, got.AttendanceMode)
		}
	}
}

// --- attendance: lists, sessions, notes -------------------------------------------

type stubAttendance struct {
	attendancev1connect.UnimplementedAttendanceServiceHandler
	gotRoster *attendancev1.GetDailyRosterRequest
	gotSave   *attendancev1.SaveDailyAttendanceRequest
	session   *attendancev1.AttendanceSession
}

func (s *stubAttendance) GetDailyRoster(_ context.Context, req *connect.Request[attendancev1.GetDailyRosterRequest]) (*connect.Response[attendancev1.GetDailyRosterResponse], error) {
	s.gotRoster = req.Msg
	return connect.NewResponse(&attendancev1.GetDailyRosterResponse{
		Rows:    []*attendancev1.RosterRow{{EnrollmentId: "enr-1", StudentId: "stu-1", Status: "late", Note: "8:15"}},
		Session: s.session,
	}), nil
}

func (s *stubAttendance) SaveDailyAttendance(_ context.Context, req *connect.Request[attendancev1.SaveDailyAttendanceRequest]) (*connect.Response[attendancev1.SaveDailyAttendanceResponse], error) {
	s.gotSave = req.Msg
	return connect.NewResponse(&attendancev1.SaveDailyAttendanceResponse{AbsentCount: 1, MarkedCount: 2}), nil
}

func (s *stubAttendance) ListAttendance(context.Context, *connect.Request[attendancev1.ListAttendanceRequest]) (*connect.Response[attendancev1.ListAttendanceResponse], error) {
	return connect.NewResponse(&attendancev1.ListAttendanceResponse{Records: []*attendancev1.AttendanceRecord{
		{Id: "r1", EnrollmentId: "enr-1", Date: "2026-09-15", Status: "absent", SubjectId: "sub-1"},
	}}), nil
}

func newAttendanceStub(t *testing.T, stub *stubAttendance) *client.AttendanceClient {
	t.Helper()
	mux := http.NewServeMux()
	path, h := attendancev1connect.NewAttendanceServiceHandler(stub)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return client.NewAttendanceClient(srv.URL, srv.Client())
}

// Both directions of every T3 field: subject and taker out, note and session
// back, subject on history records.
func TestAttendanceClientListRoundTrip(t *testing.T) {
	takenAt := time.Date(2026, 9, 15, 14, 0, 0, 0, time.UTC)
	stub := &stubAttendance{session: &attendancev1.AttendanceSession{
		TakenAt: timestamppb.New(takenAt), TakenByName: "Gabriela", UpdatedAt: timestamppb.New(takenAt.Add(time.Hour)),
	}}
	c := newAttendanceStub(t, stub)
	ctx := context.Background()

	roster, err := c.GetDailyRoster(ctx, "grp-1", "2026-09-15", "sub-1")
	require.NoError(t, err)
	assert.Equal(t, "sub-1", stub.gotRoster.GetSubjectId())
	assert.Equal(t, []client.AttendanceRosterRow{{EnrollmentID: "enr-1", StudentID: "stu-1", Status: "late", Note: "8:15"}}, roster.Rows)
	require.NotNil(t, roster.Session)
	assert.Equal(t, client.AttendanceSession{TakenAt: takenAt, TakenByName: "Gabriela", UpdatedAt: takenAt.Add(time.Hour)}, *roster.Session)

	stub.session = nil
	untaken, err := c.GetDailyRoster(ctx, "grp-1", "2026-09-15", "")
	require.NoError(t, err)
	assert.Nil(t, untaken.Session)

	absent, marked, err := c.SaveDailyAttendance(ctx, client.SaveAttendanceInput{
		GroupID: "grp-1", Date: "2026-09-15", SubjectID: "sub-1", TakenBy: "usr-1", TakenByName: "Gabriela",
		Marks: []client.Mark{{EnrollmentID: "enr-1", Status: "absent", Note: "gripa"}},
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), absent)
	assert.Equal(t, int32(2), marked)
	assert.Equal(t, "sub-1", stub.gotSave.GetSubjectId())
	assert.Equal(t, "usr-1", stub.gotSave.GetTakenBy())
	assert.Equal(t, "Gabriela", stub.gotSave.GetTakenByName())
	assert.Equal(t, "gripa", stub.gotSave.GetMarks()[0].GetNote())

	records, err := c.ListAttendance(ctx, "enr-1", "", "")
	require.NoError(t, err)
	assert.Equal(t, "sub-1", records[0].SubjectID)
}
