package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/garcia-lorca/services/gateway/internal/receipt"
)

// ReceiptBuilder assembles a receipt from the services that own its parts.
type ReceiptBuilder interface {
	Payment(ctx context.Context, depositID string) (receipt.Receipt, error)
	Sale(ctx context.Context, orderID string) (receipt.Receipt, error)
}

// ReceiptsHandler serves the proof of payment (task 12): JSON for the 80 mm
// ticket the browser prints, and a PDF to download. Each route runs behind the
// permissions of what it proves — a deposit's, the counter's or finance's; a
// sale's, the orders' (cmd/main.go).
type ReceiptsHandler struct {
	receipts ReceiptBuilder
}

// NewReceiptsHandler wires the handler.
func NewReceiptsHandler(receipts ReceiptBuilder) *ReceiptsHandler {
	return &ReceiptsHandler{receipts: receipts}
}

// PaymentJSON handles GET /api/v1/receipts/payment/{id}.
func (h *ReceiptsHandler) PaymentJSON(w http.ResponseWriter, r *http.Request) {
	h.serveJSON(w, r, h.receipts.Payment)
}

// PaymentPDF handles GET /api/v1/receipts/payment/{id}/pdf.
func (h *ReceiptsHandler) PaymentPDF(w http.ResponseWriter, r *http.Request) {
	h.servePDF(w, r, h.receipts.Payment)
}

// SaleJSON handles GET /api/v1/receipts/sale/{id}.
func (h *ReceiptsHandler) SaleJSON(w http.ResponseWriter, r *http.Request) {
	h.serveJSON(w, r, h.receipts.Sale)
}

// SalePDF handles GET /api/v1/receipts/sale/{id}/pdf.
func (h *ReceiptsHandler) SalePDF(w http.ResponseWriter, r *http.Request) {
	h.servePDF(w, r, h.receipts.Sale)
}

type build func(ctx context.Context, id string) (receipt.Receipt, error)

func (h *ReceiptsHandler) serveJSON(w http.ResponseWriter, r *http.Request, get build) {
	got, err := get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeReceiptJSON(w, got)
}

func (h *ReceiptsHandler) servePDF(w http.ResponseWriter, r *http.Request, get build) {
	got, err := get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeReceiptPDF(w, got)
}

// writeReceiptJSON answers a receipt as the ticket's JSON.
func writeReceiptJSON(w http.ResponseWriter, got receipt.Receipt) {
	// It names a family's money: no shared cache keeps a copy.
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, got)
}

// writeReceiptPDF answers a receipt as a PDF download named after its folio.
func writeReceiptPDF(w http.ResponseWriter, got receipt.Receipt) {
	doc, err := receipt.PDF(got)
	if err != nil {
		http.Error(w, "the receipt could not be rendered", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "comprobante-"+got.Folio+".pdf"))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(doc) // the status is sent; a client that hung up is not an error to report
}
