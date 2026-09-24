package domain_test

import (
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

var today = time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)

func validProfile() domain.ProfileInput {
	return domain.ProfileInput{
		BirthDate: "2019-03-12",
		Sex:       "F",
		Email:     " Ana.Cruz@Correo.MX ",
		Phones:    []domain.Phone{{Kind: "celular", Number: "55 1234-5678"}},
		Address: domain.Address{
			Street: "Av. Juárez", ExtNumber: "123", IntNumber: "4B", Neighborhood: "Centro",
			PostalCode: "44100", Municipality: "Guadalajara", State: "Jalisco",
		},
	}
}

func TestNewProfileNormalizesWhatItKeeps(t *testing.T) {
	p, err := domain.NewProfile(validProfile(), today)
	if err != nil {
		t.Fatalf("NewProfile: %v", err)
	}
	if p.Email() != "ana.cruz@correo.mx" {
		t.Errorf("email = %q", p.Email())
	}
	if got := p.Phones(); len(got) != 1 || got[0].Number != "5512345678" {
		t.Errorf("phones = %+v, want the ten digits alone", got)
	}
	if p.BirthDate().Format("2006-01-02") != "2019-03-12" || p.Sex() != domain.SexFemale {
		t.Errorf("birth/sex = %v %q", p.BirthDate(), p.Sex())
	}
}

func TestAnEmptyProfileIsValid(t *testing.T) {
	// Every field is optional: the office fills what it has.
	if _, err := domain.NewProfile(domain.ProfileInput{}, today); err != nil {
		t.Fatalf("empty profile: %v", err)
	}
}

func TestNewProfileRejectsMalformedFields(t *testing.T) {
	cases := map[string]func(*domain.ProfileInput){
		"future birth date":  func(p *domain.ProfileInput) { p.BirthDate = "2027-01-01" },
		"birth date shape":   func(p *domain.ProfileInput) { p.BirthDate = "12/03/2019" },
		"unknown sex":        func(p *domain.ProfileInput) { p.Sex = "Q" },
		"malformed email":    func(p *domain.ProfileInput) { p.Email = "ana@correo" },
		"short phone":        func(p *domain.ProfileInput) { p.Phones = []domain.Phone{{Kind: "celular", Number: "12345"}} },
		"unknown phone kind": func(p *domain.ProfileInput) { p.Phones = []domain.Phone{{Kind: "fax", Number: "5512345678"}} },
		"four phones": func(p *domain.ProfileInput) {
			p.Phones = []domain.Phone{{"celular", "5512345678"}, {"casa", "5512345679"}, {"trabajo", "5512345670"}, {"otro", "5512345671"}}
		},
		"postal code shape": func(p *domain.ProfileInput) { p.Address.PostalCode = "4410" },
		"unknown state":     func(p *domain.ProfileInput) { p.Address.State = "Texas" },
		"street too long":   func(p *domain.ProfileInput) { p.Address.Street = strings.Repeat("a", 121) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := validProfile()
			mutate(&in)
			if _, err := domain.NewProfile(in, today); glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("want KindInvalid, got %v", err)
			}
		})
	}
}

func TestUpdateDetailsCorrectsNamesCURPAndProfile(t *testing.T) {
	s := newApplicant(t)
	curp, _ := domain.NewCURP("HEGA050312MDFRNN09")
	profile, _ := domain.NewProfile(validProfile(), today)
	if err := s.UpdateDetails(" Ana María ", " Hernández ", curp, profile); err != nil {
		t.Fatalf("UpdateDetails: %v", err)
	}
	if s.FirstName() != "Ana María" || s.LastName() != "Hernández" || s.CURP().String() != "HEGA050312MDFRNN09" {
		t.Errorf("identity = %q %q %q", s.FirstName(), s.LastName(), s.CURP())
	}
	if s.Profile().Email() != "ana.cruz@correo.mx" {
		t.Errorf("profile not attached")
	}
	if err := s.UpdateDetails(" ", "X", curp, profile); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("a blank first name must be refused, got %v", err)
	}
}

// The limit counts letters, not bytes: an accented colonia is two bytes per
// accent and must not be refused under 120 letters.
func TestAddressLimitCountsCharacters(t *testing.T) {
	in := validProfile()
	in.Address.Neighborhood = strings.Repeat("ñ", 120)
	if _, err := domain.NewProfile(in, today); err != nil {
		t.Errorf("120 accented letters refused: %v", err)
	}
}
