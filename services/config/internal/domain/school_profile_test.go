package domain_test

import (
	"strings"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func validProfileInput() domain.SchoolProfileInput {
	return domain.SchoolProfileInput{
		Name:      "  Colegio García Lorca  ",
		LegalName: "Educación García Lorca, S.C.",
		RFC:       " egl150101ab1 ",
		CCT:       "09pPR1234x",
		Address:   "Av. Siempre Viva 742, Col. Centro, CDMX",
		Phone:     "55 1234 5678",
		Legend:    "Gracias por su pago.",
	}
}

// Task 12: what a receipt prints about the school. The name is the only thing
// a receipt cannot do without; the rest is printed when the school fills it.
func TestNewSchoolProfileNormalises(t *testing.T) {
	p, err := domain.NewSchoolProfile("t1", validProfileInput())
	if err != nil {
		t.Fatalf("NewSchoolProfile: %v", err)
	}
	if p.Name() != "Colegio García Lorca" || p.RFC() != "EGL150101AB1" || p.CCT() != "09PPR1234X" || p.Phone() != "5512345678" {
		t.Errorf("profile = %q %q %q %q; want trimmed name, upper-case RFC and CCT, digits-only phone",
			p.Name(), p.RFC(), p.CCT(), p.Phone())
	}
}

func TestNewSchoolProfileOptionalFieldsMayBeEmpty(t *testing.T) {
	p, err := domain.NewSchoolProfile("t1", domain.SchoolProfileInput{Name: "Colegio García Lorca"})
	if err != nil {
		t.Fatalf("a profile with only the name: %v", err)
	}
	if p.RFC() != "" || p.CCT() != "" || p.Phone() != "" {
		t.Errorf("empty optional fields came back %q %q %q", p.RFC(), p.CCT(), p.Phone())
	}
}

func TestNewSchoolProfileValidation(t *testing.T) {
	cases := map[string]func(*domain.SchoolProfileInput){
		"no name":                func(in *domain.SchoolProfileInput) { in.Name = "  " },
		"name too long":          func(in *domain.SchoolProfileInput) { in.Name = strings.Repeat("a", 201) },
		"RFC of the wrong shape": func(in *domain.SchoolProfileInput) { in.RFC = "EGL-150101" },
		"CCT of the wrong shape": func(in *domain.SchoolProfileInput) { in.CCT = "09PPR12" },
		"phone not ten digits":   func(in *domain.SchoolProfileInput) { in.Phone = "55 1234" },
		"address too long":       func(in *domain.SchoolProfileInput) { in.Address = strings.Repeat("a", 301) },
		"legend too long":        func(in *domain.SchoolProfileInput) { in.Legend = strings.Repeat("a", 501) },
	}
	for name, spoil := range cases {
		t.Run(name, func(t *testing.T) {
			in := validProfileInput()
			spoil(&in)
			if _, err := domain.NewSchoolProfile("t1", in); glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("err = %v, want KindInvalid", err)
			}
		})
	}
}
