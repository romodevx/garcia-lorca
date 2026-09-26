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
	commsv1 "github.com/garcia-lorca/gen/go/garcialorca/comms/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/comms/v1/commsv1connect"
	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	"github.com/garcia-lorca/services/gateway/internal/client"
)

func serveConnect(t *testing.T, path string, h http.Handler) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

type stubT5Attendance struct {
	attendancev1connect.UnimplementedAttendanceServiceHandler
	gotSave     *attendancev1.SaveDailyAttendanceRequest
	gotSessions *attendancev1.ListSessionsRequest
}

func (s *stubT5Attendance) SaveDailyAttendance(_ context.Context, req *connect.Request[attendancev1.SaveDailyAttendanceRequest]) (*connect.Response[attendancev1.SaveDailyAttendanceResponse], error) {
	s.gotSave = req.Msg
	return connect.NewResponse(&attendancev1.SaveDailyAttendanceResponse{}), nil
}

func (s *stubT5Attendance) ListSessions(_ context.Context, req *connect.Request[attendancev1.ListSessionsRequest]) (*connect.Response[attendancev1.ListSessionsResponse], error) {
	s.gotSessions = req.Msg
	return connect.NewResponse(&attendancev1.ListSessionsResponse{Lists: []*attendancev1.TakenList{
		{GroupId: "grp-1", Date: "2026-09-16", SubjectId: "sub-1"},
		{GroupId: "grp-1", Date: "2026-09-15"},
	}}), nil
}

// The wire carries the list label out, and the taken lists back.
func TestAttendanceClientT5RoundTrip(t *testing.T) {
	stub := &stubT5Attendance{}
	path, h := attendancev1connect.NewAttendanceServiceHandler(stub)
	c := client.NewAttendanceClient(serveConnect(t, path, h), nil)

	_, _, err := c.SaveDailyAttendance(context.Background(), client.SaveAttendanceInput{GroupID: "grp-1", Date: "2026-09-15", SubjectID: "sub-1", SubjectName: "Matemáticas"})
	require.NoError(t, err)
	assert.Equal(t, "Matemáticas", stub.gotSave.GetSubjectName())

	lists, err := c.ListSessions(context.Background(), []string{"grp-1"}, "2026-09-01", "2026-09-30")
	require.NoError(t, err)
	assert.Equal(t, []string{"grp-1"}, stub.gotSessions.GetGroupIds())
	assert.Equal(t, "2026-09-01", stub.gotSessions.GetFromDate())
	assert.Equal(t, "2026-09-30", stub.gotSessions.GetToDate())
	assert.Equal(t, []client.TakenList{
		{GroupID: "grp-1", Date: "2026-09-16", SubjectID: "sub-1"},
		{GroupID: "grp-1", Date: "2026-09-15"},
	}, lists)
}

type stubT5Comms struct {
	commsv1connect.UnimplementedCommsServiceHandler
	got *commsv1.UnregisterDeviceRequest
}

func (s *stubT5Comms) UnregisterDevice(_ context.Context, req *connect.Request[commsv1.UnregisterDeviceRequest]) (*connect.Response[commsv1.UnregisterDeviceResponse], error) {
	s.got = req.Msg
	return connect.NewResponse(&commsv1.UnregisterDeviceResponse{}), nil
}

func TestCommsClientUnregisterDevice(t *testing.T) {
	stub := &stubT5Comms{}
	path, h := commsv1connect.NewCommsServiceHandler(stub)
	c := client.NewCommsClient(serveConnect(t, path, h), nil)
	require.NoError(t, c.UnregisterDevice(context.Background(), "usr-1", "ExponentPushToken[a]"))
	assert.Equal(t, "usr-1", stub.got.GetUserId())
	assert.Equal(t, "ExponentPushToken[a]", stub.got.GetFcmToken())
}

type stubT5Config struct {
	configv1connect.UnimplementedConfigServiceHandler
}

func (stubT5Config) GetSubject(_ context.Context, req *connect.Request[configv1.GetSubjectRequest]) (*connect.Response[configv1.GetSubjectResponse], error) {
	return connect.NewResponse(&configv1.GetSubjectResponse{Subject: &configv1.Subject{Id: req.Msg.GetId(), Name: "Historia"}}), nil
}

func TestConfigClientGetSubject(t *testing.T) {
	path, h := configv1connect.NewConfigServiceHandler(stubT5Config{})
	c := client.NewConfigClient(serveConnect(t, path, h), nil)
	s, err := c.GetSubject(context.Background(), "sub-9")
	require.NoError(t, err)
	assert.Equal(t, "sub-9", s.ID)
	assert.Equal(t, "Historia", s.Name)
}
