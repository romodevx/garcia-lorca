package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/garcia-lorca/pkg/studentname"
	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// The period grade surface (T15) serves two audiences with ONE handler:
//
//   - a teacher, on /api/v1/teaching/grades/*, limited to the assignments they
//     teach in the active cycle (TeachingHandler.OwnsAssignment);
//   - the director, on /api/v1/grades/{period-sheet,period-grades}, over any
//     assignment of the school (AnyTenantAssignment).
//
// The differences are the AssignmentGate and the CaptureScope: the director is
// school-wide, so the periods a transferred pupil spent in a previous group
// stay writable; for a teacher they are read-only (per row on the sheet).
// Everything the grade itself must satisfy — a WHOLE number on the level's
// scale, a period of the group's grade that has started, the enrollment in the
// group, a level with numeric grading, a term not closed — is judged by the
// grades service, so both audiences get the same rules and the same sentences.
//
// Every refusal carries a machine code in the X-Error-Code header
// (GRADE_NOT_INTEGER, GRADE_OUT_OF_RANGE, GRADE_NOT_NUMBER,
// PERIOD_NOT_IN_GRADE, PERIOD_NOT_STARTED, PERIOD_PREVIOUS_GROUP,
// LEVEL_NOT_GRADED, ENROLLMENT_NOT_IN_GROUP, and PERIOD_LOCKED with 409).
// Monthly capture (T6) is retired: its routes are gone.

// PeriodGradesService is the grades surface the period handler consumes. The
// Connect GradesClient satisfies it.
type PeriodGradesService interface {
	GetPeriodSheet(ctx context.Context, q client.PeriodSheetQuery) (client.PeriodSheet, error)
	UpsertPeriodGrade(ctx context.Context, in client.PeriodGradeInput) (client.PeriodGrade, error)
	ClearPeriodGrade(ctx context.Context, cell client.PeriodCell) error
}

// StudentNamesReader names a sheet's pupils by id in one read. The Connect
// StudentsClient satisfies it.
type StudentNamesReader interface {
	StudentsByIDs(ctx context.Context, ids []string) ([]client.StudentResult, error)
}

// CaptureScope is on whose behalf the capture routes act.
type CaptureScope uint8

const (
	// ScopeTeacher: a teacher on their own assignments; periods that ended
	// before a pupil's transfer into the group are read-only.
	ScopeTeacher CaptureScope = iota
	// ScopeSchool: the director, acting for the whole school.
	ScopeSchool
)

// AssignmentGate decides whether the request may touch assignmentID. It writes
// the refusal itself (404 for an assignment the caller may not see) and returns
// true only when the request may proceed.
type AssignmentGate func(w http.ResponseWriter, r *http.Request, assignmentID string) bool

// AnyTenantAssignment is the director's gate: grades:read/grades:write reach any
// assignment of the school, and an unknown one is the grades service's 404.
func AnyTenantAssignment(http.ResponseWriter, *http.Request, string) bool { return true }

// Sentences (and machine codes) the edge refuses with before the grades
// service is called — the same the service would answer.
const (
	msgValueNotNumber        = "La calificación debe ser un número"
	msgEnrollmentNotGroup    = "El alumno no está inscrito en este grupo"
	msgPeriodNotInGrade      = "Ese periodo no pertenece al grado del grupo"
	codeGradeNotNumber       = "GRADE_NOT_NUMBER"
	codeEnrollmentNotInGroup = "ENROLLMENT_NOT_IN_GROUP"
	codePeriodNotInGrade     = "PERIOD_NOT_IN_GRADE"
)

// PeriodGradesHandler serves the period capture sheet and its per-cell writes.
type PeriodGradesHandler struct {
	grades   PeriodGradesService
	students StudentNamesReader
	names    UserNamer
	gate     AssignmentGate
	scope    CaptureScope
}

// NewPeriodGradesHandler wires the handler. students resolves row names (the
// gateway reads them on the caller's behalf: a Maestro holds no students:read);
// names resolves who captured a grade (nil: the token's email); scope is
// ScopeSchool on the director's routes, ScopeTeacher on the teacher's.
func NewPeriodGradesHandler(grades PeriodGradesService, students StudentNamesReader, names UserNamer, gate AssignmentGate, scope CaptureScope) *PeriodGradesHandler {
	return &PeriodGradesHandler{grades: grades, students: students, names: names, gate: gate, scope: scope}
}

func (h *PeriodGradesHandler) schoolWide() bool { return h.scope == ScopeSchool }

// --- responses ---------------------------------------------------------------

// periodRefResponse is one evaluation period of the group's grade.
type periodRefResponse struct {
	ID       string `json:"id"`
	Ordinal  int    `json:"ordinal"`
	Name     string `json:"name"`
	StartsOn string `json:"startsOn"`
	EndsOn   string `json:"endsOn"`
}

// periodSheetRowResponse is one pupil's cell. rowEditable says whether the
// caller may write THIS row.
type periodSheetRowResponse struct {
	EnrollmentID      string       `json:"enrollmentId"`
	StudentID         string       `json:"studentId"`
	Name              string       `json:"name"`
	Value             *json.Number `json:"value"`
	Letter            *string      `json:"letter,omitempty"`
	UpdatedAt         *string      `json:"updatedAt"`
	RowEditable       bool         `json:"rowEditable"`
	RowReadOnlyReason string       `json:"rowReadOnlyReason,omitempty"`
}

type periodSheetResponse struct {
	AssignmentID    string                   `json:"assignmentId"`
	PeriodID        *string                  `json:"periodId"`
	Periods         []periodRefResponse      `json:"periods"`
	CurrentPeriodID *string                  `json:"currentPeriodId"`
	Today           string                   `json:"today"`
	Scale           gradeScaleResponse       `json:"scale"`
	GradingEnabled  bool                     `json:"gradingEnabled"`
	GradingLetters  bool                     `json:"gradingLetters"`
	Editable        bool                     `json:"editable"`
	ReadOnlyReason  string                   `json:"readOnlyReason,omitempty"`
	ClosedPeriodIDs []string                 `json:"closedPeriodIds"`
	Rows            []periodSheetRowResponse `json:"rows"`
}

// periodGradeResponse is a saved cell: a number in value, or — on a level
// that grades with letters — value null and the letter.
type periodGradeResponse struct {
	EnrollmentID string       `json:"enrollmentId"`
	PeriodID     string       `json:"periodId"`
	Value        *json.Number `json:"value"`
	Letter       *string      `json:"letter,omitempty"`
	UpdatedAt    *string      `json:"updatedAt"`
}

// putPeriodGradeRequest is one autosaved cell. value stays raw: it must be a
// JSON number, and its exact text is what the grades service judges.
type putPeriodGradeRequest struct {
	AssignmentID string          `json:"assignmentId"`
	PeriodID     string          `json:"periodId"`
	EnrollmentID string          `json:"enrollmentId"`
	Value        json.RawMessage `json:"value"`
}

// --- routes ------------------------------------------------------------------

// Sheet handles GET …/period-sheet?assignmentId=&periodId= — the capture grid
// of one assignment for one period (periodId omitted ⇒ the period of the
// school's day), rows sorted by last name in Spanish order.
func (h *PeriodGradesHandler) Sheet(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	assignmentID, ok := h.admitAssignment(w, r, q.Get("assignmentId"))
	if !ok {
		return
	}
	periodID, ok := optionalPeriod(w, q.Get("periodId"))
	if !ok {
		return
	}
	sheet, err := h.grades.GetPeriodSheet(r.Context(), client.PeriodSheetQuery{
		AssignmentID: assignmentID, PeriodID: periodID, SchoolWide: h.schoolWide(),
	})
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.toSheetResponse(r.Context(), sheet))
}

// Put handles PUT …/period-grades {assignmentId, periodId, enrollmentId,
// value}: record or correct one cell. value must be a JSON number (a whole
// number — the service refuses decimals); to empty a cell the client sends
// DELETE.
func (h *PeriodGradesHandler) Put(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req putPeriodGradeRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	value, ok := gradeText(req.Value)
	if !ok {
		writeCodedError(w, http.StatusBadRequest, codeGradeNotNumber, msgValueNotNumber)
		return
	}
	enrollmentID, ok := canonicalEnrollment(w, req.EnrollmentID)
	if !ok {
		return
	}
	periodID, ok := requiredPeriod(w, req.PeriodID)
	if !ok {
		return
	}
	assignmentID, ok := h.admitAssignment(w, r, req.AssignmentID)
	if !ok {
		return
	}
	g, err := h.grades.UpsertPeriodGrade(r.Context(), client.PeriodGradeInput{
		AssignmentID: assignmentID, PeriodID: periodID, EnrollmentID: enrollmentID, Value: value,
		CapturedBy: p.UserID, CapturedByName: callerName(r.Context(), h.names, p), SchoolWide: h.schoolWide(),
	})
	if err != nil {
		writeConnectError(w, err)
		return
	}
	number, letter := splitGrade(g.Value)
	writeJSON(w, http.StatusOK, periodGradeResponse{
		EnrollmentID: g.EnrollmentID, PeriodID: g.PeriodID, Value: number, Letter: letter, UpdatedAt: rfc3339OrNil(g.UpdatedAt),
	})
}

// Clear handles DELETE …/period-grades?assignmentId=&periodId=&enrollmentId=
// — empty one cell (204; emptying an empty cell succeeds too).
func (h *PeriodGradesHandler) Clear(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	enrollmentID, ok := canonicalEnrollment(w, q.Get("enrollmentId"))
	if !ok {
		return
	}
	periodID, ok := requiredPeriod(w, q.Get("periodId"))
	if !ok {
		return
	}
	assignmentID, ok := h.admitAssignment(w, r, q.Get("assignmentId"))
	if !ok {
		return
	}
	cell := client.PeriodCell{AssignmentID: assignmentID, PeriodID: periodID, EnrollmentID: enrollmentID, SchoolWide: h.schoolWide()}
	if err := h.grades.ClearPeriodGrade(r.Context(), cell); err != nil {
		writeConnectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- helpers -----------------------------------------------------------------

// admitAssignment canonicalises the assignment id (a malformed id names no
// assignment: 404) and runs the gate on the canonical form, so an upper-case
// spelling can neither slip past nor be refused by a string compare.
func (h *PeriodGradesHandler) admitAssignment(w http.ResponseWriter, r *http.Request, raw string) (string, bool) {
	canonical, ok := canonicalUUID(raw)
	if !ok {
		writeNotFound(w)
		return "", false
	}
	if !h.gate(w, r, canonical) {
		return "", false
	}
	return canonical, true
}

// canonicalEnrollment parses the enrollment id; a malformed one cannot be in the
// group, and says so with the same sentence the service uses.
func canonicalEnrollment(w http.ResponseWriter, raw string) (string, bool) {
	id, ok := canonicalUUID(raw)
	if !ok {
		writeCodedError(w, http.StatusBadRequest, codeEnrollmentNotInGroup, msgEnrollmentNotGroup)
		return "", false
	}
	return id, true
}

// canonicalUUID accepts a uuid in any letter case and returns it lower-cased,
// the form every service stores and compares.
func canonicalUUID(raw string) (string, bool) {
	if !uuidShape.MatchString(raw) {
		return "", false
	}
	return strings.ToLower(raw), true
}

// requiredPeriod canonicalises a period id; a malformed one names no period of
// the grade and says so with the service's sentence.
func requiredPeriod(w http.ResponseWriter, raw string) (string, bool) {
	id, ok := canonicalUUID(raw)
	if !ok {
		writeCodedError(w, http.StatusBadRequest, codePeriodNotInGrade, msgPeriodNotInGrade)
		return "", false
	}
	return id, true
}

// optionalPeriod is requiredPeriod for the sheet, where "" means "today's".
func optionalPeriod(w http.ResponseWriter, raw string) (string, bool) {
	if raw == "" {
		return "", true
	}
	return requiredPeriod(w, raw)
}

func (h *PeriodGradesHandler) toSheetResponse(ctx context.Context, s client.PeriodSheet) periodSheetResponse {
	students := studentsByID(ctx, h.students, s.Rows)
	rows := make([]periodSheetRowResponse, 0, len(s.Rows))
	for _, row := range s.Rows {
		out := periodSheetRowResponse{
			EnrollmentID: row.EnrollmentID, StudentID: row.StudentID, Name: students.name(row.StudentID),
			RowEditable: row.Editable, RowReadOnlyReason: row.ReadOnlyReason,
		}
		if row.Value != "" {
			out.Value, out.Letter = splitGrade(row.Value)
			out.UpdatedAt = rfc3339OrNil(row.UpdatedAt)
		}
		rows = append(rows, out)
	}
	sortBySurname(students, rows, func(r periodSheetRowResponse) string { return r.StudentID })
	closed := s.ClosedPeriodIDs
	if closed == nil {
		closed = []string{}
	}
	return periodSheetResponse{
		AssignmentID: s.AssignmentID, PeriodID: stringOrNil(s.PeriodID), Periods: toPeriodRefResponses(s.Periods),
		CurrentPeriodID: stringOrNil(s.CurrentPeriodID), Today: s.Today, Scale: toGradeScaleResponse(s.Scale),
		GradingEnabled: s.GradingEnabled, GradingLetters: s.GradingLetters, Editable: s.Editable, ReadOnlyReason: s.ReadOnlyReason,
		ClosedPeriodIDs: closed, Rows: rows,
	}
}

func toPeriodRefResponses(ps []client.EvaluationPeriodRef) []periodRefResponse {
	out := make([]periodRefResponse, 0, len(ps))
	for _, p := range ps {
		out = append(out, periodRefResponse{ID: p.ID, Ordinal: p.Ordinal, Name: p.Name, StartsOn: p.StartsOn, EndsOn: p.EndsOn})
	}
	return out
}

// gradeLetters are the letters a cell may carry (preescolar).
var gradeLetters = map[string]bool{"A": true, "B": true, "C": true, "D": true}

// gradeText reads a cell's value as the grades service judges it: a JSON
// number's exact text, or a JSON string holding one ASCII letter (trimmed,
// upper-cased). Which letters, and whether the level takes letters at all, is
// grades' call: "E" there reads GRADE_NOT_LETTER, the sentence preescolar needs.
func gradeText(raw json.RawMessage) (string, bool) {
	if text, ok := numberText(raw); ok {
		return text, true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	letter := strings.ToUpper(strings.TrimSpace(s))
	return letter, len(letter) == 1 && letter[0] >= 'A' && letter[0] <= 'Z'
}

// splitGrade puts a grade's text where the JSON carries it: a letter in
// "letter" (value null), a number in "value".
func splitGrade(text string) (*json.Number, *string) {
	if gradeLetters[text] {
		return nil, &text
	}
	n := json.Number(text)
	return &n, nil
}

// stringOrNil is s, or JSON null when empty.
func stringOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// studentDirectory is the id → student map a sheet's names come from.
type studentDirectory map[string]client.StudentResult

// name is the pupil as lists show them, "Paterno Materno Nombre(s)"; empty for
// a pupil the directory does not know.
func (d studentDirectory) name(id string) string { return d[id].FullName() }

// sortBySurname orders rows by their pupil's surnames then first name, in
// Spanish (pkg/studentname); rows with the same name keep their order.
func sortBySurname[T any](d studentDirectory, rows []T, studentID func(T) string) {
	studentname.Sort(rows, func(r T) studentname.Parts { return d[studentID(r)].NameParts() })
}

// studentsByID names the sheet's own pupils in ONE read by id (never the whole
// school, never one read per row). It is best-effort: a failed read leaves
// names empty, never fails the sheet.
func studentsByID(ctx context.Context, students StudentNamesReader, rows []client.PeriodSheetRow) studentDirectory {
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.StudentID)
	}
	return studentsByIDs(ctx, students, ids)
}

// studentsByIDs names the given pupils in ONE read, best-effort: a failed read
// is logged (a count, never names) and leaves the directory empty, so a screen
// or a report shows unnamed rows rather than failing.
func studentsByIDs(ctx context.Context, students StudentNamesReader, ids []string) studentDirectory {
	out := studentDirectory{}
	if len(ids) == 0 || students == nil {
		return out
	}
	list, err := students.StudentsByIDs(ctx, ids)
	if err != nil {
		slog.WarnContext(ctx, "naming pupils failed; rows go unnamed", "pupils", len(ids), "error", err)
		return out
	}
	for _, s := range list {
		out[s.ID] = s
	}
	return out
}

func rfc3339OrNil(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}
