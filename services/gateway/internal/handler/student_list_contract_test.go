package handler

import "testing"

// CC-211: the list is in the contract, and its pupils carry the classroom
// label next to the bare letter, without the profile.
func TestStudentListDocumentsTheGroupLabel(t *testing.T) {
	doc := readFamilyContract(t)
	op, ok := doc.get(t, "/api/v1/students")
	if !ok {
		t.Fatal("GET /api/v1/students is not documented")
	}
	if _, cookie := firstSecurity(op)["sessionCookie"]; !cookie {
		t.Errorf("security = %v, want sessionCookie", op.Security)
	}
	for _, code := range []string{"200", "401", "403", "404"} {
		if _, ok := op.Responses[code]; !ok {
			t.Errorf("GET /api/v1/students: no %s response", code)
		}
	}
	if !documentsQueryParam(t, op, "include") {
		t.Error("GET /api/v1/students does not document ?include=groupLabel: the label is opt-in")
	}
	summary, ok := doc.Components.Schemas["StudentSummary"]
	if !ok {
		t.Fatal("the StudentSummary schema is not documented")
	}
	for _, field := range []string{"gradeGroup", "groupLabel"} {
		if _, ok := summary.Properties[field]; !ok {
			t.Errorf("StudentSummary has no %s", field)
		}
	}
	if _, ok := summary.Properties["profile"]; ok {
		t.Error("StudentSummary documents a profile; the list never carries one")
	}
}

// documentsQueryParam reports whether op declares the query parameter name inline.
func documentsQueryParam(t *testing.T, op contractOperation, name string) bool {
	t.Helper()
	for _, node := range op.Parameters {
		var p struct {
			Name string `yaml:"name"`
			In   string `yaml:"in"`
		}
		if err := node.Decode(&p); err != nil {
			t.Fatalf("decode parameter: %v", err)
		}
		if p.Name == name && p.In == "query" {
			return true
		}
	}
	return false
}
