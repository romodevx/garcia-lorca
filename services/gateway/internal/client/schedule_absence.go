package client

import (
	"context"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
)

// Teacher absences (T19) as the edge sees them. Minutes are since midnight;
// AllDay leaves them zero. Kind and Note arrive whole from config — the
// handler decides who may read them.

// TeacherAbsence is a teacher away on StartsOn..EndsOn.
type TeacherAbsence struct {
	ID          string
	CycleID     string
	TeacherID   string
	Kind        string
	StartsOn    time.Time
	EndsOn      time.Time
	AllDay      bool
	StartMinute int
	EndMinute   int
	Note        string
	CreatedBy   ScheduleActor
	CreatedAt   time.Time
	CancelledAt time.Time // zero while active
	CancelledBy ScheduleActor
}

// TeacherAbsenceInput registers or edits an absence ("" CycleID: the cycle of StartsOn).
type TeacherAbsenceInput struct {
	CycleID     string
	TeacherID   string
	Kind        string
	StartsOn    time.Time
	EndsOn      time.Time
	AllDay      bool
	StartMinute int
	EndMinute   int
	Note        string
}

// TeacherAbsenceSaved is a saved absence and the classes it leaves uncovered.
type TeacherAbsenceSaved struct {
	Absence   TeacherAbsence
	Uncovered int
}

// TeacherAbsenceQuery lists a cycle's absences touching From..To.
type TeacherAbsenceQuery struct {
	CycleID          string
	TeacherID        string
	From             time.Time
	To               time.Time
	IncludeCancelled bool
}

// UncoveredClass is a class nobody gives on Date.
type UncoveredClass struct {
	Date  time.Time
	Class ScheduledClass
}

// CreateTeacherAbsence registers an absence.
func (c *ConfigClient) CreateTeacherAbsence(ctx context.Context, in TeacherAbsenceInput, by ScheduleActor) (TeacherAbsenceSaved, error) {
	res, err := c.rpc.CreateTeacherAbsence(ctx, connect.NewRequest(&configv1.CreateTeacherAbsenceRequest{
		Absence: toProtoAbsenceInput(in), Actor: toProtoScheduleActor(by),
	}))
	if err != nil {
		return TeacherAbsenceSaved{}, err
	}
	return TeacherAbsenceSaved{Absence: toTeacherAbsence(res.Msg.GetAbsence()), Uncovered: int(res.Msg.GetUncoveredCount())}, nil
}

// UpdateTeacherAbsence changes an active absence.
func (c *ConfigClient) UpdateTeacherAbsence(ctx context.Context, id string, in TeacherAbsenceInput, by ScheduleActor) (TeacherAbsenceSaved, error) {
	res, err := c.rpc.UpdateTeacherAbsence(ctx, connect.NewRequest(&configv1.UpdateTeacherAbsenceRequest{
		Id: id, Absence: toProtoAbsenceInput(in), Actor: toProtoScheduleActor(by),
	}))
	if err != nil {
		return TeacherAbsenceSaved{}, err
	}
	return TeacherAbsenceSaved{Absence: toTeacherAbsence(res.Msg.GetAbsence()), Uncovered: int(res.Msg.GetUncoveredCount())}, nil
}

// CancelTeacherAbsence cancels an absence (never deleted).
func (c *ConfigClient) CancelTeacherAbsence(ctx context.Context, id string, by ScheduleActor) (TeacherAbsence, error) {
	res, err := c.rpc.CancelTeacherAbsence(ctx, connect.NewRequest(&configv1.CancelTeacherAbsenceRequest{Id: id, Actor: toProtoScheduleActor(by)}))
	if err != nil {
		return TeacherAbsence{}, err
	}
	return toTeacherAbsence(res.Msg.GetAbsence()), nil
}

// ListTeacherAbsences lists a cycle's absences touching q.From..q.To.
func (c *ConfigClient) ListTeacherAbsences(ctx context.Context, q TeacherAbsenceQuery) ([]TeacherAbsence, error) {
	res, err := c.rpc.ListTeacherAbsences(ctx, connect.NewRequest(&configv1.ListTeacherAbsencesRequest{
		CycleId: q.CycleID, TeacherId: q.TeacherID, From: timestamppb.New(q.From), To: timestamppb.New(q.To), IncludeCancelled: q.IncludeCancelled,
	}))
	if err != nil {
		return nil, err
	}
	out := make([]TeacherAbsence, 0, len(res.Msg.GetAbsences()))
	for _, a := range res.Msg.GetAbsences() {
		out = append(out, toTeacherAbsence(a))
	}
	return out, nil
}

// ListUncoveredClasses lists the classes nobody gives, date by date.
func (c *ConfigClient) ListUncoveredClasses(ctx context.Context, cycleID string, from, to time.Time) ([]UncoveredClass, error) {
	res, err := c.rpc.ListUncoveredClasses(ctx, connect.NewRequest(&configv1.ListUncoveredClassesRequest{
		CycleId: cycleID, From: timestamppb.New(from), To: timestamppb.New(to),
	}))
	if err != nil {
		return nil, err
	}
	out := make([]UncoveredClass, 0, len(res.Msg.GetClasses()))
	for _, u := range res.Msg.GetClasses() {
		out = append(out, UncoveredClass{Date: u.GetDate().AsTime().UTC(), Class: toScheduledClass(u.GetScheduledClass())})
	}
	return out, nil
}

func toProtoAbsenceInput(in TeacherAbsenceInput) *configv1.TeacherAbsenceInput {
	return &configv1.TeacherAbsenceInput{
		CycleId: in.CycleID, TeacherId: in.TeacherID, Kind: in.Kind, StartsOn: timestamppb.New(in.StartsOn), EndsOn: timestamppb.New(in.EndsOn),
		AllDay: in.AllDay, StartMinute: int32(in.StartMinute), EndMinute: int32(in.EndMinute), Note: in.Note, //nolint:gosec // checked at the edge
	}
}

func toTeacherAbsence(a *configv1.TeacherAbsence) TeacherAbsence {
	out := TeacherAbsence{
		ID: a.GetId(), CycleID: a.GetCycleId(), TeacherID: a.GetTeacherId(), Kind: a.GetKind(),
		StartsOn: a.GetStartsOn().AsTime().UTC(), EndsOn: a.GetEndsOn().AsTime().UTC(), AllDay: a.GetAllDay(),
		StartMinute: int(a.GetStartMinute()), EndMinute: int(a.GetEndMinute()), Note: a.GetNote(),
		CreatedBy: ScheduleActor{UserID: a.GetCreatedBy().GetUserId(), Name: a.GetCreatedBy().GetName()},
		CreatedAt: a.GetCreatedAt().AsTime().UTC(),
	}
	if a.GetCancelledAt() != nil {
		out.CancelledAt = a.GetCancelledAt().AsTime().UTC()
		out.CancelledBy = ScheduleActor{UserID: a.GetCancelledBy().GetUserId(), Name: a.GetCancelledBy().GetName()}
	}
	return out
}
