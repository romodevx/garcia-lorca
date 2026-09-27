package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

// Sentences the period surface refuses with.
const (
	msgTermWithoutMonths = "El periodo no tiene meses asignados"
	msgMissingOne        = "Falta 1 calificación por capturar"
	msgMissingMany       = "Faltan %d calificaciones por capturar"
	msgActorRequired     = "closing director is required"
	msgFilterRequired    = "Indica el periodo, el grupo o el alumno"
)

// PeriodsService closes and reopens periods and serves their report cards
// (T7). A period is one config term: one level of one cycle, a set of months.
//
//	Close: term (config) ─> level catalog, cycle, enrollments, names (RPCs,
//	       all BEFORE writing; any failure aborts with nothing written)
//	       ─> ONE transaction holding the term's exclusive lock: closure
//	       state, the grades under the lock, missing count (force?),
//	       report cards v = closure.version+1, supersede v-1, closure closed
//	       ─> after commit, GradesPublished per report card (best-effort)
//
// Closure state and report cards live in this service's database, so there
// is no cross-service commit. It depends only on ports + domain.
type PeriodsService struct {
	periods     ports.PeriodRepository
	grades      ports.MonthlyGradeRepository
	terms       ports.AssignmentReader
	catalog     ports.CatalogReader
	enrollments ports.EnrollmentReader
	students    ports.StudentDirectory
	publisher   ports.ReportCardPublisher
	tenantID    string
	newID       func() string
	now         func() time.Time
	log         *slog.Logger
}

// PeriodsDeps are the collaborators of a PeriodsService (a parameter object:
// eleven positional arguments would invite a swapped one). Log may be nil.
type PeriodsDeps struct {
	Periods     ports.PeriodRepository
	Grades      ports.MonthlyGradeRepository
	Terms       ports.AssignmentReader
	Catalog     ports.CatalogReader
	Enrollments ports.EnrollmentReader
	Students    ports.StudentDirectory
	Publisher   ports.ReportCardPublisher
	TenantID    string
	NewID       func() string
	Now         func() time.Time
	Log         *slog.Logger
}

// NewPeriodsService wires the use case.
func NewPeriodsService(d PeriodsDeps) *PeriodsService {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &PeriodsService{
		periods: d.Periods, grades: d.Grades, terms: d.Terms, catalog: d.Catalog, enrollments: d.Enrollments,
		students: d.Students, publisher: d.Publisher, tenantID: d.TenantID, newID: d.NewID, now: d.Now, log: log,
	}
}

// Period is one term with its closure and capture progress.
type Period struct {
	Term      ports.TermRef
	LevelName string
	Closure   domain.PeriodClosure
	Expected  int
	Captured  int
}

// MissingGrades is how many captures one group+subject still owes.
type MissingGrades struct {
	GroupID     string
	GroupLabel  string
	SubjectID   string
	SubjectName string
	TeacherName string
	Missing     int
}

// PeriodProgress is one term's capture progress.
type PeriodProgress struct {
	TermID   string
	Months   []domain.Month
	Expected int
	Captured int
	Missing  []MissingGrades
}

// CloseResult is what a close answers.
type CloseResult struct {
	Status      domain.PeriodStatus
	Version     int
	ReportCards int
}

// levelView is one level of one cycle as a period reads it: the catalog and
// the ACTIVE enrollments of its groups.
type levelView struct {
	catalog     ports.LevelCatalog
	cycle       ports.CycleRef
	enrollments []ports.EnrollmentRef
	groupLabel  map[string]string
	// subjects lists each group's taught subjects (first-seen order), and
	// teacher names the teacher of each group+subject.
	subjects map[string][]string
	teacher  map[string]string
}

func (v levelView) enrollmentIDs() []string {
	ids := make([]string, 0, len(v.enrollments))
	for _, e := range v.enrollments {
		ids = append(ids, e.ID)
	}
	return ids
}

// --- reads -------------------------------------------------------------------

// Progress is one term's expected and captured grades, and what is missing
// grouped by group and subject (sorted by group label, then subject name).
func (s *PeriodsService) Progress(ctx context.Context, termID string) (PeriodProgress, error) {
	term, err := s.terms.TermByID(ctx, termID)
	if err != nil {
		return PeriodProgress{}, err
	}
	enrollments, err := s.enrollments.ByCycle(ctx, term.CycleID)
	if err != nil {
		return PeriodProgress{}, err
	}
	view, err := s.levelView(ctx, term.CycleID, term.LevelID, enrollments)
	if err != nil {
		return PeriodProgress{}, err
	}
	grades, err := s.grades.MonthlyGradesForEnrollments(ctx, view.enrollmentIDs(), term.Months)
	if err != nil {
		return PeriodProgress{}, err
	}
	expected, captured, missing := s.tally(view, term, grades)
	return PeriodProgress{TermID: term.ID, Months: term.Months, Expected: expected, Captured: captured, Missing: missing}, nil
}

// ListPeriods lists every term of a cycle with its closure and progress. It
// reads the cycle's terms, their closures, the cycle's enrollments, each
// level's catalog once and every grade of the cycle's terms in ONE query.
func (s *PeriodsService) ListPeriods(ctx context.Context, cycleID string) ([]Period, error) {
	terms, err := s.terms.ListTerms(ctx, cycleID, "")
	if err != nil {
		return nil, err
	}
	termIDs := make([]string, 0, len(terms))
	for _, t := range terms {
		termIDs = append(termIDs, t.ID)
	}
	closures, err := s.grades.PeriodClosures(ctx, termIDs)
	if err != nil {
		return nil, err
	}
	enrollments, err := s.enrollments.ByCycle(ctx, cycleID)
	if err != nil {
		return nil, err
	}
	views := map[string]levelView{}
	var ids []string
	var months []domain.Month
	for _, t := range terms {
		if _, ok := views[t.LevelID]; !ok {
			v, err := s.levelView(ctx, cycleID, t.LevelID, enrollments)
			if err != nil {
				return nil, err
			}
			views[t.LevelID] = v
			ids = append(ids, v.enrollmentIDs()...)
		}
		months = appendMissing(months, t.Months)
	}
	grades, err := s.grades.MonthlyGradesForEnrollments(ctx, ids, months)
	if err != nil {
		return nil, err
	}
	out := make([]Period, 0, len(terms))
	for _, t := range terms {
		closure, ok := closures[t.ID]
		if !ok {
			closure = domain.NewOpenPeriod(t.ID, t.CycleID, t.LevelID)
		}
		v := views[t.LevelID]
		expected, captured, _ := s.tally(v, t, grades)
		out = append(out, Period{Term: t, LevelName: v.catalog.LevelName, Closure: closure, Expected: expected, Captured: captured})
	}
	return out, nil
}

// ReportCards lists the latest published report card of each enrollment
// matching filter. At least one filter is required: never the whole school.
func (s *PeriodsService) ReportCards(ctx context.Context, filter ports.ReportCardFilter) ([]domain.ReportCard, error) {
	if filter.TermID == "" && filter.GroupID == "" && filter.StudentID == "" {
		return nil, glerrors.New(glerrors.KindInvalid, msgFilterRequired)
	}
	return s.periods.LatestReportCards(ctx, filter)
}

// ReportCard reads one report card's full snapshot (any version).
func (s *PeriodsService) ReportCard(ctx context.Context, id string) (domain.ReportCard, error) {
	return s.periods.ReportCard(ctx, id)
}

// --- writes ------------------------------------------------------------------

// Close closes a term and issues its report cards (see the type doc). With
// missing captures it refuses (MISSING_GRADES) unless force is set.
func (s *PeriodsService) Close(ctx context.Context, termID string, force bool, actor domain.Actor) (CloseResult, error) {
	if strings.TrimSpace(actor.ID) == "" {
		return CloseResult{}, glerrors.New(glerrors.KindInvalid, msgActorRequired)
	}
	term, err := s.terms.TermByID(ctx, termID)
	if err != nil {
		return CloseResult{}, err
	}
	if len(term.Months) == 0 {
		return CloseResult{}, domain.Conflict(domain.CodeTermWithoutMonths, msgTermWithoutMonths)
	}
	if err := s.refuseClosed(ctx, termID); err != nil {
		return CloseResult{}, err
	}
	enrollments, err := s.enrollments.ByCycle(ctx, term.CycleID)
	if err != nil {
		return CloseResult{}, err
	}
	view, err := s.levelView(ctx, term.CycleID, term.LevelID, enrollments)
	if err != nil {
		return CloseResult{}, err
	}
	if !view.catalog.GradingEnabled {
		return CloseResult{}, domain.Conflict(domain.CodeLevelNotGraded, msgGradingDisabled)
	}
	names, err := s.students.StudentsByIDs(ctx, studentIDs(view.enrollments))
	if err != nil {
		return CloseResult{}, err
	}

	var closed domain.PeriodClosure
	var issued []domain.ReportCard
	err = s.periods.WithPeriodLock(ctx, termID, func(ctx context.Context, tx ports.PeriodTx) error {
		current, found, err := tx.Closure(ctx)
		if err != nil {
			return err
		}
		if !found {
			current = domain.NewOpenPeriod(term.ID, term.CycleID, term.LevelID)
		}
		if closed, err = current.Close(actor, s.now()); err != nil {
			return err
		}
		grades, err := tx.MonthlyGrades(ctx, view.enrollmentIDs(), term.Months)
		if err != nil {
			return err
		}
		if _, _, missing := s.tally(view, term, grades); !force && totalMissing(missing) > 0 {
			return domain.Conflict(domain.CodeMissingGrades, missingMessage(totalMissing(missing)))
		}
		if issued, err = s.buildReportCards(view, term, closed, actor, names, grades); err != nil {
			return err
		}
		if err := tx.IssueReportCards(ctx, issued); err != nil {
			return err
		}
		return tx.SaveClosure(ctx, closed)
	})
	if err != nil {
		return CloseResult{}, err
	}
	s.announce(ctx, issued)
	return CloseResult{Status: closed.Status(), Version: closed.Version(), ReportCards: len(issued)}, nil
}

// Reopen opens a closed term so its grades can be corrected. Its report cards
// stay published until the next close supersedes them. A term never closed is
// refused with ALREADY_OPEN. Who reopened it is logged (the closure keeps who
// closed it last).
func (s *PeriodsService) Reopen(ctx context.Context, termID, actorID string) (domain.PeriodStatus, error) {
	term, err := s.terms.TermByID(ctx, termID)
	if err != nil {
		return "", err
	}
	var reopened domain.PeriodClosure
	err = s.periods.WithPeriodLock(ctx, termID, func(ctx context.Context, tx ports.PeriodTx) error {
		current, found, err := tx.Closure(ctx)
		if err != nil {
			return err
		}
		if !found {
			current = domain.NewOpenPeriod(term.ID, term.CycleID, term.LevelID)
		}
		if reopened, err = current.Reopen(s.now()); err != nil {
			return err
		}
		return tx.SaveClosure(ctx, reopened)
	})
	if err != nil {
		return "", err
	}
	s.log.Info("grades period reopened",
		slog.String("term_id", termID),
		slog.String("actor_id", actorID),
		slog.Int("version", reopened.Version()))
	return reopened.Status(), nil
}

// refuseClosed answers ALREADY_CLOSED before any RPC is spent on a period
// that is closed (the transaction re-checks it under the lock).
func (s *PeriodsService) refuseClosed(ctx context.Context, termID string) error {
	closures, err := s.grades.PeriodClosures(ctx, []string{termID})
	if err != nil {
		return err
	}
	if c, ok := closures[termID]; ok {
		_, err := c.Close(domain.Actor{}, s.now())
		return err
	}
	return nil
}

// announce publishes GradesPublished for each issued report card. The cards
// are committed: a failed publish is logged, never returned (the close stands).
func (s *PeriodsService) announce(ctx context.Context, cards []domain.ReportCard) {
	if s.publisher == nil {
		return
	}
	for _, rc := range cards {
		if err := s.publisher.PublishReportCardIssued(ctx, rc); err != nil {
			s.log.Warn("grades could not announce a report card",
				slog.String("report_card_id", rc.ID()),
				slog.Int("version", rc.Version()),
				slog.Any("error", err))
		}
	}
}

// --- computation ---------------------------------------------------------------

// levelView reads the level's catalog and cycle and keeps the ACTIVE
// enrollments of its groups.
func (s *PeriodsService) levelView(ctx context.Context, cycleID, levelID string, enrollments []ports.EnrollmentRef) (levelView, error) {
	catalog, err := s.catalog.LevelCatalog(ctx, cycleID, levelID)
	if err != nil {
		return levelView{}, err
	}
	cycle, err := s.catalog.CycleByID(ctx, cycleID)
	if err != nil {
		return levelView{}, err
	}
	v := levelView{
		catalog: catalog, cycle: cycle,
		groupLabel: map[string]string{}, subjects: map[string][]string{}, teacher: map[string]string{},
	}
	for _, g := range catalog.Groups {
		v.groupLabel[g.ID] = g.Label
	}
	for _, a := range catalog.Assignments {
		key := a.GroupID + "|" + a.SubjectID
		if _, seen := v.teacher[key]; seen {
			continue
		}
		v.teacher[key] = catalog.TeacherNames[a.TeacherID]
		v.subjects[a.GroupID] = append(v.subjects[a.GroupID], a.SubjectID)
	}
	for _, e := range enrollments {
		if _, inLevel := v.groupLabel[e.GroupID]; inLevel && e.Status == enrollmentStatusActive && e.CycleID == cycleID {
			v.enrollments = append(v.enrollments, e)
		}
	}
	sort.Slice(v.enrollments, func(i, j int) bool { return v.enrollments[i].ID < v.enrollments[j].ID })
	return v, nil
}

// owedMonths are the term's months up to the current UTC month: a later
// month cannot be captured yet, so it is not owed.
func (s *PeriodsService) owedMonths(term ports.TermRef) []domain.Month {
	current := domain.MonthOf(s.now()).FirstDay()
	out := []domain.Month{}
	for _, m := range term.Months {
		if !m.FirstDay().After(current) {
			out = append(out, m)
		}
	}
	return out
}

// tally counts the expected cells of a term — active enrollment × subject
// taught in its group × owed month — how many are captured, and what is
// missing per group and subject. A level without numeric grades owes nothing.
func (s *PeriodsService) tally(v levelView, term ports.TermRef, grades []domain.MonthlyGrade) (expected, captured int, missing []MissingGrades) {
	missing = []MissingGrades{}
	if !v.catalog.GradingEnabled {
		return 0, 0, missing
	}
	have := make(map[string]bool, len(grades))
	for _, g := range grades {
		have[g.EnrollmentID()+"|"+g.SubjectID()+"|"+g.Month().String()] = true
	}
	owed := s.owedMonths(term)
	perGroupSubject := map[string]*MissingGrades{}
	for _, e := range v.enrollments {
		for _, subject := range v.subjects[e.GroupID] {
			for _, m := range owed {
				expected++
				if have[e.ID+"|"+subject+"|"+m.String()] {
					captured++
					continue
				}
				key := e.GroupID + "|" + subject
				if perGroupSubject[key] == nil {
					perGroupSubject[key] = &MissingGrades{
						GroupID: e.GroupID, GroupLabel: v.groupLabel[e.GroupID], SubjectID: subject,
						SubjectName: v.catalog.SubjectNames[subject], TeacherName: v.teacher[key],
					}
				}
				perGroupSubject[key].Missing++
			}
		}
	}
	for _, m := range perGroupSubject {
		missing = append(missing, *m)
	}
	col := collate.New(language.Spanish)
	sort.Slice(missing, func(i, j int) bool {
		if c := col.CompareString(missing[i].GroupLabel, missing[j].GroupLabel); c != 0 {
			return c < 0
		}
		return col.CompareString(missing[i].SubjectName, missing[j].SubjectName) < 0
	})
	return expected, captured, missing
}

// buildReportCards freezes one report card per active enrollment of the level.
// Each lists its group's subjects plus any subject graded in the term, in
// Spanish name order.
func (s *PeriodsService) buildReportCards(v levelView, term ports.TermRef, closed domain.PeriodClosure, actor domain.Actor,
	names map[string]ports.StudentRef, grades []domain.MonthlyGrade) ([]domain.ReportCard, error) {
	graded := map[string][]string{}
	for _, g := range grades {
		graded[g.EnrollmentID()] = append(graded[g.EnrollmentID()], g.SubjectID())
	}
	out := make([]domain.ReportCard, 0, len(v.enrollments))
	for _, e := range v.enrollments {
		student := names[e.StudentID]
		rc, err := domain.BuildReportCard(domain.ReportCardHeader{
			ID: s.newID(), TenantID: s.tenantID, TermID: term.ID, CycleID: term.CycleID, LevelID: term.LevelID,
			GroupID: e.GroupID, EnrollmentID: e.ID, StudentID: e.StudentID, Version: closed.Version(),
			IssuedAt: closed.ClosedAt(), IssuedBy: actor.ID, StudentName: student.Name,
			EnrollmentNumber: student.EnrollmentNumber, GroupLabel: v.groupLabel[e.GroupID],
			LevelName: v.catalog.LevelName, CycleName: v.cycle.Name, TermName: term.Name,
			Months: term.Months, Scale: v.catalog.Scale,
		}, s.subjectsFor(v, e.GroupID, graded[e.ID]), grades)
		if err != nil {
			return nil, err
		}
		out = append(out, rc)
	}
	return out, nil
}

func (s *PeriodsService) subjectsFor(v levelView, groupID string, graded []string) []domain.ReportCardSubject {
	ids := []string{}
	seen := map[string]bool{}
	for _, id := range append(append([]string{}, v.subjects[groupID]...), graded...) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	out := make([]domain.ReportCardSubject, 0, len(ids))
	for _, id := range ids {
		out = append(out, domain.ReportCardSubject{ID: id, Name: v.catalog.SubjectNames[id]})
	}
	col := collate.New(language.Spanish)
	sort.SliceStable(out, func(i, j int) bool { return col.CompareString(out[i].Name, out[j].Name) < 0 })
	return out
}

func studentIDs(enrollments []ports.EnrollmentRef) []string {
	out := make([]string, 0, len(enrollments))
	for _, e := range enrollments {
		out = append(out, e.StudentID)
	}
	return out
}

func totalMissing(missing []MissingGrades) int {
	n := 0
	for _, m := range missing {
		n += m.Missing
	}
	return n
}

func missingMessage(n int) string {
	if n == 1 {
		return msgMissingOne
	}
	return fmt.Sprintf(msgMissingMany, n)
}

// appendMissing appends the months of add not already in ms.
func appendMissing(ms, add []domain.Month) []domain.Month {
	for _, m := range add {
		if !domain.ContainsMonth(ms, m) {
			ms = append(ms, m)
		}
	}
	return ms
}
