package handler

import (
	"slices"
	"testing"
)

// ADR 0041 (T5): the contract names "family" as an announcement audience.
func TestAnnouncementContractNamesTheFamilyScope(t *testing.T) {
	doc := readFamilyContract(t)
	node, ok := doc.Paths["/api/v1/comms/announcements"]["post"]
	if !ok {
		t.Fatal("POST /api/v1/comms/announcements is not documented")
	}
	var op struct {
		RequestBody struct {
			Content map[string]struct {
				Schema struct {
					Properties map[string]struct {
						Enum []string `yaml:"enum"`
					} `yaml:"properties"`
				} `yaml:"schema"`
				Examples map[string]struct {
					Value map[string]any `yaml:"value"`
				} `yaml:"examples"`
			} `yaml:"content"`
		} `yaml:"requestBody"`
	}
	if err := node.Decode(&op); err != nil {
		t.Fatalf("decode: %v", err)
	}
	media := op.RequestBody.Content["application/json"]
	props := media.Schema.Properties
	if !slices.Contains(props["scope"].Enum, scopeFamily) {
		t.Errorf("scope enum = %v, want it to name %q", props["scope"].Enum, scopeFamily)
	}
	for _, s := range []string{scopeStudent, scopeGroup, scopeLevel, scopeCycle} {
		if !slices.Contains(props["scope"].Enum, s) {
			t.Errorf("scope enum lost %q", s)
		}
	}
	documented := false
	for _, ex := range media.Examples {
		ids, _ := ex.Value["targetIds"].([]any)
		if ex.Value["scope"] == scopeFamily && len(ids) > 0 && len(ids) <= maxAnnouncementFamilies {
			documented = true
		}
	}
	if !documented {
		t.Error("no request example addresses families")
	}
}
