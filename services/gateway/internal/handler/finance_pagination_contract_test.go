package handler

import (
	"os"
	"path/filepath"
	"testing"

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
			Type    string `yaml:"type"`
			Minimum *int   `yaml:"minimum"`
			Maximum *int   `yaml:"maximum"`
		} `yaml:"schema"`
	}
	type operation struct {
		Parameters []param `yaml:"parameters"`
		Responses  map[string]struct {
			Content map[string]struct {
				Schema struct {
					Properties map[string]yaml.Node `yaml:"properties"`
				} `yaml:"schema"`
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
		if p, ok := params["pageToken"]; !ok || p.In != "query" || p.Schema.Type != "string" {
			t.Errorf("GET %s: pageToken = %+v, want a string query parameter", tc.path, p)
		}
		size, ok := params["pageSize"]
		if !ok || size.In != "query" || size.Schema.Type != "integer" ||
			size.Schema.Minimum == nil || *size.Schema.Minimum != 1 ||
			size.Schema.Maximum == nil || *size.Schema.Maximum != 500 {
			t.Errorf("GET %s: pageSize = %+v, want an integer query parameter in 1..500", tc.path, size)
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
