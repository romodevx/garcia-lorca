package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// StudentsService is the students surface the edge needs (consumer-defined, so
// the handler is testable with a fake). The Connect StudentsClient implements it.
type StudentsService interface {
	CreateApplicant(ctx context.Context, in client.CreateApplicantInput) (client.StudentResult, error)
	ListStudents(ctx context.Context, status string) ([]client.StudentResult, error)
	GetStudent(ctx context.Context, id string) (client.StudentResult, error)
	UpdateStudent(ctx context.Context, in client.UpdateStudentInput) (client.StudentResult, error)
}

// StudentsHandler exposes the front-facing (HTTP/JSON) students endpoints,
// proxying to the students service. It translates JSON ↔ the StudentsClient and
// Connect error codes ↔ HTTP status. Every route must run behind the module
// guard (module_students) plus RequireAuth + rotation-enforcement, with the
// write (create) gated on students:create and reads (list/get) on students:read.
type StudentsHandler struct {
	svc StudentsService
}

// NewStudentsHandler wires the handler to a students service.
func NewStudentsHandler(svc StudentsService) *StudentsHandler {
	return &StudentsHandler{svc: svc}
}

type createStudentRequest struct {
	CURP         string `json:"curp"`
	FirstName    string `json:"firstName"`
	LastName     string `json:"lastName"`
	LevelID      string `json:"levelId"`
	GradeGroup   string `json:"gradeGroup"`
	GuardianName string `json:"guardianName"`
}

type studentResponse struct {
	ID              string `json:"id"`
	CURP            string `json:"curp"`
	FirstName       string `json:"firstName"`
	LastName        string `json:"lastName"`
	Level           string `json:"level"`   // the school's name for the level
	LevelID         string `json:"levelId"` // the config level id
	GradeGroup      string `json:"gradeGroup"`
	GuardianName    string `json:"guardianName"`
	AdmissionStatus string `json:"admissionStatus"`
	// EnrollmentNumber is the matrícula (task 13).
	EnrollmentNumber string `json:"enrollmentNumber,omitempty"`
	// Nil in lists: a picker needs names, not every pupil's address and phones.
	Profile *client.StudentProfile `json:"profile,omitempty"`
}

func toStudentResponse(s client.StudentResult) studentResponse {
	return studentResponse{
		ID:               s.ID,
		CURP:             s.CURP,
		FirstName:        s.FirstName,
		LastName:         s.LastName,
		Level:            s.Level,
		LevelID:          s.LevelID,
		GradeGroup:       s.GradeGroup,
		GuardianName:     s.GuardianName,
		AdmissionStatus:  s.AdmissionStatus,
		EnrollmentNumber: s.EnrollmentNumber,
		Profile:          &s.Profile,
	}
}

// toStudentSummary is a pupil as a list shows it: without the profile.
func toStudentSummary(s client.StudentResult) studentResponse {
	out := toStudentResponse(s)
	out.Profile = nil
	return out
}

type updateStudentRequest struct {
	FirstName string                `json:"firstName"`
	LastName  string                `json:"lastName"`
	CURP      string                `json:"curp"`
	Profile   client.StudentProfile `json:"profile"`
}

// Update handles PUT /api/v1/students/{id}: corrects the pupil's file (names,
// CURP, profile). Level and group are not editable here — a group changes by
// transfer.
func (h *StudentsHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req updateStudentRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	res, err := h.svc.UpdateStudent(r.Context(), client.UpdateStudentInput{
		ID: r.PathValue("id"), FirstName: req.FirstName, LastName: req.LastName,
		CURP: req.CURP, Profile: req.Profile,
	})
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toStudentResponse(res))
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
		LevelID:      req.LevelID,
		GradeGroup:   req.GradeGroup,
		GuardianName: req.GuardianName,
	})
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toStudentResponse(res))
}

// List handles GET /api/v1/students?status=... : lists students, optionally
// filtered by admission status (empty = all).
func (h *StudentsHandler) List(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.ListStudents(r.Context(), r.URL.Query().Get("status"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]studentResponse, 0, len(res))
	for _, s := range res {
		out = append(out, toStudentSummary(s))
	}
	writeJSON(w, http.StatusOK, map[string]any{"students": out})
}

// Get handles GET /api/v1/students/{id}: fetches a single student.
func (h *StudentsHandler) Get(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.GetStudent(r.Context(), r.PathValue("id"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toStudentResponse(res))
}

// --- enrollments ----------------------------------------------------------

// EnrollmentService is the enrollment surface the edge needs (consumer-defined,
// so the handler is testable with a fake). The Connect StudentsClient implements
// it. Enroll takes no cycleID: the students service derives the cycle from the
// group so it can never drift.
type EnrollmentService interface {
	Enroll(ctx context.Context, studentID, groupID, termID string) (client.Enrollment, error)
	EnrollIntoGrade(ctx context.Context, studentID, gradeLevelID string) (client.Placement, error)
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
type enrollIntoGradeRequest struct {
	StudentID    string `json:"studentId"`
	GradeLevelID string `json:"gradeLevelId"`
}

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

// EnrollIntoGrade: POST /api/v1/enrollments/by-grade
//
// The panel asks for a level and a grade — what a school actually knows when a
// child arrives — and the group is chosen by whoever still has a seat. It used
// to ask for "grado y grupo" as free text, so a secretary typed "1° A" and
// nothing checked whether 1°A had room.
func (h *EnrollmentsHandler) EnrollIntoGrade(w http.ResponseWriter, r *http.Request) {
	var body enrollIntoGradeRequest
	if !decodeBody(w, r, &body) {
		return
	}
	placed, err := h.svc.EnrollIntoGrade(r.Context(), body.StudentID, body.GradeLevelID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	// Through the response type, not the client struct: the client's Enrollment
	// carries no JSON tags, so returning it directly serialises Go field names
	// (ID, StudentID) into an API that is camelCase everywhere else.
	writeJSON(w, http.StatusCreated, placementResponse{
		Enrollment: toEnrollmentResponse(placed.Enrollment),
		GroupName:  placed.GroupName,
	})
}

// placementResponse is where a pupil landed. GroupName is the field a person
// reads — "quedó en 1° B" — and the reason the response is not just an enrolment.
type placementResponse struct {
	Enrollment enrollmentResponse `json:"enrollment"`
	GroupName  string             `json:"groupName"`
}
