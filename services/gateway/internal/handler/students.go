package handler

import (
	"context"
	"net/http"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// StudentCreator is the students surface the edge needs (consumer-defined, so the
// handler is testable with a fake). The Connect StudentsClient implements it.
type StudentCreator interface {
	CreateApplicant(ctx context.Context, in client.CreateApplicantInput) (client.StudentResult, error)
}

// StudentsHandler exposes the front-facing (HTTP/JSON) students endpoints,
// proxying to the students service. It translates JSON ↔ the StudentsClient and
// Connect error codes ↔ HTTP status. It must run behind the module guard
// (module_students) plus RequireAuth + RequirePermission("students:create").
type StudentsHandler struct {
	svc StudentCreator
}

// NewStudentsHandler wires the handler to a student creator.
func NewStudentsHandler(svc StudentCreator) *StudentsHandler {
	return &StudentsHandler{svc: svc}
}

type createStudentRequest struct {
	CURP         string `json:"curp"`
	FirstName    string `json:"firstName"`
	LastName     string `json:"lastName"`
	Level        string `json:"level"`
	GradeGroup   string `json:"gradeGroup"`
	GuardianName string `json:"guardianName"`
}

type studentResponse struct {
	ID              string `json:"id"`
	CURP            string `json:"curp"`
	FirstName       string `json:"firstName"`
	LastName        string `json:"lastName"`
	Level           string `json:"level"`
	GradeGroup      string `json:"gradeGroup"`
	GuardianName    string `json:"guardianName"`
	AdmissionStatus string `json:"admissionStatus"`
}

// Create handles POST /api/v1/students: registers a new applicant (pending).
func (h *StudentsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createStudentRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	res, err := h.svc.CreateApplicant(r.Context(), client.CreateApplicantInput{
		CURP:         req.CURP,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Level:        req.Level,
		GradeGroup:   req.GradeGroup,
		GuardianName: req.GuardianName,
	})
	if err != nil {
		status := httpStatusFor(err)
		http.Error(w, http.StatusText(status), status)
		return
	}
	writeJSON(w, http.StatusCreated, studentResponse{
		ID:              res.ID,
		CURP:            res.CURP,
		FirstName:       res.FirstName,
		LastName:        res.LastName,
		Level:           res.Level,
		GradeGroup:      res.GradeGroup,
		GuardianName:    res.GuardianName,
		AdmissionStatus: res.AdmissionStatus,
	})
}
