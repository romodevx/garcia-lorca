package handler

import (
	"context"
	"net/http"
	"os"
	"time"
	"unicode/utf8"

	"connectrpc.com/connect"
	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
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
	CURP      string `json:"curp"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"` // the paternal surname
	// MaternalLastName is optional (T3).
	MaternalLastName string `json:"maternalLastName"`
	LevelID          string `json:"levelId"`
	GradeGroup       string `json:"gradeGroup"`
	GuardianName     string `json:"guardianName"`
	// BirthDate is optional at intake (T11).
	BirthDate string `json:"birthDate"`
}

type studentResponse struct {
	ID        string `json:"id"`
	CURP      string `json:"curp"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"` // the paternal surname
	// MaternalLastName is empty when not captured (T3).
	MaternalLastName string `json:"maternalLastName"`
	Level            string `json:"level"`   // the school's name for the level
	LevelID          string `json:"levelId"` // the config level id
	GradeGroup       string `json:"gradeGroup"`
	GuardianName     string `json:"guardianName"`
	AdmissionStatus  string `json:"admissionStatus"`
	// EnrollmentNumber is the matrícula (task 13).
	EnrollmentNumber string `json:"enrollmentNumber,omitempty"`
	// BankReference is the BBVA payment reference (ADR 0033); "" when none.
	BankReference string `json:"bankReference"`
	// Nil in lists: a picker needs names, not every pupil's address and phones.
	Profile *client.StudentProfile `json:"profile,omitempty"`
}

func toStudentResponse(s client.StudentResult) studentResponse {
	return studentResponse{
		ID:               s.ID,
		CURP:             s.CURP,
		FirstName:        s.FirstName,
		LastName:         s.LastName,
		MaternalLastName: s.MaternalLastName,
		Level:            s.Level,
		LevelID:          s.LevelID,
		GradeGroup:       s.GradeGroup,
		GuardianName:     s.GuardianName,
		AdmissionStatus:  s.AdmissionStatus,
		EnrollmentNumber: s.EnrollmentNumber,
		BankReference:    s.BankReference,
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
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	// MaternalLastName is a pointer: absent (a client that predates T3) keeps
	// the surname on file instead of erasing it.
	MaternalLastName *string               `json:"maternalLastName"`
	CURP             string                `json:"curp"`
	Profile          client.StudentProfile `json:"profile"`
	// BankReference is a pointer: absent keeps the stored reference.
	BankReference *string `json:"bankReference"`
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
	maternal, err := h.maternalOrStored(r.Context(), r.PathValue("id"), req.MaternalLastName)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	res, err := h.svc.UpdateStudent(r.Context(), client.UpdateStudentInput{
		ID: r.PathValue("id"), FirstName: req.FirstName, LastName: req.LastName,
		MaternalLastName: maternal, CURP: req.CURP, Profile: req.Profile,
		BankReference: req.BankReference,
	})
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toStudentResponse(res))
}

// maternalOrStored is the maternal surname an update sends, or — when the
// request does not carry the field at all — the one already on file.
func (h *StudentsHandler) maternalOrStored(ctx context.Context, id string, sent *string) (string, error) {
	if sent != nil {
		return *sent, nil
	}
	stored, err := h.svc.GetStudent(ctx, id)
	if err != nil {
		return "", err
	}
	return stored.MaternalLastName, nil
}

// Create handles POST /api/v1/students: registers a new applicant (pending).
func (h *StudentsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createStudentRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	res, err := h.svc.CreateApplicant(r.Context(), client.CreateApplicantInput{
		CURP:             req.CURP,
		FirstName:        req.FirstName,
		LastName:         req.LastName,
		MaternalLastName: req.MaternalLastName,
		LevelID:          req.LevelID,
		GradeGroup:       req.GradeGroup,
		GuardianName:     req.GuardianName,
		BirthDate:        req.BirthDate,
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
	WithdrawEnrollment(ctx context.Context, id, withdrawnOn, reason string) (client.Enrollment, error)
}

type WithdrawalPreviewService interface {
	PreviewWithdrawal(ctx context.Context, studentID, cycleID, withdrawnOn string) (client.WithdrawalPreview, error)
}

// EnrollmentsHandler exposes the front-facing (HTTP/JSON) enrollment endpoints,
// proxying to the students service. It translates JSON ↔ the StudentsClient and
// Connect error codes ↔ HTTP status. Every route must run behind the module
// guard (module_students) plus RequireAuth + rotation-enforcement, with writes
// (enroll/transfer/withdraw) gated on students:enroll and reads on students:read.
type EnrollmentsHandler struct {
	svc     EnrollmentService
	finance WithdrawalPreviewService
	now     func() time.Time
}

// EnrollmentsOption configures optional cross-module enrollment behavior.
type EnrollmentsOption func(*EnrollmentsHandler)

// WithWithdrawalPreview wires finance's optional withdrawal preview.
func WithWithdrawalPreview(finance WithdrawalPreviewService) EnrollmentsOption {
	return func(h *EnrollmentsHandler) { h.finance = finance }
}

// WithEnrollmentsClock replaces the clock used for withdrawal date checks.
func WithEnrollmentsClock(now func() time.Time) EnrollmentsOption {
	return func(h *EnrollmentsHandler) { h.now = now }
}

// NewEnrollmentsHandler wires the handler to its clients.
func NewEnrollmentsHandler(svc EnrollmentService, opts ...EnrollmentsOption) *EnrollmentsHandler {
	h := &EnrollmentsHandler{svc: svc, now: time.Now}
	for _, opt := range opts {
		opt(h)
	}
	return h
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
	WithdrawnOn  string `json:"withdrawnOn"`
	Reason       string `json:"reason"`
}

type enrollmentResponse struct {
	ID               string `json:"id"`
	StudentID        string `json:"studentId"`
	CycleID          string `json:"cycleId"`
	GroupID          string `json:"groupId"`
	TermID           string `json:"termId"`
	Status           string `json:"status"`
	EnrolledAt       string `json:"enrolledAt"`
	WithdrawnOn      string `json:"withdrawnOn,omitempty"`
	WithdrawalReason string `json:"withdrawalReason,omitempty"`
}

func toEnrollmentResponse(e client.Enrollment) enrollmentResponse {
	enrolledAt := ""
	if !e.EnrolledAt.IsZero() {
		enrolledAt = e.EnrolledAt.UTC().Format(time.RFC3339)
	}
	return enrollmentResponse{
		ID:               e.ID,
		StudentID:        e.StudentID,
		CycleID:          e.CycleID,
		GroupID:          e.GroupID,
		TermID:           e.TermID,
		Status:           e.Status,
		EnrolledAt:       enrolledAt,
		WithdrawnOn:      e.WithdrawnOn,
		WithdrawalReason: e.WithdrawalReason,
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
	if !validOptionalISODate(req.WithdrawnOn) {
		http.Error(w, "withdrawnOn must be a valid YYYY-MM-DD date.", http.StatusBadRequest)
		return
	}
	today := schoolDate(h.now()).Format("2006-01-02")
	withdrawalDate := req.WithdrawnOn
	if withdrawalDate == "" {
		withdrawalDate = today
	}
	if withdrawalDate < today {
		principal, ok := middleware.PrincipalFrom(r.Context())
		if !ok {
			http.Error(w, "Se requiere autenticación.", http.StatusUnauthorized)
			return
		}
		if !middleware.HasPermission(principal, "finance:write") {
			http.Error(w, "Para una baja con fecha anterior a hoy se necesita permiso de finanzas", http.StatusForbidden)
			return
		}
	}
	if utf8.RuneCountInString(req.Reason) > 500 {
		http.Error(w, "reason must be at most 500 characters.", http.StatusBadRequest)
		return
	}
	res, err := h.svc.WithdrawEnrollment(r.Context(), req.EnrollmentID, withdrawalDate, req.Reason)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toEnrollmentResponse(res))
}

// PreviewWithdrawal handles GET /api/v1/enrollments/{id}/withdrawal-preview.
func (h *EnrollmentsHandler) PreviewWithdrawal(w http.ResponseWriter, r *http.Request) {
	withdrawnOn := r.URL.Query().Get("withdrawnOn")
	if withdrawnOn == "" {
		withdrawnOn = schoolDate(h.now()).Format("2006-01-02")
	}
	if !validOptionalISODate(withdrawnOn) {
		http.Error(w, "withdrawnOn must be a valid YYYY-MM-DD date.", http.StatusBadRequest)
		return
	}
	enrollment, err := h.svc.GetEnrollment(r.Context(), r.PathValue("id"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if enrollment.Status != "active" {
		http.Error(w, "Only an active enrollment can be previewed for withdrawal.", http.StatusConflict)
		return
	}
	if h.finance == nil {
		WriteConnectError(w, connect.NewError(connect.CodeUnavailable, nil))
		return
	}
	preview, err := h.finance.PreviewWithdrawal(r.Context(), enrollment.StudentID, enrollment.CycleID, withdrawnOn)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if preview.ToCancel == nil {
		preview.ToCancel = []client.WithdrawalCharge{}
	}
	charges := make([]withdrawalChargeResponse, 0, len(preview.ToCancel))
	for _, charge := range preview.ToCancel {
		charges = append(charges, withdrawalChargeResponse{ID: charge.ID, Description: charge.Description, DueDate: charge.DueDate, AmountCents: charge.AmountCents})
	}
	writeJSON(w, http.StatusOK, withdrawalPreviewResponse{ToCancel: charges, ToCancelCents: preview.ToCancelCents, RemainingDebtCents: preview.RemainingDebtCents})
}

type withdrawalChargeResponse struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	DueDate     string `json:"dueDate"`
	AmountCents int64  `json:"amountCents"`
}

type withdrawalPreviewResponse struct {
	ToCancel           []withdrawalChargeResponse `json:"toCancel"`
	ToCancelCents      int64                      `json:"toCancelCents"`
	RemainingDebtCents int64                      `json:"remainingDebtCents"`
}

func validOptionalISODate(value string) bool {
	if value == "" {
		return true
	}
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

const defaultSchoolTimezone = "America/Mexico_City"

func schoolDate(now time.Time) time.Time {
	name := os.Getenv("SCHOOL_TIMEZONE")
	if name == "" {
		name = defaultSchoolTimezone
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		location, _ = time.LoadLocation(defaultSchoolTimezone)
	}
	return now.In(location)
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
