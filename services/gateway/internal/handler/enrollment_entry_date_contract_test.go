package handler

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// T9: both enrol routes document the optional entry day the alta now sends,
// and the 403 a past day without finance:write answers.
func TestEnrollRoutesDocumentTheEntryDate(t *testing.T) {
	raw, err := os.ReadFile(filepath.Clean("../../../../docs/api/openapi.yaml"))
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	type schema struct {
		Required   []string `yaml:"required"`
		Properties map[string]struct {
			Type   string `yaml:"type"`
			Format string `yaml:"format"`
		} `yaml:"properties"`
	}
	type operation struct {
		RequestBody struct {
			Content map[string]struct {
				Schema schema `yaml:"schema"`
			} `yaml:"content"`
		} `yaml:"requestBody"`
		Responses map[string]any `yaml:"responses"`
	}
	var doc struct {
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse contract: %v", err)
	}
	for _, path := range []string{"/api/v1/enrollments", "/api/v1/enrollments/by-grade"} {
		node, ok := doc.Paths[path]["post"]
		if !ok {
			t.Errorf("POST %s is not documented", path)
			continue
		}
		var op operation
		if err := node.Decode(&op); err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		body := op.RequestBody.Content["application/json"].Schema
		field, ok := body.Properties["enrolledOn"]
		if !ok || field.Type != "string" || field.Format != "date" {
			t.Errorf("POST %s: enrolledOn = %+v, want an optional string date", path, field)
		}
		for _, required := range body.Required {
			if required == "enrolledOn" {
				t.Errorf("POST %s: enrolledOn must be optional", path)
			}
		}
		for _, code := range []string{"201", "400", "403"} {
			if _, ok := op.Responses[code]; !ok {
				t.Errorf("POST %s does not document %s", path, code)
			}
		}
	}
}
