package domain

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/garcia-lorca/pkg/email"
	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Sex is the pupil's sex as the school records it. Empty means not captured.
type Sex string

// The sexes a pupil's file may record.
const (
	SexUnspecified Sex = ""
	SexFemale      Sex = "F"
	SexMale        Sex = "M"
	SexOther       Sex = "X"
)

// PhoneKind says which phone it is: the one that answers during school hours
// is rarely the house line.
type PhoneKind string

// The phone kinds a file may record.
const (
	PhoneMobile PhoneKind = "celular"
	PhoneHome   PhoneKind = "casa"
	PhoneWork   PhoneKind = "trabajo"
	PhoneOther  PhoneKind = "otro"
)

// Phone is one number the school can call about the pupil, stored as its ten
// digits alone.
type Phone struct {
	Kind   PhoneKind
	Number string
}

// Address is where the pupil lives. Every field is optional.
type Address struct {
	Street       string
	ExtNumber    string
	IntNumber    string
	Neighborhood string
	PostalCode   string
	Municipality string
	State        string
}

// Profile is the part of a pupil's file the office keeps up to date by hand:
// birth date, sex, the pupil's own email and phones, and address. Every field is
// optional — the office fills what it has — but what is filled must be well
// formed.
type Profile struct {
	birthDate time.Time
	sex       Sex
	email     string
	phones    []Phone
	address   Address
}

// ProfileInput is a profile as captured, before validation. BirthDate is an ISO
// date ("2019-03-12") or empty.
type ProfileInput struct {
	BirthDate string
	Sex       string
	Email     string
	Phones    []Phone
	Address   Address
}

// MaxProfilePhones is the most numbers a pupil's file keeps.
const MaxProfilePhones = 3

const (
	phoneDigits     = 10
	postalCodeLen   = 5
	maxAddressField = 120
	isoDate         = "2006-01-02"
)

// mexicanStates are the 32 federal entities, as a file writes them.
var mexicanStates = map[string]bool{
	"Aguascalientes": true, "Baja California": true, "Baja California Sur": true, "Campeche": true,
	"Chiapas": true, "Chihuahua": true, "Ciudad de México": true, "Coahuila": true, "Colima": true,
	"Durango": true, "Estado de México": true, "Guanajuato": true, "Guerrero": true, "Hidalgo": true,
	"Jalisco": true, "Michoacán": true, "Morelos": true, "Nayarit": true, "Nuevo León": true,
	"Oaxaca": true, "Puebla": true, "Querétaro": true, "Quintana Roo": true, "San Luis Potosí": true,
	"Sinaloa": true, "Sonora": true, "Tabasco": true, "Tamaulipas": true, "Tlaxcala": true,
	"Veracruz": true, "Yucatán": true, "Zacatecas": true,
}

var phoneKinds = map[PhoneKind]bool{PhoneMobile: true, PhoneHome: true, PhoneWork: true, PhoneOther: true}

// NewProfile validates and normalizes a captured profile. `today` is the
// school's current date: a birth date after it is a typo, not a pupil.
func NewProfile(in ProfileInput, today time.Time) (Profile, error) {
	var p Profile
	if raw := strings.TrimSpace(in.BirthDate); raw != "" {
		born, err := time.Parse(isoDate, raw)
		if err != nil {
			return Profile{}, glerrors.New(glerrors.KindInvalid, "birth date must be YYYY-MM-DD")
		}
		if born.After(today) {
			return Profile{}, glerrors.New(glerrors.KindInvalid, "birth date cannot be in the future")
		}
		p.birthDate = born
	}
	switch sex := Sex(strings.ToUpper(strings.TrimSpace(in.Sex))); sex {
	case SexUnspecified, SexFemale, SexMale, SexOther:
		p.sex = sex
	default:
		return Profile{}, glerrors.New(glerrors.KindInvalid, "sex must be F, M or X")
	}
	if address := email.Normalize(in.Email); address != "" {
		if err := email.Validate(address); err != nil {
			return Profile{}, err
		}
		p.email = address
	}
	phones, err := normalizePhones(in.Phones)
	if err != nil {
		return Profile{}, err
	}
	p.phones = phones
	address, err := normalizeAddress(in.Address)
	if err != nil {
		return Profile{}, err
	}
	p.address = address
	return p, nil
}

// RestoreProfile rebuilds a stored profile without judging it: it was valid
// when written, and a rule tightened since must not make the file unreadable.
func RestoreProfile(birthDate time.Time, sex Sex, email string, phones []Phone, address Address) Profile {
	return Profile{birthDate: birthDate, sex: sex, email: email, phones: phones, address: address}
}

func normalizePhones(in []Phone) ([]Phone, error) {
	if len(in) > MaxProfilePhones {
		return nil, glerrors.New(glerrors.KindInvalid, "a file keeps at most three phones")
	}
	out := make([]Phone, 0, len(in))
	for _, ph := range in {
		kind := PhoneKind(strings.ToLower(strings.TrimSpace(string(ph.Kind))))
		if !phoneKinds[kind] {
			return nil, glerrors.New(glerrors.KindInvalid, "phone kind must be celular, casa, trabajo or otro")
		}
		digits := onlyDigits(ph.Number)
		if len(digits) != phoneDigits {
			return nil, glerrors.New(glerrors.KindInvalid, "a phone has ten digits")
		}
		out = append(out, Phone{Kind: kind, Number: digits})
	}
	return out, nil
}

func normalizeAddress(a Address) (Address, error) {
	out := Address{
		Street:       strings.TrimSpace(a.Street),
		ExtNumber:    strings.TrimSpace(a.ExtNumber),
		IntNumber:    strings.TrimSpace(a.IntNumber),
		Neighborhood: strings.TrimSpace(a.Neighborhood),
		PostalCode:   strings.TrimSpace(a.PostalCode),
		Municipality: strings.TrimSpace(a.Municipality),
		State:        strings.TrimSpace(a.State),
	}
	for _, field := range []string{out.Street, out.ExtNumber, out.IntNumber, out.Neighborhood, out.Municipality} {
		if utf8.RuneCountInString(field) > maxAddressField {
			return Address{}, glerrors.New(glerrors.KindInvalid, "an address field is too long")
		}
	}
	if out.PostalCode != "" && (len(out.PostalCode) != postalCodeLen || onlyDigits(out.PostalCode) != out.PostalCode) {
		return Address{}, glerrors.New(glerrors.KindInvalid, "a postal code has five digits")
	}
	if out.State != "" && !mexicanStates[out.State] {
		return Address{}, glerrors.New(glerrors.KindInvalid, "state must be one of the 32 federal entities")
	}
	return out, nil
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// BirthDate is the pupil's birth date; the zero time means not captured.
func (p Profile) BirthDate() time.Time { return p.birthDate }

// Sex is the recorded sex; empty means not captured.
func (p Profile) Sex() Sex { return p.sex }

// Email is the pupil's own address, trimmed and lowercase; empty if none.
func (p Profile) Email() string { return p.email }

// Phones are the pupil's numbers, ten digits each.
func (p Profile) Phones() []Phone { return append([]Phone(nil), p.phones...) }

// Address is where the pupil lives.
func (p Profile) Address() Address { return p.address }
