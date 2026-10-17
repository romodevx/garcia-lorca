package main

import (
	"context"
	"errors"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

// GetDepositReceipt on the stub: dep-own is the stub guardian's child's
// (stu-1), dep-other another pupil's, dep-1 the office tests' (stu-1);
// anything else does not exist.
func (stubFinance) GetDepositReceipt(_ context.Context, req *connect.Request[financev1.GetDepositReceiptRequest]) (*connect.Response[financev1.GetDepositReceiptResponse], error) {
	owners := map[string]string{"dep-own": "stu-1", "dep-other": "stu-2", "dep-1": "stu-1"}
	owner, ok := owners[req.Msg.GetDepositId()]
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("no such deposit"))
	}
	return connect.NewResponse(&financev1.GetDepositReceiptResponse{Deposit: &financev1.Deposit{
		Id: req.Msg.GetDepositId(), StudentId: owner, AmountCents: 300000, Method: "cash",
		PaidOn: "2026-10-17", Folio: "P-2026-000002",
	}}), nil
}

// ISSUE-T14-107 through the real mux: the family's receipt answers to
// family:read and ownership of BOTH the child and the deposit; staff use their
// own door; module_finance or module_students off hides it.
func TestFamilyReceiptRoutes(t *testing.T) {
	on := map[string]bool{"module_finance": true, "module_students": true, "module_iam": true}
	const own = "/api/v1/family/students/stu-1/receipts/dep-own"
	cases := []struct {
		name, path, token string
		flags             map[string]bool
		want              int
	}{
		{"family reads its child's receipt", own, "family-reader", on, http.StatusOK},
		{"family pdf", own + "/pdf", "family-reader", on, http.StatusOK},
		{"another family's child", "/api/v1/family/students/stu-999/receipts/dep-own", "family-reader", on, http.StatusNotFound},
		{"another pupil's deposit under the own child", "/api/v1/family/students/stu-1/receipts/dep-other", "family-reader", on, http.StatusNotFound},
		{"a deposit that does not exist", "/api/v1/family/students/stu-1/receipts/dep-missing/pdf", "family-reader", on, http.StatusNotFound},
		{"staff cannot use the family's door", own, "finance-reader", on, http.StatusForbidden},
		{"no token", own, "", on, http.StatusUnauthorized},
		{"finance off hides it", own, "family-reader", map[string]bool{"module_students": true, "module_iam": true}, http.StatusNotFound},
		{"students off hides it", own, "family-reader", map[string]bool{"module_finance": true, "module_iam": true}, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gw := newTestGateway(t, tc.flags)
			res := doGateway(t, gw, http.MethodGet, tc.path, tc.token, "")
			if res.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", res.StatusCode, tc.want)
			}
		})
	}
}
