package handler

import (
	"context"
	"net/http"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// GuardiansService is the guardian-link surface the edge needs
// (consumer-defined, so the handler is testable with a fake). The Connect
// StudentsClient implements it.
type GuardiansService interface {
	LinkGuardian(ctx context.Context, guardianUserID, studentID, relationship string) (client.GuardianLink, error)
	UnlinkGuardian(ctx context.Context, guardianUserID, studentID string) error
	ListGuardiansForStudent(ctx context.Context, studentID string) ([]client.GuardianLink, error)
}

// GuardiansHandler exposes the front-facing (HTTP/JSON) guardian-link endpoints,
// proxying to the students service. It translates JSON ↔ the StudentsClient and
// Connect error codes ↔ HTTP status.
//
// Managing links is an ADMIN action — it decides who may see a child — so every
// route runs behind the module guard (module_students) plus RequireAuth +
// rotation-enforcement, with link/unlink gated on guardians:write and the list on
// guardians:read. students:* is deliberately NOT enough. The guardian's own,
// self-scoped view of their children lives in FamilyHandler, which derives the
// child set from the principal instead of from a request field.
type GuardiansHandler struct {
	svc GuardiansService
}

// NewGuardiansHandler wires the handler to a guardian-link service.
func NewGuardiansHandler(svc GuardiansService) *GuardiansHandler {
	return &GuardiansHandler{svc: svc}
}

// --- request bodies --------------------------------------------------------

// linkGuardianRequest ties an iam user to a student. Relationship is one of
// mother|father|guardian|other; the students service is the authoritative
// validator.
type linkGuardianRequest struct {
	GuardianUserID string `json:"guardianUserId"`
	StudentID      string `json:"studentId"`
	Relationship   string `json:"relationship"`
}

type unlinkGuardianRequest struct {
	GuardianUserID string `json:"guardianUserId"`
	StudentID      string `json:"studentId"`
}

// --- response bodies -------------------------------------------------------

type guardianLinkResponse struct {
	ID             string `json:"id"`
	TenantID       string `json:"tenantId"`
	GuardianUserID string `json:"guardianUserId"`
	StudentID      string `json:"studentId"`
	Relationship   string `json:"relationship"`
}

func toGuardianLinkResponse(l client.GuardianLink) guardianLinkResponse {
	return guardianLinkResponse{
		ID:             l.ID,
		TenantID:       l.TenantID,
		GuardianUserID: l.GuardianUserID,
		StudentID:      l.StudentID,
		Relationship:   l.Relationship,
	}
}

// --- endpoints -------------------------------------------------------------

// LinkGuardian handles POST /api/v1/guardians — link a guardian (an iam user) to
// a student. A duplicate link is 409, an unknown user or student 404.
func (h *GuardiansHandler) LinkGuardian(w http.ResponseWriter, r *http.Request) {
	var req linkGuardianRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	link, err := h.svc.LinkGuardian(r.Context(), req.GuardianUserID, req.StudentID, req.Relationship)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toGuardianLinkResponse(link))
}

// UnlinkGuardian handles POST /api/v1/guardians/unlink — revoke a guardian's
// access to a student. It is idempotent: unlinking what is not linked still
// yields 204, so a retry is safe.
func (h *GuardiansHandler) UnlinkGuardian(w http.ResponseWriter, r *http.Request) {
	var req unlinkGuardianRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := h.svc.UnlinkGuardian(r.Context(), req.GuardianUserID, req.StudentID); err != nil {
		writeConnectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListGuardiansForStudent handles GET /api/v1/students/{studentId}/guardians —
// who may see this child. The id comes from the path pattern; a query parameter
// is accepted as a fallback so the handler is usable from a flat route too.
func (h *GuardiansHandler) ListGuardiansForStudent(w http.ResponseWriter, r *http.Request) {
	studentID := r.PathValue("studentId")
	if studentID == "" {
		studentID = r.URL.Query().Get("studentId")
	}
	links, err := h.svc.ListGuardiansForStudent(r.Context(), studentID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"guardians": toGuardianLinkResponses(links)})
}

// toGuardianLinkResponses always returns a non-nil slice so the JSON carries an
// empty array rather than null.
func toGuardianLinkResponses(links []client.GuardianLink) []guardianLinkResponse {
	out := make([]guardianLinkResponse, 0, len(links))
	for _, l := range links {
		out = append(out, toGuardianLinkResponse(l))
	}
	return out
}
