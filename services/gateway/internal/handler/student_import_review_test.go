package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/legacyimport"
)

// --- PLAN-import-avisos: the office's decisions per row (module_import_review)

func newReviewFixture() importFixture {
	f := newImportFixture()
	f.h.d.ReviewEnabled = func(context.Context) bool { return true }
	return f
}

func sp(s string) *string { return &s }

// draftless is a pupil whose file has no CURP.
func draftless(group string) []any {
	r := pupil("Primaria", group, "Activo", "")
	r[1] = "SOTO RUIZ, ANA"
	return r
}

func sentRows(f importFixture) []client.ImportRowInput {
	if len(f.roster.calls) == 0 {
		return nil
	}
	return f.roster.calls[len(f.roster.calls)-1].rows
}

func rowErrors(t *testing.T, rec interface {
	Result() *http.Response
}, body []byte) []rowError {
	t.Helper()
	res := rec.Result()
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body %s, want 422", res.StatusCode, body)
	}
	if got := res.Header.Get("X-Error-Code"); got != codeImportRowsInvalid {
		t.Errorf("X-Error-Code = %q, want %q", got, codeImportRowsInvalid)
	}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details struct {
				Rows []rowError `json:"rows"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode envelope: %v (%s)", err, body)
	}
	if env.Error.Code != "invalid" || env.Error.Message == "" {
		t.Errorf("envelope = %+v", env.Error)
	}
	return env.Error.Details.Rows
}

// OFF, the decisions are ignored and nothing new reaches students or the
// screen: the import behaves exactly as before the review existed.
func TestImportReviewOffIgnoresTheDecisions(t *testing.T) {
	f := newImportFixture()
	file := importFile(t, pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"), draftless("A"))

	rec := post(t, f.h.Preview, importRequest{FileBase64: file, Rows: []rowDecision{
		{Row: 2, Action: "omit"}, {Row: 3, Action: "draft"}, {Row: 2, Overrides: legacyimport.Overrides{MobilePhone: sp("bad")}},
	}})

	got := decodePreview(t, rec)
	if got.Rows[0].Status != rowNew {
		t.Errorf("row 2 = %+v, want imported as if nothing was decided", got.Rows[0])
	}
	for _, r := range sentRows(f) {
		if r.Action != "" || len(r.Issues) != 0 || r.Original != (client.ImportOriginal{}) {
			t.Errorf("row %d sent with review data while OFF: %+v", r.RowNumber, r)
		}
	}
	for _, key := range []string{`"issues"`, `"canDraft"`, `"draft"`} {
		if strings.Contains(rec.Body.String(), key) {
			t.Errorf("the OFF preview carries %s: %s", key, rec.Body.String())
		}
	}
}

func TestImportReviewOmitLeavesTheRowOut(t *testing.T) {
	f := newReviewFixture()
	file := importFile(t, pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"),
		pupil("Primaria", "B", "Activo", "AAMI210219MJCLLRA1"))

	got := decodePreview(t, post(t, f.h.Preview, importRequest{FileBase64: file, Rows: []rowDecision{{Row: 3, Action: "omit"}}}))

	if got.Rows[1].Status != rowSkipped || got.Rows[1].Reason != legacyimport.SkipOmitted {
		t.Errorf("row 3 = %+v, want skipped as omitted", got.Rows[1])
	}
	if sent := sentRows(f); len(sent) != 1 || sent[0].RowNumber != 2 {
		t.Errorf("sent = %+v, want only row 2", sent)
	}
	if len(got.Placements) != 1 || got.Placements[0].Group != "A" {
		t.Errorf("placements = %+v, want no group created for the omitted row", got.Placements)
	}
}

func TestImportReviewAppliesAnOverrideBeforeTheRowIsJudged(t *testing.T) {
	f := newReviewFixture()
	r := pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0")
	r[10] = "55 1234" // the mobile phone students would drop
	file := importFile(t, r)

	got := decodePreview(t, post(t, f.h.Preview, importRequest{FileBase64: file, Rows: []rowDecision{
		{Row: 2, Overrides: legacyimport.Overrides{MobilePhone: sp("33 1111 2222")}},
	}}))

	sent := sentRows(f)[0]
	if !slices.Contains(sent.Profile.Phones, client.StudentPhone{Kind: "celular", Number: "3311112222"}) {
		t.Errorf("phones = %+v, want the corrected number", sent.Profile.Phones)
	}
	if sent.Original.PhoneMobile != "55 1234" {
		t.Errorf("original = %+v, want the file's number", sent.Original)
	}
	if got.Rows[0].Status != rowNew || len(got.Rows[0].Issues) != 0 {
		t.Errorf("row = %+v, want new with no issue: the field was corrected", got.Rows[0])
	}
}

func TestImportReviewRefusesAnInvalidOverride(t *testing.T) {
	cases := []struct {
		name string
		rows []rowDecision
		want []rowError
	}{
		{"phone", []rowDecision{{Row: 2, Overrides: legacyimport.Overrides{MobilePhone: sp("55 1234")}}},
			[]rowError{{Row: 2, Field: "mobilePhone", Code: "phone_invalid"}}},
		{"curp and date", []rowDecision{{Row: 2, Overrides: legacyimport.Overrides{CURP: sp("ABC"), BirthDate: sp("31/02/2021")}}},
			[]rowError{{Row: 2, Field: "curp", Code: "curp_invalid"}, {Row: 2, Field: "birthDate", Code: "birth_date_invalid"}}},
		{"unknown row", []rowDecision{{Row: 99, Action: "omit"}}, []rowError{{Row: 99, Field: "row", Code: "row_unknown"}}},
		{"repeated row", []rowDecision{{Row: 2}, {Row: 2, Action: "omit"}}, []rowError{{Row: 2, Field: "row", Code: "row_repeated"}}},
		{"unknown action", []rowDecision{{Row: 2, Action: "delete"}}, []rowError{{Row: 2, Field: "action", Code: "action_unknown"}}},
		{"draft with a valid CURP", []rowDecision{{Row: 2, Action: "draft"}}, []rowError{{Row: 2, Field: "action", Code: "draft_has_curp"}}},
		{"draft once corrected to a valid CURP", []rowDecision{{Row: 3, Action: "draft", Overrides: legacyimport.Overrides{CURP: sp("SOXA120101MJCRRNA1")}}},
			[]rowError{{Row: 3, Field: "action", Code: "draft_has_curp"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newReviewFixture()
			file := importFile(t, pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"), draftless("A"))
			for _, fn := range []http.HandlerFunc{f.h.Preview, f.h.Commit} {
				rec := post(t, fn, importRequest{FileBase64: file, Rows: tc.rows, PlanHash: "x"})
				if got := rowErrors(t, rec, rec.Body.Bytes()); !slices.Equal(got, tc.want) {
					t.Errorf("errors = %+v, want %+v", got, tc.want)
				}
			}
			if len(f.roster.calls) != 0 || len(f.catalog.placements) != 0 {
				t.Errorf("a refused decision still asked config or students: %+v %+v", f.catalog.placements, f.roster.calls)
			}
		})
	}
}

// students is the authority on what it keeps (a state it does not recognize):
// a correction it would still drop is refused, not stored as a new issue.
func TestImportReviewRefusesACorrectionStudentsWouldStillDrop(t *testing.T) {
	f := newReviewFixture()
	file := importFile(t, pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"))

	rec := post(t, f.h.Preview, importRequest{FileBase64: file, Rows: []rowDecision{
		{Row: 2, Overrides: legacyimport.Overrides{State: sp("Atlántida")}},
	}})

	if got := rowErrors(t, rec, rec.Body.Bytes()); !slices.Equal(got, []rowError{{Row: 2, Field: "state", Code: "state_unknown"}}) {
		t.Errorf("errors = %+v", got)
	}
}

func TestImportReviewDraftsARowWithoutCURP(t *testing.T) {
	f := newReviewFixture()
	file := importFile(t, draftless("A"))

	got := decodePreview(t, post(t, f.h.Preview, importRequest{FileBase64: file, Rows: []rowDecision{{Row: 2, Action: "draft"}}}))

	sent := sentRows(f)[0]
	if sent.Action != client.ImportActionDraft {
		t.Errorf("action = %q, want draft", sent.Action)
	}
	row := got.Rows[0]
	if !row.CanDraft || !row.Draft {
		t.Errorf("row = %+v, want canDraft and draft", row)
	}
	if !slices.Contains(row.Issues, importIssue{Field: "curp", Code: "curp_missing", Original: "", Stored: ""}) {
		t.Errorf("issues = %+v, want the missing CURP", row.Issues)
	}
}

// A row with a valid CURP can never be a draft; one without, can.
func TestImportReviewTellsWhichRowsCanBeDrafted(t *testing.T) {
	f := newReviewFixture()
	file := importFile(t, pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"), draftless("A"))

	got := decodePreview(t, post(t, f.h.Preview, importRequest{FileBase64: file}))

	if got.Rows[0].CanDraft || !got.Rows[1].CanDraft || got.Rows[1].Draft {
		t.Errorf("rows = %+v / %+v", got.Rows[0], got.Rows[1])
	}
	if sentRows(f)[1].Action != "" {
		t.Errorf("an undecided row was sent as %q", sentRows(f)[1].Action)
	}
}

func TestImportReviewSendsTheGatewaysIssuesWithTheFileValue(t *testing.T) {
	f := newReviewFixture()
	r := pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0")
	r[6] = "31/02/2021"
	r[10] = "55 1234"
	file := importFile(t, r)

	got := decodePreview(t, post(t, f.h.Preview, importRequest{FileBase64: file}))

	sent := sentRows(f)[0]
	want := client.ImportIssue{Field: "birth_date", Code: "birth_date_from_curp", Original: "31/02/2021", Stored: "2021-02-19"}
	if !slices.Equal(sent.Issues, []client.ImportIssue{want}) {
		t.Errorf("issues sent = %+v, want %+v", sent.Issues, want)
	}
	if sent.Original.BirthDate != "31/02/2021" || sent.Original.CURP != "AAMI210219MJCLLRA0" || sent.Original.BankReference != "231076" {
		t.Errorf("original = %+v", sent.Original)
	}
	issues := got.Rows[0].Issues
	for _, w := range []importIssue{
		{Field: "birth_date", Code: "birth_date_from_curp", Original: "31/02/2021", Stored: "2021-02-19"},
		{Field: "phone_mobile", Code: "phone_invalid", Original: "55 1234", Stored: ""},
	} {
		if !slices.Contains(issues, w) {
			t.Errorf("preview issues = %+v, want %+v", issues, w)
		}
	}

	corrected := newReviewFixture()
	decodePreview(t, post(t, corrected.h.Preview, importRequest{FileBase64: file, Rows: []rowDecision{
		{Row: 2, Overrides: legacyimport.Overrides{BirthDate: sp("2021-02-19"), MobilePhone: sp("")}},
	}}))
	if s := sentRows(corrected)[0]; len(s.Issues) != 0 || s.Profile.BirthDate != "2021-02-19" {
		t.Errorf("corrected row sent = %+v, want no issue for the corrected fields", s)
	}
}

// The commit plans with the same decisions as the preview; what the decisions
// did to the rows is in the hash, so a commit with other decisions is stale.
func TestImportReviewCommitWritesWhatThePreviewDecided(t *testing.T) {
	f := newReviewFixture()
	file := importFile(t, pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"), draftless("A"))
	decided := []rowDecision{{Row: 3, Action: "draft"}}
	preview := decodePreview(t, post(t, f.h.Preview, importRequest{FileBase64: file, Rows: decided}))

	stale := post(t, f.h.Commit, importRequest{FileBase64: file, PlanHash: preview.PlanHash, Rows: []rowDecision{{Row: 3, Action: "omit"}}})
	if stale.Code != http.StatusConflict {
		t.Fatalf("commit with other decisions = %d, want 409", stale.Code)
	}

	got := decodePreview(t, post(t, f.h.Commit, importRequest{FileBase64: file, PlanHash: preview.PlanHash, Rows: decided}))
	if got.Summary.New != 2 || !got.Rows[1].Draft {
		t.Errorf("commit = %+v, want both written, row 3 as a draft", got.Rows)
	}
	last := f.roster.calls[len(f.roster.calls)-1]
	if last.validate || last.rows[1].Action != client.ImportActionDraft {
		t.Errorf("last call = %+v, want the commit carrying the draft", last)
	}
}
