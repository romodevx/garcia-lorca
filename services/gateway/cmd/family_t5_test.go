package main

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	connect "connectrpc.com/connect"

	attendancev1 "github.com/garcia-lorca/gen/go/garcialorca/attendance/v1"
	commsv1 "github.com/garcia-lorca/gen/go/garcialorca/comms/v1"
)

func (stubAttendance) ListSessions(_ context.Context, req *connect.Request[attendancev1.ListSessionsRequest]) (*connect.Response[attendancev1.ListSessionsResponse], error) {
	out := make([]*attendancev1.TakenList, 0, len(req.Msg.GetGroupIds()))
	for _, g := range req.Msg.GetGroupIds() {
		out = append(out, &attendancev1.TakenList{GroupId: g, Date: "2026-09-01"})
	}
	return connect.NewResponse(&attendancev1.ListSessionsResponse{Lists: out}), nil
}

func (stubComms) UnregisterDevice(context.Context, *connect.Request[commsv1.UnregisterDeviceRequest]) (*connect.Response[commsv1.UnregisterDeviceResponse], error) {
	return connect.NewResponse(&commsv1.UnregisterDeviceResponse{}), nil
}

const familyDeviceBody = `{"token":"ExponentPushToken[abc123]","platform":"ios"}`

var familyDevicePath = "/api/v1/family/devices/" + url.PathEscape("ExponentPushToken[abc123]")

// T5 device routes: family:read, guarded on comms AND students like every other
// family route (either module OFF ⇒ 404, invisible rather than refused).
func TestFamilyDeviceRoutes(t *testing.T) {
	tests := []struct {
		name     string
		flags    map[string]bool
		method   string
		path     string
		token    string
		body     string
		wantCode int
	}{
		{"register", familyModulesOn, http.MethodPost, "/api/v1/family/devices", "family-reader", familyDeviceBody, http.StatusNoContent},
		{"unregister", familyModulesOn, http.MethodDelete, familyDevicePath, "family-reader", "", http.StatusNoContent},
		{"register comms OFF", map[string]bool{"module_students": true}, http.MethodPost, "/api/v1/family/devices", "family-reader", familyDeviceBody, http.StatusNotFound},
		{"register students OFF", map[string]bool{"module_comms": true}, http.MethodPost, "/api/v1/family/devices", "family-reader", familyDeviceBody, http.StatusNotFound},
		{"unregister comms OFF", map[string]bool{"module_students": true}, http.MethodDelete, familyDevicePath, "family-reader", "", http.StatusNotFound},
		{"register without token", familyModulesOn, http.MethodPost, "/api/v1/family/devices", "", familyDeviceBody, http.StatusUnauthorized},
		{"register without family:read", familyModulesOn, http.MethodPost, "/api/v1/family/devices", "comms-reader", familyDeviceBody, http.StatusForbidden},
		{"unregister without family:read", familyModulesOn, http.MethodDelete, familyDevicePath, "comms-reader", "", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := newTestGateway(t, tt.flags)
			res := doGateway(t, gw, tt.method, tt.path, tt.token, tt.body)
			if res.StatusCode != tt.wantCode {
				t.Errorf("status = %d, want %d", res.StatusCode, tt.wantCode)
			}
		})
	}
}
