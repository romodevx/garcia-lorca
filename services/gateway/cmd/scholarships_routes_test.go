package main

import (
	"context"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

func (stubFinance) ListScholarships(context.Context, *connect.Request[financev1.ListScholarshipsRequest]) (*connect.Response[financev1.ListScholarshipsResponse], error) {
	return connect.NewResponse(&financev1.ListScholarshipsResponse{}), nil
}

func (stubFinance) CreateScholarship(_ context.Context, req *connect.Request[financev1.CreateScholarshipRequest]) (*connect.Response[financev1.CreateScholarshipResponse], error) {
	return connect.NewResponse(&financev1.CreateScholarshipResponse{Scholarship: &financev1.Scholarship{Id: "5c200000-0000-0000-0000-000000000001", Name: req.Msg.GetScholarship().GetName()}}), nil
}

func (stubFinance) AssignScholarship(context.Context, *connect.Request[financev1.AssignScholarshipRequest]) (*connect.Response[financev1.AssignScholarshipResponse], error) {
	return connect.NewResponse(&financev1.AssignScholarshipResponse{}), nil
}

// T18 through the real mux: Becas needs module_scholarships (and
// module_finance, where its data lives); reads answer to scholarships:read,
// writes to scholarships:write. finance:write alone does not open the module.
func TestScholarshipsRoutes(t *testing.T) {
	both := map[string]bool{"module_finance": true, "module_scholarships": true}
	financeOnly := map[string]bool{"module_finance": true}
	const beca = `{"name":"Hermanos","kind":"percent","value":5000,"targets":["tuition"]}`
	const assign = `{"studentIds":["5c200000-0000-0000-0000-00000000005a"]}`
	const assignPath = "/api/v1/scholarships/5c200000-0000-0000-0000-000000000001/students"
	cases := []struct {
		name, method, path, token, body string
		flags                           map[string]bool
		want                            int
	}{
		{"a reader lists", http.MethodGet, "/api/v1/scholarships", "scholarships-reader", "", both, http.StatusOK},
		{"finance:write is not enough", http.MethodGet, "/api/v1/scholarships", "finance-writer", "", both, http.StatusForbidden},
		{"no token", http.MethodGet, "/api/v1/scholarships", "", "", both, http.StatusUnauthorized},
		{"module off hides it", http.MethodGet, "/api/v1/scholarships", "scholarships-reader", "", financeOnly, http.StatusNotFound},
		{"finance off hides it", http.MethodGet, "/api/v1/scholarships", "scholarships-reader", "", map[string]bool{"module_scholarships": true}, http.StatusNotFound},
		{"a writer creates", http.MethodPost, "/api/v1/scholarships", "scholarships-writer", beca, both, http.StatusCreated},
		{"a reader cannot create", http.MethodPost, "/api/v1/scholarships", "scholarships-reader", beca, both, http.StatusForbidden},
		{"a writer assigns", http.MethodPost, assignPath, "scholarships-writer", assign, both, http.StatusOK},
		{"a reader cannot assign", http.MethodPost, assignPath, "scholarships-reader", assign, both, http.StatusForbidden},
		{"a reader cannot remove", http.MethodPost, assignPath + "/remove", "scholarships-reader", assign, both, http.StatusForbidden},
		{"assigning needs the module", http.MethodPost, assignPath, "scholarships-writer", assign, financeOnly, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gw := newTestGateway(t, tc.flags)
			if res := doGateway(t, gw, tc.method, tc.path, tc.token, tc.body); res.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", res.StatusCode, tc.want)
			}
		})
	}
}
