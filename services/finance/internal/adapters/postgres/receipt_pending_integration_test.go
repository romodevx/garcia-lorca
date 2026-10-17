//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// T14-020 (review N1): a receipt says what its charges still owed right after
// it, ordered by when each payment was RECORDED — not by the day it is dated —
// so a later deposit backdated before it never changes an old receipt, and a
// reprint says what the paper said.
func TestDepositReceiptSaysWhatWasPendingRightAfterIt(t *testing.T) {
	ctx := context.Background()
	tenant, student := tenantA, "80000000-0000-0000-0000-0000000000f1"
	due := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	// $3,200 less a $100 discount: $3,100 owed.
	charge := seedConceptAndCharge(t, tenant, "60000000-0000-0000-0000-0000000000f1", "colegiatura pendiente",
		"61000000-0000-0000-0000-0000000000f1", student, "70000000-0000-0000-0000-0000000000f1", 320000, 10000, due)
	repo := appRepo(t, tenant)
	pay := func(depositID, paymentID string, amount int64, paidOn time.Time) domain.Deposit {
		d := newDeposit(t, depositID, tenant, student, amount, paidOn, "")
		if _, _, err := repo.RegisterDeposit(ctx, d, func([]ports.ChargeWithPaid) (ports.DepositWrite, error) {
			return ports.DepositWrite{Allocations: []domain.Payment{newPayment(t, paymentID, tenant, charge.ID(), amount, paidOn)}}, nil
		}, nil); err != nil {
			t.Fatalf("RegisterDeposit %s: %v", depositID, err)
		}
		return d
	}
	first := pay("62000000-0000-0000-0000-0000000000f1", "90000000-0000-0000-0000-0000000000f1", 200000,
		time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC))
	// Recorded AFTER the first, dated BEFORE it: a cashier catching up.
	second := pay("62000000-0000-0000-0000-0000000000f2", "90000000-0000-0000-0000-0000000000f2", 60000,
		time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))

	for _, tc := range []struct {
		name string
		id   string
		want int64
	}{
		{"the first abono leaves $1,100, whatever came after", first.ID(), 110000},
		{"the backdated second leaves $500", second.ID(), 50000},
	} {
		got, err := repo.DepositReceipt(ctx, tc.id)
		if err != nil {
			t.Fatalf("%s: DepositReceipt: %v", tc.name, err)
		}
		if got.PendingCents != tc.want {
			t.Errorf("%s: pending = %d, want %d", tc.name, got.PendingCents, tc.want)
		}
	}
}
