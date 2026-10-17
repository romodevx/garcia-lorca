package receipt_test

import (
	"context"
	"errors"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/receipt"
)

// failingAccount answers everything as fakeSources does, except the account.
type failingAccount struct{ fakeSources }

func (failingAccount) GetAccount(context.Context, string) (client.AccountStatement, error) {
	return client.AccountStatement{}, errors.New("finance is down")
}

// ISSUE-T14-020: an abono's receipt says what is still owed on what it paid,
// as of right after it — a later payment does not rewrite an old receipt.
func TestAPartialPaymentSaysWhatIsStillPending(t *testing.T) {
	cases := []struct {
		name    string
		lines   []client.ReceiptLine
		account client.AccountStatement
		want    int64
	}{
		{
			name:  "an abono to one charge",
			lines: []client.ReceiptLine{{ChargeID: "ins", Description: "Inscripción", AmountCents: 300000}},
			account: client.AccountStatement{
				Charges:  []client.Charge{{ID: "ins", Status: "partial", RemainingCents: 100000}},
				Payments: []client.Payment{{ChargeID: "ins", AmountCents: 300000, DepositID: "dep-1", PaidAt: "2026-10-17T16:00:00Z"}},
			},
			want: 100000,
		},
		{
			name:  "a later payment is added back: the receipt keeps its own day's figure",
			lines: []client.ReceiptLine{{ChargeID: "ins", Description: "Inscripción", AmountCents: 300000}},
			account: client.AccountStatement{
				Charges: []client.Charge{{ID: "ins", Status: "paid", RemainingCents: 0}},
				Payments: []client.Payment{
					{ChargeID: "ins", AmountCents: 300000, DepositID: "dep-1", PaidAt: "2026-10-17T16:00:00Z"},
					{ChargeID: "ins", AmountCents: 100000, DepositID: "dep-2", PaidAt: "2026-10-20T16:00:00Z"},
				},
			},
			want: 100000,
		},
		{
			name: "a settled charge and a cancelled one owe nothing; the surcharge line is a charge of its own",
			lines: []client.ReceiptLine{
				{ChargeID: "sep", Description: "Colegiatura septiembre", AmountCents: 200000},
				{ChargeID: "fee", Description: "Recargo", AmountCents: 10000, Surcharge: true},
				{ChargeID: "oct", Description: "Colegiatura octubre", AmountCents: 50000},
			},
			account: client.AccountStatement{Charges: []client.Charge{
				{ID: "sep", Status: "paid", RemainingCents: 0},
				{ID: "fee", Status: "partial", RemainingCents: 5000},
				{ID: "oct", Status: "cancelled", RemainingCents: 150000},
			}},
			want: 5000,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := fakeSources{
				account: tc.account,
				deposit: client.DepositReceipt{
					Deposit: client.Deposit{ID: "dep-1", StudentID: "stu-1", AmountCents: 300000, Method: "cash",
						PaidAt: "2026-10-17T16:00:00Z", Folio: "P-2026-000002"},
					Lines: tc.lines,
				},
			}
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

// The balance is a courtesy: an account that cannot be read still prints the receipt.
func TestAReceiptPrintsWhenTheBalanceCannotBeRead(t *testing.T) {
	f := failingAccount{fakeSources{deposit: client.DepositReceipt{
		Deposit: client.Deposit{ID: "dep-1", StudentID: "stu-1", AmountCents: 100, Method: "cash", PaidAt: "2026-10-17T16:00:00Z", Folio: "P-2026-000003"},
		Lines:   []client.ReceiptLine{{ChargeID: "ins", Description: "Inscripción", AmountCents: 100}},
	}}}
	got, err := receipt.NewBuilder(f, f, f, f, f).Payment(context.Background(), "dep-1")
	if err != nil {
		t.Fatalf("Payment: %v", err)
	}
	if got.PendingCents != 0 || got.Folio != "P-2026-000003" {
		t.Errorf("pending %d, folio %q", got.PendingCents, got.Folio)
	}
}
