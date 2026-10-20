package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// The family period grades read (T15): a parent sees each subject's grade per
// evaluation period of the child's grade in the current school year, as soon
// as the teacher captures it. Same scoping rule as every by-child family
// route: the {studentId} in the path is checked against the principal's own
// children first (404 otherwise).

// FamilyEnrollmentReader resolves the caller's children and one child's
// enrollments. The Connect StudentsClient satisfies it.
type FamilyEnrollmentReader interface {
	GuardianLinkReader
	ListEnrollments(ctx context.Context, studentID, cycleID, groupID string) ([]client.Enrollment, error)
}

// EnrollmentPeriodGradesReader reads one enrollment's period grades. The
// Connect GradesClient satisfies it.
type EnrollmentPeriodGradesReader interface {
	GetEnrollmentPeriodGrades(ctx context.Context, enrollmentID string) (client.EnrollmentPeriodGrades, error)
}

// SubjectNamesReader names subjects. The Connect ConfigClient satisfies it.
type SubjectNamesReader interface {
	ListSubjects(ctx context.Context, gradeLevelID string) ([]client.SubjectResult, error)
}

// FamilyPeriodGradesHandler serves GET /family/students/{studentId}/grades/periods.
type FamilyPeriodGradesHandler struct {
	students FamilyEnrollmentReader
	grades   EnrollmentPeriodGradesReader
	subjects SubjectNamesReader
	cycles   CycleLister
	now      func() time.Time
}

// NewFamilyPeriodGradesHandler wires the handler. cycles decides which of a
// child's active enrollments is the current school year's.
func NewFamilyPeriodGradesHandler(students FamilyEnrollmentReader, grades EnrollmentPeriodGradesReader, subjects SubjectNamesReader, cycles CycleLister) *FamilyPeriodGradesHandler {
	return &FamilyPeriodGradesHandler{students: students, grades: grades, subjects: subjects, cycles: cycles, now: time.Now}
}

// WithClock replaces the clock "today" is read from (tests).
func (h *FamilyPeriodGradesHandler) WithClock(now func() time.Time) *FamilyPeriodGradesHandler {
	h.now = now
	return h
}

// enrollmentStatusActive is how the students client names a live enrollment.
const enrollmentStatusActive = "active"

type familySubjectGradesResponse struct {
	SubjectID   string                 `json:"subjectId"`
	SubjectName string                 `json:"subjectName"`
	Grades      map[string]json.Number `json:"grades"`
	// Letters are the subject's grades on a level that grades with letters.
	Letters map[string]string `json:"letters,omitempty"`
}

type familyPeriodGradesResponse struct {
	GradingEnabled bool                `json:"gradingEnabled"`
	GradingLetters bool                `json:"gradingLetters"`
	CycleName      string              `json:"cycleName"`
	Periods        []periodRefResponse `json:"periods"`
	// Scale is absent when the level no longer grades numerically: grades do
	// not record the scale they were captured under, so there is nothing
	// honest to judge them by (the app then marks none as failing).
	Scale    *gradeScaleResponse           `json:"scale,omitempty"`
	Subjects []familySubjectGradesResponse `json:"subjects"`
}

// PeriodGrades handles GET /api/v1/family/students/{studentId}/grades/periods
// — the child's ACTIVE enrollment's grades by evaluation period: the periods
// of the child's grade, and one entry per subject of the group (sorted by
// name, Spanish order), each a period id → grade object ({} when nothing is
// captured yet). A level without numeric grading answers
// gradingEnabled:false, still listing the grades captured before it stopped
// (never hidden); a child without an active enrollment is 404. Of two active
// enrollments (re-enrolled for next year) the current cycle's is shown.
//
// It runs behind RequireAuth + rotation, the module_grades + module_students
// guards and RequirePermission("family:read").
func (h *FamilyPeriodGradesHandler) PeriodGrades(w http.ResponseWriter, r *http.Request) {
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
	got, err := h.grades.GetEnrollmentPeriodGrades(r.Context(), enrollmentID)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.toResponse(r.Context(), got))
}

// activeEnrollment is the child's current enrollment id, or "" when there is
// none. A child re-enrolled for next year holds TWO active enrollments, so the
// pick is by cycle, deterministically: the enrollment whose cycle is ACTIVE;
// else the one whose cycle's dates contain today (UTC); else the active
// enrollment of the earliest-starting cycle (enrollment id breaks every tie).
func (h *FamilyPeriodGradesHandler) activeEnrollment(ctx context.Context, studentID string) (string, error) {
	enrollments, err := h.students.ListEnrollments(ctx, studentID, "", "")
	if err != nil {
		return "", err
	}
	var live []client.Enrollment
	for _, e := range enrollments {
		if e.Status == enrollmentStatusActive {
			live = append(live, e)
		}
	}
	if len(live) <= 1 {
		if len(live) == 1 {
			return live[0].ID, nil
		}
		return "", nil
	}
	cycles, err := h.cycles.ListCycles(ctx)
	if err != nil {
		return "", err
	}
	return pickCurrentEnrollment(live, cycles, h.now().UTC()), nil
}

// pickCurrentEnrollment ranks each live enrollment by its cycle (active <
// contains today < anything else), then by the cycle's start, then by id.
func pickCurrentEnrollment(live []client.Enrollment, cycles []client.CycleResult, today time.Time) string {
	byID := make(map[string]client.CycleResult, len(cycles))
	for _, c := range cycles {
		byID[c.ID] = c
	}
	rank := func(e client.Enrollment) int {
		c := byID[e.CycleID]
		switch {
		case c.Status == cycleStatusActive:
			return 0
		case !c.StartsOn.IsZero() && !c.EndsOn.IsZero() && !today.Before(c.StartsOn) && !today.After(c.EndsOn):
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(live, func(i, j int) bool {
		ri, rj := rank(live[i]), rank(live[j])
		if ri != rj {
			return ri < rj
		}
		si, sj := byID[live[i].CycleID].StartsOn, byID[live[j].CycleID].StartsOn
		if !si.Equal(sj) {
			return si.Before(sj)
		}
		return live[i].ID < live[j].ID
	})
	return live[0].ID
}

func (h *FamilyPeriodGradesHandler) toResponse(ctx context.Context, g client.EnrollmentPeriodGrades) familyPeriodGradesResponse {
	out := familyPeriodGradesResponse{
		GradingEnabled: g.GradingEnabled, GradingLetters: g.GradingLetters, CycleName: g.CycleName, Periods: toPeriodRefResponses(g.Periods),
		Subjects: []familySubjectGradesResponse{},
	}
	if g.GradingEnabled {
		scale := toGradeScaleResponse(g.Scale)
		out.Scale = &scale
	}
	names := h.subjectNames(ctx, len(g.SubjectIDs) > 0)
	bySubject := make(map[string]*familySubjectGradesResponse, len(g.SubjectIDs))
	for _, id := range g.SubjectIDs {
		bySubject[id] = &familySubjectGradesResponse{SubjectID: id, SubjectName: names[id], Grades: map[string]json.Number{}}
	}
	for _, grade := range g.Grades {
		s, ok := bySubject[grade.SubjectID]
		if !ok {
			continue
		}
		number, letter := splitGrade(grade.Value)
		if letter != nil {
			if s.Letters == nil {
				s.Letters = map[string]string{}
			}
			s.Letters[grade.PeriodID] = *letter
			continue
		}
		s.Grades[grade.PeriodID] = *number
	}
	for _, id := range g.SubjectIDs {
		out.Subjects = append(out.Subjects, *bySubject[id])
	}
	col := collate.New(language.Spanish)
	sort.SliceStable(out.Subjects, func(i, j int) bool {
		return col.CompareString(out.Subjects[i].SubjectName, out.Subjects[j].SubjectName) < 0
	})
	return out
}

// subjectNames is best-effort: a failed read leaves names empty, never fails
// the parent's screen.
func (h *FamilyPeriodGradesHandler) subjectNames(ctx context.Context, enrich bool) map[string]string {
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
