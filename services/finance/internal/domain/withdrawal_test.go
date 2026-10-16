package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanWithdrawal(t *testing.T) {
	on := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	newCharge := func(id string, due time.Time, status string) Charge {
		c, err := NewCharge(id, "tenant", "student", "cycle", "concept", "cuota", 10000, 0, due)
		require.NoError(t, err)
		c.status = status
		return c
	}
	before := newCharge("before", on.AddDate(0, 0, -1), StatusPending)
	equal := newCharge("equal", on, StatusPending)
	future, err := newCharge("future", on.AddDate(0, 0, 1), StatusPending).WithExternalRef("tuition:cycle:student:2026-10")
	require.NoError(t, err)
	paid := newCharge("paid", on.AddDate(0, 0, 5), StatusPaid)
	partial := newCharge("partial", on.AddDate(0, 0, 5), StatusPartiallyPaid)
	cancelled := newCharge("cancelled", on.AddDate(0, 0, 5), StatusCancelled)
	activity, err := newCharge("activity", on.AddDate(0, 0, 2), StatusPending).WithExternalRef("activity:act:student:2026-10")
	require.NoError(t, err)
	fee, err := newCharge("fee", on.AddDate(0, 0, 2), StatusPending).WithExternalRef("fee:plan:2026-10")
	require.NoError(t, err)
	enrollment, err := newCharge("enrollment", on.AddDate(0, 0, 2), StatusPending).WithExternalRef("enrollment:cycle:student")
	require.NoError(t, err)
	uniforms, err := newCharge("uniforms", on.AddDate(0, 0, 2), StatusPending).WithExternalRef("uniforms:sale-1")
	require.NoError(t, err)
	manual := newCharge("manual", on.AddDate(0, 0, 2), StatusPending)

	tests := []struct {
		name            string
		charges         []Charge
		paid            map[string]int64
		wantCancel      []string
		wantCancelCents int64
		wantDebt        int64
	}{
		{name: "boundary and protections", charges: []Charge{before, equal, future, paid, partial, cancelled, activity, fee, enrollment, uniforms, manual}, paid: map[string]int64{"paid": 10000, "partial": 2500}, wantCancel: []string{"future", "activity", "fee", "enrollment"}, wantCancelCents: 40000, wantDebt: 47500},
		{name: "empty", wantCancel: []string{}, wantCancelCents: 0, wantDebt: 0},
		{name: "future with any payment is kept", charges: []Charge{partial}, paid: map[string]int64{"partial": 1}, wantCancel: []string{}, wantCancelCents: 0, wantDebt: 9999},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PlanWithdrawal(tt.charges, tt.paid, on)
			require.Len(t, got.ToCancel, len(tt.wantCancel))
			assert.Equal(t, tt.wantCancel, withdrawalIDs(got.ToCancel))
			assert.Equal(t, tt.wantCancelCents, got.ToCancelCents)
			assert.Equal(t, tt.wantDebt, got.RemainingDebtCents)
		})
	}
}

func withdrawalIDs(charges []Charge) []string {
	ids := make([]string, len(charges))
	for i, c := range charges {
		ids[i] = c.ID()
	}
	return ids
}
