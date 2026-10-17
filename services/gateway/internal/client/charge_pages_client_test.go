package client_test

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/finance/v1/financev1connect"
	"github.com/garcia-lorca/services/gateway/internal/client"
)

type stubPagedFinance struct {
	financev1connect.UnimplementedFinanceServiceHandler
	charges *financev1.ListChargesRequest
	overdue *financev1.ListOverdueRequest
}

func (s *stubPagedFinance) ListCharges(_ context.Context, req *connect.Request[financev1.ListChargesRequest]) (*connect.Response[financev1.ListChargesResponse], error) {
	s.charges = req.Msg
	return connect.NewResponse(&financev1.ListChargesResponse{
		Charges:       []*financev1.Charge{{Id: "ch-1"}},
		NextPageToken: "next-charges",
	}), nil
}

func (s *stubPagedFinance) ListOverdue(_ context.Context, req *connect.Request[financev1.ListOverdueRequest]) (*connect.Response[financev1.ListOverdueResponse], error) {
	s.overdue = req.Msg
	return connect.NewResponse(&financev1.ListOverdueResponse{
		Charges:       []*financev1.Charge{{Id: "ch-2"}},
		NextPageToken: "next-overdue",
	}), nil
}

// T14: the page asked for and the way to the next one cross the gateway both
// ways, for both cycle-wide reads.
func TestChargePagesCrossTheWire(t *testing.T) {
	stub := &stubPagedFinance{}
	path, h := financev1connect.NewFinanceServiceHandler(stub)
	c := client.NewFinanceClient(serveConnect(t, path, h), nil)
	ctx := context.Background()

	charges, err := c.ListCharges(ctx, "stu-1", "cyc-1", client.PageRequest{Token: "tok-a", Size: 200})
	require.NoError(t, err)
	assert.Equal(t, "stu-1", stub.charges.GetStudentId())
	assert.Equal(t, "cyc-1", stub.charges.GetCycleId())
	assert.Equal(t, "tok-a", stub.charges.GetPageToken())
	assert.Equal(t, int32(200), stub.charges.GetPageSize())
	require.Len(t, charges.Charges, 1)
	assert.Equal(t, "next-charges", charges.NextPageToken)

	overdue, err := c.ListOverdue(ctx, "cyc-1", client.PageRequest{Token: "tok-b", Size: 50})
	require.NoError(t, err)
	assert.Equal(t, "cyc-1", stub.overdue.GetCycleId())
	assert.Equal(t, "tok-b", stub.overdue.GetPageToken())
	assert.Equal(t, int32(50), stub.overdue.GetPageSize())
	require.Len(t, overdue.Charges, 1)
	assert.Equal(t, "next-overdue", overdue.NextPageToken)
}
