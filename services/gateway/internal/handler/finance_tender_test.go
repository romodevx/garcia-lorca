package handler_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// T8: the counter's tenders reach finance on a deposit and on a payment, and a
// combined payment answers every row it wrote.
func TestFinanceForwardsTenders(t *testing.T) {
	svc := &fakeFinance{
		depositResult: client.DepositResult{Deposit: client.Deposit{ID: "dep-1"}},
		payments:      []client.Payment{{ID: "p1", Method: "credit_note"}, {ID: "p2", Method: "cash"}},
	}
	h := handler.NewFinanceHandler(svc)

	doReq(t, h.RegisterDeposit, http.MethodPost, "/api/v1/finance/deposits",
		`{"studentId":"stu-1","amountCents":350000,"tenders":[{"method":"credit_note","amountCents":200000,"reference":"NC-7"},{"method":"card","amountCents":150000,"cardType":"debit"}]}`)
	want := []client.Tender{
		{Method: "credit_note", AmountCents: 200000, Reference: "NC-7"},
		{Method: "card", AmountCents: 150000, CardType: "debit"},
	}
	if len(svc.depositTenders) != 2 || svc.depositTenders[0] != want[0] || svc.depositTenders[1] != want[1] {
		t.Errorf("deposit tenders = %+v, want %+v", svc.depositTenders, want)
	}

	rec := doReq(t, h.RegisterPayment, http.MethodPost, "/api/v1/finance/payments",
		`{"chargeId":"ch-1","amountCents":100000,"tenders":[{"method":"credit_note","amountCents":40000},{"method":"cash","amountCents":60000}]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("payment: status %d: %s", rec.Code, rec.Body.String())
	}
	if len(svc.payTenders) != 2 || svc.payTenders[0].Method != "credit_note" {
		t.Errorf("payment tenders = %+v", svc.payTenders)
	}
	body := rec.Body.String()
	for _, w := range []string{`"payments":[`, `"id":"p2"`, `"payment":{`} {
		if !strings.Contains(body, w) {
			t.Errorf("payment body lacks %s: %s", w, body)
		}
	}

	// A legacy single method still arrives as one tender.
	doReq(t, h.RegisterPayment, http.MethodPost, "/api/v1/finance/payments",
		`{"chargeId":"ch-1","amountCents":1000,"method":"transfer","reference":"SPEI"}`)
	if len(svc.payTenders) != 1 || svc.payTenders[0] != (client.Tender{Method: "transfer", AmountCents: 1000, Reference: "SPEI"}) {
		t.Errorf("legacy payment tenders = %+v", svc.payTenders)
	}
}

// Tenders are validated at the edge against the contract: at most four, a
// known method, a positive amount — never forwarded as whatever a client sends.
func TestFinanceRefusesTendersOutsideTheContract(t *testing.T) {
	five := strings.Repeat(`{"method":"cash","amountCents":100},`, 5)
	cases := map[string]string{
		"unknown method": `[{"method":"vale","amountCents":1000}]`,
		"zero amount":    `[{"method":"cash","amountCents":0}]`,
		"too many":       "[" + strings.TrimSuffix(five, ",") + "]",
		"long reference": `[{"method":"credit_note","amountCents":1000,"reference":"` + strings.Repeat("x", 121) + `"}]`,
	}
	for name, tenders := range cases {
		t.Run(name, func(t *testing.T) {
			svc := &fakeFinance{}
			h := handler.NewFinanceHandler(svc)
			if rec := doReq(t, h.RegisterDeposit, http.MethodPost, "/api/v1/finance/deposits",
				`{"studentId":"stu-1","amountCents":1000,"tenders":`+tenders+`}`); rec.Code != http.StatusBadRequest {
				t.Errorf("deposit: status %d, want 400", rec.Code)
			}
			if rec := doReq(t, h.RegisterPayment, http.MethodPost, "/api/v1/finance/payments",
				`{"chargeId":"ch-1","amountCents":1000,"tenders":`+tenders+`}`); rec.Code != http.StatusBadRequest {
				t.Errorf("payment: status %d, want 400", rec.Code)
			}
			if svc.depositStudent != "" || svc.payChargeID != "" {
				t.Error("invalid tenders reached finance")
			}
		})
	}
}
