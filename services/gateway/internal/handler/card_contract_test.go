package handler

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// jsonFields lists the JSON names of a response struct's fields.
func jsonFields(v any) []string {
	t := reflect.TypeOf(v)
	names := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func propertyNames(props map[string]yaml.Node) []string {
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Task 13: the family's card and photo routes are in the contract, behind the
// family's bearer, answering 404 for another family's child.
func TestFamilyCardRoutesAreInTheContract(t *testing.T) {
	doc := readFamilyContract(t)
	for _, path := range []string{
		"/api/v1/family/students/{studentId}/card",
		"/api/v1/family/students/{studentId}/photo",
	} {
		op, ok := doc.get(t, path)
		if !ok {
			t.Errorf("GET %s is not documented", path)
			continue
		}
		if _, bearer := firstSecurity(op)["familyBearer"]; !bearer {
			t.Errorf("GET %s: security = %v, want familyBearer", path, op.Security)
		}
		for _, code := range []string{"200", "401", "403", "404"} {
			if _, ok := op.Responses[code]; !ok {
				t.Errorf("GET %s: no %s response", path, code)
			}
		}
	}
}

// The documented card is exactly what the handler writes: a field added to
// the response, or a school detail leaked onto it, fails here.
func TestFamilyCardSchemaMatchesTheResponse(t *testing.T) {
	doc := readFamilyContract(t)
	for schema, response := range map[string]any{"FamilyCard": cardResponse{}, "FamilyCardSchool": cardSchool{}} {
		s, ok := doc.Components.Schemas[schema]
		if !ok {
			t.Errorf("the %s schema is not documented", schema)
			continue
		}
		if got, want := propertyNames(s.Properties), jsonFields(response); !slices.Equal(got, want) {
			t.Errorf("%s properties = %v, want %v", schema, got, want)
		}
		if s.AdditionalProperties == nil || *s.AdditionalProperties {
			t.Errorf("%s must declare additionalProperties: false", schema)
		}
	}
}
