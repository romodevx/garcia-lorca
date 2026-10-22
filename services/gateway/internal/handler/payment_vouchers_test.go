package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

const (
	pvChild = "a1000000-0000-0000-0000-000000000001"
	pvOther = "a1000000-0000-0000-0000-000000000002"
	pvID    = "b1000000-0000-0000-0000-000000000001"
)

// fakeVouchers records what the handler asked finance and answers canned values.
type fakeVouchers struct {
	upload    client.VoucherUpload
	uploads   int
	replayed  bool
	scope     client.VoucherScope
	status    string
	limit     int
	list      []client.PaymentVoucher
	fileID    string
	file      []byte
	fileType  string
	rejectID  string
	reason    string
	rejectBy  client.Receiver
	err       error
	voucher   client.PaymentVoucher
	listCalls int
}

func (f *fakeVouchers) CreatePaymentVoucher(_ context.Context, in client.VoucherUpload) (client.PaymentVoucher, bool, error) {
	f.upload, f.uploads = in, f.uploads+1
	return f.voucher, f.replayed, f.err
}

func (f *fakeVouchers) ListPaymentVouchers(_ context.Context, scope client.VoucherScope, status string, limit int) ([]client.PaymentVoucher, error) {
	f.scope, f.status, f.limit, f.listCalls = scope, status, limit, f.listCalls+1
	return f.list, f.err
}

func (f *fakeVouchers) GetPaymentVoucherFile(_ context.Context, id string, scope client.VoucherScope) ([]byte, string, error) {
	f.fileID, f.scope = id, scope
	return f.file, f.fileType, f.err
}

func (f *fakeVouchers) RejectPaymentVoucher(_ context.Context, id, reason string, by client.Receiver) (client.PaymentVoucher, bool, error) {
	f.rejectID, f.reason, f.rejectBy = id, reason, by
	return f.voucher, f.replayed, f.err
}

func pvRequest(method, path string, body *bytes.Buffer, contentType string, values map[string]string, perms ...string) *http.Request {
	var r *http.Request
	if body == nil {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, body)
	}
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	for k, v := range values {
		r.SetPathValue(k, v)
	}
	return r.WithContext(middleware.WithPrincipal(r.Context(),
		middleware.Principal{UserID: "g-1", Email: "rosa@example.com", Permissions: perms}))
}

// multipartBody builds a form with a file part (when data is not nil) and the
// given fields.
func multipartBody(t *testing.T, data []byte, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if data != nil {
		part, err := mw.CreateFormFile("file", "comprobante.pdf")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

func newPV(svc *fakeVouchers) *handler.PaymentVouchersHandler {
	return handler.NewPaymentVouchersHandler(svc, fakeLinks{children: []string{pvChild}}, nil)
}

func upload(t *testing.T, h *handler.PaymentVouchersHandler, child string, data []byte, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	body, ct := multipartBody(t, data, fields)
	rec := httptest.NewRecorder()
	h.FamilyUpload(rec, pvRequest(http.MethodPost, "/api/v1/family/students/"+child+"/payment-vouchers", body, ct,
		map[string]string{"studentId": child}, "vouchers_own:write"))
	return rec
}

var pvPDF = []byte("%PDF-1.7\ntransferencia")

func TestFamilyUploadsAVoucher(t *testing.T) {
	svc := &fakeVouchers{voucher: client.PaymentVoucher{
		ID: pvID, StudentID: pvChild, Status: "pending", ClaimedAmountCents: 320000, ClaimedPaidOn: "2026-10-20",
		FileContentType: "application/pdf", UploadedByName: "Rosa", CreatedAt: "2026-10-21T15:00:00Z",
	}}
	rec := upload(t, newPV(svc), pvChild, pvPDF, map[string]string{"amountCents": "320000", "paidOn": "2026-10-20"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if svc.upload.StudentID != pvChild || !bytes.Equal(svc.upload.Data, pvPDF) || svc.upload.ClaimedAmountCents != 320000 ||
		svc.upload.ClaimedPaidOn != "2026-10-20" || svc.upload.By.UserID != "g-1" {
		t.Fatalf("upload = %+v", svc.upload)
	}
	var out struct {
		Voucher  map[string]any `json:"voucher"`
		Replayed bool           `json:"replayed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	v := out.Voucher
	if v["id"] != pvID || v["status"] != "pending" || v["claimedAmountCents"] != float64(320000) {
		t.Fatalf("voucher = %v", v)
	}
	for _, staffOnly := range []string{"uploadedByName", "decidedByName", "studentId", "depositId"} {
		if _, has := v[staffOnly]; has {
			t.Errorf("the family's view carries %q", staffOnly)
		}
	}

	svc.replayed = true
	if rec := upload(t, newPV(svc), pvChild, pvPDF, nil); rec.Code != http.StatusOK {
		t.Fatalf("a replay is 200, got %d", rec.Code)
	}
}

func TestFamilyUploadRefusals(t *testing.T) {
	big := append(append([]byte{}, pvPDF...), make([]byte, 5<<20)...)
	cases := []struct {
		name   string
		child  string
		data   []byte
		fields map[string]string
		want   int
	}{
		{"another family's child", pvOther, pvPDF, nil, http.StatusNotFound},
		{"a child id that is not an id", "x", pvPDF, nil, http.StatusNotFound},
		{"no file", pvChild, nil, map[string]string{"amountCents": "100"}, http.StatusBadRequest},
		{"over 5 MB", pvChild, big, nil, http.StatusRequestEntityTooLarge},
		{"amount not a number", pvChild, pvPDF, map[string]string{"amountCents": "mil"}, http.StatusBadRequest},
		{"amount zero", pvChild, pvPDF, map[string]string{"amountCents": "0"}, http.StatusBadRequest},
		{"amount negative", pvChild, pvPDF, map[string]string{"amountCents": "-5"}, http.StatusBadRequest},
		{"day not a date", pvChild, pvPDF, map[string]string{"paidOn": "20/10/2026"}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeVouchers{}
			rec := upload(t, newPV(svc), tc.child, tc.data, tc.fields)
			if rec.Code != tc.want || svc.uploads != 0 {
				t.Fatalf("status = %d (finance called %d), want %d: %s", rec.Code, svc.uploads, tc.want, rec.Body.String())
			}
		})
	}
	t.Run("not multipart", func(t *testing.T) {
		svc := &fakeVouchers{}
		rec := httptest.NewRecorder()
		newPV(svc).FamilyUpload(rec, pvRequest(http.MethodPost, "/x", bytes.NewBuffer(pvPDF), "application/pdf",
			map[string]string{"studentId": pvChild}, "vouchers_own:write"))
		if rec.Code != http.StatusBadRequest || svc.uploads != 0 {
			t.Fatalf("status = %d", rec.Code)
		}
	})
}

// finance's refusal codes reach the family's screen.
func TestFamilyUploadForwardsTheLimit(t *testing.T) {
	err := connect.NewError(connect.CodeAlreadyExists, errors.New("ya hay 3 comprobantes por revisar"))
	err.Meta().Set("X-Error-Code", "VOUCHER_LIMIT")
	rec := upload(t, newPV(&fakeVouchers{err: err}), pvChild, pvPDF, nil)
	if rec.Code != http.StatusConflict || rec.Header().Get("X-Error-Code") != "VOUCHER_LIMIT" {
		t.Fatalf("status = %d, code %q", rec.Code, rec.Header().Get("X-Error-Code"))
	}
}

func TestFamilyListsItsChildsVouchers(t *testing.T) {
	svc := &fakeVouchers{list: []client.PaymentVoucher{
		{ID: pvID, StudentID: pvChild, Status: "rejected", RejectionReason: "ilegible", UploadedByName: "Rosa", DecidedByName: "Caja"},
	}}
	h := newPV(svc)
	rec := httptest.NewRecorder()
	h.FamilyList(rec, pvRequest(http.MethodGet, "/x", nil, "", map[string]string{"studentId": pvChild}, "family:read"))
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status = %d, cache %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if svc.scope.SchoolQueue || len(svc.scope.StudentIDs) != 1 || svc.scope.StudentIDs[0] != pvChild {
		t.Fatalf("scope = %+v: the family reads its own child only", svc.scope)
	}
	if strings.Contains(rec.Body.String(), "Caja") || !strings.Contains(rec.Body.String(), "ilegible") {
		t.Fatalf("body = %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	svc.listCalls = 0
	h.FamilyList(rec, pvRequest(http.MethodGet, "/x", nil, "", map[string]string{"studentId": pvOther}, "family:read"))
	if rec.Code != http.StatusNotFound || svc.listCalls != 0 {
		t.Fatalf("another child: %d, calls %d", rec.Code, svc.listCalls)
	}
}

func TestStaffQueue(t *testing.T) {
	svc := &fakeVouchers{list: []client.PaymentVoucher{{ID: pvID, StudentID: pvChild, Status: "pending", UploadedByName: "Rosa"}}}
	h := newPV(svc)
	rec := httptest.NewRecorder()
	h.Queue(rec, pvRequest(http.MethodGet, "/api/v1/finance/payment-vouchers?status=pending&studentId="+pvChild+"&limit=20", nil, "", nil, "collections:read"))
	if rec.Code != http.StatusOK || !svc.scope.SchoolQueue || svc.status != "pending" || svc.limit != 20 ||
		len(svc.scope.StudentIDs) != 1 || svc.scope.StudentIDs[0] != pvChild {
		t.Fatalf("status %d scope %+v status %q limit %d", rec.Code, svc.scope, svc.status, svc.limit)
	}
	if !strings.Contains(rec.Body.String(), `"uploadedByName":"Rosa"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	for _, q := range []string{"?status=lost", "?studentId=x", "?limit=0", "?limit=501", "?limit=many"} {
		svc.listCalls = 0
		rec := httptest.NewRecorder()
		h.Queue(rec, pvRequest(http.MethodGet, "/api/v1/finance/payment-vouchers"+q, nil, "", nil, "collections:read"))
		if rec.Code != http.StatusBadRequest || svc.listCalls != 0 {
			t.Errorf("%s: status %d", q, rec.Code)
		}
	}
}

func TestStaffReadsTheFileSandboxed(t *testing.T) {
	svc := &fakeVouchers{file: pvPDF, fileType: "application/pdf"}
	h := newPV(svc)
	rec := httptest.NewRecorder()
	h.File(rec, pvRequest(http.MethodGet, "/x", nil, "", map[string]string{"id": pvID}, "collections:read"))
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), pvPDF) || svc.fileID != pvID || !svc.scope.SchoolQueue {
		t.Fatalf("status %d, id %q, scope %+v", rec.Code, svc.fileID, svc.scope)
	}
	for header, want := range map[string]string{
		"Content-Type":           "application/pdf",
		"Cache-Control":          "private, no-store",
		"X-Content-Type-Options": "nosniff",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.HasPrefix(csp, "sandbox") {
		t.Errorf("CSP = %q", csp)
	}
	// Review: a family's upload is never rendered as a page of the app's
	// origin — the web previews it from a Blob it fetched.
	if !strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Errorf("disposition = %q", rec.Header().Get("Content-Disposition"))
	}

	rec = httptest.NewRecorder()
	h.File(rec, pvRequest(http.MethodGet, "/x", nil, "", map[string]string{"id": "x"}, "collections:read"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("a malformed id: %d", rec.Code)
	}
}

func TestStaffRejectsAVoucher(t *testing.T) {
	svc := &fakeVouchers{voucher: client.PaymentVoucher{ID: pvID, Status: "rejected", RejectionReason: "ilegible"}}
	h := newPV(svc)
	rec := httptest.NewRecorder()
	h.Reject(rec, pvRequest(http.MethodPost, "/x", bytes.NewBufferString(`{"reason":"  ilegible  "}`), "application/json",
		map[string]string{"id": pvID}, "payments:write"))
	if rec.Code != http.StatusOK || svc.rejectID != pvID || svc.reason != "ilegible" || svc.rejectBy.UserID != "g-1" {
		t.Fatalf("status %d, id %q reason %q by %+v", rec.Code, svc.rejectID, svc.reason, svc.rejectBy)
	}
	for name, body := range map[string]string{
		"no reason":      `{}`,
		"two letters":    `{"reason":"no"}`,
		"301 characters": `{"reason":"` + strings.Repeat("a", 301) + `"}`,
		"not json":       `reason`,
	} {
		svc.rejectID = ""
		rec := httptest.NewRecorder()
		h.Reject(rec, pvRequest(http.MethodPost, "/x", bytes.NewBufferString(body), "application/json",
			map[string]string{"id": pvID}, "payments:write"))
		if rec.Code != http.StatusBadRequest || svc.rejectID != "" {
			t.Errorf("%s: status %d", name, rec.Code)
		}
	}
	rec = httptest.NewRecorder()
	h.Reject(rec, pvRequest(http.MethodPost, "/x", bytes.NewBufferString(`{"reason":"ilegible"}`), "application/json",
		map[string]string{"id": "x"}, "payments:write"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("malformed id: %d", rec.Code)
	}
}
