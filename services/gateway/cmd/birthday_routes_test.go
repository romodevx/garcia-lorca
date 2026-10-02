package main

import (
	"context"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"

	commsv1 "github.com/garcia-lorca/gen/go/garcialorca/comms/v1"
)

func (stubComms) GetBirthdayTemplate(context.Context, *connect.Request[commsv1.GetBirthdayTemplateRequest]) (*connect.Response[commsv1.GetBirthdayTemplateResponse], error) {
	return connect.NewResponse(&commsv1.GetBirthdayTemplateResponse{Template: &commsv1.BirthdayTemplate{Title: "t", Body: "b", IsDefault: true}}), nil
}

func (stubComms) UpdateBirthdayTemplate(_ context.Context, req *connect.Request[commsv1.UpdateBirthdayTemplateRequest]) (*connect.Response[commsv1.UpdateBirthdayTemplateResponse], error) {
	return connect.NewResponse(&commsv1.UpdateBirthdayTemplateResponse{Template: &commsv1.BirthdayTemplate{Title: req.Msg.GetTitle(), Body: req.Msg.GetBody()}}), nil
}

// T11 through the real mux: the greeting answers to comms:write (the
// permission that already sends announcements) behind module_comms. A family
// account, or comms:read alone, cannot touch it. And nothing at the edge can
// trigger the run itself: Cloud Scheduler calls comms directly.
func TestBirthdayTemplateRoutes(t *testing.T) {
	on := map[string]bool{"module_comms": true}
	const body = `{"title":"¡Feliz día!","body":"Felicidades, {nombre}."}`
	cases := []struct {
		name, method, path, token, body string
		flags                           map[string]bool
		want                            int
	}{
		{"a comms writer reads", http.MethodGet, "/api/v1/comms/birthday-template", "comms-writer", "", on, http.StatusOK},
		{"a comms writer saves", http.MethodPut, "/api/v1/comms/birthday-template", "comms-writer", body, on, http.StatusOK},
		{"comms:read alone cannot read", http.MethodGet, "/api/v1/comms/birthday-template", "comms-reader", "", on, http.StatusForbidden},
		{"comms:read alone cannot save", http.MethodPut, "/api/v1/comms/birthday-template", "comms-reader", body, on, http.StatusForbidden},
		{"a family cannot save", http.MethodPut, "/api/v1/comms/birthday-template", "family-reader", body, on, http.StatusForbidden},
		{"no token", http.MethodGet, "/api/v1/comms/birthday-template", "", "", on, http.StatusUnauthorized},
		{"module off hides it", http.MethodGet, "/api/v1/comms/birthday-template", "comms-writer", "", map[string]bool{}, http.StatusNotFound},
		{"the run is not exposed", http.MethodPost, "/api/v1/comms/birthdays/run", "comms-writer", "{}", on, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gw := newTestGateway(t, tc.flags)
			if res := doGateway(t, gw, tc.method, tc.path, tc.token, tc.body); res.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", res.StatusCode, tc.want)
			}
		})
	}
}
