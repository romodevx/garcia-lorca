package handler

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// familyContract is the slice of docs/api/openapi.yaml these tests read.
type familyContract struct {
	Paths      map[string]map[string]yaml.Node `yaml:"paths"`
	Components struct {
		Schemas map[string]struct {
			Required             []string             `yaml:"required"`
			Properties           map[string]yaml.Node `yaml:"properties"`
			AdditionalProperties *bool                `yaml:"additionalProperties"`
		} `yaml:"schemas"`
	} `yaml:"components"`
}

func readFamilyContract(t *testing.T) familyContract {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean("../../../../docs/api/openapi.yaml"))
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	var doc familyContract
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse contract: %v", err)
	}
	return doc
}

// contractOperation is one documented operation.
type contractOperation struct {
	Security   []map[string][]string `yaml:"security"`
	Parameters []yaml.Node           `yaml:"parameters"`
	Responses  map[string]yaml.Node  `yaml:"responses"`
}

// firstSecurity is the operation's only security requirement; nil when it
// declares none or several.
func firstSecurity(op contractOperation) map[string][]string {
	if len(op.Security) != 1 {
		return nil
	}
	return op.Security[0]
}

// get decodes GET path; ok is false when it is not documented.
func (c familyContract) get(t *testing.T, path string) (contractOperation, bool) {
	t.Helper()
	node, ok := c.Paths[path]["get"]
	if !ok {
		return contractOperation{}, false
	}
	var op contractOperation
	if err := node.Decode(&op); err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return op, true
}

// ISSUE-T14-107: the family's own receipt routes are in the contract, behind
// the family's bearer, answering 404 for anything not theirs.
func TestFamilyReceiptRoutesAreInTheContract(t *testing.T) {
	doc := readFamilyContract(t)
	for _, path := range []string{
		"/api/v1/family/students/{studentId}/receipts/{depositId}",
		"/api/v1/family/students/{studentId}/receipts/{depositId}/pdf",
	} {
		op, ok := doc.get(t, path)
		if !ok {
			t.Errorf("GET %s is not documented", path)
			continue
		}
		if _, bearer := firstSecurity(op)["familyBearer"]; !bearer {
			t.Errorf("GET %s: security = %v, want familyBearer", path, op.Security)
		}
		if len(op.Parameters) != 2 {
			t.Errorf("GET %s: %d parameters, want studentId and depositId", path, len(op.Parameters))
		}
		for _, code := range []string{"200", "401", "403", "404"} {
			if _, ok := op.Responses[code]; !ok {
				t.Errorf("GET %s: no %s response", path, code)
			}
		}
	}
	if _, ok := doc.Components.Schemas["PaymentReceipt"]; !ok {
		t.Error("the PaymentReceipt schema is not documented")
	}
}

// ISSUE-T14-108: the child's record carries the group as every screen names
// it, next to the bare letter it always carried.
func TestFamilyStudentDocumentsTheGroupLabel(t *testing.T) {
	doc := readFamilyContract(t)
	for _, path := range []string{"/api/v1/family/students", "/api/v1/family/students/{studentId}/profile"} {
		if _, ok := doc.get(t, path); !ok {
			t.Errorf("GET %s is not documented", path)
		}
	}
	student, ok := doc.Components.Schemas["FamilyStudent"]
	if !ok {
		t.Fatal("the FamilyStudent schema is not documented")
	}
	for _, field := range []string{"gradeGroup", "groupLabel"} {
		if _, ok := student.Properties[field]; !ok {
			t.Errorf("FamilyStudent has no %s", field)
		}
	}
}
