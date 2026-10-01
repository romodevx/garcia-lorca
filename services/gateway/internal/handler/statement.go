package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
	"github.com/garcia-lorca/services/gateway/internal/receipt"
)

// StatementSource is the student's ledger (T7), as JSON and ready to print.
// finance computes it; this layer only renders it.
type StatementSource interface {
	Ledger(ctx context.Context, studentID string, family bool) (client.Statement, error)
	Document(ctx context.Context, studentID string, family bool) (receipt.StatementDoc, error)
}

// StatementHandler serves the office's view of a student's ledger (T7):
// GET /api/v1/finance/accounts/{studentId}/statement and its /pdf. Behind the
// same permissions as the account it details (cmd/main.go).
type StatementHandler struct {
	statements StatementSource
}

// NewStatementHandler wires the handler.
func NewStatementHandler(statements StatementSource) *StatementHandler {
	return &StatementHandler{statements: statements}
}

// JSON handles GET /api/v1/finance/accounts/{studentId}/statement.
func (h *StatementHandler) JSON(w http.ResponseWriter, r *http.Request) {
	serveStatementJSON(w, r, h.statements, r.PathValue("studentId"), false)
}

// PDF handles GET /api/v1/finance/accounts/{studentId}/statement/pdf.
func (h *StatementHandler) PDF(w http.ResponseWriter, r *http.Request) {
	serveStatementPDF(w, r, h.statements, r.PathValue("studentId"), false)
}

// Statement handles GET /api/v1/family/students/{studentId}/statement — the
// family's view of its own child's ledger.
func (h *FamilyHandler) Statement(w http.ResponseWriter, r *http.Request) {
	if studentID, ok := h.ownedChild(w, r); ok {
		serveStatementJSON(w, r, h.statements, studentID, true)
	}
}

// StatementPDF handles GET /api/v1/family/students/{studentId}/statement/pdf.
func (h *FamilyHandler) StatementPDF(w http.ResponseWriter, r *http.Request) {
	if studentID, ok := h.ownedChild(w, r); ok {
		serveStatementPDF(w, r, h.statements, studentID, true)
	}
}

// ownedChild is the path's child when it is the principal's own (401 with no
// principal, 404 for anyone else's — the answer a missing id gets).
func (h *FamilyHandler) ownedChild(w http.ResponseWriter, r *http.Request) (string, bool) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", false
	}
	studentID := r.PathValue("studentId")
	return studentID, h.guardOwnsChild(w, r, p.UserID, studentID)
}

func serveStatementJSON(w http.ResponseWriter, r *http.Request, src StatementSource, studentID string, family bool) {
	got, err := src.Ledger(r.Context(), studentID, family)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	// It names a family's money: no shared cache keeps a copy.
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, got)
}

func serveStatementPDF(w http.ResponseWriter, r *http.Request, src StatementSource, studentID string, family bool) {
	doc, err := src.Document(r.Context(), studentID, family)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	body, err := receipt.StatementPDF(doc)
	if err != nil {
		http.Error(w, "the statement could not be rendered", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "estado-de-cuenta-"+doc.Statement.AsOf+".pdf"))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body) // the status is sent; a client that hung up is not an error to report
}
