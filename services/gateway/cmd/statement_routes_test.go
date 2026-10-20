package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

// A school that has not filled its details: the PDF still prints.
func (stubConfig) GetSchoolProfile(context.Context, *connect.Request[configv1.GetSchoolProfileRequest]) (*connect.Response[configv1.GetSchoolProfileResponse], error) {
	return connect.NewResponse(&configv1.GetSchoolProfileResponse{}), nil
}

// GetStatement on the stub says which view it was asked for in the line's
// description, so a route that reads the wrong view fails visibly.
func (stubFinance) GetStatement(_ context.Context, req *connect.Request[financev1.GetStatementRequest]) (*connect.Response[financev1.GetStatementResponse], error) {
	view := "office"
	if req.Msg.GetFamilyView() {
		view = "family"
	}
	return connect.NewResponse(&financev1.GetStatementResponse{
		Movements: []*financev1.StatementMovement{{Kind: "charge", Date: "2026-09-10", Description: view, AmountCents: 150000, BalanceCents: 150000}},
		Totals:    &financev1.StatementTotals{ChargedCents: 150000, BalanceCents: 150000},
		AsOf:      "2026-10-01",
	}), nil
}

// T7 through the real mux: the office's statement answers to the account's
// permissions (finance:read or collections:read); the family's to family:read
// and ownership; module_finance off hides both.
func TestStatementRoutes(t *testing.T) {
	on := map[string]bool{"module_finance": true, "module_students": true, "module_iam": true}
	cases := []struct {
		name, path, token string
		flags             map[string]bool
		want              int
		view              string
	}{
		{"office reads", "/api/v1/finance/accounts/stu-1/statement", "finance-reader", on, http.StatusOK, "office"},
		{"the counter reads", "/api/v1/finance/accounts/stu-1/statement", "cobranza", on, http.StatusOK, "office"},
		{"office pdf", "/api/v1/finance/accounts/stu-1/statement/pdf", "finance-reader", on, http.StatusOK, ""},
		{"a teacher is refused", "/api/v1/finance/accounts/stu-1/statement", "teaching-writer", on, http.StatusForbidden, ""},
		{"a family cannot use the office's door", "/api/v1/finance/accounts/stu-1/statement", "family-reader", on, http.StatusForbidden, ""},
		{"no token", "/api/v1/finance/accounts/stu-1/statement", "", on, http.StatusUnauthorized, ""},
		{"finance off hides the office's", "/api/v1/finance/accounts/stu-1/statement", "finance-reader", map[string]bool{"module_students": true}, http.StatusNotFound, ""},
		{"family reads its child", "/api/v1/family/students/stu-1/statement", "family-reader", on, http.StatusOK, "family"},
		{"family pdf", "/api/v1/family/students/stu-1/statement/pdf", "family-reader", on, http.StatusOK, ""},
		{"another family's child", "/api/v1/family/students/stu-999/statement", "family-reader", on, http.StatusNotFound, ""},
		{"staff cannot use the family's door", "/api/v1/family/students/stu-1/statement", "finance-reader", on, http.StatusForbidden, ""},
		{"finance off hides the family's", "/api/v1/family/students/stu-1/statement", "family-reader", map[string]bool{"module_students": true}, http.StatusNotFound, ""},
		{"family reads how to pay", "/api/v1/family/students/stu-1/payment-info", "family-reader", on, http.StatusOK, ""},
		{"how to pay for another family's child", "/api/v1/family/students/stu-999/payment-info", "family-reader", on, http.StatusNotFound, ""},
		{"staff cannot read the family's payment info", "/api/v1/family/students/stu-1/payment-info", "finance-reader", on, http.StatusForbidden, ""},
		{"finance off hides how to pay", "/api/v1/family/students/stu-1/payment-info", "family-reader", map[string]bool{"module_students": true}, http.StatusNotFound, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gw := newTestGateway(t, tc.flags)
			res := doGateway(t, gw, http.MethodGet, tc.path, tc.token, "")
			if res.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", res.StatusCode, tc.want)
			}
			if tc.view == "" {
				return
			}
			var body struct {
				Movements []struct {
					Description string `json:"description"`
				} `json:"movements"`
			}
			if err := json.NewDecoder(res.Body).Decode(&body); err != nil || len(body.Movements) != 1 || body.Movements[0].Description != tc.view {
				t.Errorf("read the %+v view (%v), want %s", body, err, tc.view)
			}
		})
	}
}
