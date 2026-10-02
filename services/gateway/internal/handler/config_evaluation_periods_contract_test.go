package handler_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// T14: the evaluation-period routes are in the contract, answer failures as
// text/plain like the rest of the config chain, and a period on the wire has
// exactly the fields ConfigEvaluationPeriod requires.
func TestEvaluationPeriodsContract(t *testing.T) {
	doc := loadOpenAPI(t)
	for _, op := range []struct{ path, method string }{
		{"/api/v1/config/evaluation-periods", "get"},
		{"/api/v1/config/grade-levels/{id}/evaluation-periods", "put"},
		{"/api/v1/config/evaluation-periods/copy", "post"},
	} {
		node, ok := doc.Paths[op.path][op.method]
		require.True(t, ok, "%s %s is documented", op.method, op.path)
		var operation openAPIOperation
		require.NoError(t, node.Decode(&operation))
		for status, response := range operation.Responses {
			if strings.HasPrefix(status, "4") {
				assert.Contains(t, doc.resolve(t, response).Content, "text/plain", "%s %s %s", op.method, op.path, status)
			}
		}
	}

	raw, err := os.ReadFile(filepath.Clean(openAPIPath))
	require.NoError(t, err)
	var schemas struct {
		Components struct {
			Schemas map[string]struct {
				Required []string `yaml:"required"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &schemas))
	want := schemas.Components.Schemas["ConfigEvaluationPeriod"].Required
	require.NotEmpty(t, want)

	rec := serve(handler.NewConfigHandler(&fakeConfig{}).ListEvaluationPeriods,
		newPathRequest(http.MethodGet, "/api/v1/config/evaluation-periods?cycleId="+evCycle, "", "", ""))
	require.Equal(t, http.StatusOK, rec.Code)
	var body struct{ Periods []map[string]any }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Periods, 1)
	got := make([]string, 0, len(body.Periods[0]))
	for k := range body.Periods[0] {
		got = append(got, k)
	}
	sort.Strings(got)
	sort.Strings(want)
	assert.Equal(t, want, got)
}
