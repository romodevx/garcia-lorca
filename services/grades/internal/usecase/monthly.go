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
	msgPreviousGroup     = "Ese mes corresponde al grupo anterior del alumno"
	msgMonthInFuture     = "Aún no llega ese mes"
	msgPeriodClosed      = "El periodo está cerrado"
)

// CaptureScope is on whose behalf a monthly read or write runs.
type CaptureScope uint8

const (
	// ScopeTeacher is a teacher on their own assignment: months that belong to
	// a transferred pupil's previous group are read-only. It is the zero value,
	// so forgetting to set a scope is the restrictive choice.
	ScopeTeacher CaptureScope = iota
	// ScopeSchool is the director, acting for the whole school: any month of
	// the cycle is writable.
	ScopeSchool
)

// MonthlyGradesService captures, corrects, clears and reads monthly grades
// (T6). Every write runs the same knot:
//
//	assignment (config) ─> group's level: grading enabled + scale (config)
//	  ─> enrollment active in the assignment's group and cycle (students)
//	  ─> month inside the assignment cycle's months (config) — only the
//	     months mapped into the level's terms once it has any (T7)
//	  ─> ensureEditable (future month, previous group, closed period)
//	  ─> value judged on the level's scale ─> upsert/delete, guarded by the
//	     covering term's lock (the store re-checks the closure in its tx)
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
	Scope        CaptureScope
}

// MonthlySheetRow is one active enrollment of the group and its grade for the
// month, if any. Editable is whether THIS row's cell accepts a write from the
// caller; ReadOnlyReason says why not (e.g. the month belongs to the pupil's
// previous group).
type MonthlySheetRow struct {
	EnrollmentID   string
	StudentID      string
	Value          domain.GradeValue
	HasValue       bool
	UpdatedAt      time.Time
	Editable       bool
	ReadOnlyReason string
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
	// ClosedMonths are the sheet's months that belong to a CLOSED term of the
	// level (T7): read-only for everyone until the director reopens it.
	ClosedMonths []domain.Month
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
	// termOf maps each month of the level's terms to its term id; closed
	// holds the months of the terms that are closed. Both are read once per
	// request (one config read, one closures read).
	termOf map[domain.Month]string
	closed map[domain.Month]bool
}

// lockFor is the period lock guarding a write of month m.
func (ac assignmentContext) lockFor(m domain.Month) ports.PeriodLock {
	return ports.PeriodLock{TermID: ac.termOf[m]}
}

// closedMonths lists the months of ac.months that belong to a closed term.
func (ac assignmentContext) closedMonths() []domain.Month {
	out := []domain.Month{}
	for _, m := range ac.months {
		if ac.closed[m] {
			out = append(out, m)
		}
	}
	return out
}

// editTarget is the cell (or, for the sheet-wide check, no cell) whose
// editability ensureEditable judges.
type editTarget struct {
	scope CaptureScope
	// joinedGroup is the month the pupil last moved INTO the assignment's group;
	// zero when they never moved (or when judging the whole sheet).
	joinedGroup domain.Month
}

// Upsert records or corrects one cell. See the type doc for the knot.
func (s *MonthlyGradesService) Upsert(ctx context.Context, in UpsertMonthlyInput) (domain.MonthlyGrade, error) {
	ac, month, err := s.authorizeWrite(ctx, in.Scope, in.AssignmentID, in.Month, in.EnrollmentID)
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
	return s.grades.UpsertMonthlyGrade(ctx, g, ac.lockFor(month))
}

// Clear removes one cell (an emptied input). Clearing an empty cell succeeds;
// it still runs the whole knot, so nobody clears what they could not write.
func (s *MonthlyGradesService) Clear(ctx context.Context, scope CaptureScope, assignmentID, month, enrollmentID string) error {
	ac, m, err := s.authorizeWrite(ctx, scope, assignmentID, month, enrollmentID)
	if err != nil {
		return err
	}
	return s.grades.DeleteMonthlyGrade(ctx, enrollmentID, ac.assignment.SubjectID, m, ac.lockFor(m))
}

// Sheet builds the capture grid. An empty month means the current UTC month
// pulled into the cycle. A level without numeric grading still answers, read
// only, so the screen can say why. Each row also says whether the caller may
// write it (scope decides the previous-group rule).
func (s *MonthlyGradesService) Sheet(ctx context.Context, scope CaptureScope, assignmentID, month string) (MonthlySheet, error) {
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
		GradingEnabled: ac.level.GradingEnabled, Editable: true, Rows: rows, ClosedMonths: ac.closedMonths(),
	}
	if reason := s.readOnlyReason(ctx, ac, m); reason != "" {
		sheet.Editable, sheet.ReadOnlyReason = false, reason
	}
	if err := s.markRows(ctx, scope, ac, m, &sheet); err != nil {
		return MonthlySheet{}, err
	}
	return sheet, nil
}

// markRows sets each row's Editable/ReadOnlyReason. A read-only sheet makes
// every row read-only for the same reason; otherwise each row runs
// ensureEditable with the month its pupil joined the group (ONE transfer read
// for the whole sheet).
func (s *MonthlyGradesService) markRows(ctx context.Context, scope CaptureScope, ac assignmentContext, m domain.Month, sheet *MonthlySheet) error {
	if !sheet.Editable {
		for i := range sheet.Rows {
			sheet.Rows[i].ReadOnlyReason = sheet.ReadOnlyReason
		}
		return nil
	}
	joined, err := s.joinedMonths(ctx, scope, ac.assignment.GroupID)
	if err != nil {
		return err
	}
	for i := range sheet.Rows {
		row := &sheet.Rows[i]
		err := s.ensureEditable(ctx, ac, m, editTarget{scope: scope, joinedGroup: joined[row.EnrollmentID]})
		row.Editable = err == nil
		if err != nil {
			row.ReadOnlyReason = glerrors.SafeMessage(err)
		}
	}
	return nil
}

// joinedMonths maps each enrollment that moved INTO the group to the month of
// its latest such move. The director's scope ignores moves, so it skips the
// read.
func (s *MonthlyGradesService) joinedMonths(ctx context.Context, scope CaptureScope, groupID string) (map[string]domain.Month, error) {
	out := map[string]domain.Month{}
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
		out[id] = domain.MonthOf(at)
	}
	return out, nil
}

// ForEnrollment reads one enrollment's monthly grades for its cycle. Subjects
// are the group's assignments plus any subject already graded (a subject whose
// assignment was removed keeps its history visible).
//
// Grades are returned exactly as captured: each was judged, when written, on
// the scale its level had then, and is never re-judged or hidden on read. The
// scale returned is the level's CURRENT one (grades do not record the scale
// they were captured under). A level that has stopped grading numerically
// still returns what was already captured, with only the subjects that carry a
// grade; with nothing captured it answers no subjects and no grades.
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
	grades, err := s.grades.MonthlyGradesForEnrollment(ctx, enrollmentID)
	if err != nil {
		return EnrollmentMonthlyGrades{}, err
	}
	var assignments []ports.AssignmentRef
	if level.GradingEnabled {
		if assignments, err = s.catalog.AssignmentsByGroup(ctx, enr.CycleID, enr.GroupID); err != nil {
			return EnrollmentMonthlyGrades{}, err
		}
	}
	out.SubjectIDs = subjectsOf(assignments, grades)
	if grades != nil {
		out.Grades = grades
	}
	return out, nil
}

// CountScaleViolations counts the monthly grades already captured in the
// level's ACTIVE cycle that check would refuse (every grade, when check
// disables grading). No active cycle, or a level without groups in it, counts
// 0. It reads the level's groups (config), the cycle's enrollments (students,
// one read) and counts in ONE query. The caller (the gateway, before config
// changes the scale) decides what a non-zero count means.
func (s *MonthlyGradesService) CountScaleViolations(ctx context.Context, levelID string, check ports.ScaleCheck) (int, error) {
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

// --- knot --------------------------------------------------------------------

// authorizeWrite runs every check a write needs and returns the resolved
// context and month.
func (s *MonthlyGradesService) authorizeWrite(ctx context.Context, scope CaptureScope, assignmentID, month, enrollmentID string) (assignmentContext, domain.Month, error) {
	ac, err := s.resolveAssignment(ctx, assignmentID)
	if err != nil {
		return assignmentContext{}, domain.Month{}, err
	}
	if !ac.level.GradingEnabled {
		return assignmentContext{}, domain.Month{}, domain.Refusal(domain.CodeLevelNotGraded, msgGradingDisabled)
	}
	if err := s.requireEnrollmentInGroup(ctx, enrollmentID, ac.assignment); err != nil {
		return assignmentContext{}, domain.Month{}, err
	}
	m, err := requireMonthInCycle(ac.months, month)
	if err != nil {
		return assignmentContext{}, domain.Month{}, err
	}
	joined, err := s.joinedMonths(ctx, scope, ac.assignment.GroupID)
	if err != nil {
		return assignmentContext{}, domain.Month{}, err
	}
	if err := s.ensureEditable(ctx, ac, m, editTarget{scope: scope, joinedGroup: joined[enrollmentID]}); err != nil {
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
	ac := assignmentContext{
		assignment: asg, level: level, cycle: cycle,
		months: domain.CycleMonths(cycle.StartsOn, cycle.EndsOn),
	}
	if err := s.loadPeriods(ctx, &ac); err != nil {
		return assignmentContext{}, err
	}
	return ac, nil
}

// loadPeriods reads the level's terms in the cycle (one config read) and their
// closures (one query). Once the level maps any month into a term, the months
// a sheet shows and a write accepts are only the mapped ones; a level with no
// mapping keeps every month of the cycle.
func (s *MonthlyGradesService) loadPeriods(ctx context.Context, ac *assignmentContext) error {
	ac.termOf, ac.closed = map[domain.Month]string{}, map[domain.Month]bool{}
	terms, err := s.assignments.ListTerms(ctx, ac.assignment.CycleID, ac.level.ID)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(terms))
	for _, t := range terms {
		ids = append(ids, t.ID)
		for _, m := range t.Months {
			ac.termOf[m] = t.ID
		}
	}
	if len(ac.termOf) == 0 {
		return nil
	}
	mapped := make([]domain.Month, 0, len(ac.termOf))
	for _, m := range ac.months {
		if _, ok := ac.termOf[m]; ok {
			mapped = append(mapped, m)
		}
	}
	ac.months = mapped
	closures, err := s.grades.PeriodClosures(ctx, ids)
	if err != nil {
		return err
	}
	for m, termID := range ac.termOf {
		if c, ok := closures[termID]; ok && c.IsClosed() {
			ac.closed[m] = true
		}
	}
	return nil
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
		return domain.Refusal(domain.CodeEnrollmentNotInGroup, msgNotInGroup)
	}
	return nil
}

func requireMonthInCycle(months []domain.Month, raw string) (domain.Month, error) {
	m, err := domain.ParseMonth(raw)
	if err != nil {
		return domain.Month{}, err
	}
	if !domain.ContainsMonth(months, m) {
		return domain.Month{}, domain.Refusal(domain.CodeMonthOutsideCycle, msgMonthOutsideCycle)
	}
	return m, nil
}

// ensureEditable is the single place that decides whether a month of an
// assignment still accepts corrections. Writes, the sheet's editable flag and
// each row's editable flag all go through here, so every lock lands in ONE
// place.
//
// A month of a CLOSED term of the level is read-only for everyone, teacher
// and director alike (T7): the director reopens the period to correct it.
//
// A month after the current UTC month is not graded yet, for anyone (teacher
// or director): nothing has been taught in it.
//
// It also enforces the previous-group rule: a transfer rewrites the
// enrollment's group in place, so after a move the new group's teacher would
// otherwise write the months the pupil spent in the old group. For a teacher,
// a month before the one the pupil joined the group in belongs to the previous
// group and is read-only. The director (ScopeSchool) may edit any month. The
// sheet-wide check passes a zero joinedGroup and is never refused by it.
func (s *MonthlyGradesService) ensureEditable(_ context.Context, ac assignmentContext, m domain.Month, target editTarget) error {
	if ac.closed[m] {
		return domain.Conflict(domain.CodeMonthLocked, msgPeriodClosed)
	}
	if m.FirstDay().After(domain.MonthOf(s.now()).FirstDay()) {
		return domain.Refusal(domain.CodeMonthInFuture, msgMonthInFuture)
	}
	if target.scope != ScopeSchool && !target.joinedGroup.IsZero() && m.FirstDay().Before(target.joinedGroup.FirstDay()) {
		return domain.Refusal(domain.CodeMonthPreviousGroup, msgPreviousGroup)
	}
	return nil
}

// readOnlyReason is why the sheet cannot be edited, or "" when it can.
func (s *MonthlyGradesService) readOnlyReason(ctx context.Context, ac assignmentContext, m domain.Month) string {
	if !ac.level.GradingEnabled {
		return msgGradingDisabled
	}
	if err := s.ensureEditable(ctx, ac, m, editTarget{scope: ScopeSchool}); err != nil {
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
