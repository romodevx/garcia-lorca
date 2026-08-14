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
	commsv1 "github.com/garcia-lorca/gen/go/garcialorca/comms/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/comms/v1/commsv1connect"
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
		"teaching-writer":  {"teaching:write", "teaching:read"},
		"teaching-reader":  {"teaching:read"},
		"finance-writer":   {"finance:write", "finance:read"},
		"finance-reader":   {"finance:read"},
		"att-writer":       {"attendance:write", "attendance:read"},
		"att-reader":       {"attendance:read"},
		"comms-writer":     {"comms:write", "comms:read"},
		"comms-reader":     {"comms:read"},
		// users-writer / roles-writer also hold the permissions they hand out in
		// the happy-path bodies below: the caller-subset guard only lets an admin
		// grant or assign permissions it holds itself. A real school admin (the
		// owner) holds the whole catalog; these fixtures mirror the subset needed.
		"users-writer":    {"users:write", "users:read", "grades:read"},
		"users-reader":    {"users:read"},
		"roles-writer":    {"roles:write", "roles:read", "students:read", "grades:read", "grades:write"},
		"roles-reader":    {"roles:read"},
		"guardian-writer": {"guardians:write", "guardians:read"},
		"guardian-reader": {"guardians:read"},
		"family-reader":   {"family:read"},
	}
	granted, ok := perms[req.Msg.GetAccessToken()]
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errStr("invalid token"))
	}
	return connect.NewResponse(&iamv1.ValidateTokenResponse{Claims: &iamv1.Claims{
		Subject: "u1", TenantId: "t1", Email: "ana@garcialorca.mx", Permissions: granted,
	}}), nil
}

// Login on the stub accepts one credential pair, so the mobile-login route can
// be exercised end to end (tokens must come back in the BODY).
func (stubIAM) Login(_ context.Context, req *connect.Request[iamv1.LoginRequest]) (*connect.Response[iamv1.LoginResponse], error) {
	if req.Msg.GetEmail() != "mama@example.com" || req.Msg.GetPassword() != "secret" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errStr("bad credentials"))
	}
	return connect.NewResponse(&iamv1.LoginResponse{
		AccessToken: "acc-tok", RefreshToken: "ref-tok", ExpiresIn: 900,
	}), nil
}

// RefreshToken rotates the canned pair for the one known refresh token, so the
// mobile refresh route can be exercised end to end.
func (stubIAM) RefreshToken(_ context.Context, req *connect.Request[iamv1.RefreshTokenRequest]) (*connect.Response[iamv1.RefreshTokenResponse], error) {
	if req.Msg.GetRefreshToken() != "ref-tok" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errStr("stale refresh token"))
	}
	return connect.NewResponse(&iamv1.RefreshTokenResponse{
		AccessToken: "acc-tok-2", RefreshToken: "ref-tok-2", ExpiresIn: 900,
	}), nil
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

// Guardian-link RPCs on the stub. ListStudentsForGuardian answers with TWO
// children so the family aggregation is observable end to end; the guardian id
// is echoed back, proving the gateway sent the principal's own id.
func (stubStudents) LinkGuardian(_ context.Context, req *connect.Request[studentsv1.LinkGuardianRequest]) (*connect.Response[studentsv1.LinkGuardianResponse], error) {
	return connect.NewResponse(&studentsv1.LinkGuardianResponse{Link: &studentsv1.GuardianLink{
		Id: "gl-1", TenantId: "t1", GuardianUserId: req.Msg.GetGuardianUserId(),
		StudentId: req.Msg.GetStudentId(), Relationship: req.Msg.GetRelationship(),
	}}), nil
}

func (stubStudents) UnlinkGuardian(_ context.Context, _ *connect.Request[studentsv1.UnlinkGuardianRequest]) (*connect.Response[studentsv1.UnlinkGuardianResponse], error) {
	return connect.NewResponse(&studentsv1.UnlinkGuardianResponse{}), nil
}

func (stubStudents) ListGuardiansForStudent(_ context.Context, req *connect.Request[studentsv1.ListGuardiansForStudentRequest]) (*connect.Response[studentsv1.ListGuardiansForStudentResponse], error) {
	return connect.NewResponse(&studentsv1.ListGuardiansForStudentResponse{Links: []*studentsv1.GuardianLink{
		{Id: "gl-1", TenantId: "t1", GuardianUserId: "usr-9", StudentId: req.Msg.GetStudentId(), Relationship: "mother"},
	}}), nil
}

func (stubStudents) ListStudentsForGuardian(_ context.Context, req *connect.Request[studentsv1.ListStudentsForGuardianRequest]) (*connect.Response[studentsv1.ListStudentsForGuardianResponse], error) {
	guardian := req.Msg.GetGuardianUserId()
	return connect.NewResponse(&studentsv1.ListStudentsForGuardianResponse{Links: []*studentsv1.GuardianLink{
		{Id: "gl-1", TenantId: "t1", GuardianUserId: guardian, StudentId: "stu-1", Relationship: "mother"},
		{Id: "gl-2", TenantId: "t1", GuardianUserId: guardian, StudentId: "stu-2", Relationship: "mother"},
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

// ListTeachers backs the teaching surface's caller→teacher resolution: it returns
// one teacher whose user_id is "u1", the Subject the stub iam mints for every
// token, so the authenticated principal resolves to teacher tch-1.
func (stubConfig) ListTeachers(_ context.Context, _ *connect.Request[configv1.ListTeachersRequest]) (*connect.Response[configv1.ListTeachersResponse], error) {
	return connect.NewResponse(&configv1.ListTeachersResponse{Teachers: []*configv1.Teacher{
		{Id: "tch-1", FirstName: "Juan", LastName: "Pérez", Email: "juan@garcialorca.mx", UserId: "u1"},
		{Id: "tch-2", FirstName: "Otra", LastName: "Maestra", Email: "otra@garcialorca.mx", UserId: "u2"},
	}}), nil
}

// ListTeachingAssignments echoes one assignment (asg-1) for teacher tch-1, so the
// teacher self-scoped surface has exactly one owned assignment; any other id is
// unowned and must answer 404.
func (stubConfig) ListTeachingAssignments(_ context.Context, req *connect.Request[configv1.ListTeachingAssignmentsRequest]) (*connect.Response[configv1.ListTeachingAssignmentsResponse], error) {
	if req.Msg.GetTeacherId() != "tch-1" {
		return connect.NewResponse(&configv1.ListTeachingAssignmentsResponse{}), nil
	}
	return connect.NewResponse(&configv1.ListTeachingAssignmentsResponse{TeachingAssignments: []*configv1.TeachingAssignment{
		{Id: "asg-1", CycleId: "cyc-1", GroupId: "grp-1", SubjectId: "sub-1", TeacherId: "tch-1"},
	}}), nil
}

func (stubConfig) ListGroups(_ context.Context, _ *connect.Request[configv1.ListGroupsRequest]) (*connect.Response[configv1.ListGroupsResponse], error) {
	return connect.NewResponse(&configv1.ListGroupsResponse{Groups: []*configv1.Group{
		{Id: "grp-1", CycleId: "cyc-1", Name: "3° A"},
	}}), nil
}

func (stubConfig) ListSubjects(_ context.Context, _ *connect.Request[configv1.ListSubjectsRequest]) (*connect.Response[configv1.ListSubjectsResponse], error) {
	return connect.NewResponse(&configv1.ListSubjectsResponse{Subjects: []*configv1.Subject{
		{Id: "sub-1", Name: "Matemáticas"},
	}}), nil
}

// ListTerms echoes one term for cyc-1, the cycle stubConfig's one teaching
// assignment lives in, so the teacher self-scoped terms surface has exactly one
// term to return.
func (stubConfig) ListTerms(_ context.Context, req *connect.Request[configv1.ListTermsRequest]) (*connect.Response[configv1.ListTermsResponse], error) {
	if req.Msg.GetCycleId() != "cyc-1" {
		return connect.NewResponse(&configv1.ListTermsResponse{}), nil
	}
	return connect.NewResponse(&configv1.ListTermsResponse{Terms: []*configv1.Term{
		{Id: "trm-1", CycleId: "cyc-1", Name: "Bloque 1"},
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

// stubComms echoes a canned device / notification list / marked notification,
// enough to exercise the comms route security chain end to end. The unread state
// shows up as an unset read_at, which the edge must render as an empty string.
type stubComms struct {
	commsv1connect.UnimplementedCommsServiceHandler
}

func (stubComms) RegisterDevice(_ context.Context, req *connect.Request[commsv1.RegisterDeviceRequest]) (*connect.Response[commsv1.RegisterDeviceResponse], error) {
	return connect.NewResponse(&commsv1.RegisterDeviceResponse{Device: &commsv1.Device{
		Id: "dev-1", UserId: req.Msg.GetUserId(), FcmToken: req.Msg.GetFcmToken(), Platform: req.Msg.GetPlatform(),
	}}), nil
}

func (stubComms) ListNotifications(_ context.Context, req *connect.Request[commsv1.ListNotificationsRequest]) (*connect.Response[commsv1.ListNotificationsResponse], error) {
	return connect.NewResponse(&commsv1.ListNotificationsResponse{Notifications: []*commsv1.Notification{
		{
			Id: "ntf-1", StudentId: req.Msg.GetStudentId(), Type: "absence",
			Title: "Ausencia", Body: "Ana faltó hoy",
			CreatedAt: timestamppb.New(time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)),
		},
	}}), nil
}

func (stubComms) MarkNotificationRead(_ context.Context, req *connect.Request[commsv1.MarkNotificationReadRequest]) (*connect.Response[commsv1.MarkNotificationReadResponse], error) {
	return connect.NewResponse(&commsv1.MarkNotificationReadResponse{Notification: &commsv1.Notification{
		Id: req.Msg.GetId(), StudentId: "stu-1", Type: "absence", Title: "Ausencia",
		CreatedAt: timestamppb.New(time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)),
		ReadAt:    timestamppb.New(time.Date(2026, 9, 1, 18, 0, 0, 0, time.UTC)),
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
	comPath, comHandler := commsv1connect.NewCommsServiceHandler(stubComms{})
	comSrv := connectServer(t, comPath, comHandler)

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
		client.NewCommsClient(comSrv.URL, comSrv.Client()),
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

// End-to-end caller-subset guard: a roles:write holder cannot grant a permission
// it does not itself hold (here finance:read), even though it may create roles.
// This is the escalation the guard closes — verified through the full stack.
func TestRolesWriteCannotGrantPermissionCallerLacks(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_iam": true})
	body := `{"name":"escalado","permissions":["students:read","finance:read"]}`
	res := doGateway(t, gw, http.MethodPost, "/api/v1/roles", "roles-writer", body)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (cannot grant finance:read it does not hold)", res.StatusCode)
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

// --- comms routes (Phase 9a): ModuleGuard(comms) → RequireAuth →
// RequirePermission. Registering a device and marking a notification read are
// writes (comms:write); the notification list is a read (comms:read). Every
// route sits behind guard.Protect("comms", …) so module_comms OFF ⇒ 404, before
// auth runs.

// validDeviceBody deliberately carries a userId: the edge must IGNORE it and
// register the device for the authenticated caller (the stub's subject "u1").
const validDeviceBody = `{"userId":"usr-1","fcmToken":"tok-abc","platform":"android"}`

func TestCommsRegisterDeviceDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_comms OFF
	// Even a caller with comms:write must not learn the module exists.
	res := doGateway(t, gw, http.MethodPost, "/api/v1/comms/devices", "comms-writer", validDeviceBody)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestCommsRegisterDeviceNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_comms": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/comms/devices", "", validDeviceBody)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

// Registering a device changes who gets pushed to, so comms:read is deliberately
// insufficient.
func TestCommsRegisterDeviceReadOnlyPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_comms": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/comms/devices", "comms-reader", validDeviceBody)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestCommsRegisterDeviceWithWritePermission200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_comms": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/comms/devices", "comms-writer", validDeviceBody)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["id"] != "dev-1" || out["fcmToken"] != "tok-abc" || out["platform"] != "android" {
		t.Errorf("body = %v", out)
	}
	// The stub comms echoes the user_id it received, so this pins the owner to the
	// token's subject rather than the "usr-1" the body asked for.
	if out["userId"] != "u1" {
		t.Errorf("device owner = %v, want the caller's u1 (the body must not decide it)", out["userId"])
	}
}

func TestCommsListNotificationsWithReadPermission200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_comms": true})
	res := doGateway(t, gw, http.MethodGet,
		"/api/v1/comms/notifications?studentId=stu-1&unreadOnly=true", "comms-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out struct {
		Notifications []map[string]any `json:"notifications"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// The studentId comes from the query, so the stub echoes it back. An unset
	// read_at survives the whole edge as an empty string, never a zero instant.
	if len(out.Notifications) != 1 || out.Notifications[0]["studentId"] != "stu-1" ||
		out.Notifications[0]["createdAt"] != "2026-09-01T10:00:00Z" || out.Notifications[0]["readAt"] != "" {
		t.Errorf("body = %v", out.Notifications)
	}
}

func TestCommsListNotificationsDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_comms OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/comms/notifications?studentId=stu-1", "comms-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestCommsListNotificationsNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_comms": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/comms/notifications?studentId=stu-1", "", "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

// The mark-read route carries the id in the path: ServeMux must bind it and the
// handler must forward it, which the stub proves by echoing it back.
func TestCommsMarkReadWithWritePermission200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_comms": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/comms/notifications/ntf-7/read", "comms-writer", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["id"] != "ntf-7" || out["readAt"] != "2026-09-01T18:00:00Z" {
		t.Errorf("body = %v", out)
	}
}

// Acknowledging a notification mutates it, so a read-only caller is refused.
func TestCommsMarkReadReadOnlyPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_comms": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/comms/notifications/ntf-7/read", "comms-reader", "")
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestCommsMarkReadDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_comms OFF
	res := doGateway(t, gw, http.MethodPost, "/api/v1/comms/notifications/ntf-7/read", "comms-writer", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

// A write token also carries comms:read, so the inbox stays reachable for the
// client that just registered its device.
func TestCommsListNotificationsWithWritePermission200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_comms": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/comms/notifications?studentId=stu-1", "comms-writer", "")
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
}

// --- Phase 9b guardian links: ModuleGuard(students) → RequireAuth → rotation →
// RequirePermission(guardians:write on link/unlink, guardians:read on the list).
// Managing who may see a child is an admin action, hence its own permission pair
// rather than students:*.

const validGuardianBody = `{"guardianUserId":"usr-9","studentId":"stu-1","relationship":"mother"}`

func TestGuardianLinkDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_students OFF
	// Even a caller with guardians:write must not learn the module exists.
	res := doGateway(t, gw, http.MethodPost, "/api/v1/guardians", "guardian-writer", validGuardianBody)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestGuardianLinkNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/guardians", "", validGuardianBody)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestGuardianLinkReadOnlyPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/guardians", "guardian-reader", validGuardianBody)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

// students:* is deliberately insufficient: a student clerk must not be able to
// grant somebody visibility of a child.
func TestGuardianLinkStudentsPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/guardians", "student-enroller", validGuardianBody)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestGuardianLinkWithWritePermissionCreates201(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/guardians", "guardian-writer", validGuardianBody)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", res.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["id"] != "gl-1" || out["guardianUserId"] != "usr-9" || out["studentId"] != "stu-1" || out["relationship"] != "mother" {
		t.Errorf("body = %v", out)
	}
}

func TestGuardianUnlinkReadOnlyPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/guardians/unlink", "guardian-reader",
		`{"guardianUserId":"usr-9","studentId":"stu-1"}`)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestGuardianUnlinkWithWritePermissionIs204(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/guardians/unlink", "guardian-writer",
		`{"guardianUserId":"usr-9","studentId":"stu-1"}`)
	if res.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want 204", res.StatusCode)
	}
}

func TestListGuardiansForStudentDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_students OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/students/stu-1/guardians", "guardian-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestListGuardiansForStudentWithReadPermission200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/students/stu-1/guardians", "guardian-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out struct {
		Guardians []map[string]any `json:"guardians"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// The path value must reach the students service — a dropped {studentId}
	// would silently list the wrong child's guardians.
	if len(out.Guardians) != 1 || out.Guardians[0]["studentId"] != "stu-1" {
		t.Errorf("guardians = %v", out.Guardians)
	}
}

// --- Phase 9b family surface: ModuleGuard(comms) → ModuleGuard(students) →
// RequireAuth → rotation → RequirePermission(family:read). The child set comes
// from the principal, so a guardian can only ever read their own children's
// notifications.
//
// The route reads from BOTH modules — students for the guardian links, comms for
// the inbox — so it is guarded on both: either one disabled ⇒ 404. A disabled
// module must be invisible and removable without breaking others, and a
// half-guarded composite route would answer 500 (or leak the links) instead.

// familyModulesOn is the flag set the family route needs: both of the modules it
// composes.
var familyModulesOn = map[string]bool{"module_comms": true, "module_students": true}

func TestFamilyNotificationsDisabledCommsModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true}) // module_comms OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/family/notifications", "family-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestFamilyNotificationsDisabledStudentsModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_comms": true}) // module_students OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/family/notifications", "family-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestFamilyNotificationsNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, familyModulesOn)
	res := doGateway(t, gw, http.MethodGet, "/api/v1/family/notifications", "", "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

// comms:read reads ONE student by id; family:read is the guardian-scoped
// permission, so a comms reader is refused here.
func TestFamilyNotificationsWithoutFamilyReadIs403(t *testing.T) {
	gw := newTestGateway(t, familyModulesOn)
	res := doGateway(t, gw, http.MethodGet, "/api/v1/family/notifications", "comms-reader", "")
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestFamilyNotificationsWithFamilyReadAggregatesChildren200(t *testing.T) {
	gw := newTestGateway(t, familyModulesOn)
	res := doGateway(t, gw, http.MethodGet, "/api/v1/family/notifications", "family-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out struct {
		Notifications []map[string]any `json:"notifications"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// The stub guardian has two children and the stub inbox yields one
	// notification each, tagged with the child it belongs to.
	if len(out.Notifications) != 2 {
		t.Fatalf("notifications = %d, want 2 (one per linked child)", len(out.Notifications))
	}
	seen := map[string]bool{}
	for _, n := range out.Notifications {
		seen[n["studentId"].(string)] = true
	}
	if !seen["stu-1"] || !seen["stu-2"] {
		t.Errorf("notifications tagged %v, want both stu-1 and stu-2", seen)
	}
}

// --- Phase 9b B6.x self-scoped by-child family reads. Each composite route is
// guarded on module_students (the ownership check reads the guardian links) plus
// the module that owns the data, needs family:read, and answers 404 for a
// {studentId} outside the caller's own children. The stub guardian is linked to
// stu-1 and stu-2, so stu-1 is owned and stu-999 is not.

// The students list is scoped by the principal alone and needs only
// module_students; the stub guardian is linked to stu-1 and stu-2.
func TestFamilyListStudentsOwnedReturnsChildren200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/family/students", "family-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out struct {
		Students []map[string]any `json:"students"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Students) != 2 {
		t.Fatalf("students = %d, want 2 (the guardian's linked children)", len(out.Students))
	}
	seen := map[string]bool{}
	for _, s := range out.Students {
		seen[s["id"].(string)] = true
	}
	if !seen["stu-1"] || !seen["stu-2"] {
		t.Errorf("students = %v, want both stu-1 and stu-2", out.Students)
	}
}

func TestFamilyListStudentsDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_students OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/family/students", "family-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestFamilyListStudentsNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/family/students", "", "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

// students:read is not family:read: an admin student reader is refused the
// guardian-scoped list.
func TestFamilyListStudentsWithoutFamilyReadIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/family/students", "student-reader", "")
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

// familyAccountModulesOn is the flag set the account route needs.
var familyAccountModulesOn = map[string]bool{"module_finance": true, "module_students": true}

func TestFamilyAccountDisabledFinanceModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true}) // module_finance OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/family/students/stu-1/account", "family-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestFamilyAccountDisabledStudentsModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_finance": true}) // module_students OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/family/students/stu-1/account", "family-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestFamilyAccountNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, familyAccountModulesOn)
	res := doGateway(t, gw, http.MethodGet, "/api/v1/family/students/stu-1/account", "", "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestFamilyAccountWithoutFamilyReadIs403(t *testing.T) {
	gw := newTestGateway(t, familyAccountModulesOn)
	res := doGateway(t, gw, http.MethodGet, "/api/v1/family/students/stu-1/account", "finance-reader", "")
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestFamilyAccountOwnedChildReturnsStatement200(t *testing.T) {
	gw := newTestGateway(t, familyAccountModulesOn)
	res := doGateway(t, gw, http.MethodGet, "/api/v1/family/students/stu-1/account", "family-reader", "")
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
	if len(out.Charges) != 1 || out.Charges[0]["studentId"] != "stu-1" {
		t.Errorf("charges = %v, want the owned child's statement", out.Charges)
	}
}

// The core security property: a guardian guessing another family's child id gets
// 404, not the statement — and not 403, which would confirm the id exists.
func TestFamilyAccountUnlinkedChildIs404(t *testing.T) {
	gw := newTestGateway(t, familyAccountModulesOn)
	res := doGateway(t, gw, http.MethodGet, "/api/v1/family/students/stu-999/account", "family-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (no cross-family read, no existence leak)", res.StatusCode)
	}
}

func TestFamilyProfileOwnedChildReturns200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/family/students/stu-1/profile", "family-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ID != "stu-1" {
		t.Errorf("profile id = %q, want stu-1", out.ID)
	}
}

func TestFamilyProfileUnlinkedChildIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/family/students/stu-999/profile", "family-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

var familyGradesModulesOn = map[string]bool{"module_grades": true, "module_students": true}

func TestFamilyGradesOwnedChildReturns200(t *testing.T) {
	gw := newTestGateway(t, familyGradesModulesOn)
	res := doGateway(t, gw, http.MethodGet, "/api/v1/family/students/stu-1/grades?termId=trm-1", "family-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out struct {
		GradeEntries []map[string]any `json:"gradeEntries"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// The stub resolves stu-1 to enr-1 and returns one entry for it, proving the
	// student id was translated to the enrollment id the grades service keys on.
	if len(out.GradeEntries) != 1 || out.GradeEntries[0]["enrollmentId"] != "enr-1" {
		t.Errorf("gradeEntries = %v, want one keyed on enr-1", out.GradeEntries)
	}
}

func TestFamilyGradesDisabledGradesModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true}) // module_grades OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/family/students/stu-1/grades", "family-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestFamilyGradesUnlinkedChildIs404(t *testing.T) {
	gw := newTestGateway(t, familyGradesModulesOn)
	res := doGateway(t, gw, http.MethodGet, "/api/v1/family/students/stu-999/grades", "family-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

var familyAttendanceModulesOn = map[string]bool{"module_attendance": true, "module_students": true}

func TestFamilyAttendanceOwnedChildReturns200(t *testing.T) {
	gw := newTestGateway(t, familyAttendanceModulesOn)
	res := doGateway(t, gw, http.MethodGet,
		"/api/v1/family/students/stu-1/attendance?from=2026-09-01&to=2026-09-30", "family-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out struct {
		Records []map[string]any `json:"records"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Records) != 1 || out.Records[0]["enrollmentId"] != "enr-1" {
		t.Errorf("records = %v, want one keyed on enr-1", out.Records)
	}
}

func TestFamilyAttendanceUnlinkedChildIs404(t *testing.T) {
	gw := newTestGateway(t, familyAttendanceModulesOn)
	res := doGateway(t, gw, http.MethodGet, "/api/v1/family/students/stu-999/attendance", "family-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

// B6.5 guardian mark-read: family:read (NOT comms:write) marks one of the
// caller's OWN children's notifications. The stub inbox yields ntf-1 for each
// linked child, so ntf-1 is owned and a guessed id is not.
func TestFamilyMarkReadOwnedNotification200(t *testing.T) {
	gw := newTestGateway(t, familyModulesOn)
	res := doGateway(t, gw, http.MethodPost, "/api/v1/family/notifications/ntf-1/read", "family-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out struct {
		ID     string `json:"id"`
		ReadAt string `json:"readAt"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ID != "ntf-1" || out.ReadAt == "" {
		t.Errorf("marked = %+v, want ntf-1 with a readAt stamp", out)
	}
}

func TestFamilyMarkReadOtherFamilysNotificationIs404(t *testing.T) {
	gw := newTestGateway(t, familyModulesOn)
	res := doGateway(t, gw, http.MethodPost, "/api/v1/family/notifications/ntf-x/read", "family-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (a guardian cannot mark another family's notification)", res.StatusCode)
	}
}

func TestFamilyMarkReadWithoutFamilyReadIs403(t *testing.T) {
	gw := newTestGateway(t, familyModulesOn)
	res := doGateway(t, gw, http.MethodPost, "/api/v1/family/notifications/ntf-1/read", "comms-reader", "")
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (comms:read is not family:read)", res.StatusCode)
	}
}

// --- M07 teaching: the teacher self-scoped grade surface. Assignments composes
// module_grades + module_config; roster and capture need only module_grades. The
// stub iam mints Subject "u1" for every token, and stubConfig links user "u1" to
// teacher tch-1, who teaches exactly asg-1 — so asg-1 is owned and any other id
// is not.

var teachingAssignmentsModulesOn = map[string]bool{"module_grades": true, "module_config": true}

func TestTeachingAssignmentsDisabledGradesModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true}) // module_grades OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/teaching/assignments", "teaching-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestTeachingAssignmentsDisabledConfigModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_grades": true}) // module_config OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/teaching/assignments", "teaching-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestTeachingAssignmentsNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, teachingAssignmentsModulesOn)
	res := doGateway(t, gw, http.MethodGet, "/api/v1/teaching/assignments", "", "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

// grades:read is NOT teaching:read: the broad staff permission cannot reach the
// teacher self-scoped surface.
func TestTeachingAssignmentsWithoutTeachingReadIs403(t *testing.T) {
	gw := newTestGateway(t, teachingAssignmentsModulesOn)
	res := doGateway(t, gw, http.MethodGet, "/api/v1/teaching/assignments", "grades-reader", "")
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (grades:read is not teaching:read)", res.StatusCode)
	}
}

func TestTeachingAssignmentsWithTeachingReadReturns200(t *testing.T) {
	gw := newTestGateway(t, teachingAssignmentsModulesOn)
	res := doGateway(t, gw, http.MethodGet, "/api/v1/teaching/assignments", "teaching-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out struct {
		Assignments []map[string]any `json:"assignments"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// The stub resolves the principal (Subject "u1") to teacher tch-1, whose one
	// assignment asg-1 comes back enriched with its group and subject names.
	if len(out.Assignments) != 1 || out.Assignments[0]["id"] != "asg-1" ||
		out.Assignments[0]["groupName"] != "3° A" || out.Assignments[0]["subjectName"] != "Matemáticas" {
		t.Errorf("assignments = %v, want the caller's own asg-1 enriched", out.Assignments)
	}
}

// /teaching/terms composes module_grades + module_config (it reads the config
// term catalog), teaching:read, and serves only the terms of cycles the caller
// teaches in.

func TestTeachingTermsDisabledGradesModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true}) // module_grades OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/teaching/terms", "teaching-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestTeachingTermsDisabledConfigModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_grades": true}) // module_config OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/teaching/terms", "teaching-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestTeachingTermsNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, teachingAssignmentsModulesOn)
	res := doGateway(t, gw, http.MethodGet, "/api/v1/teaching/terms", "", "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestTeachingTermsWithoutTeachingReadIs403(t *testing.T) {
	gw := newTestGateway(t, teachingAssignmentsModulesOn)
	res := doGateway(t, gw, http.MethodGet, "/api/v1/teaching/terms", "grades-reader", "")
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (grades:read is not teaching:read)", res.StatusCode)
	}
}

func TestTeachingTermsWithTeachingReadReturns200(t *testing.T) {
	gw := newTestGateway(t, teachingAssignmentsModulesOn)
	res := doGateway(t, gw, http.MethodGet, "/api/v1/teaching/terms", "teaching-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out struct {
		Terms []map[string]any `json:"terms"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// The stub resolves the principal to teacher tch-1, whose one assignment lives
	// in cyc-1, whose one term trm-1 comes back.
	if len(out.Terms) != 1 || out.Terms[0]["id"] != "trm-1" ||
		out.Terms[0]["name"] != "Bloque 1" || out.Terms[0]["cycleId"] != "cyc-1" {
		t.Errorf("terms = %v, want the caller's own cycle's trm-1", out.Terms)
	}
}

func TestTeachingRosterDisabledGradesModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true}) // module_grades OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/teaching/roster?assignmentId=asg-1&termId=trm-1", "teaching-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestTeachingRosterNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_grades": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/teaching/roster?assignmentId=asg-1", "", "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestTeachingRosterWithoutTeachingReadIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_grades": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/teaching/roster?assignmentId=asg-1", "grades-reader", "")
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestTeachingRosterOwnedAssignmentReturns200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_grades": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/teaching/roster?assignmentId=asg-1&termId=trm-1", "teaching-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out struct {
		Roster []map[string]any `json:"roster"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// The gateway resolves the student name for the Maestro (who holds no
	// students:read): the stub roster's stu-1 comes back named "Ana".
	if len(out.Roster) != 1 || out.Roster[0]["enrollmentId"] != "enr-1" ||
		out.Roster[0]["studentId"] != "stu-1" || out.Roster[0]["studentName"] != "Ana" {
		t.Errorf("roster = %v, want one row keyed on enr-1 with studentName Ana", out.Roster)
	}
}

// An assignment the caller does not teach answers 404 — the ownership guard runs
// before the grades service is ever consulted.
func TestTeachingRosterUnownedAssignmentIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_grades": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/teaching/roster?assignmentId=asg-999&termId=trm-1", "teaching-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (not the caller's assignment)", res.StatusCode)
	}
}

func TestTeachingCaptureDisabledGradesModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true}) // module_grades OFF
	res := doGateway(t, gw, http.MethodPost, "/api/v1/teaching/grades", "teaching-writer",
		`{"assignmentId":"asg-1","enrollmentId":"enr-1","termId":"trm-1","value":9}`)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestTeachingCaptureNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_grades": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/teaching/grades", "",
		`{"assignmentId":"asg-1","enrollmentId":"enr-1","termId":"trm-1","value":9}`)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

// teaching:read is not teaching:write: a reader cannot capture.
func TestTeachingCaptureWithoutTeachingWriteIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_grades": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/teaching/grades", "teaching-reader",
		`{"assignmentId":"asg-1","enrollmentId":"enr-1","termId":"trm-1","value":9}`)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (teaching:read is not teaching:write)", res.StatusCode)
	}
}

func TestTeachingCaptureOwnedAssignmentReturns200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_grades": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/teaching/grades", "teaching-writer",
		`{"assignmentId":"asg-1","enrollmentId":"enr-1","termId":"trm-1","value":9}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["id"] != "grd-1" || out["enrollmentId"] != "enr-1" {
		t.Errorf("captured = %v, want grd-1 keyed on enr-1", out)
	}
}

// Capturing against an assignment the caller does not teach answers 404 — the
// ownership guard runs before the grades service is ever consulted.
func TestTeachingCaptureUnownedAssignmentIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_grades": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/teaching/grades", "teaching-writer",
		`{"assignmentId":"asg-999","enrollmentId":"enr-1","termId":"trm-1","value":9}`)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (not the caller's assignment)", res.StatusCode)
	}
}

// --- Phase 9b mobile auth: unauthenticated like the web login, tokens in the
// BODY (no cookie jar in React Native), and MobileLogin behind the SAME per-IP
// login limiter as the web login.

func TestMobileLoginRouteReturnsBodyTokensWithoutCookies(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/auth/mobile/login", "",
		`{"email":"mama@example.com","password":"secret"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if cs := res.Cookies(); len(cs) != 0 {
		t.Errorf("mobile login set cookies %v, want none", cs)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["accessToken"] != "acc-tok" || out["refreshToken"] != "ref-tok" {
		t.Errorf("body = %v, want the token pair", out)
	}
}

func TestMobileLoginRouteBadCredentialsIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/auth/mobile/login", "",
		`{"email":"mama@example.com","password":"wrong"}`)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

// The web login keeps its cookie-only posture: tokens must never appear in its
// body, even though the mobile sibling now returns them.
func TestWebLoginRouteKeepsCookieOnlyPosture(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/auth/login", "",
		`{"email":"mama@example.com","password":"secret"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := out["accessToken"]; ok {
		t.Errorf("web login body leaked accessToken: %v", out)
	}
	if _, ok := out["refreshToken"]; ok {
		t.Errorf("web login body leaked refreshToken: %v", out)
	}
	names := map[string]bool{}
	for _, c := range res.Cookies() {
		names[c.Name] = true
	}
	if !names["gl_access"] || !names["gl_refresh"] {
		t.Errorf("web login cookies = %v, want gl_access + gl_refresh", names)
	}
}

// The mobile refresh is authorized by the refresh token in the BODY, never by a
// cookie, and rotates without emitting Set-Cookie.
func TestMobileRefreshRouteRotatesFromBody(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/auth/mobile/refresh", "",
		`{"refreshToken":"ref-tok"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if cs := res.Cookies(); len(cs) != 0 {
		t.Errorf("mobile refresh set cookies %v, want none", cs)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["accessToken"] != "acc-tok-2" || out["refreshToken"] != "ref-tok-2" {
		t.Errorf("body = %v, want the rotated pair", out)
	}
}

// A cookie must not be able to authorize a mobile refresh: the body is the only
// credential source, so an empty body is 401 even with a valid cookie present.
func TestMobileRefreshRouteEmptyBodyIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/auth/mobile/refresh", "", `{}`)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}
