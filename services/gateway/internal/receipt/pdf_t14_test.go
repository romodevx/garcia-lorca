package receipt_test

import (
	"strings"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/receipt"
)

// T14-012 and T14-020 on paper: the day with its month in words, and what an
// abono leaves pending.
func TestThePDFSaysTheDayInWordsAndWhatIsPending(t *testing.T) {
	doc, err := receipt.PDF(receipt.Receipt{
		Kind: receipt.KindPayment, Folio: "P-2026-000002", PaidOn: "2026-10-17", TotalCents: 300000, PendingCents: 100000,
		Lines: []receipt.Line{{Description: "Inscripción", AmountCents: 300000}},
	})
	if err != nil {
		t.Fatalf("PDF: %v", err)
	}
	text := textOf(t, doc)
	for _, want := range []string{"Fecha de pago: 17 oct 2026", "Saldo pendiente", "$1,000.00"} {
		if !strings.Contains(text, want) {
			t.Errorf("the PDF does not say %q; it reads:\n%s", want, text)
		}
	}
	if strings.Contains(text, "2026-10-17") {
		t.Errorf("the PDF still prints the ISO day; it reads:\n%s", text)
	}
}

func TestDayLabel(t *testing.T) {
	cases := map[string]string{"2026-10-17": "17 oct 2026", "2027-01-06": "6 ene 2027", "": "", "mañana": "mañana"}
	for in, want := range cases {
		if got := receipt.DayLabel(in); got != want {
			t.Errorf("DayLabel(%q) = %q, want %q", in, got, want)
		}
	}
}
