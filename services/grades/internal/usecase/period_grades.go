package usecase

import (
	"context"
	"sort"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

// Sentences the period-grade knot refuses with (shown to the user).
const (
	msgGradingDisabled  = "Este nivel no lleva calificación numérica"
	msgNotInGroup       = "El alumno no está inscrito en este grupo"
	msgPeriodNotInGrade = "Ese periodo no pertenece al grado del grupo"
	msgPeriodNotStarted = "Aún no empieza ese periodo"
	msgPreviousGroup    = "Ese periodo corresponde al grupo anterior del alumno"
	msgPeriodClosed     = "El periodo está cerrado"
	msgGradeWithout     = "Este grado no tiene periodos de evaluación configurados"
)

// CaptureScope is on whose behalf a grade read or write runs.
type CaptureScope uint8

const (
	// ScopeTeacher is a teacher on their own assignment: periods that belong
	// to a transferred pupil's previous group are read-only. It is the zero
	// value, so forgetting to set a scope is the restrictive choice.
	ScopeTeacher CaptureScope = iota
	// ScopeSchool is the director, acting for the whole school.
	ScopeSchool
)

// PeriodGradesService captures, corrects, clears and reads grades per
// evaluation period (T15, ADR 0030). Every write runs the same knot:
//
//	assignment (config) ─> group's level: grading enabled + scale, and the
//	  group's grade (config) ─> the grade's evaluation periods in the
//	  assignment's cycle (config): the period must be one of them
//	  ─> enrollment active in the assignment's group and cycle (students)
//	  ─> ensureEditable (closed term, period not started, previous group)
//	  ─> value judged as a WHOLE number on the level's scale ─> upsert/delete,
//	     guarded by the lock of the level term with the period's ordinal (the
//	     store re-checks the closure in its transaction)
//
// "Today" is the school's local day (SCHOOL_TIMEZONE). subject_id is DERIVED
// from the assignment; a client never sets it. It depends only on ports +
// domain.
type PeriodGradesService struct {
	grades      ports.PeriodGradeRepository
	assignments ports.AssignmentReader
	catalog     ports.CatalogReader
	periods     ports.EvaluationPeriodReader
	enrollments ports.EnrollmentReader
	tenantID    string
	newID       func() string
	now         func() time.Time
	loc         *time.Location
}

// PeriodGradesDeps are the collaborators of a PeriodGradesService. Location
// is the school's time zone (nil = UTC).
type PeriodGradesDeps struct {
	Grades      ports.PeriodGradeRepository
	Assignments ports.AssignmentReader
	Catalog     ports.CatalogReader
	Periods     ports.EvaluationPeriodReader
	Enrollments ports.EnrollmentReader
	TenantID    string
	NewID       func() string
	Now         func() time.Time
	Location    *time.Location
}

// NewPeriodGradesService wires the use case.
func NewPeriodGradesService(d PeriodGradesDeps) *PeriodGradesService {
	loc := d.Location
	if loc == nil {
		loc = time.UTC
	}
	return &PeriodGradesService{
		grades: d.Grades, assignments: d.Assignments, catalog: d.Catalog, periods: d.Periods,
		enrollments: d.Enrollments, tenantID: d.TenantID, newID: d.NewID, now: d.Now, loc: loc,
	}
}

// UpsertPeriodGradeInput is one autosaved cell: the client-supplied edges of
// the knot plus the authenticated user who typed it.
type UpsertPeriodGradeInput struct {
	AssignmentID   string
	PeriodID       string
	EnrollmentID   string
	Value          string // decimal text as typed ("9")
	CapturedBy     string
	CapturedByName string
	Scope          CaptureScope
}

// PeriodSheetRow is one active enrollment of the group and its grade for the
// period, if any. Editable is whether THIS row accepts a write from the
// caller; ReadOnlyReason says why not.
type PeriodSheetRow struct {
	EnrollmentID   string
	StudentID      string
	Value          domain.GradeValue
	HasValue       bool
	UpdatedAt      time.Time
	Editable       bool
	ReadOnlyReason string
}

// PeriodSheet is the capture grid of one assignment for one period.
type PeriodSheet struct {
	AssignmentID string
	// PeriodID is the period shown; "" when the grade has no periods.
	PeriodID        string
	Periods         []domain.EvaluationPeriod
	CurrentPeriodID string
	Today           time.Time
	Scale           domain.GradingScale
	GradingEnabled  bool
	Editable        bool
	ReadOnlyReason  string
	Rows            []PeriodSheetRow
	// ClosedPeriodIDs are the grade's periods whose level term is CLOSED:
	// read-only for everyone until the director reopens it.
	ClosedPeriodIDs []string
}

// EnrollmentPeriodGrades is one enrollment's period grades for its cycle.
type EnrollmentPeriodGrades struct {
	CycleID        string
	CycleName      string
	Periods        []domain.EvaluationPeriod
	Scale          domain.GradingScale
	GradingEnabled bool
	SubjectIDs     []string
	Grades         []domain.PeriodGrade
}

// gradeContext is what the knot resolves once per request.
type gradeContext struct {
	assignment ports.AssignmentRef
	level      ports.LevelRef
	periods    []domain.EvaluationPeriod
	today      time.Time
	// termOf maps a period ordinal to the level term of that ordinal; closed
	// holds the ids of the periods whose term is closed.
	termOf map[int]string
	closed map[string]bool
}

// lockFor is the term lock guarding a write of period p.
func (gc gradeContext) lockFor(p domain.EvaluationPeriod) ports.PeriodLock {
	return ports.PeriodLock{TermID: gc.termOf[p.Ordinal]}
}

// closedIDs lists the closed periods in ordinal order.
func (gc gradeContext) closedIDs() []string {
	out := []string{}
	for _, p := range gc.periods {
		if gc.closed[p.ID] {
			out = append(out, p.ID)
		}
	}
	return out
}

// Upsert records or corrects one cell. See the type doc for the knot.
func (s *PeriodGradesService) Upsert(ctx context.Context, in UpsertPeriodGradeInput) (domain.PeriodGrade, error) {
	gc, period, err := s.authorizeWrite(ctx, in.Scope, in.AssignmentID, in.PeriodID, in.EnrollmentID)
	if err != nil {
		return domain.PeriodGrade{}, err
	}
	value, err := gc.level.Scale.ParseWholeValue(in.Value)
	if err != nil {
		return domain.PeriodGrade{}, err
	}
	g, err := domain.NewPeriodGrade(domain.PeriodGradeFields{
		ID: s.newID(), TenantID: s.tenantID, EnrollmentID: in.EnrollmentID, SubjectID: gc.assignment.SubjectID,
		PeriodID: period.ID, CapturedBy: in.CapturedBy, CapturedByName: in.CapturedByName,
		Value: value, UpdatedAt: s.now(),
	})
	if err != nil {
		return domain.PeriodGrade{}, err
	}
	return s.grades.UpsertPeriodGrade(ctx, g, gc.lockFor(period))
}

// Clear removes one cell (an emptied input). Clearing an empty cell succeeds;
// it still runs the whole knot, so nobody clears what they could not write.
func (s *PeriodGradesService) Clear(ctx context.Context, scope CaptureScope, assignmentID, periodID, enrollmentID string) error {
	gc, period, err := s.authorizeWrite(ctx, scope, assignmentID, periodID, enrollmentID)
	if err != nil {
		return err
	}
	return s.grades.DeletePeriodGrade(ctx, enrollmentID, gc.assignment.SubjectID, period.ID, gc.lockFor(period))
}

// Sheet builds the capture grid. An empty periodID opens the period of the
// school's day (domain.DefaultPeriod). A level without numeric grading, or a
// grade without periods, still answers, read only, so the screen can say why.
func (s *PeriodGradesService) Sheet(ctx context.Context, scope CaptureScope, assignmentID, periodID string) (PeriodSheet, error) {
	gc, err := s.resolveAssignment(ctx, assignmentID)
	if err != nil {
		return PeriodSheet{}, err
	}
	sheet := PeriodSheet{
		AssignmentID: assignmentID, Periods: gc.periods, Today: gc.today, Scale: gc.level.Scale,
		GradingEnabled: gc.level.GradingEnabled, Editable: true, ClosedPeriodIDs: gc.closedIDs(),
	}
	if current, ok := domain.PeriodOn(gc.periods, gc.today); ok {
		sheet.CurrentPeriodID = current.ID
	}
	period, hasPeriod, err := sheetPeriod(gc, periodID)
	if err != nil {
		return PeriodSheet{}, err
	}
	if sheet.Rows, err = s.sheetRows(ctx, gc.assignment, period.ID); err != nil {
		return PeriodSheet{}, err
	}
	sheet.PeriodID = period.ID
	switch {
	case !gc.level.GradingEnabled:
		sheet.Editable, sheet.ReadOnlyReason = false, msgGradingDisabled
	case !hasPeriod:
		sheet.Editable, sheet.ReadOnlyReason = false, msgGradeWithout
	default:
		if err := s.ensureEditable(gc, period, editTarget{scope: ScopeSchool}); err != nil {
			sheet.Editable, sheet.ReadOnlyReason = false, glerrors.SafeMessage(err)
		}
	}
	if err := s.markRows(ctx, scope, gc, period, &sheet); err != nil {
		return PeriodSheet{}, err
	}
	return sheet, nil
}

// sheetPeriod is the period a sheet shows: the requested one (of the grade)
// or the default for the school's day. hasPeriod is false for a grade without
// periods.
func sheetPeriod(gc gradeContext, periodID string) (domain.EvaluationPeriod, bool, error) {
	if periodID != "" {
		p, err := requirePeriodOfGrade(gc.periods, periodID)
		return p, err == nil, err
	}
	p, ok := domain.DefaultPeriod(gc.periods, gc.today)
	return p, ok, nil
}

// markRows sets each row's Editable/ReadOnlyReason. A read-only sheet makes
// every row read-only for the same reason; otherwise each row runs
// ensureEditable with the day its pupil joined the group (ONE transfer read
// for the whole sheet).
func (s *PeriodGradesService) markRows(ctx context.Context, scope CaptureScope, gc gradeContext, period domain.EvaluationPeriod, sheet *PeriodSheet) error {
	if !sheet.Editable {
		for i := range sheet.Rows {
			sheet.Rows[i].ReadOnlyReason = sheet.ReadOnlyReason
		}
		return nil
	}
	joined, err := s.joinedDays(ctx, scope, gc.assignment.GroupID)
	if err != nil {
		return err
	}
	for i := range sheet.Rows {
		row := &sheet.Rows[i]
		err := s.ensureEditable(gc, period, editTarget{scope: scope, joinedGroup: joined[row.EnrollmentID]})
		row.Editable = err == nil
		if err != nil {
			row.ReadOnlyReason = glerrors.SafeMessage(err)
		}
	}
	return nil
}

// joinedDays maps each enrollment that moved INTO the group to the school day
// of its latest such move. The director's scope ignores moves, so it skips
// the read.
func (s *PeriodGradesService) joinedDays(ctx context.Context, scope CaptureScope, groupID string) (map[string]time.Time, error) {
	out := map[string]time.Time{}
	if scope == ScopeSchool {
		return out, nil
	}
	moves, err := s.enrollments.TransfersIntoGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	latest := map[string]time.Time{}
	for _, mv := range moves {
		if mv.MovedAt.After(latest[mv.EnrollmentID]) {
			latest[mv.EnrollmentID] = mv.MovedAt
		}
	}
	for id, at := range latest {
		out[id] = domain.SchoolDay(at, s.loc)
	}
	return out, nil
}

// ForEnrollment reads one enrollment's period grades for its cycle: the
// periods of its group's grade, the level's CURRENT scale, the subjects (the
// group's assignments plus any subject already graded) and every grade, as
// captured — never re-judged or hidden. A level that stopped grading
// numerically still returns what was captured, with only the graded subjects.
func (s *PeriodGradesService) ForEnrollment(ctx context.Context, enrollmentID string) (EnrollmentPeriodGrades, error) {
	enr, err := s.enrollments.ByID(ctx, enrollmentID)
	if err != nil {
		return EnrollmentPeriodGrades{}, err
	}
	level, err := s.catalog.LevelOfGroup(ctx, enr.GroupID)
	if err != nil {
		return EnrollmentPeriodGrades{}, err
	}
	cycle, err := s.catalog.CycleByID(ctx, enr.CycleID)
	if err != nil {
		return EnrollmentPeriodGrades{}, err
	}
	periods, err := s.gradePeriods(ctx, enr.CycleID, level.GradeLevelID)
	if err != nil {
		return EnrollmentPeriodGrades{}, err
	}
	grades, err := s.grades.PeriodGradesForEnrollment(ctx, enrollmentID)
	if err != nil {
		return EnrollmentPeriodGrades{}, err
	}
	var assignments []ports.AssignmentRef
	if level.GradingEnabled {
		if assignments, err = s.catalog.AssignmentsByGroup(ctx, enr.CycleID, enr.GroupID); err != nil {
			return EnrollmentPeriodGrades{}, err
		}
	}
	out := EnrollmentPeriodGrades{
		CycleID: cycle.ID, CycleName: cycle.Name, Periods: periods, Scale: level.Scale,
		GradingEnabled: level.GradingEnabled, SubjectIDs: subjectsOf(assignments, grades), Grades: grades,
	}
	if out.Grades == nil {
		out.Grades = []domain.PeriodGrade{}
	}
	return out, nil
}

// CountScaleViolations counts the period grades already captured in the
// level's ACTIVE cycle that check would refuse (every grade, when check
// disables grading). No active cycle, or a level without groups in it, counts
// 0. The caller (the gateway, before config changes the scale) decides what a
// non-zero count means.
func (s *PeriodGradesService) CountScaleViolations(ctx context.Context, levelID string, check ports.ScaleCheck) (int, error) {
	cycle, ok, err := s.catalog.ActiveCycle(ctx)
	if err != nil || !ok {
		return 0, err
	}
	groups, err := s.catalog.GroupsOfLevel(ctx, cycle.ID, levelID)
	if err != nil || len(groups) == 0 {
		return 0, err
	}
	inLevel := make(map[string]bool, len(groups))
	for _, g := range groups {
		inLevel[g] = true
	}
	enrollments, err := s.enrollments.ByCycle(ctx, cycle.ID)
	if err != nil {
		return 0, err
	}
	ids := make([]string, 0, len(enrollments))
	for _, e := range enrollments {
		if inLevel[e.GroupID] {
			ids = append(ids, e.ID)
		}
	}
	return s.grades.CountScaleViolations(ctx, ids, check)
}

// PeriodsWithGrades answers which of periodIDs hold at least one grade (one
// read). The gateway asks before config removes a period.
func (s *PeriodGradesService) PeriodsWithGrades(ctx context.Context, periodIDs []string) ([]string, error) {
	if len(periodIDs) == 0 {
		return []string{}, nil
	}
	return s.grades.PeriodsWithGrades(ctx, periodIDs)
}

// --- knot --------------------------------------------------------------------

// editTarget is the cell (or, for the sheet-wide check, no cell) whose
// editability ensureEditable judges.
type editTarget struct {
	scope CaptureScope
	// joinedGroup is the school day the pupil last moved INTO the
	// assignment's group; zero when they never moved (or for the whole sheet).
	joinedGroup time.Time
}

// authorizeWrite runs every check a write needs and returns the resolved
// context and period.
func (s *PeriodGradesService) authorizeWrite(ctx context.Context, scope CaptureScope, assignmentID, periodID, enrollmentID string) (gradeContext, domain.EvaluationPeriod, error) {
	gc, err := s.resolveAssignment(ctx, assignmentID)
	if err != nil {
		return gradeContext{}, domain.EvaluationPeriod{}, err
	}
	if gc.level.Deleting {
		return gradeContext{}, domain.EvaluationPeriod{}, levelarchive.LevelBeingDeleted()
	}
	if !gc.level.GradingEnabled {
		return gradeContext{}, domain.EvaluationPeriod{}, domain.Refusal(domain.CodeLevelNotGraded, msgGradingDisabled)
	}
	if err := s.requireEnrollmentInGroup(ctx, enrollmentID, gc.assignment); err != nil {
		return gradeContext{}, domain.EvaluationPeriod{}, err
	}
	period, err := requirePeriodOfGrade(gc.periods, periodID)
	if err != nil {
		return gradeContext{}, domain.EvaluationPeriod{}, err
	}
	joined, err := s.joinedDays(ctx, scope, gc.assignment.GroupID)
	if err != nil {
		return gradeContext{}, domain.EvaluationPeriod{}, err
	}
	if err := s.ensureEditable(gc, period, editTarget{scope: scope, joinedGroup: joined[enrollmentID]}); err != nil {
		return gradeContext{}, domain.EvaluationPeriod{}, err
	}
	return gc, period, nil
}

// resolveAssignment loads the assignment, its group's level and grade, the
// grade's periods in the cycle and their terms' closures. An unknown
// assignment is KindNotFound (404 at the edge, the same answer an assignment
// the caller does not own gets).
func (s *PeriodGradesService) resolveAssignment(ctx context.Context, assignmentID string) (gradeContext, error) {
	asg, err := s.assignments.ByID(ctx, assignmentID)
	if err != nil {
		return gradeContext{}, err
	}
	level, err := s.catalog.LevelOfGroup(ctx, asg.GroupID)
	if err != nil {
		return gradeContext{}, err
	}
	periods, err := s.gradePeriods(ctx, asg.CycleID, level.GradeLevelID)
	if err != nil {
		return gradeContext{}, err
	}
	gc := gradeContext{assignment: asg, level: level, periods: periods, today: domain.SchoolDay(s.now(), s.loc)}
	if err := s.loadClosures(ctx, &gc); err != nil {
		return gradeContext{}, err
	}
	return gc, nil
}

// gradePeriods reads one grade's evaluation periods in a cycle, by ordinal.
// A group without a grade has none.
func (s *PeriodGradesService) gradePeriods(ctx context.Context, cycleID, gradeLevelID string) ([]domain.EvaluationPeriod, error) {
	if gradeLevelID == "" {
		return []domain.EvaluationPeriod{}, nil
	}
	periods, err := s.periods.EvaluationPeriods(ctx, cycleID, "", gradeLevelID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(periods, func(i, j int) bool { return periods[i].Ordinal < periods[j].Ordinal })
	return periods, nil
}

// loadClosures maps the level's terms by ordinal (one config read) and marks
// the periods whose term is closed (one closures read). A period maps to the
// term of the same cycle, level and ordinal (ADR 0029/0030).
func (s *PeriodGradesService) loadClosures(ctx context.Context, gc *gradeContext) error {
	gc.termOf, gc.closed = map[int]string{}, map[string]bool{}
	if len(gc.periods) == 0 {
		return nil
	}
	terms, err := s.assignments.ListTerms(ctx, gc.assignment.CycleID, gc.level.ID)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(terms))
	for _, t := range terms {
		gc.termOf[t.Ordinal] = t.ID
		ids = append(ids, t.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	closures, err := s.grades.PeriodClosures(ctx, ids)
	if err != nil {
		return err
	}
	for _, p := range gc.periods {
		if c, ok := closures[gc.termOf[p.Ordinal]]; ok && c.IsClosed() {
			gc.closed[p.ID] = true
		}
	}
	return nil
}

// requireEnrollmentInGroup asserts the enrollment exists, is active, and sits in
// the assignment's group and cycle. Every miss reads the same sentence, so the
// answer never tells whether another group's enrollment exists.
func (s *PeriodGradesService) requireEnrollmentInGroup(ctx context.Context, enrollmentID string, asg ports.AssignmentRef) error {
	enr, err := s.enrollments.ByID(ctx, enrollmentID)
	if err != nil && glerrors.KindOf(err) != glerrors.KindNotFound {
		return err
	}
	if err != nil || enr.GroupID != asg.GroupID || enr.CycleID != asg.CycleID || enr.Status != enrollmentStatusActive {
		return domain.Refusal(domain.CodeEnrollmentNotInGroup, msgNotInGroup)
	}
	return nil
}

// requirePeriodOfGrade finds periodID among the grade's periods.
func requirePeriodOfGrade(periods []domain.EvaluationPeriod, periodID string) (domain.EvaluationPeriod, error) {
	p, ok := domain.FindPeriod(periods, periodID)
	if !ok {
		return domain.EvaluationPeriod{}, domain.Refusal(domain.CodePeriodNotInGrade, msgPeriodNotInGrade)
	}
	return p, nil
}

// ensureEditable is the single place that decides whether a period of an
// assignment accepts corrections. Writes, the sheet's editable flag and each
// row's editable flag all go through here.
//
//   - A period whose level term is CLOSED is read-only for everyone, teacher
//     and director alike: the director reopens the term to correct it.
//   - A period that has not started on the school's local day is not graded
//     yet, for anyone. A finished period stays writable until its term closes.
//   - The previous-group rule: a transfer rewrites the enrollment's group in
//     place, so for a teacher a period that ENDED before the pupil joined the
//     group belongs to the previous group and is read-only. The director
//     (ScopeSchool) may edit any period.
func (s *PeriodGradesService) ensureEditable(gc gradeContext, period domain.EvaluationPeriod, target editTarget) error {
	if gc.closed[period.ID] {
		return domain.Conflict(domain.CodePeriodLocked, msgPeriodClosed)
	}
	if !period.StartedBy(gc.today) {
		return domain.Refusal(domain.CodePeriodNotStarted, msgPeriodNotStarted)
	}
	if target.scope != ScopeSchool && !target.joinedGroup.IsZero() && period.EndedBefore(target.joinedGroup) {
		return domain.Refusal(domain.CodePeriodPreviousGroup, msgPreviousGroup)
	}
	return nil
}

// sheetRows joins the group's active enrollments with their grades for the
// period in ONE batched read (none without a period). Rows keep enrollment-id
// order; the edge sorts by name once it has resolved them.
func (s *PeriodGradesService) sheetRows(ctx context.Context, asg ports.AssignmentRef, periodID string) ([]PeriodSheetRow, error) {
	enrollments, err := s.enrollments.ByGroup(ctx, asg.GroupID)
	if err != nil {
		return nil, err
	}
	rows := make([]PeriodSheetRow, 0, len(enrollments))
	ids := make([]string, 0, len(enrollments))
	for _, e := range enrollments {
		if e.Status != enrollmentStatusActive || e.CycleID != asg.CycleID {
			continue
		}
		ids = append(ids, e.ID)
		rows = append(rows, PeriodSheetRow{EnrollmentID: e.ID, StudentID: e.StudentID})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].EnrollmentID < rows[j].EnrollmentID })
	if periodID == "" {
		return rows, nil
	}
	grades, err := s.grades.PeriodGradesForSheet(ctx, ids, asg.SubjectID, periodID)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if g, ok := grades[rows[i].EnrollmentID]; ok {
			rows[i].Value, rows[i].HasValue, rows[i].UpdatedAt = g.Value(), true, g.UpdatedAt()
		}
	}
	return rows, nil
}

// subjectsOf is the distinct subjects of the assignments, then of the grades,
// in first-seen order.
func subjectsOf(assignments []ports.AssignmentRef, grades []domain.PeriodGrade) []string {
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
