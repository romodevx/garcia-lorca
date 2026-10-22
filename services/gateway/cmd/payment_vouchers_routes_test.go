package main

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	connect "connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

const stubVoucherID = "b1000000-0000-0000-0000-000000000001"

func (stubFinance) CreatePaymentVoucher(_ context.Context, req *connect.Request[financev1.CreatePaymentVoucherRequest]) (*connect.Response[financev1.CreatePaymentVoucherResponse], error) {
	return connect.NewResponse(&financev1.CreatePaymentVoucherResponse{Voucher: &financev1.PaymentVoucher{
		Id: stubVoucherID, StudentId: req.Msg.GetStudentId(), Status: "pending",
	}}), nil
}

func (stubFinance) ListPaymentVouchers(_ context.Context, req *connect.Request[financev1.ListPaymentVouchersRequest]) (*connect.Response[financev1.ListPaymentVouchersResponse], error) {
	return connect.NewResponse(&financev1.ListPaymentVouchersResponse{Vouchers: []*financev1.PaymentVoucher{
		{Id: stubVoucherID, StudentId: "stu-1", Status: "pending"},
	}}), nil
}

func (stubFinance) GetPaymentVoucherFile(context.Context, *connect.Request[financev1.GetPaymentVoucherFileRequest]) (*connect.Response[financev1.GetPaymentVoucherFileResponse], error) {
	return connect.NewResponse(&financev1.GetPaymentVoucherFileResponse{Data: []byte("%PDF-1.7"), ContentType: "application/pdf"}), nil
}

func (stubFinance) RejectPaymentVoucher(_ context.Context, req *connect.Request[financev1.RejectPaymentVoucherRequest]) (*connect.Response[financev1.RejectPaymentVoucherResponse], error) {
	return connect.NewResponse(&financev1.RejectPaymentVoucherResponse{Voucher: &financev1.PaymentVoucher{
		Id: req.Msg.GetId(), Status: "rejected", RejectionReason: req.Msg.GetReason(),
	}}), nil
}

// RegisterDeposit on the stub: a deposit, and the voucher back as linked when
// the counter named one.
func (stubFinance) RegisterDeposit(_ context.Context, req *connect.Request[financev1.RegisterDepositRequest]) (*connect.Response[financev1.RegisterDepositResponse], error) {
	res := &financev1.RegisterDepositResponse{Deposit: &financev1.Deposit{Id: "dep-1", StudentId: req.Msg.GetStudentId(), AmountCents: req.Msg.GetAmountCents()}}
	if id := req.Msg.GetVoucherId(); id != "" {
		res.LinkedVoucher = &financev1.PaymentVoucher{Id: id, Status: "applied", DepositFolio: "P-2026-000042"}
	}
	return connect.NewResponse(res), nil
}

func voucherForm(t *testing.T) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "c.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("%PDF-1.7\nx")); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

func postVoucher(t *testing.T, gwURL string, client *http.Client, path, token string) *http.Response {
	t.Helper()
	body, ct := voucherForm(t)
	r, err := http.NewRequest(http.MethodPost, gwURL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", ct)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

var (
	vouchersAll       = map[string]bool{"module_payment_vouchers": true, "module_finance": true, "module_students": true, "module_iam": true}
	vouchersOff       = map[string]bool{"module_finance": true, "module_students": true, "module_iam": true}
	vouchersNoFinance = map[string]bool{"module_payment_vouchers": true, "module_students": true, "module_iam": true}
	vouchersNoPupils  = map[string]bool{"module_payment_vouchers": true, "module_finance": true, "module_iam": true}
)

// ADR 0040 through the real mux, family side: the guardian sends and reads
// its OWN child's vouchers; any of the three modules off hides the route
// before auth; another family's child is 404.
func TestFamilyPaymentVoucherRoutes(t *testing.T) {
	const own = "/api/v1/family/students/stu-1/payment-vouchers"
	upload := []struct {
		name, path, token string
		flags             map[string]bool
		want              int
	}{
		{"a guardian sends one", own, "family-voucher", vouchersAll, http.StatusCreated},
		{"family:read alone cannot send", own, "family-reader", vouchersAll, http.StatusForbidden},
		{"staff cannot use the family's door", own, "cobranza", vouchersAll, http.StatusForbidden},
		{"another family's child", "/api/v1/family/students/stu-999/payment-vouchers", "family-voucher", vouchersAll, http.StatusNotFound},
		{"the owner on a child it is not linked to", "/api/v1/family/students/stu-999/payment-vouchers", "owner-vouchers", vouchersAll, http.StatusNotFound},
		{"no token", own, "", vouchersAll, http.StatusUnauthorized},
		{"module off hides it", own, "family-voucher", vouchersOff, http.StatusNotFound},
		{"module off hides it before auth", own, "", vouchersOff, http.StatusNotFound},
		{"finance off hides it", own, "family-voucher", vouchersNoFinance, http.StatusNotFound},
		{"students off hides it", own, "family-voucher", vouchersNoPupils, http.StatusNotFound},
	}
	for _, tc := range upload {
		t.Run("POST "+tc.name, func(t *testing.T) {
			gw := newTestGateway(t, tc.flags)
			if res := postVoucher(t, gw.URL, gw.Client(), tc.path, tc.token); res.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", res.StatusCode, tc.want)
			}
		})
	}
	list := []struct {
		name, path, token string
		flags             map[string]bool
		want              int
	}{
		{"a guardian reads them", own, "family-reader", vouchersAll, http.StatusOK},
		{"another family's child", "/api/v1/family/students/stu-999/payment-vouchers", "family-reader", vouchersAll, http.StatusNotFound},
		{"module off hides it before auth", own, "", vouchersOff, http.StatusNotFound},
		{"staff cannot use the family's door", own, "finance-reader", vouchersAll, http.StatusForbidden},
	}
	for _, tc := range list {
		t.Run("GET "+tc.name, func(t *testing.T) {
			gw := newTestGateway(t, tc.flags)
			if res := doGateway(t, gw, http.MethodGet, tc.path, tc.token, ""); res.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", res.StatusCode, tc.want)
			}
		})
	}
}

// The upload budget is per guardian, not per IP (behind Cloud Run every
// request comes from the same front end), and it is its own budget.
func TestFamilyPaymentVoucherUploadIsRateLimitedPerGuardian(t *testing.T) {
	gw := newTestGateway(t, vouchersAll)
	const own = "/api/v1/family/students/stu-1/payment-vouchers"
	for i := range voucherUploadBurst {
		if res := postVoucher(t, gw.URL, gw.Client(), own, "family-voucher"); res.StatusCode != http.StatusCreated {
			t.Fatalf("upload %d: status %d", i+1, res.StatusCode)
		}
	}
	res := postVoucher(t, gw.URL, gw.Client(), own, "family-voucher")
	if res.StatusCode != http.StatusTooManyRequests || res.Header.Get("Retry-After") == "" {
		t.Fatalf("past the burst: status %d, Retry-After %q", res.StatusCode, res.Header.Get("Retry-After"))
	}
	// The list is not throttled by it.
	if res := doGateway(t, gw, http.MethodGet, own, "family-reader", ""); res.StatusCode != http.StatusOK {
		t.Fatalf("list after the limit: %d", res.StatusCode)
	}
}

// The office's side: the queue and the file answer to collections:read, the
// refusal to payments:write; module_payment_vouchers or module_finance off
// hides all three before auth.
func TestStaffPaymentVoucherRoutes(t *testing.T) {
	const queue = "/api/v1/finance/payment-vouchers?status=pending"
	const file = "/api/v1/finance/payment-vouchers/" + stubVoucherID + "/file"
	const reject = "/api/v1/finance/payment-vouchers/" + stubVoucherID + "/reject"
	const reason = `{"reason":"no se lee"}`
	cases := []struct {
		name, method, path, token, body string
		flags                           map[string]bool
		want                            int
	}{
		{"the cashier reads the queue", http.MethodGet, queue, "cobranza", "", vouchersAll, http.StatusOK},
		{"the owner reads the queue", http.MethodGet, queue, "finance-reader", "", vouchersAll, http.StatusOK},
		{"a family cannot read the queue", http.MethodGet, queue, "family-voucher", "", vouchersAll, http.StatusForbidden},
		{"queue: module off before auth", http.MethodGet, queue, "", "", vouchersOff, http.StatusNotFound},
		{"queue: finance off", http.MethodGet, queue, "cobranza", "", map[string]bool{"module_payment_vouchers": true}, http.StatusNotFound},
		{"the cashier opens the file", http.MethodGet, file, "cobranza", "", vouchersAll, http.StatusOK},
		{"a family cannot open the file", http.MethodGet, file, "family-voucher", "", vouchersAll, http.StatusForbidden},
		{"file: module off before auth", http.MethodGet, file, "", "", vouchersOff, http.StatusNotFound},
		{"the cashier rejects", http.MethodPost, reject, "cobranza", reason, vouchersAll, http.StatusOK},
		{"finance:read cannot reject", http.MethodPost, reject, "finance-reader", reason, vouchersAll, http.StatusForbidden},
		{"reject: module off before auth", http.MethodPost, reject, "", reason, vouchersOff, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gw := newTestGateway(t, tc.flags)
			res := doGateway(t, gw, tc.method, tc.path, tc.token, tc.body)
			if res.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", res.StatusCode, tc.want)
			}
			if tc.path == file && res.StatusCode == http.StatusOK &&
				(res.Header.Get("Cache-Control") != "private, no-store" || res.Header.Get("X-Content-Type-Options") != "nosniff" ||
					!strings.HasPrefix(res.Header.Get("Content-Security-Policy"), "sandbox") ||
					!strings.HasPrefix(res.Header.Get("Content-Disposition"), "attachment")) {
				t.Fatalf("file headers through the mux = %v", res.Header)
			}
		})
	}
}

// The counter's deposit with a voucher: through with the module on (and the
// linked voucher back), invisible (404) with it off.
func TestDepositWithVoucherFollowsTheFlag(t *testing.T) {
	body := `{"studentId":"a1000000-0000-0000-0000-000000000001","amountCents":1000,"method":"transfer","voucherId":"` + stubVoucherID + `"}`
	gw := newTestGateway(t, vouchersAll)
	res := doGateway(t, gw, http.MethodPost, "/api/v1/finance/deposits", "cobranza", body)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("on: status %d", res.StatusCode)
	}
	var out struct {
		LinkedVoucher struct {
			ID string `json:"id"`
		} `json:"linkedVoucher"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil || out.LinkedVoucher.ID != stubVoucherID {
		t.Fatalf("linkedVoucher = %+v (%v)", out.LinkedVoucher, err)
	}

	gw = newTestGateway(t, vouchersOff)
	if res := doGateway(t, gw, http.MethodPost, "/api/v1/finance/deposits", "cobranza", body); res.StatusCode != http.StatusNotFound {
		t.Fatalf("off: status %d, want 404", res.StatusCode)
	}
	plain := `{"studentId":"a1000000-0000-0000-0000-000000000001","amountCents":1000,"method":"cash"}`
	if res := doGateway(t, gw, http.MethodPost, "/api/v1/finance/deposits", "cobranza", plain); res.StatusCode != http.StatusCreated {
		t.Fatalf("off, no voucher: status %d, want 201", res.StatusCode)
	}
}
