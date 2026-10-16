package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

// Sentences the period surface refuses with.
const (
	msgPeriodNotConfiguredOne  = "El grupo %s no tiene el periodo %d configurado"
	msgPeriodNotConfiguredMany = "Los grupos %s no tienen el periodo %d configurado"
	msgMissingOne              = "Falta 1 calificación por capturar"
	msgMissingMany             = "Faltan %d calificaciones por capturar"
	msgActorRequired           = "closing director is required"
	msgFilterRequired          = "Indica el periodo, el grupo o el alumno"
)

// PeriodsService closes and reopens periods and serves their report cards
// (T7). A period is one config term: one level of one cycle. Since T15 the
// term stands for the evaluation period with the SAME ordinal in each grade of
// the level (ADR 0030): its grades are the period grades of those periods.
//
//	Close: term (config) ─> level catalog, cycle, the level's evaluation
//	       periods, enrollments, names (RPCs, all BEFORE writing; any
//	       failure aborts with nothing written)
//	       ─> ONE transaction holding the term's exclusive lock: closure
//	       state, the grades under the lock, missing count (force?),
//	       report cards v = closure.version+1, supersede v-1, one
//	       GradesPublished per card QUEUED in the outbox, closure closed
//	       ─> after commit, the outbox is drained (detached, bounded); what
//	          the broker refuses stays queued for the sweep (DrainOutbox)
//
// Closure state and report cards live in this service's database, so there
// is no cross-service commit. It depends only on ports + domain.
type PeriodsService struct {
	periods     ports.PeriodRepository
	grades      ports.PeriodGradeRepository
	evalPeriods ports.EvaluationPeriodReader
	terms       ports.AssignmentReader
	catalog     ports.CatalogReader
	enrollments ports.EnrollmentReader
	students    ports.StudentDirectory
	publisher   ports.ReportCardPublisher
	tenantID    string
	newID       func() string
	now         func() time.Time
	loc         *time.Location
	log         *slog.Logger
}

// PeriodsDeps are the collaborators of a PeriodsService (a parameter object:
// eleven positional arguments would invite a swapped one). Log may be nil;
// Location is the school's time zone (nil = UTC).
type PeriodsDeps struct {
	Periods     ports.PeriodRepository
	Grades      ports.PeriodGradeRepository
	EvalPeriods ports.EvaluationPeriodReader
	Terms       ports.AssignmentReader
	Catalog     ports.CatalogReader
	Enrollments ports.EnrollmentReader
	Students    ports.StudentDirectory
	Publisher   ports.ReportCardPublisher
	TenantID    string
	NewID       func() string
	Now         func() time.Time
	Location    *time.Location
	Log         *slog.Logger
}

// NewPeriodsService wires the use case.
func NewPeriodsService(d PeriodsDeps) *PeriodsService {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	loc := d.Location
	if loc == nil {
		loc = time.UTC
	}
	return &PeriodsService{
		periods: d.Periods, grades: d.Grades, evalPeriods: d.EvalPeriods, terms: d.Terms, catalog: d.Catalog,
		enrollments: d.Enrollments, students: d.Students, publisher: d.Publisher, tenantID: d.TenantID,
		newID: d.NewID, now: d.Now, loc: loc, log: log,
	}
}

// Period is one term with its closure and capture progress. StartsOn/EndsOn
// span the term's evaluation periods across the level's grades (zero when no
// grade has one).
type Period struct {
	Term      ports.TermRef
	LevelName string
	Closure   domain.PeriodClosure
	Expected  int
	Captured  int
	StartsOn  time.Time
	EndsOn    time.Time
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

// PeriodProgress is one term's capture progress. Unfinished is true while a
// grade's period of the term has not ended (the school's local day): still
// expected. GroupsWithoutPeriod are the groups (with active pupils) whose
// grade has no period of the term's ordinal: the term cannot close.
type PeriodProgress struct {
	TermID              string
	Unfinished          bool
	EndsOn              time.Time
	GroupsWithoutPeriod []string
	Expected            int
	Captured            int
	Missing             []MissingGrades
	// ReportCards is how many report cards closing now would publish, and
	// SkippedWithoutGrades how many active pupils would get none because
	// they have no grade in the term's period (T10).
	ReportCards          int
	SkippedWithoutGrades int
}

// CloseResult is what a close answers: the report cards it published and the
// active pupils it skipped for having no grade in the term's period (T10).
type CloseResult struct {
	Status               domain.PeriodStatus
	Version              int
	ReportCards          int
	SkippedWithoutGrades int
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
	// gradeOf is each group's grade; periods are each grade's evaluation
	// periods in the cycle, by ordinal.
	gradeOf map[string]string
	periods map[string][]domain.EvaluationPeriod
	// joined is the school day each enrollment joined its group (enrolment or
	// the latest move INTO the group, whichever is later); absent = unknown,
	// so the pupil is expected in every period.
	joined map[string]time.Time
}

// termPeriod is the evaluation period of the term's ordinal in a group's
// grade; false when the grade has none.
func (v levelView) termPeriod(term ports.TermRef, groupID string) (domain.EvaluationPeriod, bool) {
	for _, p := range v.periods[v.gradeOf[groupID]] {
		if p.Ordinal == term.Ordinal {
			return p, true
		}
	}
	return domain.EvaluationPeriod{}, false
}

// columns are a group's grade's periods from the first up to the term's
// ordinal: the columns of its report cards.
func (v levelView) columns(term ports.TermRef, groupID string) []domain.EvaluationPeriod {
	out := []domain.EvaluationPeriod{}
	for _, p := range v.periods[v.gradeOf[groupID]] {
		if p.Ordinal <= term.Ordinal {
			out = append(out, p)
		}
	}
	return out
}

// columnIDs are the ids of every column of the term across the level's groups
// with active pupils (one grades read covers the whole close).
func (v levelView) columnIDs(term ports.TermRef) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, e := range v.enrollments {
		for _, p := range v.columns(term, e.GroupID) {
			if !seen[p.ID] {
				seen[p.ID] = true
				out = append(out, p.ID)
			}
		}
	}
	return out
}

// groupsWithoutPeriod lists, by label, the groups with active pupils whose
// grade has no period of the term's ordinal.
func (v levelView) groupsWithoutPeriod(term ports.TermRef) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, e := range v.enrollments {
		if _, ok := v.termPeriod(term, e.GroupID); ok || seen[e.GroupID] {
			continue
		}
		seen[e.GroupID] = true
		out = append(out, v.groupLabel[e.GroupID])
	}
	col := collate.New(language.Spanish)
	sort.Slice(out, func(i, j int) bool { return col.CompareString(out[i], out[j]) < 0 })
	return out
}

// gradedInTerm is the set of enrollments with at least one grade in the
// term's period of their OWN grade: only they get a report card (T10). A
// grade of another period, or of another grade's period, does not count.
func (v levelView) gradedInTerm(term ports.TermRef, grades []domain.PeriodGrade) map[string]bool {
	own := make(map[string]string, len(v.enrollments))
	for _, e := range v.enrollments {
		if p, ok := v.termPeriod(term, e.GroupID); ok {
			own[e.ID] = p.ID
		}
	}
	out := map[string]bool{}
	for _, g := range grades {
		if id, ok := own[g.EnrollmentID()]; ok && id == g.PeriodID() {
			out[g.EnrollmentID()] = true
		}
	}
	return out
}

// span is the earliest start and latest end of the term's period across the
// level's grades; zero times when no grade has it.
func (v levelView) span(term ports.TermRef) (start, end time.Time) {
	for _, ps := range v.periods {
		for _, p := range ps {
			if p.Ordinal != term.Ordinal {
				continue
			}
			if start.IsZero() || p.StartsOn.Before(start) {
				start = p.StartsOn
			}
			if p.EndsOn.After(end) {
				end = p.EndsOn
			}
		}
	}
	return start, end
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
	grades, err := s.grades.PeriodGradesForEnrollments(ctx, view.enrollmentIDs(), view.termPeriodIDs(term))
	if err != nil {
		return PeriodProgress{}, err
	}
	expected, captured, missing := s.tally(view, term, grades)
	_, endsOn := view.span(term)
	graded := len(view.gradedInTerm(term, grades))
	return PeriodProgress{
		TermID: term.ID, Unfinished: s.unfinished(view, term), EndsOn: endsOn,
		GroupsWithoutPeriod: view.groupsWithoutPeriod(term),
		Expected:            expected, Captured: captured, Missing: missing,
		ReportCards: graded, SkippedWithoutGrades: len(view.enrollments) - graded,
	}, nil
}

// ClosedTerms lists which of termIDs are closed now (one read), in request
// order. The gateway asks it before config changes a term's months.
func (s *PeriodsService) ClosedTerms(ctx context.Context, termIDs []string) ([]string, error) {
	closures, err := s.grades.PeriodClosures(ctx, termIDs)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, id := range termIDs {
		if c, ok := closures[id]; ok && c.IsClosed() {
			out = append(out, id)
		}
	}
	return out, nil
}

// ListPeriods lists every term of a cycle with its closure and progress,
// except the terms of a level without numeric grades: there is nothing to
// capture or close there (T10 ISSUE-019). It reads the cycle's terms, their closures, the cycle's enrollments, each
// level's catalog and periods once and every grade of the terms' periods in
// ONE query.
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
	var ids, periodIDs []string
	for _, t := range terms {
		v, ok := views[t.LevelID]
		if !ok {
			if v, err = s.levelView(ctx, cycleID, t.LevelID, enrollments); err != nil {
				return nil, err
			}
			views[t.LevelID] = v
			ids = append(ids, v.enrollmentIDs()...)
		}
		periodIDs = append(periodIDs, v.termPeriodIDs(t)...)
	}
	grades, err := s.grades.PeriodGradesForEnrollments(ctx, ids, periodIDs)
	if err != nil {
		return nil, err
	}
	out := make([]Period, 0, len(terms))
	for _, t := range terms {
		v := views[t.LevelID]
		if !v.catalog.GradingEnabled {
			continue // no numeric grades: nothing to capture or close (T10 ISSUE-019)
		}
		closure, ok := closures[t.ID]
		if !ok {
			closure = domain.NewOpenPeriod(t.ID, t.CycleID, t.LevelID)
		}
		expected, captured, _ := s.tally(v, t, grades)
		start, end := v.span(t)
		out = append(out, Period{
			Term: t, LevelName: v.catalog.LevelName, Closure: closure, Expected: expected, Captured: captured,
			StartsOn: start, EndsOn: end,
		})
	}
	return out, nil
}

// ReportCards lists the latest published report card of each enrollment
// matching filter. At least one filter is required: never the whole school.
// A pupil's list (student filter only) spans periods, so it reads newest
// period first; a term's or group's list keeps the repository's name order.
func (s *PeriodsService) ReportCards(ctx context.Context, filter ports.ReportCardFilter) ([]domain.ReportCard, error) {
	if filter.TermID == "" && filter.GroupID == "" && filter.StudentID == "" {
		return nil, glerrors.New(glerrors.KindInvalid, msgFilterRequired)
	}
	cards, err := s.periods.LatestReportCards(ctx, filter)
	if err != nil || filter.StudentID == "" {
		return cards, err
	}
	return domain.SortNewestTermFirst(cards), nil
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
	if view.catalog.Deleting {
		return CloseResult{}, levelarchive.LevelBeingDeleted()
	}
	if !view.catalog.GradingEnabled {
		return CloseResult{}, domain.Conflict(domain.CodeLevelNotGraded, msgGradingDisabled)
	}
	if without := view.groupsWithoutPeriod(term); len(without) > 0 {
		return CloseResult{}, domain.Conflict(domain.CodePeriodNotConfigured, notConfiguredMessage(without, term.Ordinal))
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
		if closed, err = current.Close(actor, s.now(), nil); err != nil {
			return err
		}
		grades, err := tx.PeriodGrades(ctx, view.enrollmentIDs(), view.columnIDs(term))
		if err != nil {
			return err
		}
		if _, _, missing := s.tally(view, term, grades); !force && totalMissing(missing) > 0 {
			return domain.Conflict(domain.CodeMissingGrades, missingMessage(totalMissing(missing)))
		}
		if issued, err = s.buildReportCards(view, term, closed, actor, names, grades); err != nil {
			return err
		}
		news, err := newsworthy(ctx, tx, issued, closed.Version())
		if err != nil {
			return err
		}
		if err := tx.IssueReportCards(ctx, issued); err != nil {
			return err
		}
		events, err := issuedEvents(news)
		if err != nil {
			return err
		}
		if err := tx.QueueEvents(ctx, events); err != nil {
			return err
		}
		return tx.SaveClosure(ctx, closed)
	})
	if err != nil {
		return CloseResult{}, err
	}
	s.drainQuietly(ctx)
	return CloseResult{Status: closed.Status(), Version: closed.Version(), ReportCards: len(issued),
		SkippedWithoutGrades: len(view.enrollments) - len(issued)}, nil
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
	if err := s.refuseDeletingLevel(ctx, term); err != nil {
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

// refuseDeletingLevel refuses a write on a period whose level is being
// deleted (LEVEL_BEING_DELETED, ADR 0021): the level's catalog carries
// config's status.
func (s *PeriodsService) refuseDeletingLevel(ctx context.Context, term ports.TermRef) error {
	catalog, err := s.catalog.LevelCatalog(ctx, term.CycleID, term.LevelID)
	if err != nil {
		return err
	}
	if catalog.Deleting {
		return levelarchive.LevelBeingDeleted()
	}
	return nil
}

// refuseClosed answers ALREADY_CLOSED before any RPC is spent on a period
// that is closed (the transaction re-checks it under the lock).
func (s *PeriodsService) refuseClosed(ctx context.Context, termID string) error {
	closures, err := s.grades.PeriodClosures(ctx, []string{termID})
	if err != nil {
		return err
	}
	if c, ok := closures[termID]; ok {
		_, err := c.Close(domain.Actor{}, s.now(), nil)
		return err
	}
	return nil
}

// DrainOutbox publishes the queued events (up to limit; 0 = drainBatch). The
// sweep calls it so nothing stays stuck when the broker was down or the
// process died between a close's commit and its drain.
func (s *PeriodsService) DrainOutbox(ctx context.Context, limit int) (int, error) {
	if s.publisher == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = drainBatch
	}
	return s.periods.DrainOutbox(ctx, limit, func(e ports.OutboxEvent) error {
		return s.publishQueued(ctx, e)
	})
}

// drainQuietly gets a close's events out right after its commit, detached
// from the request (a client that hangs up must not cancel a publication in
// flight) and bounded by drainDeadline (a dead broker costs the director a
// blink, not a wait). A failure is logged: the events stay queued.
func (s *PeriodsService) drainQuietly(ctx context.Context) {
	drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), drainDeadline)
	defer cancel()
	if _, err := s.DrainOutbox(drainCtx, 0); err != nil {
		s.log.Warn("grades could not drain the outbox after a close", slog.Any("error", err))
	}
}

// publishQueued turns one queued row back into a typed publication. An
// unknown type is refused, never dropped: its climbing attempt count is how
// somebody notices.
func (s *PeriodsService) publishQueued(ctx context.Context, e ports.OutboxEvent) error {
	switch e.Type {
	case ports.OutboxGradesPublished:
		var ev ports.ReportCardIssuedEvent
		if err := json.Unmarshal(e.Payload, &ev); err != nil {
			return fmt.Errorf("decoding a queued %s: %w", e.Type, err)
		}
		return s.publisher.PublishReportCardIssued(ctx, ev)
	default:
		return glerrors.New(glerrors.KindInternal, "unknown outbox event: "+e.Type)
	}
}

// newsworthy is the issued cards the families must hear about. A first close
// (version 1) announces every card. A re-close announces only a pupil whose
// card differs from their previous published one (SameGradesAs) or who had
// none; an unchanged pupil still gets the new version, silently.
func newsworthy(ctx context.Context, tx ports.PeriodTx, issued []domain.ReportCard, version int) ([]domain.ReportCard, error) {
	if version <= 1 {
		return issued, nil
	}
	previous, err := tx.PublishedReportCards(ctx)
	if err != nil {
		return nil, err
	}
	byStudent := make(map[string]domain.ReportCard, len(previous))
	for _, rc := range previous {
		byStudent[rc.StudentID()] = rc
	}
	out := make([]domain.ReportCard, 0, len(issued))
	for _, rc := range issued {
		if prev, ok := byStudent[rc.StudentID()]; ok && rc.SameGradesAs(prev) {
			continue
		}
		out = append(out, rc)
	}
	return out, nil
}

// issuedEvents is one queued GradesPublished per issued report card.
func issuedEvents(cards []domain.ReportCard) ([]ports.OutboxEvent, error) {
	out := make([]ports.OutboxEvent, 0, len(cards))
	for _, rc := range cards {
		h := rc.Header()
		payload, err := json.Marshal(ports.ReportCardIssuedEvent{
			ReportCardID: h.ID, TenantID: h.TenantID, StudentID: h.StudentID, EnrollmentID: h.EnrollmentID,
			CycleID: h.CycleID, TermID: h.TermID, TermName: h.TermName, Version: h.Version,
		})
		if err != nil {
			return nil, glerrors.Wrap(glerrors.KindInternal, "encoding GradesPublished", err)
		}
		out = append(out, ports.OutboxEvent{Type: ports.OutboxGradesPublished, Payload: payload})
	}
	return out, nil
}

// drainDeadline bounds the drain on the close path; drainBatch is how many
// events one drain publishes (a whole level's cards clear in a few passes).
const (
	drainDeadline = 2 * time.Second
	drainBatch    = 200
)

// --- computation ---------------------------------------------------------------

// levelView reads the level's catalog and cycle, keeps the ACTIVE
// enrollments of its groups and the month each joined its group (one transfer
// read per group that has any).
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
		gradeOf: map[string]string{}, periods: map[string][]domain.EvaluationPeriod{},
	}
	for _, g := range catalog.Groups {
		v.groupLabel[g.ID] = g.Label
		v.gradeOf[g.ID] = g.GradeLevelID
	}
	periods, err := s.evalPeriods.EvaluationPeriods(ctx, cycleID, levelID, "")
	if err != nil {
		return levelView{}, err
	}
	for _, p := range periods {
		v.periods[p.GradeLevelID] = append(v.periods[p.GradeLevelID], p)
	}
	for g := range v.periods {
		sort.SliceStable(v.periods[g], func(i, j int) bool { return v.periods[g][i].Ordinal < v.periods[g][j].Ordinal })
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
	if v.joined, err = s.joinedDays(ctx, v.enrollments); err != nil {
		return levelView{}, err
	}
	return v, nil
}

// termPeriodIDs are the ids of the term's period in each grade of the level.
func (v levelView) termPeriodIDs(term ports.TermRef) []string {
	out := []string{}
	for _, ps := range v.periods {
		for _, p := range ps {
			if p.Ordinal == term.Ordinal {
				out = append(out, p.ID)
			}
		}
	}
	sort.Strings(out)
	return out
}

// joinedDays is the school day each enrollment joined the group it sits in
// now: the later of its enrolment and its latest move INTO that group. It
// reads the moves once per group (never per pupil).
func (s *PeriodsService) joinedDays(ctx context.Context, enrollments []ports.EnrollmentRef) (map[string]time.Time, error) {
	joinedAt := make(map[string]time.Time, len(enrollments))
	groups := []string{}
	seen := map[string]bool{}
	for _, e := range enrollments {
		joinedAt[e.ID] = e.EnrolledAt
		if !seen[e.GroupID] {
			seen[e.GroupID] = true
			groups = append(groups, e.GroupID)
		}
	}
	for _, g := range groups {
		moves, err := s.enrollments.TransfersIntoGroup(ctx, g)
		if err != nil {
			return nil, err
		}
		for _, mv := range moves {
			if at, ok := joinedAt[mv.EnrollmentID]; ok && mv.MovedAt.After(at) {
				joinedAt[mv.EnrollmentID] = mv.MovedAt
			}
		}
	}
	out := make(map[string]time.Time, len(joinedAt))
	for id, at := range joinedAt {
		if !at.IsZero() {
			out[id] = domain.SchoolDay(at, s.loc)
		}
	}
	return out, nil
}

// unfinished reports whether the term's period of a grade with active pupils
// has not ended on the school's day: its grades are still expected.
func (s *PeriodsService) unfinished(v levelView, term ports.TermRef) bool {
	today := domain.SchoolDay(s.now(), s.loc)
	for _, e := range v.enrollments {
		if p, ok := v.termPeriod(term, e.GroupID); ok && !p.EndedBefore(today) {
			return true
		}
	}
	return false
}

// tally counts the expected cells of a term — active enrollment × subject
// taught in its group, one grade for its grade's period of the term's ordinal
// (a pupil who joined the group after that period ended owes none; a group
// whose grade lacks the period owes none here, and blocks the close) — how
// many are captured, and what is missing per group and subject. A level
// without numeric grades owes nothing.
func (s *PeriodsService) tally(v levelView, term ports.TermRef, grades []domain.PeriodGrade) (expected, captured int, missing []MissingGrades) {
	missing = []MissingGrades{}
	if !v.catalog.GradingEnabled {
		return 0, 0, missing
	}
	have := make(map[string]bool, len(grades))
	for _, g := range grades {
		have[g.EnrollmentID()+"|"+g.SubjectID()+"|"+g.PeriodID()] = true
	}
	perGroupSubject := map[string]*MissingGrades{}
	for _, e := range v.enrollments {
		period, ok := v.termPeriod(term, e.GroupID)
		if !ok {
			continue
		}
		if joined, known := v.joined[e.ID]; known && period.EndedBefore(joined) {
			continue
		}
		for _, subject := range v.subjects[e.GroupID] {
			expected++
			if have[e.ID+"|"+subject+"|"+period.ID] {
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

// buildReportCards freezes one report card per active enrollment of the level
// that has a grade in the term's period (T10: a pupil without one — a group
// with no subjects, say — gets no card and so no notification). Its columns are the pupil's grade's periods up to the term's ordinal; it
// lists its group's subjects plus any subject graded in those columns, in
// Spanish name order. The card is named after the pupil's period (each grade
// may name it its own way).
func (s *PeriodsService) buildReportCards(v levelView, term ports.TermRef, closed domain.PeriodClosure, actor domain.Actor,
	names map[string]ports.StudentRef, grades []domain.PeriodGrade) ([]domain.ReportCard, error) {
	graded := map[string][]string{}
	for _, g := range grades {
		graded[g.EnrollmentID()] = append(graded[g.EnrollmentID()], g.SubjectID())
	}
	inTerm := v.gradedInTerm(term, grades)
	out := make([]domain.ReportCard, 0, len(inTerm))
	for _, e := range v.enrollments {
		if !inTerm[e.ID] {
			continue
		}
		student := names[e.StudentID]
		termName := term.Name
		if p, ok := v.termPeriod(term, e.GroupID); ok {
			termName = p.Name
		}
		rc, err := domain.BuildReportCard(domain.ReportCardHeader{
			ID: s.newID(), TenantID: s.tenantID, TermID: term.ID, CycleID: term.CycleID, LevelID: term.LevelID,
			GroupID: e.GroupID, EnrollmentID: e.ID, StudentID: e.StudentID, Version: closed.Version(),
			IssuedAt: closed.ClosedAt(), IssuedBy: actor.ID, StudentName: student.Name,
			EnrollmentNumber: student.EnrollmentNumber, GroupLabel: v.groupLabel[e.GroupID],
			LevelName: v.catalog.LevelName, CycleName: v.cycle.Name, TermName: termName,
			TermOrdinal: term.Ordinal, Periods: cardPeriods(v.columns(term, e.GroupID)), Scale: v.catalog.Scale,
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

// cardPeriods are a report card's columns as it freezes them.
func cardPeriods(periods []domain.EvaluationPeriod) []domain.ReportCardPeriod {
	out := make([]domain.ReportCardPeriod, 0, len(periods))
	for _, p := range periods {
		out = append(out, domain.ReportCardPeriod{ID: p.ID, Ordinal: p.Ordinal, Name: p.Name, StartsOn: p.StartsOn})
	}
	return out
}

// notConfiguredMessage names the groups whose grade lacks the term's period.
func notConfiguredMessage(groups []string, ordinal int) string {
	if len(groups) == 1 {
		return fmt.Sprintf(msgPeriodNotConfiguredOne, groups[0], ordinal)
	}
	return fmt.Sprintf(msgPeriodNotConfiguredMany, strings.Join(groups, ", "), ordinal)
}
