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
	AddContact(ctx context.Context, studentID, relationship, fullName, phone, email string) (client.GuardianLink, error)
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
	FullName       string `json:"fullName"`
	Phone          string `json:"phone"`
	Email          string `json:"email"`
	// HasAccount is what the screen actually asks: can this contact open the
	// family app, or is it a phone number the office calls?
	HasAccount bool `json:"hasAccount"`
}

func toGuardianLinkResponse(l client.GuardianLink) guardianLinkResponse {
	return guardianLinkResponse{
		ID:             l.ID,
		TenantID:       l.TenantID,
		GuardianUserID: l.GuardianUserID,
		StudentID:      l.StudentID,
		Relationship:   l.Relationship,
		FullName:       l.FullName,
		Phone:          l.Phone,
		Email:          l.Email,
		HasAccount:     l.GuardianUserID != "",
	}
}

// --- endpoints -------------------------------------------------------------

// LinkGuardian handles POST /api/v1/guardians — link a guardian (an iam user) to
// a student. A duplicate link is 409. An unknown user, an unknown student and a
// bad relationship are all 400: the students service treats every reference in
// the body as an ARGUMENT, and nothing in the URL addresses a resource that could
// be missing.
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
// access to a student. Revoking a link that is there yields 204; unlinking an
// unknown link returns 404, because the students contract answers NotFound. That
// is the admin UI's "there was nothing to revoke" signal, so a retry is NOT a
// silent success.
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

// contactRequest is a person, not an account.
type contactRequest struct {
	StudentID    string `json:"studentId"`
	Relationship string `json:"relationship"`
	FullName     string `json:"fullName"`
	Phone        string `json:"phone"`
	Email        string `json:"email"`
}

// AddContact handles POST /api/v1/contacts — record somebody the school can
// reach about a pupil, with no account and no login.
//
// It is separate from LinkGuardian because the two answer different questions:
// one grants access to the family app, the other writes down the number that
// answers. Conflating them is why the school kept the second number on paper.
func (h *GuardiansHandler) AddContact(w http.ResponseWriter, r *http.Request) {
	var req contactRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	contact, err := h.svc.AddContact(r.Context(), req.StudentID, req.Relationship, req.FullName, req.Phone, req.Email)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toGuardianLinkResponse(contact))
}

// ListGuardiansForStudent handles GET /api/v1/students/{studentId}/guardians —
// who may see this child. The id comes from the path pattern, which is the only
// route registered for it.
func (h *GuardiansHandler) ListGuardiansForStudent(w http.ResponseWriter, r *http.Request) {
	links, err := h.svc.ListGuardiansForStudent(r.Context(), r.PathValue("studentId"))
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
