package email_test

import (
	"testing"

	"github.com/garcia-lorca/pkg/email"
	glerrors "github.com/garcia-lorca/pkg/errors"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name, raw, want string
	}{
		{"already normal", "ana@colegio.mx", "ana@colegio.mx"},
		{"capitals", "Ana.Ruiz@Colegio.MX", "ana.ruiz@colegio.mx"},
		{"ascii padding", "  ana@colegio.mx ", "ana@colegio.mx"},
		{"unicode padding", "\t ana@colegio.mx　", "ana@colegio.mx"},
		{"accented capital", "JOSÉ@colegio.mx", "josé@colegio.mx"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := email.Normalize(tt.raw); got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestValidateAcceptsAPlausibleAddress(t *testing.T) {
	if err := email.Validate("ana.ruiz@colegio.edu.mx"); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateRejectsImplausibleShapes(t *testing.T) {
	for _, bad := range []string{
		"",
		"ana.ruiz.gmail.com",
		"ana ruiz@colegio.mx",
		"ana@@colegio.mx",
		"@colegio.mx",
		"ana@colegio",
		"ana@colegio.",
		"ana@.mx",
	} {
		t.Run(bad, func(t *testing.T) {
			if glerrors.KindOf(email.Validate(bad)) != glerrors.KindInvalid {
				t.Errorf("Validate(%q) must be KindInvalid", bad)
			}
		})
	}
}

// Validate is exactly the rule iam enforced before it moved here: users already
// stored under it must stay readable, so it may not become stricter.
func TestValidateKeepsIamsPreviousRule(t *testing.T) {
	if err := email.Validate("x@colegio.mx."); err != nil {
		t.Fatalf("a stored iam address that passed before must still pass: %v", err)
	}
}
