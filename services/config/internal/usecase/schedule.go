package usecase

import (
	"context"
	"sort"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
)

// refusal is the domain's coded refusal, for the use case's own checks.
var refusal = domain.ScheduleRefusal

// The history's default and largest page.
const (
	defaultScheduleEvents = 100
	maxScheduleEvents     = 500
)

// ScheduleService is the school timetable (T16, ADR 0023): a base weekly
// timetable per cycle built from the cycle's teaching assignments, and
// date-specific changes that never touch the base. A teacher is never in two
// groups at once and a group never has two classes at once — on the base
// (and the database enforces it there too) and on the effective timetable of
// every date that has changes. Every write runs under the cycle's timetable
// lock (ports.ScheduleRepository.RunScheduleTx), so those checks cannot race.
//
// "Today" is the school's date (loc): it decides whether the cycle has
// started — from then on base edits need a reason and go to the history —
// and which recorded changes are still ahead.
type ScheduleService struct {
	repo   ports.ScheduleRepository
	tenant string
	newID  func() string
	now    func() time.Time
	loc    *time.Location
}

// NewScheduleService wires the use case; loc is the school's time zone.
func NewScheduleService(repo ports.ScheduleRepository, tenant string, newID func() string, now func() time.Time, loc *time.Location) *ScheduleService {
	return &ScheduleService{repo: repo, tenant: tenant, newID: newID, now: now, loc: loc}
}

// BlockInput places a teaching assignment on a weekday and time (minutes since
// midnight). Reason is required once the cycle has started.
type BlockInput struct {
	AssignmentID string
	Weekday      int
	Start        int
	End          int
	Reason       string
}

// OverrideInput is a change of one class on one date. ReplaceOverrideID
// names a change of the same class that date (a substitute or a cancellation,
// never a swap) to replace in the same step — T19: the substitute became
// absent and the class needs another one.
type OverrideInput struct {
	BlockID           string
	Date              time.Time
	Kind              domain.OverrideKind
	TeacherID         string
	SwapBlockID       string
	Reason            string
	ReplaceOverrideID string
}

// AvailabilityQuery asks who is free for a class of GroupID/SubjectID between
// Start and End: on Date (effective timetable) when it is set, otherwise in
// the base timetable of Weekday. ExcludeBlockID leaves that class out of the
// busy check.
type AvailabilityQuery struct {
	CycleID        string
	GroupID        string
	SubjectID      string
	Weekday        int
	Date           time.Time
	Start          int
	End            int
	ExcludeBlockID string
}

// ScheduleBoard is the base timetable of a cycle with what it is drawn with.
type ScheduleBoard struct {
	Catalog ports.ScheduleCatalog
	Cycles  []domain.Cycle
	Started bool
	// Grids holds one grid per level of the catalog: the saved one or the default.
	Grids  []domain.ScheduleGrid
	Blocks []domain.ScheduleBlock
}

// ScheduleDay is the effective timetable of one date. NonWorking is the label
// of the school-calendar period that closes the date ("" on a school day): a
// closed date has no classes.
type ScheduleDay struct {
	Date       time.Time
	Weekday    int
	NonWorking string
	Classes    []domain.ScheduledClass
}

// today is the school's calendar date, as midnight UTC like every date here.
func (s *ScheduleService) today() time.Time {
	n := s.now().In(s.loc)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

func (s *ScheduleService) started(c domain.Cycle) bool { return !s.today().Before(c.StartsOn()) }

// Board returns the base timetable of cycleID — the active cycle when it is
// empty — narrowed to teacherID's classes when that is set.
func (s *ScheduleService) Board(ctx context.Context, cycleID, teacherID string) (ScheduleBoard, error) {
	cycles, err := s.repo.ListCycles(ctx)
	if err != nil {
		return ScheduleBoard{}, err
	}
	if strings.TrimSpace(cycleID) == "" {
		cycleID = activeCycleID(cycles)
		if cycleID == "" {
			return ScheduleBoard{}, glerrors.New(glerrors.KindNotFound, "No hay un ciclo activo")
		}
	}
	cat, err := s.repo.LoadScheduleCatalog(ctx, cycleID)
	if err != nil {
		return ScheduleBoard{}, err
	}
	blocks, err := s.repo.ListScheduleBlocks(ctx, cycleID)
	if err != nil {
		return ScheduleBoard{}, err
	}
	if teacherID != "" {
		blocks = filterBlocks(blocks, func(b domain.ScheduleBlock) bool { return b.TeacherID() == teacherID })
	}
	return ScheduleBoard{
		Catalog: cat, Cycles: cycles, Started: s.started(cat.Cycle),
		Grids: gridsOf(cat), Blocks: blocks,
	}, nil
}

// Day is the effective timetable of date in cycleID — the base, that date's
// changes and the teachers' absences (T19: a class of an absent teacher is
// ClassUncovered) — with teacherID set, the classes that teacher gives that
// day (covers included) or was replaced in.
func (s *ScheduleService) Day(ctx context.Context, cycleID string, date time.Time, teacherID string) (ScheduleDay, error) {
	if strings.TrimSpace(cycleID) == "" {
		return ScheduleDay{}, glerrors.New(glerrors.KindInvalid, "cycle id is required")
	}
	cycle, err := s.repo.GetCycle(ctx, cycleID)
	if err != nil {
		return ScheduleDay{}, err
	}
	date = asDate(date)
	day := ScheduleDay{Date: date, Weekday: domain.WeekdayOf(date)}
	t, err := s.loadCalendar(ctx, cycle)
	if err != nil {
		return ScheduleDay{}, err
	}
	if open, closed := t.schoolDay(date); !open {
		day.NonWorking = closed
		return day, nil
	}
	if err := s.loadClasses(ctx, &t, date, date); err != nil {
		return ScheduleDay{}, err
	}
	for _, c := range t.on(date) {
		if teacherID == "" || c.TeacherID == teacherID || c.Block.TeacherID() == teacherID {
			day.Classes = append(day.Classes, c)
		}
	}
	return day, nil
}

// CreateBlock adds a class to the base timetable.
func (s *ScheduleService) CreateBlock(ctx context.Context, in BlockInput, by domain.Actor) (domain.ScheduleBlock, error) {
	p, err := s.placement(ctx, s.newID(), in)
	if err != nil {
		return domain.ScheduleBlock{}, err
	}
	p.by = by
	err = s.repo.RunScheduleTx(ctx, p.block.CycleID(), func(tx ports.ScheduleTx) error {
		blocks, err := tx.Blocks(p.block.CycleID())
		if err != nil {
			return err
		}
		if err := s.checkPlacement(tx, blocks, p.block, p.names); err != nil {
			return err
		}
		if err := tx.InsertBlock(p.block); err != nil {
			return err
		}
		return s.recordBase(tx, p, domain.ActionBlockCreated, domain.BlockSummary("Clase nueva", p.block, p.names))
	})
	if err != nil {
		return domain.ScheduleBlock{}, err
	}
	return p.block, nil
}

// UpdateBlock moves or changes a class of the base timetable. A class with
// changes on dates still ahead keeps its weekday and assignment.
func (s *ScheduleService) UpdateBlock(ctx context.Context, id string, in BlockInput, by domain.Actor) (domain.ScheduleBlock, error) {
	p, err := s.placement(ctx, id, in)
	if err != nil {
		return domain.ScheduleBlock{}, err
	}
	p.by = by
	err = s.repo.RunScheduleTx(ctx, p.block.CycleID(), func(tx ports.ScheduleTx) error {
		before, err := tx.GetBlock(id)
		if err != nil {
			return err
		}
		if before.CycleID() != p.block.CycleID() {
			return refuseAssignment("La asignación es de otro ciclo")
		}
		if before.Weekday() != p.block.Weekday() || before.AssignmentID() != p.block.AssignmentID() {
			if err := s.refuseIfAhead(tx, before); err != nil {
				return err
			}
		}
		blocks, err := tx.Blocks(p.block.CycleID())
		if err != nil {
			return err
		}
		if err := s.checkPlacement(tx, blocks, p.block, p.names); err != nil {
			return err
		}
		if err := tx.UpdateBlock(p.block); err != nil {
			return err
		}
		return s.recordBase(tx, p, domain.ActionBlockUpdated, domain.BlockChangeSummary(before, p.block, p.names))
	})
	if err != nil {
		return domain.ScheduleBlock{}, err
	}
	return p.block, nil
}

// DeleteBlock removes a class from the base timetable, with its changes —
// unless dropping them would double-book someone on a date ahead.
func (s *ScheduleService) DeleteBlock(ctx context.Context, id, reason string, by domain.Actor) error {
	b, cat, err := s.blockAndCatalog(ctx, id)
	if err != nil {
		return err
	}
	started := s.started(cat.Cycle)
	reason, err = domain.ScheduleReason(reason, started)
	if err != nil {
		return err
	}
	names := newScheduleNames(cat)
	return s.repo.RunScheduleTx(ctx, b.CycleID(), func(tx ports.ScheduleTx) error {
		current, err := tx.GetBlock(id)
		if err != nil {
			return err
		}
		undone, err := s.checkRemoval(tx, current, names)
		if err != nil {
			return err
		}
		blocks, err := tx.Blocks(current.CycleID())
		if err != nil {
			return err
		}
		if err := tx.DeleteBlock(id); err != nil {
			return err
		}
		// The class's changes ahead go with it (FK cascade): each undo is
		// recorded, as every override and its undo always are.
		for _, o := range undone {
			if err := tx.AppendEvent(s.event(cat.Cycle.ID(), by, domain.ActionOverrideDeleted, o.BlockID(), o.ID(), o.Date(), reason,
				"Se deshizo al quitar la clase — "+overrideSummaryIn(blocks, o, names))); err != nil {
				return err
			}
		}
		if !started {
			return nil
		}
		return tx.AppendEvent(s.event(cat.Cycle.ID(), by, domain.ActionBlockDeleted, id, "", time.Time{}, reason,
			domain.BlockSummary("Clase quitada", current, names)))
	})
}

// CreateOverride changes one class on one date, judged on that date's
// effective timetable.
func (s *ScheduleService) CreateOverride(ctx context.Context, in OverrideInput, by domain.Actor) (domain.ScheduleOverride, error) {
	b, cat, err := s.blockAndCatalog(ctx, in.BlockID)
	if err != nil {
		return domain.ScheduleOverride{}, err
	}
	date := asDate(in.Date)
	calendar, err := s.loadCalendar(ctx, cat.Cycle)
	if err != nil {
		return domain.ScheduleOverride{}, err
	}
	if err := checkOverrideDate(calendar, b, date); err != nil {
		return domain.ScheduleOverride{}, err
	}
	if in.Kind == domain.OverrideSubstitute {
		if err := checkSubstitute(cat, b, in.TeacherID); err != nil {
			return domain.ScheduleOverride{}, err
		}
	}
	o, err := domain.NewScheduleOverride(s.newID(), s.tenant, b.CycleID(), b.ID(), date, in.Kind, in.TeacherID, in.SwapBlockID,
		in.Reason, by, s.now().UTC())
	if err != nil {
		return domain.ScheduleOverride{}, err
	}
	names := newScheduleNames(cat)
	err = s.repo.RunScheduleTx(ctx, b.CycleID(), func(tx ports.ScheduleTx) error {
		// The class was read before the lock: it must still be the same one.
		current, err := tx.GetBlock(b.ID())
		if err != nil {
			return err
		}
		if current != b {
			return domain.ScheduleChangedMeanwhile()
		}
		partner, err := s.swapPartner(tx, b, o)
		if err != nil {
			return err
		}
		sameDay, err := tx.OverridesOn(b.CycleID(), date)
		if err != nil {
			return err
		}
		sameDay, replaced, err := withoutReplaced(sameDay, b, in.ReplaceOverrideID)
		if err != nil {
			return err
		}
		for _, other := range sameDay {
			if other.Touches(b.ID()) || (partner != nil && other.Touches(partner.ID())) {
				return refusal(glerrors.KindConflict, domain.CodeScheduleBlockTaken,
					"Esa clase ya tiene un cambio el "+domain.FormatSpanishDate(date)+"; deshazlo primero")
			}
		}
		blocks, err := tx.Blocks(b.CycleID())
		if err != nil {
			return err
		}
		if err := clashFreeDay(blocks, append(sameDay, o), date, o, names); err != nil {
			return err
		}
		if err := refuseAbsentInClass(tx, blocks, append(sameDay, o), date, o, names); err != nil {
			return err
		}
		if replaced != nil {
			if err := tx.DeleteOverride(replaced.ID()); err != nil {
				return err
			}
			if err := tx.AppendEvent(s.event(b.CycleID(), by, domain.ActionOverrideDeleted, b.ID(), replaced.ID(), date, o.Reason(),
				"Reemplazado — "+domain.OverrideSummary(*replaced, b, nil, names))); err != nil {
				return err
			}
		}
		if err := tx.InsertOverride(o); err != nil {
			return err
		}
		return tx.AppendEvent(s.event(b.CycleID(), by, domain.ActionOverrideCreated, b.ID(), o.ID(), date, o.Reason(),
			domain.OverrideSummary(o, b, partner, names)))
	})
	if err != nil {
		return domain.ScheduleOverride{}, err
	}
	return o, nil
}

// DeleteOverride undoes a change: the class goes back to its base timetable
// that date — unless that would now double-book someone.
func (s *ScheduleService) DeleteOverride(ctx context.Context, id, reason string, by domain.Actor) error {
	reason, err := domain.ScheduleReason(reason, false)
	if err != nil {
		return err
	}
	// The change names its cycle (the lock to take): read it, then re-read it
	// under the lock.
	found, err := s.repo.GetScheduleOverride(ctx, id)
	if err != nil {
		return err
	}
	cycleID := found.CycleID()
	cat, err := s.repo.LoadScheduleCatalog(ctx, cycleID)
	if err != nil {
		return err
	}
	names := newScheduleNames(cat)
	return s.repo.RunScheduleTx(ctx, cycleID, func(tx ports.ScheduleTx) error {
		o, err := tx.GetOverride(id)
		if err != nil {
			return err
		}
		b, err := tx.GetBlock(o.BlockID())
		if err != nil {
			return err
		}
		partner, err := s.swapPartner(tx, b, o)
		if err != nil {
			return err
		}
		sameDay, err := tx.OverridesOn(cycleID, o.Date())
		if err != nil {
			return err
		}
		var rest []domain.ScheduleOverride
		for _, other := range sameDay {
			if other.ID() != id {
				rest = append(rest, other)
			}
		}
		blocks, err := tx.Blocks(cycleID)
		if err != nil {
			return err
		}
		day := domain.EffectiveDay(blocks, rest, o.Date())
		if a, other, kind, found := domain.FirstClashInDay(day); found {
			if a.Block.ID() == b.ID() || (partner != nil && a.Block.ID() == partner.ID()) {
				a, other = other, a
			}
			return domain.ClashError(kind, other, names)
		}
		if err := tx.DeleteOverride(id); err != nil {
			return err
		}
		return tx.AppendEvent(s.event(cycleID, by, domain.ActionOverrideDeleted, b.ID(), id, o.Date(), reason,
			"Se deshizo — "+domain.OverrideSummary(o, b, partner, names)))
	})
}

// AvailableTeachers lists who is free for a class, best fits first.
func (s *ScheduleService) AvailableTeachers(ctx context.Context, q AvailabilityQuery) ([]domain.AvailableTeacher, error) {
	slot, err := domain.NewClassTime(q.Start, q.End)
	if err != nil {
		return nil, err
	}
	cat, err := s.repo.LoadScheduleCatalog(ctx, q.CycleID)
	if err != nil {
		return nil, err
	}
	group, ok := findGroup(cat, q.GroupID)
	if !ok {
		return nil, glerrors.New(glerrors.KindInvalid, "group not found in the cycle")
	}
	subject, ok := findSubject(cat, q.SubjectID)
	if !ok {
		return nil, glerrors.New(glerrors.KindInvalid, "subject not found")
	}
	blocks, err := s.repo.ListScheduleBlocks(ctx, q.CycleID)
	if err != nil {
		return nil, err
	}
	var day []domain.ScheduledClass
	cover := ""
	absent := make(map[string]bool)
	if q.Date.IsZero() {
		if q.Weekday < domain.FirstSchoolDay || q.Weekday > domain.LastSchoolDay {
			return nil, refusal(glerrors.KindInvalid, domain.CodeScheduleWeekdayInvalid, "El día va de lunes (1) a viernes (5)")
		}
		day = domain.BaseDay(blocks, q.Weekday)
	} else {
		date := asDate(q.Date)
		if date.Before(cat.Cycle.StartsOn()) || date.After(cat.Cycle.EndsOn()) || domain.WeekdayOf(date) > domain.LastSchoolDay {
			return []domain.AvailableTeacher{}, nil
		}
		// Covering a class on a date: its own teacher is no substitute.
		for _, b := range blocks {
			if b.ID() == q.ExcludeBlockID {
				cover = b.TeacherID()
			}
		}
		t, err := s.loadCalendar(ctx, cat.Cycle)
		if err != nil {
			return nil, err
		}
		if open, _ := t.schoolDay(date); !open {
			return []domain.AvailableTeacher{}, nil
		}
		if err := s.loadClasses(ctx, &t, date, date); err != nil {
			return nil, err
		}
		day = domain.EffectiveDay(t.blocks, t.overrides, date)
		// An absent teacher is no substitute (T19).
		for _, tc := range cat.Teachers {
			if _, away := domain.AbsentAt(t.absences, tc.ID(), date, slot); away {
				absent[tc.ID()] = true
			}
		}
	}
	subjectName := foldName(subject.Name())
	sameSubject := make(map[string]bool)
	for _, sj := range cat.Subjects {
		if foldName(sj.Name()) == subjectName {
			sameSubject[sj.ID()] = true
		}
	}
	sameGrade := make(map[string]bool)
	for _, g := range cat.Groups {
		if g.GradeLevelID() == group.GradeLevelID() {
			sameGrade[g.ID()] = true
		}
	}
	free := domain.AvailableTeachers(cat.Teachers, day, slot, q.ExcludeBlockID, cat.Assignments,
		func(id string) bool { return sameSubject[id] }, func(id string) bool { return sameGrade[id] })
	out := free[:0]
	for _, a := range free {
		if a.Teacher.ID() != cover && !absent[a.Teacher.ID()] {
			out = append(out, a)
		}
	}
	return out, nil
}

// SetGrid saves the hours the calendar shows for a level.
func (s *ScheduleService) SetGrid(ctx context.Context, levelID string, start, end int) (domain.ScheduleGrid, error) {
	if _, err := s.repo.GetLevel(ctx, levelID); err != nil {
		return domain.ScheduleGrid{}, err
	}
	g, err := domain.NewScheduleGrid(levelID, start, end)
	if err != nil {
		return domain.ScheduleGrid{}, err
	}
	if err := s.repo.SaveScheduleGrid(ctx, g); err != nil {
		return domain.ScheduleGrid{}, err
	}
	return g, nil
}

// Events is the cycle's history, newest first, at most limit entries
// (default 100, max 500).
func (s *ScheduleService) Events(ctx context.Context, cycleID string, limit int) ([]domain.ScheduleEvent, error) {
	if strings.TrimSpace(cycleID) == "" {
		return nil, glerrors.New(glerrors.KindInvalid, "cycle id is required")
	}
	if limit <= 0 {
		limit = defaultScheduleEvents
	}
	if limit > maxScheduleEvents {
		limit = maxScheduleEvents
	}
	return s.repo.ListScheduleEvents(ctx, cycleID, limit)
}

// --- placement -----------------------------------------------------------------

// placed is a class ready to write, with what its checks and history need.
type placed struct {
	block   domain.ScheduleBlock
	cycle   domain.Cycle
	names   scheduleNames
	started bool
	reason  string
	by      domain.Actor
}

// placement validates in against its assignment, the level's grid and the
// cycle's start, and builds the class with id.
func (s *ScheduleService) placement(ctx context.Context, id string, in BlockInput) (placed, error) {
	a, err := s.repo.GetTeachingAssignment(ctx, strings.TrimSpace(in.AssignmentID))
	if err != nil {
		if glerrors.KindOf(err) == glerrors.KindNotFound {
			return placed{}, refuseAssignment("Esa asignatura y docente no es una asignación del ciclo")
		}
		return placed{}, err
	}
	t, err := domain.NewClassTime(in.Start, in.End)
	if err != nil {
		return placed{}, err
	}
	b, err := domain.NewScheduleBlock(id, s.tenant, a.CycleID(), a.ID(), a.GroupID(), a.SubjectID(), a.TeacherID(), in.Weekday, t)
	if err != nil {
		return placed{}, err
	}
	cat, err := s.repo.LoadScheduleCatalog(ctx, a.CycleID())
	if err != nil {
		return placed{}, err
	}
	if grid := gridOfGroup(cat, a.GroupID()); !grid.Contains(t) {
		return placed{}, domain.OutsideGrid(grid)
	}
	started := s.started(cat.Cycle)
	reason, err := domain.ScheduleReason(in.Reason, started)
	if err != nil {
		return placed{}, err
	}
	return placed{block: b, cycle: cat.Cycle, names: newScheduleNames(cat), started: started, reason: reason}, nil
}

// checkPlacement refuses a class that collides on the base timetable or on
// any date ahead that has changes.
func (s *ScheduleService) checkPlacement(tx ports.ScheduleTx, blocks []domain.ScheduleBlock, b domain.ScheduleBlock, names scheduleNames) error {
	others := filterBlocks(blocks, func(x domain.ScheduleBlock) bool { return x.ID() != b.ID() })
	if other, kind, found := domain.FindClash(domain.BaseDay(others, b.Weekday()), domain.RegularClass(b)); found {
		return domain.ClashError(kind, other, names)
	}
	ahead, err := tx.OverridesFrom(b.CycleID(), s.today())
	if err != nil {
		return err
	}
	withB := append(others, b)
	for _, date := range datesOn(ahead, b.Weekday()) {
		day := domain.EffectiveDay(withB, onDate(ahead, date), date)
		if x, y, kind, found := domain.FirstClashInDay(day); found {
			other := y
			if y.Block.ID() == b.ID() {
				other = x
			}
			return domain.OverrideConflictError(date, kind, other, names)
		}
	}
	return nil
}

// checkRemoval refuses removing b when a change it takes with it (a swap
// sends the other class back to its base time) would double-book someone on a
// date ahead — the same check as undoing that change.
func (s *ScheduleService) checkRemoval(tx ports.ScheduleTx, b domain.ScheduleBlock, names scheduleNames) ([]domain.ScheduleOverride, error) {
	ahead, err := tx.OverridesFrom(b.CycleID(), s.today())
	if err != nil {
		return nil, err
	}
	var touching []domain.ScheduleOverride
	for _, o := range ahead {
		if o.Touches(b.ID()) {
			touching = append(touching, o)
		}
	}
	if len(touching) == 0 {
		return nil, nil
	}
	blocks, err := tx.Blocks(b.CycleID())
	if err != nil {
		return nil, err
	}
	others := filterBlocks(blocks, func(x domain.ScheduleBlock) bool { return x.ID() != b.ID() })
	for _, date := range datesOn(touching, b.Weekday()) {
		// Without b, EffectiveDay skips b's own changes and leaves a swap
		// partner at its base time: exactly what the cascade leaves behind.
		if _, other, kind, found := domain.FirstClashInDay(domain.EffectiveDay(others, onDate(ahead, date), date)); found {
			return nil, domain.OverrideConflictError(date, kind, other, names)
		}
	}
	return touching, nil
}

// overrideSummaryIn describes o with its classes looked up in blocks.
func overrideSummaryIn(blocks []domain.ScheduleBlock, o domain.ScheduleOverride, names scheduleNames) string {
	var b domain.ScheduleBlock
	var partner *domain.ScheduleBlock
	for i := range blocks {
		switch blocks[i].ID() {
		case o.BlockID():
			b = blocks[i]
		case o.SwapBlockID():
			partner = &blocks[i]
		}
	}
	return domain.OverrideSummary(o, b, partner, names)
}

// refuseIfAhead refuses moving a class that has changes on dates ahead.
func (s *ScheduleService) refuseIfAhead(tx ports.ScheduleTx, b domain.ScheduleBlock) error {
	ahead, err := tx.OverridesFrom(b.CycleID(), s.today())
	if err != nil {
		return err
	}
	for _, o := range ahead {
		if o.Touches(b.ID()) {
			return refusal(glerrors.KindConflict, domain.CodeScheduleBlockHasOverrides,
				"Esta clase tiene un cambio el "+domain.FormatSpanishDate(o.Date())+"; deshazlo antes de moverla de día o de asignación")
		}
	}
	return nil
}

func (s *ScheduleService) recordBase(tx ports.ScheduleTx, p placed, action domain.ScheduleAction, summary string) error {
	if !p.started {
		return nil
	}
	return tx.AppendEvent(s.event(p.cycle.ID(), p.by, action, p.block.ID(), "", time.Time{}, p.reason, summary))
}

func (s *ScheduleService) event(cycleID string, by domain.Actor, action domain.ScheduleAction, blockID, overrideID string,
	date time.Time, reason, summary string,
) domain.ScheduleEvent {
	return domain.ScheduleEvent{
		ID: s.newID(), TenantID: s.tenant, CycleID: cycleID, At: s.now().UTC(), By: by, Action: action,
		BlockID: blockID, OverrideID: overrideID, Date: date, Reason: reason, Summary: summary,
	}
}

// --- overrides -----------------------------------------------------------------

func (s *ScheduleService) blockAndCatalog(ctx context.Context, blockID string) (domain.ScheduleBlock, ports.ScheduleCatalog, error) {
	b, err := s.repo.GetScheduleBlock(ctx, blockID)
	if err != nil {
		return domain.ScheduleBlock{}, ports.ScheduleCatalog{}, err
	}
	cat, err := s.repo.LoadScheduleCatalog(ctx, b.CycleID())
	return b, cat, err
}

// swapPartner is the other class of a swap (nil for other kinds): of the same
// cycle, group and weekday.
func (s *ScheduleService) swapPartner(tx ports.ScheduleTx, b domain.ScheduleBlock, o domain.ScheduleOverride) (*domain.ScheduleBlock, error) {
	if o.Kind() != domain.OverrideSwap {
		return nil, nil
	}
	p, err := tx.GetBlock(o.SwapBlockID())
	if err != nil {
		if glerrors.KindOf(err) == glerrors.KindNotFound {
			return nil, refusal(glerrors.KindInvalid, domain.CodeScheduleSwapInvalid, "La otra clase no existe")
		}
		return nil, err
	}
	if p.CycleID() != b.CycleID() || p.GroupID() != b.GroupID() || p.Weekday() != b.Weekday() {
		return nil, refusal(glerrors.KindInvalid, domain.CodeScheduleSwapInvalid, "Solo se intercambian clases del mismo grupo y el mismo día")
	}
	return &p, nil
}

// checkOverrideDate is every rule on a date-specific change's date: inside
// the cycle, on the class's weekday, and a day the school calendar does not
// close (T19, ADR 0032).
func checkOverrideDate(t dayTimetable, b domain.ScheduleBlock, date time.Time) error {
	c := t.cycle
	if date.Before(c.StartsOn()) || date.After(c.EndsOn()) {
		return refusal(glerrors.KindInvalid, domain.CodeScheduleDateOutsideCycle, "La fecha queda fuera del ciclo")
	}
	if domain.WeekdayOf(date) != b.Weekday() {
		return refusal(glerrors.KindInvalid, domain.CodeScheduleDateWrongWeekday,
			"La clase es los "+domain.WeekdayName(b.Weekday())+" y el "+domain.FormatSpanishDate(date)+" no lo es")
	}
	if open, closed := t.schoolDay(date); !open {
		return domain.NonWorkingDateError(date, closed)
	}
	return nil
}

func checkSubstitute(cat ports.ScheduleCatalog, b domain.ScheduleBlock, teacherID string) error {
	if teacherID == b.TeacherID() {
		return refusal(glerrors.KindInvalid, domain.CodeScheduleSubstituteInvalid, "Ese docente ya es quien da la clase")
	}
	for _, t := range cat.Teachers {
		if t.ID() == teacherID {
			return nil
		}
	}
	return refusal(glerrors.KindInvalid, domain.CodeScheduleSubstituteInvalid, "Ese docente no existe")
}

// clashFreeDay refuses o when the effective timetable of date with it would
// double-book someone, naming the class o's class collides with.
func clashFreeDay(blocks []domain.ScheduleBlock, overrides []domain.ScheduleOverride, date time.Time, o domain.ScheduleOverride, names scheduleNames) error {
	day := domain.EffectiveDay(blocks, overrides, date)
	a, b, kind, found := domain.FirstClashInDay(day)
	if !found {
		return nil
	}
	other := b
	if o.Touches(b.Block.ID()) {
		other = a
	}
	return domain.ClashError(kind, other, names)
}

// withoutReplaced takes the change replaceID out of sameDay: it must be a
// substitute or a cancellation of b that date (a swap moves two classes and is
// undone on its own).
func withoutReplaced(sameDay []domain.ScheduleOverride, b domain.ScheduleBlock, replaceID string) ([]domain.ScheduleOverride, *domain.ScheduleOverride, error) {
	if replaceID == "" {
		return sameDay, nil, nil
	}
	rest := make([]domain.ScheduleOverride, 0, len(sameDay))
	var replaced *domain.ScheduleOverride
	for i := range sameDay {
		if sameDay[i].ID() == replaceID {
			replaced = &sameDay[i]
			continue
		}
		rest = append(rest, sameDay[i])
	}
	if replaced == nil || replaced.BlockID() != b.ID() {
		return nil, nil, refusal(glerrors.KindConflict, domain.CodeScheduleBlockTaken, "Ese cambio ya no está en esta clase ese día; recarga el horario")
	}
	if replaced.Kind() == domain.OverrideSwap {
		return nil, nil, refusal(glerrors.KindConflict, domain.CodeScheduleBlockTaken,
			"Esa clase tiene un intercambio ese día; deshaz el intercambio primero")
	}
	return rest, replaced, nil
}

// refuseAbsentInClass refuses o when, on date, it puts a teacher in a class
// while they are absent: a substitute who is away then, or a swap moving a
// class into its teacher's absence.
func refuseAbsentInClass(tx ports.ScheduleTx, blocks []domain.ScheduleBlock, overrides []domain.ScheduleOverride, date time.Time,
	o domain.ScheduleOverride, names scheduleNames,
) error {
	absences, err := tx.ActiveAbsences(o.CycleID(), date, date)
	if err != nil {
		return err
	}
	for _, c := range domain.EffectiveDay(blocks, overrides, date) {
		if c.Status == domain.ClassCancelled || !o.Touches(c.Block.ID()) {
			continue
		}
		if a, away := domain.AbsentAt(absences, c.TeacherID, date, c.Time); away {
			return domain.TeacherAbsentError(names.Teacher(c.TeacherID), a, date)
		}
	}
	return nil
}

// --- helpers ---------------------------------------------------------------------

func refuseAssignment(msg string) error {
	return refusal(glerrors.KindInvalid, domain.CodeScheduleAssignmentInvalid, msg)
}

func activeCycleID(cycles []domain.Cycle) string {
	for _, c := range cycles {
		if c.Status() == domain.CycleStatusActive {
			return c.ID()
		}
	}
	return ""
}

func asDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func filterBlocks(blocks []domain.ScheduleBlock, keep func(domain.ScheduleBlock) bool) []domain.ScheduleBlock {
	out := make([]domain.ScheduleBlock, 0, len(blocks))
	for _, b := range blocks {
		if keep(b) {
			out = append(out, b)
		}
	}
	return out
}

func onDate(overrides []domain.ScheduleOverride, date time.Time) []domain.ScheduleOverride {
	var out []domain.ScheduleOverride
	for _, o := range overrides {
		if o.Date().Equal(date) {
			out = append(out, o)
		}
	}
	return out
}

// datesOn lists the distinct dates of overrides falling on weekday, in order.
func datesOn(overrides []domain.ScheduleOverride, weekday int) []time.Time {
	seen := make(map[time.Time]bool)
	var out []time.Time
	for _, o := range overrides {
		if domain.WeekdayOf(o.Date()) == weekday && !seen[o.Date()] {
			seen[o.Date()] = true
			out = append(out, o.Date())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

// gridsOf gives every level of the catalog its grid: saved or default.
func gridsOf(cat ports.ScheduleCatalog) []domain.ScheduleGrid {
	saved := make(map[string]domain.ScheduleGrid, len(cat.Grids))
	for _, g := range cat.Grids {
		saved[g.LevelID()] = g
	}
	out := make([]domain.ScheduleGrid, 0, len(cat.Levels))
	for _, l := range cat.Levels {
		g, ok := saved[l.ID()]
		if !ok {
			g = domain.DefaultScheduleGrid(l.ID())
		}
		out = append(out, g)
	}
	return out
}

func gridOfGroup(cat ports.ScheduleCatalog, groupID string) domain.ScheduleGrid {
	levelID := ""
	if g, ok := findGroup(cat, groupID); ok {
		for _, gl := range cat.Grades {
			if gl.ID() == g.GradeLevelID() {
				levelID = gl.LevelID()
			}
		}
	}
	for _, g := range gridsOf(cat) {
		if g.LevelID() == levelID {
			return g
		}
	}
	return domain.DefaultScheduleGrid(levelID)
}

func findGroup(cat ports.ScheduleCatalog, id string) (domain.Group, bool) {
	for _, g := range cat.Groups {
		if g.ID() == id {
			return g, true
		}
	}
	return domain.Group{}, false
}

func findSubject(cat ports.ScheduleCatalog, id string) (domain.Subject, bool) {
	for _, s := range cat.Subjects {
		if s.ID() == id {
			return s, true
		}
	}
	return domain.Subject{}, false
}

// foldName compares subject names the way 0015 does: case-insensitively.
func foldName(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// scheduleNames resolves the catalog's ids to what a person reads; a group is
// "<grade> <group>" ("3.º A").
type scheduleNames struct {
	teachers map[string]string
	subjects map[string]string
	groups   map[string]string
}

func newScheduleNames(cat ports.ScheduleCatalog) scheduleNames {
	n := scheduleNames{teachers: map[string]string{}, subjects: map[string]string{}, groups: map[string]string{}}
	for _, t := range cat.Teachers {
		n.teachers[t.ID()] = t.FullName()
	}
	for _, s := range cat.Subjects {
		n.subjects[s.ID()] = s.Name()
	}
	grades := make(map[string]string, len(cat.Grades))
	for _, g := range cat.Grades {
		grades[g.ID()] = g.Name()
	}
	for _, g := range cat.Groups {
		n.groups[g.ID()] = strings.TrimSpace(grades[g.GradeLevelID()] + " " + g.Name())
	}
	return n
}

func (n scheduleNames) Teacher(id string) string { return orDash(n.teachers[id]) }
func (n scheduleNames) Subject(id string) string { return orDash(n.subjects[id]) }
func (n scheduleNames) Group(id string) string   { return orDash(n.groups[id]) }

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
