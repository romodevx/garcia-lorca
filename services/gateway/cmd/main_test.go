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
	"google.golang.org/protobuf/types/known/timestamppb"

	attendancev1 "github.com/garcia-lorca/gen/go/garcialorca/attendance/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/attendance/v1/attendancev1connect"
	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/finance/v1/financev1connect"
	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/grades/v1/gradesv1connect"
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
		"grades-writer":    {"grades:write", "grades:read"},
		"grades-reader":    {"grades:read"},
		"finance-writer":   {"finance:write", "finance:read"},
		"finance-reader":   {"finance:read"},
		"att-writer":       {"attendance:write", "attendance:read"},
		"att-reader":       {"attendance:read"},
		"users-writer":     {"users:write", "users:read"},
		"users-reader":     {"users:read"},
		"roles-writer":     {"roles:write", "roles:read"},
		"roles-reader":     {"roles:read"},
	}
	granted, ok := perms[req.Msg.GetAccessToken()]
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errStr("invalid token"))
	}
	return connect.NewResponse(&iamv1.ValidateTokenResponse{Claims: &iamv1.Claims{
		Subject: "u1", TenantId: "t1", Email: "ana@garcialorca.mx", Permissions: granted,
	}}), nil
}

// iam admin RPCs on the stub: enough canned behavior to drive the route-security
// chain for the user/role admin surface and the teacher-link user validation.

func (stubIAM) CreateUser(_ context.Context, req *connect.Request[iamv1.CreateUserRequest]) (*connect.Response[iamv1.CreateUserResponse], error) {
	return connect.NewResponse(&iamv1.CreateUserResponse{User: &iamv1.User{
		Id: "usr-1", Email: req.Msg.GetEmail(), DisplayName: req.Msg.GetDisplayName(),
		Status: iamv1.UserStatus_USER_STATUS_ACTIVE, MustChangePassword: true, RoleIds: req.Msg.GetRoleIds(),
	}}), nil
}

func (stubIAM) GetUser(_ context.Context, req *connect.Request[iamv1.GetUserRequest]) (*connect.Response[iamv1.GetUserResponse], error) {
	if req.Msg.GetId() == "ghost" {
		return nil, connect.NewError(connect.CodeNotFound, errStr("no user"))
	}
	return connect.NewResponse(&iamv1.GetUserResponse{User: &iamv1.User{
		Id: req.Msg.GetId(), Email: "prof@example.com", Status: iamv1.UserStatus_USER_STATUS_ACTIVE,
	}}), nil
}

func (stubIAM) ListUsers(_ context.Context, _ *connect.Request[iamv1.ListUsersRequest]) (*connect.Response[iamv1.ListUsersResponse], error) {
	return connect.NewResponse(&iamv1.ListUsersResponse{Users: []*iamv1.User{
		{Id: "usr-1", Email: "prof@example.com", Status: iamv1.UserStatus_USER_STATUS_ACTIVE},
	}}), nil
}

func (stubIAM) CreateRole(_ context.Context, req *connect.Request[iamv1.CreateRoleRequest]) (*connect.Response[iamv1.CreateRoleResponse], error) {
	return connect.NewResponse(&iamv1.CreateRoleResponse{Role: &iamv1.Role{
		Id: "rol-1", Name: req.Msg.GetName(), Permissions: req.Msg.GetPermissions(),
	}}), nil
}

func (stubIAM) ListRoles(_ context.Context, _ *connect.Request[iamv1.ListRolesRequest]) (*connect.Response[iamv1.ListRolesResponse], error) {
	return connect.NewResponse(&iamv1.ListRolesResponse{Roles: []*iamv1.Role{
		{Id: "rol-1", Name: "docente", Permissions: []string{"grades:read"}},
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

func (stubConfig) SetTeacherUser(_ context.Context, req *connect.Request[configv1.SetTeacherUserRequest]) (*connect.Response[configv1.SetTeacherUserResponse], error) {
	return connect.NewResponse(&configv1.SetTeacherUserResponse{Teacher: &configv1.Teacher{
		Id: req.Msg.GetTeacherId(), UserId: req.Msg.GetUserId(),
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

// stubGrades echoes a canned grade entry / roster / list, enough to exercise the
// grades route security chain end to end. CaptureGrade echoes the derived
// subject/teacher to prove the edge surfaces them without accepting them.
type stubGrades struct {
	gradesv1connect.UnimplementedGradesServiceHandler
}

func (stubGrades) CaptureGrade(_ context.Context, req *connect.Request[gradesv1.CaptureGradeRequest]) (*connect.Response[gradesv1.CaptureGradeResponse], error) {
	return connect.NewResponse(&gradesv1.CaptureGradeResponse{Entry: &gradesv1.GradeEntry{
		Id:           "grd-1",
		EnrollmentId: req.Msg.GetEnrollmentId(),
		SubjectId:    "sub-1",
		TermId:       req.Msg.GetTermId(),
		TeacherId:    "tch-1",
		Value:        req.Msg.GetValue(),
	}}), nil
}

func (stubGrades) GetRoster(_ context.Context, req *connect.Request[gradesv1.GetRosterRequest]) (*connect.Response[gradesv1.GetRosterResponse], error) {
	return connect.NewResponse(&gradesv1.GetRosterResponse{Rows: []*gradesv1.RosterRow{
		{EnrollmentId: "enr-1", StudentId: "stu-1", Value: 8.5, HasValue: true},
	}}), nil
}

func (stubGrades) ListGradeEntries(_ context.Context, req *connect.Request[gradesv1.ListGradeEntriesRequest]) (*connect.Response[gradesv1.ListGradeEntriesResponse], error) {
	return connect.NewResponse(&gradesv1.ListGradeEntriesResponse{Entries: []*gradesv1.GradeEntry{
		{Id: "grd-1", EnrollmentId: req.Msg.GetEnrollmentId(), SubjectId: "sub-1", TermId: req.Msg.GetTermId(), TeacherId: "tch-1", Value: 8.5},
	}}), nil
}

// stubFinance echoes canned concepts, charges and payments, enough to exercise
// the finance route security chain end to end. Money stays integer cents.
type stubFinance struct {
	financev1connect.UnimplementedFinanceServiceHandler
}

func (stubFinance) CreateConcept(_ context.Context, req *connect.Request[financev1.CreateConceptRequest]) (*connect.Response[financev1.CreateConceptResponse], error) {
	return connect.NewResponse(&financev1.CreateConceptResponse{Concept: &financev1.ChargeConcept{
		Id: "cpt-1", Name: req.Msg.GetName(), DefaultAmountCents: req.Msg.GetDefaultAmountCents(),
		Recurrence: req.Msg.GetRecurrence(), Active: true,
	}}), nil
}

func (stubFinance) ListConcepts(_ context.Context, _ *connect.Request[financev1.ListConceptsRequest]) (*connect.Response[financev1.ListConceptsResponse], error) {
	return connect.NewResponse(&financev1.ListConceptsResponse{Concepts: []*financev1.ChargeConcept{
		{Id: "cpt-1", Name: "Colegiatura", DefaultAmountCents: 150000, Recurrence: "monthly", Active: true},
	}}), nil
}

func (stubFinance) CreateCharge(_ context.Context, req *connect.Request[financev1.CreateChargeRequest]) (*connect.Response[financev1.CreateChargeResponse], error) {
	return connect.NewResponse(&financev1.CreateChargeResponse{Charge: &financev1.Charge{
		Id: "chg-1", StudentId: req.Msg.GetStudentId(), CycleId: req.Msg.GetCycleId(), ConceptId: req.Msg.GetConceptId(),
		Description: "Colegiatura", AmountCents: 150000, DueDate: req.Msg.GetDueDate(),
		Status: "pending", RemainingCents: 150000,
	}}), nil
}

func (stubFinance) GenerateCharges(_ context.Context, _ *connect.Request[financev1.GenerateChargesRequest]) (*connect.Response[financev1.GenerateChargesResponse], error) {
	return connect.NewResponse(&financev1.GenerateChargesResponse{CreatedCount: 2, SkippedCount: 1}), nil
}

func (stubFinance) ListCharges(_ context.Context, req *connect.Request[financev1.ListChargesRequest]) (*connect.Response[financev1.ListChargesResponse], error) {
	return connect.NewResponse(&financev1.ListChargesResponse{Charges: []*financev1.Charge{
		{Id: "chg-1", StudentId: req.Msg.GetStudentId(), AmountCents: 150000, Status: "pending", RemainingCents: 150000},
	}}), nil
}

func (stubFinance) RegisterPayment(_ context.Context, req *connect.Request[financev1.RegisterPaymentRequest]) (*connect.Response[financev1.RegisterPaymentResponse], error) {
	return connect.NewResponse(&financev1.RegisterPaymentResponse{
		Payment: &financev1.Payment{
			Id: "pay-1", ChargeId: req.Msg.GetChargeId(), AmountCents: req.Msg.GetAmountCents(),
			Method: req.Msg.GetMethod(), Reference: req.Msg.GetReference(),
			PaidAt: timestamppb.New(time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)),
		},
		Charge: &financev1.Charge{
			Id: req.Msg.GetChargeId(), AmountCents: 150000, PaidCents: req.Msg.GetAmountCents(),
			RemainingCents: 150000 - req.Msg.GetAmountCents(), Status: "partially_paid",
		},
	}), nil
}

func (stubFinance) GetAccount(_ context.Context, req *connect.Request[financev1.GetAccountRequest]) (*connect.Response[financev1.GetAccountResponse], error) {
	return connect.NewResponse(&financev1.GetAccountResponse{
		Charges:           []*financev1.Charge{{Id: "chg-1", StudentId: req.Msg.GetStudentId(), AmountCents: 150000, RemainingCents: 150000}},
		TotalBalanceCents: 150000,
	}), nil
}

func (stubFinance) ListOverdue(_ context.Context, _ *connect.Request[financev1.ListOverdueRequest]) (*connect.Response[financev1.ListOverdueResponse], error) {
	return connect.NewResponse(&financev1.ListOverdueResponse{Charges: []*financev1.Charge{
		{Id: "chg-1", DueDate: "2026-08-05", AmountCents: 150000, RemainingCents: 150000, Status: "pending"},
	}}), nil
}

// stubAttendance echoes a canned roster / save summary / record list, enough to
// exercise the attendance route security chain end to end. The default-present
// model shows up in the roster: a row with no stored mark reads "present".
type stubAttendance struct {
	attendancev1connect.UnimplementedAttendanceServiceHandler
}

func (stubAttendance) GetDailyRoster(_ context.Context, _ *connect.Request[attendancev1.GetDailyRosterRequest]) (*connect.Response[attendancev1.GetDailyRosterResponse], error) {
	return connect.NewResponse(&attendancev1.GetDailyRosterResponse{Rows: []*attendancev1.RosterRow{
		{EnrollmentId: "enr-1", StudentId: "stu-1", Status: "present"},
		{EnrollmentId: "enr-2", StudentId: "stu-2", Status: "absent"},
	}}), nil
}

func (stubAttendance) SaveDailyAttendance(_ context.Context, req *connect.Request[attendancev1.SaveDailyAttendanceRequest]) (*connect.Response[attendancev1.SaveDailyAttendanceResponse], error) {
	var absent int32
	for _, m := range req.Msg.GetMarks() {
		if m.GetStatus() == "absent" {
			absent++
		}
	}
	return connect.NewResponse(&attendancev1.SaveDailyAttendanceResponse{
		AbsentCount: absent, MarkedCount: int32(len(req.Msg.GetMarks())),
	}), nil
}

func (stubAttendance) ListAttendance(_ context.Context, req *connect.Request[attendancev1.ListAttendanceRequest]) (*connect.Response[attendancev1.ListAttendanceResponse], error) {
	return connect.NewResponse(&attendancev1.ListAttendanceResponse{Records: []*attendancev1.AttendanceRecord{
		{Id: "att-1", EnrollmentId: req.Msg.GetEnrollmentId(), Date: "2026-09-01", Status: "absent", Note: "cita médica"},
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
	grdPath, grdHandler := gradesv1connect.NewGradesServiceHandler(stubGrades{})
	grdSrv := connectServer(t, grdPath, grdHandler)
	finPath, finHandler := financev1connect.NewFinanceServiceHandler(stubFinance{})
	finSrv := connectServer(t, finPath, finHandler)
	attPath, attHandler := attendancev1connect.NewAttendanceServiceHandler(stubAttendance{})
	attSrv := connectServer(t, attPath, attHandler)

	// A generous burst keeps the limiter out of the way of these route tests;
	// rate-limit behavior is covered directly in the middleware package.
	loginLimiter := middleware.NewRateLimiter(1000, 1000, time.Now)
	mux := newMux(
		client.NewIAMClient(iamSrv.URL, iamSrv.Client()),
		client.NewAuthClient(iamSrv.URL, iamSrv.Client()),
		client.NewStudentsClient(stuSrv.URL, stuSrv.Client()),
		client.NewConfigClient(cfgSrv.URL, cfgSrv.Client()),
		client.NewGradesClient(grdSrv.URL, grdSrv.Client()),
		client.NewFinanceClient(finSrv.URL, finSrv.Client()),
		client.NewAttendanceClient(attSrv.URL, attSrv.Client()),
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

// --- grades routes: ModuleGuard(grades) → RequireAuth → RequirePermission.
// Capture is a write (grades:write); roster + list are reads (grades:read).
// Every route sits behind guard.Protect("grades", …) so module_grades OFF ⇒ 404.

const validCaptureBody = `{"assignmentId":"asg-1","enrollmentId":"enr-1","termId":"trm-1","value":8.5}`

func TestGradesCaptureDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_grades OFF
	// Even a caller with grades:write must not learn the module exists.
	res := doGateway(t, gw, http.MethodPost, "/api/v1/grades", "grades-writer", validCaptureBody)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestGradesCaptureNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_grades": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/grades", "", validCaptureBody)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestGradesCaptureReadOnlyPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_grades": true})
	// capture is a write: grades:read is insufficient.
	res := doGateway(t, gw, http.MethodPost, "/api/v1/grades", "grades-reader", validCaptureBody)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestGradesCaptureWithWritePermission200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_grades": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/grades", "grades-writer", validCaptureBody)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// subject/teacher are derived server-side; the client never sends them.
	if out["id"] != "grd-1" || out["value"] != 8.5 || out["subjectId"] != "sub-1" || out["teacherId"] != "tch-1" {
		t.Errorf("body = %v", out)
	}
}

func TestGradesRosterDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_grades OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/grades/roster?assignmentId=asg-1&termId=trm-1", "grades-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestGradesRosterNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_grades": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/grades/roster?assignmentId=asg-1&termId=trm-1", "", "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestGradesRosterWithReadPermissionLists200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_grades": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/grades/roster?assignmentId=asg-1&termId=trm-1", "grades-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out struct {
		Roster []map[string]any `json:"roster"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Roster) != 1 || out.Roster[0]["enrollmentId"] != "enr-1" || out.Roster[0]["value"] != 8.5 {
		t.Errorf("body = %v", out)
	}
}

func TestGradesListReadOnlyPermissionLists200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_grades": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/grades?enrollmentId=enr-1&termId=trm-1", "grades-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
}

func TestGradesListDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_grades OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/grades?enrollmentId=enr-1", "grades-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

// --- iam admin routes (users/roles): ModuleGuard(iam) → RequireAuth →
// RequirePermission. User routes gate on users:* , role routes on roles:* .
// Every route sits behind guard.Protect("iam", …) so module_iam OFF ⇒ 404.

const validCreateUserBody = `{"email":"prof@example.com","displayName":"Prof","tempPassword":"init-pw-123","roleIds":["rol-1"]}`
const validCreateRoleBody = `{"name":"docente","permissions":["students:read","grades:read","grades:write"]}`

func TestUsersRouteDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_iam OFF
	// Even a caller with users:write must not learn the admin surface exists.
	res := doGateway(t, gw, http.MethodPost, "/api/v1/users", "users-writer", validCreateUserBody)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestUsersRouteNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_iam": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/users", "", validCreateUserBody)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestUsersWriteReadOnlyPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_iam": true})
	// users:read is insufficient to create a user.
	res := doGateway(t, gw, http.MethodPost, "/api/v1/users", "users-reader", validCreateUserBody)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestUsersWriteWithPermissionCreates201(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_iam": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/users", "users-writer", validCreateUserBody)
	if res.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", res.StatusCode)
	}
}

func TestUsersListWithReadPermission200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_iam": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/users", "users-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
}

func TestUsersListDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_iam OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/users", "users-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestRolesRouteDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_iam OFF
	res := doGateway(t, gw, http.MethodPost, "/api/v1/roles", "roles-writer", validCreateRoleBody)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestRolesWriteReadOnlyPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_iam": true})
	// roles:read is insufficient to create a role.
	res := doGateway(t, gw, http.MethodPost, "/api/v1/roles", "roles-reader", validCreateRoleBody)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestRolesWriteWithPermissionCreates201(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_iam": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/roles", "roles-writer", validCreateRoleBody)
	if res.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", res.StatusCode)
	}
}

func TestRolesListWithReadPermission200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_iam": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/roles", "roles-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
}

// Users routes must NOT be reachable with roles:* (and vice versa): the
// permission is resource-scoped, so a roles-only principal is forbidden on users.
func TestUsersWriteWithRolesPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_iam": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/users", "roles-writer", validCreateUserBody)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

// --- teacher↔user link: ModuleGuard(config) → RequireAuth → config:write. The
// handler validates the iam user (GetUser) before the config write, so a bogus
// user id maps to 404 and no link is stored.

func TestTeacherLinkDisabledConfigModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_iam": true}) // module_config OFF
	res := doGateway(t, gw, http.MethodPost, "/api/v1/config/teachers/tch-1/user", "config-writer", `{"userId":"usr-1"}`)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestTeacherLinkNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/config/teachers/tch-1/user", "", `{"userId":"usr-1"}`)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestTeacherLinkReadOnlyPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/config/teachers/tch-1/user", "config-reader", `{"userId":"usr-1"}`)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestTeacherLinkWithPermission200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/config/teachers/tch-1/user", "config-writer", `{"userId":"usr-1"}`)
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
}

// A bogus user id fails the gateway-orchestrated iam validation before any
// config write: iam GetUser NotFound ⇒ 404.
func TestTeacherLinkBogusUserIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/config/teachers/tch-1/user", "config-writer", `{"userId":"ghost"}`)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

// --- finance routes (Phase 7): ModuleGuard(finance) → RequireAuth →
// RequirePermission. Concepts/charges/discount/cancel/payments are writes
// (finance:write); concept list, charge list, account and overdue are reads
// (finance:read). Every route sits behind guard.Protect("finance", …) so
// module_finance OFF ⇒ 404, before auth runs.

const validConceptBody = `{"name":"Colegiatura","defaultAmountCents":150000,"recurrence":"monthly"}`
const validChargeBody = `{"studentId":"stu-1","cycleId":"cyc-1","conceptId":"cpt-1","dueDate":"2026-09-05"}`
const validPaymentBody = `{"chargeId":"chg-1","amountCents":50000,"method":"transfer","reference":"SPEI-993"}`

func TestFinanceConceptCreateDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_finance OFF
	// Even a caller with finance:write must not learn the module exists.
	res := doGateway(t, gw, http.MethodPost, "/api/v1/finance/concepts", "finance-writer", validConceptBody)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestFinanceConceptCreateNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_finance": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/finance/concepts", "", validConceptBody)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestFinanceConceptCreateReadOnlyPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_finance": true})
	// finance:read is insufficient to define a charge concept.
	res := doGateway(t, gw, http.MethodPost, "/api/v1/finance/concepts", "finance-reader", validConceptBody)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestFinanceConceptCreateWithWritePermission201(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_finance": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/finance/concepts", "finance-writer", validConceptBody)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", res.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Money crosses the whole edge as integer cents.
	if out["id"] != "cpt-1" || out["defaultAmountCents"] != float64(150000) {
		t.Errorf("body = %v", out)
	}
}

func TestFinanceConceptListWithReadPermission200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_finance": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/finance/concepts", "finance-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
}

func TestFinanceConceptListDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_finance OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/finance/concepts", "finance-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestFinanceChargeCreateWithWritePermission201(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_finance": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/finance/charges", "finance-writer", validChargeBody)
	if res.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", res.StatusCode)
	}
}

// /charges/generate is a more specific pattern than /charges: ServeMux must
// route it to the bulk handler, and it must carry the same write guard.
func TestFinanceGenerateRouteWithWritePermission200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_finance": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/finance/charges/generate", "finance-writer",
		`{"conceptId":"cpt-1","cycleId":"cyc-1","dueDate":"2026-09-05"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["created"] != float64(2) || out["skipped"] != float64(1) {
		t.Errorf("body = %v", out)
	}
}

func TestFinanceGenerateRouteReadOnlyPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_finance": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/finance/charges/generate", "finance-reader",
		`{"conceptId":"cpt-1","cycleId":"cyc-1","dueDate":"2026-09-05"}`)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestFinanceDiscountRouteReadOnlyPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_finance": true})
	// applying a discount is a write: finance:read is insufficient.
	res := doGateway(t, gw, http.MethodPost, "/api/v1/finance/charges/chg-1/discount", "finance-reader",
		`{"discountCents":50000,"reason":"beca"}`)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestFinanceCancelRouteDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_finance OFF
	res := doGateway(t, gw, http.MethodPost, "/api/v1/finance/charges/chg-1/cancel", "finance-writer", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestFinancePaymentWithWritePermission201(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_finance": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/finance/payments", "finance-writer", validPaymentBody)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", res.StatusCode)
	}
	var out struct {
		Payment map[string]any `json:"payment"`
		Charge  map[string]any `json:"charge"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// The amount survives the whole chain as int64 cents, and the charge comes
	// back with its server-recomputed remaining balance.
	if out.Payment["amountCents"] != float64(50000) || out.Payment["paidAt"] != "2026-09-01T10:00:00Z" {
		t.Errorf("payment = %v", out.Payment)
	}
	if out.Charge["remainingCents"] != float64(100000) {
		t.Errorf("charge = %v", out.Charge)
	}
}

func TestFinancePaymentReadOnlyPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_finance": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/finance/payments", "finance-reader", validPaymentBody)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestFinanceAccountDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_finance OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/finance/accounts/stu-1", "finance-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestFinanceAccountNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_finance": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/finance/accounts/stu-1", "", "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestFinanceAccountWithReadPermission200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_finance": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/finance/accounts/stu-1", "finance-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out struct {
		Charges           []map[string]any `json:"charges"`
		TotalBalanceCents int64            `json:"totalBalanceCents"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// The studentId comes from the path param, so the stub echoes it back.
	if len(out.Charges) != 1 || out.Charges[0]["studentId"] != "stu-1" || out.TotalBalanceCents != 150000 {
		t.Errorf("body = %+v", out)
	}
}

func TestFinanceOverdueWithReadPermission200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_finance": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/finance/overdue?cycleId=cyc-1", "finance-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
}

func TestFinanceOverdueDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_finance OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/finance/overdue", "finance-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestFinanceChargeListWithReadPermission200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_finance": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/finance/charges?studentId=stu-1", "finance-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
}

// --- attendance routes (Phase 8): ModuleGuard(attendance) → RequireAuth →
// RequirePermission. Saving a group·day is the write (attendance:write); the
// roster and the per-enrollment history are reads (attendance:read). Every
// route sits behind guard.Protect("attendance", …) so module_attendance OFF ⇒
// 404, before auth runs.

const validSaveDayBody = `{"groupId":"grp-1","date":"2026-09-01","marks":[` +
	`{"enrollmentId":"enr-1","status":"absent","note":"cita médica"},` +
	`{"enrollmentId":"enr-2","status":"late","note":""}]}`

func TestAttendanceSaveDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_attendance OFF
	// Even a caller with attendance:write must not learn the module exists.
	res := doGateway(t, gw, http.MethodPost, "/api/v1/attendance", "att-writer", validSaveDayBody)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestAttendanceSaveNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_attendance": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/attendance", "", validSaveDayBody)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

// Saving a day reconciles it — it can erase marks — so attendance:read is
// deliberately insufficient.
func TestAttendanceSaveReadOnlyPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_attendance": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/attendance", "att-reader", validSaveDayBody)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestAttendanceSaveWithWritePermission200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_attendance": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/attendance", "att-writer", validSaveDayBody)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Two exceptions crossed the whole edge; only one of them is an absence.
	if out["absentCount"] != float64(1) || out["markedCount"] != float64(2) {
		t.Errorf("body = %v", out)
	}
}

// /attendance/roster is a more specific pattern than /attendance: ServeMux must
// route it to the roster handler, under the read guard.
func TestAttendanceRosterWithReadPermission200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_attendance": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/attendance/roster?groupId=grp-1&date=2026-09-01", "att-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out struct {
		Roster []map[string]any `json:"roster"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Default-present survives the whole chain: an unmarked row reads "present".
	if len(out.Roster) != 2 || out.Roster[0]["status"] != "present" || out.Roster[1]["status"] != "absent" {
		t.Errorf("body = %v", out.Roster)
	}
}

func TestAttendanceRosterDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_attendance OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/attendance/roster?groupId=grp-1&date=2026-09-01", "att-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestAttendanceRosterNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_attendance": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/attendance/roster?groupId=grp-1&date=2026-09-01", "", "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestAttendanceListWithReadPermission200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_attendance": true})
	res := doGateway(t, gw, http.MethodGet,
		"/api/v1/attendance?enrollmentId=enr-1&from=2026-09-01&to=2026-09-30", "att-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out struct {
		Records []map[string]any `json:"records"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// The enrollmentId comes from the query, so the stub echoes it back.
	if len(out.Records) != 1 || out.Records[0]["enrollmentId"] != "enr-1" ||
		out.Records[0]["status"] != "absent" {
		t.Errorf("body = %v", out.Records)
	}
}

func TestAttendanceListDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_attendance OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/attendance?enrollmentId=enr-1", "att-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

// A write token also carries attendance:read, so the read routes stay reachable
// for the teacher who is capturing the day.
func TestAttendanceListWithWritePermission200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_attendance": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/attendance?enrollmentId=enr-1", "att-writer", "")
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
}
