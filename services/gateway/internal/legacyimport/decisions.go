package legacyimport

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/garcia-lorca/pkg/email"
)

// --- the office's decisions on a row (PLAN-import-avisos D2) ----------------
//
// Before "Importar" the office may correct a value of a row, omit the row or
// keep it as a draft. A correction is applied to the Row BEFORE Plan, so the
// row is judged with it exactly as with the file's own value, and is checked
// here first so a bad one is refused instead of being dropped as a warning.

// IssueField names the part of a pupil's file an issue is about — the
// students contract (students.v1 ImportIssue.field).
type IssueField string

// The fields an import issue can be about.
const (
	FieldBirthDate     IssueField = "birth_date"
	FieldPhoneHome     IssueField = "phone_home"
	FieldPhoneMobile   IssueField = "phone_mobile"
	FieldEmail         IssueField = "email"
	FieldPostalCode    IssueField = "postal_code"
	FieldState         IssueField = "state"
	FieldBankReference IssueField = "bank_reference"
	FieldSurnames      IssueField = "surnames"
	FieldFirstName     IssueField = "first_name"
	FieldAddress       IssueField = "address"
	FieldCURP          IssueField = "curp"
)

// SkipOmitted is a row the office chose to leave out.
const SkipOmitted = "omitted"

// Codes of a refused correction, as the screen words them.
const (
	CodeCURPInvalid = "curp_invalid"
	// CodeCURPMissing is a draft without any CURP; CodeCURPInvalid, with one
	// students refuses. Both are students' codes (domain.IssueCURP*).
	CodeCURPMissing          = "curp_missing"
	CodeBirthDateInvalid     = "birth_date_invalid"
	CodePhoneInvalid         = "phone_invalid"
	CodeEmailInvalid         = "email_invalid"
	CodePostalCodeInvalid    = "postal_code_invalid"
	CodeStateUnknown         = "state_unknown"
	CodeBankReferenceInvalid = "bank_reference_invalid"
	CodeAddressTooLong       = "address_too_long"
	CodeNameUnreadable       = "name_unreadable"
)

// Bounds of a correction. maxAddressField is students' (domain/profile.go):
// a longer value would be cut there, which is the silent change D1 forbids.
const (
	maxAddressField   = 120
	maxStateLen       = 60
	maxBankRefLen     = 20
	maxFullNameLen    = 200
	postalCodeDigits  = 5
	nationalPhoneLen  = 10
	isoDateLayout     = time.DateOnly
	fileDateLayout    = "02/01/2006"
	shortFileDateForm = "2/1/2006"
)

// curpPattern is the students service's CURP shape (services/students
// internal/domain/curp.go), copied because a service's internal package
// cannot be imported. TestValidCURPMatchesTheStudentsShape pins the cases.
var curpPattern = regexp.MustCompile(
	`^[A-Z][AEIOUX][A-Z]{2}` +
		`[0-9]{2}(0[1-9]|1[0-2])(0[1-9]|[12][0-9]|3[01])` +
		`[HM]` +
		`[A-Z]{2}[B-DF-HJ-NP-TV-Z]{3}[A-Z0-9][0-9]$`,
)

// ValidCURP reports whether students would accept the CURP (trimmed,
// upper-cased) — so whether a row may be drafted, and a correction kept.
func ValidCURP(raw string) bool {
	c := strings.ToUpper(strings.TrimSpace(raw))
	return len(c) == curpLen && curpPattern.MatchString(c)
}

// phonePrefixes are students' (domain/profile_lenient.go): what a previous
// system left before the ten digits. Longest first.
var phonePrefixes = []string{"521", "044", "045", "52"}

// phoneSeparators are what a person types between digits.
var phoneSeparators = strings.NewReplacer(" ", "", "-", "", ".", "", "(", "", ")", "", "+", "")

// NationalPhone reduces a number to the ten national digits students keeps,
// removing separators and one known prefix; ok is false when it is not one.
func NationalPhone(raw string) (string, bool) {
	digits := phoneSeparators.Replace(strings.TrimSpace(raw))
	if !allDigits(digits) {
		return "", false
	}
	if len(digits) == nationalPhoneLen {
		return digits, true
	}
	for _, prefix := range phonePrefixes {
		if len(digits) == len(prefix)+nationalPhoneLen && strings.HasPrefix(digits, prefix) {
			return digits[len(prefix):], true
		}
	}
	return "", false
}

// Overrides are the office's corrections to one row. A nil field keeps the
// file's value; an empty one clears an optional value (not the CURP nor the
// name, which a pupil cannot lack).
type Overrides struct {
	CURP          *string `json:"curp,omitempty"`
	BirthDate     *string `json:"birthDate,omitempty"`
	HomePhone     *string `json:"homePhone,omitempty"`
	MobilePhone   *string `json:"mobilePhone,omitempty"`
	Email         *string `json:"email,omitempty"`
	PostalCode    *string `json:"postalCode,omitempty"`
	State         *string `json:"state,omitempty"`
	BankReference *string `json:"bankReference,omitempty"`
	Municipality  *string `json:"municipality,omitempty"`
	Street        *string `json:"street,omitempty"`
	FullName      *string `json:"fullName,omitempty"`
}

// FieldError is a refused correction: the override's name and why.
type FieldError struct {
	Field string
	Code  string
}

// correction is one override: its name, the issue fields it settles, how to
// check and normalize it, and where it goes in the row.
type correction struct {
	name   string
	value  *string
	fields []IssueField
	clean  func(string) (string, string) // normalized value, refusal code
	set    func(*Row, string)
}

func (o Overrides) corrections() []correction {
	return []correction{
		{"curp", o.CURP, []IssueField{FieldCURP}, cleanCURP, func(r *Row, v string) { r.CURP = v }},
		{"birthDate", o.BirthDate, []IssueField{FieldBirthDate}, cleanBirthDate, func(r *Row, v string) { r.BirthDate = v }},
		{"homePhone", o.HomePhone, []IssueField{FieldPhoneHome}, cleanPhone, func(r *Row, v string) { r.HomePhone = v }},
		{"mobilePhone", o.MobilePhone, []IssueField{FieldPhoneMobile}, cleanPhone, func(r *Row, v string) { r.MobilePhone = v }},
		{"email", o.Email, []IssueField{FieldEmail}, cleanEmail, func(r *Row, v string) { r.Email = v }},
		{"postalCode", o.PostalCode, []IssueField{FieldPostalCode}, cleanPostalCode, func(r *Row, v string) { r.PostalCode = v }},
		{"state", o.State, []IssueField{FieldState}, cleanState, func(r *Row, v string) { r.State = v }},
		{"bankReference", o.BankReference, []IssueField{FieldBankReference}, cleanBankReference, func(r *Row, v string) { r.LegacyRef = v }},
		{"municipality", o.Municipality, []IssueField{FieldAddress}, cleanAddress, func(r *Row, v string) { r.Municipality = v }},
		{"street", o.Street, []IssueField{FieldAddress}, cleanAddress, func(r *Row, v string) { r.Street = v }},
		{"fullName", o.FullName, []IssueField{FieldSurnames, FieldFirstName}, cleanFullName, func(r *Row, v string) { r.FullName = v }},
	}
}

// KnownOverride reports whether name is an override of the contract.
func KnownOverride(name string) bool {
	for _, c := range (Overrides{}).corrections() {
		if c.name == name {
			return true
		}
	}
	return false
}

// Canonical is the corrections as the office approved them, normalized (two
// spellings of one phone are one), in a fixed order; "" when there are none.
// It is what the plan hash covers of a row's overrides.
func (o Overrides) Canonical() string {
	var parts []string
	for _, c := range o.corrections() {
		if c.value == nil {
			continue
		}
		value, code := c.clean(*c.value)
		if code != "" {
			value = strings.TrimSpace(*c.value)
		}
		parts = append(parts, c.name+"="+value)
	}
	return strings.Join(parts, "\x1f")
}

// DraftCURPCode is why a draft has no CURP: missing or invalid.
func DraftCURPCode(curp string) string {
	if strings.TrimSpace(curp) == "" {
		return CodeCURPMissing
	}
	return CodeCURPInvalid
}

// Fields maps an override's name to the issue fields it settles.
func (o Overrides) Fields() map[string][]IssueField {
	out := map[string][]IssueField{}
	for _, c := range o.corrections() {
		if c.value != nil {
			out[c.name] = c.fields
		}
	}
	return out
}

// Apply checks every correction and returns the row with the valid ones
// written in (normalized) and marked in Corrected, plus one FieldError per
// refused one. A row with any error must not be imported.
func (o Overrides) Apply(r Row) (Row, []FieldError) {
	var errs []FieldError
	for _, c := range o.corrections() {
		if c.value == nil {
			continue
		}
		value, code := c.clean(*c.value)
		if code != "" {
			errs = append(errs, FieldError{Field: c.name, Code: code})
			continue
		}
		c.set(&r, value)
		r.markCorrected(c.fields...)
	}
	return r, errs
}

func (r *Row) markCorrected(fields ...IssueField) {
	for _, f := range fields {
		r.Corrected |= fieldBits[f]
	}
}

// FieldSet is a set of issue fields; a bit set, so a Row stays comparable.
type FieldSet uint16

var fieldBits = map[IssueField]FieldSet{
	FieldBirthDate: 1 << 0, FieldPhoneHome: 1 << 1, FieldPhoneMobile: 1 << 2, FieldEmail: 1 << 3,
	FieldPostalCode: 1 << 4, FieldState: 1 << 5, FieldBankReference: 1 << 6, FieldSurnames: 1 << 7,
	FieldFirstName: 1 << 8, FieldAddress: 1 << 9, FieldCURP: 1 << 10,
}

// Has reports whether f is in the set.
func (s FieldSet) Has(f IssueField) bool { return s&fieldBits[f] != 0 }

func cleanCURP(raw string) (string, string) {
	if !ValidCURP(raw) {
		return "", CodeCURPInvalid
	}
	return strings.ToUpper(strings.TrimSpace(raw)), ""
}

// cleanBirthDate accepts dd/mm/yyyy (as the file writes it, with or without
// leading zeros) or ISO, and writes it the file's way for Plan to read.
func cleanBirthDate(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	for _, layout := range []string{fileDateLayout, shortFileDateForm, isoDateLayout} {
		if day, err := time.Parse(layout, raw); err == nil {
			return day.Format(fileDateLayout), ""
		}
	}
	return "", CodeBirthDateInvalid
}

func cleanPhone(raw string) (string, string) {
	if strings.TrimSpace(raw) == "" {
		return "", ""
	}
	number, ok := NationalPhone(raw)
	if !ok {
		return "", CodePhoneInvalid
	}
	return number, ""
}

func cleanEmail(raw string) (string, string) {
	address := email.Normalize(raw)
	if address == "" {
		return "", ""
	}
	if email.Validate(address) != nil {
		return "", CodeEmailInvalid
	}
	return address, ""
}

func cleanPostalCode(raw string) (string, string) {
	cp := strings.TrimSpace(raw)
	if cp == "" {
		return "", ""
	}
	if len(cp) != postalCodeDigits || !allDigits(cp) {
		return "", CodePostalCodeInvalid
	}
	return cp, ""
}

// cleanState bounds the name; whether it is a Mexican state is students' to
// say (its MatchState), and a refusal there is reported as state_unknown.
func cleanState(raw string) (string, string) {
	state := collapse(raw)
	if utf8.RuneCountInString(state) > maxStateLen {
		return "", CodeStateUnknown
	}
	return state, ""
}

func cleanBankReference(raw string) (string, string) {
	ref := strings.TrimSpace(raw)
	if len(ref) > maxBankRefLen || !allDigits(ref) {
		return "", CodeBankReferenceInvalid
	}
	return ref, ""
}

func cleanAddress(raw string) (string, string) {
	v := collapse(raw)
	if utf8.RuneCountInString(v) > maxAddressField {
		return "", CodeAddressTooLong
	}
	return v, ""
}

// cleanFullName requires "APELLIDOS, NOMBRES": a correction has no CURP to
// infer a missing comma from.
func cleanFullName(raw string) (string, string) {
	name := collapse(raw)
	if utf8.RuneCountInString(name) > maxFullNameLen || !strings.Contains(name, ",") {
		return "", CodeNameUnreadable
	}
	if _, err := SplitName(name, ""); err != nil {
		return "", CodeNameUnreadable
	}
	return name, ""
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// --- issues: what the import changes or drops, per field (D1) ---------------

// Issue is one value of a row the import does not keep as the file wrote it:
// the field, the stable code, the file's value and what is stored instead.
type Issue struct {
	Field    IssueField
	Code     string
	Original string
	Stored   string
}

// IssuesOf are the gateway's own issues of a planned row — the name split and
// the birth date it changed — against the file's row (before corrections). A
// corrected field raises none: Plan drops those warnings.
func IssuesOf(c Candidate, file Row) []Issue {
	var out []Issue
	surnames := strings.TrimSpace(c.Name.Paternal + " " + c.Name.Maternal)
	for _, w := range c.Warnings {
		switch w {
		case WarnSurnamesMismatch, WarnSurnamesAmbiguous:
			out = append(out, Issue{FieldSurnames, w, file.FullName, surnames})
		case WarnNameWithoutComma:
			out = append(out, Issue{FieldSurnames, w, file.FullName, surnames + ", " + c.Name.First})
		case WarnFirstNameMismatch:
			out = append(out, Issue{FieldFirstName, w, file.FullName, c.Name.First})
		case WarnBirthDateFromCURP:
			out = append(out, Issue{FieldBirthDate, w, file.BirthDate, c.BirthDate})
		case WarnBirthDateInvalid:
			out = append(out, Issue{FieldBirthDate, w, file.BirthDate, ""})
		}
	}
	return out
}

// StudentIssues names the field of each warning students answered for a row
// (it answers codes only), so the preview can open that field for editing.
// file holds the file's values (reported as original); sent, what the row
// carried to students — which phone or address part was the bad one. Codes
// this gateway does not know raise no issue; they stay in the warnings.
func StudentIssues(codes []string, file, sent Row) []Issue {
	var out []Issue
	for _, code := range codes {
		switch code {
		case CodePhoneInvalid:
			out = append(out, phoneIssues(file, sent)...)
		case CodeEmailInvalid:
			out = append(out, Issue{FieldEmail, code, file.Email, ""})
		case CodePostalCodeInvalid:
			out = append(out, Issue{FieldPostalCode, code, file.PostalCode, ""})
		case CodeStateUnknown:
			out = append(out, Issue{FieldState, code, file.State, ""})
		case CodeBankReferenceInvalid, "bank_reference_taken":
			out = append(out, Issue{FieldBankReference, code, file.LegacyRef, ""})
		case CodeBirthDateInvalid:
			out = append(out, Issue{FieldBirthDate, code, file.BirthDate, ""})
		case "address_truncated":
			out = append(out, addressIssues(code, file, sent)...)
		}
	}
	return out
}

func phoneIssues(file, sent Row) []Issue {
	var out []Issue
	if _, ok := NationalPhone(sent.HomePhone); sent.HomePhone != "" && !ok {
		out = append(out, Issue{FieldPhoneHome, CodePhoneInvalid, file.HomePhone, ""})
	}
	if _, ok := NationalPhone(sent.MobilePhone); sent.MobilePhone != "" && !ok {
		out = append(out, Issue{FieldPhoneMobile, CodePhoneInvalid, file.MobilePhone, ""})
	}
	return out
}

func addressIssues(code string, file, sent Row) []Issue {
	var out []Issue
	for _, part := range [][2]string{{file.Street, sent.Street}, {file.Municipality, sent.Municipality}} {
		if utf8.RuneCountInString(part[1]) > maxAddressField {
			cut := strings.TrimSpace(string([]rune(part[1])[:maxAddressField]))
			out = append(out, Issue{FieldAddress, code, part[0], cut})
		}
	}
	return out
}
