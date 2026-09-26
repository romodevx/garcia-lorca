package handler

import (
	"context"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"

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
// list needs: the group → grade → level chain (mode) and the group's teaching
// assignments, which say which subjects a per-class list may be taken for.
type DirectorAttendanceCatalogReader interface {
	AttendanceGroupReader
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
	cards    ScanCardVerifier // T4 scan; nil = the route is not served
	now      func() time.Time
}

// AttendanceHandlerOption configures an AttendanceHandler (functional options).
type AttendanceHandlerOption func(*AttendanceHandler)

// WithDirectorAttendanceClock replaces the wall clock the roster's `today` is
// read from (tests pin it).
func WithDirectorAttendanceClock(now func() time.Time) AttendanceHandlerOption {
	return func(h *AttendanceHandler) { h.now = now }
}

// NewAttendanceHandler wires the handler to the attendance service, the config
// catalog (mode + subjects), the students directory (row names) and the iam
// directory (the taker's display name).
func NewAttendanceHandler(svc AttendanceService, catalog DirectorAttendanceCatalogReader, students TeachingStudentReader, users AttendanceUserReader, opts ...AttendanceHandlerOption) *AttendanceHandler {
	h := &AttendanceHandler{svc: svc, catalog: catalog, students: students, users: users, now: time.Now}
	for _, opt := range opts {
		opt(h)
	}
	return h
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

// resolveList checks a list key against the catalog and writes the refusal
// itself: 404 for an unknown group, 400 for a subject that does not fit the
// group's mode or is not taught in the group. It returns the list's access
// (mode, and the roster when it had to read it) and true when the list may be
// read and saved.
//
// The group's chain (group → grade → level) and, for a per-class key, the
// group's assignments are independent reads, so they run concurrently.
//
// The level's mode governs NEW lists only: a key that does not fit it is still
// served when that exact list already exists (listExists) — it was taken before
// the level changed mode — and answers in the key's own shape.
func (h *AttendanceHandler) resolveList(w http.ResponseWriter, r *http.Request, groupID, date, subjectID string) (listAccess, bool) {
	var (
		group  listGroup
		found  bool
		taught bool
	)
	g, gctx := errgroup.WithContext(r.Context())
	g.Go(func() (err error) {
		group, found, err = resolveListGroup(gctx, h.catalog, groupID)
		return err
	})
	if subjectID != "" {
		g.Go(func() (err error) {
			taught, err = h.subjectTaughtIn(gctx, groupID, subjectID)
			return err
		})
	}
	if err := g.Wait(); err != nil {
		writeConnectError(w, err)
		return listAccess{}, false
	}
	if !found {
		writeNotFound(w)
		return listAccess{}, false
	}
	if msg := listModeRefusal(group.Mode, subjectID); msg != "" {
		roster, exists, err := listExists(r.Context(), h.svc, groupID, date, subjectID)
		if err != nil {
			writeConnectError(w, err)
			return listAccess{}, false
		}
		if !exists {
			writeBadRequest(w, msg)
			return listAccess{}, false
		}
		return listAccess{mode: keyMode(subjectID), roster: roster}, true
	}
	if subjectID != "" && !taught {
		writeBadRequest(w, msgSubjectNotInGroup)
		return listAccess{}, false
	}
	return listAccess{mode: group.Mode}, true
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
// no date window, so every list is editable (no readOnlyReason); `today` is
// the server's school day.
func (h *AttendanceHandler) GetRoster(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	groupID, date, subjectID := q.Get("groupId"), q.Get("date"), q.Get("subjectId")
	access, ok := h.resolveList(w, r, groupID, date, subjectID)
	if !ok {
		return
	}
	roster, err := rosterOf(r.Context(), h.svc, access, groupID, date, subjectID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	names := studentNamesFrom(r.Context(), h.students, len(roster.Rows) > 0)
	view := listView{Mode: access.mode, Date: date, Today: schoolToday(h.now())}
	writeJSON(w, http.StatusOK, toAttendanceListResponse(view, roster, names))
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
	if _, ok := h.resolveList(w, r, req.GroupID, req.Date, req.SubjectID); !ok {
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
