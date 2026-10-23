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
	uniformsv1 "github.com/garcia-lorca/gen/go/garcialorca/uniforms/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/uniforms/v1/uniformsv1connect"
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
		"with-perm":     {"students:create"},
		"no-perm":       {"students:read"},
		"config-writer": {"config:write", "config:read"},
		"config-reader": {"config:read"},
		// ADR 0021: deleting a level is the owner's alone (iam 0024).
		"level-deleter": {"levels:delete"},
		// ADR 0033: the legacy import is the owner's alone (iam 0031).
		"importer": {"students:import"},
		// T2: deleting a user is the owner's alone (iam 0025).
		"user-deleter": {"users:delete", "users:read"},
		// M13: the staff roster left Configuración, so it answers to its own
		// permission. A config holder must NOT reach it any more — that split is
		// the whole point of the move.
		"teachers-writer":            {"teachers:write", "teachers:read"},
		"teachers-reader":            {"teachers:read"},
		"student-enroller":           {"students:enroll"},
		"student-collections-reader": {"students:enroll", "collections:read"},
		"student-reader":             {"students:read"},
		// PLAN-import-avisos: «Está bien así» needs students:update.
		"student-updater": {"students:update", "students:read"},
		"grades-writer":   {"grades:write", "grades:read"},
		"grades-reader":   {"grades:read"},
		// T7: closing a period is the owner.s alone (iam 0023); grades:write is not enough.
		"grades-closer":   {"grades:close", "grades:read"},
		"teaching-writer": {"teaching:write", "teaching:read"},
		"teaching-reader": {"teaching:read"},
		"finance-writer":  {"finance:write", "finance:read"},
		"finance-reader":  {"finance:read"},
		"att-writer":      {"attendance:write", "attendance:read"},
		"att-reader":      {"attendance:read"},
		"att-own-writer":  {"attendance_own:write", "attendance_own:read"},
		"att-own-reader":  {"attendance_own:read"},
		"comms-writer":    {"comms:write", "comms:read"},
		"comms-reader":    {"comms:read"},
		// The uniform shop's split. The storeroom role holds stock + write and
		// deliberately NOT uniforms:orders, which carries every pupil's name.
		"uni-storeroom": {"uniforms:stock", "uniforms:write"},
		// The same role once the school also lets it take money.
		"uni-cashier": {"uniforms:stock", "uniforms:write", "finance:write"},
		// The director, who may read the order list.
		"uni-director": {"uniforms:stock", "uniforms:write", "uniforms:orders", "finance:write"},
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
		// ADR 0041: every families permission at once (the owner's subset).
		"guardian-student-admin": {"guardians:write", "guardians:read", "students:read", "students:update"},
		"family-reader":          {"family:read"},
		// ADR 0040: the guardian role as iam 0033 leaves it.
		"family-voucher": {"family:read", "vouchers_own:write"},
		// The owner as iam 0033 leaves it: it holds vouchers_own:write to
		// delegate it, never to send for a child it is not linked to.
		"owner-vouchers": {"finance:read", "finance:write", "students:read", "family:read", "vouchers_own:write"},
		// Task 5: health data answers to its own permission, not students:read.
		"medical-reader": {"medical:read", "students:read"},
		// Task 6: writing a note is its own permission, beside reading the pupil.
		"notes-writer": {"students:read", "students:notes_write"},
		// Task 9: the cashier. Reads collections and takes money; nothing else
		// in finance.
		"cobranza": {"students:read", "students:list", "collections:read", "payments:write"},
		// T9: a role the director handed payments:edit (iam 0026, delegable).
		"payments-editor": {"payments:edit"},
		// T10: the Egresos module's two permissions (iam 0027, delegable).
		"expenses-writer": {"expenses:write", "expenses:read"},
		"expenses-reader": {"expenses:read"},
		// T16: the Horarios module's two permissions (iam 0028, delegable).
		"schedule-writer":   {"schedule:write", "schedule:read"},
		"schedule-reader":   {"schedule:read"},
		"activities-writer": {"activities:write", "activities:read"},
		"activities-reader": {"activities:read"},
		// T18: the Becas module's two permissions (iam 0030, delegable).
		"scholarships-writer": {"scholarships:write", "scholarships:read"},
		"scholarships-reader": {"scholarships:read"},
		// T2: the maestro role, exactly as migrations/iam/0022 seeds it.
		"maestro": {"teaching:read", "teaching:write", "attendance_own:read", "attendance_own:write"},
		// "Dar acceso al sistema" needs teachers:write AND users:write, and the
		// caller must hold the maestro role's four to grant it.
		"teacher-access-admin":       {"teachers:write", "users:write", "teaching:read", "teaching:write", "attendance_own:read", "attendance_own:write"},
		"teacher-access-no-users":    {"teachers:write", "teaching:read", "teaching:write", "attendance_own:read", "attendance_own:write"},
		"teacher-access-no-teachers": {"users:write", "teaching:read", "teaching:write", "attendance_own:read", "attendance_own:write"},
		"teacher-access-no-subset":   {"teachers:write", "users:write"},
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
		{Id: "00000000-0000-0000-0000-0000000000a4", Name: "maestro", Permissions: []string{"teaching:read", "teaching:write", "attendance_own:read", "attendance_own:write"}},
	}}), nil
}

// stubStudents returns a canned pending applicant for any CreateApplicant.
type stubStudents struct {
	studentsv1connect.UnimplementedStudentsServiceHandler
}

// VerifyCard answers every code as one valid card (task 13): the gateway tests
// are about who reaches the route, not about the codes.
func (stubStudents) VerifyCard(_ context.Context, _ *connect.Request[studentsv1.VerifyCardRequest]) (*connect.Response[studentsv1.VerifyCardResponse], error) {
	return connect.NewResponse(&studentsv1.VerifyCardResponse{Student: &studentsv1.Student{Id: "stu-1"}, Valid: true}), nil
}

func (stubStudents) CreateApplicant(_ context.Context, req *connect.Request[studentsv1.CreateApplicantRequest]) (*connect.Response[studentsv1.CreateApplicantResponse], error) {
	return connect.NewResponse(&studentsv1.CreateApplicantResponse{Student: &studentsv1.Student{
		Id:              "stu-1",
		Curp:            req.Msg.GetCurp(),
		FirstName:       req.Msg.GetFirstName(),
		LevelId:         req.Msg.GetLevelId(),
		AdmissionStatus: studentsv1.AdmissionStatus_ADMISSION_STATUS_PENDING,
	}}), nil
}

func (stubStudents) ListStudents(_ context.Context, req *connect.Request[studentsv1.ListStudentsRequest]) (*connect.Response[studentsv1.ListStudentsResponse], error) {
	return connect.NewResponse(&studentsv1.ListStudentsResponse{Students: []*studentsv1.Student{
		{Id: "stu-1", FirstName: "Ana", AdmissionStatus: studentsv1.AdmissionStatus_ADMISSION_STATUS_PENDING},
	}}), nil
}

func (stubStudents) AddStudentNote(_ context.Context, req *connect.Request[studentsv1.AddStudentNoteRequest]) (*connect.Response[studentsv1.AddStudentNoteResponse], error) {
	return connect.NewResponse(&studentsv1.AddStudentNoteResponse{Note: &studentsv1.StudentNote{Id: "n-1", Body: req.Msg.GetBody()}}), nil
}

func (stubStudents) ListStudentNotes(_ context.Context, _ *connect.Request[studentsv1.ListStudentNotesRequest]) (*connect.Response[studentsv1.ListStudentNotesResponse], error) {
	return connect.NewResponse(&studentsv1.ListStudentNotesResponse{}), nil
}

func (stubStudents) EditStudentNote(_ context.Context, _ *connect.Request[studentsv1.EditStudentNoteRequest]) (*connect.Response[studentsv1.EditStudentNoteResponse], error) {
	return connect.NewResponse(&studentsv1.EditStudentNoteResponse{Note: &studentsv1.StudentNote{Id: "n-1"}}), nil
}

func (stubStudents) VoidStudentNote(_ context.Context, _ *connect.Request[studentsv1.VoidStudentNoteRequest]) (*connect.Response[studentsv1.VoidStudentNoteResponse], error) {
	return connect.NewResponse(&studentsv1.VoidStudentNoteResponse{Note: &studentsv1.StudentNote{Id: "n-1"}}), nil
}

func (stubStudents) GetStudentMedical(_ context.Context, _ *connect.Request[studentsv1.GetStudentMedicalRequest]) (*connect.Response[studentsv1.GetStudentMedicalResponse], error) {
	return connect.NewResponse(&studentsv1.GetStudentMedicalResponse{Medical: &studentsv1.StudentMedical{BloodType: "O+"}}), nil
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

func (stubStudents) GetEnrollment(_ context.Context, req *connect.Request[studentsv1.GetEnrollmentRequest]) (*connect.Response[studentsv1.GetEnrollmentResponse], error) {
	return connect.NewResponse(&studentsv1.GetEnrollmentResponse{Enrollment: &studentsv1.Enrollment{Id: req.Msg.GetId(), StudentId: "stu-1", CycleId: "cyc-1", Status: studentsv1.EnrollmentStatus_ENROLLMENT_STATUS_ACTIVE}}), nil
}

// ListTransfers: the stub's pupils never moved.
func (stubStudents) ListTransfers(_ context.Context, _ *connect.Request[studentsv1.ListTransfersRequest]) (*connect.Response[studentsv1.ListTransfersResponse], error) {
	return connect.NewResponse(&studentsv1.ListTransfersResponse{}), nil
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

func (stubConfig) SetTermMonths(_ context.Context, req *connect.Request[configv1.SetTermMonthsRequest]) (*connect.Response[configv1.SetTermMonthsResponse], error) {
	return connect.NewResponse(&configv1.SetTermMonthsResponse{Term: &configv1.Term{Id: req.Msg.GetTermId(), Months: req.Msg.GetMonths()}}), nil
}

func (stubConfig) CopyTermMonths(_ context.Context, req *connect.Request[configv1.CopyTermMonthsRequest]) (*connect.Response[configv1.CopyTermMonthsResponse], error) {
	return connect.NewResponse(&configv1.CopyTermMonthsResponse{}), nil
}

// ListEvaluationPeriods: grade "…a9" holds one period, "per-graded" (it has
// grades in stubGrades); every other grade has none.
func (stubConfig) ListEvaluationPeriods(_ context.Context, req *connect.Request[configv1.ListEvaluationPeriodsRequest]) (*connect.Response[configv1.ListEvaluationPeriodsResponse], error) {
	if req.Msg.GetGradeLevelId() != gradeWithGradedPeriod {
		return connect.NewResponse(&configv1.ListEvaluationPeriodsResponse{}), nil
	}
	return connect.NewResponse(&configv1.ListEvaluationPeriodsResponse{Periods: []*configv1.EvaluationPeriod{
		{Id: periodGraded, GradeLevelId: gradeWithGradedPeriod, Ordinal: 1, Name: "1.er trimestre"},
	}}), nil
}

func (stubConfig) UpsertEvaluationPeriods(_ context.Context, _ *connect.Request[configv1.UpsertEvaluationPeriodsRequest]) (*connect.Response[configv1.UpsertEvaluationPeriodsResponse], error) {
	return connect.NewResponse(&configv1.UpsertEvaluationPeriodsResponse{}), nil
}

func (stubConfig) CopyEvaluationPeriods(_ context.Context, _ *connect.Request[configv1.CopyEvaluationPeriodsRequest]) (*connect.Response[configv1.CopyEvaluationPeriodsResponse], error) {
	return connect.NewResponse(&configv1.CopyEvaluationPeriodsResponse{}), nil
}

func (stubConfig) RenameSubject(_ context.Context, req *connect.Request[configv1.RenameSubjectRequest]) (*connect.Response[configv1.RenameSubjectResponse], error) {
	return connect.NewResponse(&configv1.RenameSubjectResponse{Subject: &configv1.Subject{Id: req.Msg.GetId(), Name: req.Msg.GetName()}}), nil
}

func (stubConfig) DeleteSubject(_ context.Context, _ *connect.Request[configv1.DeleteSubjectRequest]) (*connect.Response[configv1.DeleteSubjectResponse], error) {
	return connect.NewResponse(&configv1.DeleteSubjectResponse{}), nil
}

func (stubConfig) ListCycles(_ context.Context, _ *connect.Request[configv1.ListCyclesRequest]) (*connect.Response[configv1.ListCyclesResponse], error) {
	return connect.NewResponse(&configv1.ListCyclesResponse{Cycles: []*configv1.Cycle{
		// Active: the teacher surfaces answer only for the live cycle, and the
		// one teaching assignment below (asg-1) lives in cyc-1.
		{Id: "cyc-1", Name: "2026-2027", Status: configv1.CycleStatus_CYCLE_STATUS_ACTIVE},
	}}), nil
}

func (stubConfig) ActivateCycle(_ context.Context, req *connect.Request[configv1.ActivateCycleRequest]) (*connect.Response[configv1.ActivateCycleResponse], error) {
	return connect.NewResponse(&configv1.ActivateCycleResponse{
		Cycle: &configv1.Cycle{Id: req.Msg.GetId(), Name: "2026-2027", Status: configv1.CycleStatus_CYCLE_STATUS_ACTIVE},
	}), nil
}

func (stubConfig) RenameCycle(_ context.Context, req *connect.Request[configv1.RenameCycleRequest]) (*connect.Response[configv1.RenameCycleResponse], error) {
	return connect.NewResponse(&configv1.RenameCycleResponse{
		Cycle: &configv1.Cycle{Id: req.Msg.GetId(), Name: req.Msg.GetName(), Status: configv1.CycleStatus_CYCLE_STATUS_DRAFT},
	}), nil
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

func (stubConfig) ListLevels(_ context.Context, _ *connect.Request[configv1.ListLevelsRequest]) (*connect.Response[configv1.ListLevelsResponse], error) {
	return connect.NewResponse(&configv1.ListLevelsResponse{Levels: []*configv1.Level{{Id: "lvl-pre", Name: "Preescolar"}}}), nil
}

func (stubConfig) ListGroups(_ context.Context, _ *connect.Request[configv1.ListGroupsRequest]) (*connect.Response[configv1.ListGroupsResponse], error) {
	return connect.NewResponse(&configv1.ListGroupsResponse{Groups: []*configv1.Group{
		{Id: "grp-1", CycleId: "cyc-1", Name: "3° A", GradeLevelId: "gl-1"},
	}}), nil
}

// GetGroup backs one pass list's group → grade → level resolution: grp-1 (the
// only stub group) carries no grade, so its list resolves as daily; any other id
// is NotFound.
func (stubConfig) GetGroup(_ context.Context, req *connect.Request[configv1.GetGroupRequest]) (*connect.Response[configv1.GetGroupResponse], error) {
	// correctionGroup is the uuid-shaped group the T8 group routes act on.
	if id := req.Msg.GetId(); id != "grp-1" && id != correctionGroup {
		return nil, connect.NewError(connect.CodeNotFound, errStr("group not found"))
	}
	return connect.NewResponse(&configv1.GetGroupResponse{Group: &configv1.Group{Id: req.Msg.GetId(), CycleId: "cyc-1", Name: "3° A"}}), nil
}

// ListGradeLevels places the stub group's grade gl-1 in lvl-1, the level whose
// terms the teacher may pick (a term is per cycle AND level, T6). lvl-1 is not
// among ListLevels, so an attendance mode resolved from it reads daily.
func (stubConfig) ListGradeLevels(_ context.Context, _ *connect.Request[configv1.ListGradeLevelsRequest]) (*connect.Response[configv1.ListGradeLevelsResponse], error) {
	return connect.NewResponse(&configv1.ListGradeLevelsResponse{GradeLevels: []*configv1.GradeLevel{
		{Id: "gl-1", LevelId: "lvl-1", Name: "3°"},
	}}), nil
}

// schoolTodayISO is the school's day (America/Mexico_City, ADR 0035), the one
// date a teacher may always save. It used to be UTC's, which after 18:00 in
// Morelia is already tomorrow — "a future date", 400 — so these tests failed
// every evening.
func schoolTodayISO() string {
	zone, err := time.LoadLocation(defaultSchoolTimezone)
	if err != nil {
		panic(err)
	}
	return time.Now().In(zone).Format("2006-01-02")
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
	if req.Msg.GetCycleId() != "cyc-1" || (req.Msg.GetLevelId() != "" && req.Msg.GetLevelId() != "lvl-1") {
		return connect.NewResponse(&configv1.ListTermsResponse{}), nil
	}
	return connect.NewResponse(&configv1.ListTermsResponse{Terms: []*configv1.Term{
		{Id: "trm-1", CycleId: "cyc-1", LevelId: "lvl-1", Name: "Bloque 1"},
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

func (stubFinance) PreviewWithdrawal(_ context.Context, _ *connect.Request[financev1.PreviewWithdrawalRequest]) (*connect.Response[financev1.PreviewWithdrawalResponse], error) {
	return connect.NewResponse(&financev1.PreviewWithdrawalResponse{}), nil
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

// GetDailyRoster answers the daily list with one stored absence. A per-class
// key (the stub group is daily) is a list nobody took — all present, no
// session — so the edge's mode refusal still applies to it (a list that
// already exists would be served in its own shape).
func (stubAttendance) GetDailyRoster(_ context.Context, req *connect.Request[attendancev1.GetDailyRosterRequest]) (*connect.Response[attendancev1.GetDailyRosterResponse], error) {
	if req.Msg.GetSubjectId() != "" {
		return connect.NewResponse(&attendancev1.GetDailyRosterResponse{Rows: []*attendancev1.RosterRow{
			{EnrollmentId: "enr-1", StudentId: "stu-1", Status: "present"},
		}}), nil
	}
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
// stubUniforms answers enough for the ROUTE tests: what is under test here is
// the flag guard and the permission split, not the shop's behaviour.
type stubUniforms struct {
	uniformsv1connect.UnimplementedUniformsServiceHandler
}

func (stubUniforms) ListItems(context.Context, *connect.Request[uniformsv1.ListItemsRequest]) (*connect.Response[uniformsv1.ListItemsResponse], error) {
	return connect.NewResponse(&uniformsv1.ListItemsResponse{}), nil
}

func (stubUniforms) ListStock(context.Context, *connect.Request[uniformsv1.ListStockRequest]) (*connect.Response[uniformsv1.ListStockResponse], error) {
	return connect.NewResponse(&uniformsv1.ListStockResponse{}), nil
}

func (stubUniforms) GetSettings(context.Context, *connect.Request[uniformsv1.GetSettingsRequest]) (*connect.Response[uniformsv1.GetSettingsResponse], error) {
	return connect.NewResponse(&uniformsv1.GetSettingsResponse{Settings: &uniformsv1.Settings{PickupWindowBusinessDays: 10}}), nil
}

func (stubUniforms) ListOrders(context.Context, *connect.Request[uniformsv1.ListOrdersRequest]) (*connect.Response[uniformsv1.ListOrdersResponse], error) {
	return connect.NewResponse(&uniformsv1.ListOrdersResponse{}), nil
}

func (stubUniforms) CounterSale(context.Context, *connect.Request[uniformsv1.CounterSaleRequest]) (*connect.Response[uniformsv1.CounterSaleResponse], error) {
	return connect.NewResponse(&uniformsv1.CounterSaleResponse{Order: &uniformsv1.Order{Id: "ord-1", Folio: "U-2026-000001"}}), nil
}

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
	return newTestGatewayWithReauth(t, flags, nil)
}

// newTestGatewayWithReauth is newTestGateway with the re-authentication
// limiter under the test's control (nil: the same generous one as login).
func newTestGatewayWithReauth(t *testing.T, flags map[string]bool, reauthLimiter *middleware.RateLimiter) *httptest.Server {
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
	uniPath, uniHandler := uniformsv1connect.NewUniformsServiceHandler(stubUniforms{})
	uniSrv := connectServer(t, uniPath, uniHandler)

	// A generous burst keeps the limiter out of the way of these route tests;
	// rate-limit behavior is covered directly in the middleware package.
	loginLimiter := middleware.NewRateLimiter(1000, 1000, time.Now)
	if reauthLimiter == nil {
		reauthLimiter = loginLimiter
	}
	mux := newMux(
		client.NewIAMClient(iamSrv.URL, iamSrv.Client()),
		client.NewAuthClient(iamSrv.URL, iamSrv.Client()),
		client.NewStudentsClient(stuSrv.URL, stuSrv.Client()),
		client.NewConfigClient(cfgSrv.URL, cfgSrv.Client()),
		client.NewGradesClient(grdSrv.URL, grdSrv.Client()),
		client.NewFinanceClient(finSrv.URL, finSrv.Client()),
		client.NewAttendanceClient(attSrv.URL, attSrv.Client()),
		client.NewCommsClient(comSrv.URL, comSrv.Client()),
		client.NewUniformsClient(uniSrv.URL, uniSrv.Client()),
		featureflag.NewInMemory(flags),
		loginLimiter,
		// The invitation doors run on their own budget in production; the test
		// harness passes the same instance because none of these tests probe
		// rate limiting, and sharing keeps the fixture honest about that.
		loginLimiter,
		reauthLimiter,
		[]byte("test-deletion-key-0123456789abcdef"),
	)
	gw := httptest.NewServer(mux)
	t.Cleanup(gw.Close)
	return gw
}

const validBody = `{"curp":"HEGA050312MDFRNN08","firstName":"Ana","lastName":"Herrera","levelId":"lvl-primaria"}`

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

// doGatewayWithHeaders is doGateway plus extra headers, for the routes that
// require one (the counter sale's Idempotency-Key).
func doGatewayWithHeaders(t *testing.T, gw *httptest.Server, method, path, token, body string, headers map[string]string) *http.Response {
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
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	res, err := gw.Client().Do(r)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

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

// T7: the term-months writes ride the config chain — module_config OFF ⇒ 404,
// config:read alone ⇒ 403, config:write ⇒ 200. The table pins the real route
// patterns (the handler tests bind the path value by hand).
func TestTermMonthsRoutesGuardChain(t *testing.T) {
	const (
		putPath  = "/api/v1/config/terms/trm-1/months"
		putBody  = `{"months":["2026-09"]}`
		copyPath = "/api/v1/config/terms/copy-months"
		copyBody = `{"cycleId":"cyc-1","fromLevelId":"lvl-1","toLevelIds":["lvl-2"]}`
	)
	on := map[string]bool{"module_config": true}
	tests := []struct {
		name   string
		flags  map[string]bool
		method string
		path   string
		body   string
		token  string
		want   int
	}{
		{"put: module off hides it", map[string]bool{}, http.MethodPut, putPath, putBody, "config-writer", http.StatusNotFound},
		{"put: no token", on, http.MethodPut, putPath, putBody, "", http.StatusUnauthorized},
		{"put: read-only", on, http.MethodPut, putPath, putBody, "config-reader", http.StatusForbidden},
		{"put: writer", on, http.MethodPut, putPath, putBody, "config-writer", http.StatusOK},
		{"copy: module off hides it", map[string]bool{}, http.MethodPost, copyPath, copyBody, "config-writer", http.StatusNotFound},
		{"copy: read-only", on, http.MethodPost, copyPath, copyBody, "config-reader", http.StatusForbidden},
		{"copy: writer", on, http.MethodPost, copyPath, copyBody, "config-writer", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := newTestGateway(t, tt.flags)
			res := doGateway(t, gw, tt.method, tt.path, tt.token, tt.body)
			if res.StatusCode != tt.want {
				t.Errorf("status = %d, want %d", res.StatusCode, tt.want)
			}
		})
	}
}

// T14: the evaluation-period routes ride the config chain — module_config OFF
// ⇒ 404, no token ⇒ 401, reads need config:read, writes config:write.
func TestEvaluationPeriodRoutesGuardChain(t *testing.T) {
	const (
		cycle    = "00000000-0000-4000-8000-0000000000c1"
		grade    = "00000000-0000-4000-8000-0000000000a1"
		other    = "00000000-0000-4000-8000-0000000000a2"
		listPath = "/api/v1/config/evaluation-periods?cycleId=" + cycle
		putPath  = "/api/v1/config/grade-levels/" + grade + "/evaluation-periods"
		putBody  = `{"cycleId":"` + cycle + `","periods":[]}`
		copyPath = "/api/v1/config/evaluation-periods/copy"
		copyBody = `{"cycleId":"` + cycle + `","fromGradeLevelId":"` + grade + `","toGradeLevelIds":["` + other + `"]}`
	)
	on := map[string]bool{"module_config": true}
	tests := []struct {
		name   string
		flags  map[string]bool
		method string
		path   string
		body   string
		token  string
		want   int
	}{
		{"list: module off hides it", map[string]bool{}, http.MethodGet, listPath, "", "config-reader", http.StatusNotFound},
		{"list: no token", on, http.MethodGet, listPath, "", "", http.StatusUnauthorized},
		{"list: reader", on, http.MethodGet, listPath, "", "config-reader", http.StatusOK},
		{"put: module off hides it", map[string]bool{}, http.MethodPut, putPath, putBody, "config-writer", http.StatusNotFound},
		{"put: read-only", on, http.MethodPut, putPath, putBody, "config-reader", http.StatusForbidden},
		{"put: writer", on, http.MethodPut, putPath, putBody, "config-writer", http.StatusOK},
		{"copy: read-only", on, http.MethodPost, copyPath, copyBody, "config-reader", http.StatusForbidden},
		{"copy: writer", on, http.MethodPost, copyPath, copyBody, "config-writer", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := newTestGateway(t, tt.flags)
			res := doGateway(t, gw, tt.method, tt.path, tt.token, tt.body)
			if res.StatusCode != tt.want {
				t.Errorf("status = %d, want %d", res.StatusCode, tt.want)
			}
		})
	}
}

// T15: the composition root wires the period-grades guard: clearing a grade's
// periods when one of them holds grades is 409 PERIOD_HAS_GRADES, and config
// is never asked to remove it.
func TestEvaluationPeriodWithGradesCannotBeRemoved(t *testing.T) {
	const cycle = "00000000-0000-4000-8000-0000000000c1"
	gw := newTestGateway(t, map[string]bool{"module_config": true})
	res := doGateway(t, gw, http.MethodPut, "/api/v1/config/grade-levels/"+gradeWithGradedPeriod+"/evaluation-periods",
		"config-writer", `{"cycleId":"`+cycle+`","periods":[]}`)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", res.StatusCode)
	}
	if got := res.Header.Get("X-Error-Code"); got != "PERIOD_HAS_GRADES" {
		t.Errorf("X-Error-Code = %q, want PERIOD_HAS_GRADES", got)
	}
}

const gradeWithGradedPeriod = "00000000-0000-4000-8000-0000000000a9"

// T13: renaming and deleting a subject ride the config chain — module_config
// OFF ⇒ 404, no token ⇒ 401, config:read alone ⇒ 403, config:write ⇒ done.
func TestSubjectEditRoutesGuardChain(t *testing.T) {
	const path = "/api/v1/config/subjects/00000000-0000-4000-8000-000000000020"
	const body = `{"name":"Lengua"}`
	on := map[string]bool{"module_config": true}
	tests := []struct {
		name   string
		flags  map[string]bool
		method string
		body   string
		token  string
		want   int
	}{
		{"rename: module off hides it", map[string]bool{}, http.MethodPatch, body, "config-writer", http.StatusNotFound},
		{"rename: no token", on, http.MethodPatch, body, "", http.StatusUnauthorized},
		{"rename: read-only", on, http.MethodPatch, body, "config-reader", http.StatusForbidden},
		{"rename: writer", on, http.MethodPatch, body, "config-writer", http.StatusOK},
		{"delete: module off hides it", map[string]bool{}, http.MethodDelete, "", "config-writer", http.StatusNotFound},
		{"delete: no token", on, http.MethodDelete, "", "", http.StatusUnauthorized},
		{"delete: read-only", on, http.MethodDelete, "", "config-reader", http.StatusForbidden},
		{"delete: writer", on, http.MethodDelete, "", "config-writer", http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := newTestGateway(t, tt.flags)
			res := doGateway(t, gw, tt.method, path, tt.token, tt.body)
			if res.StatusCode != tt.want {
				t.Errorf("status = %d, want %d", res.StatusCode, tt.want)
			}
		})
	}
}

// --- teachers routes: ModuleGuard(teachers) → RequireAuth → RequirePermission
//
// M13 moved the staff roster out of Configuración into a module of its own, so
// the whole guard chain moved with it: the flag is module_teachers and the
// permission is teachers:read/teachers:write. The last test here is the one that
// gives the move its meaning — a config holder no longer reaches the roster.

const validAssignmentBody = `{"groupId":"grp-1","subjectId":"sub-1","teacherId":"tch-1"}`
const validTeacherBody = `{"firstName":"Ada","lastName":"Lovelace","email":"ada@example.com"}`

func TestTeachersRouteDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_teachers OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/teachers", "teachers-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestTeachersWriteNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_teachers": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/teachers", "", validTeacherBody)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestTeachersWriteReadOnlyPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_teachers": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/teachers", "teachers-reader", validTeacherBody)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestTeachersWriteWithPermissionCreates201(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_teachers": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/teachers", "teachers-writer", validTeacherBody)
	if res.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", res.StatusCode)
	}
}

func TestTeachersReadWithPermissionLists200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_teachers": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/teachers", "teachers-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
}

func TestTeachingAssignmentDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_teachers OFF
	res := doGateway(t, gw, http.MethodPost, "/api/v1/teaching-assignments", "teachers-writer", validAssignmentBody)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestTeachingAssignmentWriteReadOnlyPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_teachers": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/teaching-assignments", "teachers-reader", validAssignmentBody)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestTeachingAssignmentWriteWithPermissionCreates201(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_teachers": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/teaching-assignments", "teachers-writer", validAssignmentBody)
	if res.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", res.StatusCode)
	}
}

// The move only means something if the old key stops working. A holder of
// config:write ran the staff roster yesterday; today it must not.
func TestConfigPermissionNoLongerReachesTheStaffRoster(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_teachers": true, "module_config": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/teachers", "config-writer", validTeacherBody)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403 — config:write must not open the roster", res.StatusCode)
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

func TestWithdrawalPreviewRequiresFinanceReadPermission(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true, "module_finance": true})
	denied := doGateway(t, gw, http.MethodGet, "/api/v1/enrollments/enr-1/withdrawal-preview", "student-enroller", "")
	if denied.StatusCode != http.StatusForbidden {
		t.Fatalf("students:enroll alone status = %d, want 403", denied.StatusCode)
	}
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if got := denied.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("denied content type = %q, want application/json", got)
	}
	if err := json.NewDecoder(denied.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "forbidden" || envelope.Error.Message == "" {
		t.Errorf("error envelope = %+v, want forbidden code and non-empty message", envelope.Error)
	}
	denied.Body.Close()

	allowed := doGateway(t, gw, http.MethodGet, "/api/v1/enrollments/enr-1/withdrawal-preview", "student-collections-reader", "")
	defer allowed.Body.Close()
	if allowed.StatusCode != http.StatusOK {
		t.Errorf("students:enroll + collections:read status = %d, want 200", allowed.StatusCode)
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
	res := doGateway(t, gw, http.MethodGet, "/api/v1/grades/roster?assignmentId=a5000000-0000-4000-8000-000000000001&termId=a5000000-0000-4000-8000-000000000002", "grades-reader", "")
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

func TestTeacherLinkDisabledTeachersModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_iam": true}) // module_teachers OFF
	res := doGateway(t, gw, http.MethodPost, "/api/v1/teachers/tch-1/user", "teachers-writer", `{"userId":"usr-1"}`)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestTeacherLinkNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_teachers": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/teachers/tch-1/user", "", `{"userId":"usr-1"}`)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestTeacherLinkReadOnlyPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_teachers": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/teachers/tch-1/user", "teachers-reader", `{"userId":"usr-1"}`)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

func TestTeacherLinkWithPermission200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_teachers": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/teachers/tch-1/user", "teachers-writer", `{"userId":"usr-1"}`)
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
}

// A bogus user id fails the gateway-orchestrated iam validation before any
// config write: iam GetUser NotFound ⇒ 404.
func TestTeacherLinkBogusUserIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/teachers/tch-1/user", "teachers-writer", `{"userId":"ghost"}`)
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
	res := doGateway(t, gw, http.MethodPost, "/api/v1/finance/charges/chg-1/cancel", "finance-writer", `{"reason":"x"}`)
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
		Mode string           `json:"mode"`
		Rows []map[string]any `json:"rows"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Default-present survives the whole chain: an unmarked row reads "present".
	if out.Mode != "daily" || len(out.Rows) != 2 || out.Rows[0]["status"] != "present" || out.Rows[1]["status"] != "absent" {
		t.Errorf("body = %+v", out)
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
		"/api/v1/family/students/stu-1/attendance?month=2026-09", "family-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out struct {
		Month string           `json:"month"`
		Days  []map[string]any `json:"days"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Month != "2026-09" {
		t.Errorf("month = %q", out.Month)
	}
	// The stub stores one absence on 2026-09-01 and its group's list that day.
	var found bool
	for _, d := range out.Days {
		if d["date"] == "2026-09-01" {
			found = true
			if d["status"] != "absent" {
				t.Errorf("2026-09-01 = %v, want absent", d)
			}
		}
	}
	if !found {
		t.Errorf("days = %v, want 2026-09-01", out.Days)
	}
}

func TestFamilyAttendanceMalformedMonthIs400(t *testing.T) {
	gw := newTestGateway(t, familyAttendanceModulesOn)
	res := doGateway(t, gw, http.MethodGet, "/api/v1/family/students/stu-1/attendance?month=2026-13", "family-reader", "")
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", res.StatusCode)
	}
}

func TestFamilyAttendanceDisabledAttendanceModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/family/students/stu-1/attendance", "family-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
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

// --- M07 teaching-attendance (teacher self-scoped daily pass list): every route
// behind RequireAuth + rotation + module guards + attendance_own permission +
// group-ownership scoping. attendance_own:* is NOT attendance:* — a broad
// attendance-staff token must be forbidden here, and a foreign group answers 404.

// GET /api/v1/teaching/groups was the pre-T3 daily-group picker; the web now
// reads /teaching/attendance/classes, so the route is gone — even for a token
// that holds attendance_own:read with every module on.
func TestTeachingGroupsRouteIsGone(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_attendance": true, "module_config": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/teaching/groups", "att-own-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (route removed)", res.StatusCode)
	}
}

func TestTeachingAttendanceRosterDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true}) // module_attendance OFF
	res := doGateway(t, gw, http.MethodGet, "/api/v1/teaching/attendance/roster?groupId=grp-1&date=2026-09-01", "att-own-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestTeachingAttendanceRosterNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_attendance": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/teaching/attendance/roster?groupId=grp-1&date=2026-09-01", "", "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

func TestTeachingAttendanceRosterWithoutAttendanceOwnReadIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_attendance": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/teaching/attendance/roster?groupId=grp-1&date=2026-09-01", "att-reader", "")
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (attendance:read is not attendance_own:read)", res.StatusCode)
	}
}

func TestTeachingAttendanceRosterOwnedGroupReturns200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_attendance": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/teaching/attendance/roster?groupId=grp-1&date=2026-09-01", "att-own-reader", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out struct {
		Rows []map[string]any `json:"rows"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// The gateway resolves the student name for the Maestro (who holds no
	// students:read): the stub roster's stu-1 comes back named "Ana".
	if len(out.Rows) != 2 || out.Rows[0]["enrollmentId"] != "enr-1" ||
		out.Rows[0]["studentId"] != "stu-1" || out.Rows[0]["name"] != "Ana" ||
		out.Rows[0]["status"] != "present" {
		t.Errorf("rows = %v, want enr-1/stu-1/Ana/present enriched", out.Rows)
	}
}

// A group the caller does not teach answers 404 — the ownership guard runs before
// the attendance service is ever consulted.
func TestTeachingAttendanceRosterForeignGroupIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_attendance": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/teaching/attendance/roster?groupId=grp-999&date=2026-09-01", "att-own-reader", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (not the caller's group)", res.StatusCode)
	}
}

func TestTeachingAttendanceSaveDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true}) // module_attendance OFF
	res := doGateway(t, gw, http.MethodPost, "/api/v1/teaching/attendance", "att-own-writer",
		`{"groupId":"grp-1","date":"2026-09-01","marks":[]}`)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestTeachingAttendanceSaveNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_attendance": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/teaching/attendance", "",
		`{"groupId":"grp-1","date":"2026-09-01","marks":[]}`)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

// attendance_own:read is not attendance_own:write: a reader cannot save a day.
func TestTeachingAttendanceSaveWithoutAttendanceOwnWriteIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_attendance": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/teaching/attendance", "att-own-reader",
		`{"groupId":"grp-1","date":"2026-09-01","marks":[]}`)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (attendance_own:read is not attendance_own:write)", res.StatusCode)
	}
}

func TestTeachingAttendanceSaveOwnedGroupReturns200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_attendance": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/teaching/attendance", "att-own-writer",
		`{"groupId":"grp-1","date":"`+schoolTodayISO()+`","marks":[{"enrollmentId":"enr-1","status":"absent"}]}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["absentCount"] != float64(1) || out["markedCount"] != float64(1) {
		t.Errorf("summary = %v, want absentCount 1 / markedCount 1", out)
	}
}

// Saving against a group the caller does not teach answers 404 — the ownership
// guard runs before the attendance service is ever consulted.
func TestTeachingAttendanceSaveForeignGroupIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_attendance": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/teaching/attendance", "att-own-writer",
		`{"groupId":"grp-999","date":"2026-09-01","marks":[]}`)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (not the caller's group)", res.StatusCode)
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

// --- M12 uniforms routes -----------------------------------------------------

func TestUniformsDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_uniforms OFF
	// Even the director must not learn the module exists: a disabled module is
	// invisible, not merely refused.
	for _, path := range []string{
		"/api/v1/uniforms/items",
		"/api/v1/uniforms/stock",
		"/api/v1/uniforms/orders",
		"/api/v1/uniforms/settings",
		"/api/v1/uniforms/levels",
	} {
		res := doGateway(t, gw, http.MethodGet, path, "uni-director", "")
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", path, res.StatusCode)
		}
	}
}

func TestUniformsNoTokenIs401(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_uniforms": true})
	res := doGateway(t, gw, http.MethodGet, "/api/v1/uniforms/items", "", "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}

// TestUniformsStoreroomCanRunTheStoreroom is the point of splitting the read: a
// Caja/Almacen role that holds no student data must still be able to do the whole
// of its job.
func TestUniformsStoreroomCanRunTheStoreroom(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_uniforms": true})
	// levels included: the storeroom sees the school's levels for its chips
	// without holding config:read (ADR 0014).
	for _, path := range []string{"/api/v1/uniforms/items", "/api/v1/uniforms/stock", "/api/v1/uniforms/settings", "/api/v1/uniforms/levels"} {
		res := doGateway(t, gw, http.MethodGet, path, "uni-storeroom", "")
		if res.StatusCode != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", path, res.StatusCode)
		}
	}
}

// TestUniformsOrderListIsOwnerOnly is the other half. The order list carries
// every pupil's name and purchase history for the whole school, so holding
// uniforms:stock is deliberately not enough to see it.
func TestUniformsOrderListIsOwnerOnly(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_uniforms": true})

	res := doGateway(t, gw, http.MethodGet, "/api/v1/uniforms/orders", "uni-storeroom", "")
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("the storeroom read the school roll: status = %d, want 403", res.StatusCode)
	}
	res = doGateway(t, gw, http.MethodGet, "/api/v1/uniforms/orders", "uni-director", "")
	if res.StatusCode != http.StatusOK {
		t.Errorf("the director cannot read the order list: status = %d, want 200", res.StatusCode)
	}
}

// TestUniformsCounterSaleNeedsBothPermissions is the composition: the sale moves
// goods AND takes money, so holding one right is not enough.
func TestUniformsCounterSaleNeedsBothPermissions(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_uniforms": true})
	body := `{"lines":[{"itemId":"it-1","sizeLabel":"8","qty":1}],"method":"cash","note":"x"}`

	res := doGatewayWithHeaders(t, gw, http.MethodPost, "/api/v1/uniforms/counter-sale",
		"uni-storeroom", body, map[string]string{"Idempotency-Key": "till-1"})
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("without finance:write: status = %d, want 403", res.StatusCode)
	}

	// And the refusal NAMES what is missing, so the panel can disable the entry
	// point and say why rather than letting a cashier key in six lines first.
	var refusal struct {
		Error struct {
			Details map[string]string `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&refusal); err != nil {
		t.Fatalf("the refusal is not the shared envelope: %v", err)
	}
	if refusal.Error.Details["missingPermission"] != "finance:write" {
		t.Errorf("the refusal does not name the missing permission: %v", refusal.Error.Details)
	}

	res = doGatewayWithHeaders(t, gw, http.MethodPost, "/api/v1/uniforms/counter-sale",
		"uni-cashier", body, map[string]string{"Idempotency-Key": "till-2"})
	if res.StatusCode != http.StatusCreated {
		t.Errorf("with both permissions: status = %d, want 201", res.StatusCode)
	}
}

// TestUniformsCounterSaleWithoutTheIdempotencyHeaderIs400: the header is rejected
// at the edge so the message names it, rather than surfacing as a generic
// invalid-argument from two services away.
func TestUniformsCounterSaleWithoutTheIdempotencyHeaderIs400(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_uniforms": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/uniforms/counter-sale", "uni-cashier",
		`{"lines":[],"method":"cash"}`)
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", res.StatusCode)
	}
}

// TestUniformsWritesRefuseAReadOnlyCaller: creating a garment or moving stock is
// not something uniforms:stock alone may do.
func TestUniformsWritesRefuseAReadOnlyCaller(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_uniforms": true})
	// A caller with only the read half of the storeroom permission.
	res := doGateway(t, gw, http.MethodPost, "/api/v1/uniforms/items", "comms-reader",
		`{"code":"PLA","name":"Playera","priceCents":32000}`)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
}

// --- activating a school year (M04 lifecycle, closed in the demo prep) -------
//
// A cycle was created as a draft and had no way out, so every school year read
// "Borrador" and the panel resolved "the active cycle" to whichever came first.
// The route is a config WRITE: it changes which year the whole product is about.

// CC-103: renaming a year is a config write behind module_config.
func TestRenameCycleRoute(t *testing.T) {
	const path = "/api/v1/config/cycles/00000000-0000-4000-8000-0000000000c1"
	cases := map[string]struct {
		flags  map[string]bool
		caller string
		want   int
	}{
		"module off":   {map[string]bool{}, "config-writer", http.StatusNotFound},
		"read only":    {map[string]bool{"module_config": true}, "config-reader", http.StatusForbidden},
		"config:write": {map[string]bool{"module_config": true}, "config-writer", http.StatusOK},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			gw := newTestGateway(t, tc.flags)
			res := doGateway(t, gw, http.MethodPatch, path, tc.caller, `{"name":"2027-2028"}`)
			if res.StatusCode != tc.want {
				t.Errorf("status = %d, want %d", res.StatusCode, tc.want)
			}
		})
	}
}

func TestActivateCycleDisabledModuleIs404(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{}) // module_config OFF
	res := doGateway(t, gw, http.MethodPost, "/api/v1/config/cycles/cyc-1/activate", "config-writer", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestActivateCycleReadOnlyPermissionIs403(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/config/cycles/cyc-1/activate", "config-reader", "")
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403 — choosing the live year is a write", res.StatusCode)
	}
}

func TestActivateCycleWithPermission200(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_config": true})
	res := doGateway(t, gw, http.MethodPost, "/api/v1/config/cycles/cyc-1/activate", "config-writer", "")
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
}

// A pupil's medical record is not part of "reading the pupil": students:read
// alone is refused, medical:read is served.
func TestMedicalRecordAnswersToItsOwnPermission(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	if res := doGateway(t, gw, http.MethodGet, "/api/v1/students/stu-1/medical", "student-reader", ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("students:read alone: status = %d, want 403", res.StatusCode)
	}
	if res := doGateway(t, gw, http.MethodGet, "/api/v1/students/stu-1/medical", "medical-reader", ""); res.StatusCode != http.StatusOK {
		t.Errorf("medical:read: status = %d, want 200", res.StatusCode)
	}
}

// Notes on a pupil (task 6): reading them is reading the pupil; writing and
// voiding and editing need students:notes_write.
func TestStudentNotesAnswerToTheirPermissions(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_students": true})
	cases := []struct {
		method, path, token, body string
		want                      int
	}{
		{http.MethodGet, "/api/v1/students/stu-1/notes", "student-reader", "", http.StatusOK},
		{http.MethodPost, "/api/v1/students/stu-1/notes", "student-reader", `{"body":"x"}`, http.StatusForbidden},
		{http.MethodPost, "/api/v1/students/stu-1/notes", "notes-writer", `{"body":"x"}`, http.StatusCreated},
		{http.MethodPost, "/api/v1/students/stu-1/notes/n-1/void", "student-reader", "", http.StatusForbidden},
		{http.MethodPost, "/api/v1/students/stu-1/notes/n-1/void", "notes-writer", "", http.StatusOK},
		{http.MethodPatch, "/api/v1/students/stu-1/notes/n-1", "student-reader", `{"body":"y"}`, http.StatusForbidden},
		{http.MethodPatch, "/api/v1/students/stu-1/notes/n-1", "notes-writer", `{"body":"y"}`, http.StatusOK},
	}
	for _, tc := range cases {
		if res := doGateway(t, gw, tc.method, tc.path, tc.token, tc.body); res.StatusCode != tc.want {
			t.Errorf("%s %s as %s: status = %d, want %d", tc.method, tc.path, tc.token, res.StatusCode, tc.want)
		}
	}
}

// Task 9: the Cobranza role reaches exactly the counter — balances, what is
// due, what is late, the day's cut, and taking money — and nothing that
// prices, bills, discounts, cancels, voids or reports income.
func TestCobranzaReachesTheCounterAndNothingElse(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_finance": true, "module_students": true, "module_iam": true})
	allowed := []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/finance/accounts/stu-1", ""},
		{http.MethodGet, "/api/v1/finance/expected?studentId=stu-1", ""},
		{http.MethodGet, "/api/v1/finance/overdue?cycleId=cyc-1", ""},
		{http.MethodGet, "/api/v1/finance/charges?studentId=stu-1", ""},
		{http.MethodGet, "/api/v1/finance/cash-up", ""},
		{http.MethodGet, "/api/v1/finance/deposits?studentId=stu-1", ""},
		{http.MethodPost, "/api/v1/finance/deposits/preview", `{"studentId":"stu-1","amountCents":100}`},
		{http.MethodPost, "/api/v1/finance/deposits", `{"studentId":"stu-1","amountCents":100,"method":"cash"}`},
		{http.MethodGet, "/api/v1/students", ""},
	}
	for _, c := range allowed {
		res := doGateway(t, gw, c.method, c.path, "cobranza", c.body)
		if res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusNotFound {
			t.Errorf("cobranza %s %s: status %d, want it reachable", c.method, c.path, res.StatusCode)
		}
	}
	denied := []struct{ method, path, body string }{
		{http.MethodPut, "/api/v1/finance/tuition-plans", `{}`},
		{http.MethodGet, "/api/v1/finance/tuition-plans?cycleId=cyc-1", ""},
		{http.MethodPost, "/api/v1/finance/billing-runs", `{}`},
		{http.MethodPost, "/api/v1/finance/concepts", `{}`},
		{http.MethodPost, "/api/v1/finance/charges", `{}`},
		{http.MethodPost, "/api/v1/finance/charges/ch-1/discount", `{}`},
		{http.MethodPost, "/api/v1/finance/charges/ch-1/cancel", ""},
		{http.MethodPost, "/api/v1/finance/deposits/dep-1/void", `{"reason":"x"}`},
		// Paying one charge by id takes its stored amount: no daily late fee, no
		// inscription climb. The counter collects through deposits, priced.
		{http.MethodPost, "/api/v1/finance/payments", `{"chargeId":"ch-1","amountCents":100,"method":"cash"}`},
		{http.MethodGet, "/api/v1/finance/income?from=2026-09-01&to=2026-09-30", ""},
		// The whole cycle's ledger is not the counter's: one pupil at a time.
		{http.MethodGet, "/api/v1/finance/charges?cycleId=cyc-1", ""},
		{http.MethodGet, "/api/v1/finance/deposits", ""},
		{http.MethodGet, "/api/v1/users", ""},
	}
	for _, c := range denied {
		if res := doGateway(t, gw, c.method, c.path, "cobranza", c.body); res.StatusCode != http.StatusForbidden {
			t.Errorf("cobranza %s %s: status %d, want 403", c.method, c.path, res.StatusCode)
		}
	}
}

// Whoever holds finance:read still reads the whole ledger: the owner's
// Finanzas lists a cycle's charges and every deposit.
func TestFinanceReadStillReadsTheWholeLedger(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_finance": true})
	for _, path := range []string{"/api/v1/finance/charges?cycleId=cyc-1", "/api/v1/finance/deposits"} {
		if res := doGateway(t, gw, http.MethodGet, path, "finance-reader", ""); res.StatusCode == http.StatusForbidden {
			t.Errorf("finance:read %s: 403", path)
		}
	}
}

// Task 12: a payment's receipt answers to whoever can see the money (the
// counter or finance); a sale's to whoever reads orders. A teacher reaches
// neither.
func TestReceiptsAnswerToWhoCanSeeTheMoney(t *testing.T) {
	gw := newTestGateway(t, map[string]bool{"module_finance": true, "module_uniforms": true})
	cases := []struct {
		path, token string
		reachable   bool
	}{
		{"/api/v1/receipts/payment/dep-1", "cobranza", true},
		{"/api/v1/receipts/payment/dep-1/pdf", "finance-reader", true},
		{"/api/v1/receipts/payment/dep-1", "teaching-writer", false},
		{"/api/v1/receipts/sale/ord-1", "uni-director", true},
		{"/api/v1/receipts/sale/ord-1/pdf", "cobranza", false},
		{"/api/v1/receipts/sale/ord-1", "teaching-writer", false},
	}
	for _, c := range cases {
		res := doGateway(t, gw, http.MethodGet, c.path, c.token, "")
		denied := res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusUnauthorized
		if c.reachable == denied || res.StatusCode == http.StatusNotFound && c.reachable {
			t.Errorf("%s as %s: status %d, reachable want %v", c.path, c.token, res.StatusCode, c.reachable)
		}
	}
}

// Task 13: the card lives behind module_student_card — off, every route is 404
// before auth runs. On, a family reads its child's card; staff with
// students:read verify and with students:update revoke; a family cannot verify.
func TestTheCardSitsBehindItsFlag(t *testing.T) {
	routes := []struct{ method, path, body, token string }{
		{http.MethodGet, "/api/v1/family/students/stu-1/card", "", "family-reader"},
		{http.MethodGet, "/api/v1/family/students/stu-1/photo", "", "family-reader"},
		{http.MethodPost, "/api/v1/students/card/verify", `{"code":"GLCARD:AQAAAAAAAAAAAAAAAAAAAA"}`, "student-reader"},
		{http.MethodPost, "/api/v1/students/stu-1/card/revoke", "", "roles-writer"},
		{http.MethodPost, "/api/v1/students/stu-1/card/reissue", "", "roles-writer"},
	}
	off := newTestGateway(t, map[string]bool{"module_students": true})
	for _, r := range routes {
		if res := doGateway(t, off, r.method, r.path, r.token, r.body); res.StatusCode != http.StatusNotFound {
			t.Errorf("flag off: %s %s = %d, want 404", r.method, r.path, res.StatusCode)
		}
	}

	on := newTestGateway(t, map[string]bool{"module_students": true, "module_student_card": true})
	if res := doGateway(t, on, http.MethodPost, "/api/v1/students/card/verify", "family-reader", `{"code":"x"}`); res.StatusCode != http.StatusForbidden {
		t.Errorf("a family verifying a card: %d, want 403", res.StatusCode)
	}
	for _, action := range []string{"revoke", "reissue"} {
		if res := doGateway(t, on, http.MethodPost, "/api/v1/students/stu-1/card/"+action, "student-reader", ""); res.StatusCode != http.StatusForbidden {
			t.Errorf("%s with students:read only: %d, want 403", action, res.StatusCode)
		}
	}
	if res := doGateway(t, on, http.MethodPost, "/api/v1/students/card/verify", "student-reader", `{"code":"x"}`); res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusUnauthorized {
		t.Errorf("staff verifying a card: %d, want it reachable", res.StatusCode)
	}
}

// T7: the periods and report cards sit behind module_grades. Reads answer to
// grades:read; closing and reopening ONLY to grades:close (owner-only), so a
// grades:write holder or a teacher is 403. A family reaches its child's report
// cards with family:read (the ownership check itself is the handler's).
func TestPeriodsAndReportCardsAuthz(t *testing.T) {
	const (
		term   = "70000000-0000-0000-0000-000000000001"
		cycle  = "c0000000-0000-0000-0000-000000000001"
		card   = "7c000000-0000-0000-0000-000000000001"
		pupil  = "57000000-0000-0000-0000-000000000001"
		family = "/api/v1/family/students/" + pupil + "/report-cards"
	)
	routes := []struct {
		method, path, token string
		reachable           bool
	}{
		{http.MethodGet, "/api/v1/grades/periods?cycleId=" + cycle, "grades-reader", true},
		{http.MethodGet, "/api/v1/grades/periods?cycleId=" + cycle, "maestro", false},
		{http.MethodGet, "/api/v1/grades/periods/" + term + "/progress", "grades-reader", true},
		{http.MethodGet, "/api/v1/grades/periods/" + term + "/progress", "teaching-writer", false},
		{http.MethodPost, "/api/v1/grades/periods/" + term + "/close", "grades-closer", true},
		{http.MethodPost, "/api/v1/grades/periods/" + term + "/close", "grades-writer", false},
		{http.MethodPost, "/api/v1/grades/periods/" + term + "/close", "maestro", false},
		{http.MethodPost, "/api/v1/grades/periods/" + term + "/reopen", "grades-closer", true},
		{http.MethodPost, "/api/v1/grades/periods/" + term + "/reopen", "grades-writer", false},
		{http.MethodGet, "/api/v1/grades/report-cards?termId=" + term, "grades-reader", true},
		{http.MethodGet, "/api/v1/grades/report-cards?termId=" + term, "maestro", false},
		{http.MethodGet, "/api/v1/grades/report-cards/" + card, "grades-reader", true},
		{http.MethodGet, "/api/v1/grades/report-cards/" + card + "/pdf", "grades-reader", true},
		{http.MethodGet, "/api/v1/grades/report-cards/" + card + "/pdf", "family-reader", false},
		{http.MethodGet, family, "family-reader", true},
		{http.MethodGet, family + "/" + card, "family-reader", true},
		{http.MethodGet, family + "/" + card + "/pdf", "family-reader", true},
		{http.MethodGet, family, "grades-reader", false},
	}
	on := newTestGateway(t, map[string]bool{"module_grades": true, "module_students": true})
	for _, r := range routes {
		res := doGateway(t, on, r.method, r.path, r.token, "")
		denied := res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusUnauthorized
		// A reachable staff route must also EXIST (a 404 would be a missing route);
		// a family route may 404 from the ownership check against the stub.
		missing := r.reachable && res.StatusCode == http.StatusNotFound && !strings.HasPrefix(r.path, "/api/v1/family/")
		if denied == r.reachable || missing || res.StatusCode == http.StatusMethodNotAllowed {
			t.Errorf("%s %s as %s: status %d, reachable want %v", r.method, r.path, r.token, res.StatusCode, r.reachable)
		}
	}

	off := newTestGateway(t, map[string]bool{"module_students": true})
	for _, r := range routes {
		if res := doGateway(t, off, r.method, r.path, r.token, ""); res.StatusCode != http.StatusNotFound {
			t.Errorf("module_grades off: %s %s = %d, want 404", r.method, r.path, res.StatusCode)
		}
	}
	familyOnly := newTestGateway(t, map[string]bool{"module_grades": true})
	if res := doGateway(t, familyOnly, http.MethodGet, family, "family-reader", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("module_students off: family report cards = %d, want 404", res.StatusCode)
	}
}
