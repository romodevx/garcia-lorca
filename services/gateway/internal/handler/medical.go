package handler

import (
	"context"
	"net/http"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// MedicalService is the medical-record surface the edge needs.
type MedicalService interface {
	GetStudentMedical(ctx context.Context, studentID string) (client.StudentMedical, error)
	UpdateStudentMedical(ctx context.Context, studentID string, rec client.StudentMedical) (client.StudentMedical, error)
}

// MedicalHandler serves a pupil's medical record. Its routes answer to
// medical:read / medical:write, not students:* — who reads the class list is
// not thereby entitled to its allergies (task 5).
type MedicalHandler struct {
	svc MedicalService
}

// NewMedicalHandler wires the handler to the students service.
func NewMedicalHandler(svc MedicalService) *MedicalHandler {
	return &MedicalHandler{svc: svc}
}

// Get handles GET /api/v1/students/{id}/medical.
func (h *MedicalHandler) Get(w http.ResponseWriter, r *http.Request) {
	rec, err := h.svc.GetStudentMedical(r.Context(), r.PathValue("id"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	// A medical record is never cached by a shared computer's browser.
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, rec)
}

// Put handles PUT /api/v1/students/{id}/medical.
func (h *MedicalHandler) Put(w http.ResponseWriter, r *http.Request) {
	var body client.StudentMedical
	if err := decodeJSONBody(w, r, &body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	rec, err := h.svc.UpdateStudentMedical(r.Context(), r.PathValue("id"), body)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, rec)
}
