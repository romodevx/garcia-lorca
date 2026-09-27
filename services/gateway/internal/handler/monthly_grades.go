package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// The monthly grade surface (T6) serves two audiences with ONE handler:
//
//   - a teacher, on /api/v1/teaching/grades/*, limited to the assignments they
//     teach in the active cycle (TeachingHandler.OwnsAssignment);
//   - the director, on /api/v1/grades/{sheet,monthly}, over any assignment of
//     the school (AnyTenantAssignment).
//
// The differences are the AssignmentGate and the CaptureScope: the director is
// school-wide, so the months a transferred pupil spent in a previous group stay
// writable; for a teacher they are read-only (per row on the sheet). Everything
// the grade itself must satisfy — scale, decimals, month in the cycle,
// enrollment in the group, level with numeric grading — is judged by the grades
// service, so both audiences get the same rules and the same sentences.
//
// Every refusal carries a machine code in the X-Error-Code header
// (GRADE_OUT_OF_RANGE, GRADE_DECIMALS, GRADE_NOT_NUMBER, MONTH_OUTSIDE_CYCLE,
// LEVEL_NOT_GRADED, ENROLLMENT_NOT_IN_GROUP, MONTH_PREVIOUS_GROUP, …).

// MonthlyGradesService is the grades surface the monthly handler consumes. The
// Connect GradesClient satisfies it.
type MonthlyGradesService interface {
	GetMonthlySheet(ctx context.Context, q client.MonthlySheetQuery) (client.MonthlySheet, error)
	UpsertMonthlyGrade(ctx context.Context, in client.MonthlyGradeInput) (client.MonthlyGrade, error)
	ClearMonthlyGrade(ctx context.Context, cell client.MonthlyCell) error
}

// StudentNamesReader names a sheet's pupils by id in one read. The Connect
// StudentsClient satisfies it.
type StudentNamesReader interface {
	StudentsByIDs(ctx context.Context, ids []string) ([]client.StudentResult, error)
}

// CaptureScope is on whose behalf the monthly routes act.
type CaptureScope uint8

const (
	// ScopeTeacher: a teacher on their own assignments; months before a
	// pupil's transfer into the group are read-only.
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
	codeGradeNotNumber       = "GRADE_NOT_NUMBER"
	codeEnrollmentNotInGroup = "ENROLLMENT_NOT_IN_GROUP"
)

// MonthlyGradesHandler serves the monthly capture sheet and its per-cell writes.
type MonthlyGradesHandler struct {
	grades   MonthlyGradesService
	students StudentNamesReader
	gate     AssignmentGate
	scope    CaptureScope
}

// NewMonthlyGradesHandler wires the handler. students resolves row names (the
// gateway reads them on the caller's behalf: a Maestro holds no students:read);
// scope is ScopeSchool on the director's routes, ScopeTeacher on the teacher's.
func NewMonthlyGradesHandler(grades MonthlyGradesService, students StudentNamesReader, gate AssignmentGate, scope CaptureScope) *MonthlyGradesHandler {
	return &MonthlyGradesHandler{grades: grades, students: students, gate: gate, scope: scope}
}

func (h *MonthlyGradesHandler) schoolWide() bool { return h.scope == ScopeSchool }

// --- responses ---------------------------------------------------------------

// monthlySheetRowResponse is one pupil's cell. rowEditable says whether the
// caller may write THIS row (false, with rowReadOnlyReason, for the months a
// transferred pupil spent in their previous group, or when the whole sheet is
// read-only).
type monthlySheetRowResponse struct {
	EnrollmentID      string       `json:"enrollmentId"`
	StudentID         string       `json:"studentId"`
	Name              string       `json:"name"`
	Value             *json.Number `json:"value"`
	UpdatedAt         *string      `json:"updatedAt"`
	RowEditable       bool         `json:"rowEditable"`
	RowReadOnlyReason string       `json:"rowReadOnlyReason,omitempty"`
}

type monthlySheetResponse struct {
	AssignmentID   string                    `json:"assignmentId"`
	Month          string                    `json:"month"`
	Months         []string                  `json:"months"`
	Scale          gradeScaleResponse        `json:"scale"`
	GradingEnabled bool                      `json:"gradingEnabled"`
	Editable       bool                      `json:"editable"`
	ReadOnlyReason string                    `json:"readOnlyReason,omitempty"`
	Rows           []monthlySheetRowResponse `json:"rows"`
}

type monthlyGradeResponse struct {
	EnrollmentID string      `json:"enrollmentId"`
	Month        string      `json:"month"`
	Value        json.Number `json:"value"`
	UpdatedAt    *string     `json:"updatedAt"`
}

// putMonthlyGradeRequest is one autosaved cell. value stays raw: it must be a
// JSON number, and its exact text is what the grades service judges.
type putMonthlyGradeRequest struct {
	AssignmentID string          `json:"assignmentId"`
	Month        string          `json:"month"`
	EnrollmentID string          `json:"enrollmentId"`
	Value        json.RawMessage `json:"value"`
}

// --- routes ------------------------------------------------------------------

// Sheet handles GET …/grades/sheet?assignmentId=&month=YYYY-MM — the capture
// grid of one assignment for one month (month omitted ⇒ the current UTC month
// clamped into the cycle), rows sorted by last name in Spanish order.
func (h *MonthlyGradesHandler) Sheet(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	assignmentID, ok := h.admitAssignment(w, r, q.Get("assignmentId"))
	if !ok {
		return
	}
	sheet, err := h.grades.GetMonthlySheet(r.Context(), client.MonthlySheetQuery{
		AssignmentID: assignmentID, Month: q.Get("month"), SchoolWide: h.schoolWide(),
	})
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.toSheetResponse(r.Context(), sheet))
}

// Put handles PUT …/grades/monthly {assignmentId, month, enrollmentId, value}:
// record or correct one cell. value must be a JSON number; to empty a cell the
// client sends DELETE.
func (h *MonthlyGradesHandler) Put(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req putMonthlyGradeRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	value, ok := numberText(req.Value)
	if !ok {
		writeCodedError(w, http.StatusBadRequest, codeGradeNotNumber, msgValueNotNumber)
		return
	}
	enrollmentID, ok := canonicalEnrollment(w, req.EnrollmentID)
	if !ok {
		return
	}
	assignmentID, ok := h.admitAssignment(w, r, req.AssignmentID)
	if !ok {
		return
	}
	g, err := h.grades.UpsertMonthlyGrade(r.Context(), client.MonthlyGradeInput{
		AssignmentID: assignmentID, Month: req.Month, EnrollmentID: enrollmentID, Value: value, CapturedBy: p.UserID,
		SchoolWide: h.schoolWide(),
	})
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, monthlyGradeResponse{
		EnrollmentID: g.EnrollmentID, Month: g.Month, Value: json.Number(g.Value), UpdatedAt: rfc3339OrNil(g.UpdatedAt),
	})
}

// Clear handles DELETE …/grades/monthly?assignmentId=&month=&enrollmentId= —
// empty one cell (204; emptying an empty cell succeeds too).
func (h *MonthlyGradesHandler) Clear(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	enrollmentID, ok := canonicalEnrollment(w, q.Get("enrollmentId"))
	if !ok {
		return
	}
	assignmentID, ok := h.admitAssignment(w, r, q.Get("assignmentId"))
	if !ok {
		return
	}
	cell := client.MonthlyCell{AssignmentID: assignmentID, Month: q.Get("month"), EnrollmentID: enrollmentID, SchoolWide: h.schoolWide()}
	if err := h.grades.ClearMonthlyGrade(r.Context(), cell); err != nil {
		writeConnectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- helpers -----------------------------------------------------------------

// admitAssignment canonicalises the assignment id (a malformed id names no
// assignment: 404) and runs the gate on the canonical form, so an upper-case
// spelling can neither slip past nor be refused by a string compare.
func (h *MonthlyGradesHandler) admitAssignment(w http.ResponseWriter, r *http.Request, raw string) (string, bool) {
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

func (h *MonthlyGradesHandler) toSheetResponse(ctx context.Context, s client.MonthlySheet) monthlySheetResponse {
	students := studentsByID(ctx, h.students, s.Rows)
	rows := make([]monthlySheetRowResponse, 0, len(s.Rows))
	for _, row := range s.Rows {
		out := monthlySheetRowResponse{
			EnrollmentID: row.EnrollmentID, StudentID: row.StudentID, Name: students.name(row.StudentID),
			RowEditable: row.Editable, RowReadOnlyReason: row.ReadOnlyReason,
		}
		if row.Value != "" {
			v := json.Number(row.Value)
			out.Value, out.UpdatedAt = &v, rfc3339OrNil(row.UpdatedAt)
		}
		rows = append(rows, out)
	}
	sortByLastName(rows, students)
	months := s.Months
	if months == nil {
		months = []string{}
	}
	return monthlySheetResponse{
		AssignmentID: s.AssignmentID, Month: s.Month, Months: months, Scale: toGradeScaleResponse(s.Scale),
		GradingEnabled: s.GradingEnabled, Editable: s.Editable, ReadOnlyReason: s.ReadOnlyReason, Rows: rows,
	}
}

// studentDirectory is the id → student map a sheet's names come from.
type studentDirectory map[string]client.StudentResult

func (d studentDirectory) name(id string) string {
	s := d[id]
	return joinName(s.FirstName, s.LastName)
}

func joinName(first, last string) string {
	switch {
	case first == "":
		return last
	case last == "":
		return first
	default:
		return first + " " + last
	}
}

// studentsByID names the sheet's own pupils in ONE read by id (never the whole
// school, never one read per row). It is best-effort: a failed read leaves
// names empty, never fails the sheet.
func studentsByID(ctx context.Context, students StudentNamesReader, rows []client.MonthlySheetRow) studentDirectory {
	out := studentDirectory{}
	if len(rows) == 0 || students == nil {
		return out
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.StudentID)
	}
	list, err := students.StudentsByIDs(ctx, ids)
	if err != nil {
		return out
	}
	for _, s := range list {
		out[s.ID] = s
	}
	return out
}

// sortByLastName orders rows by last name, then first name, in Spanish
// collation (accents fold, ñ sorts after n); ties keep enrollment-id order.
func sortByLastName(rows []monthlySheetRowResponse, students studentDirectory) {
	col := collate.New(language.Spanish)
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := students[rows[i].StudentID], students[rows[j].StudentID]
		if c := col.CompareString(a.LastName, b.LastName); c != 0 {
			return c < 0
		}
		return col.CompareString(a.FirstName, b.FirstName) < 0
	})
}

func rfc3339OrNil(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}
