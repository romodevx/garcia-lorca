package handler

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// T14: the cycle-wide charge reads are paged. The contract documents the cursor
// (pageToken in, nextPageToken out) and the 1..500 bound on pageSize, so a client
// built from it walks every page instead of trusting the first.
func TestPagedFinanceReadsDocumentTheCursor(t *testing.T) {
	raw, err := os.ReadFile(filepath.Clean("../../../../docs/api/openapi.yaml"))
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	type param struct {
		Name   string `yaml:"name"`
		In     string `yaml:"in"`
		Schema struct {
			Type      string `yaml:"type"`
			Minimum   *int   `yaml:"minimum"`
			Maximum   *int   `yaml:"maximum"`
			MaxLength *int   `yaml:"maxLength"`
		} `yaml:"schema"`
	}
	type operation struct {
		Parameters []param `yaml:"parameters"`
		Responses  map[string]struct {
			Content map[string]struct {
				Schema struct {
					Properties map[string]yaml.Node `yaml:"properties"`
				} `yaml:"schema"`
				Examples map[string]struct {
					Value map[string]any `yaml:"value"`
				} `yaml:"examples"`
			} `yaml:"content"`
		} `yaml:"responses"`
	}
	var doc struct {
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse contract: %v", err)
	}
	cases := []struct {
		path, listKey string
	}{
		{"/api/v1/finance/charges", "charges"},
		{"/api/v1/finance/overdue", "overdue"},
	}
	for _, tc := range cases {
		node, ok := doc.Paths[tc.path]["get"]
		if !ok {
			t.Errorf("GET %s is not documented", tc.path)
			continue
		}
		var op operation
		if err := node.Decode(&op); err != nil {
			t.Fatalf("GET %s: %v", tc.path, err)
		}
		params := map[string]param{}
		for _, p := range op.Parameters {
			params[p.Name] = p
		}
		if p, ok := params["pageToken"]; !ok || p.In != "query" || p.Schema.Type != "string" ||
			p.Schema.MaxLength == nil || *p.Schema.MaxLength != maxFinancePageTokenLen {
			t.Errorf("GET %s: pageToken = %+v, want a string query parameter of at most %d", tc.path, p, maxFinancePageTokenLen)
		}
		size, ok := params["pageSize"]
		if !ok || size.In != "query" || size.Schema.Type != "integer" ||
			size.Schema.Minimum == nil || *size.Schema.Minimum != 1 ||
			size.Schema.Maximum == nil || *size.Schema.Maximum != maxFinancePageSize {
			t.Errorf("GET %s: pageSize = %+v, want an integer query parameter in 1..%d", tc.path, size, maxFinancePageSize)
		}
		for name, ex := range op.Responses["200"].Content["application/json"].Examples {
			requireMintableExampleToken(t, tc.path+" "+name, ex.Value, tc.listKey)
		}
		body := op.Responses["200"].Content["application/json"].Schema
		if _, ok := body.Properties[tc.listKey]; !ok {
			t.Errorf("GET %s: 200 lacks %q", tc.path, tc.listKey)
		}
		if _, ok := body.Properties["nextPageToken"]; !ok {
			t.Errorf("GET %s: 200 lacks nextPageToken", tc.path)
		}
		for _, code := range []string{"200", "400", "401", "403", "404"} {
			if _, ok := op.Responses[code]; !ok {
				t.Errorf("GET %s does not document %s", tc.path, code)
			}
		}
	}
}

// requireMintableExampleToken: an example nextPageToken is one finance could
// have minted for that page — unpadded URL-safe base64 of "YYYY-MM-DD|<uuid>",
// the (dueDate, id) of the page's last row. A token a client copies out of the
// docs must be one the server accepts, not an InvalidArgument.
func requireMintableExampleToken(t *testing.T, where string, value map[string]any, listKey string) {
	t.Helper()
	token, _ := value["nextPageToken"].(string)
	if token == "" {
		return
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Errorf("%s: nextPageToken %q is not unpadded URL-safe base64: %v", where, token, err)
		return
	}
	day, id, ok := strings.Cut(string(raw), "|")
	canonical, isUUID := canonicalUUID(id)
	if _, err := time.Parse("2006-01-02", day); !ok || err != nil || !isUUID || canonical != id {
		t.Errorf("%s: nextPageToken decodes to %q, want \"YYYY-MM-DD|<uuid>\"", where, raw)
		return
	}
	rows, _ := value[listKey].([]any)
	if len(rows) == 0 {
		t.Errorf("%s: example has a nextPageToken but no %s", where, listKey)
		return
	}
	last, _ := rows[len(rows)-1].(map[string]any)
	if last["id"] != id || last["dueDate"] != day {
		t.Errorf("%s: nextPageToken points at (%s, %s), want the last row's (%v, %v)", where, day, id, last["dueDate"], last["id"])
	}
}
