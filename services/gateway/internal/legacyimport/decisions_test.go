package legacyimport

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func ptr(s string) *string { return &s }

// The CURP check is the students service's shape check, copied: a row the
// gateway calls valid must be one students accepts, or a draft decision would
// be refused (or allowed) on the wrong rows.
func TestValidCURPMatchesTheStudentsShape(t *testing.T) {
	cases := map[string]bool{
		"AAMI210219MJCLLRA0":  true,
		"aami210219mjcllra0":  true, // case is normalized
		" AAMI210219MJCLLRA0": true,
		"":                    false,
		"AAMI210219MJCLLRA":   false, // 17
		"AAMI211319MJCLLRA0":  false, // month 13
		"AAMI210219XJCLLRA0":  false, // sex X
		"AAMI210219MJCLARA0":  false, // a vowel among the inner consonants
		"KNOWN000000000000A":  false,
	}
	for in, want := range cases {
		if got := ValidCURP(in); got != want {
			t.Errorf("ValidCURP(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestNationalPhoneReducesToTenDigitsLikeStudents(t *testing.T) {
	cases := map[string]struct {
		want string
		ok   bool
	}{
		"3314626871":         {"3314626871", true},
		"33 1462-6871":       {"3314626871", true},
		"(33) 1462.6871":     {"3314626871", true},
		"+52 33 1462 6871":   {"3314626871", true},
		"521 33 1462 6871":   {"3314626871", true},
		"044 33 1462 6871":   {"3314626871", true},
		"55 1234":            {"", false},
		"33146268711":        {"", false},
		"33-1462-6871 ext 2": {"", false},
	}
	for in, tc := range cases {
		got, ok := NationalPhone(in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("NationalPhone(%q) = %q, %v; want %q, %v", in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestOverridesApplyNormalizesEveryField(t *testing.T) {
	base := row(14, "SOTO RUIZ, ANA", "Primaria", "1", "A", "Activo", "31/02/2021", "")
	o := Overrides{
		CURP: ptr(" soxa120101mjcrrna1 "), BirthDate: ptr("2012-01-01"), HomePhone: ptr("33 1462 6871"),
		MobilePhone: ptr(""), Email: ptr(" Ana@Example.MX "), PostalCode: ptr(" 45640 "), State: ptr(" Jalisco "),
		BankReference: ptr(" 231076 "), Municipality: ptr(" Tlajomulco  de Zúñiga "), Street: ptr(" Lerdo  35 "),
		FullName: ptr(" SOTO  RUIZ , ANA "),
	}

	got, errs := o.Apply(base)

	if len(errs) != 0 {
		t.Fatalf("errors = %v", errs)
	}
	want := Row{Number: 14, LegacyRef: "231076", FullName: "SOTO RUIZ , ANA", LevelName: "Primaria", Grade: "1",
		GroupName: "A", Status: "Activo", BirthDate: "01/01/2012", CURP: "SOXA120101MJCRRNA1", Email: "ana@example.mx",
		HomePhone: "3314626871", MobilePhone: "", Street: "Lerdo 35", State: "Jalisco", Municipality: "Tlajomulco de Zúñiga",
		PostalCode: "45640"}
	got.Corrected = nil
	if !reflect.DeepEqual(got, want) {
		t.Errorf("row =\n%+v\nwant\n%+v", got, want)
	}
}

func TestOverridesApplyMarksOnlyTheCorrectedFields(t *testing.T) {
	got, _ := Overrides{BirthDate: ptr("19/02/2021"), FullName: ptr("A B, C")}.Apply(Row{})
	for _, f := range []IssueField{FieldBirthDate, FieldSurnames, FieldFirstName} {
		if !got.Corrected[f] {
			t.Errorf("%s not marked corrected: %v", f, got.Corrected)
		}
	}
	if got.Corrected[FieldCURP] || got.Corrected[FieldPhoneHome] {
		t.Errorf("untouched fields marked: %v", got.Corrected)
	}
	if untouched, _ := (Overrides{}).Apply(Row{CURP: "X"}); untouched.CURP != "X" || len(untouched.Corrected) != 0 {
		t.Errorf("no overrides changed the row: %+v", untouched)
	}
}

func TestOverridesApplyRefusesWhatStudentsWouldDrop(t *testing.T) {
	cases := []struct {
		name  string
		o     Overrides
		field string
		code  string
	}{
		{"curp shape", Overrides{CURP: ptr("ABC")}, "curp", CodeCURPInvalid},
		{"curp cleared", Overrides{CURP: ptr("")}, "curp", CodeCURPInvalid},
		{"birth date unreadable", Overrides{BirthDate: ptr("31/02/2021")}, "birthDate", CodeBirthDateInvalid},
		{"birth date other format", Overrides{BirthDate: ptr("Feb 19 2021")}, "birthDate", CodeBirthDateInvalid},
		{"home phone", Overrides{HomePhone: ptr("55 1234")}, "homePhone", CodePhoneInvalid},
		{"mobile phone", Overrides{MobilePhone: ptr("abc")}, "mobilePhone", CodePhoneInvalid},
		{"email", Overrides{Email: ptr("not-an-email")}, "email", CodeEmailInvalid},
		{"postal code letters", Overrides{PostalCode: ptr("4564A")}, "postalCode", CodePostalCodeInvalid},
		{"postal code short", Overrides{PostalCode: ptr("4564")}, "postalCode", CodePostalCodeInvalid},
		{"state too long", Overrides{State: ptr(strings.Repeat("J", 61))}, "state", CodeStateUnknown},
		{"bank reference letters", Overrides{BankReference: ptr("23A")}, "bankReference", CodeBankReferenceInvalid},
		{"bank reference long", Overrides{BankReference: ptr(strings.Repeat("1", 21))}, "bankReference", CodeBankReferenceInvalid},
		{"street too long", Overrides{Street: ptr(strings.Repeat("s", 121))}, "street", CodeAddressTooLong},
		{"municipality too long", Overrides{Municipality: ptr(strings.Repeat("m", 121))}, "municipality", CodeAddressTooLong},
		{"name without given name", Overrides{FullName: ptr("SOTO RUIZ,")}, "fullName", CodeNameUnreadable},
		{"name cleared", Overrides{FullName: ptr("")}, "fullName", CodeNameUnreadable},
		{"name too long", Overrides{FullName: ptr(strings.Repeat("A", 200) + ", B")}, "fullName", CodeNameUnreadable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := tc.o.Apply(Row{})
			if !slices.Equal(errs, []FieldError{{Field: tc.field, Code: tc.code}}) {
				t.Errorf("errors = %v, want %s/%s", errs, tc.field, tc.code)
			}
		})
	}
}

// An empty override clears an optional value: the office's way to drop a
// phone it cannot fix, without an issue left behind.
func TestOverridesApplyClearsOptionalValues(t *testing.T) {
	base := Row{HomePhone: "55 1234", Email: "x@", PostalCode: "4", State: "Atlántida", LegacyRef: "A1", BirthDate: "31/02/2021"}
	got, errs := Overrides{HomePhone: ptr(""), Email: ptr(""), PostalCode: ptr(""), State: ptr(""),
		BankReference: ptr(""), BirthDate: ptr("")}.Apply(base)
	if len(errs) != 0 {
		t.Fatalf("errors = %v", errs)
	}
	if got.HomePhone != "" || got.Email != "" || got.PostalCode != "" || got.State != "" || got.LegacyRef != "" || got.BirthDate != "" {
		t.Errorf("row = %+v, want every value cleared", got)
	}
}

func TestPlanLeavesAnOmittedRowOutOfEveryPlacement(t *testing.T) {
	r := row(2, "ALCAZAR MELENDEZ, IRLANDA", "Preescolar", "3", "B", "Activo", "19/02/2021", "AAMI210219MJCLLRA0")
	r.Omitted = true

	got := Plan([]Row{r})

	if got.Candidates[0].Skip != SkipOmitted {
		t.Errorf("skip = %q, want %q", got.Candidates[0].Skip, SkipOmitted)
	}
	if len(got.Placements) != 0 || len(got.Levels) != 0 {
		t.Errorf("placements %v levels %v, want none for an omitted row", got.Placements, got.Levels)
	}
}

// The office's date wins over the CURP's: it was typed on purpose.
func TestPlanKeepsACorrectedBirthDateWithoutWarning(t *testing.T) {
	r, _ := Overrides{BirthDate: ptr("20/02/2021")}.Apply(
		row(2, "ALCAZAR MELENDEZ, IRLANDA", "Preescolar", "3", "B", "Activo", "31/02/2021", "AAMI210219MJCLLRA0"))

	c := Plan([]Row{r}).Candidates[0]

	if c.BirthDate != "2021-02-20" || len(c.Warnings) != 0 {
		t.Errorf("birth %q warnings %v, want the typed date and no warning", c.BirthDate, c.Warnings)
	}
	cleared, _ := Overrides{BirthDate: ptr("")}.Apply(r)
	if c := Plan([]Row{cleared}).Candidates[0]; c.BirthDate != "" || len(c.Warnings) != 0 {
		t.Errorf("cleared: birth %q warnings %v, want none", c.BirthDate, c.Warnings)
	}
}

func TestPlanDropsTheNameWarningsOfACorrectedName(t *testing.T) {
	r, _ := Overrides{FullName: ptr("SOTO RUIZ, ANA")}.Apply(
		row(2, "X", "Primaria", "1", "A", "Activo", "01/01/2012", "ZZZZ120101MJCRRNA1"))

	c := Plan([]Row{r}).Candidates[0]

	if c.Skip != "" || c.Name.Paternal != "SOTO" || len(c.Warnings) != 0 {
		t.Errorf("candidate %+v, want the corrected split without warnings", c)
	}
}

func TestIssuesOfReportsTheFileValueAndWhatIsStored(t *testing.T) {
	file := row(2, "SOTO RUIZ, ANA", "Primaria", "1", "A", "Activo", "31/02/2012", "ZZZZ120101MJCRRNA1")
	c := Plan([]Row{file}).Candidates[0]

	got := IssuesOf(c, file)

	want := []Issue{
		{Field: FieldSurnames, Code: WarnSurnamesMismatch, Original: "SOTO RUIZ, ANA", Stored: "SOTO RUIZ"},
		{Field: FieldBirthDate, Code: WarnBirthDateFromCURP, Original: "31/02/2012", Stored: "2012-01-01"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("issues =\n%+v\nwant\n%+v", got, want)
	}
}

func TestIssuesOfNamesEveryNameWarningsField(t *testing.T) {
	cases := []struct {
		full, curp string
		want       Issue
	}{
		{"ESPINOSA DE LOS MONTEROS MENDOZA, EDUARDO", "EIME120101HJCSNDA1",
			Issue{FieldSurnames, WarnSurnamesAmbiguous, "ESPINOSA DE LOS MONTEROS MENDOZA, EDUARDO", "ESPINOSA DE LOS MONTEROS MENDOZA"}},
		{"LOPEZ PEREZ JUAN", "LOPJ120101HJCPRNA1",
			Issue{FieldSurnames, WarnNameWithoutComma, "LOPEZ PEREZ JUAN", "LOPEZ PEREZ, JUAN"}},
		{"ALCAZAR MELENDEZ, ZOE", "AAMI120101MJCLLRA0",
			Issue{FieldFirstName, WarnFirstNameMismatch, "ALCAZAR MELENDEZ, ZOE", "ZOE"}},
	}
	for _, tc := range cases {
		file := row(2, tc.full, "Primaria", "1", "A", "Activo", "01/01/2012", tc.curp)
		got := IssuesOf(Plan([]Row{file}).Candidates[0], file)
		if !slices.Contains(got, tc.want) {
			t.Errorf("%s: issues %+v, want %+v", tc.full, got, tc.want)
		}
	}
}

// students answers warning codes only; the preview names the field they are
// about so the screen can open it for editing.
func TestStudentIssuesNamesTheFieldOfEachStudentsWarning(t *testing.T) {
	file := Row{HomePhone: "3314626871", MobilePhone: "55 1234", Email: "x@", PostalCode: "4564",
		State: "Atlántida", LegacyRef: "A1", BirthDate: "01/01/2099", Street: strings.Repeat("s", 121)}

	got := StudentIssues([]string{"phone_invalid", "email_invalid", "postal_code_invalid", "state_unknown",
		"bank_reference_invalid", "birth_date_invalid", "address_truncated", "unknown_code"}, file, file)

	want := []Issue{
		{FieldPhoneMobile, "phone_invalid", "55 1234", ""},
		{FieldEmail, "email_invalid", "x@", ""},
		{FieldPostalCode, "postal_code_invalid", "4564", ""},
		{FieldState, "state_unknown", "Atlántida", ""},
		{FieldBankReference, "bank_reference_invalid", "A1", ""},
		{FieldBirthDate, "birth_date_invalid", "01/01/2099", ""},
		{FieldAddress, "address_truncated", strings.Repeat("s", 121), strings.Repeat("s", 120)},
	}
	if !slices.Equal(got, want) {
		t.Errorf("issues =\n%+v\nwant\n%+v", got, want)
	}
}
