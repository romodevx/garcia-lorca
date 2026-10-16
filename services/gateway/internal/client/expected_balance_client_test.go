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

type stubExpectedFinance struct {
	financev1connect.UnimplementedFinanceServiceHandler
	got *financev1.ExpectedTodayRequest
}

func (s *stubExpectedFinance) ExpectedToday(_ context.Context, req *connect.Request[financev1.ExpectedTodayRequest]) (*connect.Response[financev1.ExpectedTodayResponse], error) {
	s.got = req.Msg
	return connect.NewResponse(&financev1.ExpectedTodayResponse{
		AnnualBalanceCents: 3200000, OnTimeBalanceCents: 3000000,
		Lines: []*financev1.ExpectedLine{
			{ChargeId: "ch-sep", TotalCents: 300000, DiscountCents: 20000, InBalance: true},
			{ChargeId: "ch-ins", TotalCents: 50000},
		},
	}), nil
}

// D1 (T5 review): the "si paga a tiempo" figure and each line's in_balance
// cross the gateway; withBalance=false asks the service to skip the ledger.
func TestExpectedTodayCarriesTheBalanceItHolds(t *testing.T) {
	stub := &stubExpectedFinance{}
	path, h := financev1connect.NewFinanceServiceHandler(stub)
	c := client.NewFinanceClient(serveConnect(t, path, h), nil)

	got, err := c.ExpectedToday(context.Background(), "stu-1", "", true)
	require.NoError(t, err)
	assert.False(t, stub.got.GetSkipBalance())
	assert.Equal(t, int64(3000000), got.OnTimeBalanceCents)
	require.Len(t, got.Lines, 2)
	assert.True(t, got.Lines[0].InBalance)
	assert.False(t, got.Lines[1].InBalance)

	_, err = c.ExpectedToday(context.Background(), "stu-1", "", false)
	require.NoError(t, err)
	assert.True(t, stub.got.GetSkipBalance(), "withBalance=false skips the ledger")
}
