package handler

import (
	"context"
	"net/http"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// GradesService is the grade-capture surface the edge needs (consumer-defined,
// so the handler is testable with a fake). The Connect GradesClient implements
// it.
type GradesService interface {
	CaptureGrade(ctx context.Context, assignmentID, enrollmentID, termID string, value float64) (client.GradeEntry, error)
	GetRoster(ctx context.Context, assignmentID, termID string) ([]client.RosterRow, error)
	ListGradeEntries(ctx context.Context, enrollmentID, termID string) ([]client.GradeEntry, error)
}

// GradesHandler exposes the front-facing (HTTP/JSON) grade-capture endpoints,
// proxying to the grades service. It translates JSON ↔ the GradesClient and
// Connect error codes ↔ HTTP status. Every route must run behind the module
// guard (module_grades) plus RequireAuth + rotation-enforcement, with the write
// (capture) gated on grades:write and reads on grades:read.
type GradesHandler struct {
	svc GradesService
}

// NewGradesHandler wires the handler to a grade service.
func NewGradesHandler(svc GradesService) *GradesHandler {
	return &GradesHandler{svc: svc}
}

// captureGradeRequest carries only the client-supplied edges of the capture
// knot. subjectId and teacherId are intentionally absent: the grades service
// derives them from the teaching assignment so they can never be forged.
type captureGradeRequest struct {
	AssignmentID string  `json:"assignmentId"`
	EnrollmentID string  `json:"enrollmentId"`
	TermID       string  `json:"termId"`
	Value        float64 `json:"value"`
}

type gradeEntryResponse struct {
	ID           string  `json:"id"`
	EnrollmentID string  `json:"enrollmentId"`
	SubjectID    string  `json:"subjectId"`
	TermID       string  `json:"termId"`
	TeacherID    string  `json:"teacherId"`
	Value        float64 `json:"value"`
}

func toGradeEntryResponse(e client.GradeEntry) gradeEntryResponse {
	return gradeEntryResponse{
		ID:           e.ID,
		EnrollmentID: e.EnrollmentID,
		SubjectID:    e.SubjectID,
		TermID:       e.TermID,
		TeacherID:    e.TeacherID,
		Value:        e.Value,
	}
}

type rosterRowResponse struct {
	EnrollmentID string  `json:"enrollmentId"`
	StudentID    string  `json:"studentId"`
	Value        float64 `json:"value"`
	HasValue     bool    `json:"hasValue"`
}

func toRosterRowResponse(r client.RosterRow) rosterRowResponse {
	return rosterRowResponse{
		EnrollmentID: r.EnrollmentID,
		StudentID:    r.StudentID,
		Value:        r.Value,
		HasValue:     r.HasValue,
	}
}

// CaptureGrade handles POST /api/v1/grades: record or edit a student's numeric
// grade for a term. The body carries only {assignmentId, enrollmentId, termId,
// value}; subject/teacher are derived server-side.
func (h *GradesHandler) CaptureGrade(w http.ResponseWriter, r *http.Request) {
	var req captureGradeRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	res, err := h.svc.CaptureGrade(r.Context(), req.AssignmentID, req.EnrollmentID, req.TermID, req.Value)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toGradeEntryResponse(res))
}

// GetRoster handles GET /api/v1/grades/roster?assignmentId=...&termId=...
func (h *GradesHandler) GetRoster(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	assignmentID, ok := requireUUIDQuery(w, q, "assignmentId")
	if !ok {
		return
	}
	termID, ok := optionalUUIDQuery(w, q, "termId")
	if !ok {
		return
	}
	res, err := h.svc.GetRoster(r.Context(), assignmentID, termID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]rosterRowResponse, 0, len(res))
	for _, row := range res {
		out = append(out, toRosterRowResponse(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{"roster": out})
}

// ListGradeEntries handles GET /api/v1/grades?enrollmentId=...&termId=...
func (h *GradesHandler) ListGradeEntries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := h.svc.ListGradeEntries(r.Context(), q.Get("enrollmentId"), q.Get("termId"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]gradeEntryResponse, 0, len(res))
	for _, e := range res {
		out = append(out, toGradeEntryResponse(e))
	}
	writeJSON(w, http.StatusOK, map[string]any{"gradeEntries": out})
}
