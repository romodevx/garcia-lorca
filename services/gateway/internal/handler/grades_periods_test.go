package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

const (
	pTerm  = "70000000-0000-0000-0000-000000000001"
	pCycle = "c0000000-0000-0000-0000-000000000001"
	pDir   = "d0000000-0000-0000-0000-000000000001"
)

var pClosedAt = time.Date(2026, 12, 1, 10, 0, 0, 0, time.UTC)

type fakePeriods struct {
	periods  []client.Period
	progress client.PeriodProgress
	closeRes client.ClosePeriodResult
	err      error

	gotCycle, gotTerm, gotReopenActor string
	gotReopen                         client.ReopenPeriodInput
	gotClose                          client.ClosePeriodInput
	calls                             int
}

func (f *fakePeriods) ListPeriods(_ context.Context, cycleID string) ([]client.Period, error) {
	f.calls++
	f.gotCycle = cycleID
	return f.periods, f.err
}

func (f *fakePeriods) GetPeriodProgress(_ context.Context, termID string) (client.PeriodProgress, error) {
	f.calls++
	f.gotTerm = termID
	return f.progress, f.err
}

func (f *fakePeriods) ClosePeriod(_ context.Context, in client.ClosePeriodInput) (client.ClosePeriodResult, error) {
	f.calls++
	f.gotClose = in
	return f.closeRes, f.err
}

func (f *fakePeriods) ReopenPeriod(_ context.Context, in client.ReopenPeriodInput) (string, error) {
	f.calls++
	f.gotTerm, f.gotReopenActor, f.gotReopen = in.TermID, in.ActorID, in
	if f.err != nil {
		return "", f.err
	}
	return "open", nil
}

type fakeUserNames struct{ name string }

func (f fakeUserNames) GetUser(_ context.Context, id string) (client.User, error) {
	return client.User{ID: id, DisplayName: f.name}, nil
}

func closerReq(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	return req.WithContext(middleware.WithPrincipal(req.Context(), middleware.Principal{UserID: pDir, Email: "dir@colegio.mx", TenantID: "t1"}))
}

func withTerm(req *http.Request, term string) *http.Request {
	req.SetPathValue("termId", term)
	return req
}

func TestListPeriodsShape(t *testing.T) {
	f := &fakePeriods{periods: []client.Period{
		{TermID: pTerm, TermName: "1.er trimestre", Ordinal: 1, LevelID: "lvl-1", LevelName: "Primaria", StartsOn: "2026-08-24", EndsOn: "2026-11-30",
			Status: "closed", ClosedAt: pClosedAt, ClosedByName: "Directora Ruiz", Version: 2, Captured: 7, Expected: 9},
		{TermID: "term-2", TermName: "2.º trimestre", Ordinal: 2, LevelID: "lvl-1", LevelName: "Primaria", Status: "open"},
	}}
	h := handler.NewGradesPeriodsHandler(f, nil)
	rec := httptest.NewRecorder()
	h.List(rec, closerReq(http.MethodGet, "/api/v1/grades/periods?cycleId="+strings.ToUpper(pCycle), ""))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, pCycle, f.gotCycle, "canonicalised")
	assert.JSONEq(t, `{"periods":[
		{"termId":"`+pTerm+`","termName":"1.er trimestre","ordinal":1,"levelId":"lvl-1","levelName":"Primaria","startsOn":"2026-08-24","endsOn":"2026-11-30",
		 "status":"closed","closedAt":"2026-12-01T10:00:00Z","closedByName":"Directora Ruiz","version":2,"progress":{"captured":7,"expected":9}},
		{"termId":"term-2","termName":"2.º trimestre","ordinal":2,"levelId":"lvl-1","levelName":"Primaria","startsOn":null,"endsOn":null,
		 "status":"open","version":0,"progress":{"captured":0,"expected":0}}
	]}`, rec.Body.String())

	for _, bad := range []string{"", "?cycleId=nope"} {
		rec := httptest.NewRecorder()
		h.List(rec, closerReq(http.MethodGet, "/api/v1/grades/periods"+bad, ""))
		assert.Equal(t, http.StatusBadRequest, rec.Code, bad)
	}
}

func TestPeriodProgressShape(t *testing.T) {
	f := &fakePeriods{progress: client.PeriodProgress{TermID: pTerm, Unfinished: true, EndsOn: "2026-11-30", GroupsWithoutPeriod: []string{"2° · A"}, Expected: 9, Captured: 7,
		ReportCards: 30, SkippedWithoutGrades: 1, WithdrawnReportCards: 2,
		Missing: []client.MissingGrades{{GroupID: "grp-1", GroupLabel: "1° · A", SubjectID: "sub-1", SubjectName: "Matemáticas", TeacherName: "Gabriela Rojas", MissingCount: 2}}}}
	h := handler.NewGradesPeriodsHandler(f, nil)
	rec := httptest.NewRecorder()
	h.Progress(rec, withTerm(closerReq(http.MethodGet, "/x", ""), pTerm))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"termId":"`+pTerm+`","unfinished":true,"endsOn":"2026-11-30","groupsWithoutPeriod":["2° · A"],"expected":9,"captured":7,"reportCards":30,"skippedWithoutGrades":1,"withdrawnReportCards":2,
		"missing":[{"groupId":"grp-1","groupLabel":"1° · A","subjectId":"sub-1","subjectName":"Matemáticas","teacherName":"Gabriela Rojas","missingCount":2}]}`, rec.Body.String())

	rec = httptest.NewRecorder()
	h.Progress(rec, withTerm(closerReq(http.MethodGet, "/x", ""), "not-a-uuid"))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, 1, f.calls, "a malformed term never reaches grades")
}

// The close carries the authenticated director (id + display name) and the
// force flag; a refusal is 409 with its machine code.
func TestClosePeriod(t *testing.T) {
	f := &fakePeriods{closeRes: client.ClosePeriodResult{Status: "closed", Version: 1, ReportCards: 31, SkippedWithoutGrades: 1, WithdrawnReportCards: 1}}
	h := handler.NewGradesPeriodsHandler(f, fakeUserNames{name: "Directora Ruiz"})
	rec := httptest.NewRecorder()
	h.Close(rec, withTerm(closerReq(http.MethodPost, "/x", `{"force":true}`), strings.ToUpper(pTerm)))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"status":"closed","version":1,"reportCards":31,"skippedWithoutGrades":1,"withdrawnReportCards":1}`, rec.Body.String())
	assert.Equal(t, client.ClosePeriodInput{TermID: pTerm, Force: true, ActorID: pDir, ActorName: "Directora Ruiz"}, f.gotClose)

	rec = httptest.NewRecorder()
	h.Close(rec, withTerm(closerReq(http.MethodPost, "/x", ""), pTerm))
	require.Equal(t, http.StatusOK, rec.Code, "an empty body means force:false")
	assert.False(t, f.gotClose.Force)

	rec = httptest.NewRecorder()
	h.Close(rec, withTerm(closerReq(http.MethodPost, "/x", `{"force":"yes"}`), pTerm))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	for code, msg := range map[string]string{
		"MISSING_GRADES":      "Faltan 3 calificaciones por capturar",
		"TERM_WITHOUT_MONTHS": "El periodo no tiene meses asignados",
		"ALREADY_CLOSED":      "El periodo ya está cerrado",
	} {
		ce := connect.NewError(connect.CodeAlreadyExists, errors.New(msg))
		ce.Meta().Set("X-Error-Code", code)
		f.err = ce
		rec := httptest.NewRecorder()
		h.Close(rec, withTerm(closerReq(http.MethodPost, "/x", `{}`), pTerm))
		assert.Equal(t, http.StatusConflict, rec.Code, code)
		assert.Equal(t, code, rec.Header().Get("X-Error-Code"))
		assert.Contains(t, rec.Body.String(), msg)
	}
}

func TestReopenPeriod(t *testing.T) {
	f := &fakePeriods{}
	h := handler.NewGradesPeriodsHandler(f, nil)
	rec := httptest.NewRecorder()
	h.Reopen(rec, withTerm(closerReq(http.MethodPost, "/x", `{"reason":"  Corrección de Matemáticas  "}`), pTerm))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"status":"open"}`, rec.Body.String())
	assert.Equal(t, pDir, f.gotReopenActor)
	assert.Equal(t, "Corrección de Matemáticas", f.gotReopen.Reason, "CC-308: the reason travels, trimmed")

	ce := connect.NewError(connect.CodeAlreadyExists, errors.New("El periodo ya está abierto"))
	ce.Meta().Set("X-Error-Code", "ALREADY_OPEN")
	f.err = ce
	rec = httptest.NewRecorder()
	h.Reopen(rec, withTerm(closerReq(http.MethodPost, "/x", `{"reason":"otra vez"}`), pTerm))
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, "ALREADY_OPEN", rec.Header().Get("X-Error-Code"))
}

// CC-308: a reopen without a reason (no body, blank, or over 500 characters)
// is 400 in the error envelope naming the field, and grades is never called.
func TestReopenPeriodRequiresAReason(t *testing.T) {
	for name, body := range map[string]string{
		"no body":   "",
		"no reason": `{}`,
		"blank":     `{"reason":"   "}`,
		"too long":  `{"reason":"` + strings.Repeat("a", 501) + `"}`,
		"not json":  `motivo`,
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakePeriods{}
			rec := httptest.NewRecorder()
			handler.NewGradesPeriodsHandler(f, nil).Reopen(rec, withTerm(closerReq(http.MethodPost, "/x", body), pTerm))
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), `"field":"reason"`)
			assert.Zero(t, f.calls, "grades is not called")
		})
	}
}

func TestPeriodWritesWithoutPrincipalAre401(t *testing.T) {
	h := handler.NewGradesPeriodsHandler(&fakePeriods{}, nil)
	for name, serve := range map[string]http.HandlerFunc{"close": h.Close, "reopen": h.Reopen} {
		rec := httptest.NewRecorder()
		serve(rec, withTerm(httptest.NewRequest(http.MethodPost, "/x", nil), pTerm))
		assert.Equal(t, http.StatusUnauthorized, rec.Code, name)
	}
}
