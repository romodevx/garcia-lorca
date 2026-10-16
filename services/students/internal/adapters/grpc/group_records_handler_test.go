package grpcadapter_test

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	grpcadapter "github.com/garcia-lorca/services/students/internal/adapters/grpc"
	"github.com/garcia-lorca/services/students/internal/ports"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

type stubGroupRecords struct{ renamedTo string }

func (s *stubGroupRecords) GroupUsage(context.Context, string) (ports.GroupUsage, error) {
	return ports.GroupUsage{Enrollments: 25, Transfers: 3}, nil
}

func (s *stubGroupRecords) RefreshGroupName(_ context.Context, _, name string) (int64, error) {
	s.renamedTo = name
	return 24, nil
}

func newGroupRecordsHandler(stub *stubGroupRecords) *grpcadapter.StudentsHandler {
	return grpcadapter.NewStudentsHandler(nil, nil, nil, nil).WithGroupRecords(usecase.NewGroupRecordService(stub))
}

func TestGroupUsageHandler(t *testing.T) {
	h := newGroupRecordsHandler(&stubGroupRecords{})

	res, err := h.GroupUsage(context.Background(), connect.NewRequest(&studentsv1.GroupUsageRequest{GroupId: haGroup}))

	require.NoError(t, err)
	assert.Equal(t, int64(25), res.Msg.GetEnrollments())
	assert.Equal(t, int64(3), res.Msg.GetTransfers())

	_, err = h.GroupUsage(context.Background(), connect.NewRequest(&studentsv1.GroupUsageRequest{GroupId: "nope"}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestRefreshGroupNameHandler(t *testing.T) {
	stub := &stubGroupRecords{}
	h := newGroupRecordsHandler(stub)

	res, err := h.RefreshGroupName(context.Background(), connect.NewRequest(&studentsv1.RefreshGroupNameRequest{GroupId: haGroup, Name: "B"}))

	require.NoError(t, err)
	assert.Equal(t, int64(24), res.Msg.GetStudents())
	assert.Equal(t, "B", stub.renamedTo)

	_, err = h.RefreshGroupName(context.Background(), connect.NewRequest(&studentsv1.RefreshGroupNameRequest{GroupId: haGroup, Name: " "}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestGroupRecordsUnwiredAreUnimplemented(t *testing.T) {
	h := grpcadapter.NewStudentsHandler(nil, nil, nil, nil)

	_, err := h.GroupUsage(context.Background(), connect.NewRequest(&studentsv1.GroupUsageRequest{GroupId: haGroup}))

	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}
