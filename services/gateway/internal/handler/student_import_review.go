package handler

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"net/http"
	"slices"

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
// The plan hash covers what the decisions do to each row (its status, reason
// and destination) AND the decisions themselves, canonical (decisionsDigest):
// the screen re-previews with its decisions and commits with the same ones,
// and a commit with other decisions — even a phone corrected to another
// number, which moves no row's fate — is PREVIEW_STALE: the office approved
// that result, not another. Without decisions the hash is the one it always
// was.
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
	// codeOverrideUnknown is a key the contract does not have (a decision and
	// its overrides are closed: additionalProperties false).
	codeOverrideUnknown   = "override_unknown"
	codeOverrideMalformed = "override_malformed"
	codeRowMalformed      = "row_malformed"
	codeTooManyDecisions  = "too_many_rows"
)

// maxRowDecisions is the contract's maxItems, a file's row limit; checked
// before any decision is read. maxRowErrors bounds the refusal's size.
const (
	maxRowDecisions = 1000
	maxRowErrors    = 100
	fieldRows       = "rows"
)

// Where a refusal points when it is not an override.
const (
	fieldRow    = "row"
	fieldAction = "action"
)

// rowDecision is the office's choice for one spreadsheet row. Decoding never
// fails: what does not fit the contract is kept in problems and refused only
// when the review is on — off, the decisions are not read at all.
type rowDecision struct {
	Row       int                    `json:"row"`
	Action    string                 `json:"action,omitempty"`
	Overrides legacyimport.Overrides `json:"overrides"`
	problems  []rowProblem
}

// rowProblem is a key of a decision the contract refuses.
type rowProblem struct{ field, code string }

// UnmarshalJSON reads a decision strictly: an unknown key (of the decision or
// of its overrides) or a value of the wrong type is a problem, by its key.
func (d *rowDecision) UnmarshalJSON(raw []byte) error {
	*d = rowDecision{}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		d.problems = append(d.problems, rowProblem{fieldRow, codeRowMalformed})
		return nil
	}
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		value := fields[key]
		switch key {
		case "row":
			if json.Unmarshal(value, &d.Row) != nil {
				d.problems = append(d.problems, rowProblem{fieldRow, codeRowMalformed})
			}
		case "action":
			if json.Unmarshal(value, &d.Action) != nil {
				d.problems = append(d.problems, rowProblem{fieldAction, codeActionUnknown})
			}
		case "overrides":
			d.decodeOverrides(value)
		default:
			d.problems = append(d.problems, rowProblem{key, codeOverrideUnknown})
		}
	}
	return nil
}

func (d *rowDecision) decodeOverrides(raw json.RawMessage) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		d.problems = append(d.problems, rowProblem{"overrides", codeOverrideMalformed})
		return
	}
	known := map[string]json.RawMessage{}
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		var text *string
		switch {
		case !legacyimport.KnownOverride(key):
			d.problems = append(d.problems, rowProblem{key, codeOverrideUnknown})
		case json.Unmarshal(fields[key], &text) != nil:
			d.problems = append(d.problems, rowProblem{key, codeOverrideMalformed})
		default:
			known[key] = fields[key]
		}
	}
	// Every value left is a string or null: re-reading them cannot fail.
	encoded, _ := json.Marshal(known)
	_ = json.Unmarshal(encoded, &d.Overrides)
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
	// digest is the decisions as approved, canonical (by row, normalized): the
	// plan hash covers it, so a commit with other corrections is stale. "" when
	// nothing differs from a plain import.
	digest string
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
	if len(decisions) > maxRowDecisions {
		return rv, []rowError{{0, fieldRows, codeTooManyDecisions}}
	}
	seen := make(map[int]bool, len(decisions))
	var errs []rowError
	for _, d := range decisions {
		if len(errs) >= maxRowErrors {
			return rv, errs[:maxRowErrors]
		}
		i, known := at[d.Row]
		switch {
		case len(d.problems) > 0:
			for _, p := range d.problems {
				errs = append(errs, rowError{d.Row, p.field, p.code})
			}
		case !known:
			errs = append(errs, rowError{d.Row, fieldRow, codeRowUnknown})
		case seen[d.Row]:
			errs = append(errs, rowError{d.Row, fieldRow, codeRowRepeated})
		default:
			seen[d.Row] = true
			errs = append(errs, rv.decide(&rows[i], d)...)
		}
	}
	if len(errs) > maxRowErrors {
		errs = errs[:maxRowErrors]
	}
	rv.digest = decisionsDigest(decisions)
	return rv, errs
}

// decisionsDigest is the decisions in a canonical form: sorted by row, the
// action named, the overrides normalized; a plain import (no action, no
// override) is left out, so sending it or not is the same plan.
func decisionsDigest(decisions []rowDecision) string {
	type canonical struct {
		Row       int
		Action    string
		Overrides string
	}
	var out []canonical
	for _, d := range decisions {
		action := cmp.Or(d.Action, actionImport)
		overrides := d.Overrides.Canonical()
		if action == actionImport && overrides == "" {
			continue
		}
		out = append(out, canonical{d.Row, action, overrides})
	}
	if len(out) == 0 {
		return ""
	}
	slices.SortFunc(out, func(a, b canonical) int { return cmp.Compare(a.Row, b.Row) })
	raw, _ := json.Marshal(out) // strings and ints only
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
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
		issues = append(issues, legacyimport.Issue{Field: legacyimport.FieldCURP, Code: legacyimport.DraftCURPCode(c.Row.CURP), Original: file.CURP})
	}
	for _, i := range issues {
		shown := importIssue{Field: string(i.Field), Code: i.Code, Original: i.Original, Stored: i.Stored}
		if !slices.ContainsFunc(row.Issues, func(o importIssue) bool { return o.Field == shown.Field && o.Code == shown.Code }) {
			row.Issues = append(row.Issues, shown)
		}
	}
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
