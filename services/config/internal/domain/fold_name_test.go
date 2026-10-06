package domain_test

import (
	"testing"

	"github.com/garcia-lorca/services/config/internal/domain"
)

// FoldName is the Go twin of the SQL config_fold_name (migration 0019): the
// two decide together whether a level already exists, so they must agree on
// every input — including what they deliberately leave alone.
func TestFoldName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "Preparatoria", "preparatoria"},
		{"upper case", "PREPARATORIA", "preparatoria"},
		{"surrounding spaces", "  Primaria  ", "primaria"},
		{"inner runs collapse", "Educación   Básica", "educacion basica"},
		{"tabs and newlines are whitespace", "Primaria\t\n Bilingüe\r\v\f", "primaria bilingue"},
		{"lower accents", "áéíóúüñ", "aeiouun"},
		{"upper accents", "ÁÉÍÓÚÜÑ", "aeiouun"},
		{"empty", "", ""},
		{"only whitespace", " \t ", ""},
		{"digits and symbols kept", "1° Kínder-B", "1° kinder-b"},
		// Outside the fold, exactly as the SQL leaves them.
		{"other accents untouched", "Àà Çç", "Àà Çç"},
		{"no-break space is not whitespace", "a b", "a b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.FoldName(tt.in); got != tt.want {
				t.Errorf("FoldName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
