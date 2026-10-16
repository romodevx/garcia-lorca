package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// 16 Oct 2026, 01:00 UTC: still the 15th in Morelia.
func lateEveningOn15Oct() time.Time { return time.Date(2026, 10, 16, 1, 0, 0, 0, time.UTC) }

// T9. The alta's entry day travels to students as typed, both routes. A day
// before the school's today bills months already past, so it also needs
// finance:write (same precedent as the past-dated withdrawal, ADR 0034).
func TestEnrollRoutesGateAPastEntryDayOnFinanceWrite(t *testing.T) {
	routes := []struct {
		name, path, body string
		call             func(*handler.EnrollmentsHandler) http.HandlerFunc
		got              func(*fakeEnrollmentSvc) string
	}{
		{"group", "/api/v1/enrollments", `{"studentId":"stu-1","groupId":"grp-1"%s}`,
			func(h *handler.EnrollmentsHandler) http.HandlerFunc { return h.Enroll },
			func(f *fakeEnrollmentSvc) string { return f.gotEnrolledOn }},
		{"grade", "/api/v1/enrollments/by-grade", `{"studentId":"stu-1","gradeLevelId":"gl-1"%s}`,
			func(h *handler.EnrollmentsHandler) http.HandlerFunc { return h.EnrollIntoGrade },
			func(f *fakeEnrollmentSvc) string { return f.gradeEnrolledOn }},
	}
	cases := []struct {
		name, date string
		perms      []string
		want       int
	}{
		{name: "omitted", want: http.StatusCreated},
		{name: "today in Morelia after 18:00", date: "2026-10-15", want: http.StatusCreated},
		{name: "past without finance write", date: "2026-08-24", want: http.StatusForbidden},
		{name: "past with finance write", date: "2026-08-24", perms: []string{"finance:write"}, want: http.StatusCreated},
		{name: "not a date", date: "2026-02-30", perms: []string{"finance:write"}, want: http.StatusBadRequest},
		{name: "wrong shape", date: "24/08/2026", perms: []string{"finance:write"}, want: http.StatusBadRequest},
	}
	for _, route := range routes {
		for _, tc := range cases {
			t.Run(route.name+"/"+tc.name, func(t *testing.T) {
				svc := &fakeEnrollmentSvc{res: client.Enrollment{ID: "enr-1", Status: "active"}}
				h := handler.NewEnrollmentsHandler(svc, handler.WithEnrollmentsClock(lateEveningOn15Oct))
				extra := ""
				if tc.date != "" {
					extra = `,"enrolledOn":"` + tc.date + `"`
				}
				body := strings.Replace(route.body, "%s", extra, 1)
				req := httptest.NewRequest(http.MethodPost, route.path, strings.NewReader(body))
				req = req.WithContext(middleware.WithPrincipal(req.Context(), middleware.Principal{Permissions: tc.perms}))
				rec := httptest.NewRecorder()

				route.call(h)(rec, req)

				if rec.Code != tc.want {
					t.Fatalf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
				}
				switch tc.want {
				case http.StatusCreated:
					if got := route.got(svc); got != tc.date {
						t.Errorf("forwarded enrolledOn %q, want %q", got, tc.date)
					}
				case http.StatusForbidden:
					if !strings.Contains(rec.Body.String(), "fecha de ingreso anterior a hoy") {
						t.Errorf("forbidden body = %q", rec.Body.String())
					}
					fallthrough
				default:
					if svc.gotStudentID != "" || svc.gradeStudent != "" {
						t.Errorf("a refused request reached students: %+v", svc)
					}
				}
			})
		}
	}
}
