package domain

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Limits on what a person types into the school's details, in characters.
const (
	maxProfileNameLen    = 200
	maxProfileAddressLen = 300
	maxProfileLegendLen  = 500
	profilePhoneDigits   = 10
)

var (
	// rfcShape is the SAT's: three letters (a company) or four (a person), the
	// date as six digits, and a three-character homoclave.
	rfcShape = regexp.MustCompile(`^[A-ZÑ&]{3,4}[0-9]{6}[A-Z0-9]{3}$`)
	// cctShape is the SEP's school key: two digits of the state, three letters of
	// the kind of school, four digits and a check letter.
	cctShape = regexp.MustCompile(`^[0-9]{2}[A-Z]{3}[0-9]{4}[A-Z]$`)
)

// SchoolProfileInput is the school's details as typed.
type SchoolProfileInput struct {
	Name      string
	LegalName string
	RFC       string
	CCT       string
	Address   string
	Phone     string
	Legend    string
	CLABE     string
}

// SchoolProfile is what a receipt prints about the school (task 12), and the
// CLABE the family app shows for transfers: one per school. The name is the only field a receipt cannot do without; the rest is
// printed when the school has filled it in.
type SchoolProfile struct {
	tenantID  string
	name      string
	legalName string
	rfc       string
	cct       string
	address   string
	phone     string
	legend    string
	clabe     string
}

// NewSchoolProfile validates and normalises the school's details: trimmed text,
// RFC and CCT in upper case, the phone as its ten digits. Every failure is
// KindInvalid.
func NewSchoolProfile(tenantID string, in SchoolProfileInput) (SchoolProfile, error) {
	if strings.TrimSpace(tenantID) == "" {
		return SchoolProfile{}, glerrors.New(glerrors.KindInvalid, "tenant id is required")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > maxProfileNameLen {
		return SchoolProfile{}, glerrors.New(glerrors.KindInvalid, "the school's name is required (up to 200 characters)")
	}
	legalName := strings.TrimSpace(in.LegalName)
	if utf8.RuneCountInString(legalName) > maxProfileNameLen {
		return SchoolProfile{}, glerrors.New(glerrors.KindInvalid, "the legal name is too long")
	}
	rfc, err := optionalShape(in.RFC, rfcShape, "the RFC does not have the SAT's shape")
	if err != nil {
		return SchoolProfile{}, err
	}
	cct, err := optionalShape(in.CCT, cctShape, "the CCT does not have the SEP's shape")
	if err != nil {
		return SchoolProfile{}, err
	}
	address := strings.TrimSpace(in.Address)
	if utf8.RuneCountInString(address) > maxProfileAddressLen {
		return SchoolProfile{}, glerrors.New(glerrors.KindInvalid, "the address is too long")
	}
	phone := digitsOf(in.Phone)
	if phone != "" && len(phone) != profilePhoneDigits {
		return SchoolProfile{}, glerrors.New(glerrors.KindInvalid, "the phone must have ten digits")
	}
	legend := strings.TrimSpace(in.Legend)
	if utf8.RuneCountInString(legend) > maxProfileLegendLen {
		return SchoolProfile{}, glerrors.New(glerrors.KindInvalid, "the legend is too long")
	}
	clabe, err := normaliseCLABE(in.CLABE)
	if err != nil {
		return SchoolProfile{}, err
	}
	return SchoolProfile{
		tenantID: tenantID, name: name, legalName: legalName, rfc: rfc, cct: cct,
		address: address, phone: phone, legend: legend, clabe: clabe,
	}, nil
}

// optionalShape upper-cases a value and checks its shape; empty is allowed.
func optionalShape(raw string, shape *regexp.Regexp, refusal string) (string, error) {
	value := strings.ToUpper(strings.TrimSpace(raw))
	if value != "" && !shape.MatchString(value) {
		return "", glerrors.New(glerrors.KindInvalid, refusal)
	}
	return value, nil
}

// digitsOf keeps the digits of a typed phone ("55 1234-5678" → "5512345678").
func digitsOf(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Accessors.
func (p SchoolProfile) TenantID() string  { return p.tenantID }
func (p SchoolProfile) Name() string      { return p.name }
func (p SchoolProfile) LegalName() string { return p.legalName }
func (p SchoolProfile) RFC() string       { return p.rfc }
func (p SchoolProfile) CCT() string       { return p.cct }
func (p SchoolProfile) Address() string   { return p.address }
func (p SchoolProfile) Phone() string     { return p.phone }
func (p SchoolProfile) Legend() string    { return p.legend }
func (p SchoolProfile) CLABE() string     { return p.clabe }
