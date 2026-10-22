package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// The reads and writes the virtual card needs (task 13), each defined here.
type (
	CardStudents interface {
		GetStudent(ctx context.Context, id string) (client.StudentResult, error)
		CardCode(ctx context.Context, studentID string) (code string, suspended bool, err error)
		RevokeCard(ctx context.Context, studentID string) error
		ReissueCard(ctx context.Context, studentID string) error
		VerifyCard(ctx context.Context, code string) (client.StudentResult, bool, error)
	}
	StudentPhotoReader interface {
		GetStudentPhoto(ctx context.Context, studentID string) ([]byte, string, error)
	}
	SchoolProfileReader interface {
		GetSchoolProfile(ctx context.Context) (client.SchoolProfile, bool, error)
	}
	CycleLister interface {
		ListCycles(ctx context.Context) ([]client.CycleResult, error)
	}
)

// CardHandler serves a pupil's virtual card (task 13): the family reads its own
// child's card and photo; the staff verify a scanned code and revoke a card.
// Every route runs behind module_student_card (cmd/main.go).
type CardHandler struct {
	links    GuardianLinkReader
	students CardStudents
	photos   StudentPhotoReader
	profiles SchoolProfileReader
	cycles   CycleLister
	// review is module_import_review: a draft's status is named only with it.
	review importReview
}

// CardOption configures optional behavior of the card endpoints.
type CardOption func(*CardHandler)

// WithCardImportReview names a draft pupil's status on the card check only
// when module_import_review is on (PLAN-import-avisos); unwired is off.
func WithCardImportReview(enabled func(context.Context) bool) CardOption {
	return func(h *CardHandler) { h.review = enabled }
}

// NewCardHandler wires the handler.
func NewCardHandler(links GuardianLinkReader, students CardStudents, photos StudentPhotoReader, profiles SchoolProfileReader, cycles CycleLister, opts ...CardOption) *CardHandler {
	h := &CardHandler{links: links, students: students, photos: photos, profiles: profiles, cycles: cycles}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// cardResponse is the card as the parents' app draws it. The QR carries only
// the opaque code; everything a person reads is beside it.
type cardResponse struct {
	StudentID        string     `json:"studentId"`
	Name             string     `json:"name"`
	Level            string     `json:"level"`
	GradeGroup       string     `json:"gradeGroup"`
	Cycle            string     `json:"cycle"`
	EnrollmentNumber string     `json:"enrollmentNumber"`
	School           cardSchool `json:"school"`
	// QR is empty while the office has the card suspended.
	QR        string `json:"qr"`
	Suspended bool   `json:"suspended"`
}

// cardSchool is what the card prints about the school. The RFC and legal name
// stay on receipts: a school registered under a person's RFC would otherwise
// hand every family that person's name and birthdate. So does the legend, the
// receipt's footer ("Gracias por su pago.").
type cardSchool struct {
	Name    string `json:"name"`
	CCT     string `json:"cct"`
	Address string `json:"address"`
	Phone   string `json:"phone"`
}

func toCardSchool(p client.SchoolProfile) cardSchool {
	return cardSchool{Name: p.Name, CCT: p.CCT, Address: p.Address, Phone: p.Phone}
}

// FamilyCard handles GET /api/v1/family/students/{studentId}/card. Another
// family's child is 404: nothing says whether that child exists.
func (h *CardHandler) FamilyCard(w http.ResponseWriter, r *http.Request) {
	studentID, ok := h.ownChild(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	pupil, err := h.students.GetStudent(ctx, studentID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	code, suspended, err := h.students.CardCode(ctx, studentID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	school, _, err := h.profiles.GetSchoolProfile(ctx)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	cycle, err := h.activeCycle(ctx)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	// The code identifies a child: no cache keeps it.
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, cardResponse{
		StudentID: pupil.ID, Name: fullName(pupil), Level: pupil.Level, GradeGroup: pupil.GradeGroup,
		Cycle: cycle, EnrollmentNumber: pupil.EnrollmentNumber, School: toCardSchool(school), QR: code, Suspended: suspended,
	})
}

// FamilyPhoto handles GET /api/v1/family/students/{studentId}/photo, with the
// same ownership check as the card.
func (h *CardHandler) FamilyPhoto(w http.ResponseWriter, r *http.Request) {
	studentID, ok := h.ownChild(w, r)
	if !ok {
		return
	}
	data, contentType, err := h.photos.GetStudentPhoto(r.Context(), studentID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writePhoto(w, data, contentType)
}

type verifyCardRequest struct {
	Code string `json:"code"`
}

// verifiedCardResponse is what the staff see after a scan.
type verifiedCardResponse struct {
	Valid            bool   `json:"valid"`
	StudentID        string `json:"studentId"`
	Name             string `json:"name"`
	Level            string `json:"level"`
	GradeGroup       string `json:"gradeGroup"`
	EnrollmentNumber string `json:"enrollmentNumber"`
	AdmissionStatus  string `json:"admissionStatus"`
}

// Verify handles POST /api/v1/students/card/verify (students:read). The code
// travels in the body, never the URL, so no log line keeps it. An unknown code
// is 404; something that is not a card code, 400.
func (h *CardHandler) Verify(w http.ResponseWriter, r *http.Request) {
	var req verifyCardRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	pupil, valid, err := h.students.VerifyCard(r.Context(), req.Code)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, verifiedCardResponse{
		Valid: valid, StudentID: pupil.ID, Name: fullName(pupil), Level: pupil.Level, GradeGroup: pupil.GradeGroup,
		EnrollmentNumber: pupil.EnrollmentNumber, AdmissionStatus: h.review.visibleAdmission(r.Context(), pupil.AdmissionStatus),
	})
}

// Revoke handles POST /api/v1/students/{id}/card/revoke (students:update): the
// old code stops counting and the card is suspended until reissued (the
// owner's decision: a stolen phone keeps its session, and would otherwise get a
// valid new code).
func (h *CardHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	h.change(w, r, "student card revoked", h.students.RevokeCard)
}

// Reissue handles POST /api/v1/students/{id}/card/reissue (students:update):
// the office gives a suspended pupil a new card once it trusts the phone.
func (h *CardHandler) Reissue(w http.ResponseWriter, r *http.Request) {
	h.change(w, r, "student card reissued", h.students.ReissueCard)
}

// change runs a card change and leaves a record of who made it: a revoked or
// reissued card is a security decision somebody may need to trace.
func (h *CardHandler) change(w http.ResponseWriter, r *http.Request, event string, apply func(context.Context, string) error) {
	studentID := r.PathValue("id")
	if err := apply(r.Context(), studentID); err != nil {
		writeConnectError(w, err)
		return
	}
	actor := ""
	if p, ok := middleware.PrincipalFrom(r.Context()); ok {
		actor = p.UserID
	}
	slog.InfoContext(r.Context(), event, "student_id", studentID, "by_user_id", actor)
	w.WriteHeader(http.StatusNoContent)
}

// ownChild answers 401/404 itself and reports whether the caller may go on.
func (h *CardHandler) ownChild(w http.ResponseWriter, r *http.Request) (string, bool) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", false
	}
	studentID := r.PathValue("studentId")
	owned, err := ownsChild(r.Context(), h.links, p.UserID, studentID)
	if err != nil {
		writeConnectError(w, err)
		return "", false
	}
	if !owned {
		writeNotFound(w)
		return "", false
	}
	return studentID, true
}

// activeCycle names the school's active cycle, or "" when none is.
func (h *CardHandler) activeCycle(ctx context.Context) (string, error) {
	cycles, err := h.cycles.ListCycles(ctx)
	if err != nil {
		return "", err
	}
	for _, c := range cycles {
		if c.Status == "active" {
			return c.Name, nil
		}
	}
	return "", nil
}

func fullName(s client.StudentResult) string { return s.FullName() }
