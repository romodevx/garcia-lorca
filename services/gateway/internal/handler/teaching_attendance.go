package handler

import (
	"context"
	"net/http"
	"sort"
	"time"

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

// --- scoping core (methods on the scope handler) ---------------------------

// callerGroups resolves the DISTINCT groups the PRINCIPAL teaches. Attendance is
// per group, so a teacher assigned the same group for two subjects supervises
// that one group's pass list once. The teacher id and the group set come only
// from the principal (via callerAssignments), so no request field can widen them.
// A principal with no linked teacher record teaches no groups — teacherID "" and
// an empty set, never an error and never a downstream read.
func (h *TeachingHandler) callerGroups(ctx context.Context, principal middleware.Principal) (teacherID string, groups map[string]struct{}, err error) {
	teacherID, assignments, err := h.callerAssignments(ctx, principal)
	if err != nil {
		return "", nil, err
	}
	groups = make(map[string]struct{}, len(assignments))
	for _, a := range assignments {
		if a.GroupID != "" {
			groups[a.GroupID] = struct{}{}
		}
	}
	return teacherID, groups, nil
}

// groupCatalogNames builds the group id→name and group id→grade-name maps used
// to enrich the caller's groups. It is best-effort by design: no groups to
// enrich or a failed lookup each yields an empty map, and the caller reads a
// missing id as "" — a display gap, never a failed read. Both maps are always
// non-nil, so lookups never panic.
func (h *TeachingHandler) groupCatalogNames(ctx context.Context, enrich bool) (names, grades map[string]string) {
	names, grades = map[string]string{}, map[string]string{}
	if !enrich {
		return names, grades
	}
	groups, err := h.config.ListGroups(ctx, "", "")
	if err != nil {
		return names, grades
	}
	gradeNames := map[string]string{}
	if levels, err := h.config.ListGradeLevels(ctx, ""); err == nil {
		for _, gl := range levels {
			gradeNames[gl.ID] = gl.Name
		}
	}
	for _, g := range groups {
		names[g.ID] = g.Name
		grades[g.ID] = gradeNames[g.GradeLevelID]
	}
	return names, grades
}

// --- groups ----------------------------------------------------------------

// teacherGroupResponse is one of the caller's OWN groups, enriched with its
// display name. A name that cannot be resolved degrades to "" — never an error.
type teacherGroupResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// GradeLevelName is the group's grade ("1° PRIMARIA"): groups are named by
	// letter, so "A" alone does not say which one.
	GradeLevelName string `json:"gradeLevelName"`
}

// Groups handles GET /api/v1/teaching/groups — the DISTINCT groups the CALLER
// teaches, each enriched with its display name. The group set is derived from the
// principal (via callerGroups), so no request field can widen it. A principal with
// no linked teacher record gets an empty list — they simply teach nothing.
//
// It runs behind RequireAuth + rotation, the module_attendance + module_config
// guards and RequirePermission("attendance_own:read"). The output is sorted by
// group id so the list is deterministic.
func (h *TeachingAttendanceHandler) Groups(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	_, groups, err := h.scope.callerGroups(r.Context(), p)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	names, grades := h.scope.groupCatalogNames(r.Context(), len(groups) > 0)
	ids := make([]string, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]teacherGroupResponse, 0, len(ids))
	for _, id := range ids {
		out = append(out, teacherGroupResponse{ID: id, Name: names[id], GradeLevelName: grades[id]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": out})
}

// --- classes (T3) -------------------------------------------------------------

// attendanceClassResponse is one pass list the caller takes: a DAILY group
// (once, however many subjects the caller teaches there) or a PER_CLASS
// assignment (group + subject).
type attendanceClassResponse struct {
	GroupID      string `json:"groupId"`
	GroupLabel   string `json:"groupLabel"`
	LevelName    string `json:"levelName"`
	Mode         string `json:"mode"`
	SubjectID    string `json:"subjectId,omitempty"`
	SubjectName  string `json:"subjectName,omitempty"`
	AssignmentID string `json:"assignmentId,omitempty"`
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
		class := attendanceClassResponse{GroupID: a.GroupID, GroupLabel: group.Label, LevelName: group.LevelName, Mode: group.Mode}
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
// the refusal itself and returns the group's mode and true only when the caller
// may touch the list:
//
//   - 404 when the caller teaches nothing in the group (active cycle) — the same
//     answer as a group that does not exist, so nothing leaks;
//   - 400 when the subject does not fit the group's mode;
//   - 404 on a per-class list whose (group, subject) assignment is not the
//     caller's — another teacher's class in the same group is invisible too.
//
// The caller's assignments come from the PRINCIPAL, never the request.
func (h *TeachingAttendanceHandler) assertOwnsList(w http.ResponseWriter, r *http.Request, p middleware.Principal, groupID, subjectID string) (string, bool) {
	_, assignments, err := h.scope.callerAssignments(r.Context(), p)
	if err != nil {
		writeConnectError(w, err)
		return "", false
	}
	teachesGroup, teachesSubject := false, false
	for _, a := range assignments {
		if a.GroupID != groupID || groupID == "" {
			continue
		}
		teachesGroup = true
		if a.SubjectID == subjectID {
			teachesSubject = true
		}
	}
	if !teachesGroup {
		writeNotFound(w)
		return "", false
	}
	catalog, err := loadAttendanceCatalog(r.Context(), h.scope.config)
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
	if group.Mode == attendanceModePerClass && !teachesSubject {
		writeNotFound(w)
		return "", false
	}
	return group.Mode, true
}

// --- roster ----------------------------------------------------------------

// Roster handles GET /api/v1/teaching/attendance/roster?groupId=&date=[&subjectId=]
// — one of the caller's OWN lists (see assertOwnsList), each row enriched with
// the student's name. Any date may be READ; editable says whether the caller may
// still save it (today back to attendanceWindowDays days, never the future).
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
	mode, ok := h.assertOwnsList(w, r, p, groupID, subjectID)
	if !ok {
		return
	}
	date, ok := parseListDate(dateStr)
	if !ok {
		writeBadRequest(w, msgInvalidDate)
		return
	}
	roster, err := h.attendance.GetDailyRoster(r.Context(), groupID, dateStr, subjectID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	names := h.scope.studentNames(r.Context(), len(roster.Rows) > 0)
	editable := teacherWindowRefusal(date, h.now()) == ""
	writeJSON(w, http.StatusOK, toAttendanceListResponse(mode, dateStr, editable, roster, names))
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
	if _, ok := h.assertOwnsList(w, r, p, req.GroupID, req.SubjectID); !ok {
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
