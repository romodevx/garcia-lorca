package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/receipt"
)

type fakeReceipts struct {
	got receipt.Receipt
	err error
	id  string
}

func (f *fakeReceipts) Payment(_ context.Context, id string) (receipt.Receipt, error) {
	f.id = id
	return f.got, f.err
}

func (f *fakeReceipts) Sale(_ context.Context, id string) (receipt.Receipt, error) {
	f.id = id
	return f.got, f.err
}

func receiptRequest(path, id string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.SetPathValue("id", id)
	return req
}

// Task 12: the receipt as JSON for the ticket the browser prints, and as a PDF
// download named after its folio. Neither is cached: it names a family's money.
func TestReceiptsServeJSONAndPDF(t *testing.T) {
	svc := &fakeReceipts{got: receipt.Receipt{Kind: receipt.KindPayment, Folio: "P-2026-000042", PaidOn: "2026-10-10", TotalCents: 350000}}
	h := handler.NewReceiptsHandler(svc)

	rec := httptest.NewRecorder()
	h.PaymentJSON(rec, receiptRequest("/api/v1/receipts/payment/dep-1", "dep-1"))
	if rec.Code != http.StatusOK || svc.id != "dep-1" {
		t.Fatalf("JSON: %d, asked for %q", rec.Code, svc.id)
	}
	var body receipt.Receipt
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Folio != "P-2026-000042" {
		t.Errorf("JSON body = %s (%v)", rec.Body.String(), err)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("JSON Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}

	rec = httptest.NewRecorder()
	h.PaymentPDF(rec, receiptRequest("/api/v1/receipts/payment/dep-1/pdf", "dep-1"))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("PDF: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="comprobante-P-2026-000042.pdf"` {
		t.Errorf("Content-Disposition = %q", got)
	}
	if !bytes.HasPrefix(rec.Body.Bytes(), []byte("%PDF-")) || rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("PDF body or caching wrong")
	}

	rec = httptest.NewRecorder()
	svc.got = receipt.Receipt{Kind: receipt.KindSale, Folio: "U-2026-000007"}
	h.SaleJSON(rec, receiptRequest("/api/v1/receipts/sale/ord-1", "ord-1"))
	if rec.Code != http.StatusOK || svc.id != "ord-1" {
		t.Errorf("sale JSON: %d, asked for %q", rec.Code, svc.id)
	}
}

func TestAnUnknownReceiptIs404(t *testing.T) {
	h := handler.NewReceiptsHandler(&fakeReceipts{err: connect.NewError(connect.CodeNotFound, nil)})
	for name, serve := range map[string]http.HandlerFunc{"json": h.PaymentJSON, "pdf": h.SalePDF} {
		rec := httptest.NewRecorder()
		serve(rec, receiptRequest("/api/v1/receipts/x/nope", "nope"))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", name, rec.Code)
		}
	}
}
