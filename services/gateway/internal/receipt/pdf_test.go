package receipt_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/ledongthuc/pdf"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/receipt"
)

func textOf(t *testing.T, doc []byte) string {
	t.Helper()
	r, err := pdf.NewReader(bytes.NewReader(doc), int64(len(doc)))
	if err != nil {
		t.Fatalf("the PDF does not open: %v", err)
	}
	plain, err := r.GetPlainText()
	if err != nil {
		t.Fatalf("reading its text: %v", err)
	}
	var b bytes.Buffer
	if _, err := io.Copy(&b, plain); err != nil {
		t.Fatalf("copying its text: %v", err)
	}
	return b.String()
}

// Task 12: the PDF carries what the receipt says — the school, the folio, the
// pupil, the concepts and the total — in Spanish, accents included.
func TestThePDFSaysWhatTheReceiptSays(t *testing.T) {
	r := receipt.Receipt{
		Kind: receipt.KindPayment, Folio: "P-2026-000042",
		School: client.SchoolProfile{Name: "COLEGIO GARCÍA LORCA", RFC: "EGL150101AB1", Legend: "Gracias por su pago."},
		PaidOn: "2026-10-10", Buyer: "Camila Zamora Sánchez", BuyerDetail: "1° A",
		Lines: []receipt.Line{
			{Description: "Colegiatura septiembre 2026", AmountCents: 310000, DiscountCents: 10000},
			{Description: "Recargo · Colegiatura septiembre 2026", AmountCents: 30000, Surcharge: true},
		},
		SurchargeCents: 30000, DiscountCents: 10000, CreditCents: 10000, TotalCents: 350000,
		Method: "Tarjeta de débito", ReceivedBy: "Laura Caja",
	}

	doc, err := receipt.PDF(r)
	if err != nil {
		t.Fatalf("PDF: %v", err)
	}
	if !bytes.HasPrefix(doc, []byte("%PDF-")) {
		t.Fatalf("not a PDF: %q", doc[:8])
	}
	text := textOf(t, doc)
	for _, want := range []string{"COLEGIO GARCÍA LORCA", "P-2026-000042", "Camila Zamora Sánchez", "Colegiatura septiembre 2026", "$3,500.00", "Tarjeta de débito", "Gracias por su pago."} {
		if !strings.Contains(text, want) {
			t.Errorf("the PDF does not say %q; it reads:\n%s", want, text)
		}
	}
}

func TestAVoidedPDFSaysCancelled(t *testing.T) {
	doc, err := receipt.PDF(receipt.Receipt{Kind: receipt.KindPayment, Folio: "P-2026-000043", PaidOn: "2026-10-10", TotalCents: 100, Voided: true, VoidReason: "capturado mal"})
	if err != nil {
		t.Fatalf("PDF: %v", err)
	}
	if text := textOf(t, doc); !strings.Contains(text, "CANCELADO") {
		t.Errorf("a voided receipt does not say CANCELADO; it reads:\n%s", text)
	}
}

// Review of task 12: a long address and legend wrap on the page instead of
// running off it; every word is still there.
func TestLongTextWrapsOnThePage(t *testing.T) {
	legend := strings.Repeat("Conserve este comprobante para cualquier aclaración. ", 9) + "FIN."
	doc, err := receipt.PDF(receipt.Receipt{
		Kind: receipt.KindPayment, Folio: "P-2026-000050", PaidOn: "2026-10-10", TotalCents: 100,
		School: client.SchoolProfile{Name: "COLEGIO", Address: strings.Repeat("AV. SIEMPRE VIVA 742 ", 12), Legend: legend},
		Lines:  []receipt.Line{{Description: strings.Repeat("Colegiatura con descripción larga ", 6), AmountCents: 100}},
	})
	if err != nil {
		t.Fatalf("PDF: %v", err)
	}
	text := textOf(t, doc)
	if !strings.Contains(text, "FIN.") {
		t.Errorf("the legend's last word is missing; it reads:\n%s", text)
	}
}

// T18: the PDF says which scholarship and how much, line by line and in total.
func TestThePDFItemizesTheScholarship(t *testing.T) {
	r := receipt.Receipt{
		Kind: receipt.KindPayment, Folio: "P-2026-000046", PaidOn: "2026-09-03", Buyer: "Camila Zamora Sánchez",
		Lines: []receipt.Line{{Description: "Colegiatura septiembre 2026", AmountCents: 150000,
			ScholarshipCents: 150000, ScholarshipName: "Hermanos"}},
		ScholarshipCents: 150000, TotalCents: 150000, Method: "Efectivo",
	}
	doc, err := receipt.PDF(r)
	if err != nil {
		t.Fatalf("PDF: %v", err)
	}
	text := textOf(t, doc)
	for _, want := range []string{"Beca Hermanos", "Becas aplicadas", "$1,500.00"} {
		if !strings.Contains(text, want) {
			t.Errorf("the PDF does not say %q; it reads:\n%s", want, text)
		}
	}
}
