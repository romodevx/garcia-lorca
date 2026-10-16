// Package grpcadapter adapts the grades use cases to the generated
// garcialorca.grades.v1 Connect service. It only translates: proto messages ↔
// use-case calls, domain entities ↔ proto messages, and domain error Kinds ↔
// Connect codes. All logic lives in usecase/domain — GORM and transport details
// never leak inward.
package grpcadapter

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"

	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/grades/v1/gradesv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/usecase"
)

// Compile-time proof the adapter implements the generated Connect handler.
var _ gradesv1connect.GradesServiceHandler = (*GradesHandler)(nil)

// GradesHandler adapts the grades use cases to the generated Connect service.
//
// It embeds the generated Unimplemented handler for the monthly RPCs retired
// in T15 (UpsertMonthlyGrade, ClearMonthlyGrade, GetMonthlySheet,
// GetEnrollmentMonthlyGrades — deprecated in the contract): they answer
// Unimplemented, so nothing can write or depend on monthly_grades any more.
// Every other RPC is implemented explicitly below.
type GradesHandler struct {
	gradesv1connect.UnimplementedGradesServiceHandler

	grades       *usecase.GradesService
	periodGrades *usecase.PeriodGradesService
	periods      PeriodsUseCase
	health       *usecase.HealthService
	levels       *usecase.LevelArchiveService
	groups       *usecase.GroupUsageService
}

// NewGradesHandler wires the handler to the term grades, period grades (T15),
// period closing (T7) and health use cases.
func NewGradesHandler(grades *usecase.GradesService, periodGrades *usecase.PeriodGradesService, periods PeriodsUseCase, health *usecase.HealthService) *GradesHandler {
	return &GradesHandler{grades: grades, periodGrades: periodGrades, periods: periods, health: health}
}

// Health is the liveness/ping probe present on every service.
func (h *GradesHandler) Health(ctx context.Context, _ *connect.Request[gradesv1.HealthRequest]) (*connect.Response[gradesv1.HealthResponse], error) {
	status, err := h.health.Ping(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&gradesv1.HealthResponse{Status: status.Status}), nil
}

// CaptureGrade runs the capture knot. subject_id and teacher_id are DERIVED from
// the assignment by the use case; the request carries only assignment/enrollment/
// term/value.
func (h *GradesHandler) CaptureGrade(ctx context.Context, req *connect.Request[gradesv1.CaptureGradeRequest]) (*connect.Response[gradesv1.CaptureGradeResponse], error) {
	m := req.Msg
	entry, err := h.grades.CaptureGrade(ctx, m.GetAssignmentId(), m.GetEnrollmentId(), m.GetTermId(), m.GetValue())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&gradesv1.CaptureGradeResponse{Entry: toProtoGradeEntry(entry)}), nil
}

// GetRoster returns one row per active enrollment in the assignment's group,
// joined with the current grade value for the term.
func (h *GradesHandler) GetRoster(ctx context.Context, req *connect.Request[gradesv1.GetRosterRequest]) (*connect.Response[gradesv1.GetRosterResponse], error) {
	m := req.Msg
	rows, err := h.grades.Roster(ctx, m.GetAssignmentId(), m.GetTermId())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*gradesv1.RosterRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, &gradesv1.RosterRow{
			EnrollmentId: r.EnrollmentID,
			StudentId:    r.StudentID,
			Value:        r.Value,
			HasValue:     r.HasValue,
		})
	}
	return connect.NewResponse(&gradesv1.GetRosterResponse{Rows: out}), nil
}

// ListGradeEntries returns the entries for an enrollment in a term.
func (h *GradesHandler) ListGradeEntries(ctx context.Context, req *connect.Request[gradesv1.ListGradeEntriesRequest]) (*connect.Response[gradesv1.ListGradeEntriesResponse], error) {
	m := req.Msg
	entries, err := h.grades.ListGradeEntries(ctx, m.GetEnrollmentId(), m.GetTermId())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*gradesv1.GradeEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, toProtoGradeEntry(e))
	}
	return connect.NewResponse(&gradesv1.ListGradeEntriesResponse{Entries: out}), nil
}

// toProtoGradeEntry maps a domain GradeEntry to its proto message. published_at
// is deliberately left nil: publication is a later phase and capture never sets
// it (the domain's capture instant is not the boleta publish time).
func toProtoGradeEntry(e domain.GradeEntry) *gradesv1.GradeEntry {
	return &gradesv1.GradeEntry{
		Id:           e.ID(),
		EnrollmentId: e.EnrollmentID(),
		SubjectId:    e.SubjectID(),
		TermId:       e.TermID(),
		TeacherId:    e.TeacherID(),
		Value:        e.Value(),
		// PublishedAt intentionally unset (nil).
	}
}

// connectErr maps a domain error Kind to the Connect status code. Internal
// failures return a generic message (no internal detail leaks to the client);
// the other Kinds carry already-generic domain messages.
// Every code goes through glerrors.SafeMessage, not just the internal one. The
// kind decides the STATUS; it does not decide whether the wrapped cause is the
// caller's business, and it never is. Error() concatenates the cause, so handing
// the error straight to connect.NewError shipped Postgres constraint names and
// SQLSTATEs — and on some constraints the conflicting value — to whoever called.
func connectErr(err error) *connect.Error {
	ce := connectErrForKind(err)
	if code := domain.RefusalCodeOf(err); code != "" {
		ce.Meta().Set(ErrorCodeMeta, string(code))
	} else if code := levelarchive.CodeOf(err); code != "" {
		ce.Meta().Set(ErrorCodeMeta, code)
	}
	return ce
}

// ErrorCodeMeta is the error-metadata key a refusal's machine code rides in; the
// gateway forwards it as the X-Error-Code response header.
const ErrorCodeMeta = "X-Error-Code"

func connectErrForKind(err error) *connect.Error {
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
