package receipt_test

import (
	"context"
	"strings"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/receipt"
)

// T9: both receipts of a correction can be printed again, each naming the
// other — the original "Sustituido por P-…", the correction "Corrige a P-…".
func TestACorrectionsReceiptsNameEachOther(t *testing.T) {
	original := fakeSources{deposit: client.DepositReceipt{Deposit: client.Deposit{
		ID: "dep-1", StudentID: "stu-1", AmountCents: 50000, Method: "cash", PaidAt: "2026-10-10T00:00:00Z",
		Folio: "P-2026-000001", SupersededAt: "2026-10-11T00:00:00Z", SupersededBy: "dep-2", SupersededByFolio: "P-2026-000002",
	}}}
	got, err := builder(original).Payment(context.Background(), "dep-1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Superseded || got.SupersededByFolio != "P-2026-000002" || got.Voided {
		t.Errorf("original receipt = %+v", got)
	}

	corrected := fakeSources{deposit: client.DepositReceipt{Deposit: client.Deposit{
		ID: "dep-2", StudentID: "stu-1", AmountCents: 100000, Method: "cash", PaidAt: "2026-10-10T00:00:00Z",
		Folio: "P-2026-000002", CorrectsDepositID: "dep-1", CorrectsFolio: "P-2026-000001",
	}}}
	got, err = builder(corrected).Payment(context.Background(), "dep-2")
	if err != nil {
		t.Fatal(err)
	}
	if got.CorrectsFolio != "P-2026-000001" || got.Superseded {
		t.Errorf("corrected receipt = %+v", got)
	}
}

func TestTheCorrectionPDFsSayIt(t *testing.T) {
	doc, err := receipt.PDF(receipt.Receipt{Kind: receipt.KindPayment, Folio: "P-2026-000001", PaidOn: "2026-10-10", TotalCents: 50000, Superseded: true, SupersededByFolio: "P-2026-000002"})
	if err != nil {
		t.Fatal(err)
	}
	if text := textOf(t, doc); !strings.Contains(text, "SUSTITUIDO") || !strings.Contains(text, "P-2026-000002") {
		t.Errorf("the superseded receipt reads:\n%s", text)
	}
	doc, err = receipt.PDF(receipt.Receipt{Kind: receipt.KindPayment, Folio: "P-2026-000002", PaidOn: "2026-10-10", TotalCents: 100000, CorrectsFolio: "P-2026-000001"})
	if err != nil {
		t.Fatal(err)
	}
	if text := textOf(t, doc); !strings.Contains(text, "Corrige a P-2026-000001") {
		t.Errorf("the correction's receipt reads:\n%s", text)
	}
}

func TestTheStatementPDFNamesTheCorrection(t *testing.T) {
	doc, err := receipt.StatementPDF(receipt.StatementDoc{Statement: client.Statement{Movements: []client.StatementMovement{
		{Kind: "payment", Date: "2026-10-10", Description: "Pago", AmountCents: 50000, Folio: "P-2026-000001", Superseded: true, SupersededByFolio: "P-2026-000002"},
		{Kind: "payment", Date: "2026-10-10", Description: "Pago", AmountCents: 100000, Folio: "P-2026-000002", CorrectsFolio: "P-2026-000001"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	// The concept column wraps; read the words, not the line breaks.
	text := strings.Join(strings.Fields(textOf(t, doc)), " ")
	for _, want := range []string{"Sustituido por P-2026-000002", "Corrige a P-2026-000001"} {
		if !strings.Contains(text, want) {
			t.Errorf("the statement PDF does not say %q; it reads:\n%s", want, text)
		}
	}
}
