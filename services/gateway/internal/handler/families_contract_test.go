package handler

import (
	"slices"
	"strings"
	"testing"
)

// ADR 0041 (T3): every families route is in the contract, behind the session
// cookie, with a CSRF header on each write, and answering 404 when
// module_families or module_students is off.

type familiesRoute struct {
	method, path string
	write        bool
}

var familiesRoutes = []familiesRoute{
	{"get", "/api/v1/families", false},
	{"post", "/api/v1/families", true},
	{"get", "/api/v1/families/suggestions", false},
	{"post", "/api/v1/families/suggestions/dismiss", true},
	{"get", "/api/v1/families/{familyId}", false},
	{"patch", "/api/v1/families/{familyId}", true},
	{"delete", "/api/v1/families/{familyId}", true},
	{"get", "/api/v1/families/{familyId}/contacts", false},
	{"post", "/api/v1/families/{familyId}/contacts", true},
	{"post", "/api/v1/families/{familyId}/students", true},
	{"delete", "/api/v1/families/{familyId}/students/{studentId}", true},
	{"post", "/api/v1/families/{familyId}/access/revoke", true},
}

// operation decodes one documented operation; ok is false when it is missing.
func (c familyContract) operation(t *testing.T, method, path string) (contractOperation, bool) {
	t.Helper()
	node, ok := c.Paths[path][method]
	if !ok {
		return contractOperation{}, false
	}
	var op contractOperation
	if err := node.Decode(&op); err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return op, true
}

// refsParameter reports whether op lists the shared parameter by $ref.
func refsParameter(t *testing.T, op contractOperation, name string) bool {
	t.Helper()
	for _, node := range op.Parameters {
		var p struct {
			Ref string `yaml:"$ref"`
		}
		if err := node.Decode(&p); err != nil {
			t.Fatalf("decode parameter: %v", err)
		}
		if p.Ref == "#/components/parameters/"+name {
			return true
		}
	}
	return false
}

func TestFamiliesRoutesAreInTheContract(t *testing.T) {
	doc := readFamilyContract(t)
	for _, r := range familiesRoutes {
		name := strings.ToUpper(r.method) + " " + r.path
		op, ok := doc.operation(t, r.method, r.path)
		if !ok {
			t.Errorf("%s is not documented", name)
			continue
		}
		if _, cookie := firstSecurity(op)["sessionCookie"]; !cookie {
			t.Errorf("%s: security = %v, want sessionCookie", name, op.Security)
		}
		if r.write && !refsParameter(t, op, "CsrfToken") {
			t.Errorf("%s: a write must require the CSRF header", name)
		}
		for _, code := range []string{"401", "403", "404"} {
			if _, ok := op.Responses[code]; !ok {
				t.Errorf("%s: no %s response", name, code)
			}
		}
		if r.write || strings.Contains(r.path, "{") {
			if _, ok := op.Responses["400"]; !ok {
				t.Errorf("%s: no 400 response (ids are validated at the edge)", name)
			}
		}
	}
}

// The documented schemas are exactly what the handlers write: a field added
// to a response, or one the screen would wait for in vain, fails here.
func TestFamiliesSchemasMatchTheResponses(t *testing.T) {
	doc := readFamilyContract(t)
	for schema, response := range map[string]any{
		"Family":           familyResponse{},
		"FamilyDetail":     familyDetailResponse{},
		"FamilyMember":     familyMemberResponse{},
		"FamilySuggestion": familySuggestionResponse{},
		"FamilyPerson":     familyPersonResponse{},
		"FamilyContactRef": familyContactRef{},
	} {
		s, ok := doc.Components.Schemas[schema]
		if !ok {
			t.Errorf("the %s schema is not documented", schema)
			continue
		}
		if got, want := propertyNames(s.Properties), jsonFields(response); !slices.Equal(got, want) {
			t.Errorf("%s properties = %v, want %v", schema, got, want)
		}
	}
}

// The directory carries each pupil's family and filters by it.
func TestStudentListDocumentsTheFamily(t *testing.T) {
	doc := readFamilyContract(t)
	op, ok := doc.get(t, "/api/v1/students")
	if !ok {
		t.Fatal("GET /api/v1/students is not documented")
	}
	if !documentsQueryParam(t, op, "familyId") {
		t.Error("GET /api/v1/students does not document ?familyId=")
	}
	if _, ok := op.Responses["400"]; !ok {
		t.Error("GET /api/v1/students: no 400 response for a malformed familyId")
	}
	for _, schema := range []string{"StudentSummary", "StudentWithProfile"} {
		if _, ok := doc.Components.Schemas[schema].Properties["familyId"]; !ok {
			t.Errorf("%s has no familyId", schema)
		}
	}
}
