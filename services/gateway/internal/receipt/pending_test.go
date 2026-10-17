package receipt_test

import (
	"context"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// ISSUE-T14-020: an abono's receipt says what is still owed on what it paid.
// Finance works the figure out in recording order (review N1: a backdated
// later deposit must not change an old receipt), so the receipt prints what
// finance says — except on a deposit whose money is gone or moved (N3).
func TestAPartialPaymentSaysWhatIsStillPending(t *testing.T) {
	cases := []struct {
		name    string
		deposit client.Deposit
		pending int64
		want    int64
	}{
		{name: "an abono", deposit: client.Deposit{}, pending: 100000, want: 100000},
		{name: "a settled payment", deposit: client.Deposit{}, pending: 0, want: 0},
		{name: "a voided deposit owes nothing on paper", deposit: client.Deposit{VoidedAt: "2026-10-18T00:00:00Z"}, pending: 100000, want: 0},
		{name: "a corrected (superseded) deposit neither", deposit: client.Deposit{SupersededBy: "dep-9", SupersededByFolio: "P-2026-000009"}, pending: 100000, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := tc.deposit
			d.ID, d.StudentID, d.AmountCents, d.Method, d.PaidAt, d.Folio = "dep-1", "stu-1", 300000, "cash", "2026-10-17T16:00:00Z", "P-2026-000002"
			f := fakeSources{deposit: client.DepositReceipt{
				Deposit:      d,
				Lines:        []client.ReceiptLine{{ChargeID: "ins", Description: "Inscripción", AmountCents: 300000}},
				PendingCents: tc.pending,
			}}
			got, err := builder(f).Payment(context.Background(), "dep-1")
			if err != nil {
				t.Fatalf("Payment: %v", err)
			}
			if got.PendingCents != tc.want {
				t.Errorf("pending = %d, want %d", got.PendingCents, tc.want)
			}
		})
	}
}
