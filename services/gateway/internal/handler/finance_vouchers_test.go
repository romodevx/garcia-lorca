package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garcia-lorca/pkg/featureflag"
	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

const (
	voucherA = "b1000000-0000-0000-0000-00000000000a"
	voucherB = "b1000000-0000-0000-0000-00000000000b"
)

func vouchersOn() featureflag.Checker {
	return featureflag.NewInMemory(map[string]bool{"module_payment_vouchers": true})
}

// ADR 0040 on the counter's deposit: voucherId and declinedVoucherIds reach
// finance, and the voucher the deposit paid comes back as linkedVoucher.
func TestDepositCarriesTheVoucherChoice(t *testing.T) {
	svc := &fakeFinance{depositResult: client.DepositResult{
		Deposit:       client.Deposit{ID: "dep-1"},
		LinkedVoucher: &client.PaymentVoucher{ID: voucherA, Status: "applied", DepositFolio: "P-2026-000042"},
	}}
	h := handler.NewFinanceHandler(svc, handler.WithPaymentVoucherFlag(vouchersOn()))
	rec := httptest.NewRecorder()
	h.RegisterDeposit(rec, notesRequest(http.MethodPost, "/api/v1/finance/deposits",
		`{"studentId":"stu-1","amountCents":1000,"method":"transfer","voucherId":"`+voucherA+`"}`, "payments:write"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if svc.depositVouchers.VoucherID != voucherA || len(svc.depositVouchers.Declined) != 0 {
		t.Fatalf("vouchers = %+v", svc.depositVouchers)
	}
	var out struct {
		LinkedVoucher *struct {
			ID           string `json:"id"`
			DepositFolio string `json:"depositFolio"`
		} `json:"linkedVoucher"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.LinkedVoucher == nil ||
		out.LinkedVoucher.ID != voucherA || out.LinkedVoucher.DepositFolio != "P-2026-000042" {
		t.Fatalf("linkedVoucher = %+v (%v): %s", out.LinkedVoucher, err, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.RegisterDeposit(rec, notesRequest(http.MethodPost, "/api/v1/finance/deposits",
		`{"studentId":"stu-1","amountCents":1000,"method":"cash","declinedVoucherIds":["`+voucherB+`"]}`, "payments:write"))
	if rec.Code != http.StatusCreated || len(svc.depositVouchers.Declined) != 1 || svc.depositVouchers.Declined[0] != voucherB {
		t.Fatalf("declined: %d %+v", rec.Code, svc.depositVouchers)
	}
}

// No linked voucher, no linkedVoucher key (the field is optional).
func TestDepositWithoutVoucherOmitsLinkedVoucher(t *testing.T) {
	svc := &fakeFinance{depositResult: client.DepositResult{Deposit: client.Deposit{ID: "dep-1"}}}
	h := handler.NewFinanceHandler(svc, handler.WithPaymentVoucherFlag(vouchersOn()))
	rec := httptest.NewRecorder()
	h.RegisterDeposit(rec, notesRequest(http.MethodPost, "/api/v1/finance/deposits",
		`{"studentId":"stu-1","amountCents":1000,"method":"cash"}`, "payments:write"))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if _, has := out["linkedVoucher"]; has || rec.Code != http.StatusCreated {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

// With module_payment_vouchers off the voucher fields are invisible: sending
// them is 404 and finance is never called.
func TestDepositVoucherFieldsWithTheFlagOff(t *testing.T) {
	for name, opts := range map[string][]handler.FinanceOption{
		"flag off": {handler.WithPaymentVoucherFlag(featureflag.NewInMemory(nil))},
		"no flag":  nil,
	} {
		for _, body := range []string{
			`{"studentId":"stu-1","amountCents":1000,"method":"transfer","voucherId":"` + voucherA + `"}`,
			`{"studentId":"stu-1","amountCents":1000,"method":"cash","declinedVoucherIds":["` + voucherB + `"]}`,
		} {
			svc := &fakeFinance{}
			h := handler.NewFinanceHandler(svc, opts...)
			rec := httptest.NewRecorder()
			h.RegisterDeposit(rec, notesRequest(http.MethodPost, "/api/v1/finance/deposits", body, "payments:write"))
			if rec.Code != http.StatusNotFound || svc.depositStudent != "" {
				t.Fatalf("%s: status %d, finance called=%v; want 404 and no call", name, rec.Code, svc.depositStudent != "")
			}
		}
	}
}

func TestDepositVoucherFieldsAreValidatedAtTheEdge(t *testing.T) {
	cases := map[string]string{
		"voucherId not a uuid":       `{"studentId":"stu-1","amountCents":1000,"voucherId":"x"}`,
		"declined not uuids":         `{"studentId":"stu-1","amountCents":1000,"method":"cash","declinedVoucherIds":["x"]}`,
		"both at once":               `{"studentId":"stu-1","amountCents":1000,"voucherId":"` + voucherA + `","declinedVoucherIds":["` + voucherB + `"]}`,
		"voucher with its own key":   `{"studentId":"stu-1","amountCents":1000,"voucherId":"` + voucherA + `","idempotencyKey":"k-1"}`,
		"too many declined vouchers": `{"studentId":"stu-1","amountCents":1000,"method":"cash","declinedVoucherIds":["` + voucherA + `","` + voucherB + `","` + voucherA + `","` + voucherB + `"]}`,
	}
	for name, body := range cases {
		svc := &fakeFinance{}
		h := handler.NewFinanceHandler(svc, handler.WithPaymentVoucherFlag(vouchersOn()))
		rec := httptest.NewRecorder()
		h.RegisterDeposit(rec, notesRequest(http.MethodPost, "/api/v1/finance/deposits", body, "payments:write"))
		if rec.Code != http.StatusBadRequest || svc.depositStudent != "" {
			t.Errorf("%s: status %d, called=%v; want 400 and no call", name, rec.Code, svc.depositStudent != "")
		}
	}
}

// Review (MEDIUM): the counter is voucher-aware exactly while the module is
// on — only then may a payment by hand link a voucher or ask to choose.
func TestDepositIsVoucherAwareOnlyWithTheFlagOn(t *testing.T) {
	plain := `{"studentId":"stu-1","amountCents":1000,"method":"cash"}`
	for name, tc := range map[string]struct {
		opts []handler.FinanceOption
		want bool
	}{
		"flag on":  {[]handler.FinanceOption{handler.WithPaymentVoucherFlag(vouchersOn())}, true},
		"flag off": {[]handler.FinanceOption{handler.WithPaymentVoucherFlag(featureflag.NewInMemory(nil))}, false},
		"no flag":  {nil, false},
	} {
		svc := &fakeFinance{depositResult: client.DepositResult{Deposit: client.Deposit{ID: "dep-1"}}}
		rec := httptest.NewRecorder()
		handler.NewFinanceHandler(svc, tc.opts...).RegisterDeposit(rec, notesRequest(http.MethodPost, "/api/v1/finance/deposits", plain, "payments:write"))
		if rec.Code != http.StatusCreated || svc.depositVouchers.Aware != tc.want {
			t.Errorf("%s: status %d, aware %v, want %v", name, rec.Code, svc.depositVouchers.Aware, tc.want)
		}
	}
}
