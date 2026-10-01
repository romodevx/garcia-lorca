package handler_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// The teaching-load routes answer every failure the way the rest of the
// ConfigHandler and its middleware chain do (ModuleGuard's http.NotFound,
// RequirePermission's "forbidden", WriteConnectError forwarding config's own
// sentence): text/plain. The contract must say so instead of promising the
// JSON envelope only the uniform shop speaks. Found by /code-review on 2026-09-25.

const openAPIPath = "../../../../docs/api/openapi.yaml"

type openAPIDoc struct {
	Paths      map[string]map[string]yaml.Node `yaml:"paths"` // path-level parameters are a list
	Components struct {
		Responses map[string]openAPIResponse `yaml:"responses"`
	} `yaml:"components"`
}

type openAPIOperation struct {
	Responses map[string]openAPIResponse `yaml:"responses"`
}

type openAPIResponse struct {
	Ref     string         `yaml:"$ref"`
	Content map[string]any `yaml:"content"`
}

func loadOpenAPI(t *testing.T) openAPIDoc {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(openAPIPath))
	require.NoError(t, err)
	var doc openAPIDoc
	require.NoError(t, yaml.Unmarshal(raw, &doc))
	return doc
}

func (d openAPIDoc) resolve(t *testing.T, r openAPIResponse) openAPIResponse {
	t.Helper()
	const prefix = "#/components/responses/"
	if r.Ref == "" {
		return r
	}
	require.True(t, strings.HasPrefix(r.Ref, prefix), "unexpected $ref %q", r.Ref)
	resolved, ok := d.Components.Responses[strings.TrimPrefix(r.Ref, prefix)]
	require.True(t, ok, "dangling $ref %q", r.Ref)
	return resolved
}

func TestTeachingLoadContractDocumentsPlainTextErrors(t *testing.T) {
	doc := loadOpenAPI(t)
	operations := []struct{ path, method string }{
		{"/api/v1/teaching-assignments/bulk", "post"},
		{"/api/v1/teaching-assignments/{id}", "delete"},
		{"/api/v1/teaching-assignments/{id}", "patch"},
	}
	for _, op := range operations {
		node, ok := doc.Paths[op.path][op.method]
		require.True(t, ok, "%s %s is documented", op.method, op.path)
		var operation openAPIOperation
		require.NoError(t, node.Decode(&operation))
		for status, response := range operation.Responses {
			if !strings.HasPrefix(status, "4") {
				continue
			}
			content := doc.resolve(t, response).Content
			assert.Contains(t, content, "text/plain", "%s %s %s", op.method, op.path, status)
			assert.NotContains(t, content, "application/json", "%s %s %s", op.method, op.path, status)
		}
	}
}

func TestTeachingLoadErrorsAreTextPlain(t *testing.T) {
	const configSays = "items[0]: group not found"
	h := handler.NewConfigHandler(&fakeConfig{err: connect.NewError(connect.CodeNotFound, errString(configSays))})

	badBody := doReq(t, h.AssignTeachingLoad, http.MethodPost, "/api/v1/teaching-assignments/bulk", `{`)
	upstream := doPathReq(t, h.DeleteTeachingAssignment, http.MethodDelete, "/api/v1/teaching-assignments/"+asgA, asgA, "")

	assert.Equal(t, http.StatusBadRequest, badBody.Code)
	assert.True(t, strings.HasPrefix(badBody.Header().Get("Content-Type"), "text/plain"))
	assert.Equal(t, http.StatusNotFound, upstream.Code)
	assert.True(t, strings.HasPrefix(upstream.Header().Get("Content-Type"), "text/plain"))
	assert.Equal(t, configSays, strings.TrimSpace(upstream.Body.String()))
}

// T10: the Egresos routes answer their failures as text/plain too
// (http.Error and WriteConnectError), and the contract says so.
func TestExpensesContractDocumentsPlainTextErrors(t *testing.T) {
	doc := loadOpenAPI(t)
	operations := []struct{ path, method string }{
		{"/api/v1/expenses", "get"}, {"/api/v1/expenses", "post"},
		{"/api/v1/expenses/{id}", "get"}, {"/api/v1/expenses/{id}", "put"},
		{"/api/v1/expenses/{id}/cancel", "post"},
		{"/api/v1/expenses/{id}/receipt", "get"}, {"/api/v1/expenses/{id}/receipt", "put"}, {"/api/v1/expenses/{id}/receipt", "delete"},
		{"/api/v1/expense-categories", "get"}, {"/api/v1/expense-categories", "post"}, {"/api/v1/expense-categories/{id}", "put"},
		{"/api/v1/expenses/summary", "get"}, {"/api/v1/expenses/reports", "get"},
		{"/api/v1/expenses/reports/pdf", "get"}, {"/api/v1/expenses/reports/xlsx", "get"},
		{"/api/v1/finance/cash-up/close", "post"}, {"/api/v1/finance/cash-up/reopen", "post"},
	}
	for _, op := range operations {
		node, ok := doc.Paths[op.path][op.method]
		require.True(t, ok, "%s %s is documented", op.method, op.path)
		var operation openAPIOperation
		require.NoError(t, node.Decode(&operation))
		for status, response := range operation.Responses {
			if !strings.HasPrefix(status, "4") || status == "413" {
				continue
			}
			content := doc.resolve(t, response).Content
			assert.Contains(t, content, "text/plain", "%s %s %s", op.method, op.path, status)
		}
	}
}
