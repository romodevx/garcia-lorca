package handler_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// CC-304: a required id missing from the query (or not a uuid) is the
// caller's mistake, answered 400 in the shared error envelope naming the
// parameter — never forwarded, where the empty string reached Postgres as
// invalid uuid syntax and came back a 500.
func TestRequiredQueryIDsAreValidatedAtTheEdge(t *testing.T) {
	const okID = "8d7f6c1e-2b3a-4c5d-9e8f-0a1b2c3d4e5f"
	config := &fakeConfig{}
	finance := &fakeFinance{}
	grades := &fakeGrades{}
	configH := handler.NewConfigHandler(config)
	financeH := handler.NewFinanceHandler(finance)
	gradesH := handler.NewGradesHandler(grades)

	tests := []struct {
		name    string
		handle  http.HandlerFunc
		target  string
		field   string
		reached func() bool
	}{
		{"calendar without cycleId", configH.ListNonWorkingPeriods, "/api/v1/config/calendar", "cycleId", func() bool { return config.periodsCycle != "" }},
		{"calendar with a malformed cycleId", configH.ListNonWorkingPeriods, "/api/v1/config/calendar?cycleId=nope", "cycleId", func() bool { return config.periodsCycle != "" }},
		{"events without cycleId", configH.ListSchoolEvents, "/api/v1/config/events", "cycleId", func() bool { return config.eventsCycle != "" }},
		{"events with a blank cycleId", configH.ListSchoolEvents, "/api/v1/config/events?cycleId=%20", "cycleId", func() bool { return config.eventsCycle != "" }},
		{"expected without studentId", financeH.ExpectedToday, "/api/v1/finance/expected", "studentId", func() bool { return finance.expectedStudent != "" }},
		{"expected with a malformed studentId", financeH.ExpectedToday, "/api/v1/finance/expected?studentId=x", "studentId", func() bool { return finance.expectedStudent != "" }},
		{"roster without assignmentId", gradesH.GetRoster, "/api/v1/grades/roster?termId=" + okID, "assignmentId", func() bool { return grades.rosterAssignment != "" }},
		{"roster with a malformed termId", gradesH.GetRoster, "/api/v1/grades/roster?assignmentId=" + okID + "&termId=t1", "termId", func() bool { return grades.rosterAssignment != "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doReq(t, tt.handle, http.MethodGet, tt.target, "")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("content-type = %q, want the JSON envelope", ct)
			}
			var body struct {
				Error struct {
					Code    string            `json:"code"`
					Message string            `json:"message"`
					Details map[string]string `json:"details"`
				} `json:"error"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode envelope: %v", err)
			}
			if body.Error.Code != "invalid" || body.Error.Details["field"] != tt.field || !strings.Contains(body.Error.Message, tt.field) {
				t.Errorf("envelope = %+v, want code invalid naming %s", body.Error, tt.field)
			}
			if tt.reached() {
				t.Error("the request reached the service")
			}
		})
	}
}

// The roster's termId stays optional: an absent one is forwarded as before.
func TestRosterWithoutTermStillReachesTheService(t *testing.T) {
	const okID = "8d7f6c1e-2b3a-4c5d-9e8f-0a1b2c3d4e5f"
	grades := &fakeGrades{}
	rec := doReq(t, handler.NewGradesHandler(grades).GetRoster, http.MethodGet, "/api/v1/grades/roster?assignmentId="+okID, "")
	if rec.Code != http.StatusOK || grades.rosterAssignment != okID {
		t.Errorf("status = %d, forwarded %q", rec.Code, grades.rosterAssignment)
	}
}
