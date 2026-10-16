package grpcadapter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/finance/internal/domain"
)

// ADR 0035: the wire carries a payment's school-local DAY apart from its
// instant. 19:00 on the 15th in Morelia is 01:00 UTC on the 16th; the gateway
// prints the receipt's "Fecha de pago" from paid_on, never from paid_at.
func TestThePaidOnDayCrossesTheWire(t *testing.T) {
	instant := time.Date(2026, time.October, 16, 1, 0, 0, 0, time.UTC)
	day := time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)

	d, err := domain.NewDeposit("dep-1", "t1", "st-1", 100000, domain.MethodCash, "", instant, "", "")
	require.NoError(t, err)
	d = d.WithPaidOn(day)
	p, err := domain.NewPayment("pay-1", "t1", "ch-1", 100000, domain.MethodCash, "", instant, "")
	require.NoError(t, err)
	p = p.WithPaidOn(day)

	cases := []struct {
		name   string
		paidOn string
		paidAt time.Time
	}{
		{"deposit", toProtoDeposit(d).GetPaidOn(), toProtoDeposit(d).GetPaidAt().AsTime()},
		{"payment", toProtoPayment(p).GetPaidOn(), toProtoPayment(p).GetPaidAt().AsTime()},
		{"snapshot", toProtoSnapshot(domain.SnapshotOf(d, nil), nil).GetPaidOn(), toProtoSnapshot(domain.SnapshotOf(d, nil), nil).GetPaidAt().AsTime()},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, "2026-10-15", tt.paidOn)
			assert.True(t, tt.paidAt.Equal(instant), "paid_at stays the instant")
		})
	}
}

// A snapshot written before the day was kept carries none: the wire says so
// with an empty paid_on rather than inventing "0001-01-01".
func TestAnUndatedSnapshotSendsNoDay(t *testing.T) {
	got := toProtoSnapshot(domain.DepositSnapshot{PaidAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}, nil)
	assert.Empty(t, got.GetPaidOn())
}
