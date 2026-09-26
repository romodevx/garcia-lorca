package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// T2: the maestro role reaches Calificaciones and Asistencias through the
// self-scoped /teaching/* surface and NOTHING else. Every admin-wide route is
// 403 for it even with every module on — the broad surfaces read and write any
// group, so holding teaching:* / attendance_own:* must never open them.

var allModulesOn = map[string]bool{
	"module_students": true, "module_grades": true, "module_attendance": true,
	"module_teachers": true, "module_config": true, "module_iam": true, "module_finance": true,
}

func TestMaestroIsForbiddenFromEveryAdminSurface(t *testing.T) {
	routes := []struct {
		method, path, body string
	}{
		{http.MethodGet, "/api/v1/grades?assignmentId=asg-1", ""},
		{http.MethodGet, "/api/v1/grades/roster?assignmentId=asg-1", ""},
		{http.MethodPost, "/api/v1/grades", `{"assignmentId":"asg-1","enrollmentId":"enr-1","termId":"trm-1","value":9}`},
		{http.MethodGet, "/api/v1/attendance/roster?groupId=grp-1&date=2026-09-01", ""},
		{http.MethodGet, "/api/v1/attendance?enrollmentId=enr-1", ""},
		{http.MethodPost, "/api/v1/attendance", `{"groupId":"grp-1","date":"2026-09-01","marks":[]}`},
		{http.MethodGet, "/api/v1/students", ""},
		{http.MethodGet, "/api/v1/students/stu-1", ""},
		{http.MethodPost, "/api/v1/students", validBody},
		{http.MethodGet, "/api/v1/teachers", ""},
		{http.MethodPost, "/api/v1/teachers", validTeacherBody},
		{http.MethodGet, "/api/v1/teaching-assignments", ""},
		{http.MethodPost, "/api/v1/teachers/tch-1/access", ""},
		{http.MethodGet, "/api/v1/config/groups", ""},
		{http.MethodGet, "/api/v1/users", ""},
		{http.MethodGet, "/api/v1/finance/overdue", ""},
	}
	gw := newTestGateway(t, allModulesOn)
	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			res := doGateway(t, gw, rt.method, rt.path, "maestro", rt.body)
			assert.Equal(t, http.StatusForbidden, res.StatusCode)
		})
	}
}

// The same token opens its own surface — the 403s above are about the route,
// not a broken token.
func TestMaestroReachesItsOwnTeachingSurface(t *testing.T) {
	routes := []struct {
		method, path, body string
	}{
		{http.MethodGet, "/api/v1/teaching/assignments", ""},
		{http.MethodGet, "/api/v1/teaching/groups", ""},
		{http.MethodGet, "/api/v1/teaching/roster?assignmentId=asg-1&termId=trm-1", ""},
		{http.MethodPost, "/api/v1/teaching/grades", `{"assignmentId":"asg-1","enrollmentId":"enr-1","termId":"trm-1","value":9}`},
		{http.MethodGet, "/api/v1/teaching/attendance/roster?groupId=grp-1&date=2026-09-01", ""},
		// A teacher saves inside the window only: today's school (UTC) date.
		{http.MethodPost, "/api/v1/teaching/attendance", `{"groupId":"grp-1","date":"` + schoolTodayISO() + `","marks":[]}`},
		{http.MethodGet, "/api/v1/teaching/attendance/classes", ""},
	}
	gw := newTestGateway(t, allModulesOn)
	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			res := doGateway(t, gw, rt.method, rt.path, "maestro", rt.body)
			assert.Equal(t, http.StatusOK, res.StatusCode)
		})
	}
}

// Another teacher's assignment and group through the full chain: 404.
func TestMaestroAimingAtAnotherTeachersScopeIs404(t *testing.T) {
	routes := []struct {
		method, path, body string
	}{
		{http.MethodGet, "/api/v1/teaching/roster?assignmentId=asg-other&termId=trm-1", ""},
		{http.MethodPost, "/api/v1/teaching/grades", `{"assignmentId":"asg-other","enrollmentId":"enr-1","termId":"trm-1","value":9}`},
		{http.MethodGet, "/api/v1/teaching/attendance/roster?groupId=grp-other&date=2026-09-01", ""},
		{http.MethodPost, "/api/v1/teaching/attendance", `{"groupId":"grp-other","date":"2026-09-01","marks":[]}`},
	}
	gw := newTestGateway(t, allModulesOn)
	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			res := doGateway(t, gw, rt.method, rt.path, "maestro", rt.body)
			assert.Equal(t, http.StatusNotFound, res.StatusCode)
		})
	}
}
