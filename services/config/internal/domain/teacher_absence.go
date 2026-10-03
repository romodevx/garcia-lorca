package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Teacher absences (T19, ADR 0032). The school registers that a teacher will
// not be there — a whole day or a time range of every day of a span — and the
// effective timetable of each date it covers shows that teacher's classes as
// uncovered until a substitute is assigned or the class is cancelled for that
// date. It is not teacher attendance. An absence is never deleted: cancelling
// it keeps the record.

const (
	// MaxAbsenceNoteLen bounds an absence's note, in characters.
	MaxAbsenceNoteLen = 500
	// MaxAbsenceDays bounds one absence: a school year at most.
	MaxAbsenceDays = 366
)

// The refusal codes of teacher absences.
const (
	CodeAbsenceKindInvalid     RefusalCode = "SCHEDULE_ABSENCE_KIND_INVALID"
	CodeAbsenceDatesInvalid    RefusalCode = "SCHEDULE_ABSENCE_DATES_INVALID"
	CodeAbsenceNoteTooLong     RefusalCode = "SCHEDULE_ABSENCE_NOTE_TOO_LONG"
	CodeAbsenceCancelled       RefusalCode = "SCHEDULE_ABSENCE_CANCELLED"
	CodeAbsenceOverlap         RefusalCode = "SCHEDULE_ABSENCE_OVERLAP"
	CodeAbsenceOwn             RefusalCode = "SCHEDULE_ABSENCE_OWN"
	CodeScheduleTeacherAbsent  RefusalCode = "SCHEDULE_TEACHER_ABSENT"
	CodeScheduleDateNonWorking RefusalCode = "SCHEDULE_DATE_NON_WORKING"
)

// AbsenceKind is why a teacher is absent. Only schedule:write holders read it.
type AbsenceKind string

// The kinds of absence.
const (
	AbsenceSick     AbsenceKind = "sick"
	AbsenceVacation AbsenceKind = "vacation"
	AbsencePersonal AbsenceKind = "personal"
	AbsenceOther    AbsenceKind = "other"
)

var absenceLabels = map[AbsenceKind]string{
	AbsenceSick: "Incapacidad", AbsenceVacation: "Vacaciones", AbsencePersonal: "Asunto personal", AbsenceOther: "Otro",
}

// Valid reports whether k is a known kind.
func (k AbsenceKind) Valid() bool { _, ok := absenceLabels[k]; return ok }

// Label is the Spanish name of the kind ("Incapacidad").
func (k AbsenceKind) Label() string { return absenceLabels[k] }

// AbsenceFields are what a person writes when registering an absence. Span
// nil is a full day; otherwise it applies to every date of the span.
type AbsenceFields struct {
	ID        string
	TenantID  string
	CycleID   string
	TeacherID string
	Kind      AbsenceKind
	StartsOn  time.Time
	EndsOn    time.Time
	Span      *ClassTime
	Note      string
}

// AbsenceChanges are what an edit may change: everything but the teacher.
type AbsenceChanges struct {
	Kind     AbsenceKind
	StartsOn time.Time
	EndsOn   time.Time
	Span     *ClassTime
	Note     string
}

// TeacherAbsence is a teacher away on the dates [StartsOn, EndsOn], all day or
// during Span of each of those dates.
type TeacherAbsence struct {
	id          string
	tenantID    string
	cycleID     string
	teacherID   string
	kind        AbsenceKind
	startsOn    time.Time
	endsOn      time.Time
	span        *ClassTime
	note        string
	createdBy   Actor
	createdAt   time.Time
	updatedAt   time.Time
	cancelledAt time.Time
	cancelledBy Actor
}

// NewTeacherAbsence validates and builds an active absence registered by by at at.
func NewTeacherAbsence(f AbsenceFields, by Actor, at time.Time) (TeacherAbsence, error) {
	for _, required := range []struct{ value, what string }{
		{f.ID, "absence id"}, {f.TenantID, "absence tenant id"}, {f.CycleID, "absence cycle id"}, {f.TeacherID, "absence teacher id"},
	} {
		if strings.TrimSpace(required.value) == "" {
			return TeacherAbsence{}, glerrors.New(glerrors.KindInvalid, required.what+" is required")
		}
	}
	a := TeacherAbsence{id: f.ID, tenantID: f.TenantID, cycleID: f.CycleID, teacherID: f.TeacherID, createdBy: by, createdAt: at, updatedAt: at}
	if err := a.apply(AbsenceChanges{Kind: f.Kind, StartsOn: f.StartsOn, EndsOn: f.EndsOn, Span: f.Span, Note: f.Note}); err != nil {
		return TeacherAbsence{}, err
	}
	return a, nil
}

// RestoreTeacherAbsence rebuilds a stored absence (the adapter's mapper).
func RestoreTeacherAbsence(f AbsenceFields, createdBy Actor, createdAt, updatedAt time.Time, cancelledBy Actor, cancelledAt time.Time) (TeacherAbsence, error) {
	a, err := NewTeacherAbsence(f, createdBy, createdAt)
	if err != nil {
		return TeacherAbsence{}, err
	}
	a.updatedAt, a.cancelledBy, a.cancelledAt = updatedAt, cancelledBy, cancelledAt
	return a, nil
}

// apply validates c and writes it into a.
func (a *TeacherAbsence) apply(c AbsenceChanges) error {
	if !c.Kind.Valid() {
		return refusal(glerrors.KindInvalid, CodeAbsenceKindInvalid, "La ausencia es incapacidad, vacaciones, asunto personal u otro")
	}
	if c.StartsOn.IsZero() || c.EndsOn.IsZero() {
		return refusal(glerrors.KindInvalid, CodeAbsenceDatesInvalid, "Indica desde y hasta qué fecha")
	}
	from, to := calendarDate(c.StartsOn), calendarDate(c.EndsOn)
	if to.Before(from) {
		return refusal(glerrors.KindInvalid, CodeAbsenceDatesInvalid, "La fecha final no puede ser antes de la inicial")
	}
	if DaysInclusive(from, to) > MaxAbsenceDays {
		return refusal(glerrors.KindInvalid, CodeAbsenceDatesInvalid, "Una ausencia dura a lo más un año; revisa las fechas")
	}
	note := strings.TrimSpace(c.Note)
	if utf8.RuneCountInString(note) > MaxAbsenceNoteLen {
		return refusal(glerrors.KindInvalid, CodeAbsenceNoteTooLong, fmt.Sprintf("La nota admite hasta %d caracteres", MaxAbsenceNoteLen))
	}
	var span *ClassTime
	if c.Span != nil {
		s := *c.Span
		span = &s
	}
	a.kind, a.startsOn, a.endsOn, a.span, a.note = c.Kind, from, to, span, note
	return nil
}

// DaysInclusive counts the calendar dates from..to, both included.
func DaysInclusive(from, to time.Time) int {
	return int(calendarDate(to).Sub(calendarDate(from)).Hours()/hoursPerDay) + 1
}

const hoursPerDay = 24

// Edit returns the absence with c applied; a cancelled absence is not edited.
func (a TeacherAbsence) Edit(c AbsenceChanges, at time.Time) (TeacherAbsence, error) {
	if !a.Active() {
		return TeacherAbsence{}, absenceCancelled()
	}
	if err := a.apply(c); err != nil {
		return TeacherAbsence{}, err
	}
	a.updatedAt = at
	return a, nil
}

// Cancel returns the absence cancelled by by at at; once is enough.
func (a TeacherAbsence) Cancel(by Actor, at time.Time) (TeacherAbsence, error) {
	if !a.Active() {
		return TeacherAbsence{}, absenceCancelled()
	}
	a.cancelledAt, a.cancelledBy, a.updatedAt = at, by, at
	return a, nil
}

func absenceCancelled() error {
	return refusal(glerrors.KindConflict, CodeAbsenceCancelled, "Esa ausencia ya está cancelada")
}

// ID is the absence's id.
func (a TeacherAbsence) ID() string { return a.id }

// TenantID is the school.
func (a TeacherAbsence) TenantID() string { return a.tenantID }

// CycleID is the cycle its dates fall in.
func (a TeacherAbsence) CycleID() string { return a.cycleID }

// TeacherID is who is absent.
func (a TeacherAbsence) TeacherID() string { return a.teacherID }

// Kind is why (schedule:write only).
func (a TeacherAbsence) Kind() AbsenceKind { return a.kind }

// StartsOn is the first date (midnight UTC).
func (a TeacherAbsence) StartsOn() time.Time { return a.startsOn }

// EndsOn is the last date, inclusive.
func (a TeacherAbsence) EndsOn() time.Time { return a.endsOn }

// FullDay reports whether the absence takes every date whole.
func (a TeacherAbsence) FullDay() bool { return a.span == nil }

// Span is the time range of each date; the zero ClassTime when FullDay.
func (a TeacherAbsence) Span() ClassTime {
	if a.span == nil {
		return ClassTime{}
	}
	return *a.span
}

// Note is the free text (schedule:write only).
func (a TeacherAbsence) Note() string { return a.note }

// CreatedBy is who registered it.
func (a TeacherAbsence) CreatedBy() Actor { return a.createdBy }

// CreatedAt is when it was registered.
func (a TeacherAbsence) CreatedAt() time.Time { return a.createdAt }

// UpdatedAt is when it last changed.
func (a TeacherAbsence) UpdatedAt() time.Time { return a.updatedAt }

// CancelledAt is when it was cancelled (zero while active).
func (a TeacherAbsence) CancelledAt() time.Time { return a.cancelledAt }

// CancelledBy is who cancelled it.
func (a TeacherAbsence) CancelledBy() Actor { return a.cancelledBy }

// Active reports whether the absence still counts.
func (a TeacherAbsence) Active() bool { return a.cancelledAt.IsZero() }

// Covers is THE rule: the teacher is away on date during slot — the absence is
// active, date is one of its dates, and it takes the whole day or its time
// range shares a minute with slot (half-open, like classes).
func (a TeacherAbsence) Covers(date time.Time, slot ClassTime) bool {
	date = calendarDate(date)
	if !a.Active() || date.Before(a.startsOn) || date.After(a.endsOn) {
		return false
	}
	return a.span == nil || a.span.Overlaps(slot)
}

// Overlaps reports whether two active absences of the same teacher take some
// common minute (the same absence never overlaps itself).
func (a TeacherAbsence) Overlaps(o TeacherAbsence) bool {
	if a.id == o.id || a.teacherID != o.teacherID || !a.Active() || !o.Active() {
		return false
	}
	if a.endsOn.Before(o.startsOn) || o.endsOn.Before(a.startsOn) {
		return false
	}
	return a.span == nil || o.span == nil || a.span.Overlaps(*o.span)
}

// AbsentAt finds the active absence of teacherID covering date during slot.
func AbsentAt(absences []TeacherAbsence, teacherID string, date time.Time, slot ClassTime) (TeacherAbsence, bool) {
	for _, a := range absences {
		if a.teacherID == teacherID && a.Covers(date, slot) {
			return a, true
		}
	}
	return TeacherAbsence{}, false
}

// MarkUncovered marks the classes of day (the effective timetable of date)
// whose teacher — the substitute when there is one — is absent then: they are
// ClassUncovered and carry the absence. Cancelled classes need nobody.
func MarkUncovered(day []ScheduledClass, absences []TeacherAbsence, date time.Time) []ScheduledClass {
	out := make([]ScheduledClass, len(day))
	copy(out, day)
	for i := range out {
		if out[i].Status == ClassCancelled {
			continue
		}
		if a, ok := AbsentAt(absences, out[i].TeacherID, date, out[i].Time); ok {
			found := a
			out[i].Status, out[i].Absence = ClassUncovered, &found
		}
	}
	return out
}

// AbsenceWhen is "del 5 de octubre de 2026 al 7 de octubre de 2026, todo el
// día" or "el 5 de octubre de 2026, de 08:00 a 10:00".
func AbsenceWhen(a TeacherAbsence) string {
	dates := "el " + FormatSpanishDate(a.startsOn)
	if !a.endsOn.Equal(a.startsOn) {
		dates = "del " + FormatSpanishDate(a.startsOn) + " al " + FormatSpanishDate(a.endsOn)
	}
	return dates + ", " + absenceHours(a)
}

func absenceHours(a TeacherAbsence) string {
	if a.span == nil {
		return "todo el día"
	}
	return "de " + FormatClock(a.span.start) + " a " + FormatClock(a.span.end)
}

// AbsenceSummary is the history line of an absence. It never names the kind
// or the note: the history is read with schedule:read.
func AbsenceSummary(prefix string, a TeacherAbsence, teacherName string) string {
	return prefix + ": " + teacherName + ", " + AbsenceWhen(a)
}

// TeacherAbsentError refuses putting an absent teacher in a class on date:
// "Ana Ruiz está ausente el 5 de octubre de 2026 (todo el día)".
func TeacherAbsentError(teacherName string, a TeacherAbsence, date time.Time) error {
	return refusal(glerrors.KindConflict, CodeScheduleTeacherAbsent,
		fmt.Sprintf("%s está ausente el %s (%s)", teacherName, FormatSpanishDate(date), absenceHours(a)))
}

// AbsenceOverlapError refuses a second absence over the same time.
func AbsenceOverlapError(teacherName string, other TeacherAbsence) error {
	return refusal(glerrors.KindConflict, CodeAbsenceOverlap,
		fmt.Sprintf("%s ya tiene una ausencia %s", teacherName, AbsenceWhen(other)))
}

// NonWorkingDateError refuses a date-specific change on a day without classes.
func NonWorkingDateError(date time.Time, label string) error {
	return refusal(glerrors.KindInvalid, CodeScheduleDateNonWorking,
		fmt.Sprintf("El %s no hay clases (%s)", FormatSpanishDate(date), label))
}

// NonWorkingLabel is the label of the period of periods that closes date, if any.
func NonWorkingLabel(periods []NonWorkingPeriod, date time.Time) (string, bool) {
	date = calendarDate(date)
	for _, p := range periods {
		if !date.Before(calendarDate(p.from)) && !date.After(calendarDate(p.to)) {
			return p.label, true
		}
	}
	return "", false
}
