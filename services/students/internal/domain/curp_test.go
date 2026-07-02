package domain_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

func TestNewCURPNormalizesAndValidates(t *testing.T) {
	// A structurally valid CURP (18 chars, standard shape).
	got, err := domain.NewCURP("  hega050312mdfrnn08 ")
	if err != nil {
		t.Fatalf("NewCURP valid: %v", err)
	}
	if got.String() != "HEGA050312MDFRNN08" {
		t.Errorf("String() = %q, want uppercased+trimmed", got.String())
	}
}

func TestNewCURPRejectsMalformed(t *testing.T) {
	for _, in := range []string{
		"",                    // empty
		"ABC123",              // too short
		"HEGA050312MDFRNN0",   // 17 chars
		"HEGA050312MDFRNN088", // 19 chars
		"1EGA050312MDFRNN08",  // must start with a letter
		"HEGA059912MDFRNN08",  // invalid month 99
		"HEGA050332MDFRNN08",  // invalid day 32
		"HEGA050312XDFRNN08",  // sex must be H or M
	} {
		if _, err := domain.NewCURP(in); err == nil || glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Errorf("NewCURP(%q): want KindInvalid, got %v", in, err)
		}
	}
}

func TestCURPEqualityByValue(t *testing.T) {
	a, _ := domain.NewCURP("hega050312mdfrnn08")
	b, _ := domain.NewCURP("HEGA050312MDFRNN08")
	if a != b {
		t.Errorf("normalized CURPs should be equal: %q != %q", a, b)
	}
}
