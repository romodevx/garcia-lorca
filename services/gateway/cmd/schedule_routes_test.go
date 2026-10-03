package main

import (
	"context"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
)

func (stubConfig) GetScheduleBoard(context.Context, *connect.Request[configv1.GetScheduleBoardRequest]) (*connect.Response[configv1.GetScheduleBoardResponse], error) {
	return connect.NewResponse(&configv1.GetScheduleBoardResponse{Cycle: &configv1.Cycle{Id: "c1"}}), nil
}

func (stubConfig) CreateScheduleBlock(context.Context, *connect.Request[configv1.CreateScheduleBlockRequest]) (*connect.Response[configv1.CreateScheduleBlockResponse], error) {
	return connect.NewResponse(&configv1.CreateScheduleBlockResponse{Block: &configv1.ScheduleBlock{Id: "b1", Weekday: 1, StartsAt: 480, EndsAt: 530}}), nil
}

func (stubConfig) ListTeacherAbsences(context.Context, *connect.Request[configv1.ListTeacherAbsencesRequest]) (*connect.Response[configv1.ListTeacherAbsencesResponse], error) {
	return connect.NewResponse(&configv1.ListTeacherAbsencesResponse{}), nil
}

func (stubConfig) CreateTeacherAbsence(context.Context, *connect.Request[configv1.CreateTeacherAbsenceRequest]) (*connect.Response[configv1.CreateTeacherAbsenceResponse], error) {
	return connect.NewResponse(&configv1.CreateTeacherAbsenceResponse{Absence: &configv1.TeacherAbsence{Id: "a1", AllDay: true}}), nil
}

func (stubConfig) ListUncoveredClasses(context.Context, *connect.Request[configv1.ListUncoveredClassesRequest]) (*connect.Response[configv1.ListUncoveredClassesResponse], error) {
	return connect.NewResponse(&configv1.ListUncoveredClassesResponse{}), nil
}

// T16 through the real mux: Horarios needs module_schedule (and module_config,
// where it lives); reads answer to schedule:read, writes to schedule:write; a
// teacher reads only their own timetable, with teaching:read.
func TestScheduleRoutes(t *testing.T) {
	both := map[string]bool{"module_config": true, "module_schedule": true}
	configOnly := map[string]bool{"module_config": true}
	const block = `{"assignmentId":"a1000000-0000-0000-0000-000000000001","weekday":1,"startsAt":"08:00","endsAt":"08:50"}`
	const absence = `{"teacherId":"a1000000-0000-0000-0000-000000000002","kind":"sick","startsOn":"2026-10-05","endsOn":"2026-10-05","allDay":true}`
	const absences = "/api/v1/schedule/absences?cycleId=a1000000-0000-0000-0000-000000000003&from=2026-10-05&to=2026-10-09"
	const uncovered = "/api/v1/schedule/uncovered?cycleId=a1000000-0000-0000-0000-000000000003&from=2026-10-05&to=2026-10-09"
	cases := []struct {
		name, method, path, token, body string
		flags                           map[string]bool
		want                            int
	}{
		{"a reader reads the timetable", http.MethodGet, "/api/v1/schedule", "schedule-reader", "", both, http.StatusOK},
		{"config:read is not enough", http.MethodGet, "/api/v1/schedule", "config-reader", "", both, http.StatusForbidden},
		{"no token", http.MethodGet, "/api/v1/schedule", "", "", both, http.StatusUnauthorized},
		{"module off hides it", http.MethodGet, "/api/v1/schedule", "schedule-reader", "", configOnly, http.StatusNotFound},
		{"config off hides it", http.MethodGet, "/api/v1/schedule", "schedule-reader", "", map[string]bool{"module_schedule": true}, http.StatusNotFound},
		{"a writer adds a class", http.MethodPost, "/api/v1/schedule/blocks", "schedule-writer", block, both, http.StatusCreated},
		{"a reader cannot add a class", http.MethodPost, "/api/v1/schedule/blocks", "schedule-reader", block, both, http.StatusForbidden},
		{"a teacher reads their own", http.MethodGet, "/api/v1/teaching/schedule", "maestro", "", both, http.StatusOK},
		{"a teacher cannot read the school's", http.MethodGet, "/api/v1/schedule", "maestro", "", both, http.StatusForbidden},
		{"the own timetable needs the module", http.MethodGet, "/api/v1/teaching/schedule", "maestro", "", configOnly, http.StatusNotFound},
		// T19 — absences reuse schedule:read / schedule:write.
		{"a reader lists absences", http.MethodGet, absences, "schedule-reader", "", both, http.StatusOK},
		{"a reader lists the classes to cover", http.MethodGet, uncovered, "schedule-reader", "", both, http.StatusOK},
		{"a writer registers an absence", http.MethodPost, "/api/v1/schedule/absences", "schedule-writer", absence, both, http.StatusCreated},
		{"a reader cannot register one", http.MethodPost, "/api/v1/schedule/absences", "schedule-reader", absence, both, http.StatusForbidden},
		{"a teacher cannot register one", http.MethodPost, "/api/v1/schedule/absences", "maestro", absence, both, http.StatusForbidden},
		{"a teacher cannot list them", http.MethodGet, absences, "maestro", "", both, http.StatusForbidden},
		{"absences need the module", http.MethodGet, absences, "schedule-reader", "", configOnly, http.StatusNotFound},
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
