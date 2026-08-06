package handler

import (
	"context"
	"net/http"
	"time"

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

// --- enrollments ----------------------------------------------------------

// EnrollmentService is the enrollment surface the edge needs (consumer-defined,
// so the handler is testable with a fake). The Connect StudentsClient implements
// it. Enroll takes no cycleID: the students service derives the cycle from the
// group so it can never drift.
type EnrollmentService interface {
	Enroll(ctx context.Context, studentID, groupID, termID string) (client.Enrollment, error)
	ListEnrollments(ctx context.Context, studentID, cycleID, groupID string) ([]client.Enrollment, error)
	GetEnrollment(ctx context.Context, id string) (client.Enrollment, error)
	TransferStudent(ctx context.Context, enrollmentID, toGroupID, reason string) (client.Enrollment, error)
	WithdrawEnrollment(ctx context.Context, id string) (client.Enrollment, error)
}

// EnrollmentsHandler exposes the front-facing (HTTP/JSON) enrollment endpoints,
// proxying to the students service. It translates JSON ↔ the StudentsClient and
// Connect error codes ↔ HTTP status. Every route must run behind the module
// guard (module_students) plus RequireAuth + rotation-enforcement, with writes
// (enroll/transfer/withdraw) gated on students:enroll and reads on students:read.
type EnrollmentsHandler struct {
	svc EnrollmentService
}

// NewEnrollmentsHandler wires the handler to an enrollment service.
func NewEnrollmentsHandler(svc EnrollmentService) *EnrollmentsHandler {
	return &EnrollmentsHandler{svc: svc}
}

// enrollRequest carries the client-supplied edges. cycleId is intentionally
// absent: the students service derives it from the group so it can never drift.
type enrollRequest struct {
	StudentID string `json:"studentId"`
	GroupID   string `json:"groupId"`
	TermID    string `json:"termId"`
}

type transferRequest struct {
	EnrollmentID string `json:"enrollmentId"`
	ToGroupID    string `json:"toGroupId"`
	Reason       string `json:"reason"`
}

type withdrawRequest struct {
	EnrollmentID string `json:"enrollmentId"`
}

type enrollmentResponse struct {
	ID         string `json:"id"`
	StudentID  string `json:"studentId"`
	CycleID    string `json:"cycleId"`
	GroupID    string `json:"groupId"`
	TermID     string `json:"termId"`
	Status     string `json:"status"`
	EnrolledAt string `json:"enrolledAt"`
}

func toEnrollmentResponse(e client.Enrollment) enrollmentResponse {
	enrolledAt := ""
	if !e.EnrolledAt.IsZero() {
		enrolledAt = e.EnrolledAt.UTC().Format(time.RFC3339)
	}
	return enrollmentResponse{
		ID:         e.ID,
		StudentID:  e.StudentID,
		CycleID:    e.CycleID,
		GroupID:    e.GroupID,
		TermID:     e.TermID,
		Status:     e.Status,
		EnrolledAt: enrolledAt,
	}
}

// Enroll handles POST /api/v1/enrollments: places a student into a group. The
// response carries the server-derived cycleId.
func (h *EnrollmentsHandler) Enroll(w http.ResponseWriter, r *http.Request) {
	var req enrollRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	res, err := h.svc.Enroll(r.Context(), req.StudentID, req.GroupID, req.TermID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toEnrollmentResponse(res))
}

// ListEnrollments handles
// GET /api/v1/enrollments?studentId=...&cycleId=...&groupId=...
func (h *EnrollmentsHandler) ListEnrollments(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := h.svc.ListEnrollments(r.Context(), q.Get("studentId"), q.Get("cycleId"), q.Get("groupId"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]enrollmentResponse, 0, len(res))
	for _, e := range res {
		out = append(out, toEnrollmentResponse(e))
	}
	writeJSON(w, http.StatusOK, map[string]any{"enrollments": out})
}

// GetEnrollment handles GET /api/v1/enrollments/{id}.
func (h *EnrollmentsHandler) GetEnrollment(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.GetEnrollment(r.Context(), r.PathValue("id"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toEnrollmentResponse(res))
}

// Transfer handles POST /api/v1/enrollments/transfer: moves an active enrollment
// to another group in the same cycle, recording a dated, reasoned history row.
func (h *EnrollmentsHandler) Transfer(w http.ResponseWriter, r *http.Request) {
	var req transferRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	res, err := h.svc.TransferStudent(r.Context(), req.EnrollmentID, req.ToGroupID, req.Reason)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toEnrollmentResponse(res))
}

// Withdraw handles POST /api/v1/enrollments/withdraw: marks an active enrollment
// as withdrawn.
func (h *EnrollmentsHandler) Withdraw(w http.ResponseWriter, r *http.Request) {
	var req withdrawRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	res, err := h.svc.WithdrawEnrollment(r.Context(), req.EnrollmentID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toEnrollmentResponse(res))
}
