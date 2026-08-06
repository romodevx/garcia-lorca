package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	iamv1 "github.com/garcia-lorca/gen/go/garcialorca/iam/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/iam/v1/iamv1connect"
	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
	"github.com/garcia-lorca/pkg/featureflag"
	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// These tests assemble the exact production routes (newMux) against stub iam and
// students Connect servers, exercising the full security chain of the M03
// students route: ModuleGuard(students) → RequireAuth → RequirePermission.

type errStr string

func (e errStr) Error() string { return string(e) }

// stubIAM validates tokens by name: "with-perm" holds students:create,
// "no-perm" is authenticated but lacks it, anything else is invalid.
type stubIAM struct {
	iamv1connect.UnimplementedIamServiceHandler
}

func (stubIAM) ValidateToken(_ context.Context, req *connect.Request[iamv1.ValidateTokenRequest]) (*connect.Response[iamv1.ValidateTokenResponse], error) {
	perms := map[string][]string{
		"with-perm":        {"students:create"},
		"no-perm":          {"students:read"},
		"config-writer":    {"config:write", "config:read"},
		"config-reader":    {"config:read"},
		"student-enroller": {"students:enroll"},
		"student-reader":   {"students:read"},
	}
	granted, ok := perms[req.Msg.GetAccessToken()]
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errStr("invalid token"))
	}
	return connect.NewResponse(&iamv1.ValidateTokenResponse{Claims: &iamv1.Claims{
		Subject: "u1", TenantId: "t1", Email: "ana@garcialorca.mx", Permissions: granted,
	}}), nil
}

// stubStudents returns a canned pending applicant for any CreateApplicant.
type stubStudents struct {
	studentsv1connect.UnimplementedStudentsServiceHandler
}

func (stubStudents) CreateApplicant(_ context.Context, req *connect.Request[studentsv1.CreateApplicantRequest]) (*connect.Response[studentsv1.CreateApplicantResponse], error) {
	return connect.NewResponse(&studentsv1.CreateApplicantResponse{Student: &studentsv1.Student{
		Id:              "stu-1",
		Curp:            req.Msg.GetCurp(),
		FirstName:       req.Msg.GetFirstName(),
		Level:           req.Msg.GetLevel(),
		AdmissionStatus: studentsv1.AdmissionStatus_ADMISSION_STATUS_PENDING,
	}}), nil
}

func (stubStudents) ListStudents(_ context.Context, req *connect.Request[studentsv1.ListStudentsRequest]) (*connect.Response[studentsv1.ListStudentsResponse], error) {
	return connect.NewResponse(&studentsv1.ListStudentsResponse{Students: []*studentsv1.Student{
		{Id: "stu-1", FirstName: "Ana", AdmissionStatus: studentsv1.AdmissionStatus_ADMISSION_STATUS_PENDING},
	}}), nil
}

func (stubStudents) GetStudent(_ context.Context, req *connect.Request[studentsv1.GetStudentRequest]) (*connect.Response[studentsv1.GetStudentResponse], error) {
	return connect.NewResponse(&studentsv1.GetStudentResponse{Student: &studentsv1.Student{
		Id:              req.Msg.GetId(),
		FirstName:       "Ana",
		AdmissionStatus: studentsv1.AdmissionStatus_ADMISSION_STATUS_ACCEPTED,
	}}), nil
}

func (stubStudents) Enroll(_ context.Context, req *connect.Request[studentsv1.EnrollRequest]) (*connect.Response[studentsv1.EnrollResponse], error) {
	// Echo a canned enrollment with a server-derived cycleId, proving the edge
	// surfaces the derived cycle without ever accepting one from the client.
	return connect.NewResponse(&studentsv1.EnrollResponse{Enrollment: &studentsv1.Enrollment{
		Id:        "enr-1",
		StudentId: req.Msg.GetStudentId(),
		CycleId:   "cyc-1",
		GroupId:   req.Msg.GetGroupId(),
		TermId:    req.Msg.GetTermId(),
		Status:    studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_ACTIVE,
	}}), nil
}

func (stubStudents) ListEnrollments(_ context.Context, req *connect.Request[studentsv1.ListEnrollmentsRequest]) (*connect.Response[studentsv1.ListEnrollmentsResponse], error) {
	return connect.NewResponse(&studentsv1.ListEnrollmentsResponse{Enrollments: []*studentsv1.Enrollment{
		{Id: "enr-1", StudentId: req.Msg.GetStudentId(), CycleId: "cyc-1", GroupId: "grp-1", Status: studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_ACTIVE},
	}}), nil
}

// stubConfig echoes a canned cycle for CreateCycle and one cycle for ListCycles,
// enough to exercise the config route security chain end to end.
type stubConfig struct {
	configv1connect.UnimplementedConfigServiceHandler
}

func (stubConfig) CreateCycle(_ context.Context, req *connect.Request[configv1.CreateCycleRequest]) (*connect.Response[configv1.CreateCycleResponse], error) {
	return connect.NewResponse(&configv1.CreateCycleResponse{Cycle: &configv1.Cycle{
		Id:     "cyc-1",
		Name:   req.Msg.GetName(),
		Status: req.Msg.GetStatus(),
	}}), nil
}

func (stubConfig) ListCycles(_ context.Context, _ *connect.Request[configv1.ListCyclesRequest]) (*connect.Response[configv1.ListCyclesResponse], error) {
	return connect.NewResponse(&configv1.ListCyclesResponse{Cycles: []*configv1.Cycle{
		{Id: "cyc-1", Name: "2026-2027", Status: configv1.CycleStatus_CYCLE_STATUS_DRAFT},
	}}), nil
}

func (stubConfig) CreateTeacher(_ context.Context, req *connect.Request[configv1.CreateTeacherRequest]) (*connect.Response[configv1.CreateTeacherResponse], error) {
	return connect.NewResponse(&configv1.CreateTeacherResponse{Teacher: &configv1.Teacher{
		Id:        "tch-1",
		FirstName: req.Msg.GetFirstName(),
		LastName:  req.Msg.GetLastName(),
		Email:     req.Msg.GetEmail(),
		UserId:    req.Msg.GetUserId(),
	}}), nil
}

// CreateTeachingAssignment echoes a derived cycle to prove the edge surfaces the
// server-derived cycleId without ever accepting one from the client.
func (stubConfig) CreateTeachingAssignment(_ context.Context, req *connect.Request[configv1.CreateTeachingAssignmentRequest]) (*connect.Response[configv1.CreateTeachingAssignmentResponse], error) {
	return connect.NewResponse(&configv1.CreateTeachingAssignmentResponse{TeachingAssignment: &configv1.TeachingAssignment{
		Id:        "asg-1",
		CycleId:   "cyc-1",
		GroupId:   req.Msg.GetGroupId(),
		SubjectId: req.Msg.GetSubjectId(),
		TeacherId: req.Msg.GetTeacherId(),
	}}), nil
}

func connectServer(t *testing.T, path string, handler http.Handler) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// newTestGateway wires newMux against stub iam + students servers, with the given
// flag map (keyed by full flag key, e.g. "module_students").
func newTestGateway(t *testing.T, flags map[string]bool) *httptest.Server {
	t.Helper()
	iamPath, iamHandler := iamv1connect.NewIamServiceHandler(stubIAM{})
	iamSrv := connectServer(t, iamPath, iamHandler)
	stuPath, stuHandler := studentsv1connect.NewStudentsServiceHandler(stubStudents{})
	stuSrv := connectServer(t, stuPath, stuHandler)
	cfgPath, cfgHandler := configv1connect.NewConfigServiceHandler(stubConfig{})
	cfgSrv := connectServer(t, cfgPath, cfgHandler)

	// A generous burst keeps the limiter out of the way of these route tests;
	// rate-limit behavior is covered directly in the middleware package.
	loginLimiter := middleware.NewRateLimiter(1000, 1000, time.Now)
	mux := newMux(
		client.NewIAMClient(iamSrv.URL, iamSrv.Client()),
		client.NewAuthClient(iamSrv.URL, iamSrv.Client()),
		client.NewStudentsClient(stuSrv.URL, stuSrv.Client()),
		client.NewConfigClient(cfgSrv.URL, cfgSrv.Client()),
		featureflag.NewInMemory(flags),
		loginLimiter,
	)
	gw := httptest.NewServer(mux)
	t.Cleanup(gw.Close)
	return gw
}

const validBody = `{"curp":"HEGA050312MDFRNN08","firstName":"Ana","lastName":"Herrera","level":"primaria"}`

func postStudent(t *testing.T, gw *httptest.Server, token, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, gw.URL+"/api/v1/students", strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := gw.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

func TestStudentsRouteDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_students OFF
	// Even a caller with the right token must not learn the module exists.
	res := postStudent(t, gw, "with-perm", validBody)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestStudentsRouteEnabledNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := postStudent(t, gw, "", validBody)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestStudentsRouteEnabledMissingPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := postStudent(t, gw, "no-perm", validBody)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestStudentsRouteEnabledWithPermissionCreates201(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := postStudent(t, gw, "with-perm", validBody)
	if res.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", res.StatusCode)
	}
}

// --- students read routes (list/get): ModuleGuard(students) → RequireAuth →
// RequirePermission(students:read). These back the enroll UI's student picker
// and reuse the exact studentsRead helper, so the whole guard chain applies.

func TestListStudentsRouteDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_students OFF
	// Even a caller with students:read must not learn the module exists.
	res := doGateway(t, gw, http.MethodGet, "/api/v1/students", "student-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestListStudentsRouteNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/students", "", "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestListStudentsRouteWithReadPermissionLists200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/students?status=pending", "student-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out struct {
		Students []map[string]any `json:"students"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Students) != 1 || out.Students[0]["id"] != "stu-1" {
		t.Errorf("body = %v", out)
	}
}

func TestGetStudentRouteDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_students OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/students/stu-1", "student-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestGetStudentRouteNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/students/stu-1", "", "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestGetStudentRouteWithReadPermission200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/students/stu-9", "student-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["id"] != "stu-9" {
		t.Errorf("body = %v", out)
	}
}

// --- config catalog routes: ModuleGuard(config) → RequireAuth → RequirePermission

const validCycleBody = `{"name":"2026-2027","status":"draft","startsOn":"2026-08-01","endsOn":"2027-07-15"}`

func doGateway(t *testing.T, gw *httptest.Server, method, path, token, body string) *http.Response {
	t.Helper()
	var r *http.Request
	var err error
	if body == "" {
		r, err = http.NewRequest(method, gw.URL+path, nil)
	} else {
		r, err = http.NewRequest(method, gw.URL+path, strings.NewReader(body))
	}
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := gw.Client().Do(r)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

func TestConfigRouteDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_config OFF
	// Even a caller with config:write must not learn the module exists.
	res := doGateway(t, gw, http.MethodPost, "/api/v1/config/cycles", "config-writer", validCycleBody)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestConfigWriteNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/config/cycles", "", validCycleBody)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestConfigWriteReadOnlyPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true})
	// A read-only principal may not create.
	res := doGateway(t, gw, http.MethodPost, "/api/v1/config/cycles", "config-reader", validCycleBody)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestConfigWriteWithPermissionCreates201(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/config/cycles", "config-writer", validCycleBody)
	if res.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", res.StatusCode)
	}
}

func TestConfigReadWithReadPermissionLists200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/config/cycles", "config-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
}

func TestConfigReadDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_config OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/config/cycles", "config-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

// The teachers + teaching-assignments routes reuse the exact configWrite/
// configRead helpers, so the whole guard chain (flag → auth → permission) must
// apply to them identically. These assert that boundary on the new routes.

const validAssignmentBody = `{"groupId":"grp-1","subjectId":"sub-1","teacherId":"tch-1"}`

func TestTeachingAssignmentDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_config OFF
	res := doGateway(t, gw, http.MethodPost, "/api/v1/config/teaching-assignments", "config-writer", validAssignmentBody)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestTeachingAssignmentWriteNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/config/teaching-assignments", "", validAssignmentBody)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestTeachingAssignmentWriteReadOnlyPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/config/teaching-assignments", "config-reader", validAssignmentBody)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestTeachingAssignmentWriteWithPermissionCreates201(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/config/teaching-assignments", "config-writer", validAssignmentBody)
	if res.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", res.StatusCode)
	}
}

func TestTeacherWriteWithPermissionCreates201(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/config/teachers", "config-writer",
		`{"firstName":"Ada","lastName":"Lovelace","email":"ada@example.com"}`)
	if res.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", res.StatusCode)
	}
}

// --- enrollment routes: ModuleGuard(students) → RequireAuth → RequirePermission
// Writes (enroll/transfer/withdraw) need students:enroll; reads need students:read.

const validEnrollBody = `{"studentId":"stu-1","groupId":"grp-1"}`

func TestEnrollRouteDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_students OFF
	// Even a caller with students:enroll must not learn the module exists.
	res := doGateway(t, gw, http.MethodPost, "/api/v1/enrollments", "student-enroller", validEnrollBody)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestEnrollRouteNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/enrollments", "", validEnrollBody)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestEnrollRouteReadOnlyPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	// A read-only principal (students:read) may not enroll.
	res := doGateway(t, gw, http.MethodPost, "/api/v1/enrollments", "student-reader", validEnrollBody)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestEnrollRouteWithEnrollPermissionCreates201(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/enrollments", "student-enroller", validEnrollBody)
	if res.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", res.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// cycleId is derived server-side; the client never sends it.
	if out["cycleId"] != "cyc-1" || out["status"] != "active" {
		t.Errorf("body = %v", out)
	}
}

func TestListEnrollmentsDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_students OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/enrollments?studentId=stu-1", "student-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestListEnrollmentsWithReadPermissionLists200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/enrollments?studentId=stu-1", "student-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
}

func TestTransferRouteReadOnlyPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	// transfer is a write: students:read is insufficient.
	res := doGateway(t, gw, http.MethodPost, "/api/v1/enrollments/transfer", "student-reader",
		`{"enrollmentId":"enr-1","toGroupId":"grp-2","reason":"reassigned"}`)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}
