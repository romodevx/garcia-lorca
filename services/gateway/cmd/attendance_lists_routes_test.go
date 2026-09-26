package main

import (
	"context"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
)

// SetLevelAttendanceMode echoes the new mode so the PATCH route answers 200.
func (stubConfig) SetLevelAttendanceMode(_ context.Context, req *connect.Request[configv1.SetLevelAttendanceModeRequest]) (*connect.Response[configv1.SetLevelAttendanceModeResponse], error) {
	return connect.NewResponse(&configv1.SetLevelAttendanceModeResponse{Level: &configv1.Level{
		Id: req.Msg.GetId(), Name: "Secundaria", AttendanceMode: req.Msg.GetAttendanceMode(),
	}}), nil
}

// T3 routes through the whole chain: module flags, auth and permissions.
func TestAttendanceListRoutesSecurityChain(t *testing.T) {
	const patchBody = `{"attendanceMode":"per_class"}`
	tests := []struct {
		name    string
		modules map[string]bool
		method  string
		path    string
		token   string
		body    string
		want    int
	}{
		{"classes: own reader", map[string]bool{"module_attendance": true, "module_config": true}, http.MethodGet, "/api/v1/teaching/attendance/classes", "att-own-reader", "", http.StatusOK},
		{"classes: config module off", map[string]bool{"module_attendance": true}, http.MethodGet, "/api/v1/teaching/attendance/classes", "att-own-reader", "", http.StatusNotFound},
		{"classes: attendance module off", map[string]bool{"module_config": true}, http.MethodGet, "/api/v1/teaching/attendance/classes", "att-own-reader", "", http.StatusNotFound},
		{"classes: no token", map[string]bool{"module_attendance": true, "module_config": true}, http.MethodGet, "/api/v1/teaching/attendance/classes", "", "", http.StatusUnauthorized},
		{"classes: attendance:read is not attendance_own:read", map[string]bool{"module_attendance": true, "module_config": true}, http.MethodGet, "/api/v1/teaching/attendance/classes", "att-reader", "", http.StatusForbidden},
		{"level mode: config writer", map[string]bool{"module_config": true}, http.MethodPatch, "/api/v1/config/levels/lvl-1", "config-writer", patchBody, http.StatusOK},
		{"level mode: config reader", map[string]bool{"module_config": true}, http.MethodPatch, "/api/v1/config/levels/lvl-1", "config-reader", patchBody, http.StatusForbidden},
		{"level mode: module off", map[string]bool{}, http.MethodPatch, "/api/v1/config/levels/lvl-1", "config-writer", patchBody, http.StatusNotFound},
		{"level mode: no token", map[string]bool{"module_config": true}, http.MethodPatch, "/api/v1/config/levels/lvl-1", "", patchBody, http.StatusUnauthorized},
		{"director roster: a daily list refuses a subject", map[string]bool{"module_attendance": true}, http.MethodGet, "/api/v1/attendance/roster?groupId=grp-1&date=2026-09-01&subjectId=sub-1", "att-reader", "", http.StatusBadRequest},
		{"teacher save: outside the window", map[string]bool{"module_attendance": true}, http.MethodPost, "/api/v1/teaching/attendance", "att-own-writer", `{"groupId":"grp-1","date":"2020-01-01","marks":[]}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := newTestGateway(t, tt.modules)
			res := doGateway(t, gw, tt.method, tt.path, tt.token, tt.body)
			assert.Equal(t, tt.want, res.StatusCode)
		})
	}
}
