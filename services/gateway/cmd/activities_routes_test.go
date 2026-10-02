package main

import (
	"context"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

func (stubFinance) ListActivities(context.Context, *connect.Request[financev1.ListActivitiesRequest]) (*connect.Response[financev1.ListActivitiesResponse], error) {
	return connect.NewResponse(&financev1.ListActivitiesResponse{}), nil
}

func (stubFinance) CreateActivity(_ context.Context, req *connect.Request[financev1.CreateActivityRequest]) (*connect.Response[financev1.CreateActivityResponse], error) {
	return connect.NewResponse(&financev1.CreateActivityResponse{Activity: &financev1.Activity{Id: "ac100000-0000-0000-0000-000000000001", Name: req.Msg.GetActivity().GetName()}}), nil
}

func (stubFinance) EnrollInActivity(context.Context, *connect.Request[financev1.EnrollInActivityRequest]) (*connect.Response[financev1.EnrollInActivityResponse], error) {
	return connect.NewResponse(&financev1.EnrollInActivityResponse{ChargesCreated: 1}), nil
}

// T17 through the real mux: Actividades needs module_activities (and
// module_finance, where its data lives); reads answer to activities:read,
// writes to activities:write. finance:write alone does not open the module.
func TestActivitiesRoutes(t *testing.T) {
	both := map[string]bool{"module_finance": true, "module_activities": true}
	financeOnly := map[string]bool{"module_finance": true}
	const activity = `{"name":"Natación","schedule":[{"weekday":2,"start":"15:00","end":"16:00"}],"periodicity":"monthly",` +
		`"billingStartsOn":"2026-09-01","firstDueOn":"2026-09-10","options":[{"daysPerWeek":null,"amountCents":90000}]}`
	const enroll = `{"enrollments":[{"studentId":"ac100000-0000-0000-0000-000000000052","optionId":"ac100000-0000-0000-0000-0000000000a1"}]}`
	const enrollPath = "/api/v1/activities/ac100000-0000-0000-0000-000000000001/enrollments"
	cases := []struct {
		name, method, path, token, body string
		flags                           map[string]bool
		want                            int
	}{
		{"a reader lists", http.MethodGet, "/api/v1/activities", "activities-reader", "", both, http.StatusOK},
		{"finance:write is not enough", http.MethodGet, "/api/v1/activities", "finance-writer", "", both, http.StatusForbidden},
		{"no token", http.MethodGet, "/api/v1/activities", "", "", both, http.StatusUnauthorized},
		{"module off hides it", http.MethodGet, "/api/v1/activities", "activities-reader", "", financeOnly, http.StatusNotFound},
		{"finance off hides it", http.MethodGet, "/api/v1/activities", "activities-reader", "", map[string]bool{"module_activities": true}, http.StatusNotFound},
		{"a writer creates", http.MethodPost, "/api/v1/activities", "activities-writer", activity, both, http.StatusCreated},
		{"a reader cannot create", http.MethodPost, "/api/v1/activities", "activities-reader", activity, both, http.StatusForbidden},
		{"a writer enrols", http.MethodPost, enrollPath, "activities-writer", enroll, both, http.StatusOK},
		{"a reader cannot enrol", http.MethodPost, enrollPath, "activities-reader", enroll, both, http.StatusForbidden},
		{"enrolling needs the module", http.MethodPost, enrollPath, "activities-writer", enroll, financeOnly, http.StatusNotFound},
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
