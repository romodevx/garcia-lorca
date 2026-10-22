package client

import (
	"testing"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
)

// PLAN-import-avisos: the school's action, the file's original values and the
// gateway's own issues travel with the row; a row without them is sent as
// before (no original message at all), so the import review OFF changes
// nothing on the wire.
func TestImportRowsCarryTheirDecisionOriginalAndIssues(t *testing.T) {
	req := toImportStudentsRequest([]ImportRowInput{
		{
			RowNumber: 7, Action: ImportActionDraft,
			Original: ImportOriginal{CURP: "AAMI", BirthDate: "31/02/2021", PhoneHome: "33", PhoneMobile: "55 1234",
				Email: "x@", PostalCode: "4564", State: "Jal", BankReference: "A1"},
			Issues: []ImportIssue{{Field: "birth_date", Code: "birth_date_from_curp", Original: "31/02/2021", Stored: "2021-02-19"}},
		},
		{RowNumber: 8},
	}, true, "")

	r := req.GetRows()[0]
	if r.GetAction() != studentsv1.ImportRowAction_IMPORT_ROW_ACTION_DRAFT {
		t.Errorf("action = %v", r.GetAction())
	}
	o := r.GetOriginal()
	if o.GetCurp() != "AAMI" || o.GetBirthDate() != "31/02/2021" || o.GetPhoneHome() != "33" || o.GetPhoneMobile() != "55 1234" ||
		o.GetEmail() != "x@" || o.GetPostalCode() != "4564" || o.GetState() != "Jal" || o.GetBankReference() != "A1" {
		t.Errorf("original = %+v", o)
	}
	if len(r.GetIssues()) != 1 || r.GetIssues()[0].GetField() != "birth_date" || r.GetIssues()[0].GetCode() != "birth_date_from_curp" ||
		r.GetIssues()[0].GetOriginalValue() != "31/02/2021" || r.GetIssues()[0].GetStoredValue() != "2021-02-19" {
		t.Errorf("issues = %+v", r.GetIssues())
	}
	plain := req.GetRows()[1]
	if plain.GetAction() != studentsv1.ImportRowAction_IMPORT_ROW_ACTION_UNSPECIFIED || plain.Original != nil || len(plain.GetIssues()) != 0 {
		t.Errorf("a plain row = %+v, want it as before", plain)
	}

	got := fromImportResults([]*studentsv1.ImportRowResult{{RowNumber: 7, Draft: true}})
	if !got[0].Draft {
		t.Errorf("result = %+v, want the draft flag", got[0])
	}
}
