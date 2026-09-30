package domain_test

import (
	"testing"

	"github.com/garcia-lorca/services/finance/internal/domain"
)

// A line's identity must survive what a person does to its spelling: case,
// accents, spacing and punctuation. It is what tells the billing run that
// "INSCRIPCION" is the "Inscripción" a family already owes.
func TestLineKeyIgnoresCaseAccentsAndPunctuation(t *testing.T) {
	cases := []struct {
		a, b string
	}{
		{"Inscripción", "INSCRIPCION"},
		{"Inscripción", "inscripcion"},
		{"Graduación", "GRADUACION"},
		{"Cuota A.P.F.", "cuota apf"},
		{"Año escolar", "ANO ESCOLAR"},
		{"  Pingüino ", "PINGUINO"},
	}
	for _, c := range cases {
		if domain.LineKey(c.a) != domain.LineKey(c.b) {
			t.Errorf("LineKey(%q)=%q, LineKey(%q)=%q: want one identity",
				c.a, domain.LineKey(c.a), c.b, domain.LineKey(c.b))
		}
	}
}

func TestLineKeyKeepsDifferentNamesApart(t *testing.T) {
	cases := [][2]string{
		{"Inscripción", "Inscripciones"},
		{"APF", "Gastos administrativos"},
		{"Cuota 1", "Cuota 2"},
	}
	for _, c := range cases {
		if domain.LineKey(c[0]) == domain.LineKey(c[1]) {
			t.Errorf("%q and %q reduce to one key %q", c[0], c[1], domain.LineKey(c[0]))
		}
	}
}

func TestLineKeyReadsAsTheNameInPlainLetters(t *testing.T) {
	if got := domain.LineKey("Inscripción"); got != "inscripcion" {
		t.Errorf("LineKey(Inscripción) = %q, want %q", got, "inscripcion")
	}
}
