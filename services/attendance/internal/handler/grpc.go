package handler

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	attendancev1 "github.com/garcia-lorca/gen/go/garcialorca/attendance/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/attendance/v1/attendancev1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

// Compile-time proof the handler implements the generated Connect service.
var _ attendancev1connect.AttendanceServiceHandler = (*AttendanceHandler)(nil)

// dateLayout is the ISO form every date leaves the service in: attendance is per
// school day, so a stored date never carries a time-of-day onto the wire.
const dateLayout = "2006-01-02"

// statusServing is what a healthy service reports (the fleet-wide value).
const statusServing = "SERVING"

// AttendanceHandler adapts the attendance service to the generated Connect
// service. It only translates: proto messages ↔ service calls and error Kinds ↔
// Connect codes. Validation and logic live in the service layer.
type AttendanceHandler struct {
	svc *service.AttendanceService
}

// NewAttendanceHandler wires the handler to the attendance service.
func NewAttendanceHandler(svc *service.AttendanceService) *AttendanceHandler {
	return &AttendanceHandler{svc: svc}
}

// Health is the liveness/ping probe present on every service. Attendance is a
// thin service with no readiness dependencies of its own, so liveness is
// answered here; when it grows any, the check moves into the service layer
// without changing this seam.
func (h *AttendanceHandler) Health(_ context.Context, _ *connect.Request[attendancev1.HealthRequest]) (*connect.Response[attendancev1.HealthResponse], error) {
	return connect.NewResponse(&attendancev1.HealthResponse{Status: statusServing}), nil
}

// GetDailyRoster returns one list (the group's daily list, or a subject's
// per-class list) for one day, with its session when it has been taken. Rows
// with no stored mark come back "present" (the default-present model).
func (h *AttendanceHandler) GetDailyRoster(ctx context.Context, req *connect.Request[attendancev1.GetDailyRosterRequest]) (*connect.Response[attendancev1.GetDailyRosterResponse], error) {
	m := req.Msg
	roster, err := h.svc.GetDailyRoster(ctx, m.GetGroupId(), m.GetDate(), m.GetSubjectId())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*attendancev1.RosterRow, 0, len(roster.Rows))
	for _, r := range roster.Rows {
		out = append(out, &attendancev1.RosterRow{
			EnrollmentId: r.EnrollmentID,
			StudentId:    r.StudentID,
			Status:       r.Status,
			Note:         r.Note,
		})
	}
	return connect.NewResponse(&attendancev1.GetDailyRosterResponse{
		Rows:    out,
		Session: toProtoSession(roster.Session),
	}), nil
}

// toProtoSession maps a list's session to the wire; nil (an untaken list) stays
// unset.
func toProtoSession(s *service.Session) *attendancev1.AttendanceSession {
	if s == nil {
		return nil
	}
	return &attendancev1.AttendanceSession{
		TakenAt:     timestamppb.New(s.TakenAt),
		TakenByName: s.TakenByName,
		UpdatedAt:   timestamppb.New(s.UpdatedAt),
	}
}

// SaveDailyAttendance reconciles one list from the complete set of exceptions
// the client submits, stamping the session with the given taker.
func (h *AttendanceHandler) SaveDailyAttendance(ctx context.Context, req *connect.Request[attendancev1.SaveDailyAttendanceRequest]) (*connect.Response[attendancev1.SaveDailyAttendanceResponse], error) {
	m := req.Msg
	marks := make([]service.Mark, 0, len(m.GetMarks()))
	for _, mark := range m.GetMarks() {
		marks = append(marks, service.Mark{
			EnrollmentID: mark.GetEnrollmentId(),
			Status:       mark.GetStatus(),
			Note:         mark.GetNote(),
		})
	}
	absentCount, markedCount, err := h.svc.SaveDailyAttendance(ctx, service.SaveRequest{
		GroupID:     m.GetGroupId(),
		Date:        m.GetDate(),
		SubjectID:   m.GetSubjectId(),
		Marks:       marks,
		TakenBy:     m.GetTakenBy(),
		TakenByName: m.GetTakenByName(),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&attendancev1.SaveDailyAttendanceResponse{
		AbsentCount: int32(absentCount),
		MarkedCount: int32(markedCount),
	}), nil
}

// ListAttendance returns one enrollment's stored marks within an optional date
// range. Only exceptions are stored, so present days never appear.
func (h *AttendanceHandler) ListAttendance(ctx context.Context, req *connect.Request[attendancev1.ListAttendanceRequest]) (*connect.Response[attendancev1.ListAttendanceResponse], error) {
	m := req.Msg
	records, err := h.svc.ListAttendance(ctx, m.GetEnrollmentId(), m.GetFromDate(), m.GetToDate())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*attendancev1.AttendanceRecord, 0, len(records))
	for _, rec := range records {
		out = append(out, &attendancev1.AttendanceRecord{
			Id:           rec.ID,
			EnrollmentId: rec.EnrollmentID,
			Date:         rec.Date.Format(dateLayout),
			Status:       rec.Status,
			Note:         rec.Note,
			SubjectId:    rec.SubjectID,
		})
	}
	return connect.NewResponse(&attendancev1.ListAttendanceResponse{Records: out}), nil
}

// connectErr maps a service error Kind to the Connect status code. Internal
// failures return a generic message (no internal detail leaks to the client);
// the other Kinds carry already-generic messages.
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
