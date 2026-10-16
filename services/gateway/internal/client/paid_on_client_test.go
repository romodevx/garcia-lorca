package client

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/timestamppb"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

// ADR 0035: the school-local day crosses the edge apart from the instant, on
// deposits, payments and correction snapshots.
func TestThePaidOnDayReachesTheEdge(t *testing.T) {
	instant := timestamppb.New(time.Date(2026, time.October, 16, 1, 0, 0, 0, time.UTC))

	d := toDeposit(&financev1.Deposit{Id: "d", PaidAt: instant, PaidOn: "2026-10-15"})
	p := toPayment(&financev1.Payment{Id: "p", PaidAt: instant, PaidOn: "2026-10-15"})
	s := toSnapshot(&financev1.DepositSnapshot{DepositId: "d", PaidAt: instant, PaidOn: "2026-10-15"})
	cut := toCashUp(&financev1.DailyCashUpResponse{Payments: []*financev1.CashUpPayment{
		{Payment: &financev1.Payment{Id: "p", PaidAt: instant, PaidOn: "2026-10-15"}},
	}})

	for label, got := range map[string]string{
		"deposit": d.PaidOn, "payment": p.PaidOn, "snapshot": s.PaidOn, "cash-up payment": cut.Payments[0].PaidOn,
	} {
		assert.Equal(t, "2026-10-15", got, label)
	}
	assert.Equal(t, "2026-10-16T01:00:00Z", d.PaidAt, "paid_at stays the instant")
}
