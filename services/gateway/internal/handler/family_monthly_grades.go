package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// The family monthly grades read (T6): a parent sees each subject's grade per
// month of the current school year, as soon as the teacher captures it. Same
// scoping rule as every by-child family route: the {studentId} in the path is
// checked against the principal's own children first (404 otherwise).

// FamilyEnrollmentReader resolves the caller's children and one child's
// enrollments. The Connect StudentsClient satisfies it.
type FamilyEnrollmentReader interface {
	GuardianLinkReader
	ListEnrollments(ctx context.Context, studentID, cycleID, groupID string) ([]client.Enrollment, error)
}

// EnrollmentMonthlyGradesReader reads one enrollment's monthly grades. The
// Connect GradesClient satisfies it.
type EnrollmentMonthlyGradesReader interface {
	GetEnrollmentMonthlyGrades(ctx context.Context, enrollmentID string) (client.EnrollmentMonthlyGrades, error)
}

// SubjectNamesReader names subjects. The Connect ConfigClient satisfies it.
type SubjectNamesReader interface {
	ListSubjects(ctx context.Context, gradeLevelID string) ([]client.SubjectResult, error)
}

// FamilyMonthlyGradesHandler serves GET /family/students/{studentId}/grades/monthly.
type FamilyMonthlyGradesHandler struct {
	students FamilyEnrollmentReader
	grades   EnrollmentMonthlyGradesReader
	subjects SubjectNamesReader
}

// NewFamilyMonthlyGradesHandler wires the handler.
func NewFamilyMonthlyGradesHandler(students FamilyEnrollmentReader, grades EnrollmentMonthlyGradesReader, subjects SubjectNamesReader) *FamilyMonthlyGradesHandler {
	return &FamilyMonthlyGradesHandler{students: students, grades: grades, subjects: subjects}
}

// enrollmentStatusActive is how the students client names a live enrollment.
const enrollmentStatusActive = "active"

type familySubjectGradesResponse struct {
	SubjectID   string                 `json:"subjectId"`
	SubjectName string                 `json:"subjectName"`
	Grades      map[string]json.Number `json:"grades"`
}

type familyMonthlyGradesResponse struct {
	GradingEnabled bool                          `json:"gradingEnabled"`
	CycleName      string                        `json:"cycleName"`
	Months         []string                      `json:"months"`
	Scale          gradeScaleResponse            `json:"scale"`
	Subjects       []familySubjectGradesResponse `json:"subjects"`
}

// MonthlyGrades handles GET /api/v1/family/students/{studentId}/grades/monthly
// — the child's ACTIVE enrollment's monthly grades: one entry per subject of
// the group (sorted by name, Spanish order), each a month → grade object ({}
// when nothing is captured yet). A level without numeric grading answers
// gradingEnabled:false with no subjects; a child without an active enrollment
// is 404.
//
// It runs behind RequireAuth + rotation, the module_grades + module_students
// guards and RequirePermission("family:read").
func (h *FamilyMonthlyGradesHandler) MonthlyGrades(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	studentID := r.PathValue("studentId")
	owned, err := ownsChild(r.Context(), h.students, p.UserID, studentID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if !owned {
		writeNotFound(w)
		return
	}
	enrollmentID, err := h.activeEnrollment(r.Context(), studentID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	if enrollmentID == "" {
		writeNotFound(w)
		return
	}
	got, err := h.grades.GetEnrollmentMonthlyGrades(r.Context(), enrollmentID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.toResponse(r.Context(), got))
}

// activeEnrollment is the child's live enrollment id, or "" when there is none.
func (h *FamilyMonthlyGradesHandler) activeEnrollment(ctx context.Context, studentID string) (string, error) {
	enrollments, err := h.students.ListEnrollments(ctx, studentID, "", "")
	if err != nil {
		return "", err
	}
	for _, e := range enrollments {
		if e.Status == enrollmentStatusActive {
			return e.ID, nil
		}
	}
	return "", nil
}

func (h *FamilyMonthlyGradesHandler) toResponse(ctx context.Context, g client.EnrollmentMonthlyGrades) familyMonthlyGradesResponse {
	months := g.Months
	if months == nil {
		months = []string{}
	}
	out := familyMonthlyGradesResponse{
		GradingEnabled: g.GradingEnabled, CycleName: g.CycleName, Months: months,
		Scale: toGradeScaleResponse(g.Scale), Subjects: []familySubjectGradesResponse{},
	}
	if !g.GradingEnabled {
		return out
	}
	names := h.subjectNames(ctx, len(g.SubjectIDs) > 0)
	bySubject := make(map[string]map[string]json.Number, len(g.SubjectIDs))
	for _, id := range g.SubjectIDs {
		bySubject[id] = map[string]json.Number{}
	}
	for _, grade := range g.Grades {
		if cells, ok := bySubject[grade.SubjectID]; ok {
			cells[grade.Month] = json.Number(grade.Value)
		}
	}
	for _, id := range g.SubjectIDs {
		out.Subjects = append(out.Subjects, familySubjectGradesResponse{SubjectID: id, SubjectName: names[id], Grades: bySubject[id]})
	}
	col := collate.New(language.Spanish)
	sort.SliceStable(out.Subjects, func(i, j int) bool {
		return col.CompareString(out.Subjects[i].SubjectName, out.Subjects[j].SubjectName) < 0
	})
	return out
}

// subjectNames is best-effort: a failed read leaves names empty, never fails
// the parent's screen.
func (h *FamilyMonthlyGradesHandler) subjectNames(ctx context.Context, enrich bool) map[string]string {
	out := map[string]string{}
	if !enrich || h.subjects == nil {
		return out
	}
	subjects, err := h.subjects.ListSubjects(ctx, "")
	if err != nil {
		return out
	}
	for _, s := range subjects {
		out[s.ID] = s.Name
	}
	return out
}
