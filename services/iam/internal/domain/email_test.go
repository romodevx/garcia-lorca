package domain_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
)

func TestNewEmailNormalizes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"lowercases", "User@Example.com", "user@example.com"},
		{"trims surrounding space", "  ana@colegio.mx  ", "ana@colegio.mx"},
		{"already normal", "director@garcialorca.mx", "director@garcialorca.mx"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.NewEmail(tt.in)
			if err != nil {
				t.Fatalf("NewEmail(%q) unexpected error: %v", tt.in, err)
			}
			if got.String() != tt.want {
				t.Errorf("NewEmail(%q) = %q, want %q", tt.in, got.String(), tt.want)
			}
		})
	}
}

func TestNewEmailRejectsInvalid(t *testing.T) {
	invalid := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
		{"no at sign", "anacolegio.mx"},
		{"no local part", "@colegio.mx"},
		{"no domain", "ana@"},
		{"internal space", "an a@colegio.mx"},
		{"no dot in domain", "ana@colegio"},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewEmail(tt.in)
			if err == nil {
				t.Fatalf("NewEmail(%q) = nil error, want invalid", tt.in)
			}
			if got := glerrors.KindOf(err); got != glerrors.KindInvalid {
				t.Errorf("NewEmail(%q) kind = %v, want KindInvalid", tt.in, got)
			}
		})
	}
}

func TestEmailEquality(t *testing.T) {
	a, _ := domain.NewEmail("Ana@Colegio.MX")
	b, _ := domain.NewEmail("ana@colegio.mx")
	if a != b {
		t.Errorf("normalized emails should be equal as values: %q != %q", a, b)
	}
}
