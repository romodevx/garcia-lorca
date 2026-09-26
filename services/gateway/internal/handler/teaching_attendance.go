package handler

import (
	"context"
	"net/http"
	"sort"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// The teaching-attendance surface is the SELF-SCOPED, teacher-facing daily
// pass-list surface: a maestro reading and reconciling attendance ONLY for the
// groups they teach. It is the attendance twin of the teaching (grades) surface
// in teaching.go and carries the identical safety property.
//
// The caller's scope is their teaching_assignments of the active cycle: the
// DISTINCT groups for a daily list, the (group, subject) pairs for a per-class
// one. That set is
// derived BY CONSTRUCTION from the authenticated principal (the Teacher whose
// user_id == principal.UserID → that teacher's assignments → their groups), so no
// request field (body, query or path) can widen it. This is what makes
// /teaching/attendance/* safe to grant to a non-owner Maestro role via the
// attendance_own:read / attendance_own:write permissions — in contrast to
// attendance:read / attendance:write, which touch ANY group and stay broad staff
// permissions.
//
// A groupId (and subjectId) a roster/save request carries IS caller-supplied,
// so each one first calls assertOwnsList and answers 404 for any list outside
// the caller's own — the same answer a non-existent group gets, so a teacher can
// never probe whether another group or class exists.
//
// T3: a group's LEVEL decides whether its list is daily (any teacher of the
// group takes it) or per class (only the subject's teacher does); a teacher may
// save today and up to attendanceWindowDays days back, never the future.

// --- consumer-defined attendance interface ---------------------------------

// TeachingAttendanceService is the daily pass-list surface the teaching
// attendance handler consumes: the per-group roster read and the per-group·day
// reconcile save. It is deliberately narrower than the admin AttendanceService
// (no per-enrollment history) — a consumer defines only what it uses. The Connect
// AttendanceClient (the same one backing the admin /attendance/* routes)
// satisfies it.
type TeachingAttendanceService interface {
	GetDailyRoster(ctx context.Context, groupID, date, subjectID string) (client.AttendanceRoster, error)
	SaveDailyAttendance(ctx context.Context, in client.SaveAttendanceInput) (int32, int32, error)
}

// TeachingAttendanceHandler is the teacher-facing, self-scoped daily attendance
// surface. It owns no service of its own: it composes the config-derived scoping
// of the teaching handler (caller→teacher→groups, group-name enrichment and
// roster student-name enrichment — all reused through the scope handler) with the
// attendance service behind ONE ownership rule.
type TeachingAttendanceHandler struct {
	scope      *TeachingHandler
	attendance TeachingAttendanceService
	users      AttendanceUserReader
	cards      ScanCardVerifier // T4 scan; nil = the route is not served
	now        func() time.Time
}

// TeachingAttendanceOption configures a TeachingAttendanceHandler (functional
// options).
type TeachingAttendanceOption func(*TeachingAttendanceHandler)

// WithAttendanceClock replaces the wall clock the teacher's date window is
// measured against (tests pin it).
func WithAttendanceClock(now func() time.Time) TeachingAttendanceOption {
	return func(h *TeachingAttendanceHandler) { h.now = now }
}

// NewTeachingAttendanceHandler wires the attendance surface to the teaching scope
// handler (config + students: caller→teacher→assignments, catalog and student
// names), the attendance service (roster + save) and the iam directory (the
// taker's display name).
func NewTeachingAttendanceHandler(scope *TeachingHandler, attendance TeachingAttendanceService, users AttendanceUserReader, opts ...TeachingAttendanceOption) *TeachingAttendanceHandler {
	h := &TeachingAttendanceHandler{scope: scope, attendance: attendance, users: users, now: time.Now}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// --- classes (T3) -------------------------------------------------------------

// attendanceClassResponse is one pass list the caller takes: a DAILY group
// (once, however many subjects the caller teaches there) or a PER_CLASS
// assignment (group + subject). GradeLevelName and GroupName are groupLabel's
// two halves, so a picker can tell two same-named subjects of different grades
// apart ("MATEMÁTICAS · 1° SECUNDARIA" vs "· 2° SECUNDARIA").
type attendanceClassResponse struct {
	GroupID        string `json:"groupId"`
	GroupLabel     string `json:"groupLabel"`
	GroupName      string `json:"groupName"`
	GradeLevelName string `json:"gradeLevelName,omitempty"`
	LevelName      string `json:"levelName"`
	Mode           string `json:"mode"`
	SubjectID      string `json:"subjectId,omitempty"`
	SubjectName    string `json:"subjectName,omitempty"`
	AssignmentID   string `json:"assignmentId,omitempty"`
}

// Classes handles GET /api/v1/teaching/attendance/classes — the lists the CALLER
// takes in the active cycle: one per daily group they teach (deduped) and one
// per per-class assignment. Derived from the principal (callerAssignments), so
// no request field can widen it; a principal with no linked teacher gets an
// empty list. The catalog read is a hard dependency — a group's mode is never
// guessed. Sorted by group label, then subject name.
//
// It runs behind RequireAuth + rotation, the module_attendance + module_config
// guards and RequirePermission("attendance_own:read").
func (h *TeachingAttendanceHandler) Classes(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	_, assignments, err := h.scope.callerAssignments(r.Context(), p)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]attendanceClassResponse, 0, len(assignments))
	if len(assignments) > 0 {
		catalog, err := loadAttendanceCatalog(r.Context(), h.scope.config)
		if err != nil {
			writeConnectError(w, err)
			return
		}
		out = classesFor(assignments, catalog)
	}
	writeJSON(w, http.StatusOK, map[string]any{"classes": out})
}

// classesFor projects the caller's assignments onto their pass lists.
func classesFor(assignments []client.TeachingAssignment, catalog attendanceCatalog) []attendanceClassResponse {
	out := make([]attendanceClassResponse, 0, len(assignments))
	seenDaily := make(map[string]bool, len(assignments))
	for _, a := range assignments {
		group, ok := catalog.group(a.GroupID)
		if !ok {
			continue // a group the catalog no longer holds takes no list
		}
		class := attendanceClassResponse{
			GroupID: a.GroupID, GroupLabel: group.Label, GroupName: group.Name, GradeLevelName: group.GradeName,
			LevelName: group.LevelName, Mode: group.Mode,
		}
		if group.Mode == attendanceModeDaily {
			if seenDaily[a.GroupID] {
				continue
			}
			seenDaily[a.GroupID] = true
		} else {
			class.SubjectID, class.SubjectName, class.AssignmentID = a.SubjectID, catalog.subjects[a.SubjectID], a.ID
		}
		out = append(out, class)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].GroupLabel != out[j].GroupLabel {
			return out[i].GroupLabel < out[j].GroupLabel
		}
		return out[i].SubjectName < out[j].SubjectName
	})
	return out
}

// --- list ownership (T3) ---------------------------------------------------------

// assertOwnsList is the ownership gate of the roster and save routes. It writes
// the refusal itself and returns the list's access (mode, and the roster when it
// had to read it) and true only when the caller may touch the list:
//
//   - 404 when the caller teaches nothing in the group (active cycle) — the same
//     answer as a group that does not exist, so nothing leaks;
//   - 404 on a per-class key whose (group, subject) assignment is not the
//     caller's — another teacher's class in the same group is invisible too;
//   - 400 when the subject does not fit the group's mode, unless that exact
//     list already exists (listExists): the mode governs NEW lists only, so a
//     list taken before the level changed mode stays readable and correctable,
//     answered in the key's own shape.
//
// Ownership follows the key: a daily key needs any assignment in the group, a
// per-class key the assignment of that subject. The caller's assignments come
// from the PRINCIPAL, never the request. They and the group's chain (group →
// grade → level) are independent reads, so they run concurrently; ownership is
// judged first, so a group the caller does not teach is 404 even when its
// chain failed to load.
func (h *TeachingAttendanceHandler) assertOwnsList(w http.ResponseWriter, r *http.Request, p middleware.Principal, groupID, date, subjectID string) (listAccess, bool) {
	ctx := r.Context()
	var (
		assignments       []client.TeachingAssignment
		group             listGroup
		found             bool
		scopeErr, listErr error
		reads             errgroup.Group
	)
	reads.Go(func() error {
		_, assignments, scopeErr = h.scope.callerAssignments(ctx, p)
		return nil
	})
	reads.Go(func() error {
		group, found, listErr = resolveListGroup(ctx, h.scope.config, groupID)
		return nil
	})
	_ = reads.Wait() // both closures record their own error and return nil
	if scopeErr != nil {
		writeConnectError(w, scopeErr)
		return listAccess{}, false
	}
	if !ownsKey(assignments, groupID, subjectID) {
		writeNotFound(w)
		return listAccess{}, false
	}
	if listErr != nil {
		writeConnectError(w, listErr)
		return listAccess{}, false
	}
	if !found {
		writeNotFound(w)
		return listAccess{}, false
	}
	msg := listModeRefusal(group.Mode, subjectID)
	if msg == "" {
		return listAccess{mode: group.Mode}, true
	}
	roster, exists, err := listExists(ctx, h.attendance, groupID, date, subjectID)
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

// ownsKey reports whether assignments cover the list key: any assignment in
// the group for a daily key (no subject), the (group, subject) assignment for a
// per-class one.
func ownsKey(assignments []client.TeachingAssignment, groupID, subjectID string) bool {
	if groupID == "" {
		return false
	}
	for _, a := range assignments {
		if a.GroupID == groupID && (subjectID == "" || a.SubjectID == subjectID) {
			return true
		}
	}
	return false
}

// --- roster ----------------------------------------------------------------

// Roster handles GET /api/v1/teaching/attendance/roster?groupId=&date=[&subjectId=]
// — one of the caller's OWN lists (see assertOwnsList), each row enriched with
// the student's name. Any date may be READ; editable says whether the caller may
// still save it (today back to attendanceWindowDays days, never the future),
// readOnlyReason why not ("future" | "window"), and today is the server's
// school day.
//
// It runs behind RequireAuth + rotation, the module_attendance guard and
// RequirePermission("attendance_own:read").
func (h *TeachingAttendanceHandler) Roster(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	q := r.URL.Query()
	groupID, dateStr, subjectID := q.Get("groupId"), q.Get("date"), q.Get("subjectId")
	access, ok := h.assertOwnsList(w, r, p, groupID, dateStr, subjectID)
	if !ok {
		return
	}
	date, ok := parseListDate(dateStr)
	if !ok {
		writeBadRequest(w, msgInvalidDate)
		return
	}
	roster, err := rosterOf(r.Context(), h.attendance, access, groupID, dateStr, subjectID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	names := h.scope.studentNames(r.Context(), len(roster.Rows) > 0)
	now := h.now()
	view := listView{Mode: access.mode, Date: dateStr, ReadOnlyReason: teacherReadOnlyReason(date, now), Today: schoolToday(now)}
	writeJSON(w, http.StatusOK, toAttendanceListResponse(view, roster, names))
}

// --- save ------------------------------------------------------------------

// SaveAttendance handles POST /api/v1/teaching/attendance — reconcile one of the
// caller's OWN lists (see assertOwnsList) on a day inside the window (today back
// to attendanceWindowDays days; never the future), stamping the caller as its
// taker. The model is DEFAULT-PRESENT: an empty marks array legitimately means
// "everybody present" and is never rejected.
//
// It runs behind RequireAuth + rotation, the module_attendance guard and
// RequirePermission("attendance_own:write").
func (h *TeachingAttendanceHandler) SaveAttendance(w http.ResponseWriter, r *http.Request) {
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
	if _, ok := h.assertOwnsList(w, r, p, req.GroupID, req.Date, req.SubjectID); !ok {
		return
	}
	date, ok := parseListDate(req.Date)
	if !ok {
		writeBadRequest(w, msgInvalidDate)
		return
	}
	if msg := teacherWindowRefusal(date, h.now()); msg != "" {
		writeBadRequest(w, msg)
		return
	}
	absent, marked, err := h.attendance.SaveDailyAttendance(r.Context(), client.SaveAttendanceInput{
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
