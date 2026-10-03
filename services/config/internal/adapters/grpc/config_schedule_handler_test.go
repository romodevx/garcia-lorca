package grpcadapter_test

import (
	"context"
	"errors"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	grpcadapter "github.com/garcia-lorca/services/config/internal/adapters/grpc"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

// stubSchedule records what the edge hands the use case and answers canned
// values.
type stubSchedule struct {
	gotBlock    usecase.BlockInput
	gotOverride usecase.OverrideInput
	gotActor    domain.Actor
	gotQuery    usecase.AvailabilityQuery
	gotTeacher  string
	err         error
	block       domain.ScheduleBlock
	// T19
	gotAbsence      usecase.AbsenceInput
	gotAbsenceQuery usecase.AbsenceQuery
	gotID           string
	gotFrom, gotTo  time.Time
	nonWorking      string
	event           domain.ScheduleEvent
}

func (s *stubSchedule) Board(_ context.Context, _, teacherID string) (usecase.ScheduleBoard, error) {
	s.gotTeacher = teacherID
	cycle, _ := domain.NewCycle("c1", "t1", "2026-2027", domain.CycleStatusActive,
		time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC), time.Date(2027, 7, 9, 0, 0, 0, 0, time.UTC))
	return usecase.ScheduleBoard{
		Catalog: ports.ScheduleCatalog{Cycle: cycle}, Cycles: []domain.Cycle{cycle}, Started: true,
		Grids: []domain.ScheduleGrid{domain.DefaultScheduleGrid("l1")}, Blocks: []domain.ScheduleBlock{s.block},
	}, s.err
}

func (s *stubSchedule) Day(_ context.Context, _ string, date time.Time, _ string) (usecase.ScheduleDay, error) {
	o, _ := domain.NewScheduleOverride("o1", "t1", "c1", s.block.ID(), date, domain.OverrideSubstitute, "t-eva", "", "tarde",
		domain.Actor{UserID: "u", Name: "D"}, date)
	c := domain.RegularClass(s.block)
	c.TeacherID, c.Status, c.Override = "t-eva", domain.ClassSubstitute, &o
	return usecase.ScheduleDay{Date: date, Weekday: domain.WeekdayOf(date), NonWorking: s.nonWorking, Classes: []domain.ScheduledClass{c}}, s.err
}

func (s *stubSchedule) CreateBlock(_ context.Context, in usecase.BlockInput, by domain.Actor) (domain.ScheduleBlock, error) {
	s.gotBlock, s.gotActor = in, by
	return s.block, s.err
}

func (s *stubSchedule) UpdateBlock(_ context.Context, _ string, in usecase.BlockInput, by domain.Actor) (domain.ScheduleBlock, error) {
	s.gotBlock, s.gotActor = in, by
	return s.block, s.err
}

func (s *stubSchedule) DeleteBlock(context.Context, string, string, domain.Actor) error { return s.err }

func (s *stubSchedule) CreateOverride(_ context.Context, in usecase.OverrideInput, by domain.Actor) (domain.ScheduleOverride, error) {
	s.gotOverride, s.gotActor = in, by
	if s.err != nil {
		return domain.ScheduleOverride{}, s.err
	}
	return domain.NewScheduleOverride("o1", "t1", "c1", in.BlockID, in.Date, in.Kind, in.TeacherID, in.SwapBlockID, in.Reason, by, in.Date)
}

func (s *stubSchedule) DeleteOverride(context.Context, string, string, domain.Actor) error {
	return s.err
}

func (s *stubSchedule) AvailableTeachers(_ context.Context, q usecase.AvailabilityQuery) ([]domain.AvailableTeacher, error) {
	s.gotQuery = q
	tc, _ := domain.NewTeacher("t-eva", "t1", "Eva", "Soto", "eva@x.mx", "")
	return []domain.AvailableTeacher{{Teacher: tc, Match: domain.MatchSubject}}, s.err
}

func (s *stubSchedule) SetGrid(_ context.Context, levelID string, start, end int) (domain.ScheduleGrid, error) {
	g, err := domain.NewScheduleGrid(levelID, start, end)
	if s.err != nil {
		return g, s.err
	}
	return g, err
}

func (s *stubSchedule) Events(context.Context, string, int) ([]domain.ScheduleEvent, error) {
	if s.event.ID != "" {
		return []domain.ScheduleEvent{s.event}, s.err
	}
	return []domain.ScheduleEvent{{ID: "e1", CycleID: "c1", Action: domain.ActionOverrideCreated, Summary: "Suplencia",
		Date: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), By: domain.Actor{UserID: "u", Name: "D"}}}, s.err
}

func newScheduleHandler(t *testing.T) (*grpcadapter.ConfigHandler, *stubSchedule) {
	t.Helper()
	h, _ := newHandler(t)
	ct, err := domain.NewClassTime(480, 530)
	require.NoError(t, err)
	b, err := domain.NewScheduleBlock("b1", "t1", "c1", "a1", "g1", "s1", "t-ana", 1, ct)
	require.NoError(t, err)
	stub := &stubSchedule{block: b}
	return h.WithSchedule(stub), stub
}

func TestScheduleRPCsWithoutTheUseCaseAreUnimplemented(t *testing.T) {
	h, _ := newHandler(t)
	_, err := h.GetScheduleBoard(ctx(), connect.NewRequest(&configv1.GetScheduleBoardRequest{}))
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}

func TestScheduleBlockRoundTripsAndForwardsTheActor(t *testing.T) {
	h, stub := newScheduleHandler(t)
	res, err := h.CreateScheduleBlock(ctx(), connect.NewRequest(&configv1.CreateScheduleBlockRequest{
		Block: &configv1.ScheduleBlockInput{AssignmentId: "a1", Weekday: 1, StartsAt: 480, EndsAt: 530, Reason: "r"},
		Actor: &configv1.ScheduleActor{UserId: "u-dir", Name: "Directora"},
	}))
	require.NoError(t, err)
	assert.Equal(t, usecase.BlockInput{AssignmentID: "a1", Weekday: 1, Start: 480, End: 530, Reason: "r"}, stub.gotBlock)
	assert.Equal(t, domain.Actor{UserID: "u-dir", Name: "Directora"}, stub.gotActor)
	got := res.Msg.GetBlock()
	assert.Equal(t, "b1", got.GetId())
	assert.Equal(t, int32(480), got.GetStartsAt())
	assert.Equal(t, "t-ana", got.GetTeacherId())
}

func TestScheduleRefusalCodesRideInTheErrorMeta(t *testing.T) {
	h, stub := newScheduleHandler(t)
	stub.err = domain.ScheduleRefusal(glerrors.KindConflict, domain.CodeScheduleTeacherBusy, "Ana Ruiz ya da Matemáticas")
	_, err := h.CreateScheduleBlock(ctx(), connect.NewRequest(&configv1.CreateScheduleBlockRequest{Block: &configv1.ScheduleBlockInput{}}))
	var ce *connect.Error
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, connect.CodeAlreadyExists, ce.Code())
	assert.Equal(t, "SCHEDULE_TEACHER_BUSY", ce.Meta().Get(grpcadapter.ErrorCodeMeta))
	assert.Equal(t, "Ana Ruiz ya da Matemáticas", ce.Message())
}

func TestScheduleBoardDayOverridesAndAvailability(t *testing.T) {
	h, stub := newScheduleHandler(t)
	board, err := h.GetScheduleBoard(ctx(), connect.NewRequest(&configv1.GetScheduleBoardRequest{TeacherId: "t-ana"}))
	require.NoError(t, err)
	assert.Equal(t, "t-ana", stub.gotTeacher)
	assert.True(t, board.Msg.GetStarted())
	require.Len(t, board.Msg.GetGrids(), 1)
	assert.Equal(t, int32(domain.DefaultGridStart), board.Msg.GetGrids()[0].GetGridStart())
	require.Len(t, board.Msg.GetBlocks(), 1)

	_, err = h.GetScheduleDay(ctx(), connect.NewRequest(&configv1.GetScheduleDayRequest{CycleId: "c1"}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "a day needs its date")

	mon := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	day, err := h.GetScheduleDay(ctx(), connect.NewRequest(&configv1.GetScheduleDayRequest{CycleId: "c1", Date: timestamppb.New(mon)}))
	require.NoError(t, err)
	assert.Equal(t, int32(1), day.Msg.GetWeekday())
	require.Len(t, day.Msg.GetClasses(), 1)
	c := day.Msg.GetClasses()[0]
	assert.Equal(t, "substitute", c.GetStatus())
	assert.Equal(t, "t-eva", c.GetTeacherId())
	assert.Equal(t, "t-ana", c.GetBlock().GetTeacherId())
	assert.Equal(t, "tarde", c.GetOverride().GetReason())

	o, err := h.CreateScheduleOverride(ctx(), connect.NewRequest(&configv1.CreateScheduleOverrideRequest{
		BlockId: "b1", Date: timestamppb.New(mon), Kind: "substitute", TeacherId: "t-eva", Reason: "tarde",
		Actor: &configv1.ScheduleActor{UserId: "u", Name: "D"},
	}))
	require.NoError(t, err)
	assert.Equal(t, domain.OverrideSubstitute, stub.gotOverride.Kind)
	assert.Equal(t, "D", o.Msg.GetOverride().GetCreatedBy().GetName())

	free, err := h.ListAvailableTeachers(ctx(), connect.NewRequest(&configv1.ListAvailableTeachersRequest{
		CycleId: "c1", GroupId: "g1", SubjectId: "s1", Date: timestamppb.New(mon), StartsAt: 480, EndsAt: 530, ExcludeBlockId: "b1",
	}))
	require.NoError(t, err)
	assert.Equal(t, mon, stub.gotQuery.Date)
	assert.Equal(t, "b1", stub.gotQuery.ExcludeBlockID)
	require.Len(t, free.Msg.GetTeachers(), 1)
	assert.Equal(t, "subject", free.Msg.GetTeachers()[0].GetMatch())

	ev, err := h.ListScheduleEvents(ctx(), connect.NewRequest(&configv1.ListScheduleEventsRequest{CycleId: "c1"}))
	require.NoError(t, err)
	require.Len(t, ev.Msg.GetEvents(), 1)
	assert.Equal(t, mon, ev.Msg.GetEvents()[0].GetDate().AsTime())

	grid, err := h.SetScheduleGrid(ctx(), connect.NewRequest(&configv1.SetScheduleGridRequest{LevelId: "l1", GridStart: 420, GridEnd: 840}))
	require.NoError(t, err)
	assert.Equal(t, int32(840), grid.Msg.GetGrid().GetGridEnd())
}
