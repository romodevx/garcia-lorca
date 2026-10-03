package grpcadapter_test

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

var absMon = time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)

func stubAbsence(in usecase.AbsenceInput, by domain.Actor) domain.TeacherAbsence {
	var span *domain.ClassTime
	if !in.AllDay {
		ct, _ := domain.NewClassTime(in.Start, in.End)
		span = &ct
	}
	a, _ := domain.NewTeacherAbsence(domain.AbsenceFields{ID: "abs1", TenantID: "t1", CycleID: "c1", TeacherID: in.TeacherID,
		Kind: in.Kind, StartsOn: in.StartsOn, EndsOn: in.EndsOn, Span: span, Note: in.Note}, by, absMon)
	return a
}

func (s *stubSchedule) CreateAbsence(_ context.Context, in usecase.AbsenceInput, by domain.Actor) (usecase.AbsenceResult, error) {
	s.gotAbsence, s.gotActor = in, by
	return usecase.AbsenceResult{Absence: stubAbsence(in, by), Uncovered: 3}, s.err
}

func (s *stubSchedule) UpdateAbsence(_ context.Context, id string, in usecase.AbsenceInput, by domain.Actor) (usecase.AbsenceResult, error) {
	s.gotAbsence, s.gotActor, s.gotID = in, by, id
	return usecase.AbsenceResult{Absence: stubAbsence(in, by), Uncovered: 1}, s.err
}

func (s *stubSchedule) CancelAbsence(_ context.Context, id string, by domain.Actor, callerTeacherID string) (domain.TeacherAbsence, error) {
	s.gotID, s.gotActor, s.gotCaller = id, by, callerTeacherID
	a := stubAbsence(usecase.AbsenceInput{TeacherID: "t-ana", Kind: domain.AbsenceSick, StartsOn: absMon, EndsOn: absMon, AllDay: true}, by)
	c, _ := a.Cancel(by, absMon)
	return c, s.err
}

func (s *stubSchedule) Absences(_ context.Context, q usecase.AbsenceQuery) ([]domain.TeacherAbsence, error) {
	s.gotAbsenceQuery = q
	return []domain.TeacherAbsence{stubAbsence(usecase.AbsenceInput{TeacherID: "t-ana", Kind: domain.AbsenceVacation, StartsOn: absMon, EndsOn: absMon, Start: 480, End: 600}, domain.Actor{})}, s.err
}

func (s *stubSchedule) Uncovered(_ context.Context, _ string, from, to time.Time) ([]usecase.UncoveredClass, error) {
	s.gotFrom, s.gotTo = from, to
	c := domain.RegularClass(s.block)
	a := stubAbsence(usecase.AbsenceInput{TeacherID: "t-ana", Kind: domain.AbsenceSick, StartsOn: absMon, EndsOn: absMon, AllDay: true, Note: "gripe"}, domain.Actor{})
	c.Status, c.Absence = domain.ClassUncovered, &a
	return []usecase.UncoveredClass{{Date: absMon, Class: c}}, s.err
}

func TestTeacherAbsenceRPCs(t *testing.T) {
	h, stub := newScheduleHandler(t)
	actor := &configv1.ScheduleActor{UserId: "u-dir", Name: "Directora"}

	created, err := h.CreateTeacherAbsence(ctx(), connect.NewRequest(&configv1.CreateTeacherAbsenceRequest{
		Absence: &configv1.TeacherAbsenceInput{CycleId: "c1", TeacherId: "t-ana", Kind: "sick", StartsOn: timestamppb.New(absMon),
			EndsOn: timestamppb.New(absMon.AddDate(0, 0, 2)), StartMinute: 480, EndMinute: 600, Note: "cita"},
		Actor: actor,
	}))
	require.NoError(t, err)
	assert.Equal(t, usecase.AbsenceInput{CycleID: "c1", TeacherID: "t-ana", Kind: domain.AbsenceSick, StartsOn: absMon,
		EndsOn: absMon.AddDate(0, 0, 2), Start: 480, End: 600, Note: "cita"}, stub.gotAbsence)
	assert.Equal(t, "Directora", stub.gotActor.Name)
	assert.Equal(t, int32(3), created.Msg.GetUncoveredCount())
	got := created.Msg.GetAbsence()
	assert.False(t, got.GetAllDay())
	assert.Equal(t, int32(480), got.GetStartMinute())
	assert.Equal(t, "cita", got.GetNote())
	assert.Equal(t, "Directora", got.GetCreatedBy().GetName())
	assert.Nil(t, got.GetCancelledAt(), "active")

	_, err = h.CreateTeacherAbsence(ctx(), connect.NewRequest(&configv1.CreateTeacherAbsenceRequest{Absence: &configv1.TeacherAbsenceInput{TeacherId: "t-ana"}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "dates are required")

	upd, err := h.UpdateTeacherAbsence(ctx(), connect.NewRequest(&configv1.UpdateTeacherAbsenceRequest{
		Id: "abs1", Absence: &configv1.TeacherAbsenceInput{TeacherId: "t-ana", Kind: "other", StartsOn: timestamppb.New(absMon), EndsOn: timestamppb.New(absMon), AllDay: true},
		Actor: actor,
	}))
	require.NoError(t, err)
	assert.Equal(t, "abs1", stub.gotID)
	assert.True(t, stub.gotAbsence.AllDay)
	assert.True(t, upd.Msg.GetAbsence().GetAllDay())
	assert.Equal(t, int32(1), upd.Msg.GetUncoveredCount())

	cancelled, err := h.CancelTeacherAbsence(ctx(), connect.NewRequest(&configv1.CancelTeacherAbsenceRequest{Id: "abs1", Actor: actor, CallerTeacherId: "t-zoe"}))
	require.NoError(t, err)
	assert.Equal(t, "t-zoe", stub.gotCaller)
	assert.NotNil(t, cancelled.Msg.GetAbsence().GetCancelledAt())
	assert.Equal(t, "Directora", cancelled.Msg.GetAbsence().GetCancelledBy().GetName())

	list, err := h.ListTeacherAbsences(ctx(), connect.NewRequest(&configv1.ListTeacherAbsencesRequest{
		CycleId: "c1", TeacherId: "t-ana", From: timestamppb.New(absMon), To: timestamppb.New(absMon), IncludeCancelled: true,
	}))
	require.NoError(t, err)
	assert.Equal(t, usecase.AbsenceQuery{CycleID: "c1", TeacherID: "t-ana", From: absMon, To: absMon, IncludeCancelled: true}, stub.gotAbsenceQuery)
	require.Len(t, list.Msg.GetAbsences(), 1)
	assert.Equal(t, "vacation", list.Msg.GetAbsences()[0].GetKind())

	unc, err := h.ListUncoveredClasses(ctx(), connect.NewRequest(&configv1.ListUncoveredClassesRequest{CycleId: "c1", From: timestamppb.New(absMon), To: timestamppb.New(absMon)}))
	require.NoError(t, err)
	require.Len(t, unc.Msg.GetClasses(), 1)
	c := unc.Msg.GetClasses()[0]
	assert.Equal(t, absMon, c.GetDate().AsTime())
	assert.Equal(t, "uncovered", c.GetScheduledClass().GetStatus())
	assert.Equal(t, "gripe", c.GetScheduledClass().GetAbsence().GetNote(), "config hands everything; the gateway shapes")

	_, err = h.ListUncoveredClasses(ctx(), connect.NewRequest(&configv1.ListUncoveredClassesRequest{CycleId: "c1"}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestScheduleDayCarriesTheNonWorkingLabelAndAbsenceEvents(t *testing.T) {
	h, stub := newScheduleHandler(t)
	stub.nonWorking = "Independencia"
	day, err := h.GetScheduleDay(ctx(), connect.NewRequest(&configv1.GetScheduleDayRequest{CycleId: "c1", Date: timestamppb.New(absMon)}))
	require.NoError(t, err)
	assert.Equal(t, "Independencia", day.Msg.GetNonWorking())

	stub.event = domain.ScheduleEvent{ID: "e2", Action: domain.ActionAbsenceCreated, AbsenceID: "abs1", Date: absMon}
	ev, err := h.ListScheduleEvents(ctx(), connect.NewRequest(&configv1.ListScheduleEventsRequest{CycleId: "c1"}))
	require.NoError(t, err)
	assert.Equal(t, "abs1", ev.Msg.GetEvents()[0].GetAbsenceId())
}
