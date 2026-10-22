package handler

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// ADR 0040: the contract says what the edge does. Every voucher route is
// documented with its security and refusals, the edge's bounds are the
// contract's, and the JSON the handlers write is exactly the schemas' fields —
// the front is built from the contract, so a field on one side only is a
// screen that reads blank.

type pvSchema struct {
	Required   []string             `yaml:"required"`
	Properties map[string]yaml.Node `yaml:"properties"`
}

type pvParam struct {
	Name   string `yaml:"name"`
	In     string `yaml:"in"`
	Ref    string `yaml:"$ref"`
	Schema struct {
		Maximum *int `yaml:"maximum"`
	} `yaml:"schema"`
}

type pvOperation struct {
	Security    []map[string][]string `yaml:"security"`
	Parameters  []pvParam             `yaml:"parameters"`
	Responses   map[string]yaml.Node  `yaml:"responses"`
	RequestBody struct {
		Content map[string]struct {
			Schema pvSchema `yaml:"schema"`
		} `yaml:"content"`
	} `yaml:"requestBody"`
}

func pvContract(t *testing.T) (map[string]map[string]yaml.Node, map[string]pvSchema) {
	t.Helper()
	doc := readFamilyContract(t)
	schemas := map[string]pvSchema{}
	for name, s := range doc.Components.Schemas {
		schemas[name] = pvSchema{Required: s.Required, Properties: s.Properties}
	}
	return doc.Paths, schemas
}

func pvOp(t *testing.T, paths map[string]map[string]yaml.Node, method, path string) pvOperation {
	t.Helper()
	node, ok := paths[path][method]
	if !ok {
		t.Fatalf("%s %s is not documented", strings.ToUpper(method), path)
	}
	var op pvOperation
	if err := node.Decode(&op); err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return op
}

func jsonFields(v any) []string {
	var out []string
	rt := reflect.TypeOf(v)
	for i := range rt.NumField() {
		name, _, _ := strings.Cut(rt.Field(i).Tag.Get("json"), ",")
		if name != "" && name != "-" {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

func schemaFields(s pvSchema) []string {
	out := make([]string, 0, len(s.Properties))
	for name := range s.Properties {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

func TestPaymentVoucherRoutesAreInTheContract(t *testing.T) {
	paths, _ := pvContract(t)
	const family = "/api/v1/family/students/{studentId}/payment-vouchers"
	cases := []struct {
		method, path, security string
		codes                  []string
	}{
		{"post", family, "familyBearer", []string{"201", "200", "400", "401", "403", "404", "409", "413", "429"}},
		{"get", family, "familyBearer", []string{"200", "401", "403", "404"}},
		{"get", "/api/v1/finance/payment-vouchers", "sessionCookie", []string{"200", "400", "401", "403", "404"}},
		{"get", "/api/v1/finance/payment-vouchers/{id}/file", "sessionCookie", []string{"200", "401", "403", "404"}},
		{"post", "/api/v1/finance/payment-vouchers/{id}/reject", "sessionCookie", []string{"200", "400", "401", "403", "404", "409"}},
	}
	for _, tc := range cases {
		op := pvOp(t, paths, tc.method, tc.path)
		if len(op.Security) != 1 {
			t.Errorf("%s %s: security = %v", tc.method, tc.path, op.Security)
		} else if _, ok := op.Security[0][tc.security]; !ok {
			t.Errorf("%s %s: security = %v, want %s", tc.method, tc.path, op.Security, tc.security)
		}
		for _, code := range tc.codes {
			if _, ok := op.Responses[code]; !ok {
				t.Errorf("%s %s does not document %s", tc.method, tc.path, code)
			}
		}
	}
	// A state change behind the session cookie needs the CSRF header.
	reject := pvOp(t, paths, "post", "/api/v1/finance/payment-vouchers/{id}/reject")
	if !slices.ContainsFunc(reject.Parameters, func(p pvParam) bool { return strings.HasSuffix(p.Ref, "/CsrfToken") }) {
		t.Error("the rejection does not document the CSRF header")
	}
}

func TestPaymentVoucherContractMatchesTheEdge(t *testing.T) {
	paths, schemas := pvContract(t)

	if got, want := schemaFields(schemas["FamilyPaymentVoucher"]), jsonFields(familyVoucherResponse{}); !slices.Equal(got, want) {
		t.Errorf("FamilyPaymentVoucher = %v, the family handler writes %v", got, want)
	}
	if got, want := schemaFields(schemas["PaymentVoucher"]), jsonFields(client.PaymentVoucher{}); !slices.Equal(got, want) {
		t.Errorf("PaymentVoucher = %v, the office handlers write %v", got, want)
	}
	if _, ok := schemas["DepositResult"].Properties["linkedVoucher"]; !ok {
		t.Error("DepositResult does not document linkedVoucher")
	}

	upload := pvOp(t, paths, "post", "/api/v1/family/students/{studentId}/payment-vouchers")
	form := upload.RequestBody.Content["multipart/form-data"].Schema
	if got := schemaFields(form); !slices.Equal(got, []string{"amountCents", "file", "paidOn"}) || !slices.Equal(form.Required, []string{"file"}) {
		t.Errorf("upload form = %v (required %v)", got, form.Required)
	}

	queue := pvOp(t, paths, "get", "/api/v1/finance/payment-vouchers")
	for _, p := range queue.Parameters {
		if p.Name == "limit" && (p.Schema.Maximum == nil || *p.Schema.Maximum != maxVoucherListLimit) {
			t.Errorf("limit maximum = %v, the edge allows %d", p.Schema.Maximum, maxVoucherListLimit)
		}
	}

	reject := pvOp(t, paths, "post", "/api/v1/finance/payment-vouchers/{id}/reject")
	var bounds struct {
		MinLength int `yaml:"minLength"`
		MaxLength int `yaml:"maxLength"`
	}
	reason := reject.RequestBody.Content["application/json"].Schema.Properties["reason"]
	if err := reason.Decode(&bounds); err != nil || bounds.MinLength != minVoucherReason || bounds.MaxLength != maxVoucherReason {
		t.Errorf("reason bounds = %+v (%v), the edge checks %d..%d", bounds, err, minVoucherReason, maxVoucherReason)
	}

	deposit := pvOp(t, paths, "post", "/api/v1/finance/deposits")
	props := deposit.RequestBody.Content["application/json"].Schema.Properties
	var declined struct {
		MaxItems int `yaml:"maxItems"`
	}
	if _, ok := props["voucherId"]; !ok {
		t.Error("the deposit does not document voucherId")
	}
	if node, ok := props["declinedVoucherIds"]; !ok || node.Decode(&declined) != nil || declined.MaxItems != maxDeclinedVouchers {
		t.Errorf("declinedVoucherIds maxItems = %d, the edge allows %d", declined.MaxItems, maxDeclinedVouchers)
	}
	for _, field := range []string{"voucherId", "declinedVoucherIds"} {
		if !slices.Contains(jsonFields(registerDepositRequest{}), field) {
			t.Errorf("the deposit request does not read %q", field)
		}
	}
}
