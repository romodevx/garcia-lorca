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
	"github.com/garcia-lorca/services/config/internal/usecase"
)

func (s *stubSchedule) SchoolDay(_ context.Context, cycleID string, date time.Time) (usecase.SchoolDayStatus, error) {
	s.gotID, s.gotFrom = cycleID, date
	return usecase.SchoolDayStatus{Closed: s.nonWorking, Open: s.nonWorking == ""}, s.err
}

func TestGetSchoolDayAnswersTheCalendarRule(t *testing.T) {
	h, stub := newScheduleHandler(t)
	_, err := h.GetSchoolDay(ctx(), connect.NewRequest(&configv1.GetSchoolDayRequest{CycleId: "c1"}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "a school day needs its date")

	mon := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	open, err := h.GetSchoolDay(ctx(), connect.NewRequest(&configv1.GetSchoolDayRequest{CycleId: "c1", Date: timestamppb.New(mon)}))
	require.NoError(t, err)
	assert.True(t, open.Msg.GetOpen())
	assert.Equal(t, "c1", stub.gotID)
	assert.Equal(t, mon, stub.gotFrom)

	stub.nonWorking = "Independencia"
	closed, err := h.GetSchoolDay(ctx(), connect.NewRequest(&configv1.GetSchoolDayRequest{CycleId: "c1", Date: timestamppb.New(mon)}))
	require.NoError(t, err)
	assert.False(t, closed.Msg.GetOpen())
	assert.Equal(t, "Independencia", closed.Msg.GetClosedLabel())
}
