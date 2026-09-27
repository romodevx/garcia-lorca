package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

func (f *fakeConfig) SetTermMonths(_ context.Context, termID string, months []string) (client.TermResult, error) {
	f.monthsTermID, f.monthsIn = termID, months
	if f.err != nil {
		return client.TermResult{}, f.err
	}
	return client.TermResult{ID: termID, CycleID: "cyc-1", LevelID: "lvl-1", Name: "Trimestre 1", Ordinal: 1, Months: months}, nil
}

func (f *fakeConfig) CopyTermMonths(_ context.Context, cycleID, fromLevelID string, toLevelIDs []string) ([]client.TermResult, error) {
	f.copyCycle, f.copyFrom, f.copyTo = cycleID, fromLevelID, toLevelIDs
	if f.err != nil {
		return nil, f.err
	}
	return []client.TermResult{{ID: "t-2", CycleID: cycleID, LevelID: toLevelIDs[0], Name: "Trimestre 1", Ordinal: 1, Months: []string{"2026-09"}}}, nil
}

func refusal(code connect.Code, meta, msg string) error {
	ce := connect.NewError(code, errors.New(msg))
	ce.Meta().Set("X-Error-Code", meta)
	return ce
}

func TestPutTermMonths(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		svcErr     error
		wantStatus int
		wantCode   string
		wantMonths []string
		forwarded  bool
	}{
		{name: "replaces the months", body: `{"months":["2026-09","2026-10"]}`, wantStatus: http.StatusOK, wantMonths: []string{"2026-09", "2026-10"}, forwarded: true},
		{name: "an empty list clears", body: `{"months":[]}`, wantStatus: http.StatusOK, wantMonths: []string{}, forwarded: true},
		{name: "a malformed month never leaves the edge", body: `{"months":["2026-9"]}`, wantStatus: http.StatusBadRequest, wantCode: "MONTH_INVALID"},
		{name: "a missing months field", body: `{}`, wantStatus: http.StatusBadRequest},
		{name: "a body that is not JSON", body: `nope`, wantStatus: http.StatusBadRequest},
		{
			name: "a month of another term", body: `{"months":["2026-09"]}`,
			svcErr:     refusal(connect.CodeAlreadyExists, "MONTH_IN_OTHER_TERM", "Ese mes ya pertenece a otro periodo del nivel: 2026-09"),
			wantStatus: http.StatusConflict, wantCode: "MONTH_IN_OTHER_TERM", forwarded: true,
		},
		{
			name: "a month outside the cycle", body: `{"months":["2030-01"]}`,
			svcErr:     refusal(connect.CodeInvalidArgument, "MONTH_OUTSIDE_CYCLE", "El mes está fuera del ciclo: 2030-01"),
			wantStatus: http.StatusBadRequest, wantCode: "MONTH_OUTSIDE_CYCLE", forwarded: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeConfig{err: tt.svcErr}
			h := handler.NewConfigHandler(svc)
			r := newPathRequest(http.MethodPut, "/api/v1/config/terms/trm-1/months", tt.body, "id", "trm-1")
			rec := serve(h.PutTermMonths, r)

			assert.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
			assert.Equal(t, tt.wantCode, rec.Header().Get("X-Error-Code"))
			assert.Equal(t, tt.forwarded, svc.monthsTermID == "trm-1")
			if tt.wantMonths == nil {
				return
			}
			var got map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			assert.Equal(t, "trm-1", got["id"])
			months, ok := got["months"].([]any)
			require.True(t, ok, "months is always a list, never null: %v", got["months"])
			assert.Len(t, months, len(tt.wantMonths))
		})
	}
}

func TestCopyTermMonthsHandler(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		forwarded  bool
	}{
		{"copies onto the target levels", `{"cycleId":"cyc-1","fromLevelId":"lvl-1","toLevelIds":["lvl-2","lvl-3"]}`, http.StatusOK, true},
		{"a missing cycle", `{"fromLevelId":"lvl-1","toLevelIds":["lvl-2"]}`, http.StatusBadRequest, false},
		{"a missing source", `{"cycleId":"cyc-1","toLevelIds":["lvl-2"]}`, http.StatusBadRequest, false},
		{"no targets", `{"cycleId":"cyc-1","fromLevelId":"lvl-1","toLevelIds":[]}`, http.StatusBadRequest, false},
		{"a body that is not JSON", `nope`, http.StatusBadRequest, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeConfig{}
			h := handler.NewConfigHandler(svc)
			rec := doReq(t, h.CopyTermMonths, http.MethodPost, "/api/v1/config/terms/copy-months", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
			assert.Equal(t, tt.forwarded, svc.copyCycle == "cyc-1" && svc.copyFrom == "lvl-1")
			if !tt.forwarded {
				return
			}
			assert.Equal(t, []string{"lvl-2", "lvl-3"}, svc.copyTo)
			var got struct {
				Terms []struct {
					LevelID string   `json:"levelId"`
					Months  []string `json:"months"`
				} `json:"terms"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			require.Len(t, got.Terms, 1)
			assert.Equal(t, []string{"2026-09"}, got.Terms[0].Months)
		})
	}
}

func TestTermJSONCarriesMonths(t *testing.T) {
	svc := &fakeConfig{terms: []client.TermResult{
		{ID: "t-1", Name: "Trimestre 1", Ordinal: 1, Months: []string{"2026-09"}},
		{ID: "t-2", Name: "Trimestre 2", Ordinal: 2},
	}}
	h := handler.NewConfigHandler(svc)
	rec := doReq(t, h.ListTerms, http.MethodGet, "/api/v1/config/terms?cycleId=cyc-1", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `["2026-09"]`, mustField(t, rec.Body.Bytes(), 0))
	assert.JSONEq(t, `[]`, mustField(t, rec.Body.Bytes(), 1), "a term with no months answers [], not null")
}

func mustField(t *testing.T, body []byte, i int) string {
	t.Helper()
	var got struct {
		Terms []struct {
			Months json.RawMessage `json:"months"`
		} `json:"terms"`
	}
	require.NoError(t, json.Unmarshal(body, &got))
	require.Greater(t, len(got.Terms), i)
	return string(got.Terms[i].Months)
}

// newPathRequest builds a request with one path value bound. SetPathValue makes
// the URL cosmetic here; the real route pattern is pinned by cmd/main_test.go.
func newPathRequest(method, target, body, key, value string) *http.Request {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.SetPathValue(key, value)
	return r
}

func serve(hf http.HandlerFunc, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	hf(rec, r)
	return rec
}
