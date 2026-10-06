package domain

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/garcia-lorca/pkg/email"
)

// Warning codes NewProfileLenient reports. They are a contract: the import's
// report words them for the school (students.v1 ImportRowResult.warnings).
const (
	WarnPhoneInvalid      = "phone_invalid"
	WarnEmailInvalid      = "email_invalid"
	WarnBirthDateInvalid  = "birth_date_invalid"
	WarnStateUnknown      = "state_unknown"
	WarnPostalCodeInvalid = "postal_code_invalid"
	WarnAddressTruncated  = "address_truncated"
)

// phonePrefixes are what a previous system left before the ten digits: the
// country code (52), the old mobile marker after it (521) and the long-gone
// mobile dialing prefixes (044 local, 045 long distance). Longest first.
var phonePrefixes = []string{"521", "044", "045", "52"}

// NewProfileLenient builds a profile from data the school did not type here (the
// legacy import, ADR 0033). Where NewProfile refuses the whole file for one bad
// field, this drops only that sub-field and reports a warning code, so one
// malformed phone never costs the pupil their address. Each code is reported
// once, in the order the fields are read. An unreadable sex is dropped without a
// code: the import derives it from the CURP, so it is never the school's typo.
func NewProfileLenient(in ProfileInput, today time.Time) (Profile, []string) {
	w := &warnings{}
	var p Profile
	p.birthDate = lenientBirthDate(in.BirthDate, today, w)
	switch sex := Sex(strings.ToUpper(strings.TrimSpace(in.Sex))); sex {
	case SexFemale, SexMale, SexOther:
		p.sex = sex
	}
	p.email = lenientEmail(in.Email, w)
	p.phones = lenientPhones(in.Phones, w)
	p.address = lenientAddress(in.Address, w)
	return p, w.codes
}

// warnings collects codes once each, in first-seen order.
type warnings struct{ codes []string }

func (w *warnings) add(code string) {
	for _, c := range w.codes {
		if c == code {
			return
		}
	}
	w.codes = append(w.codes, code)
}

func lenientBirthDate(raw string, today time.Time, w *warnings) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	born, err := time.Parse(isoDate, raw)
	if err != nil || born.After(today) {
		w.add(WarnBirthDateInvalid)
		return time.Time{}
	}
	return born
}

func lenientEmail(raw string, w *warnings) string {
	address := email.Normalize(raw)
	if address == "" {
		return ""
	}
	if err := email.Validate(address); err != nil {
		w.add(WarnEmailInvalid)
		return ""
	}
	return address
}

// lenientPhones keeps the numbers that reduce to ten digits, once each, at most
// maxPhones. An unknown kind becomes "otro": the number is what matters.
func lenientPhones(in []Phone, w *warnings) []Phone {
	out := make([]Phone, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, ph := range in {
		if strings.TrimSpace(ph.Number) == "" {
			continue
		}
		number, ok := tenDigits(ph.Number)
		if !ok || len(out) == maxPhones {
			w.add(WarnPhoneInvalid)
			continue
		}
		if seen[number] {
			continue
		}
		seen[number] = true
		kind := PhoneKind(strings.ToLower(strings.TrimSpace(string(ph.Kind))))
		if !phoneKinds[kind] {
			kind = PhoneOther
		}
		out = append(out, Phone{Kind: kind, Number: number})
	}
	return out
}

// tenDigits reduces a number to its ten national digits, removing one known
// prefix when that is what stands between it and ten.
func tenDigits(raw string) (string, bool) {
	digits := onlyDigits(raw)
	if len(digits) == phoneDigits {
		return digits, true
	}
	for _, prefix := range phonePrefixes {
		if len(digits) == len(prefix)+phoneDigits && strings.HasPrefix(digits, prefix) {
			return digits[len(prefix):], true
		}
	}
	return "", false
}

func lenientAddress(a Address, w *warnings) Address {
	out := Address{
		Street:       truncateRunes(a.Street, w),
		ExtNumber:    truncateRunes(a.ExtNumber, w),
		IntNumber:    truncateRunes(a.IntNumber, w),
		Neighborhood: truncateRunes(a.Neighborhood, w),
		Municipality: truncateRunes(a.Municipality, w),
	}
	if cp := strings.TrimSpace(a.PostalCode); cp != "" {
		if len(cp) == postalCodeLen && onlyDigits(cp) == cp {
			out.PostalCode = cp
		} else {
			w.add(WarnPostalCodeInvalid)
		}
	}
	if raw := strings.TrimSpace(a.State); raw != "" {
		if state, ok := MatchState(raw); ok {
			out.State = state
		} else {
			w.add(WarnStateUnknown)
		}
	}
	return out
}

// truncateRunes trims a free-text field and cuts it to maxAddressField runes.
func truncateRunes(s string, w *warnings) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= maxAddressField {
		return s
	}
	w.add(WarnAddressTruncated)
	return strings.TrimSpace(string([]rune(s)[:maxAddressField]))
}
