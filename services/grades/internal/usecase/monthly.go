package usecase

import (
	"context"
	"sort"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

// Sentences the monthly knot refuses with (KindInvalid, shown to the user).
const (
	msgGradingDisabled   = "Este nivel no lleva calificación numérica"
	msgMonthOutsideCycle = "Ese mes no pertenece al ciclo"
	msgNotInGroup        = "El alumno no está inscrito en este grupo"
	msgCycleWithoutDates = "El ciclo escolar no tiene fechas de inicio y fin"
)

// MonthlyGradesService captures, corrects, clears and reads monthly grades
// (T6). Every write runs the same knot:
//
//	assignment (config) ─> group's level: grading enabled + scale (config)
//	  ─> enrollment active in the assignment's group and cycle (students)
//	  ─> month inside the assignment cycle's months (config)
//	  ─> ensureEditable (the ONE place a period lock will live, T7)
//	  ─> value judged on the level's scale ─> upsert/delete
//
// subject_id and teacher_id are DERIVED from the assignment; a client never
// sets them. It depends only on ports + domain.
type MonthlyGradesService struct {
	grades      ports.MonthlyGradeRepository
	assignments ports.AssignmentReader
	catalog     ports.CatalogReader
	enrollments ports.EnrollmentReader
	tenantID    string
	newID       func() string
	now         func() time.Time
}

// NewMonthlyGradesService wires the use case; newID and now are injectable so
// tests are deterministic.
func NewMonthlyGradesService(
	grades ports.MonthlyGradeRepository,
	assignments ports.AssignmentReader,
	catalog ports.CatalogReader,
	enrollments ports.EnrollmentReader,
	tenantID string,
	newID func() string,
	now func() time.Time,
) *MonthlyGradesService {
	return &MonthlyGradesService{
		grades: grades, assignments: assignments, catalog: catalog, enrollments: enrollments,
		tenantID: tenantID, newID: newID, now: now,
	}
}

// UpsertMonthlyInput is one autosaved cell: the client-supplied edges of the
// knot plus the authenticated user who typed it.
type UpsertMonthlyInput struct {
	AssignmentID string
	Month        string // "YYYY-MM"
	EnrollmentID string
	Value        string // decimal text as typed ("9.5")
	CapturedBy   string
}

// MonthlySheetRow is one active enrollment of the group and its grade for the
// month, if any.
type MonthlySheetRow struct {
	EnrollmentID string
	StudentID    string
	Value        domain.GradeValue
	HasValue     bool
	UpdatedAt    time.Time
}

// MonthlySheet is the capture grid of one assignment for one month.
type MonthlySheet struct {
	AssignmentID   string
	Month          domain.Month
	Months         []domain.Month
	Scale          domain.GradingScale
	GradingEnabled bool
	Editable       bool
	ReadOnlyReason string
	Rows           []MonthlySheetRow
}

// EnrollmentMonthlyGrades is one enrollment's monthly grades for its cycle.
type EnrollmentMonthlyGrades struct {
	CycleID        string
	CycleName      string
	Months         []domain.Month
	Scale          domain.GradingScale
	GradingEnabled bool
	SubjectIDs     []string
	Grades         []domain.MonthlyGrade
}

// assignmentContext is what the knot resolves once per request.
type assignmentContext struct {
	assignment ports.AssignmentRef
	level      ports.LevelRef
	cycle      ports.CycleRef
	months     []domain.Month
}

// Upsert records or corrects one cell. See the type doc for the knot.
func (s *MonthlyGradesService) Upsert(ctx context.Context, in UpsertMonthlyInput) (domain.MonthlyGrade, error) {
	ac, month, err := s.authorizeWrite(ctx, in.AssignmentID, in.Month, in.EnrollmentID)
	if err != nil {
		return domain.MonthlyGrade{}, err
	}
	value, err := ac.level.Scale.ParseValue(in.Value)
	if err != nil {
		return domain.MonthlyGrade{}, err
	}
	g, err := domain.NewMonthlyGrade(domain.MonthlyGradeFields{
		ID: s.newID(), TenantID: s.tenantID, EnrollmentID: in.EnrollmentID,
		SubjectID: ac.assignment.SubjectID, TeacherID: ac.assignment.TeacherID,
		CapturedBy: in.CapturedBy, Month: month, Value: value, UpdatedAt: s.now(),
	})
	if err != nil {
		return domain.MonthlyGrade{}, err
	}
	return s.grades.UpsertMonthlyGrade(ctx, g)
}

// Clear removes one cell (an emptied input). Clearing an empty cell succeeds;
// it still runs the whole knot, so nobody clears what they could not write.
func (s *MonthlyGradesService) Clear(ctx context.Context, assignmentID, month, enrollmentID string) error {
	ac, m, err := s.authorizeWrite(ctx, assignmentID, month, enrollmentID)
	if err != nil {
		return err
	}
	return s.grades.DeleteMonthlyGrade(ctx, enrollmentID, ac.assignment.SubjectID, m)
}

// Sheet builds the capture grid. An empty month means the current UTC month
// pulled into the cycle. A level without numeric grading still answers, read
// only, so the screen can say why.
func (s *MonthlyGradesService) Sheet(ctx context.Context, assignmentID, month string) (MonthlySheet, error) {
	ac, err := s.resolveAssignment(ctx, assignmentID)
	if err != nil {
		return MonthlySheet{}, err
	}
	m, err := s.sheetMonth(ac.months, month)
	if err != nil {
		return MonthlySheet{}, err
	}
	rows, err := s.sheetRows(ctx, ac.assignment, m)
	if err != nil {
		return MonthlySheet{}, err
	}
	sheet := MonthlySheet{
		AssignmentID: assignmentID, Month: m, Months: ac.months, Scale: ac.level.Scale,
		GradingEnabled: ac.level.GradingEnabled, Editable: true, Rows: rows,
	}
	if reason := s.readOnlyReason(ctx, ac, m); reason != "" {
		sheet.Editable, sheet.ReadOnlyReason = false, reason
	}
	return sheet, nil
}

// ForEnrollment reads one enrollment's monthly grades for its cycle. Subjects
// are the group's assignments plus any subject already graded (a subject whose
// assignment was removed keeps its history visible). A level without numeric
// grading answers with no subjects and no grades.
func (s *MonthlyGradesService) ForEnrollment(ctx context.Context, enrollmentID string) (EnrollmentMonthlyGrades, error) {
	enr, err := s.enrollments.ByID(ctx, enrollmentID)
	if err != nil {
		return EnrollmentMonthlyGrades{}, err
	}
	level, err := s.catalog.LevelOfGroup(ctx, enr.GroupID)
	if err != nil {
		return EnrollmentMonthlyGrades{}, err
	}
	cycle, err := s.catalog.CycleByID(ctx, enr.CycleID)
	if err != nil {
		return EnrollmentMonthlyGrades{}, err
	}
	out := EnrollmentMonthlyGrades{
		CycleID: cycle.ID, CycleName: cycle.Name, Months: domain.CycleMonths(cycle.StartsOn, cycle.EndsOn),
		Scale: level.Scale, GradingEnabled: level.GradingEnabled, SubjectIDs: []string{}, Grades: []domain.MonthlyGrade{},
	}
	if !level.GradingEnabled {
		return out, nil
	}
	assignments, err := s.catalog.AssignmentsByGroup(ctx, enr.CycleID, enr.GroupID)
	if err != nil {
		return EnrollmentMonthlyGrades{}, err
	}
	grades, err := s.grades.MonthlyGradesForEnrollment(ctx, enrollmentID)
	if err != nil {
		return EnrollmentMonthlyGrades{}, err
	}
	out.SubjectIDs = subjectsOf(assignments, grades)
	out.Grades = grades
	return out, nil
}

// --- knot --------------------------------------------------------------------

// authorizeWrite runs every check a write needs and returns the resolved
// context and month.
func (s *MonthlyGradesService) authorizeWrite(ctx context.Context, assignmentID, month, enrollmentID string) (assignmentContext, domain.Month, error) {
	ac, err := s.resolveAssignment(ctx, assignmentID)
	if err != nil {
		return assignmentContext{}, domain.Month{}, err
	}
	if !ac.level.GradingEnabled {
		return assignmentContext{}, domain.Month{}, glerrors.New(glerrors.KindInvalid, msgGradingDisabled)
	}
	if err := s.requireEnrollmentInGroup(ctx, enrollmentID, ac.assignment); err != nil {
		return assignmentContext{}, domain.Month{}, err
	}
	m, err := requireMonthInCycle(ac.months, month)
	if err != nil {
		return assignmentContext{}, domain.Month{}, err
	}
	if err := s.ensureEditable(ctx, ac, m); err != nil {
		return assignmentContext{}, domain.Month{}, err
	}
	return ac, m, nil
}

// resolveAssignment loads the assignment, its group's level and its cycle. An
// unknown assignment is KindNotFound (404 at the edge, the same answer an
// assignment the caller does not own gets).
func (s *MonthlyGradesService) resolveAssignment(ctx context.Context, assignmentID string) (assignmentContext, error) {
	asg, err := s.assignments.ByID(ctx, assignmentID)
	if err != nil {
		return assignmentContext{}, err
	}
	level, err := s.catalog.LevelOfGroup(ctx, asg.GroupID)
	if err != nil {
		return assignmentContext{}, err
	}
	cycle, err := s.catalog.CycleByID(ctx, asg.CycleID)
	if err != nil {
		return assignmentContext{}, err
	}
	return assignmentContext{
		assignment: asg, level: level, cycle: cycle,
		months: domain.CycleMonths(cycle.StartsOn, cycle.EndsOn),
	}, nil
}

// requireEnrollmentInGroup asserts the enrollment exists, is active, and sits in
// the assignment's group and cycle. Every miss reads the same sentence, so the
// answer never tells whether another group's enrollment exists.
func (s *MonthlyGradesService) requireEnrollmentInGroup(ctx context.Context, enrollmentID string, asg ports.AssignmentRef) error {
	enr, err := s.enrollments.ByID(ctx, enrollmentID)
	if err != nil && glerrors.KindOf(err) != glerrors.KindNotFound {
		return err
	}
	if err != nil || enr.GroupID != asg.GroupID || enr.CycleID != asg.CycleID || enr.Status != enrollmentStatusActive {
		return glerrors.New(glerrors.KindInvalid, msgNotInGroup)
	}
	return nil
}

func requireMonthInCycle(months []domain.Month, raw string) (domain.Month, error) {
	m, err := domain.ParseMonth(raw)
	if err != nil {
		return domain.Month{}, err
	}
	if !domain.ContainsMonth(months, m) {
		return domain.Month{}, glerrors.New(glerrors.KindInvalid, msgMonthOutsideCycle)
	}
	return m, nil
}

// ensureEditable is the single place that decides whether a month of an
// assignment still accepts corrections. Periods and closing arrive in T7; until
// then every month of the cycle is open. Both writes and the sheet's
// editable flag go through here, so the lock lands in ONE place.
func (s *MonthlyGradesService) ensureEditable(_ context.Context, _ assignmentContext, _ domain.Month) error {
	return nil
}

// readOnlyReason is why the sheet cannot be edited, or "" when it can.
func (s *MonthlyGradesService) readOnlyReason(ctx context.Context, ac assignmentContext, m domain.Month) string {
	if !ac.level.GradingEnabled {
		return msgGradingDisabled
	}
	if err := s.ensureEditable(ctx, ac, m); err != nil {
		return glerrors.SafeMessage(err)
	}
	return ""
}

// sheetMonth resolves the month a sheet shows: the requested one (inside the
// cycle) or, when omitted, the current UTC month clamped into the cycle.
func (s *MonthlyGradesService) sheetMonth(months []domain.Month, raw string) (domain.Month, error) {
	if raw != "" {
		return requireMonthInCycle(months, raw)
	}
	m, ok := domain.ClampMonth(months, s.now())
	if !ok {
		return domain.Month{}, glerrors.New(glerrors.KindInvalid, msgCycleWithoutDates)
	}
	return m, nil
}

// sheetRows joins the group's active enrollments with their grades for the
// month in ONE batched read. Rows keep enrollment-id order; the edge sorts by
// name once it has resolved them.
func (s *MonthlyGradesService) sheetRows(ctx context.Context, asg ports.AssignmentRef, m domain.Month) ([]MonthlySheetRow, error) {
	enrollments, err := s.enrollments.ByGroup(ctx, asg.GroupID)
	if err != nil {
		return nil, err
	}
	rows := make([]MonthlySheetRow, 0, len(enrollments))
	ids := make([]string, 0, len(enrollments))
	for _, e := range enrollments {
		if e.Status != enrollmentStatusActive || e.CycleID != asg.CycleID {
			continue
		}
		ids = append(ids, e.ID)
		rows = append(rows, MonthlySheetRow{EnrollmentID: e.ID, StudentID: e.StudentID})
	}
	grades, err := s.grades.MonthlyGradesForSheet(ctx, ids, asg.SubjectID, m)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if g, ok := grades[rows[i].EnrollmentID]; ok {
			rows[i].Value, rows[i].HasValue, rows[i].UpdatedAt = g.Value(), true, g.UpdatedAt()
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].EnrollmentID < rows[j].EnrollmentID })
	return rows, nil
}

// subjectsOf is the distinct subjects of the assignments, then of the grades,
// in first-seen order.
func subjectsOf(assignments []ports.AssignmentRef, grades []domain.MonthlyGrade) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, a := range assignments {
		add(a.SubjectID)
	}
	for _, g := range grades {
		add(g.SubjectID())
	}
	return out
}
