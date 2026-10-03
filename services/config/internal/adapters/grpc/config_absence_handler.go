package grpcadapter

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

// Teacher absences (T19, ADR 0032). config hands the whole absence (kind and
// note included); the gateway shapes them away for callers without
// schedule:write.

// CreateTeacherAbsence registers an absence.
func (h *ConfigHandler) CreateTeacherAbsence(ctx context.Context, req *connect.Request[configv1.CreateTeacherAbsenceRequest]) (*connect.Response[configv1.CreateTeacherAbsenceResponse], error) {
	if h.schedule == nil {
		return nil, errScheduleUnwired
	}
	in, err := toAbsenceInput(req.Msg.GetAbsence())
	if err != nil {
		return nil, err
	}
	res, err := h.schedule.CreateAbsence(ctx, in, toActor(req.Msg.GetActor()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.CreateTeacherAbsenceResponse{
		Absence: toProtoAbsence(res.Absence), UncoveredCount: int32(res.Uncovered), //nolint:gosec // classes of at most a year
	}), nil
}

// UpdateTeacherAbsence changes an active absence.
func (h *ConfigHandler) UpdateTeacherAbsence(ctx context.Context, req *connect.Request[configv1.UpdateTeacherAbsenceRequest]) (*connect.Response[configv1.UpdateTeacherAbsenceResponse], error) {
	if h.schedule == nil {
		return nil, errScheduleUnwired
	}
	in, err := toAbsenceInput(req.Msg.GetAbsence())
	if err != nil {
		return nil, err
	}
	res, err := h.schedule.UpdateAbsence(ctx, req.Msg.GetId(), in, toActor(req.Msg.GetActor()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.UpdateTeacherAbsenceResponse{
		Absence: toProtoAbsence(res.Absence), UncoveredCount: int32(res.Uncovered), //nolint:gosec // classes of at most a year
	}), nil
}

// CancelTeacherAbsence cancels an absence, keeping the record.
func (h *ConfigHandler) CancelTeacherAbsence(ctx context.Context, req *connect.Request[configv1.CancelTeacherAbsenceRequest]) (*connect.Response[configv1.CancelTeacherAbsenceResponse], error) {
	if h.schedule == nil {
		return nil, errScheduleUnwired
	}
	a, err := h.schedule.CancelAbsence(ctx, req.Msg.GetId(), toActor(req.Msg.GetActor()), req.Msg.GetCallerTeacherId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.CancelTeacherAbsenceResponse{Absence: toProtoAbsence(a)}), nil
}

// ListTeacherAbsences lists a cycle's absences touching from..to.
func (h *ConfigHandler) ListTeacherAbsences(ctx context.Context, req *connect.Request[configv1.ListTeacherAbsencesRequest]) (*connect.Response[configv1.ListTeacherAbsencesResponse], error) {
	if h.schedule == nil {
		return nil, errScheduleUnwired
	}
	m := req.Msg
	if m.GetFrom() == nil || m.GetTo() == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("from and to are required"))
	}
	list, err := h.schedule.Absences(ctx, usecase.AbsenceQuery{
		CycleID: m.GetCycleId(), TeacherID: m.GetTeacherId(), From: m.GetFrom().AsTime().UTC(), To: m.GetTo().AsTime().UTC(),
		IncludeCancelled: m.GetIncludeCancelled(),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*configv1.TeacherAbsence, 0, len(list))
	for _, a := range list {
		out = append(out, toProtoAbsence(a))
	}
	return connect.NewResponse(&configv1.ListTeacherAbsencesResponse{Absences: out}), nil
}

// ListUncoveredClasses lists the classes nobody gives, date by date.
func (h *ConfigHandler) ListUncoveredClasses(ctx context.Context, req *connect.Request[configv1.ListUncoveredClassesRequest]) (*connect.Response[configv1.ListUncoveredClassesResponse], error) {
	if h.schedule == nil {
		return nil, errScheduleUnwired
	}
	m := req.Msg
	if m.GetFrom() == nil || m.GetTo() == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("from and to are required"))
	}
	list, err := h.schedule.Uncovered(ctx, m.GetCycleId(), m.GetFrom().AsTime().UTC(), m.GetTo().AsTime().UTC())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*configv1.UncoveredClass, 0, len(list))
	for _, u := range list {
		out = append(out, &configv1.UncoveredClass{Date: timestamppb.New(u.Date), ScheduledClass: toProtoClass(u.Class)})
	}
	return connect.NewResponse(&configv1.ListUncoveredClassesResponse{Classes: out}), nil
}

func toAbsenceInput(in *configv1.TeacherAbsenceInput) (usecase.AbsenceInput, error) {
	if in.GetStartsOn() == nil || in.GetEndsOn() == nil {
		return usecase.AbsenceInput{}, connect.NewError(connect.CodeInvalidArgument, errors.New("starts_on and ends_on are required"))
	}
	out := usecase.AbsenceInput{
		CycleID: in.GetCycleId(), TeacherID: in.GetTeacherId(), Kind: domain.AbsenceKind(in.GetKind()),
		StartsOn: in.GetStartsOn().AsTime().UTC(), EndsOn: in.GetEndsOn().AsTime().UTC(), AllDay: in.GetAllDay(), Note: in.GetNote(),
	}
	if !out.AllDay {
		out.Start, out.End = int(in.GetStartMinute()), int(in.GetEndMinute())
	}
	return out, nil
}

func toProtoAbsence(a domain.TeacherAbsence) *configv1.TeacherAbsence {
	out := &configv1.TeacherAbsence{
		Id: a.ID(), CycleId: a.CycleID(), TeacherId: a.TeacherID(), Kind: string(a.Kind()),
		StartsOn: timestamppb.New(a.StartsOn()), EndsOn: timestamppb.New(a.EndsOn()), AllDay: a.FullDay(), Note: a.Note(),
		CreatedBy: toProtoActor(a.CreatedBy()), CreatedAt: timestamppb.New(a.CreatedAt()), UpdatedAt: timestamppb.New(a.UpdatedAt()),
	}
	if !a.FullDay() {
		out.StartMinute, out.EndMinute = int32(a.Span().Start()), int32(a.Span().End()) //nolint:gosec // minutes of a day
	}
	if !a.Active() {
		out.CancelledAt, out.CancelledBy = timestamppb.New(a.CancelledAt()), toProtoActor(a.CancelledBy())
	}
	return out
}
