package handler

import (
	"context"
	"net/http"
	"sort"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// The teaching-attendance surface is the SELF-SCOPED, teacher-facing daily
// pass-list surface: a maestro reading and reconciling attendance ONLY for the
// groups they teach. It is the attendance twin of the teaching (grades) surface
// in teaching.go and carries the identical safety property.
//
// Attendance is taken per GROUP, not per subject, so the caller's scope is the
// DISTINCT set of group_ids across their teaching_assignments. That set is
// derived BY CONSTRUCTION from the authenticated principal (the Teacher whose
// user_id == principal.UserID → that teacher's assignments → their groups), so no
// request field (body, query or path) can widen it. This is what makes
// /teaching/attendance/* safe to grant to a non-owner Maestro role via the
// attendance_own:read / attendance_own:write permissions — in contrast to
// attendance:read / attendance:write, which touch ANY group and stay broad staff
// permissions.
//
// A groupId a roster/save request carries IS caller-supplied, so each one first
// calls assertOwnsGroup and answers 404 for any group outside the caller's own
// set — the same answer a non-existent group gets, so a teacher can never probe
// whether another group exists.

// --- consumer-defined attendance interface ---------------------------------

// TeachingAttendanceService is the daily pass-list surface the teaching
// attendance handler consumes: the per-group roster read and the per-group·day
// reconcile save. It is deliberately narrower than the admin AttendanceService
// (no per-enrollment history) — a consumer defines only what it uses. The Connect
// AttendanceClient (the same one backing the admin /attendance/* routes)
// satisfies it.
type TeachingAttendanceService interface {
	GetDailyRoster(ctx context.Context, groupID, date string) ([]client.AttendanceRosterRow, error)
	SaveDailyAttendance(ctx context.Context, groupID, date string, marks []client.Mark) (int32, int32, error)
}

// TeachingAttendanceHandler is the teacher-facing, self-scoped daily attendance
// surface. It owns no service of its own: it composes the config-derived scoping
// of the teaching handler (caller→teacher→groups, group-name enrichment and
// roster student-name enrichment — all reused through the scope handler) with the
// attendance service behind ONE ownership rule.
type TeachingAttendanceHandler struct {
	scope      *TeachingHandler
	attendance TeachingAttendanceService
}

// NewTeachingAttendanceHandler wires the attendance surface to the teaching scope
// handler (config + students, providing callerGroups / assertOwnsGroup /
// studentNames / groupCatalogNames) and the attendance service (roster + save).
func NewTeachingAttendanceHandler(scope *TeachingHandler, attendance TeachingAttendanceService) *TeachingAttendanceHandler {
	return &TeachingAttendanceHandler{scope: scope, attendance: attendance}
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

// assertOwnsGroup is the ownership gate of the roster and save routes: it resolves
// the caller's DISTINCT taught groups and reports whether groupID is among them.
// The set is derived from the PRINCIPAL, so no path/query/body value can widen it.
// It writes the failure response itself — 404 for a group the caller does not
// teach (never 403, so a teacher cannot probe whether another group exists), the
// mapped status on a lookup error — and returns true only when the read/write may
// proceed. An empty groupID is never in the set (fail closed), so it too gets 404.
func (h *TeachingHandler) assertOwnsGroup(w http.ResponseWriter, r *http.Request, principal middleware.Principal, groupID string) bool {
	_, groups, err := h.callerGroups(r.Context(), principal)
	if err != nil {
		writeConnectError(w, err)
		return false
	}
	if _, ok := groups[groupID]; ok {
		return true
	}
	writeNotFound(w)
	return false
}

// groupCatalogNames builds the group id→name map used to enrich the caller's
// groups. It is best-effort by design: no groups to enrich or a failed lookup each
// yields an empty map, and the caller reads a missing id as "" — a display gap,
// never a failed read. A non-nil map is always returned, so lookups never panic.
func (h *TeachingHandler) groupCatalogNames(ctx context.Context, enrich bool) map[string]string {
	names := map[string]string{}
	if !enrich {
		return names
	}
	groups, err := h.config.ListGroups(ctx, "", "")
	if err != nil {
		return names
	}
	for _, g := range groups {
		names[g.ID] = g.Name
	}
	return names
}

// --- groups ----------------------------------------------------------------

// teacherGroupResponse is one of the caller's OWN groups, enriched with its
// display name. A name that cannot be resolved degrades to "" — never an error.
type teacherGroupResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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
	names := h.scope.groupCatalogNames(r.Context(), len(groups) > 0)
	ids := make([]string, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]teacherGroupResponse, 0, len(ids))
	for _, id := range ids {
		out = append(out, teacherGroupResponse{ID: id, Name: names[id]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": out})
}

// --- roster ----------------------------------------------------------------

// teachingAttendanceRosterRowResponse is the teaching-route-local roster
// projection: the same fields the admin attendance roster returns (enrollment,
// student, status) PLUS the student's readable name the teacher's pass-list screen
// needs — the join a Maestro cannot do client-side without students:read. The
// admin attendance roster (attendanceRosterRowResponse in attendance.go) is
// untouched; only this surface enriches. A name that cannot be resolved degrades
// to "" — never an error.
type teachingAttendanceRosterRowResponse struct {
	EnrollmentID string `json:"enrollmentId"`
	StudentID    string `json:"studentId"`
	StudentName  string `json:"studentName"`
	Status       string `json:"status"`
}

// Roster handles GET /api/v1/teaching/attendance/roster?groupId=&date= — the day's
// pass list for one of the caller's OWN groups, each row enriched with the
// student's readable name. The groupId is caller-supplied, so it is verified
// against the caller's own set first (404 for any other group), and only then is
// the attendance service asked for the roster.
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
	groupID := q.Get("groupId")
	if !h.scope.assertOwnsGroup(w, r, p, groupID) {
		return
	}
	rows, err := h.attendance.GetDailyRoster(r.Context(), groupID, q.Get("date"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	// Resolve the studentId→name map ONCE, and only when there is a row to enrich.
	studentNames := h.scope.studentNames(r.Context(), len(rows) > 0)
	out := make([]teachingAttendanceRosterRowResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, teachingAttendanceRosterRowResponse{
			EnrollmentID: row.EnrollmentID,
			StudentID:    row.StudentID,
			StudentName:  studentNames[row.StudentID],
			Status:       row.Status,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"roster": out})
}

// --- save ------------------------------------------------------------------

// SaveAttendance handles POST /api/v1/teaching/attendance — reconcile one
// group·day for one of the caller's OWN groups. The body is the admin
// saveDailyRequest shape (groupId, date, marks[]); the groupId is caller-supplied,
// so it is verified against the caller's own set first (404 for any other group —
// a teacher can never write another group's attendance), and only then is the day
// reconciled. The model is DEFAULT-PRESENT: an empty marks array legitimately
// means "everybody present" and is never rejected.
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
	if !h.scope.assertOwnsGroup(w, r, p, req.GroupID) {
		return
	}
	absent, marked, err := h.attendance.SaveDailyAttendance(r.Context(), req.GroupID, req.Date, toMarks(req.Marks))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"absentCount": absent, "markedCount": marked})
}
