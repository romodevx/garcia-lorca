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
