package main

import (
	"context"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
)

// T15 period grade stubs: every call echoes, so a route test observes only the
// security chain (flags, auth, permissions, ownership).

func (stubGrades) GetPeriodSheet(_ context.Context, req *connect.Request[gradesv1.GetPeriodSheetRequest]) (*connect.Response[gradesv1.GetPeriodSheetResponse], error) {
	return connect.NewResponse(&gradesv1.GetPeriodSheetResponse{
		AssignmentId: req.Msg.GetAssignmentId(), PeriodId: "per-1", Today: "2026-10-01",
		Periods:        []*gradesv1.EvaluationPeriodRef{{Id: "per-1", Ordinal: 1, Name: "1.er trimestre", StartsOn: "2026-08-24", EndsOn: "2026-11-30"}},
		Scale:          &gradesv1.GradingScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600},
		GradingEnabled: true, Editable: true,
		Rows: []*gradesv1.PeriodSheetRow{{EnrollmentId: "enr-1", StudentId: "stu-1", Value: "9"}},
	}), nil
}

func (stubGrades) UpsertPeriodGrade(_ context.Context, req *connect.Request[gradesv1.UpsertPeriodGradeRequest]) (*connect.Response[gradesv1.UpsertPeriodGradeResponse], error) {
	return connect.NewResponse(&gradesv1.UpsertPeriodGradeResponse{Grade: &gradesv1.PeriodGrade{
		EnrollmentId: req.Msg.GetEnrollmentId(), SubjectId: "sub-1", PeriodId: req.Msg.GetPeriodId(), Value: req.Msg.GetValue(),
	}}), nil
}

func (stubGrades) ClearPeriodGrade(context.Context, *connect.Request[gradesv1.ClearPeriodGradeRequest]) (*connect.Response[gradesv1.ClearPeriodGradeResponse], error) {
	return connect.NewResponse(&gradesv1.ClearPeriodGradeResponse{}), nil
}

func (stubGrades) GetEnrollmentPeriodGrades(_ context.Context, req *connect.Request[gradesv1.GetEnrollmentPeriodGradesRequest]) (*connect.Response[gradesv1.GetEnrollmentPeriodGradesResponse], error) {
	return connect.NewResponse(&gradesv1.GetEnrollmentPeriodGradesResponse{
		CycleId: "cyc-1", CycleName: "2026-2027",
		Periods:        []*gradesv1.EvaluationPeriodRef{{Id: "per-1", Ordinal: 1, Name: "1.er trimestre", StartsOn: "2026-08-24", EndsOn: "2026-11-30"}},
		Scale:          &gradesv1.GradingScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600},
		GradingEnabled: true, SubjectIds: []string{"sub-1"},
		Grades: []*gradesv1.PeriodGrade{{EnrollmentId: req.Msg.GetEnrollmentId(), SubjectId: "sub-1", PeriodId: "per-1", Value: "9"}},
	}), nil
}

// PeriodsWithGrades: period "per-graded" holds grades; no other period does.
func (stubGrades) PeriodsWithGrades(_ context.Context, req *connect.Request[gradesv1.PeriodsWithGradesRequest]) (*connect.Response[gradesv1.PeriodsWithGradesResponse], error) {
	out := []string{}
	for _, id := range req.Msg.GetPeriodIds() {
		if id == periodGraded {
			out = append(out, id)
		}
	}
	return connect.NewResponse(&gradesv1.PeriodsWithGradesResponse{PeriodIds: out}), nil
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
	periodForeignAsg = "a0000000-0000-0000-0000-00000000ffff"
	periodEnr        = "e0000000-0000-0000-0000-000000000001"
	periodID         = "0e000000-0000-0000-0000-000000000001"
	periodGraded     = "0e000000-0000-0000-0000-0000000000aa"
)

var (
	gradesModulesOn         = map[string]bool{"module_grades": true, "module_config": true}
	periodFamilyModulesOn   = map[string]bool{"module_grades": true, "module_students": true}
	periodPutBody           = `{"assignmentId":"` + periodForeignAsg + `","periodId":"` + periodID + `","enrollmentId":"` + periodEnr + `","value":9}`
	periodDeleteQuery       = "?assignmentId=" + periodForeignAsg + "&periodId=" + periodID + "&enrollmentId=" + periodEnr
	directorSheetPath       = "/api/v1/grades/period-sheet?assignmentId=" + periodForeignAsg
	teacherSheetForeignPath = "/api/v1/teaching/grades/period-sheet?assignmentId=" + periodForeignAsg
)

func TestPeriodGradeRoutes(t *testing.T) {
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
		{"director put", gradesModulesOn, http.MethodPut, "/api/v1/grades/period-grades", "grades-writer", periodPutBody, http.StatusOK},
		{"director put needs grades:write", gradesModulesOn, http.MethodPut, "/api/v1/grades/period-grades", "grades-reader", periodPutBody, http.StatusForbidden},
		{"director delete", gradesModulesOn, http.MethodDelete, "/api/v1/grades/period-grades" + periodDeleteQuery, "grades-writer", "", http.StatusNoContent},
		{"director delete needs grades:write", gradesModulesOn, http.MethodDelete, "/api/v1/grades/period-grades" + periodDeleteQuery, "grades-reader", "", http.StatusForbidden},
		// the retired monthly routes are gone
		{"monthly sheet is gone", gradesModulesOn, http.MethodGet, "/api/v1/grades/sheet?assignmentId=" + periodForeignAsg, "grades-reader", "", http.StatusNotFound},
		{"monthly put is gone", gradesModulesOn, http.MethodPut, "/api/v1/grades/monthly", "grades-writer", periodPutBody, http.StatusNotFound},
		{"teacher monthly put is gone", gradesModulesOn, http.MethodPut, "/api/v1/teaching/grades/monthly", "teaching-writer", periodPutBody, http.StatusNotFound},
		{"family monthly is gone", periodFamilyModulesOn, http.MethodGet, "/api/v1/family/students/stu-1/grades/monthly", "family-reader", "", http.StatusNotFound},
		{"director terms", gradesModulesOn, http.MethodGet, "/api/v1/grades/terms?assignmentId=asg-1", "grades-reader", "", http.StatusOK},
		{"director terms config OFF", map[string]bool{"module_grades": true}, http.MethodGet, "/api/v1/grades/terms?assignmentId=asg-1", "grades-reader", "", http.StatusNotFound},
		// teacher
		{"teacher sheet foreign assignment is 404", gradesModulesOn, http.MethodGet, teacherSheetForeignPath, "teaching-reader", "", http.StatusNotFound},
		{"teacher sheet config OFF", map[string]bool{"module_grades": true}, http.MethodGet, teacherSheetForeignPath, "teaching-reader", "", http.StatusNotFound},
		{"teacher sheet no token", gradesModulesOn, http.MethodGet, teacherSheetForeignPath, "", "", http.StatusUnauthorized},
		{"teacher sheet grades:read is not teaching:read", gradesModulesOn, http.MethodGet, teacherSheetForeignPath, "grades-reader", "", http.StatusForbidden},
		{"teacher put foreign assignment is 404", gradesModulesOn, http.MethodPut, "/api/v1/teaching/grades/period-grades", "teaching-writer", periodPutBody, http.StatusNotFound},
		{"teacher put needs teaching:write", gradesModulesOn, http.MethodPut, "/api/v1/teaching/grades/period-grades", "teaching-reader", periodPutBody, http.StatusForbidden},
		{"teacher delete foreign assignment is 404", gradesModulesOn, http.MethodDelete, "/api/v1/teaching/grades/period-grades" + periodDeleteQuery, "teaching-writer", "", http.StatusNotFound},
		// family
		{"family periods", periodFamilyModulesOn, http.MethodGet, "/api/v1/family/students/stu-1/grades/periods", "family-reader", "", http.StatusOK},
		{"family periods other child", periodFamilyModulesOn, http.MethodGet, "/api/v1/family/students/stu-999/grades/periods", "family-reader", "", http.StatusNotFound},
		{"family periods grades OFF", map[string]bool{"module_students": true}, http.MethodGet, "/api/v1/family/students/stu-1/grades/periods", "family-reader", "", http.StatusNotFound},
		{"family periods students OFF", map[string]bool{"module_grades": true}, http.MethodGet, "/api/v1/family/students/stu-1/grades/periods", "family-reader", "", http.StatusNotFound},
		{"family periods needs family:read", periodFamilyModulesOn, http.MethodGet, "/api/v1/family/students/stu-1/grades/periods", "grades-reader", "", http.StatusForbidden},
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

// ClosedTerms: term "trm-closed" is a closed period; every other term is open.
func (stubGrades) ClosedTerms(_ context.Context, req *connect.Request[gradesv1.ClosedTermsRequest]) (*connect.Response[gradesv1.ClosedTermsResponse], error) {
	closed := []string{}
	for _, id := range req.Msg.GetTermIds() {
		if id == "trm-closed" {
			closed = append(closed, id)
		}
	}
	return connect.NewResponse(&gradesv1.ClosedTermsResponse{ClosedTermIds: closed}), nil
}

// T7: PUT /config/terms/{id}/months asks grades first; a closed period is 409
// TERM_CLOSED and config is never reached.
func TestTermMonthsOfAClosedPeriodAre409(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true})
	res := doGateway(t, gw, http.MethodPut, "/api/v1/config/terms/trm-closed/months", "config-writer", `{"months":["2026-09"]}`)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", res.StatusCode)
	}
	if got := res.Header.Get("X-Error-Code"); got != "TERM_CLOSED" {
		t.Errorf("X-Error-Code = %q, want TERM_CLOSED", got)
	}
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
