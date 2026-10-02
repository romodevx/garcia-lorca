package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	"github.com/garcia-lorca/services/gateway/internal/client"
)

// stubSchedule answers the timetable RPCs over a real Connect round-trip.
type stubSchedule struct {
	configv1connect.UnimplementedConfigServiceHandler
	gotCreate *configv1.CreateScheduleBlockRequest
	gotOvr    *configv1.CreateScheduleOverrideRequest
	gotAvail  *configv1.ListAvailableTeachersRequest
}

var mon5 = time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC)

func protoBlock() *configv1.ScheduleBlock {
	return &configv1.ScheduleBlock{Id: "b1", CycleId: "c1", AssignmentId: "a1", GroupId: "g1", SubjectId: "s1", TeacherId: "t1", Weekday: 1, StartsAt: 480, EndsAt: 530}
}

func (s *stubSchedule) GetScheduleBoard(_ context.Context, req *connect.Request[configv1.GetScheduleBoardRequest]) (*connect.Response[configv1.GetScheduleBoardResponse], error) {
	return connect.NewResponse(&configv1.GetScheduleBoardResponse{
		Cycle:   &configv1.Cycle{Id: "c1", Name: "2026-2027", Status: configv1.CycleStatus_CYCLE_STATUS_ACTIVE, StartsOn: timestamppb.New(mon5), EndsOn: timestamppb.New(mon5)},
		Started: true, Grids: []*configv1.ScheduleGrid{{LevelId: "l1", GridStart: 420, GridEnd: 900}},
		Teachers: []*configv1.Teacher{{Id: "t1", FirstName: "Ana", LastName: "Ruiz"}},
		Blocks:   []*configv1.ScheduleBlock{protoBlock()},
	}), nil
}

func (s *stubSchedule) GetScheduleDay(_ context.Context, req *connect.Request[configv1.GetScheduleDayRequest]) (*connect.Response[configv1.GetScheduleDayResponse], error) {
	return connect.NewResponse(&configv1.GetScheduleDayResponse{
		Date: req.Msg.GetDate(), Weekday: 1,
		Classes: []*configv1.ScheduledClass{{Block: protoBlock(), TeacherId: "t2", StartsAt: 480, EndsAt: 530, Status: "substitute",
			Override: &configv1.ScheduleOverride{Id: "o1", Kind: "substitute", TeacherId: "t2", Date: req.Msg.GetDate(), Reason: "tarde",
				CreatedBy: &configv1.ScheduleActor{UserId: "u", Name: "D"}, CreatedAt: timestamppb.New(mon5)}}},
	}), nil
}

func (s *stubSchedule) CreateScheduleBlock(_ context.Context, req *connect.Request[configv1.CreateScheduleBlockRequest]) (*connect.Response[configv1.CreateScheduleBlockResponse], error) {
	s.gotCreate = req.Msg
	return connect.NewResponse(&configv1.CreateScheduleBlockResponse{Block: protoBlock()}), nil
}

func (s *stubSchedule) CreateScheduleOverride(_ context.Context, req *connect.Request[configv1.CreateScheduleOverrideRequest]) (*connect.Response[configv1.CreateScheduleOverrideResponse], error) {
	s.gotOvr = req.Msg
	return connect.NewResponse(&configv1.CreateScheduleOverrideResponse{Override: &configv1.ScheduleOverride{Id: "o1", Date: req.Msg.GetDate(), Kind: req.Msg.GetKind()}}), nil
}

func (s *stubSchedule) ListAvailableTeachers(_ context.Context, req *connect.Request[configv1.ListAvailableTeachersRequest]) (*connect.Response[configv1.ListAvailableTeachersResponse], error) {
	s.gotAvail = req.Msg
	return connect.NewResponse(&configv1.ListAvailableTeachersResponse{Teachers: []*configv1.AvailableTeacher{{Teacher: &configv1.Teacher{Id: "t2", FirstName: "Eva"}, Match: "grade"}}}), nil
}

func TestConfigClientSchedule(t *testing.T) {
	stub := &stubSchedule{}
	mux := http.NewServeMux()
	path, h := configv1connect.NewConfigServiceHandler(stub)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := client.NewConfigClient(srv.URL, srv.Client())
	ctx := context.Background()

	board, err := c.GetScheduleBoard(ctx, "", "")
	require.NoError(t, err)
	assert.Equal(t, "c1", board.Cycle.ID)
	assert.True(t, board.Started)
	assert.Equal(t, []client.ScheduleGrid{{LevelID: "l1", GridStart: 420, GridEnd: 900}}, board.Grids)
	assert.Equal(t, []client.ScheduleBlock{{ID: "b1", CycleID: "c1", AssignmentID: "a1", GroupID: "g1", SubjectID: "s1", TeacherID: "t1", Weekday: 1, StartsAt: 480, EndsAt: 530}}, board.Blocks)

	day, err := c.GetScheduleDay(ctx, "c1", mon5, "")
	require.NoError(t, err)
	require.Len(t, day.Classes, 1)
	require.NotNil(t, day.Classes[0].Override)
	assert.Equal(t, "D", day.Classes[0].Override.CreatedBy.Name)
	assert.Equal(t, mon5, day.Date)

	_, err = c.CreateScheduleBlock(ctx, client.ScheduleBlockInput{AssignmentID: "a1", Weekday: 1, StartsAt: 480, EndsAt: 530, Reason: "r"},
		client.ScheduleActor{UserID: "u", Name: "Directora"})
	require.NoError(t, err)
	assert.Equal(t, "Directora", stub.gotCreate.GetActor().GetName())
	assert.Equal(t, int32(530), stub.gotCreate.GetBlock().GetEndsAt())

	o, err := c.CreateScheduleOverride(ctx, client.ScheduleOverrideInput{BlockID: "b1", Date: mon5, Kind: "cancel", Reason: "junta"}, client.ScheduleActor{})
	require.NoError(t, err)
	assert.Equal(t, mon5, o.Date)
	assert.Equal(t, "junta", stub.gotOvr.GetReason())

	free, err := c.ListAvailableTeachers(ctx, client.AvailabilityQuery{CycleID: "c1", GroupID: "g1", SubjectID: "s1", Weekday: 2, StartsAt: 480, EndsAt: 530})
	require.NoError(t, err)
	assert.Nil(t, stub.gotAvail.GetDate(), "no date: the base timetable of the weekday")
	assert.Equal(t, int32(2), stub.gotAvail.GetWeekday())
	require.Len(t, free, 1)
	assert.Equal(t, "grade", free[0].Match)
}
