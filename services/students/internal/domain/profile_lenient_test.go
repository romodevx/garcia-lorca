package domain_test

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/garcia-lorca/services/students/internal/domain"
)

func TestNewProfileLenientKeepsAValidProfileWhole(t *testing.T) {
	p, warnings := domain.NewProfileLenient(validProfile(), today)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v; want none", warnings)
	}
	strict, err := domain.NewProfile(validProfile(), today)
	if err != nil {
		t.Fatalf("NewProfile: %v", err)
	}
	if !reflect.DeepEqual(p, strict) {
		t.Fatalf("lenient = %+v; strict = %+v", p, strict)
	}
}

func TestNewProfileLenientDropsEachBadSubFieldAlone(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*domain.ProfileInput)
		warning string
		check   func(*testing.T, domain.Profile)
	}{
		{
			name: "birth date shape", warning: domain.WarnBirthDateInvalid,
			mutate: func(p *domain.ProfileInput) { p.BirthDate = "12/03/2019" },
			check: func(t *testing.T, p domain.Profile) {
				if !p.BirthDate().IsZero() {
					t.Errorf("birth date kept: %v", p.BirthDate())
				}
			},
		},
		{
			name: "future birth date", warning: domain.WarnBirthDateInvalid,
			mutate: func(p *domain.ProfileInput) { p.BirthDate = "2027-01-01" },
			check: func(t *testing.T, p domain.Profile) {
				if !p.BirthDate().IsZero() {
					t.Errorf("birth date kept: %v", p.BirthDate())
				}
			},
		},
		{
			name: "email", warning: domain.WarnEmailInvalid,
			mutate: func(p *domain.ProfileInput) { p.Email = "ana@correo" },
			check: func(t *testing.T, p domain.Profile) {
				if p.Email() != "" {
					t.Errorf("email kept: %q", p.Email())
				}
			},
		},
		{
			name: "phone", warning: domain.WarnPhoneInvalid,
			mutate: func(p *domain.ProfileInput) {
				p.Phones = []domain.Phone{{Kind: "casa", Number: "12345"}, {Kind: "celular", Number: "5512345678"}}
			},
			check: func(t *testing.T, p domain.Profile) {
				if got := p.Phones(); len(got) != 1 || got[0].Number != "5512345678" {
					t.Errorf("phones = %+v; want only the valid one", got)
				}
			},
		},
		{
			name: "postal code", warning: domain.WarnPostalCodeInvalid,
			mutate: func(p *domain.ProfileInput) { p.Address.PostalCode = "4410" },
			check: func(t *testing.T, p domain.Profile) {
				if a := p.Address(); a.PostalCode != "" || a.Street != "Av. Juárez" || a.State != "Jalisco" {
					t.Errorf("address = %+v; want only the postal code dropped", a)
				}
			},
		},
		{
			name: "unknown state", warning: domain.WarnStateUnknown,
			mutate: func(p *domain.ProfileInput) { p.Address.State = "Atlántida" },
			check: func(t *testing.T, p domain.Profile) {
				if a := p.Address(); a.State != "" || a.PostalCode != "44100" {
					t.Errorf("address = %+v; want only the state dropped", a)
				}
			},
		},
		{
			name: "long street", warning: domain.WarnAddressTruncated,
			mutate: func(p *domain.ProfileInput) { p.Address.Street = strings.Repeat("ñ", 130) },
			check: func(t *testing.T, p domain.Profile) {
				if n := utf8.RuneCountInString(p.Address().Street); n != 120 {
					t.Errorf("street runes = %d; want 120", n)
				}
			},
		},
		{
			name: "long municipality", warning: domain.WarnAddressTruncated,
			mutate: func(p *domain.ProfileInput) { p.Address.Municipality = strings.Repeat("á", 121) },
			check: func(t *testing.T, p domain.Profile) {
				if n := utf8.RuneCountInString(p.Address().Municipality); n != 120 {
					t.Errorf("municipality runes = %d; want 120", n)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validProfile()
			tc.mutate(&in)
			p, warnings := domain.NewProfileLenient(in, today)
			if !reflect.DeepEqual(warnings, []string{tc.warning}) {
				t.Fatalf("warnings = %v; want [%s]", warnings, tc.warning)
			}
			tc.check(t, p)
			if p.Sex() != domain.SexFemale {
				t.Errorf("an unrelated field was lost: sex = %q", p.Sex())
			}
		})
	}
}

func TestNewProfileLenientNormalizesPhones(t *testing.T) {
	cases := []struct {
		name     string
		phones   []domain.Phone
		want     []domain.Phone
		warnings []string
	}{
		{
			name:   "country code 52",
			phones: []domain.Phone{{Kind: "celular", Number: "+52 55 1234 5678"}},
			want:   []domain.Phone{{Kind: domain.PhoneMobile, Number: "5512345678"}},
		},
		{
			name:   "mobile prefix 521",
			phones: []domain.Phone{{Kind: "celular", Number: "521 (443) 123-4567"}},
			want:   []domain.Phone{{Kind: domain.PhoneMobile, Number: "4431234567"}},
		},
		{
			name:   "044 and 045",
			phones: []domain.Phone{{Kind: "celular", Number: "044 55 1234 5678"}, {Kind: "casa", Number: "045 33 1234 5678"}},
			want:   []domain.Phone{{Kind: domain.PhoneMobile, Number: "5512345678"}, {Kind: domain.PhoneHome, Number: "3312345678"}},
		},
		{
			name:   "same number twice is kept once",
			phones: []domain.Phone{{Kind: "casa", Number: "4431234567"}, {Kind: "celular", Number: "443-123-4567"}},
			want:   []domain.Phone{{Kind: domain.PhoneHome, Number: "4431234567"}},
		},
		{
			name:     "two bad numbers warn once",
			phones:   []domain.Phone{{Kind: "casa", Number: "123"}, {Kind: "celular", Number: "sin teléfono"}},
			want:     nil,
			warnings: []string{domain.WarnPhoneInvalid},
		},
		{
			name:   "blank number is no phone, not a bad one",
			phones: []domain.Phone{{Kind: "casa", Number: "  "}},
			want:   nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, warnings := domain.NewProfileLenient(domain.ProfileInput{Phones: tc.phones}, today)
			if !reflect.DeepEqual(p.Phones(), tc.want) {
				t.Errorf("phones = %+v; want %+v", p.Phones(), tc.want)
			}
			if len(warnings) != len(tc.warnings) || (len(warnings) > 0 && !reflect.DeepEqual(warnings, tc.warnings)) {
				t.Errorf("warnings = %v; want %v", warnings, tc.warnings)
			}
		})
	}
}

func TestNewProfileLenientMatchesStatesLikeAPerson(t *testing.T) {
	cases := map[string]string{
		"JALISCO":                         "Jalisco",
		"michoacán":                       "Michoacán",
		"MICHOACAN DE OCAMPO":             "Michoacán",
		"CDMX":                            "Ciudad de México",
		"DISTRITO FEDERAL":                "Ciudad de México",
		"CIUDAD DE MEXICO":                "Ciudad de México",
		"MEXICO":                          "Estado de México",
		"EDO MEX":                         "Estado de México",
		"Edo. de México":                  "Estado de México",
		"ESTADO DE MEXICO":                "Estado de México",
		"VERACRUZ DE IGNACIO DE LA LLAVE": "Veracruz",
		"COAHUILA DE ZARAGOZA":            "Coahuila",
		"QUERETARO DE ARTEAGA":            "Querétaro",
		"  nuevo   leon ":                 "Nuevo León",
		"SAN LUIS POTOSI":                 "San Luis Potosí",
		"YUCATAN":                         "Yucatán",
	}
	for raw, want := range cases {
		t.Run(raw, func(t *testing.T) {
			p, warnings := domain.NewProfileLenient(domain.ProfileInput{Address: domain.Address{State: raw}}, today)
			if len(warnings) != 0 || p.Address().State != want {
				t.Errorf("state %q = %q (warnings %v); want %q", raw, p.Address().State, warnings, want)
			}
		})
	}
}

func TestNewProfileLenientCollectsEveryWarningOnce(t *testing.T) {
	in := domain.ProfileInput{
		BirthDate: "nope",
		Email:     "x@",
		Phones:    []domain.Phone{{Kind: "casa", Number: "1"}},
		Address:   domain.Address{PostalCode: "abc", State: "Narnia", Street: strings.Repeat("x", 200)},
	}
	_, warnings := domain.NewProfileLenient(in, today)
	want := []string{
		domain.WarnBirthDateInvalid, domain.WarnEmailInvalid, domain.WarnPhoneInvalid,
		domain.WarnAddressTruncated, domain.WarnPostalCodeInvalid, domain.WarnStateUnknown,
	}
	if !reflect.DeepEqual(warnings, want) {
		t.Fatalf("warnings = %v; want %v", warnings, want)
	}
}
