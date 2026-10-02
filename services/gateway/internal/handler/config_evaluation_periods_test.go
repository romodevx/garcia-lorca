package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

const (
	evCycle = "00000000-0000-4000-8000-0000000000c1"
	evLevel = "00000000-0000-4000-8000-0000000000b1"
	evG1    = "00000000-0000-4000-8000-0000000000a1"
	evG2    = "00000000-0000-4000-8000-0000000000a2"
)

func evDay(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }

func (f *fakeConfig) ListEvaluationPeriods(_ context.Context, cycleID, levelID, gradeLevelID string) ([]client.EvaluationPeriod, error) {
	f.evalCalls++
	f.evalList = [3]string{cycleID, levelID, gradeLevelID}
	if f.err != nil {
		return nil, f.err
	}
	return []client.EvaluationPeriod{{ID: "p-1", CycleID: cycleID, GradeLevelID: evG1, Ordinal: 1, Name: "T1",
		StartsOn: evDay(time.September, 1), EndsOn: evDay(time.November, 30), TermID: "trm-1"}}, nil
}

func (f *fakeConfig) UpsertEvaluationPeriods(_ context.Context, cycleID, gradeLevelID string, in []client.EvaluationPeriodInput) ([]client.EvaluationPeriod, error) {
	f.evalCalls++
	f.evalCycle, f.evalGrade, f.evalIn = cycleID, gradeLevelID, in
	if f.err != nil {
		return nil, f.err
	}
	out := make([]client.EvaluationPeriod, 0, len(in))
	for _, p := range in {
		out = append(out, client.EvaluationPeriod{ID: "p", CycleID: cycleID, GradeLevelID: gradeLevelID, Ordinal: p.Ordinal, Name: p.Name, StartsOn: p.StartsOn, EndsOn: p.EndsOn})
	}
	return out, nil
}

func (f *fakeConfig) CopyEvaluationPeriods(_ context.Context, cycleID, from string, to []string) ([]client.EvaluationPeriod, error) {
	f.evalCalls++
	f.evalCycle, f.evalGrade, f.evalCopyTo = cycleID, from, to
	if f.err != nil {
		return nil, f.err
	}
	return []client.EvaluationPeriod{}, nil
}

type periodJSON struct {
	ID           string  `json:"id"`
	GradeLevelID string  `json:"gradeLevelId"`
	Ordinal      int     `json:"ordinal"`
	Name         string  `json:"name"`
	StartsOn     string  `json:"startsOn"`
	EndsOn       string  `json:"endsOn"`
	TermID       *string `json:"termId"`
}

func TestListEvaluationPeriods(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantScope  [3]string
	}{
		{name: "a level of a cycle", query: "?cycleId=" + evCycle + "&levelId=" + evLevel, wantStatus: http.StatusOK, wantScope: [3]string{evCycle, evLevel, ""}},
		{name: "one grade", query: "?cycleId=" + evCycle + "&gradeLevelId=" + evG1, wantStatus: http.StatusOK, wantScope: [3]string{evCycle, "", evG1}},
		{name: "no cycle", query: "?levelId=" + evLevel, wantStatus: http.StatusBadRequest},
		{name: "a malformed grade", query: "?cycleId=" + evCycle + "&gradeLevelId=x", wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeConfig{}
			rec := serve(handler.NewConfigHandler(svc).ListEvaluationPeriods, newPathRequest(http.MethodGet, "/api/v1/config/evaluation-periods"+tt.query, "", "", ""))
			require.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
			if tt.wantStatus != http.StatusOK {
				assert.Zero(t, svc.evalCalls, "a bad request never reaches config")
				return
			}
			assert.Equal(t, tt.wantScope, svc.evalList)
			var body struct{ Periods []periodJSON }
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Len(t, body.Periods, 1)
			assert.Equal(t, "2026-09-01", body.Periods[0].StartsOn)
			assert.Equal(t, "2026-11-30", body.Periods[0].EndsOn)
			require.NotNil(t, body.Periods[0].TermID)
			assert.Equal(t, "trm-1", *body.Periods[0].TermID)
		})
	}
}

func TestPutGradeEvaluationPeriods(t *testing.T) {
	ok := `{"cycleId":"` + evCycle + `","periods":[{"ordinal":1,"name":"T1","startsOn":"2026-09-01","endsOn":"2026-11-30"}]}`
	tests := []struct {
		name       string
		grade      string
		body       string
		svcErr     error
		wantStatus int
		wantCode   string
		forwarded  bool
	}{
		{name: "replaces the set", grade: evG1, body: ok, wantStatus: http.StatusOK, forwarded: true},
		{name: "an empty set clears", grade: evG1, body: `{"cycleId":"` + evCycle + `","periods":[]}`, wantStatus: http.StatusOK, forwarded: true},
		{name: "a malformed date never leaves the edge", grade: evG1, body: `{"cycleId":"` + evCycle + `","periods":[{"ordinal":1,"name":"T1","startsOn":"01/09/2026","endsOn":"2026-11-30"}]}`, wantStatus: http.StatusBadRequest, wantCode: "PERIOD_DATE_INVALID"},
		{name: "a missing periods field", grade: evG1, body: `{"cycleId":"` + evCycle + `"}`, wantStatus: http.StatusBadRequest},
		{name: "a malformed grade", grade: "g", body: ok, wantStatus: http.StatusBadRequest},
		{name: "a malformed cycle", grade: evG1, body: `{"cycleId":"c","periods":[]}`, wantStatus: http.StatusBadRequest},
		{name: "too many periods", grade: evG1, body: `{"cycleId":"` + evCycle + `","periods":[` + repeatPeriods(13) + `]}`, wantStatus: http.StatusBadRequest, wantCode: "TOO_MANY_PERIODS"},
		{
			name: "an overlap", grade: evG1, body: ok,
			svcErr:     refusal(connect.CodeAlreadyExists, "PERIOD_OVERLAP", "El periodo 1 (T1) se encima con el periodo 2 (T2)"),
			wantStatus: http.StatusConflict, wantCode: "PERIOD_OVERLAP", forwarded: true,
		},
		{
			name: "outside the cycle", grade: evG1, body: ok,
			svcErr:     refusal(connect.CodeInvalidArgument, "PERIOD_OUTSIDE_CYCLE", "«T1» queda fuera del ciclo"),
			wantStatus: http.StatusBadRequest, wantCode: "PERIOD_OUTSIDE_CYCLE", forwarded: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeConfig{err: tt.svcErr}
			r := newPathRequest(http.MethodPut, "/api/v1/config/grade-levels/"+tt.grade+"/evaluation-periods", tt.body, "id", tt.grade)
			rec := serve(handler.NewConfigHandler(svc).PutGradeEvaluationPeriods, r)
			assert.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
			assert.Equal(t, tt.wantCode, rec.Header().Get("X-Error-Code"))
			assert.Equal(t, tt.forwarded, svc.evalCalls == 1)
			if tt.forwarded && tt.svcErr == nil && len(svc.evalIn) > 0 {
				assert.Equal(t, evCycle, svc.evalCycle)
				assert.Equal(t, evG1, svc.evalGrade)
				assert.Equal(t, client.EvaluationPeriodInput{Ordinal: 1, Name: "T1", StartsOn: evDay(time.September, 1), EndsOn: evDay(time.November, 30)}, svc.evalIn[0])
			}
		})
	}
}

func repeatPeriods(n int) string {
	out := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			out += ","
		}
		out += `{"ordinal":1,"name":"T","startsOn":"2026-09-01","endsOn":"2026-09-01"}`
	}
	return out
}

func TestCopyEvaluationPeriods(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		forwarded  bool
	}{
		{name: "copies", body: `{"cycleId":"` + evCycle + `","fromGradeLevelId":"` + evG1 + `","toGradeLevelIds":["` + evG2 + `"]}`, wantStatus: http.StatusOK, forwarded: true},
		{name: "no targets", body: `{"cycleId":"` + evCycle + `","fromGradeLevelId":"` + evG1 + `","toGradeLevelIds":[]}`, wantStatus: http.StatusBadRequest},
		{name: "a malformed target", body: `{"cycleId":"` + evCycle + `","fromGradeLevelId":"` + evG1 + `","toGradeLevelIds":["x"]}`, wantStatus: http.StatusBadRequest},
		{name: "not JSON", body: `nope`, wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeConfig{}
			rec := serve(handler.NewConfigHandler(svc).CopyEvaluationPeriods, newPathRequest(http.MethodPost, "/api/v1/config/evaluation-periods/copy", tt.body, "", ""))
			assert.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
			assert.Equal(t, tt.forwarded, svc.evalCalls == 1)
			if tt.forwarded {
				assert.Equal(t, []string{evG2}, svc.evalCopyTo)
				assert.JSONEq(t, `{"periods":[]}`, rec.Body.String())
			}
		})
	}
}
