package domain

import (
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// IssueField names the part of a pupil's file an import issue is about. The
// list is a contract (students.v1 ImportIssue.field, migration 0022's CHECK).
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

var issueFields = map[IssueField]bool{
	FieldBirthDate: true, FieldPhoneHome: true, FieldPhoneMobile: true, FieldEmail: true,
	FieldPostalCode: true, FieldState: true, FieldBankReference: true, FieldSurnames: true,
	FieldFirstName: true, FieldAddress: true, FieldCURP: true,
}

// ParseIssueField resolves a field name; an unknown one is KindInvalid.
func ParseIssueField(raw string) (IssueField, error) {
	f := IssueField(strings.TrimSpace(raw))
	if !issueFields[f] {
		return "", glerrors.New(glerrors.KindInvalid, "unknown import issue field: "+raw)
	}
	return f, nil
}

// IssueResolution is how an import issue was closed; "" while it is open.
type IssueResolution string

// The resolutions.
const (
	// ResolutionCorrected: a save of the file changed the issue's field.
	ResolutionCorrected IssueResolution = "corrected"
	// ResolutionAccepted: someone said the stored value is fine as it is.
	ResolutionAccepted IssueResolution = "accepted"
)

// Codes of the curp issue a draft is stored with.
const (
	IssueCURPMissing = "curp_missing"
	IssueCURPInvalid = "curp_invalid"
)

const (
	// maxIssueValue bounds original_value and stored_value (migration 0022).
	maxIssueValue = 500
	maxIssueCode  = 60
)

// ClipIssueValue cuts a value to what an issue stores, so a very long cell of
// the file costs its tail, never the pupil.
func ClipIssueValue(s string) string {
	if utf8.RuneCountInString(s) <= maxIssueValue {
		return s
	}
	return string([]rune(s)[:maxIssueValue])
}

// ImportIssueParams carries the fields of an import issue, new or restored.
type ImportIssueParams struct {
	ID              string
	TenantID        string
	StudentID       string
	Field           string
	Code            string
	OriginalValue   string
	StoredValue     string
	BatchID         string
	IntendedGroupID string
	CreatedAt       time.Time
	ResolvedAt      time.Time
	ResolvedBy      string
	Resolution      string
}

// ImportIssue is one value the legacy import could not keep as the file wrote
// it (PLAN-import-avisos D1): what the file said, what was stored, and whether
// the school has dealt with it. It is a value: Accept returns a new one.
type ImportIssue struct {
	id, tenantID, studentID string
	field                   IssueField
	code                    string
	originalValue           string
	storedValue             string
	batchID                 string
	intendedGroupID         string
	createdAt               time.Time
	resolvedAt              time.Time
	resolvedBy              string
	resolution              IssueResolution
}

// NewImportIssue validates and builds an issue (also used to restore one).
func NewImportIssue(p ImportIssueParams) (ImportIssue, error) {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.StudentID) == "" {
		return ImportIssue{}, glerrors.New(glerrors.KindInvalid, "an import issue needs its id and pupil")
	}
	field, err := ParseIssueField(p.Field)
	if err != nil {
		return ImportIssue{}, err
	}
	code := strings.TrimSpace(p.Code)
	if code == "" || len(code) > maxIssueCode {
		return ImportIssue{}, glerrors.New(glerrors.KindInvalid, "an import issue needs a code")
	}
	if utf8.RuneCountInString(p.OriginalValue) > maxIssueValue || utf8.RuneCountInString(p.StoredValue) > maxIssueValue {
		return ImportIssue{}, glerrors.New(glerrors.KindInvalid, "an import issue value is too long")
	}
	if p.CreatedAt.IsZero() {
		return ImportIssue{}, glerrors.New(glerrors.KindInvalid, "an import issue needs its creation instant")
	}
	resolution := IssueResolution(p.Resolution)
	if resolution != "" && resolution != ResolutionCorrected && resolution != ResolutionAccepted {
		return ImportIssue{}, glerrors.New(glerrors.KindInvalid, "unknown import issue resolution: "+p.Resolution)
	}
	if (resolution == "") != p.ResolvedAt.IsZero() {
		return ImportIssue{}, glerrors.New(glerrors.KindInvalid, "an import issue's resolution and its instant go together")
	}
	return ImportIssue{
		id: p.ID, tenantID: p.TenantID, studentID: p.StudentID, field: field, code: code,
		originalValue: p.OriginalValue, storedValue: p.StoredValue, batchID: p.BatchID,
		intendedGroupID: p.IntendedGroupID, createdAt: p.CreatedAt,
		resolvedAt: p.ResolvedAt, resolvedBy: p.ResolvedBy, resolution: resolution,
	}, nil
}

// Accept closes an open issue as fine as stored, by userID at `at`. A resolved
// issue is returned as it stands: the first resolution is the record.
func (i ImportIssue) Accept(userID string, at time.Time) ImportIssue {
	if !i.IsOpen() {
		return i
	}
	i.resolution, i.resolvedBy, i.resolvedAt = ResolutionAccepted, userID, at
	return i
}

// ClosesOnlyByCorrection is true for a curp issue: a pupil without a CURP is
// not "fine as it is" — only capturing one closes it.
func (i ImportIssue) ClosesOnlyByCorrection() bool { return i.field == FieldCURP }

// IsOpen reports an issue nobody resolved yet.
func (i ImportIssue) IsOpen() bool { return i.resolution == "" }

// Accessors.
func (i ImportIssue) ID() string                  { return i.id }
func (i ImportIssue) TenantID() string            { return i.tenantID }
func (i ImportIssue) StudentID() string           { return i.studentID }
func (i ImportIssue) Field() IssueField           { return i.field }
func (i ImportIssue) Code() string                { return i.code }
func (i ImportIssue) OriginalValue() string       { return i.originalValue }
func (i ImportIssue) StoredValue() string         { return i.storedValue }
func (i ImportIssue) BatchID() string             { return i.batchID }
func (i ImportIssue) IntendedGroupID() string     { return i.intendedGroupID }
func (i ImportIssue) CreatedAt() time.Time        { return i.createdAt }
func (i ImportIssue) ResolvedAt() time.Time       { return i.resolvedAt }
func (i ImportIssue) ResolvedBy() string          { return i.resolvedBy }
func (i ImportIssue) Resolution() IssueResolution { return i.resolution }

// ChangedIssueFields lists the issue fields whose value differs between two
// states of the same file, in a fixed order: a save closes the open issues of
// exactly these fields as corrected. The level, group and status are not
// fields an import issue is about.
func ChangedIssueFields(before, after *Student) []IssueField {
	b, a := before.profile, after.profile
	ba, aa := b.address, a.address
	checks := []struct {
		field   IssueField
		changed bool
	}{
		{FieldCURP, before.curp != after.curp},
		{FieldFirstName, before.firstName != after.firstName},
		{FieldSurnames, before.lastName != after.lastName || before.maternalLastName != after.maternalLastName},
		{FieldBirthDate, !b.birthDate.Equal(a.birthDate)},
		{FieldPhoneHome, !slices.Equal(numbersOf(b.phones, true), numbersOf(a.phones, true))},
		{FieldPhoneMobile, !slices.Equal(numbersOf(b.phones, false), numbersOf(a.phones, false))},
		{FieldEmail, b.email != a.email},
		{FieldPostalCode, ba.PostalCode != aa.PostalCode},
		{FieldState, ba.State != aa.State},
		{FieldAddress, ba.Street != aa.Street || ba.ExtNumber != aa.ExtNumber || ba.IntNumber != aa.IntNumber ||
			ba.Neighborhood != aa.Neighborhood || ba.Municipality != aa.Municipality},
		{FieldBankReference, before.bankReference != after.bankReference},
	}
	var out []IssueField
	for _, c := range checks {
		if c.changed {
			out = append(out, c.field)
		}
	}
	return out
}

// PhoneIssueField is the issue field a phone of `kind` is filed under: the
// home line, or the mobile one for every other kind (the import only sends
// casa and celular).
func PhoneIssueField(kind PhoneKind) IssueField {
	if PhoneKind(strings.ToLower(strings.TrimSpace(string(kind)))) == PhoneHome {
		return FieldPhoneHome
	}
	return FieldPhoneMobile
}

// numbersOf returns the sorted numbers of the home phones (home=true) or of
// every other kind.
func numbersOf(phones []Phone, home bool) []string {
	var out []string
	for _, ph := range phones {
		if (PhoneIssueField(ph.Kind) == FieldPhoneHome) == home {
			out = append(out, ph.Number)
		}
	}
	slices.Sort(out)
	return out
}
