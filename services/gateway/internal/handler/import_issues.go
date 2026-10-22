package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// ImportIssueService is the students surface for a pupil's import issues
// (PLAN-import-avisos). Consumer-defined; the StudentsClient implements it.
type ImportIssueService interface {
	ListImportIssues(ctx context.Context, studentID string, includeResolved bool) ([]client.StudentImportIssue, error)
	AcceptImportIssue(ctx context.Context, studentID, issueID, userID string) (client.StudentImportIssue, error)
}

// ImportIssuesHandler serves /api/v1/students/{id}/import-issues. Its routes
// run behind module_students AND module_import_review; listing is
// students:read, accepting students:update (requireAuthChanged, like PUT).
type ImportIssuesHandler struct {
	svc ImportIssueService
}

// NewImportIssuesHandler wires the handler to students.
func NewImportIssuesHandler(svc ImportIssueService) *ImportIssuesHandler {
	return &ImportIssuesHandler{svc: svc}
}

type importIssueResponse struct {
	ID              string `json:"id"`
	StudentID       string `json:"studentId"`
	Field           string `json:"field"`
	Code            string `json:"code"`
	OriginalValue   string `json:"originalValue"`
	StoredValue     string `json:"storedValue"`
	CreatedAt       string `json:"createdAt"`
	ResolvedAt      string `json:"resolvedAt,omitempty"`
	Resolution      string `json:"resolution"`
	IntendedGroupID string `json:"intendedGroupId,omitempty"`
}

func toImportIssueResponse(i client.StudentImportIssue) importIssueResponse {
	out := importIssueResponse{
		ID: i.ID, StudentID: i.StudentID, Field: i.Field, Code: i.Code, OriginalValue: i.OriginalValue,
		StoredValue: i.StoredValue, CreatedAt: i.CreatedAt.UTC().Format(time.RFC3339),
		Resolution: i.Resolution, IntendedGroupID: i.IntendedGroupID,
	}
	if !i.ResolvedAt.IsZero() {
		out.ResolvedAt = i.ResolvedAt.UTC().Format(time.RFC3339)
	}
	return out
}

// List handles GET /api/v1/students/{id}/import-issues?includeResolved=bool.
func (h *ImportIssuesHandler) List(w http.ResponseWriter, r *http.Request) {
	includeResolved := false
	if raw := r.URL.Query().Get("includeResolved"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			http.Error(w, "includeResolved must be true or false", http.StatusBadRequest)
			return
		}
		includeResolved = parsed
	}
	issues, err := h.svc.ListImportIssues(r.Context(), r.PathValue("id"), includeResolved)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]importIssueResponse, 0, len(issues))
	for _, i := range issues {
		out = append(out, toImportIssueResponse(i))
	}
	writeJSON(w, http.StatusOK, map[string]any{"issues": out})
}

// Accept handles POST /api/v1/students/{id}/import-issues/{issueId}/accept:
// "está bien así", by the signed-in caller. A draft's curp issue is refused by
// students (409 STUDENT_DRAFT, relayed).
func (h *ImportIssuesHandler) Accept(w http.ResponseWriter, r *http.Request) {
	who, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	issue, err := h.svc.AcceptImportIssue(r.Context(), r.PathValue("id"), r.PathValue("issueId"), who.UserID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toImportIssueResponse(issue))
}
