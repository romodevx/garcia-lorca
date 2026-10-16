package grpcadapter_test

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
	grpcadapter "github.com/garcia-lorca/services/grades/internal/adapters/grpc"
	"github.com/garcia-lorca/services/grades/internal/usecase"
)

type stubGroupUsage struct{}

func (stubGroupUsage) CountGroupReportCards(context.Context, string) (int64, error) { return 3, nil }

func TestGroupUsageHandlerMapsTheWire(t *testing.T) {
	h := grpcadapter.NewGradesHandler(nil, nil, &stubPeriods{}, usecase.NewHealthService("grades")).
		WithGroupUsage(usecase.NewGroupUsageService(stubGroupUsage{}))

	res, err := h.GroupUsage(context.Background(), connect.NewRequest(&gradesv1.GroupUsageRequest{GroupId: "00000000-0000-4000-8000-000000000010"}))

	require.NoError(t, err)
	assert.Equal(t, int64(3), res.Msg.GetReportCards())

	_, err = h.GroupUsage(context.Background(), connect.NewRequest(&gradesv1.GroupUsageRequest{GroupId: "nope"}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestGroupUsageUnwiredIsUnimplemented(t *testing.T) {
	h := grpcadapter.NewGradesHandler(nil, nil, &stubPeriods{}, usecase.NewHealthService("grades"))

	_, err := h.GroupUsage(context.Background(), connect.NewRequest(&gradesv1.GroupUsageRequest{GroupId: "00000000-0000-4000-8000-000000000010"}))

	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}
