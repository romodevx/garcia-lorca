package main

import (
	"context"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
)

// T6 monthly grade stubs: every call echoes, so a route test observes only the
// security chain (flags, auth, permissions, ownership).

func (stubGrades) GetMonthlySheet(_ context.Context, req *connect.Request[gradesv1.GetMonthlySheetRequest]) (*connect.Response[gradesv1.GetMonthlySheetResponse], error) {
	return connect.NewResponse(&gradesv1.GetMonthlySheetResponse{
		AssignmentId: req.Msg.GetAssignmentId(), Month: "2026-09", Months: []string{"2026-09"},
		Scale:          &gradesv1.GradingScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600},
		GradingEnabled: true, Editable: true,
		Rows: []*gradesv1.MonthlySheetRow{{EnrollmentId: "enr-1", StudentId: "stu-1", Value: "9.5"}},
	}), nil
}

func (stubGrades) UpsertMonthlyGrade(_ context.Context, req *connect.Request[gradesv1.UpsertMonthlyGradeRequest]) (*connect.Response[gradesv1.UpsertMonthlyGradeResponse], error) {
	return connect.NewResponse(&gradesv1.UpsertMonthlyGradeResponse{Grade: &gradesv1.MonthlyGrade{
		EnrollmentId: req.Msg.GetEnrollmentId(), SubjectId: "sub-1", Month: req.Msg.GetMonth(), Value: req.Msg.GetValue(),
	}}), nil
}

func (stubGrades) ClearMonthlyGrade(context.Context, *connect.Request[gradesv1.ClearMonthlyGradeRequest]) (*connect.Response[gradesv1.ClearMonthlyGradeResponse], error) {
	return connect.NewResponse(&gradesv1.ClearMonthlyGradeResponse{}), nil
}

func (stubGrades) GetEnrollmentMonthlyGrades(_ context.Context, req *connect.Request[gradesv1.GetEnrollmentMonthlyGradesRequest]) (*connect.Response[gradesv1.GetEnrollmentMonthlyGradesResponse], error) {
	return connect.NewResponse(&gradesv1.GetEnrollmentMonthlyGradesResponse{
		CycleId: "cyc-1", CycleName: "2026-2027", Months: []string{"2026-09"},
		Scale:          &gradesv1.GradingScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600},
		GradingEnabled: true, SubjectIds: []string{"sub-1"},
		Grades: []*gradesv1.MonthlyGrade{{EnrollmentId: req.Msg.GetEnrollmentId(), SubjectId: "sub-1", Month: "2026-09", Value: "9.5"}},
	}), nil
}

// GetTeachingAssignment knows only the stub's one assignment, by its uuid form
// and by its short id.
func (stubConfig) GetTeachingAssignment(_ context.Context, req *connect.Request[configv1.GetTeachingAssignmentRequest]) (*connect.Response[configv1.GetTeachingAssignmentResponse], error) {
	if req.Msg.GetId() != "asg-1" {
		return nil, connect.NewError(connect.CodeNotFound, errStr("assignment not found"))
	}
	return connect.NewResponse(&configv1.GetTeachingAssignmentResponse{TeachingAssignment: &configv1.TeachingAssignment{
		Id: "asg-1", CycleId: "cyc-1", GroupId: "grp-1", SubjectId: "sub-1", TeacherId: "tch-1",
	}}), nil
}

const (
	// The teacher's owned assignment in uuid form is not known to the stubs
	// (they use "asg-1"), so a well-formed foreign uuid proves the 404 path and
	// the director routes (any assignment) prove the 200 path.
	monthlyForeignAsg = "a0000000-0000-0000-0000-00000000ffff"
	monthlyEnr        = "e0000000-0000-0000-0000-000000000001"
)

var (
	gradesModulesOn         = map[string]bool{"module_grades": true, "module_config": true}
	monthlyFamilyModulesOn  = map[string]bool{"module_grades": true, "module_students": true}
	monthlyPutBody          = `{"assignmentId":"` + monthlyForeignAsg + `","month":"2026-09","enrollmentId":"` + monthlyEnr + `","value":9.5}`
	monthlyDeleteQuery      = "?assignmentId=" + monthlyForeignAsg + "&month=2026-09&enrollmentId=" + monthlyEnr
	directorSheetPath       = "/api/v1/grades/sheet?assignmentId=" + monthlyForeignAsg
	teacherSheetForeignPath = "/api/v1/teaching/grades/sheet?assignmentId=" + monthlyForeignAsg
)

func TestMonthlyGradeRoutes(t *testing.T) {
	tests := []struct {
		name     string
		flags    map[string]bool
		method   string
		path     string
		token    string
		body     string
		wantCode int
	}{
		// director
		{"director sheet", gradesModulesOn, http.MethodGet, directorSheetPath, "grades-reader", "", http.StatusOK},
		{"director sheet grades OFF", map[string]bool{"module_config": true}, http.MethodGet, directorSheetPath, "grades-reader", "", http.StatusNotFound},
		{"director sheet no token", gradesModulesOn, http.MethodGet, directorSheetPath, "", "", http.StatusUnauthorized},
		{"director sheet teaching:read is not grades:read", gradesModulesOn, http.MethodGet, directorSheetPath, "teaching-reader", "", http.StatusForbidden},
		{"director put", gradesModulesOn, http.MethodPut, "/api/v1/grades/monthly", "grades-writer", monthlyPutBody, http.StatusOK},
		{"director put needs grades:write", gradesModulesOn, http.MethodPut, "/api/v1/grades/monthly", "grades-reader", monthlyPutBody, http.StatusForbidden},
		{"director delete", gradesModulesOn, http.MethodDelete, "/api/v1/grades/monthly" + monthlyDeleteQuery, "grades-writer", "", http.StatusNoContent},
		{"director delete needs grades:write", gradesModulesOn, http.MethodDelete, "/api/v1/grades/monthly" + monthlyDeleteQuery, "grades-reader", "", http.StatusForbidden},
		{"director terms", gradesModulesOn, http.MethodGet, "/api/v1/grades/terms?assignmentId=asg-1", "grades-reader", "", http.StatusOK},
		{"director terms config OFF", map[string]bool{"module_grades": true}, http.MethodGet, "/api/v1/grades/terms?assignmentId=asg-1", "grades-reader", "", http.StatusNotFound},
		// teacher
		{"teacher sheet foreign assignment is 404", gradesModulesOn, http.MethodGet, teacherSheetForeignPath, "teaching-reader", "", http.StatusNotFound},
		{"teacher sheet config OFF", map[string]bool{"module_grades": true}, http.MethodGet, teacherSheetForeignPath, "teaching-reader", "", http.StatusNotFound},
		{"teacher sheet no token", gradesModulesOn, http.MethodGet, teacherSheetForeignPath, "", "", http.StatusUnauthorized},
		{"teacher sheet grades:read is not teaching:read", gradesModulesOn, http.MethodGet, teacherSheetForeignPath, "grades-reader", "", http.StatusForbidden},
		{"teacher put foreign assignment is 404", gradesModulesOn, http.MethodPut, "/api/v1/teaching/grades/monthly", "teaching-writer", monthlyPutBody, http.StatusNotFound},
		{"teacher put needs teaching:write", gradesModulesOn, http.MethodPut, "/api/v1/teaching/grades/monthly", "teaching-reader", monthlyPutBody, http.StatusForbidden},
		{"teacher delete foreign assignment is 404", gradesModulesOn, http.MethodDelete, "/api/v1/teaching/grades/monthly" + monthlyDeleteQuery, "teaching-writer", "", http.StatusNotFound},
		// family
		{"family monthly", monthlyFamilyModulesOn, http.MethodGet, "/api/v1/family/students/stu-1/grades/monthly", "family-reader", "", http.StatusOK},
		{"family monthly other child", monthlyFamilyModulesOn, http.MethodGet, "/api/v1/family/students/stu-999/grades/monthly", "family-reader", "", http.StatusNotFound},
		{"family monthly grades OFF", map[string]bool{"module_students": true}, http.MethodGet, "/api/v1/family/students/stu-1/grades/monthly", "family-reader", "", http.StatusNotFound},
		{"family monthly students OFF", map[string]bool{"module_grades": true}, http.MethodGet, "/api/v1/family/students/stu-1/grades/monthly", "family-reader", "", http.StatusNotFound},
		{"family monthly needs family:read", monthlyFamilyModulesOn, http.MethodGet, "/api/v1/family/students/stu-1/grades/monthly", "grades-reader", "", http.StatusForbidden},
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

// CountScaleViolations: level "lvl-busy" has captured grades a new scale would
// break; any other level has none.
func (stubGrades) CountScaleViolations(_ context.Context, req *connect.Request[gradesv1.CountScaleViolationsRequest]) (*connect.Response[gradesv1.CountScaleViolationsResponse], error) {
	var n int32
	if req.Msg.GetLevelId() == "lvl-busy" {
		n = 4
	}
	return connect.NewResponse(&gradesv1.CountScaleViolationsResponse{Violations: n}), nil
}

// PATCH /config/levels/{id} with a grading change asks grades first (the
// composition root wires the scale guard): 409 SCALE_TIGHTENING when captured
// grades would break, 200 otherwise.
func TestPatchLevelGradingRouteChecksCapturedGrades(t *testing.T) {
	const body = `{"grading":{"enabled":true,"min":5,"max":9,"decimals":0,"passing":6}}`
	gw := newTestGateway(t, map[string]bool{"module_config": true})

	res := doGateway(t, gw, http.MethodPatch, "/api/v1/config/levels/lvl-busy", "config-writer", body)
	if res.StatusCode != http.StatusConflict || res.Header.Get("X-Error-Code") != "SCALE_TIGHTENING" {
		t.Errorf("busy level: status %d code %q, want 409 SCALE_TIGHTENING", res.StatusCode, res.Header.Get("X-Error-Code"))
	}
	res = doGateway(t, gw, http.MethodPatch, "/api/v1/config/levels/lvl-1", "config-writer", body)
	if res.StatusCode != http.StatusOK {
		t.Errorf("free level: status %d, want 200", res.StatusCode)
	}
}
