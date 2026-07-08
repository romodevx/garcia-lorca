package grpcadapter

import (
	"context"

	connect "connectrpc.com/connect"

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
	svc    *usecase.StudentService
	health *usecase.HealthService
}

// NewStudentsHandler wires the handler to the student + health use cases.
func NewStudentsHandler(svc *usecase.StudentService, health *usecase.HealthService) *StudentsHandler {
	return &StudentsHandler{svc: svc, health: health}
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

// connectErr maps a domain error Kind to the Connect status code. Internal
// failures return a generic message (no internal detail leaks); the other Kinds
// carry already-generic domain messages.
func connectErr(err error) *connect.Error {
	switch glerrors.KindOf(err) {
	case glerrors.KindInvalid:
		return connect.NewError(connect.CodeInvalidArgument, err)
	case glerrors.KindUnauthorized:
		return connect.NewError(connect.CodeUnauthenticated, err)
	case glerrors.KindForbidden:
		return connect.NewError(connect.CodePermissionDenied, err)
	case glerrors.KindNotFound:
		return connect.NewError(connect.CodeNotFound, err)
	case glerrors.KindConflict:
		return connect.NewError(connect.CodeAlreadyExists, err)
	default:
		return connect.NewError(connect.CodeInternal, glerrors.New(glerrors.KindInternal, "internal error"))
	}
}
