package handler

import (
	"context"
	"net/http"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// AttendanceService is the daily pass-list surface the edge needs
// (consumer-defined, so the handler is testable with a fake). The Connect
// AttendanceClient implements it.
type AttendanceService interface {
	GetDailyRoster(ctx context.Context, groupID, date string) ([]client.AttendanceRosterRow, error)
	SaveDailyAttendance(ctx context.Context, groupID, date string, marks []client.Mark) (int32, int32, error)
	ListAttendance(ctx context.Context, enrollmentID, from, to string) ([]client.AttendanceRecord, error)
}

// AttendanceHandler exposes the front-facing (HTTP/JSON) attendance endpoints,
// proxying to the attendance service. It translates JSON ↔ the AttendanceClient
// and Connect error codes ↔ HTTP status. Every route must run behind the module
// guard (module_attendance) plus RequireAuth + rotation-enforcement, with the
// write (saving a day) gated on attendance:write and reads on attendance:read.
//
// The model is DEFAULT-PRESENT: saving a day sends only the exceptions and
// reconciles the whole group·day, so an empty marks array legitimately means
// "everybody present" and is never rejected at the edge.
type AttendanceHandler struct {
	svc AttendanceService
}

// NewAttendanceHandler wires the handler to an attendance service.
func NewAttendanceHandler(svc AttendanceService) *AttendanceHandler {
	return &AttendanceHandler{svc: svc}
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

// saveDailyRequest carries the COMPLETE set of exceptions for one group·day:
// every enrollment missing from Marks is reset to present.
type saveDailyRequest struct {
	GroupID string        `json:"groupId"`
	Date    string        `json:"date"`
	Marks   []markRequest `json:"marks"`
}

func toMarks(in []markRequest) []client.Mark {
	out := make([]client.Mark, 0, len(in))
	for _, m := range in {
		out = append(out, client.Mark{EnrollmentID: m.EnrollmentID, Status: m.Status, Note: m.Note})
	}
	return out
}

// --- response bodies -------------------------------------------------------

type attendanceRosterRowResponse struct {
	EnrollmentID string `json:"enrollmentId"`
	StudentID    string `json:"studentId"`
	Status       string `json:"status"`
}

func toAttendanceRosterRowResponse(r client.AttendanceRosterRow) attendanceRosterRowResponse {
	return attendanceRosterRowResponse{
		EnrollmentID: r.EnrollmentID,
		StudentID:    r.StudentID,
		Status:       r.Status,
	}
}

type attendanceRecordResponse struct {
	ID           string `json:"id"`
	EnrollmentID string `json:"enrollmentId"`
	Date         string `json:"date"`
	Status       string `json:"status"`
	Note         string `json:"note"`
}

func toAttendanceRecordResponse(r client.AttendanceRecord) attendanceRecordResponse {
	return attendanceRecordResponse{
		ID:           r.ID,
		EnrollmentID: r.EnrollmentID,
		Date:         r.Date,
		Status:       r.Status,
		Note:         r.Note,
	}
}

// --- endpoints -------------------------------------------------------------

// GetRoster handles GET /api/v1/attendance/roster?groupId=...&date=... — the
// day's pass list, one row per active enrollment. Rows with no stored mark read
// back as "present".
func (h *AttendanceHandler) GetRoster(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := h.svc.GetDailyRoster(r.Context(), q.Get("groupId"), q.Get("date"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]attendanceRosterRowResponse, 0, len(res))
	for _, row := range res {
		out = append(out, toAttendanceRosterRowResponse(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{"roster": out})
}

// SaveDaily handles POST /api/v1/attendance: reconcile one group·day. The
// response is a summary of the stored state, not the records.
func (h *AttendanceHandler) SaveDaily(w http.ResponseWriter, r *http.Request) {
	var req saveDailyRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	absent, marked, err := h.svc.SaveDailyAttendance(r.Context(), req.GroupID, req.Date, toMarks(req.Marks))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"absentCount": absent, "markedCount": marked})
}

// ListAttendance handles GET /api/v1/attendance?enrollmentId=...&from=...&to=...
// — one enrollment's stored exceptions over an inclusive date range.
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
