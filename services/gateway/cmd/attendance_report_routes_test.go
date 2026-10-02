package main

import (
	"context"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"

	attendancev1 "github.com/garcia-lorca/gen/go/garcialorca/attendance/v1"
)

func (stubAttendance) SummarizeAttendance(context.Context, *connect.Request[attendancev1.SummarizeAttendanceRequest]) (*connect.Response[attendancev1.SummarizeAttendanceResponse], error) {
	return connect.NewResponse(&attendancev1.SummarizeAttendanceResponse{ListsTaken: 3, Counts: []*attendancev1.StudentAttendanceCount{
		{EnrollmentId: "enr-1", StudentId: "stu-1", Late: 1},
	}}), nil
}

// T12 through the real mux: the attendance report (and its PDF and Excel) is
// the director's — attendance:read behind module_attendance. A teacher's
// self-scoped attendance_own:read does not open it.
func TestAttendanceReportRoutes(t *testing.T) {
	on := map[string]bool{"module_attendance": true}
	const q = "?groupId=grp-1&from=2026-09-01&to=2026-09-30"
	cases := []struct {
		name, path, token string
		flags             map[string]bool
		want              int
	}{
		{"a reader reads the report", "/api/v1/attendance/reports" + q, "att-reader", on, http.StatusOK},
		{"a reader exports a PDF", "/api/v1/attendance/reports/pdf" + q, "att-reader", on, http.StatusOK},
		{"a reader exports to Excel", "/api/v1/attendance/reports/xlsx" + q, "att-reader", on, http.StatusOK},
		{"a teacher's own read is not enough", "/api/v1/attendance/reports" + q, "att-own-reader", on, http.StatusForbidden},
		{"no token", "/api/v1/attendance/reports" + q, "", on, http.StatusUnauthorized},
		{"module off hides it", "/api/v1/attendance/reports" + q, "att-reader", map[string]bool{}, http.StatusNotFound},
		{"module off hides the export", "/api/v1/attendance/reports/pdf" + q, "att-reader", map[string]bool{}, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gw := newTestGateway(t, tc.flags)
			if res := doGateway(t, gw, http.MethodGet, tc.path, tc.token, ""); res.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", res.StatusCode, tc.want)
			}
		})
	}
}
