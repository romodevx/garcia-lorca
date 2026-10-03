package usecase

import (
	"context"
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
)

// MaxUncoveredDays bounds the "Clases por cubrir" range: a month at most.
const MaxUncoveredDays = 31

// AbsenceInput registers or edits a teacher's absence (T19, ADR 0032). An
// empty CycleID means the cycle whose dates contain StartsOn. AllDay ignores
// Start/End; otherwise [Start, End) applies to every date of the span.
type AbsenceInput struct {
	CycleID   string
	TeacherID string
	Kind      domain.AbsenceKind
	StartsOn  time.Time
	EndsOn    time.Time
	AllDay    bool
	Start     int
	End       int
	Note      string
}

// AbsenceResult is a saved absence and how many classes it leaves uncovered
// on its dates (school days only, after the changes already recorded).
type AbsenceResult struct {
	Absence   domain.TeacherAbsence
	Uncovered int
}

// AbsenceQuery lists the absences of a cycle touching From..To, of TeacherID
// when set; cancelled ones only with IncludeCancelled.
type AbsenceQuery struct {
	CycleID          string
	TeacherID        string
	From             time.Time
	To               time.Time
	IncludeCancelled bool
}

// UncoveredClass is a class nobody gives on Date: its teacher is absent and
// there is no substitute nor cancellation.
type UncoveredClass struct {
	Date  time.Time
	Class domain.ScheduledClass
}

// CreateAbsence registers a teacher's absence. Two active absences of one
// teacher never share a minute; the write takes the cycle's timetable lock,
// like every write that decides who is in class.
func (s *ScheduleService) CreateAbsence(ctx context.Context, in AbsenceInput, by domain.Actor) (AbsenceResult, error) {
	cat, err := s.absenceCatalog(ctx, in.CycleID, in.StartsOn)
	if err != nil {
		return AbsenceResult{}, err
	}
	name, err := absenceTeacher(cat, in.TeacherID)
	if err != nil {
		return AbsenceResult{}, err
	}
	span, err := absenceSpan(in)
	if err != nil {
		return AbsenceResult{}, err
	}
	a, err := domain.NewTeacherAbsence(domain.AbsenceFields{
		ID: s.newID(), TenantID: s.tenant, CycleID: cat.Cycle.ID(), TeacherID: in.TeacherID, Kind: in.Kind,
		StartsOn: in.StartsOn, EndsOn: in.EndsOn, Span: span, Note: in.Note,
	}, by, s.now().UTC())
	if err != nil {
		return AbsenceResult{}, err
	}
	if err := absenceInCycle(cat.Cycle, a); err != nil {
		return AbsenceResult{}, err
	}
	err = s.repo.RunScheduleTx(ctx, a.CycleID(), func(tx ports.ScheduleTx) error {
		if err := refuseOverlap(tx, a, name); err != nil {
			return err
		}
		if err := tx.InsertAbsence(a); err != nil {
			return err
		}
		return tx.AppendEvent(s.absenceEvent(a, by, domain.ActionAbsenceCreated, domain.AbsenceSummary("Ausencia registrada", a, name)))
	})
	if err != nil {
		return AbsenceResult{}, err
	}
	return s.withUncovered(ctx, a)
}

// UpdateAbsence changes an active absence's kind, dates, hours or note; the
// teacher is the absence's and never changes.
func (s *ScheduleService) UpdateAbsence(ctx context.Context, id string, in AbsenceInput, by domain.Actor) (AbsenceResult, error) {
	found, err := s.repo.GetTeacherAbsence(ctx, id)
	if err != nil {
		return AbsenceResult{}, err
	}
	if in.TeacherID != "" && in.TeacherID != found.TeacherID() {
		return AbsenceResult{}, glerrors.New(glerrors.KindInvalid, "Una ausencia no cambia de docente; cancélala y registra otra")
	}
	cat, err := s.repo.LoadScheduleCatalog(ctx, found.CycleID())
	if err != nil {
		return AbsenceResult{}, err
	}
	name := newScheduleNames(cat).Teacher(found.TeacherID())
	span, err := absenceSpan(in)
	if err != nil {
		return AbsenceResult{}, err
	}
	var after domain.TeacherAbsence
	err = s.repo.RunScheduleTx(ctx, found.CycleID(), func(tx ports.ScheduleTx) error {
		before, err := tx.GetAbsence(id)
		if err != nil {
			return err
		}
		after, err = before.Edit(domain.AbsenceChanges{Kind: in.Kind, StartsOn: in.StartsOn, EndsOn: in.EndsOn, Span: span, Note: in.Note}, s.now().UTC())
		if err != nil {
			return err
		}
		if err := absenceInCycle(cat.Cycle, after); err != nil {
			return err
		}
		if err := refuseOverlap(tx, after, name); err != nil {
			return err
		}
		if err := tx.UpdateAbsence(after); err != nil {
			return err
		}
		summary := domain.AbsenceSummary("Ausencia cambiada", after, name) + " (antes " + domain.AbsenceWhen(before) + ")"
		return tx.AppendEvent(s.absenceEvent(after, by, domain.ActionAbsenceUpdated, summary))
	})
	if err != nil {
		return AbsenceResult{}, err
	}
	return s.withUncovered(ctx, after)
}

// CancelAbsence cancels an absence: its classes are no longer uncovered. The
// record stays, stamped with who cancelled it and when. callerTeacherID is the
// teacher record of whoever cancels ("" for none): nobody cancels their own.
func (s *ScheduleService) CancelAbsence(ctx context.Context, id string, by domain.Actor, callerTeacherID string) (domain.TeacherAbsence, error) {
	found, err := s.repo.GetTeacherAbsence(ctx, id)
	if err != nil {
		return domain.TeacherAbsence{}, err
	}
	if callerTeacherID != "" && callerTeacherID == found.TeacherID() {
		return domain.TeacherAbsence{}, refusal(glerrors.KindForbidden, domain.CodeAbsenceOwn, "No puedes cancelar tu propia ausencia; pídeselo a la dirección")
	}
	cat, err := s.repo.LoadScheduleCatalog(ctx, found.CycleID())
	if err != nil {
		return domain.TeacherAbsence{}, err
	}
	name := newScheduleNames(cat).Teacher(found.TeacherID())
	var out domain.TeacherAbsence
	err = s.repo.RunScheduleTx(ctx, found.CycleID(), func(tx ports.ScheduleTx) error {
		current, err := tx.GetAbsence(id)
		if err != nil {
			return err
		}
		out, err = current.Cancel(by, s.now().UTC())
		if err != nil {
			return err
		}
		if err := tx.UpdateAbsence(out); err != nil {
			return err
		}
		return tx.AppendEvent(s.absenceEvent(out, by, domain.ActionAbsenceCanceled, domain.AbsenceSummary("Ausencia cancelada", out, name)))
	})
	return out, err
}

// Absences lists the absences of a cycle touching q.From..q.To.
func (s *ScheduleService) Absences(ctx context.Context, q AbsenceQuery) ([]domain.TeacherAbsence, error) {
	if strings.TrimSpace(q.CycleID) == "" {
		return nil, glerrors.New(glerrors.KindInvalid, "cycle id is required")
	}
	from, to, err := dateRange(q.From, q.To)
	if err != nil {
		return nil, err
	}
	all, err := s.repo.ListTeacherAbsences(ctx, q.CycleID, from, to)
	if err != nil {
		return nil, err
	}
	out := make([]domain.TeacherAbsence, 0, len(all))
	for _, a := range all {
		if (q.TeacherID == "" || a.TeacherID() == q.TeacherID) && (q.IncludeCancelled || a.Active()) {
			out = append(out, a)
		}
	}
	return out, nil
}

// Uncovered lists, date by date, the classes of cycleID nobody gives between
// from and to (at most MaxUncoveredDays): school days of the cycle only.
func (s *ScheduleService) Uncovered(ctx context.Context, cycleID string, from, to time.Time) ([]UncoveredClass, error) {
	from, to, err := dateRange(from, to)
	if err != nil {
		return nil, err
	}
	if domain.DaysInclusive(from, to) > MaxUncoveredDays {
		return nil, glerrors.New(glerrors.KindInvalid, "El rango de fechas es de a lo más un mes")
	}
	cycle, err := s.repo.GetCycle(ctx, cycleID)
	if err != nil {
		return nil, err
	}
	return s.uncoveredBetween(ctx, cycle, from, to)
}

// --- the effective timetable with absences -----------------------------------------

// dayTimetable is everything the effective timetable of dates from..to needs.
type dayTimetable struct {
	cycle     domain.Cycle
	blocks    []domain.ScheduleBlock
	overrides []domain.ScheduleOverride
	absences  []domain.TeacherAbsence
	periods   []domain.NonWorkingPeriod
}

// loadCalendar reads what decides whether a date has classes (the cycle and
// its closed periods) — enough for schoolDay, before any class is read.
func (s *ScheduleService) loadCalendar(ctx context.Context, cycle domain.Cycle) (dayTimetable, error) {
	periods, err := s.repo.ListNonWorkingPeriods(ctx, cycle.ID())
	return dayTimetable{cycle: cycle, periods: periods}, err
}

// loadClasses adds the classes, changes and absences of from..to to t.
func (s *ScheduleService) loadClasses(ctx context.Context, t *dayTimetable, from, to time.Time) error {
	var err error
	if t.blocks, err = s.repo.ListScheduleBlocks(ctx, t.cycle.ID()); err != nil {
		return err
	}
	if t.overrides, err = s.repo.ListScheduleOverrides(ctx, t.cycle.ID(), from, to); err != nil {
		return err
	}
	t.absences, err = s.repo.ListTeacherAbsences(ctx, t.cycle.ID(), from, to)
	return err
}

// schoolDay reports whether date has classes: a weekday of the cycle that the
// school calendar does not close. closed is the closing period's label.
func (t dayTimetable) schoolDay(date time.Time) (open bool, closed string) {
	if domain.WeekdayOf(date) > domain.LastSchoolDay || date.Before(t.cycle.StartsOn()) || date.After(t.cycle.EndsOn()) {
		return false, ""
	}
	if label, ok := domain.NonWorkingLabel(t.periods, date); ok {
		return false, label
	}
	return true, ""
}

// on is the effective timetable of date, absences marked.
func (t dayTimetable) on(date time.Time) []domain.ScheduledClass {
	return domain.MarkUncovered(domain.EffectiveDay(t.blocks, t.overrides, date), t.absences, date)
}

func (s *ScheduleService) uncoveredBetween(ctx context.Context, cycle domain.Cycle, from, to time.Time) ([]UncoveredClass, error) {
	t, err := s.loadCalendar(ctx, cycle)
	if err != nil {
		return nil, err
	}
	if err := s.loadClasses(ctx, &t, from, to); err != nil {
		return nil, err
	}
	out := []UncoveredClass{}
	for date := from; !date.After(to); date = date.AddDate(0, 0, 1) {
		if open, _ := t.schoolDay(date); !open {
			continue
		}
		for _, c := range t.on(date) {
			if c.Status == domain.ClassUncovered {
				out = append(out, UncoveredClass{Date: date, Class: c})
			}
		}
	}
	return out, nil
}

// withUncovered counts the classes a just-saved absence leaves uncovered.
func (s *ScheduleService) withUncovered(ctx context.Context, a domain.TeacherAbsence) (AbsenceResult, error) {
	cycle, err := s.repo.GetCycle(ctx, a.CycleID())
	if err != nil {
		return AbsenceResult{}, err
	}
	all, err := s.uncoveredBetween(ctx, cycle, a.StartsOn(), a.EndsOn())
	if err != nil {
		return AbsenceResult{}, err
	}
	n := 0
	for _, u := range all {
		if u.Class.Absence != nil && u.Class.Absence.ID() == a.ID() {
			n++
		}
	}
	return AbsenceResult{Absence: a, Uncovered: n}, nil
}

// --- helpers ------------------------------------------------------------------------

// absenceCatalog is the catalog of cycleID, or of the cycle whose dates
// contain on when cycleID is empty.
func (s *ScheduleService) absenceCatalog(ctx context.Context, cycleID string, on time.Time) (ports.ScheduleCatalog, error) {
	if strings.TrimSpace(cycleID) == "" {
		cycles, err := s.repo.ListCycles(ctx)
		if err != nil {
			return ports.ScheduleCatalog{}, err
		}
		date := asDate(on)
		for _, c := range cycles {
			if !date.Before(c.StartsOn()) && !date.After(c.EndsOn()) {
				cycleID = c.ID()
				break
			}
		}
		if cycleID == "" {
			return ports.ScheduleCatalog{}, outsideCycle()
		}
	}
	return s.repo.LoadScheduleCatalog(ctx, cycleID)
}

func outsideCycle() error {
	return refusal(glerrors.KindInvalid, domain.CodeScheduleDateOutsideCycle, "Las fechas quedan fuera del ciclo")
}

func absenceInCycle(c domain.Cycle, a domain.TeacherAbsence) error {
	if a.StartsOn().Before(c.StartsOn()) || a.EndsOn().After(c.EndsOn()) {
		return outsideCycle()
	}
	return nil
}

// absenceTeacher is the name of teacherID, who must be one of the school's.
func absenceTeacher(cat ports.ScheduleCatalog, teacherID string) (string, error) {
	for _, t := range cat.Teachers {
		if t.ID() == teacherID {
			return t.FullName(), nil
		}
	}
	return "", refusal(glerrors.KindInvalid, domain.CodeScheduleSubstituteInvalid, "Ese docente no existe")
}

// absenceSpan is nil for a full day, else the validated time range.
func absenceSpan(in AbsenceInput) (*domain.ClassTime, error) {
	if in.AllDay {
		return nil, nil
	}
	t, err := domain.NewClassTime(in.Start, in.End)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func refuseOverlap(tx ports.ScheduleTx, a domain.TeacherAbsence, name string) error {
	others, err := tx.ActiveAbsences(a.CycleID(), a.StartsOn(), a.EndsOn())
	if err != nil {
		return err
	}
	for _, o := range others {
		if a.Overlaps(o) {
			return domain.AbsenceOverlapError(name, o)
		}
	}
	return nil
}

func (s *ScheduleService) absenceEvent(a domain.TeacherAbsence, by domain.Actor, action domain.ScheduleAction, summary string) domain.ScheduleEvent {
	e := s.event(a.CycleID(), by, action, "", "", a.StartsOn(), "", summary)
	e.AbsenceID = a.ID()
	return e
}

func dateRange(from, to time.Time) (time.Time, time.Time, error) {
	if from.IsZero() || to.IsZero() {
		return time.Time{}, time.Time{}, glerrors.New(glerrors.KindInvalid, "from and to are required")
	}
	from, to = asDate(from), asDate(to)
	if to.Before(from) {
		return time.Time{}, time.Time{}, glerrors.New(glerrors.KindInvalid, "La fecha final no puede ser antes de la inicial")
	}
	return from, to, nil
}
