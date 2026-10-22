package handler

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/legacyimport"
)

func postRaw(fn http.HandlerFunc, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	fn(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(body))))
	return rec
}

// The contract closes a decision and its overrides (additionalProperties:
// false): a key this gateway does not know is refused by name, never ignored.
func TestImportReviewRefusesUnknownKeysInADecision(t *testing.T) {
	cases := []struct {
		name, rows string
		want       []rowError
	}{
		{"unknown override", `[{"row":2,"overrides":{"phone":"3311112222"}}]`,
			[]rowError{{Row: 2, Field: "phone", Code: codeOverrideUnknown}}},
		{"unknown decision key", `[{"row":2,"bogus":1}]`,
			[]rowError{{Row: 2, Field: "bogus", Code: codeOverrideUnknown}}},
		{"several, in key order", `[{"row":2,"zz":1,"overrides":{"yy":"","curp":"AAMI210219MJCLLRA0","aa":""}}]`,
			[]rowError{{Row: 2, Field: "aa", Code: codeOverrideUnknown}, {Row: 2, Field: "yy", Code: codeOverrideUnknown},
				{Row: 2, Field: "zz", Code: codeOverrideUnknown}}},
		{"an override that is not text", `[{"row":2,"overrides":{"mobilePhone":5}}]`,
			[]rowError{{Row: 2, Field: "mobilePhone", Code: codeOverrideMalformed}}},
		{"a row that is not a number", `[{"row":"dos"}]`,
			[]rowError{{Row: 0, Field: fieldRow, Code: codeRowMalformed}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newReviewFixture()
			file := importFile(t, pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"))
			rec := postRaw(f.h.Preview, fmt.Sprintf(`{"fileBase64":%q,"rows":%s}`, file, tc.rows))
			if got := rowErrors(t, rec, rec.Body.Bytes()); !slices.Equal(got, tc.want) {
				t.Errorf("errors = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// Off, the decisions are not read at all: a key the review would refuse
// changes nothing.
func TestImportReviewOffIgnoresUnknownKeys(t *testing.T) {
	f := newImportFixture()
	file := importFile(t, pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"))
	rec := postRaw(f.h.Preview, fmt.Sprintf(`{"fileBase64":%q,"rows":[{"row":"x","overrides":{"phone":1}}]}`, file))
	decodePreview(t, rec)
}

func TestImportReviewBoundsTheDecisions(t *testing.T) {
	f := newReviewFixture()
	file := importFile(t, pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"))
	many := make([]rowDecision, maxRowDecisions+1)
	for i := range many {
		many[i] = rowDecision{Row: 2}
	}
	rec := post(t, f.h.Preview, importRequest{FileBase64: file, Rows: many})
	if got := rowErrors(t, rec, rec.Body.Bytes()); !slices.Equal(got, []rowError{{Row: 0, Field: "rows", Code: codeTooManyDecisions}}) {
		t.Errorf("errors = %+v", got)
	}

	unknown := make([]rowDecision, maxRowDecisions)
	for i := range unknown {
		unknown[i] = rowDecision{Row: 1000 + i}
	}
	rec = post(t, f.h.Preview, importRequest{FileBase64: file, Rows: unknown})
	if got := rowErrors(t, rec, rec.Body.Bytes()); len(got) != maxRowErrors {
		t.Errorf("%d errors answered, want them capped at %d", len(got), maxRowErrors)
	}
}

// The decisions are part of what the office approved: a commit that corrects
// a value differently from the preview is stale, even when no row's fate moves.
func TestImportReviewCommitWithOtherCorrectionsIsStale(t *testing.T) {
	f := newReviewFixture()
	file := importFile(t, pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"))
	at := func(phone string) []rowDecision {
		return []rowDecision{{Row: 2, Overrides: legacyimport.Overrides{MobilePhone: sp(phone)}}}
	}
	preview := decodePreview(t, post(t, f.h.Preview, importRequest{FileBase64: file, Rows: at("33 1111 2222")}))

	stale := post(t, f.h.Commit, importRequest{FileBase64: file, PlanHash: preview.PlanHash, Rows: at("3399998888")})
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Header().Get("X-Error-Code"), codePreviewStale) {
		t.Fatalf("commit with another correction = %d %q, want 409 PREVIEW_STALE", stale.Code, stale.Header().Get("X-Error-Code"))
	}
	same := post(t, f.h.Commit, importRequest{FileBase64: file, PlanHash: preview.PlanHash, Rows: at("3311112222")})
	if same.Code != http.StatusOK {
		t.Errorf("the same correction, written another way = %d %s, want 200", same.Code, same.Body.String())
	}
}

// Off, the hash is the one it always was: no decisions digest is mixed in.
func TestImportReviewOffKeepsThePlanHash(t *testing.T) {
	file := importFile(t, pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"))
	off := decodePreview(t, post(t, newImportFixture().h.Preview, importRequest{FileBase64: file,
		Rows: []rowDecision{{Row: 2, Action: "omit"}}}))
	on := decodePreview(t, post(t, newReviewFixture().h.Preview, importRequest{FileBase64: file}))
	if off.PlanHash != on.PlanHash {
		t.Errorf("hash off %q vs on-without-decisions %q, want the same", off.PlanHash, on.PlanHash)
	}
}
