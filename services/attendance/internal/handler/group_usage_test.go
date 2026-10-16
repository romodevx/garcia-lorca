package handler_test

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	attendancev1 "github.com/garcia-lorca/gen/go/garcialorca/attendance/v1"
	"github.com/garcia-lorca/services/attendance/internal/handler"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

type stubGroupUsage struct{}

func (stubGroupUsage) GroupUsage(context.Context, string) (service.GroupUsage, error) {
	return service.GroupUsage{Sessions: 40, Marks: 900}, nil
}

func TestGroupUsageHandlerMapsTheWire(t *testing.T) {
	h := handler.NewAttendanceHandler(nil).WithGroupUsage(service.NewGroupUsageService(stubGroupUsage{}))

	res, err := h.GroupUsage(context.Background(), connect.NewRequest(&attendancev1.GroupUsageRequest{GroupId: "00000000-0000-4000-8000-000000000010"}))

	require.NoError(t, err)
	assert.Equal(t, int64(40), res.Msg.GetSessions())
	assert.Equal(t, int64(900), res.Msg.GetMarks())

	_, err = h.GroupUsage(context.Background(), connect.NewRequest(&attendancev1.GroupUsageRequest{GroupId: "nope"}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestGroupUsageUnwiredIsUnimplemented(t *testing.T) {
	_, err := handler.NewAttendanceHandler(nil).GroupUsage(context.Background(),
		connect.NewRequest(&attendancev1.GroupUsageRequest{GroupId: "00000000-0000-4000-8000-000000000010"}))

	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}
