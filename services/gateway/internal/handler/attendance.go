package handler

import (
	"context"
	"net/http"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// AttendanceService is the pass-list surface the edge needs (consumer-defined,
// so the handler is testable with a fake). The Connect AttendanceClient
// implements it.
type AttendanceService interface {
	GetDailyRoster(ctx context.Context, groupID, date, subjectID string) (client.AttendanceRoster, error)
	SaveDailyAttendance(ctx context.Context, in client.SaveAttendanceInput) (int32, int32, error)
	ListAttendance(ctx context.Context, enrollmentID, from, to string) ([]client.AttendanceRecord, error)
}

// DirectorAttendanceCatalogReader is the config surface the director's pass
// list needs: the group → level chain (mode + label) and the group's teaching
// assignments, which say which subjects a per-class list may be taken for.
type DirectorAttendanceCatalogReader interface {
	AttendanceCatalogReader
	ListTeachingAssignments(ctx context.Context, cycleID, groupID, teacherID string) ([]client.TeachingAssignment, error)
}

// AttendanceHandler exposes the director-facing (HTTP/JSON) attendance
// endpoints, proxying to the attendance service. Every route runs behind the
// module guard (module_attendance) plus RequireAuth + rotation-enforcement, with
// saving gated on attendance:write and reads on attendance:read.
//
// The director reads and corrects ANY group on ANY date — no window — but the
// level's mode still applies: a daily list takes no subject, a per-class list
// needs a subject taught in that group.
//
// The model is DEFAULT-PRESENT: saving a list sends only the exceptions and
// reconciles the whole list, so an empty marks array legitimately means
// "everybody present" and is never rejected at the edge.
type AttendanceHandler struct {
	svc      AttendanceService
	catalog  DirectorAttendanceCatalogReader
	students TeachingStudentReader
	users    AttendanceUserReader
}

// NewAttendanceHandler wires the handler to the attendance service, the config
// catalog (mode + subjects), the students directory (row names) and the iam
// directory (the taker's display name).
func NewAttendanceHandler(svc AttendanceService, catalog DirectorAttendanceCatalogReader, students TeachingStudentReader, users AttendanceUserReader) *AttendanceHandler {
	return &AttendanceHandler{svc: svc, catalog: catalog, students: students, users: users}
}

// --- request bodies --------------------------------------------------------

// markRequest is one exception. Status is normally absent|late|excused;
// "present" CLEARS the row stored for that enrollment (an explicit revert to
// default-present, equivalent to omitting it).
type markRequest struct {
	EnrollmentID string `json:"enrollmentId"`
	Status       string `json:"status"`
	Note         string `json:"note"`
}

// saveDailyRequest carries the COMPLETE set of exceptions for one list: every
// enrollment missing from Marks is reset to present. SubjectID names the subject
// of a per-class list and is empty for a daily one.
type saveDailyRequest struct {
	GroupID   string        `json:"groupId"`
	Date      string        `json:"date"`
	SubjectID string        `json:"subjectId"`
	Marks     []markRequest `json:"marks"`
}

func toMarks(in []markRequest) []client.Mark {
	out := make([]client.Mark, 0, len(in))
	for _, m := range in {
		out = append(out, client.Mark{EnrollmentID: m.EnrollmentID, Status: m.Status, Note: m.Note})
	}
	return out
}

// --- response bodies -------------------------------------------------------

type attendanceRecordResponse struct {
	ID           string `json:"id"`
	EnrollmentID string `json:"enrollmentId"`
	Date         string `json:"date"`
	Status       string `json:"status"`
	Note         string `json:"note"`
	// SubjectID is the per-class subject; empty for a daily mark.
	SubjectID string `json:"subjectId"`
}

func toAttendanceRecordResponse(r client.AttendanceRecord) attendanceRecordResponse {
	return attendanceRecordResponse{
		ID:           r.ID,
		EnrollmentID: r.EnrollmentID,
		Date:         r.Date,
		Status:       r.Status,
		Note:         r.Note,
		SubjectID:    r.SubjectID,
	}
}

// --- list resolution ----------------------------------------------------------

// resolveList checks a (group, subject) pair against the catalog and writes the
// refusal itself: 404 for an unknown group, 400 for a subject that does not fit
// the group's mode or is not taught in the group. It returns the group's mode
// and true when the list exists.
func (h *AttendanceHandler) resolveList(w http.ResponseWriter, r *http.Request, groupID, subjectID string) (string, bool) {
	catalog, err := loadAttendanceCatalog(r.Context(), h.catalog)
	if err != nil {
		writeConnectError(w, err)
		return "", false
	}
	group, ok := catalog.group(groupID)
	if !ok {
		writeNotFound(w)
		return "", false
	}
	if msg := listModeRefusal(group.Mode, subjectID); msg != "" {
		writeBadRequest(w, msg)
		return "", false
	}
	if subjectID == "" {
		return group.Mode, true
	}
	taught, err := h.subjectTaughtIn(r.Context(), groupID, subjectID)
	if err != nil {
		writeConnectError(w, err)
		return "", false
	}
	if !taught {
		writeBadRequest(w, msgSubjectNotInGroup)
		return "", false
	}
	return group.Mode, true
}

// subjectTaughtIn reports whether any teaching assignment binds subjectID to
// groupID.
func (h *AttendanceHandler) subjectTaughtIn(ctx context.Context, groupID, subjectID string) (bool, error) {
	assignments, err := h.catalog.ListTeachingAssignments(ctx, "", groupID, "")
	if err != nil {
		return false, err
	}
	for _, a := range assignments {
		if a.GroupID == groupID && a.SubjectID == subjectID {
			return true, nil
		}
	}
	return false, nil
}

// --- endpoints -------------------------------------------------------------

// GetRoster handles GET /api/v1/attendance/roster?groupId=&date=[&subjectId=] —
// one list, one row per active enrollment (rows with no stored mark read back
// "present"), with the students' names and the list's session. The director has
// no date window, so every list is editable.
func (h *AttendanceHandler) GetRoster(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	groupID, date, subjectID := q.Get("groupId"), q.Get("date"), q.Get("subjectId")
	mode, ok := h.resolveList(w, r, groupID, subjectID)
	if !ok {
		return
	}
	roster, err := h.svc.GetDailyRoster(r.Context(), groupID, date, subjectID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	names := studentNamesFrom(r.Context(), h.students, len(roster.Rows) > 0)
	writeJSON(w, http.StatusOK, toAttendanceListResponse(mode, date, true, roster, names))
}

// SaveDaily handles POST /api/v1/attendance: reconcile one list, stamping the
// caller as its taker. The response is a summary of the stored state.
func (h *AttendanceHandler) SaveDaily(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req saveDailyRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if _, ok := h.resolveList(w, r, req.GroupID, req.SubjectID); !ok {
		return
	}
	absent, marked, err := h.svc.SaveDailyAttendance(r.Context(), client.SaveAttendanceInput{
		GroupID:     req.GroupID,
		Date:        req.Date,
		SubjectID:   req.SubjectID,
		Marks:       toMarks(req.Marks),
		TakenBy:     p.UserID,
		TakenByName: takerName(r.Context(), h.users, p),
	})
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"absentCount": absent, "markedCount": marked})
}

// ListAttendance handles GET /api/v1/attendance?enrollmentId=...&from=...&to=...
// — one enrollment's stored exceptions over an inclusive date range, each with
// its per-class subject (empty for a daily mark).
func (h *AttendanceHandler) ListAttendance(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := h.svc.ListAttendance(r.Context(), q.Get("enrollmentId"), q.Get("from"), q.Get("to"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]attendanceRecordResponse, 0, len(res))
	for _, rec := range res {
		out = append(out, toAttendanceRecordResponse(rec))
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": out})
}
