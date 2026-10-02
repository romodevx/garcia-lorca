package handler

import (
	"context"
	"net/http"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// BirthdayTemplates reads and replaces the school's birthday greeting (T11).
type BirthdayTemplates interface {
	GetBirthdayTemplate(ctx context.Context) (client.BirthdayTemplate, error)
	UpdateBirthdayTemplate(ctx context.Context, title, body, updatedBy string) (client.BirthdayTemplate, error)
}

// BirthdayTemplateHandler serves the greeting the daily birthday run sends. The
// run itself is NOT here: Cloud Scheduler calls comms directly, so nothing at
// the edge can trigger a send.
type BirthdayTemplateHandler struct {
	comms BirthdayTemplates
}

// NewBirthdayTemplateHandler wires the edge to comms.
func NewBirthdayTemplateHandler(comms BirthdayTemplates) *BirthdayTemplateHandler {
	return &BirthdayTemplateHandler{comms: comms}
}

type birthdayTemplateResponse struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	IsDefault bool   `json:"isDefault"`
	UpdatedAt string `json:"updatedAt"`
}

type updateBirthdayTemplateRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Get handles GET /api/v1/comms/birthday-template.
func (h *BirthdayTemplateHandler) Get(w http.ResponseWriter, r *http.Request) {
	t, err := h.comms.GetBirthdayTemplate(r.Context())
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toBirthdayTemplateResponse(t))
}

// Update handles PUT /api/v1/comms/birthday-template. Who saved it is the
// session's principal; the body cannot say otherwise.
func (h *BirthdayTemplateHandler) Update(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req updateBirthdayTemplateRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	t, err := h.comms.UpdateBirthdayTemplate(r.Context(), req.Title, req.Body, p.UserID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toBirthdayTemplateResponse(t))
}

func toBirthdayTemplateResponse(t client.BirthdayTemplate) birthdayTemplateResponse {
	return birthdayTemplateResponse{Title: t.Title, Body: t.Body, IsDefault: t.IsDefault, UpdatedAt: t.UpdatedAt}
}
