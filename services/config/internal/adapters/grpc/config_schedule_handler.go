package grpcadapter

import (
	"context"
	"errors"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

// ScheduleService is the timetable use case as the edge needs it (T16);
// *usecase.ScheduleService satisfies it.
type ScheduleService interface {
	Board(ctx context.Context, cycleID, teacherID string) (usecase.ScheduleBoard, error)
	Day(ctx context.Context, cycleID string, date time.Time, teacherID string) (usecase.ScheduleDay, error)
	// SchoolDay is the calendar rule alone (T14): does date have classes?
	SchoolDay(ctx context.Context, groupID string, date time.Time) (usecase.SchoolDayStatus, error)
	CreateBlock(ctx context.Context, in usecase.BlockInput, by domain.Actor) (domain.ScheduleBlock, error)
	UpdateBlock(ctx context.Context, id string, in usecase.BlockInput, by domain.Actor) (domain.ScheduleBlock, error)
	DeleteBlock(ctx context.Context, id, reason string, by domain.Actor) error
	CreateOverride(ctx context.Context, in usecase.OverrideInput, by domain.Actor) (domain.ScheduleOverride, error)
	DeleteOverride(ctx context.Context, id, reason string, by domain.Actor) error
	AvailableTeachers(ctx context.Context, q usecase.AvailabilityQuery) ([]domain.AvailableTeacher, error)
	SetGrid(ctx context.Context, levelID string, start, end int) (domain.ScheduleGrid, error)
	Events(ctx context.Context, cycleID string, limit int) ([]domain.ScheduleEvent, error)
	// T19 — teacher absences.
	CreateAbsence(ctx context.Context, in usecase.AbsenceInput, by domain.Actor) (usecase.AbsenceResult, error)
	UpdateAbsence(ctx context.Context, id string, in usecase.AbsenceInput, by domain.Actor) (usecase.AbsenceResult, error)
	CancelAbsence(ctx context.Context, id string, by domain.Actor, callerTeacherID string) (domain.TeacherAbsence, error)
	Absences(ctx context.Context, q usecase.AbsenceQuery) ([]domain.TeacherAbsence, error)
	Uncovered(ctx context.Context, cycleID string, from, to time.Time) ([]usecase.UncoveredClass, error)
}

// errScheduleUnwired answers the timetable RPCs when the composition root did
// not wire the use case (it always does; this only guards a nil dereference).
var errScheduleUnwired = connect.NewError(connect.CodeUnimplemented, errors.New("the timetable is not available"))

// WithSchedule wires the timetable use case (T16).
func (h *ConfigHandler) WithSchedule(s ScheduleService) *ConfigHandler {
	h.schedule = s
	return h
}

// GetScheduleBoard returns a cycle's base timetable and its catalog.
func (h *ConfigHandler) GetScheduleBoard(ctx context.Context, req *connect.Request[configv1.GetScheduleBoardRequest]) (*connect.Response[configv1.GetScheduleBoardResponse], error) {
	if h.schedule == nil {
		return nil, errScheduleUnwired
	}
	b, err := h.schedule.Board(ctx, req.Msg.GetCycleId(), req.Msg.GetTeacherId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(toProtoBoard(b)), nil
}

// GetScheduleDay returns the effective timetable of a date.
func (h *ConfigHandler) GetScheduleDay(ctx context.Context, req *connect.Request[configv1.GetScheduleDayRequest]) (*connect.Response[configv1.GetScheduleDayResponse], error) {
	if h.schedule == nil {
		return nil, errScheduleUnwired
	}
	if req.Msg.GetDate() == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("date is required"))
	}
	d, err := h.schedule.Day(ctx, req.Msg.GetCycleId(), req.Msg.GetDate().AsTime().UTC(), req.Msg.GetTeacherId())
	if err != nil {
		return nil, connectErr(err)
	}
	classes := make([]*configv1.ScheduledClass, 0, len(d.Classes))
	for _, c := range d.Classes {
		classes = append(classes, toProtoClass(c))
	}
	return connect.NewResponse(&configv1.GetScheduleDayResponse{
		Date: timestamppb.New(d.Date), Weekday: int32(d.Weekday), Classes: classes, NonWorking: d.NonWorking, //nolint:gosec // 1..7
	}), nil
}

// GetSchoolDay says whether a group has classes on a date (T14).
func (h *ConfigHandler) GetSchoolDay(ctx context.Context, req *connect.Request[configv1.GetSchoolDayRequest]) (*connect.Response[configv1.GetSchoolDayResponse], error) {
	if h.schedule == nil {
		return nil, errScheduleUnwired
	}
	if req.Msg.GetDate() == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("date is required"))
	}
	d, err := h.schedule.SchoolDay(ctx, req.Msg.GetGroupId(), req.Msg.GetDate().AsTime().UTC())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.GetSchoolDayResponse{Open: d.Open, ClosedLabel: d.Closed}), nil
}

// CreateScheduleBlock adds a class to the base timetable.
func (h *ConfigHandler) CreateScheduleBlock(ctx context.Context, req *connect.Request[configv1.CreateScheduleBlockRequest]) (*connect.Response[configv1.CreateScheduleBlockResponse], error) {
	if h.schedule == nil {
		return nil, errScheduleUnwired
	}
	b, err := h.schedule.CreateBlock(ctx, toBlockInput(req.Msg.GetBlock()), toActor(req.Msg.GetActor()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.CreateScheduleBlockResponse{Block: toProtoBlock(b)}), nil
}

// UpdateScheduleBlock moves or changes a class of the base timetable.
func (h *ConfigHandler) UpdateScheduleBlock(ctx context.Context, req *connect.Request[configv1.UpdateScheduleBlockRequest]) (*connect.Response[configv1.UpdateScheduleBlockResponse], error) {
	if h.schedule == nil {
		return nil, errScheduleUnwired
	}
	b, err := h.schedule.UpdateBlock(ctx, req.Msg.GetId(), toBlockInput(req.Msg.GetBlock()), toActor(req.Msg.GetActor()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.UpdateScheduleBlockResponse{Block: toProtoBlock(b)}), nil
}

// DeleteScheduleBlock removes a class from the base timetable.
func (h *ConfigHandler) DeleteScheduleBlock(ctx context.Context, req *connect.Request[configv1.DeleteScheduleBlockRequest]) (*connect.Response[configv1.DeleteScheduleBlockResponse], error) {
	if h.schedule == nil {
		return nil, errScheduleUnwired
	}
	if err := h.schedule.DeleteBlock(ctx, req.Msg.GetId(), req.Msg.GetReason(), toActor(req.Msg.GetActor())); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.DeleteScheduleBlockResponse{}), nil
}

// CreateScheduleOverride changes one class on one date.
func (h *ConfigHandler) CreateScheduleOverride(ctx context.Context, req *connect.Request[configv1.CreateScheduleOverrideRequest]) (*connect.Response[configv1.CreateScheduleOverrideResponse], error) {
	if h.schedule == nil {
		return nil, errScheduleUnwired
	}
	m := req.Msg
	if m.GetDate() == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("date is required"))
	}
	o, err := h.schedule.CreateOverride(ctx, usecase.OverrideInput{
		BlockID: m.GetBlockId(), Date: m.GetDate().AsTime().UTC(), Kind: domain.OverrideKind(m.GetKind()),
		TeacherID: m.GetTeacherId(), SwapBlockID: m.GetSwapBlockId(), Reason: m.GetReason(), ReplaceOverrideID: m.GetReplaceOverrideId(),
	}, toActor(m.GetActor()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.CreateScheduleOverrideResponse{Override: toProtoOverride(o)}), nil
}

// DeleteScheduleOverride undoes a date-specific change.
func (h *ConfigHandler) DeleteScheduleOverride(ctx context.Context, req *connect.Request[configv1.DeleteScheduleOverrideRequest]) (*connect.Response[configv1.DeleteScheduleOverrideResponse], error) {
	if h.schedule == nil {
		return nil, errScheduleUnwired
	}
	if err := h.schedule.DeleteOverride(ctx, req.Msg.GetId(), req.Msg.GetReason(), toActor(req.Msg.GetActor())); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.DeleteScheduleOverrideResponse{}), nil
}

// ListAvailableTeachers lists the teachers free in a slot.
func (h *ConfigHandler) ListAvailableTeachers(ctx context.Context, req *connect.Request[configv1.ListAvailableTeachersRequest]) (*connect.Response[configv1.ListAvailableTeachersResponse], error) {
	if h.schedule == nil {
		return nil, errScheduleUnwired
	}
	m := req.Msg
	q := usecase.AvailabilityQuery{
		CycleID: m.GetCycleId(), GroupID: m.GetGroupId(), SubjectID: m.GetSubjectId(), Weekday: int(m.GetWeekday()),
		Start: int(m.GetStartsAt()), End: int(m.GetEndsAt()), ExcludeBlockID: m.GetExcludeBlockId(),
	}
	if m.GetDate() != nil {
		q.Date = m.GetDate().AsTime().UTC()
	}
	free, err := h.schedule.AvailableTeachers(ctx, q)
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*configv1.AvailableTeacher, 0, len(free))
	for _, a := range free {
		out = append(out, &configv1.AvailableTeacher{Teacher: toProtoTeacher(a.Teacher), Match: string(a.Match)})
	}
	return connect.NewResponse(&configv1.ListAvailableTeachersResponse{Teachers: out}), nil
}

// SetScheduleGrid saves a level's grid.
func (h *ConfigHandler) SetScheduleGrid(ctx context.Context, req *connect.Request[configv1.SetScheduleGridRequest]) (*connect.Response[configv1.SetScheduleGridResponse], error) {
	if h.schedule == nil {
		return nil, errScheduleUnwired
	}
	g, err := h.schedule.SetGrid(ctx, req.Msg.GetLevelId(), int(req.Msg.GetGridStart()), int(req.Msg.GetGridEnd()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.SetScheduleGridResponse{Grid: toProtoGrid(g)}), nil
}

// ListScheduleEvents returns the timetable's history.
func (h *ConfigHandler) ListScheduleEvents(ctx context.Context, req *connect.Request[configv1.ListScheduleEventsRequest]) (*connect.Response[configv1.ListScheduleEventsResponse], error) {
	if h.schedule == nil {
		return nil, errScheduleUnwired
	}
	evs, err := h.schedule.Events(ctx, req.Msg.GetCycleId(), int(req.Msg.GetLimit()))
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*configv1.ScheduleEvent, 0, len(evs))
	for _, e := range evs {
		pe := &configv1.ScheduleEvent{
			Id: e.ID, CycleId: e.CycleID, At: timestamppb.New(e.At), By: toProtoActor(e.By), Action: string(e.Action),
			BlockId: e.BlockID, OverrideId: e.OverrideID, AbsenceId: e.AbsenceID, Reason: e.Reason, Summary: e.Summary,
		}
		if !e.Date.IsZero() {
			pe.Date = timestamppb.New(e.Date)
		}
		out = append(out, pe)
	}
	return connect.NewResponse(&configv1.ListScheduleEventsResponse{Events: out}), nil
}

// --- mapping -------------------------------------------------------------------

func toBlockInput(in *configv1.ScheduleBlockInput) usecase.BlockInput {
	return usecase.BlockInput{
		AssignmentID: in.GetAssignmentId(), Weekday: int(in.GetWeekday()),
		Start: int(in.GetStartsAt()), End: int(in.GetEndsAt()), Reason: in.GetReason(),
	}
}

func toActor(a *configv1.ScheduleActor) domain.Actor {
	return domain.Actor{UserID: a.GetUserId(), Name: a.GetName()}
}

func toProtoActor(a domain.Actor) *configv1.ScheduleActor {
	return &configv1.ScheduleActor{UserId: a.UserID, Name: a.Name}
}

func toProtoGrid(g domain.ScheduleGrid) *configv1.ScheduleGrid {
	return &configv1.ScheduleGrid{LevelId: g.LevelID(), GridStart: int32(g.Start()), GridEnd: int32(g.End())} //nolint:gosec // minutes of a day
}

func toProtoBlock(b domain.ScheduleBlock) *configv1.ScheduleBlock {
	return &configv1.ScheduleBlock{
		Id: b.ID(), CycleId: b.CycleID(), AssignmentId: b.AssignmentID(), GroupId: b.GroupID(),
		SubjectId: b.SubjectID(), TeacherId: b.TeacherID(),
		Weekday: int32(b.Weekday()), StartsAt: int32(b.Time().Start()), EndsAt: int32(b.Time().End()), //nolint:gosec // bounded by the domain
	}
}

func toProtoOverride(o domain.ScheduleOverride) *configv1.ScheduleOverride {
	return &configv1.ScheduleOverride{
		Id: o.ID(), CycleId: o.CycleID(), BlockId: o.BlockID(), Date: timestamppb.New(o.Date()), Kind: string(o.Kind()),
		TeacherId: o.TeacherID(), SwapBlockId: o.SwapBlockID(), Reason: o.Reason(),
		CreatedBy: toProtoActor(o.By()), CreatedAt: timestamppb.New(o.CreatedAt()),
	}
}

func toProtoClass(c domain.ScheduledClass) *configv1.ScheduledClass {
	pc := &configv1.ScheduledClass{
		Block: toProtoBlock(c.Block), TeacherId: c.TeacherID, Status: string(c.Status),
		StartsAt: int32(c.Time.Start()), EndsAt: int32(c.Time.End()), //nolint:gosec // minutes of a day
	}
	if c.Override != nil {
		pc.Override = toProtoOverride(*c.Override)
	}
	if c.Absence != nil {
		pc.Absence = toProtoAbsence(*c.Absence)
	}
	return pc
}

func toProtoBoard(b usecase.ScheduleBoard) *configv1.GetScheduleBoardResponse {
	out := &configv1.GetScheduleBoardResponse{Cycle: toProtoCycle(b.Catalog.Cycle), Started: b.Started}
	for _, c := range b.Cycles {
		out.Cycles = append(out.Cycles, toProtoCycle(c))
	}
	for _, g := range b.Grids {
		out.Grids = append(out.Grids, toProtoGrid(g))
	}
	for _, l := range b.Catalog.Levels {
		out.Levels = append(out.Levels, toProtoLevel(l))
	}
	for _, g := range b.Catalog.Grades {
		out.GradeLevels = append(out.GradeLevels, toProtoGradeLevel(g))
	}
	for _, g := range b.Catalog.Groups {
		out.Groups = append(out.Groups, toProtoGroup(g))
	}
	for _, s := range b.Catalog.Subjects {
		out.Subjects = append(out.Subjects, toProtoSubject(s))
	}
	for _, t := range b.Catalog.Teachers {
		out.Teachers = append(out.Teachers, toProtoTeacher(t))
	}
	for _, a := range b.Catalog.Assignments {
		out.Assignments = append(out.Assignments, toProtoTeachingAssignment(a))
	}
	for _, bl := range b.Blocks {
		out.Blocks = append(out.Blocks, toProtoBlock(bl))
	}
	return out
}
