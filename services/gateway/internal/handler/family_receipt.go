package handler

import (
	"context"
	"net/http"

	"connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/middleware"
	"github.com/garcia-lorca/services/gateway/internal/receipt"
)

// FamilyReceiptSource builds the receipt of one of a pupil's deposits for the
// family: NotFound for a deposit that is not that pupil's. receipt.Builder
// satisfies it.
type FamilyReceiptSource interface {
	FamilyPayment(ctx context.Context, studentID, depositID string) (receipt.Receipt, error)
}

// WithFamilyReceipts wires the receipts the family's receipt routes serve
// (ISSUE-T14-107).
func WithFamilyReceipts(src FamilyReceiptSource) FamilyOption {
	return func(h *FamilyHandler) { h.receipts = src }
}

// ReceiptJSON handles GET /api/v1/family/students/{studentId}/receipts/{depositId}.
func (h *FamilyHandler) ReceiptJSON(w http.ResponseWriter, r *http.Request) {
	if got, ok := h.familyReceipt(w, r); ok {
		writeReceiptJSON(w, got)
	}
}

// ReceiptPDF handles GET /api/v1/family/students/{studentId}/receipts/{depositId}/pdf.
func (h *FamilyHandler) ReceiptPDF(w http.ResponseWriter, r *http.Request) {
	if got, ok := h.familyReceipt(w, r); ok {
		writeReceiptPDF(w, got)
	}
}

// familyReceipt is the receipt of the path's deposit, scoped twice (ADR 0004):
// the path's child must be the principal's own (404 otherwise, before finance
// is read), and the deposit must be that child's (the builder answers
// NotFound otherwise). Both refusals read as a missing receipt.
func (h *FamilyHandler) familyReceipt(w http.ResponseWriter, r *http.Request) (receipt.Receipt, bool) {
	if h.receipts == nil {
		writeNotFound(w)
		return receipt.Receipt{}, false
	}
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return receipt.Receipt{}, false
	}
	studentID := r.PathValue("studentId")
	if !h.guardOwnsChild(w, r, p.UserID, studentID) {
		return receipt.Receipt{}, false
	}
	got, err := h.receipts.FamilyPayment(r.Context(), studentID, r.PathValue("depositId"))
	if connect.CodeOf(err) == connect.CodeNotFound {
		// One answer for every missing receipt (ISSUE-T14-302): finance's own
		// sentence or code for an unknown id would tell it apart from a
		// deposit of another pupil.
		writeNotFound(w)
		return receipt.Receipt{}, false
	}
	if err != nil {
		writeConnectError(w, err)
		return receipt.Receipt{}, false
	}
	return got, true
}
