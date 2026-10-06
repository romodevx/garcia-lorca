package domain_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

func TestNewBankReference(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		invalid bool
	}{
		{name: "digits", raw: "0012345", want: "0012345"},
		{name: "trimmed", raw: "  123  ", want: "123"},
		{name: "inner spaces removed", raw: "12 34 56", want: "123456"},
		{name: "one digit", raw: "7", want: "7"},
		{name: "twenty digits", raw: "12345678901234567890", want: "12345678901234567890"},
		{name: "empty is no reference", raw: "", want: ""},
		{name: "blank is no reference", raw: "   ", want: ""},
		{name: "twenty-one digits", raw: "123456789012345678901", invalid: true},
		{name: "S/R marker", raw: "S/R", invalid: true},
		{name: "letters", raw: "12A4", invalid: true},
		{name: "dash", raw: "12-34", invalid: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := domain.NewBankReference(tc.raw)
			if tc.invalid {
				if glerrors.KindOf(err) != glerrors.KindInvalid {
					t.Fatalf("NewBankReference(%q) err = %v; want KindInvalid", tc.raw, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewBankReference(%q): %v", tc.raw, err)
			}
			if got.String() != tc.want {
				t.Errorf("NewBankReference(%q) = %q; want %q", tc.raw, got, tc.want)
			}
			if got.IsZero() != (tc.want == "") {
				t.Errorf("IsZero = %v for %q", got.IsZero(), got)
			}
		})
	}
}

func TestStudentCarriesItsBankReference(t *testing.T) {
	ref, _ := domain.NewBankReference("98765")
	curp, _ := domain.NewCURP("HEGA050312MDFRNN08")
	s, err := domain.NewApplicant(domain.NewApplicantParams{
		ID: "s1", TenantID: "t1", CURP: curp, FirstName: "Ana", LastName: "H",
		Level: primaria(t), BankReference: ref,
	})
	if err != nil {
		t.Fatalf("NewApplicant: %v", err)
	}
	if s.BankReference() != ref {
		t.Fatalf("BankReference = %q; want %q", s.BankReference(), ref)
	}
	s.ChangeBankReference("")
	if !s.BankReference().IsZero() {
		t.Fatalf("cleared reference = %q; want none", s.BankReference())
	}
	restored := domain.Rehydrate(domain.NewApplicantParams{ID: "s2", BankReference: ref}, domain.AdmissionPending)
	if restored.BankReference() != ref {
		t.Fatalf("Rehydrate lost the reference: %q", restored.BankReference())
	}
}

func TestNewImportedWalksTheMachineToEnrolled(t *testing.T) {
	curp, _ := domain.NewCURP("HEGA050312MDFRNN08")
	s, err := domain.NewImported(domain.NewApplicantParams{
		ID: "s1", TenantID: "t1", CURP: curp, FirstName: "Ana", LastName: "H", Level: primaria(t),
	})
	if err != nil {
		t.Fatalf("NewImported: %v", err)
	}
	if s.Status() != domain.AdmissionEnrolled {
		t.Fatalf("status = %v; want enrolled", s.Status())
	}
	if _, err := domain.NewImported(domain.NewApplicantParams{ID: "s2", TenantID: "t1", CURP: curp, Level: primaria(t)}); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("an import without names: err = %v; want KindInvalid", err)
	}
}
