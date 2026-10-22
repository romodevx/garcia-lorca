package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// PLAN-import-avisos through the real mux: a pupil's import issues need
// module_students AND module_import_review; reading them is students:read,
// accepting one is students:update.

const (
	importIssuesPath = "/api/v1/students/6f1c0e2a-0000-4000-8000-000000000001/import-issues"
	acceptIssuePath  = importIssuesPath + "/7d1e0c4e-0000-4000-8000-000000000001/accept"
)

func reviewFlags(off string) map[string]bool {
	flags := map[string]bool{"module_students": true, "module_import_review": true}
	delete(flags, off)
	return flags
}

func TestImportIssueRoutesHideWhenAnyOfTheirModulesIsOff(t *testing.T) {
	for _, off := range []string{"module_students", "module_import_review"} {
		gw := newTestGateway(t, reviewFlags(off))
		t.Run(off, func(t *testing.T) {
			assert.Equal(t, http.StatusNotFound, doGateway(t, gw, http.MethodGet, importIssuesPath, "student-updater", "").StatusCode)
			assert.Equal(t, http.StatusNotFound, doGateway(t, gw, http.MethodPost, acceptIssuePath, "student-updater", "").StatusCode)
		})
	}
}

func TestImportIssueRoutesAnswerToTheirPermissions(t *testing.T) {
	gw := newTestGateway(t, reviewFlags(""))
	assert.Equal(t, http.StatusForbidden, doGateway(t, gw, http.MethodGet, importIssuesPath, "student-enroller", "").StatusCode)
	assert.Equal(t, http.StatusForbidden, doGateway(t, gw, http.MethodPost, acceptIssuePath, "student-reader", "").StatusCode)
}

// The holder reaches the handler, which asks students (the stub has not
// implemented the RPC): neither the mux's 404 nor the guard's 403.
func TestImportIssueRoutesReachTheHandler(t *testing.T) {
	gw := newTestGateway(t, reviewFlags(""))
	for _, res := range []*http.Response{
		doGateway(t, gw, http.MethodGet, importIssuesPath, "student-reader", ""),
		doGateway(t, gw, http.MethodPost, acceptIssuePath, "student-updater", ""),
	} {
		assert.NotContains(t, []int{http.StatusNotFound, http.StatusForbidden, http.StatusUnauthorized}, res.StatusCode)
	}
}
