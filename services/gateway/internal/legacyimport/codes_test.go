package legacyimport

import "testing"

// The draft's CURP codes are students' (domain.IssueCURPMissing/Invalid,
// migration 0022); the preview shows them before students stores them.
func TestDraftCURPCodesAreStudents(t *testing.T) {
	if CodeCURPMissing != "curp_missing" || CodeCURPInvalid != "curp_invalid" {
		t.Errorf("codes = %q %q", CodeCURPMissing, CodeCURPInvalid)
	}
	if DraftCURPCode("  ") != CodeCURPMissing || DraftCURPCode("ABC") != CodeCURPInvalid {
		t.Errorf("DraftCURPCode is wrong")
	}
}

func TestOverrideNamesAreTheContracts(t *testing.T) {
	for _, name := range []string{"curp", "birthDate", "homePhone", "mobilePhone", "email", "postalCode",
		"state", "bankReference", "municipality", "street", "fullName"} {
		if !KnownOverride(name) {
			t.Errorf("%q is not a known override", name)
		}
	}
	if KnownOverride("phone") {
		t.Errorf("phone is not an override")
	}
}

// Canonical is what the office approved, normalized: two spellings of the
// same correction are one.
func TestOverridesCanonicalNormalizes(t *testing.T) {
	a := Overrides{MobilePhone: ptr("33 1111 2222"), BirthDate: ptr("2021-02-19")}.Canonical()
	b := Overrides{MobilePhone: ptr("3311112222"), BirthDate: ptr("19/02/2021")}.Canonical()
	if a != b || a == "" {
		t.Errorf("canonical %q vs %q", a, b)
	}
	if (Overrides{}).Canonical() != "" {
		t.Errorf("no overrides must be empty")
	}
}
