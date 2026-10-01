package receipt_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/receipt"
)

// T8: a payment made with a credit note and cash prints both, each with its
// amount, in the office's words.
func TestAPaymentReceiptBreaksDownItsTenders(t *testing.T) {
	f := fakeSources{deposit: client.DepositReceipt{Deposit: client.Deposit{
		ID: "dep-8", StudentID: "stu-1", AmountCents: 350000, Method: "mixed", PaidAt: "2026-10-10T00:00:00Z", Folio: "P-2026-000080",
		Tenders: []client.Tender{
			{Method: "credit_note", AmountCents: 200000, Reference: "NC-7"},
			{Method: "card", AmountCents: 150000, CardType: "credit"},
		},
	}}}
	got, err := builder(f).Payment(context.Background(), "dep-8")
	if err != nil {
		t.Fatalf("Payment: %v", err)
	}
	want := []receipt.TenderLine{
		{Method: "Nota de crédito", AmountCents: 200000, Reference: "NC-7"},
		{Method: "Tarjeta de crédito", AmountCents: 150000},
	}
	if len(got.Tenders) != 2 || got.Tenders[0] != want[0] || got.Tenders[1] != want[1] {
		t.Errorf("tenders = %+v, want %+v", got.Tenders, want)
	}
	if got.Method != "Nota de crédito + Tarjeta de crédito" {
		t.Errorf("method = %q", got.Method)
	}

	pdf, err := receipt.PDF(got)
	if err != nil {
		t.Fatalf("PDF: %v", err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Error("not a PDF")
	}
}

// A uniforms sale paid by a credit note and cash: every payment on the sale's
// charge is a tender of its receipt.
func TestASaleReceiptBreaksDownItsTenders(t *testing.T) {
	f := fakeSources{
		order: client.UniformOrder{
			ID: "ord-8", Folio: "U-2026-000080", Source: "counter", StudentID: "stu-1", TotalCents: 85000,
			CreatedAt:    "2026-10-10T10:00:00Z",
			Fulfillments: []client.UniformFulfillment{{FinanceChargeID: "ch-8"}},
		},
		account: client.AccountStatement{Payments: []client.Payment{
			{ChargeID: "ch-other", Method: "cash", AmountCents: 1},
			{ChargeID: "ch-8", Method: "credit_note", AmountCents: 30000, Reference: "NC-12"},
			{ChargeID: "ch-8", Method: "cash", AmountCents: 55000},
		}},
	}
	got, err := builder(f).Sale(context.Background(), "ord-8")
	if err != nil {
		t.Fatalf("Sale: %v", err)
	}
	if len(got.Tenders) != 2 || got.Tenders[0].Method != "Nota de crédito" || got.Tenders[0].Reference != "NC-12" ||
		got.Tenders[1].Method != "Efectivo" || got.Tenders[1].AmountCents != 55000 {
		t.Errorf("tenders = %+v", got.Tenders)
	}
	if got.Method != "Nota de crédito + Efectivo" {
		t.Errorf("method = %q", got.Method)
	}
}
