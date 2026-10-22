package handler

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/legacyimport"
)

// --- the import review (PLAN-import-avisos, module_import_review) ------------
//
// Before "Importar" the office decides, per spreadsheet row: import it as it
// is, correct a value, omit it, or keep a row without a valid CURP as a draft.
// The gateway still reads the file itself; the decisions are applied to the
// parsed rows BEFORE they are planned, so a corrected row is judged — by Plan,
// config and students — exactly as if the file had said it.
//
// The plan hash does not cover the decisions themselves but what they do to
// each row (its status, reason and destination are already in it): the screen
// re-previews with its decisions and commits with the same ones, and a commit
// whose decisions change any row's fate is PREVIEW_STALE — the office approved
// that result, not another.
//
// With the flag off the decisions are ignored and nothing new is sent or
// answered: the import behaves as it did before the review existed.

// codeImportRowsInvalid is a refused decision (422, the shared envelope).
const codeImportRowsInvalid = "IMPORT_ROWS_INVALID"

// The actions of a row.
const (
	actionImport = "import"
	actionOmit   = "omit"
	actionDraft  = "draft"
)

// Codes of a refused decision that are not a field's own.
const (
	codeRowUnknown     = "row_unknown"
	codeRowRepeated    = "row_repeated"
	codeActionUnknown  = "action_unknown"
	codeDraftHasCURP   = "draft_has_curp"
	codeCURPDuplicated = "curp_duplicated"
)

// Where a refusal points when it is not an override.
const (
	fieldRow    = "row"
	fieldAction = "action"
)

// rowDecision is the office's choice for one spreadsheet row.
type rowDecision struct {
	Row       int                    `json:"row"`
	Action    string                 `json:"action,omitempty"`
	Overrides legacyimport.Overrides `json:"overrides"`
}

// rowError is one refused decision: the row, the override (or "row" /
// "action") and why.
type rowError struct {
	Row   int    `json:"row"`
	Field string `json:"field"`
	Code  string `json:"code"`
}

// importIssue is one value of a preview row the import changes or drops.
type importIssue struct {
	Field    string `json:"field"`
	Code     string `json:"code"`
	Original string `json:"original"`
	Stored   string `json:"stored"`
}

// review is the decisions applied to one planning pass. The zero value is the
// review off.
type review struct {
	on bool
	// files are the file's rows before any correction, by row number.
	files map[int]legacyimport.Row
	// drafts are the rows the office chose to keep as drafts.
	drafts map[int]bool
	// corrected names, per row, the override that settled each issue field.
	corrected map[int]map[legacyimport.IssueField]string
}

// reviewOn reports whether module_import_review is on for this request.
func (h *StudentImportHandler) reviewOn(ctx context.Context) bool {
	return h.d.ReviewEnabled != nil && h.d.ReviewEnabled(ctx)
}

// applyDecisions writes the decisions into the parsed rows (in place) and
// returns the review, or every refused decision — then nothing is applied.
func applyDecisions(rows []legacyimport.Row, decisions []rowDecision) (review, []rowError) {
	rv := review{
		on: true, files: make(map[int]legacyimport.Row, len(rows)),
		drafts: map[int]bool{}, corrected: map[int]map[legacyimport.IssueField]string{},
	}
	at := make(map[int]int, len(rows))
	for i, r := range rows {
		rv.files[r.Number], at[r.Number] = r, i
	}
	seen := make(map[int]bool, len(decisions))
	var errs []rowError
	for _, d := range decisions {
		i, known := at[d.Row]
		switch {
		case !known:
			errs = append(errs, rowError{d.Row, fieldRow, codeRowUnknown})
		case seen[d.Row]:
			errs = append(errs, rowError{d.Row, fieldRow, codeRowRepeated})
		default:
			seen[d.Row] = true
			errs = append(errs, rv.decide(&rows[i], d)...)
		}
	}
	return rv, errs
}

// decide applies one decision to its row, or answers why it cannot be.
func (rv review) decide(r *legacyimport.Row, d rowDecision) []rowError {
	corrected, fieldErrs := d.Overrides.Apply(*r)
	errs := make([]rowError, 0, len(fieldErrs))
	for _, fe := range fieldErrs {
		errs = append(errs, rowError{d.Row, fe.Field, fe.Code})
	}
	switch d.Action {
	case "", actionImport:
	case actionOmit:
		corrected.Omitted = true
	case actionDraft:
		if legacyimport.ValidCURP(corrected.CURP) {
			errs = append(errs, rowError{d.Row, fieldAction, codeDraftHasCURP})
		}
	default:
		errs = append(errs, rowError{d.Row, fieldAction, codeActionUnknown})
	}
	if len(errs) > 0 {
		return errs
	}
	*r = corrected
	rv.drafts[d.Row] = d.Action == actionDraft
	byField := map[legacyimport.IssueField]string{}
	for name, fields := range d.Overrides.Fields() {
		for _, f := range fields {
			byField[f] = name
		}
	}
	if len(byField) > 0 {
		rv.corrected[d.Row] = byField
	}
	return nil
}

// annotate adds to a row sent to students what the review knows: the draft
// action, the file's original values and the gateway's own issues.
func (rv review) annotate(in *client.ImportRowInput, c legacyimport.Candidate) {
	if !rv.on {
		return
	}
	file := rv.files[c.Row.Number]
	if rv.drafts[c.Row.Number] {
		in.Action = client.ImportActionDraft
	}
	in.Original = originalOf(file)
	for _, i := range legacyimport.IssuesOf(c, file) {
		in.Issues = append(in.Issues, client.ImportIssue{Field: string(i.Field), Code: i.Code, Original: i.Original, Stored: i.Stored})
	}
}

// originalOf is the file's text per field. The address stays unknown: one
// text cannot say which of its parts (street, municipality) an issue is
// about, so students reports the value it received there.
func originalOf(file legacyimport.Row) client.ImportOriginal {
	return client.ImportOriginal{
		CURP: file.CURP, BirthDate: file.BirthDate, PhoneHome: file.HomePhone, PhoneMobile: file.MobilePhone,
		Email: file.Email, PostalCode: file.PostalCode, State: file.State, BankReference: file.LegacyRef,
	}
}

// refusedByStudents are the corrections students would still not keep as
// sent (a warning on a corrected field, a CURP another row of the file has):
// students is the authority, and a correction must not become a new issue.
func (rv review) refusedByStudents(planned legacyimport.Planned, results map[int]client.ImportRowResult) []rowError {
	if !rv.on || len(rv.corrected) == 0 {
		return nil
	}
	var errs []rowError
	for _, c := range planned.Candidates {
		byField, ok := rv.corrected[c.Row.Number]
		res, judged := results[c.Row.Number]
		if !ok || !judged {
			continue
		}
		if name, ok := byField[legacyimport.FieldCURP]; ok && res.Reason == codeCURPDuplicated {
			errs = append(errs, rowError{c.Row.Number, name, codeCURPDuplicated})
		}
		for _, i := range legacyimport.StudentIssues(res.Warnings, rv.files[c.Row.Number], c.Row) {
			if name, ok := byField[i.Field]; ok {
				errs = append(errs, rowError{c.Row.Number, name, i.Code})
			}
		}
	}
	return errs
}

// dress adds the review's view of a row to the preview: whether it can be a
// draft, whether it is one, and each field the import changes or drops.
func (rv review) dress(row *importRow, c legacyimport.Candidate, sendable bool, res client.ImportRowResult) {
	if !rv.on {
		return
	}
	file := rv.files[c.Row.Number]
	row.CanDraft = sendable && !legacyimport.ValidCURP(c.Row.CURP)
	row.Draft = res.Draft
	issues := legacyimport.IssuesOf(c, file)
	issues = append(issues, legacyimport.StudentIssues(res.Warnings, file, c.Row)...)
	if row.CanDraft {
		issues = append(issues, legacyimport.Issue{Field: legacyimport.FieldCURP, Code: curpIssueCode(c.Row.CURP), Original: file.CURP})
	}
	for _, i := range issues {
		shown := importIssue{Field: string(i.Field), Code: i.Code, Original: i.Original, Stored: i.Stored}
		if !slices.ContainsFunc(row.Issues, func(o importIssue) bool { return o.Field == shown.Field && o.Code == shown.Code }) {
			row.Issues = append(row.Issues, shown)
		}
	}
}

// curpIssueCode is students' code for a draft's CURP (domain.IssueCURP*).
func curpIssueCode(curp string) string {
	if strings.TrimSpace(curp) == "" {
		return "curp_missing"
	}
	return "curp_invalid"
}

// writeRowErrors answers refused decisions with the shared envelope.
func writeRowErrors(w http.ResponseWriter, errs []rowError) {
	w.Header().Set(errorCodeHeader, codeImportRowsInvalid)
	writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": map[string]any{
		"code":    "invalid",
		"message": "Revisa las correcciones de la importación",
		"details": map[string]any{"rows": errs},
	}})
}
