package grpcadapter

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/adapters/protoconv"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

// Compile-time proof the adapter implements the generated Connect handler.
var _ studentsv1connect.StudentsServiceHandler = (*StudentsHandler)(nil)

// StudentsHandler adapts the students use cases to the generated
// garcialorca.students.v1 Connect service. It only translates: proto messages ↔
// use-case calls, and domain error Kinds ↔ Connect codes. All logic lives in
// usecase/domain.
type StudentsHandler struct {
	svc      *usecase.StudentService
	enroll   *usecase.EnrollmentService
	guardian *usecase.GuardianService
	health   *usecase.HealthService
}

// NewStudentsHandler wires the handler to the student, enrollment, guardian, and
// health use cases.
func NewStudentsHandler(
	svc *usecase.StudentService,
	enroll *usecase.EnrollmentService,
	guardian *usecase.GuardianService,
	health *usecase.HealthService,
) *StudentsHandler {
	return &StudentsHandler{svc: svc, enroll: enroll, guardian: guardian, health: health}
}

func (h *StudentsHandler) Health(ctx context.Context, _ *connect.Request[studentsv1.HealthRequest]) (*connect.Response[studentsv1.HealthResponse], error) {
	status, err := h.health.Ping(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.HealthResponse{Status: status.Status}), nil
}

func (h *StudentsHandler) CreateApplicant(ctx context.Context, req *connect.Request[studentsv1.CreateApplicantRequest]) (*connect.Response[studentsv1.CreateApplicantResponse], error) {
	m := req.Msg
	student, err := h.svc.CreateApplicant(ctx, usecase.CreateApplicantInput{
		CURP:         m.GetCurp(),
		FirstName:    m.GetFirstName(),
		LastName:     m.GetLastName(),
		Level:        protoconv.ToDomainLevel(m.GetLevel()),
		GradeGroup:   m.GetGradeGroup(),
		GuardianName: m.GetGuardianName(),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.CreateApplicantResponse{Student: toProtoStudent(student)}), nil
}

func (h *StudentsHandler) GetStudent(ctx context.Context, req *connect.Request[studentsv1.GetStudentRequest]) (*connect.Response[studentsv1.GetStudentResponse], error) {
	student, err := h.svc.GetStudent(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.GetStudentResponse{Student: toProtoStudent(student)}), nil
}

func (h *StudentsHandler) ListStudents(ctx context.Context, req *connect.Request[studentsv1.ListStudentsRequest]) (*connect.Response[studentsv1.ListStudentsResponse], error) {
	m := req.Msg
	students, next, err := h.svc.ListStudents(ctx, ports.ListFilter{
		Level:      protoconv.ToDomainLevel(m.GetLevel()),
		Status:     protoconv.ToDomainStatus(m.GetStatus()),
		GradeGroup: m.GetGradeGroup(),
		PageSize:   int(m.GetPageSize()),
		PageToken:  m.GetPageToken(),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*studentsv1.Student, 0, len(students))
	for _, s := range students {
		out = append(out, toProtoStudent(s))
	}
	return connect.NewResponse(&studentsv1.ListStudentsResponse{Students: out, NextPageToken: next}), nil
}

func (h *StudentsHandler) ChangeAdmissionStatus(ctx context.Context, req *connect.Request[studentsv1.ChangeAdmissionStatusRequest]) (*connect.Response[studentsv1.ChangeAdmissionStatusResponse], error) {
	m := req.Msg
	student, err := h.svc.ChangeAdmissionStatus(ctx, usecase.ChangeStatusInput{
		ID:      m.GetId(),
		Target:  protoconv.ToDomainStatus(m.GetTarget()),
		Reason:  m.GetReason(),
		CycleID: m.GetCycleId(),
	})
	if err != nil {
		// The frozen contract maps an illegal admission transition (KindConflict
		// here) to FailedPrecondition — the resource is in the wrong state — as
		// opposed to a duplicate on CreateApplicant, which is AlreadyExists.
		if glerrors.KindOf(err) == glerrors.KindConflict {
			return nil, connect.NewError(connect.CodeFailedPrecondition, err)
		}
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.ChangeAdmissionStatusResponse{Student: toProtoStudent(student)}), nil
}

// Enroll places a student into a group of a cycle. The cycle is DERIVED
// server-side from the group (never a client arg); only student_id, group_id and
// the optional term_id are passed through. A duplicate (student, cycle) surfaces
// as AlreadyExists via the shared error mapper.
func (h *StudentsHandler) Enroll(ctx context.Context, req *connect.Request[studentsv1.EnrollRequest]) (*connect.Response[studentsv1.EnrollResponse], error) {
	m := req.Msg
	enrollment, err := h.enroll.Enroll(ctx, m.GetStudentId(), m.GetGroupId(), m.GetTermId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.EnrollResponse{Enrollment: toProtoEnrollment(enrollment)}), nil
}

// EnrollIntoGrade places a pupil in a grade and lets capacity choose the group.
func (h *StudentsHandler) EnrollIntoGrade(ctx context.Context, req *connect.Request[studentsv1.EnrollIntoGradeRequest]) (*connect.Response[studentsv1.EnrollIntoGradeResponse], error) {
	m := req.Msg
	placed, err := h.enroll.EnrollIntoGrade(ctx, m.GetStudentId(), m.GetGradeLevelId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.EnrollIntoGradeResponse{
		Enrollment: toProtoEnrollment(placed.Enrollment),
		GroupName:  placed.GroupName,
	}), nil
}

func (h *StudentsHandler) GetEnrollment(ctx context.Context, req *connect.Request[studentsv1.GetEnrollmentRequest]) (*connect.Response[studentsv1.GetEnrollmentResponse], error) {
	enrollment, err := h.enroll.GetEnrollment(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.GetEnrollmentResponse{Enrollment: toProtoEnrollment(enrollment)}), nil
}

func (h *StudentsHandler) ListEnrollments(ctx context.Context, req *connect.Request[studentsv1.ListEnrollmentsRequest]) (*connect.Response[studentsv1.ListEnrollmentsResponse], error) {
	m := req.Msg
	enrollments, err := h.enroll.ListEnrollments(ctx, m.GetStudentId(), m.GetCycleId(), m.GetGroupId())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*studentsv1.Enrollment, 0, len(enrollments))
	for _, e := range enrollments {
		out = append(out, toProtoEnrollment(e))
	}
	return connect.NewResponse(&studentsv1.ListEnrollmentsResponse{Enrollments: out}), nil
}

// TransferStudent moves an active enrollment to another group in the SAME cycle.
// A non-active enrollment, a cross-cycle destination, or a same-group move are
// all InvalidArgument via the shared mapper.
func (h *StudentsHandler) TransferStudent(ctx context.Context, req *connect.Request[studentsv1.TransferStudentRequest]) (*connect.Response[studentsv1.TransferStudentResponse], error) {
	m := req.Msg
	enrollment, err := h.enroll.Transfer(ctx, m.GetEnrollmentId(), m.GetToGroupId(), m.GetReason())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.TransferStudentResponse{Enrollment: toProtoEnrollment(enrollment)}), nil
}

func (h *StudentsHandler) WithdrawEnrollment(ctx context.Context, req *connect.Request[studentsv1.WithdrawEnrollmentRequest]) (*connect.Response[studentsv1.WithdrawEnrollmentResponse], error) {
	enrollment, err := h.enroll.Withdraw(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.WithdrawEnrollmentResponse{Enrollment: toProtoEnrollment(enrollment)}), nil
}

// --- guardian links --------------------------------------------------------

// LinkGuardian links an iam user (validated by RPC at the use-case layer) to a
// student. A guardian_user_id that is unknown to iam, or that does not hold the
// guardian role, is an InvalidArgument — not a NotFound: the resource addressed by
// the call is the student, and the user id is an argument. A duplicate link
// surfaces as AlreadyExists via the shared error mapper.
func (h *StudentsHandler) LinkGuardian(ctx context.Context, req *connect.Request[studentsv1.LinkGuardianRequest]) (*connect.Response[studentsv1.LinkGuardianResponse], error) {
	m := req.Msg
	link, err := h.guardian.LinkGuardian(ctx, m.GetGuardianUserId(), m.GetStudentId(), m.GetRelationship())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.LinkGuardianResponse{Link: toProtoGuardianLink(link)}), nil
}

// UnlinkGuardian removes a guardian ↔ student link. An unknown link is NotFound.
func (h *StudentsHandler) UnlinkGuardian(ctx context.Context, req *connect.Request[studentsv1.UnlinkGuardianRequest]) (*connect.Response[studentsv1.UnlinkGuardianResponse], error) {
	m := req.Msg
	if err := h.guardian.UnlinkGuardian(ctx, m.GetGuardianUserId(), m.GetStudentId()); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.UnlinkGuardianResponse{}), nil
}

func (h *StudentsHandler) ListGuardiansForStudent(ctx context.Context, req *connect.Request[studentsv1.ListGuardiansForStudentRequest]) (*connect.Response[studentsv1.ListGuardiansForStudentResponse], error) {
	links, err := h.guardian.ListGuardiansForStudent(ctx, req.Msg.GetStudentId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.ListGuardiansForStudentResponse{Links: toProtoGuardianLinks(links)}), nil
}

func (h *StudentsHandler) ListStudentsForGuardian(ctx context.Context, req *connect.Request[studentsv1.ListStudentsForGuardianRequest]) (*connect.Response[studentsv1.ListStudentsForGuardianResponse], error) {
	links, err := h.guardian.ListStudentsForGuardian(ctx, req.Msg.GetGuardianUserId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&studentsv1.ListStudentsForGuardianResponse{Links: toProtoGuardianLinks(links)}), nil
}

// --- mapping ---------------------------------------------------------------

func toProtoStudent(s *domain.Student) *studentsv1.Student {
	return &studentsv1.Student{
		Id:              s.ID(),
		TenantId:        s.TenantID(),
		Curp:            s.CURP().String(),
		FirstName:       s.FirstName(),
		LastName:        s.LastName(),
		Level:           protoconv.ToProtoLevel(s.Level()),
		GradeGroup:      s.GradeGroup(),
		GuardianName:    s.GuardianName(),
		AdmissionStatus: protoconv.ToProtoStatus(s.Status()),
	}
}

func toProtoEnrollment(e domain.Enrollment) *studentsv1.Enrollment {
	return &studentsv1.Enrollment{
		Id:         e.ID(),
		TenantId:   e.TenantID(),
		StudentId:  e.StudentID(),
		CycleId:    e.CycleID(),
		GroupId:    e.GroupID(),
		TermId:     e.TermID(),
		Status:     protoconv.ToProtoEnrollmentStatus(e.Status()),
		EnrolledAt: timestamppb.New(e.EnrolledAt()),
	}
}

// toProtoGuardianLink projects a domain GuardianLink onto the wire. The
// relationship travels as its canonical name (the DB CHECK vocabulary).
func toProtoGuardianLink(l domain.GuardianLink) *studentsv1.GuardianLink {
	return &studentsv1.GuardianLink{
		Id:             l.ID(),
		TenantId:       l.TenantID(),
		GuardianUserId: l.GuardianUserID(),
		StudentId:      l.StudentID(),
		Relationship:   l.Relationship().String(),
	}
}

func toProtoGuardianLinks(links []domain.GuardianLink) []*studentsv1.GuardianLink {
	out := make([]*studentsv1.GuardianLink, 0, len(links))
	for _, l := range links {
		out = append(out, toProtoGuardianLink(l))
	}
	return out
}

// connectErr maps a domain error Kind to the Connect status code. Internal
// failures return a generic message (no internal detail leaks); the other Kinds
// carry already-generic domain messages.
// Every code goes through glerrors.SafeMessage, not just the internal one. The
// kind decides the STATUS; it does not decide whether the wrapped cause is the
// caller's business, and it never is. Error() concatenates the cause, so handing
// the error straight to connect.NewError shipped Postgres constraint names and
// SQLSTATEs — and on some constraints the conflicting value — to whoever called.
func connectErr(err error) *connect.Error {
	safe := errors.New(glerrors.SafeMessage(err))

	switch glerrors.KindOf(err) {
	case glerrors.KindInvalid:
		return connect.NewError(connect.CodeInvalidArgument, safe)
	case glerrors.KindUnauthorized:
		return connect.NewError(connect.CodeUnauthenticated, safe)
	case glerrors.KindForbidden:
		return connect.NewError(connect.CodePermissionDenied, safe)
	case glerrors.KindNotFound:
		return connect.NewError(connect.CodeNotFound, safe)
	case glerrors.KindConflict:
		return connect.NewError(connect.CodeAlreadyExists, safe)
	default:
		return connect.NewError(connect.CodeInternal, glerrors.New(glerrors.KindInternal, "internal error"))
	}
}
