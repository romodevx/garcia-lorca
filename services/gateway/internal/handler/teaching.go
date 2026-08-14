package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// The teaching surface is the SELF-SCOPED, teacher-facing grade surface: a
// maestro capturing and reading grades ONLY for the groups+subjects they teach.
// It owns no service of its own — it composes the config catalog (teacher
// directory + teaching assignments) with the grades service behind ONE scoping
// rule.
//
// The scoping is BY CONSTRUCTION, not by a check on request content: the teacher
// id always comes from the authenticated principal (the Teacher whose user_id ==
// principal.UserID), so the set of assignments the caller may read or write is
// whatever that teacher is assigned and nothing else. No request field (body,
// query or path) can widen it. This is what makes /teaching/* safe to grant to a
// non-owner Maestro role — in contrast to grades:read/grades:write, which read
// and write ANY group and therefore stay broad staff permissions.
//
// The assignmentId a roster/capture request carries IS caller-supplied, so each
// one first calls assertOwnsAssignment and answers 404 for any id outside the
// caller's own set — the same answer a non-existent id gets, so a teacher can
// never probe whether another teacher's assignment exists.

// --- consumer-defined reader interfaces -----------------------------------

// TeacherDirectoryReader lists the tenant's teacher records. It is the scoping
// primitive of the teaching surface: the caller's own Teacher is the one whose
// user id matches the authenticated principal, found here — never taken from the
// request.
type TeacherDirectoryReader interface {
	ListTeachers(ctx context.Context) ([]client.Teacher, error)
}

// TeachingAssignmentReader lists the assignments of one teacher (the knot binding
// a teacher to a group+subject within a cycle). Grades are captured/read per
// assignment, so this is how the surface bounds what the caller may touch.
type TeachingAssignmentReader interface {
	ListTeachingAssignments(ctx context.Context, cycleID, groupID, teacherID string) ([]client.TeachingAssignment, error)
}

// TeachingCatalogReader turns the bare group/subject UUIDs on an assignment into
// the human-readable names the teacher's screen needs to render. Passing "" lists
// the whole tenant-scoped catalog, which is what the id→name maps are built from.
type TeachingCatalogReader interface {
	ListGroups(ctx context.Context, cycleID, gradeLevelID string) ([]client.GroupResult, error)
	ListSubjects(ctx context.Context, gradeLevelID string) ([]client.SubjectResult, error)
}

// TeachingConfigReader is the config surface the teaching handler consumes: the
// teacher directory (caller→teacher resolution), the teacher's assignments, and
// the group/subject catalog for name enrichment. The Connect ConfigClient (the
// same one backing the /config/* routes) satisfies it.
type TeachingConfigReader interface {
	TeacherDirectoryReader
	TeachingAssignmentReader
	TeachingCatalogReader
}

// TeachingGradesService is the grade surface the teaching handler consumes: the
// per-assignment roster read and the grade capture. The Connect GradesClient
// satisfies it.
type TeachingGradesService interface {
	GetRoster(ctx context.Context, assignmentID, termID string) ([]client.RosterRow, error)
	CaptureGrade(ctx context.Context, assignmentID, enrollmentID, termID string, value float64) (client.GradeEntry, error)
}

// TeachingStudentReader turns a roster's bare student UUIDs into the readable
// names the teacher's capture screen needs. A Maestro role holds no
// students:read, so the gateway — trusted, and running regardless of the caller's
// permissions — resolves the names on the teacher's behalf. The Connect
// StudentsClient (the same one backing /students and /family/*) satisfies it. One
// list read (status "" — the whole tenant roster, honoring the client's page cap)
// builds the id→name map, never one GetStudent per row.
type TeachingStudentReader interface {
	ListStudents(ctx context.Context, status string) ([]client.StudentResult, error)
}

// TeachingHandler is the teacher-facing, self-scoped grade surface.
type TeachingHandler struct {
	config   TeachingConfigReader
	grades   TeachingGradesService
	students TeachingStudentReader
}

// NewTeachingHandler wires the handler to the config (teacher directory +
// assignments + catalog), grades (roster + capture) and students (roster name
// enrichment) surfaces.
func NewTeachingHandler(config TeachingConfigReader, grades TeachingGradesService, students TeachingStudentReader) *TeachingHandler {
	return &TeachingHandler{config: config, grades: grades, students: students}
}

// --- scoping core ----------------------------------------------------------

// callerAssignments resolves the assignments the PRINCIPAL teaches. The teacher
// id and the assignment set come only from the principal: the directory is
// searched for the Teacher whose user id matches, then that teacher's assignments
// are listed. No request field selects the teacher.
//
// A principal with no linked teacher record teaches nothing — teacherID "" and a
// nil slice, never an error and never a downstream assignment read. That is the
// "a maestro that teaches nothing" case, not a failure.
func (h *TeachingHandler) callerAssignments(ctx context.Context, principal middleware.Principal) (teacherID string, assignments []client.TeachingAssignment, err error) {
	// Fail closed: an empty subject can never be a teacher. Matching "" would
	// otherwise pair with the first unlinked catalog teacher (UserID=="") and
	// inherit its assignments — so bail before scanning.
	if principal.UserID == "" {
		return "", nil, nil
	}
	teachers, err := h.config.ListTeachers(ctx)
	if err != nil {
		return "", nil, err
	}
	for _, t := range teachers {
		// An unlinked catalog teacher (UserID=="") must never match anyone.
		if t.UserID != "" && t.UserID == principal.UserID {
			teacherID = t.ID
			break
		}
	}
	if teacherID == "" {
		return "", nil, nil // the principal is linked to no teacher record
	}
	assignments, err = h.config.ListTeachingAssignments(ctx, "", "", teacherID)
	if err != nil {
		return "", nil, err
	}
	return teacherID, assignments, nil
}

// assertOwnsAssignment is the ownership gate of the roster and capture routes: it
// resolves the caller's own assignment set and reports whether assignmentID is in
// it. The set is derived from the PRINCIPAL, so no path/query/body value can
// widen it. It writes the failure response itself — 404 for an assignment the
// caller does not teach (never 403, so a teacher cannot probe whether another
// teacher's assignment exists), the mapped status on a lookup error — and returns
// true only when the read/write may proceed.
func (h *TeachingHandler) assertOwnsAssignment(w http.ResponseWriter, r *http.Request, principal middleware.Principal, assignmentID string) bool {
	_, assignments, err := h.callerAssignments(r.Context(), principal)
	if err != nil {
		writeConnectError(w, err)
		return false
	}
	for _, a := range assignments {
		if a.ID == assignmentID {
			return true
		}
	}
	writeNotFound(w)
	return false
}

// --- assignments -----------------------------------------------------------

// teacherAssignmentResponse is the caller's own assignment, enriched with the
// group and subject DISPLAY NAMES the teacher's screen needs. The bare ids are
// kept alongside the names so the app can drive follow-up reads. A name that
// cannot be resolved degrades to "" — never an error.
type teacherAssignmentResponse struct {
	ID          string `json:"id"`
	CycleID     string `json:"cycleId"`
	GroupID     string `json:"groupId"`
	SubjectID   string `json:"subjectId"`
	TeacherID   string `json:"teacherId"`
	GroupName   string `json:"groupName"`
	SubjectName string `json:"subjectName"`
}

// Assignments handles GET /api/v1/teaching/assignments — the groups+subjects the
// CALLER teaches, each enriched with its group and subject display names. The
// assignment set is derived from the principal (assertLinkedChild's teacher
// analogue), so no request field can widen it. A principal with no linked teacher
// record gets an empty list — they simply teach nothing.
//
// It runs behind RequireAuth + rotation, the module_grades + module_config guards
// and RequirePermission("teaching:read").
func (h *TeachingHandler) Assignments(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	_, assignments, err := h.callerAssignments(r.Context(), p)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	// Resolve the id→name maps ONCE, and only when there is something to enrich.
	groupNames, subjectNames := h.catalogNames(r.Context(), len(assignments) > 0)
	out := make([]teacherAssignmentResponse, 0, len(assignments))
	for _, a := range assignments {
		out = append(out, teacherAssignmentResponse{
			ID:          a.ID,
			CycleID:     a.CycleID,
			GroupID:     a.GroupID,
			SubjectID:   a.SubjectID,
			TeacherID:   a.TeacherID,
			GroupName:   groupNames[a.GroupID],
			SubjectName: subjectNames[a.SubjectID],
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"assignments": out})
}

// catalogNames builds the group and subject id→name maps used to enrich the
// assignments read. It is best-effort by design: no assignments to enrich or a
// failed lookup each yields an empty map, and the caller reads a missing id as
// "" — a display gap, never a failed read. Both non-nil maps are always returned,
// so lookups never panic.
func (h *TeachingHandler) catalogNames(ctx context.Context, enrich bool) (groups, subjects map[string]string) {
	groups, subjects = map[string]string{}, map[string]string{}
	if !enrich {
		return groups, subjects
	}
	if grps, err := h.config.ListGroups(ctx, "", ""); err == nil {
		for _, g := range grps {
			groups[g.ID] = g.Name
		}
	}
	if subs, err := h.config.ListSubjects(ctx, ""); err == nil {
		for _, s := range subs {
			subjects[s.ID] = s.Name
		}
	}
	return groups, subjects
}

// --- roster ----------------------------------------------------------------

// teachingRosterRowResponse is the teaching-route-local roster projection: the
// same fields the admin roster returns (enrollment, student, value, hasValue)
// PLUS the student's readable name the teacher's capture screen needs — the join
// a Maestro cannot do client-side without students:read. The admin grades roster
// (rosterRowResponse in grades.go) is untouched; only this surface enriches. A
// name that cannot be resolved degrades to "" — never an error.
type teachingRosterRowResponse struct {
	EnrollmentID string  `json:"enrollmentId"`
	StudentID    string  `json:"studentId"`
	StudentName  string  `json:"studentName"`
	Value        float64 `json:"value"`
	HasValue     bool    `json:"hasValue"`
}

// Roster handles GET /api/v1/teaching/roster?assignmentId=&termId= — the roster
// of one of the caller's OWN assignments, each row enriched with the student's
// readable name. The assignmentId is caller-supplied, so it is verified against
// the caller's own set first (404 for any other id), and only then is the grades
// service asked for the roster.
//
// It runs behind RequireAuth + rotation, the module_grades guard and
// RequirePermission("teaching:read").
func (h *TeachingHandler) Roster(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	q := r.URL.Query()
	assignmentID := q.Get("assignmentId")
	if !h.assertOwnsAssignment(w, r, p, assignmentID) {
		return
	}
	rows, err := h.grades.GetRoster(r.Context(), assignmentID, q.Get("termId"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	// Resolve the studentId→name map ONCE, and only when there is a row to enrich.
	studentNames := h.studentNames(r.Context(), len(rows) > 0)
	out := make([]teachingRosterRowResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, teachingRosterRowResponse{
			EnrollmentID: row.EnrollmentID,
			StudentID:    row.StudentID,
			StudentName:  studentNames[row.StudentID],
			Value:        row.Value,
			HasValue:     row.HasValue,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"roster": out})
}

// studentNames builds the studentId→"First Last" map used to enrich a roster. It
// is best-effort by design: no rows to enrich or a failed list read each yields
// an empty map, and the caller reads a missing id as "" — a display gap, never a
// failed read. A non-nil map is always returned, so lookups never panic. The map
// is built from ONE list read, not one GetStudent per row.
func (h *TeachingHandler) studentNames(ctx context.Context, enrich bool) map[string]string {
	names := map[string]string{}
	if !enrich {
		return names
	}
	students, err := h.students.ListStudents(ctx, "")
	if err != nil {
		return names
	}
	for _, s := range students {
		names[s.ID] = strings.TrimSpace(s.FirstName + " " + s.LastName)
	}
	return names
}

// --- capture ---------------------------------------------------------------

// captureTeachingGradeRequest carries only the client-supplied edges of the
// capture knot; subjectId and teacherId are derived server-side by the grades
// service so they can never be forged.
type captureTeachingGradeRequest struct {
	AssignmentID string  `json:"assignmentId"`
	EnrollmentID string  `json:"enrollmentId"`
	TermID       string  `json:"termId"`
	Value        float64 `json:"value"`
}

// CaptureGrade handles POST /api/v1/teaching/grades — record or edit a grade for
// one of the caller's OWN assignments. The assignmentId is caller-supplied, so it
// is verified against the caller's own set first (404 for any other id — a
// teacher can never capture against another teacher's assignment), and only then
// is the grade captured.
//
// It runs behind RequireAuth + rotation, the module_grades guard and
// RequirePermission("teaching:write").
func (h *TeachingHandler) CaptureGrade(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req captureTeachingGradeRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if !h.assertOwnsAssignment(w, r, p, req.AssignmentID) {
		return
	}
	entry, err := h.grades.CaptureGrade(r.Context(), req.AssignmentID, req.EnrollmentID, req.TermID, req.Value)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toGradeEntryResponse(entry))
}
