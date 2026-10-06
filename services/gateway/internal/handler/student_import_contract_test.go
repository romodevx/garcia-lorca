package handler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ADR 0033: both routes are in the contract, their failures are text/plain,
// and a result on the wire carries exactly what StudentImportResult requires.
func TestStudentImportContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Clean("../../../../docs/api/openapi.yaml"))
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	var doc struct {
		Paths      map[string]map[string]any `yaml:"paths"`
		Components struct {
			Schemas map[string]struct {
				Required []string `yaml:"required"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse contract: %v", err)
	}
	for _, path := range []string{"/api/v1/imports/students/preview", "/api/v1/imports/students/commit"} {
		if _, ok := doc.Paths[path]["post"]; !ok {
			t.Errorf("POST %s is not documented", path)
		}
	}

	f := newImportFixture()
	rec := post(t, f.h.Preview, importRequest{FileBase64: importFile(t, pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"))})
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := doc.Components.Schemas["StudentImportResult"].Required
	if len(want) == 0 {
		t.Fatal("StudentImportResult has no required fields in the contract")
	}
	var missing []string
	for _, k := range want {
		if _, ok := body[k]; !ok {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("the preview omits required fields: %s", strings.Join(missing, ", "))
	}
}
