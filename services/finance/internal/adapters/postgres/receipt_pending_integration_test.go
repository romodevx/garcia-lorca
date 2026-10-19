//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		// Through the use case: pricing what is pending is its rule (CC-201).
		got, err := depositService(t, repo, tenant).DepositReceipt(ctx, tc.id)
		if err != nil {
			t.Fatalf("%s: DepositReceipt: %v", tc.name, err)
		}
		if got.PendingCents != tc.want {
			t.Errorf("%s: pending = %d, want %d", tc.name, got.PendingCents, tc.want)
		}
	}
}

// CC-201: a part payment on a charge with a scholarship still pending (read
// at the counter, written only once the charge settles, ADR 0022) says on its
// receipt what the statement says: $4,000 − beca $500 − $3,000 = $500, never
// the $1,000 the stored price gives.
func TestAReceiptPricesThePendingScholarshipLikeTheStatement(t *testing.T) {
	for _, tc := range []struct {
		name    string
		n       string
		kind    string
		value   int64
		paid    int64
		pending int64
	}{
		{"fixed $500 off, $3,000 paid", "1", domain.ScholarshipFixedOff, 50000, 300000, 50000},
		{"25 % off, $2,000 paid", "2", domain.ScholarshipPercent, 2500, 200000, 100000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			// One id space per case: concepts and charges are keyed globally.
			id := func(tail string) string { return "cc20100" + tc.n + "-0000-0000-0000-0000000000" + tail }
			tenant, student, concept, charge, beca := id("01"), id("a1"), id("b1"), id("c1"), id("d1")
			due := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
			seedConceptAndCharge(t, tenant, concept, "INSCRIPCION QA", charge, student,
				id("e1"), 400000, 0, due)
			repo := appRepo(t, tenant)
			_, err := repo.CreateScholarship(ctx, mustScholarship(t, beca, tenant, "APOYO FIJO QA", tc.kind, tc.value, true,
				domain.ConceptTarget(concept)))
			require.NoError(t, err)
			_, err = repo.Assign(ctx, []domain.ScholarshipHolding{holding(t, id("f1"), tenant, beca, student)})
			require.NoError(t, err)
			svc := depositService(t, repo, tenant)

			got, err := payChosen(svc, student, charge, "cc201-"+tc.name, tc.paid)
			require.NoError(t, err)

			receipt, err := svc.DepositReceipt(ctx, got.Deposit.ID())
			require.NoError(t, err)
			assert.Equal(t, tc.pending, receipt.PendingCents, "the receipt's Saldo pendiente")
			statement, err := svc.GetStatement(ctx, student)
			require.NoError(t, err)
			assert.Equal(t, statement.Totals.BalanceCents, receipt.PendingCents,
				"the receipt and the statement say the same balance")
		})
	}
}
