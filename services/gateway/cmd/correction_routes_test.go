package main

import (
	"context"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

func (stubFinance) CorrectDeposit(_ context.Context, req *connect.Request[financev1.CorrectDepositRequest]) (*connect.Response[financev1.CorrectDepositResponse], error) {
	return connect.NewResponse(&financev1.CorrectDepositResponse{
		Original:   &financev1.Deposit{Id: req.Msg.GetDepositId(), SupersededBy: "62000000-0000-0000-0000-000000000002"},
		Deposit:    &financev1.Deposit{Id: "62000000-0000-0000-0000-000000000002", CorrectsDepositId: req.Msg.GetDepositId()},
		Correction: &financev1.DepositCorrection{Reason: req.Msg.GetReason(), CorrectedBy: req.Msg.GetCorrectedBy()},
	}), nil
}

func (stubFinance) ListDepositCorrections(context.Context, *connect.Request[financev1.ListDepositCorrectionsRequest]) (*connect.Response[financev1.ListDepositCorrectionsResponse], error) {
	return connect.NewResponse(&financev1.ListDepositCorrectionsResponse{}), nil
}

// T9 through the real mux: correcting a payment answers to payments:edit alone
// (the owner's by default, delegable) — not to taking money (payments:write) nor
// to finance:write; the history to payments:edit or finance:read; module_finance
// off hides both.
func TestCorrectionRoutes(t *testing.T) {
	const deposit = "/api/v1/finance/deposits/62000000-0000-0000-0000-000000000001"
	const body = `{"amountCents":100000,"reason":"monto","tenders":[{"method":"cash","amountCents":100000}]}`
	on := map[string]bool{"module_finance": true}
	cases := []struct {
		name, method, path, token string
		flags                     map[string]bool
		want                      int
	}{
		{"an editor corrects", http.MethodPost, deposit + "/correct", "payments-editor", on, http.StatusOK},
		{"the counter cannot", http.MethodPost, deposit + "/correct", "cobranza", on, http.StatusForbidden},
		{"finance:write is not enough", http.MethodPost, deposit + "/correct", "finance-writer", on, http.StatusForbidden},
		{"no token", http.MethodPost, deposit + "/correct", "", on, http.StatusUnauthorized},
		{"finance off hides it", http.MethodPost, deposit + "/correct", "payments-editor", map[string]bool{}, http.StatusNotFound},
		{"an editor reads the history", http.MethodGet, deposit + "/corrections", "payments-editor", on, http.StatusOK},
		{"finance:read reads the history", http.MethodGet, deposit + "/corrections", "finance-reader", on, http.StatusOK},
		{"the counter cannot read it", http.MethodGet, deposit + "/corrections", "cobranza", on, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gw := newTestGateway(t, tc.flags)
			payload := ""
			if tc.method == http.MethodPost {
				payload = body
			}
			if res := doGateway(t, gw, tc.method, tc.path, tc.token, payload); res.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", res.StatusCode, tc.want)
			}
		})
	}
}
