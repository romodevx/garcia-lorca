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
	"github.com/garcia-lorca/services/gateway/internal/client"
)

type stubSummary struct {
	attendancev1connect.UnimplementedAttendanceServiceHandler
	got *attendancev1.SummarizeAttendanceRequest
}

func (s *stubSummary) SummarizeAttendance(_ context.Context, req *connect.Request[attendancev1.SummarizeAttendanceRequest]) (*connect.Response[attendancev1.SummarizeAttendanceResponse], error) {
	s.got = req.Msg
	return connect.NewResponse(&attendancev1.SummarizeAttendanceResponse{ListsTaken: 20, Counts: []*attendancev1.StudentAttendanceCount{
		{EnrollmentId: "enr-1", StudentId: "stu-1", Absent: 1, Late: 2, Excused: 3, Lists: 18, Active: true},
	}}), nil
}

// T12: the report's summary crosses the client intact.
func TestAttendanceClientSummarizeAttendance(t *testing.T) {
	stub := &stubSummary{}
	mux := http.NewServeMux()
	path, h := attendancev1connect.NewAttendanceServiceHandler(stub)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	got, err := client.NewAttendanceClient(srv.URL, srv.Client()).SummarizeAttendance(context.Background(), "grp-1", "2026-09-01", "2026-09-30")
	require.NoError(t, err)

	assert.Equal(t, "grp-1", stub.got.GetGroupId())
	assert.Equal(t, "2026-09-01", stub.got.GetFromDate())
	assert.Equal(t, "2026-09-30", stub.got.GetToDate())
	assert.Equal(t, client.AttendanceSummary{ListsTaken: 20, Counts: []client.AttendanceCount{
		{EnrollmentID: "enr-1", StudentID: "stu-1", Absent: 1, Late: 2, Excused: 3, Lists: 18, Active: true},
	}}, got)
}
