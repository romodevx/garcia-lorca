package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// The school timetable (T16, ADR 0023). A base weekly timetable per cycle —
// classes Monday to Friday, each one a teaching assignment placed on a weekday
// and a time — plus date-specific changes (overrides) that never touch the
// base. Times are minutes since midnight, in 5-minute steps; a class is the
// half-open range [start, end), so back-to-back classes do not collide.

const (
	// MinutesPerDay bounds a class time: a class ends at midnight at the latest.
	MinutesPerDay = 24 * 60
	// ScheduleStep is the timetable's resolution, in minutes.
	ScheduleStep = 5
	// DefaultGridStart and DefaultGridEnd are the hours the calendar shows for
	// a level with no grid saved: 07:00–15:00 (owner answer 11).
	DefaultGridStart = 7 * 60
	DefaultGridEnd   = 15 * 60
	// MinGridSpan is the shortest grid a level may set, in minutes.
	MinGridSpan = 60
	// MaxScheduleReasonLen bounds a change's reason, in characters.
	MaxScheduleReasonLen = 200
	// FirstSchoolDay and LastSchoolDay are the ISO weekdays with classes.
	FirstSchoolDay = 1
	LastSchoolDay  = 5
)

// The refusal codes of the timetable surface (T16).
const (
	CodeScheduleTimeInvalid         RefusalCode = "SCHEDULE_TIME_INVALID"
	CodeScheduleOutsideGrid         RefusalCode = "SCHEDULE_OUTSIDE_GRID"
	CodeScheduleWeekdayInvalid      RefusalCode = "SCHEDULE_WEEKDAY_INVALID"
	CodeScheduleGridInvalid         RefusalCode = "SCHEDULE_GRID_INVALID"
	CodeScheduleReasonRequired      RefusalCode = "SCHEDULE_REASON_REQUIRED"
	CodeScheduleDateOutsideCycle    RefusalCode = "SCHEDULE_DATE_OUTSIDE_CYCLE"
	CodeScheduleDateWrongWeekday    RefusalCode = "SCHEDULE_DATE_WRONG_WEEKDAY"
	CodeScheduleSwapInvalid         RefusalCode = "SCHEDULE_SWAP_INVALID"
	CodeScheduleSubstituteInvalid   RefusalCode = "SCHEDULE_SUBSTITUTE_INVALID"
	CodeScheduleOverrideKindInvalid RefusalCode = "SCHEDULE_OVERRIDE_KIND_INVALID"
	CodeScheduleAssignmentInvalid   RefusalCode = "SCHEDULE_ASSIGNMENT_INVALID"
	CodeScheduleTeacherBusy         RefusalCode = "SCHEDULE_TEACHER_BUSY"
	CodeScheduleGroupBusy           RefusalCode = "SCHEDULE_GROUP_BUSY"
	CodeScheduleOverrideConflict    RefusalCode = "SCHEDULE_OVERRIDE_CONFLICT"
	CodeScheduleBlockTaken          RefusalCode = "SCHEDULE_BLOCK_TAKEN"
	CodeScheduleBlockHasOverrides   RefusalCode = "SCHEDULE_BLOCK_HAS_OVERRIDES"
	CodeScheduleChanged             RefusalCode = "SCHEDULE_CHANGED"
)

var spanishWeekdays = [...]string{"", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado", "domingo"}

var spanishMonths = [...]string{"", "enero", "febrero", "marzo", "abril", "mayo", "junio", "julio",
	"agosto", "septiembre", "octubre", "noviembre", "diciembre"}

// WeekdayName is the Spanish name of an ISO weekday (1 = lunes … 7 = domingo).
func WeekdayName(weekday int) string {
	if weekday < 1 || weekday >= len(spanishWeekdays) {
		return ""
	}
	return spanishWeekdays[weekday]
}

// WeekdayOf is the ISO weekday of a calendar date: 1 = Monday … 7 = Sunday.
func WeekdayOf(date time.Time) int {
	wd := int(date.Weekday())
	if wd == 0 {
		return 7
	}
	return wd
}

// FormatSpanishDate is "5 de octubre de 2026".
func FormatSpanishDate(date time.Time) string {
	return fmt.Sprintf("%d de %s de %d", date.Day(), spanishMonths[date.Month()], date.Year())
}

// FormatClock renders minutes since midnight as "HH:MM".
func FormatClock(minutes int) string {
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}

// ScheduleRefusal is a coded refusal of the timetable surface, for the use
// case's own checks (a class taken that date, a swap partner of another group).
func ScheduleRefusal(kind glerrors.Kind, code RefusalCode, msg string) error {
	return refusal(kind, code, msg)
}

// ScheduleChangedMeanwhile is the KindConflict refusal for a write that lost a
// race the database caught (an EXCLUDE or a unique key): reload and retry.
func ScheduleChangedMeanwhile() error {
	return refusal(glerrors.KindConflict, CodeScheduleChanged,
		"Alguien más cambió el horario al mismo tiempo; recarga y vuelve a intentarlo")
}

// --- times and grids --------------------------------------------------------

// ClassTime is a half-open range of minutes since midnight [start, end).
type ClassTime struct {
	start int
	end   int
}

// NewClassTime builds a class time: both ends in 5-minute steps, inside the
// day, and the end after the start.
func NewClassTime(start, end int) (ClassTime, error) {
	if start < 0 || end > MinutesPerDay || start%ScheduleStep != 0 || end%ScheduleStep != 0 {
		return ClassTime{}, refusal(glerrors.KindInvalid, CodeScheduleTimeInvalid, "La hora va en pasos de 5 minutos, entre 00:00 y 24:00")
	}
	if end <= start {
		return ClassTime{}, refusal(glerrors.KindInvalid, CodeScheduleTimeInvalid, "La clase debe terminar después de empezar")
	}
	return ClassTime{start: start, end: end}, nil
}

// Start is the first minute of the class.
func (t ClassTime) Start() int { return t.start }

// End is the minute the class ends (exclusive).
func (t ClassTime) End() int { return t.end }

// Overlaps reports whether two class times share a minute (half-open).
func (t ClassTime) Overlaps(o ClassTime) bool { return t.start < o.end && o.start < t.end }

// String is "08:00–08:50".
func (t ClassTime) String() string { return FormatClock(t.start) + "–" + FormatClock(t.end) }

// ScheduleGrid is the hours the calendar shows for a level.
type ScheduleGrid struct {
	levelID string
	start   int
	end     int
}

// NewScheduleGrid builds a level's grid: 5-minute steps inside the day, at
// least MinGridSpan long.
func NewScheduleGrid(levelID string, start, end int) (ScheduleGrid, error) {
	if strings.TrimSpace(levelID) == "" {
		return ScheduleGrid{}, glerrors.New(glerrors.KindInvalid, "level id is required")
	}
	if start < 0 || end > MinutesPerDay || start%ScheduleStep != 0 || end%ScheduleStep != 0 || end-start < MinGridSpan {
		return ScheduleGrid{}, refusal(glerrors.KindInvalid, CodeScheduleGridInvalid,
			"La rejilla va en pasos de 5 minutos y dura al menos una hora")
	}
	return ScheduleGrid{levelID: levelID, start: start, end: end}, nil
}

// DefaultScheduleGrid is the grid of a level that saved none: 07:00–15:00.
func DefaultScheduleGrid(levelID string) ScheduleGrid {
	return ScheduleGrid{levelID: levelID, start: DefaultGridStart, end: DefaultGridEnd}
}

// LevelID is the level the grid belongs to.
func (g ScheduleGrid) LevelID() string { return g.levelID }

// Start is the grid's first minute.
func (g ScheduleGrid) Start() int { return g.start }

// End is the grid's last minute (exclusive).
func (g ScheduleGrid) End() int { return g.end }

// Contains reports whether a class fits inside the grid.
func (g ScheduleGrid) Contains(t ClassTime) bool { return t.start >= g.start && t.end <= g.end }

// OutsideGrid is the refusal for a class the level's grid does not show.
func OutsideGrid(g ScheduleGrid) error {
	return refusal(glerrors.KindInvalid, CodeScheduleOutsideGrid,
		fmt.Sprintf("La clase queda fuera del horario del nivel (%s a %s)", FormatClock(g.start), FormatClock(g.end)))
}

// --- the base timetable -----------------------------------------------------

// ScheduleBlock is one class of the base timetable: a teaching assignment
// (teacher + group + subject of the cycle) on a weekday at a time. The group,
// subject and teacher are the assignment's — the database ties them to it.
type ScheduleBlock struct {
	id           string
	tenantID     string
	cycleID      string
	assignmentID string
	groupID      string
	subjectID    string
	teacherID    string
	weekday      int
	time         ClassTime
}

// NewScheduleBlock constructs a class. Every id is required (a class always
// comes from an assignment) and the weekday is Monday to Friday.
func NewScheduleBlock(id, tenantID, cycleID, assignmentID, groupID, subjectID, teacherID string, weekday int, t ClassTime) (ScheduleBlock, error) {
	for _, required := range []struct{ value, what string }{
		{id, "block id"}, {tenantID, "block tenant id"}, {cycleID, "block cycle id"},
		{assignmentID, "block assignment id"}, {groupID, "block group id"}, {subjectID, "block subject id"},
		{teacherID, "block teacher id"},
	} {
		if strings.TrimSpace(required.value) == "" {
			return ScheduleBlock{}, glerrors.New(glerrors.KindInvalid, required.what+" is required")
		}
	}
	if weekday < FirstSchoolDay || weekday > LastSchoolDay {
		return ScheduleBlock{}, refusal(glerrors.KindInvalid, CodeScheduleWeekdayInvalid, "El día va de lunes (1) a viernes (5)")
	}
	if t.end <= t.start {
		return ScheduleBlock{}, refusal(glerrors.KindInvalid, CodeScheduleTimeInvalid, "La clase necesita hora de inicio y de fin")
	}
	return ScheduleBlock{
		id: id, tenantID: tenantID, cycleID: cycleID, assignmentID: assignmentID,
		groupID: groupID, subjectID: subjectID, teacherID: teacherID, weekday: weekday, time: t,
	}, nil
}

// ID is the class's id.
func (b ScheduleBlock) ID() string { return b.id }

// TenantID is the school the class belongs to.
func (b ScheduleBlock) TenantID() string { return b.tenantID }

// CycleID is the cycle of the timetable.
func (b ScheduleBlock) CycleID() string { return b.cycleID }

// AssignmentID is the teaching assignment the class places.
func (b ScheduleBlock) AssignmentID() string { return b.assignmentID }

// GroupID is the assignment's group.
func (b ScheduleBlock) GroupID() string { return b.groupID }

// SubjectID is the assignment's subject.
func (b ScheduleBlock) SubjectID() string { return b.subjectID }

// TeacherID is the assignment's teacher.
func (b ScheduleBlock) TeacherID() string { return b.teacherID }

// Weekday is 1 (Monday) … 5 (Friday).
func (b ScheduleBlock) Weekday() int { return b.weekday }

// Time is when the class runs.
func (b ScheduleBlock) Time() ClassTime { return b.time }

// --- date-specific changes --------------------------------------------------

// OverrideKind is what a date-specific change does to a class.
type OverrideKind string

// The kinds of date-specific change.
const (
	// OverrideSubstitute: another teacher gives the class that day.
	OverrideSubstitute OverrideKind = "substitute"
	// OverrideSwap: the class trades its time with another class of the same
	// group that day.
	OverrideSwap OverrideKind = "swap"
	// OverrideCancel: the class does not happen that day.
	OverrideCancel OverrideKind = "cancel"
)

// Valid reports whether k is a known kind.
func (k OverrideKind) Valid() bool {
	return k == OverrideSubstitute || k == OverrideSwap || k == OverrideCancel
}

// Actor is who made a change, as the gateway names them.
type Actor struct {
	UserID string
	Name   string
}

// ScheduleOverride changes ONE class on ONE date without touching the base.
type ScheduleOverride struct {
	id          string
	tenantID    string
	cycleID     string
	blockID     string
	date        time.Time
	kind        OverrideKind
	teacherID   string
	swapBlockID string
	reason      string
	by          Actor
	createdAt   time.Time
}

// NewScheduleOverride constructs a change. A substitute needs the teacher, a
// swap the other class (never the same one); a cancel carries neither. The
// reason is required (at most MaxScheduleReasonLen characters). Only the
// calendar date of date counts.
func NewScheduleOverride(id, tenantID, cycleID, blockID string, date time.Time, kind OverrideKind,
	teacherID, swapBlockID, reason string, by Actor, createdAt time.Time,
) (ScheduleOverride, error) {
	for _, required := range []struct{ value, what string }{
		{id, "override id"}, {tenantID, "override tenant id"}, {cycleID, "override cycle id"}, {blockID, "override block id"},
	} {
		if strings.TrimSpace(required.value) == "" {
			return ScheduleOverride{}, glerrors.New(glerrors.KindInvalid, required.what+" is required")
		}
	}
	if !kind.Valid() {
		return ScheduleOverride{}, refusal(glerrors.KindInvalid, CodeScheduleOverrideKindInvalid,
			"El cambio es suplencia, intercambio o cancelación")
	}
	reason, err := ScheduleReason(reason, true)
	if err != nil {
		return ScheduleOverride{}, err
	}
	switch kind {
	case OverrideSubstitute:
		swapBlockID = ""
		if strings.TrimSpace(teacherID) == "" {
			return ScheduleOverride{}, refusal(glerrors.KindInvalid, CodeScheduleSubstituteInvalid, "Elige quién da la clase ese día")
		}
	case OverrideSwap:
		teacherID = ""
		if strings.TrimSpace(swapBlockID) == "" || swapBlockID == blockID {
			return ScheduleOverride{}, refusal(glerrors.KindInvalid, CodeScheduleSwapInvalid, "Elige otra clase del mismo grupo para intercambiar")
		}
	case OverrideCancel:
		teacherID, swapBlockID = "", ""
	}
	return ScheduleOverride{
		id: id, tenantID: tenantID, cycleID: cycleID, blockID: blockID, date: calendarDate(date),
		kind: kind, teacherID: teacherID, swapBlockID: swapBlockID, reason: reason, by: by, createdAt: createdAt,
	}, nil
}

// ScheduleReason trims and checks a change's reason: required when required
// is true, never longer than MaxScheduleReasonLen.
func ScheduleReason(reason string, required bool) (string, error) {
	reason = strings.TrimSpace(reason)
	if required && reason == "" {
		return "", refusal(glerrors.KindInvalid, CodeScheduleReasonRequired, "Escribe el motivo del cambio")
	}
	if utf8.RuneCountInString(reason) > MaxScheduleReasonLen {
		return "", refusal(glerrors.KindInvalid, CodeScheduleReasonRequired,
			fmt.Sprintf("El motivo admite hasta %d caracteres", MaxScheduleReasonLen))
	}
	return reason, nil
}

func calendarDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// ID is the change's id.
func (o ScheduleOverride) ID() string { return o.id }

// TenantID is the school.
func (o ScheduleOverride) TenantID() string { return o.tenantID }

// CycleID is the cycle of the class.
func (o ScheduleOverride) CycleID() string { return o.cycleID }

// BlockID is the class changed.
func (o ScheduleOverride) BlockID() string { return o.blockID }

// Date is the calendar date the change applies to (midnight UTC).
func (o ScheduleOverride) Date() time.Time { return o.date }

// Kind is what the change does.
func (o ScheduleOverride) Kind() OverrideKind { return o.kind }

// TeacherID is the substitute ("" unless kind substitute).
func (o ScheduleOverride) TeacherID() string { return o.teacherID }

// SwapBlockID is the other class of a swap ("" unless kind swap).
func (o ScheduleOverride) SwapBlockID() string { return o.swapBlockID }

// Reason is why the change was made.
func (o ScheduleOverride) Reason() string { return o.reason }

// By is who made it.
func (o ScheduleOverride) By() Actor { return o.by }

// CreatedAt is when it was made.
func (o ScheduleOverride) CreatedAt() time.Time { return o.createdAt }

// Touches reports whether the change alters blockID (a swap alters both).
func (o ScheduleOverride) Touches(blockID string) bool {
	return o.blockID == blockID || (o.swapBlockID != "" && o.swapBlockID == blockID)
}

// --- the effective timetable of a date ---------------------------------------

// ClassStatus is how a class runs on a date.
type ClassStatus string

// The statuses of a class on a date.
const (
	ClassRegular    ClassStatus = "regular"
	ClassSubstitute ClassStatus = "substitute"
	ClassSwapped    ClassStatus = "swapped"
	ClassCancelled  ClassStatus = "cancelled"
)

// ScheduledClass is a class as it happens on a date: its base block, who
// gives it and when, and the change that made it differ (nil if none).
type ScheduledClass struct {
	Block     ScheduleBlock
	TeacherID string
	Time      ClassTime
	Status    ClassStatus
	Override  *ScheduleOverride
}

// RegularClass is a block as the base timetable has it.
func RegularClass(b ScheduleBlock) ScheduledClass {
	return ScheduledClass{Block: b, TeacherID: b.teacherID, Time: b.time, Status: ClassRegular}
}

// BaseDay is the base timetable of a weekday: its classes as planned, by
// start time then group.
func BaseDay(blocks []ScheduleBlock, weekday int) []ScheduledClass {
	var out []ScheduledClass
	for _, b := range blocks {
		if b.weekday == weekday {
			out = append(out, RegularClass(b))
		}
	}
	sortClasses(out)
	return out
}

func sortClasses(out []ScheduledClass) {
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Time.start != out[j].Time.start {
			return out[i].Time.start < out[j].Time.start
		}
		return out[i].Block.groupID < out[j].Block.groupID
	})
}

// EffectiveDay is the timetable of a date: the base classes of its weekday
// with that date's changes applied, by start time then group. A cancelled
// class is kept (ClassCancelled) so the screen can show it struck.
func EffectiveDay(blocks []ScheduleBlock, overrides []ScheduleOverride, date time.Time) []ScheduledClass {
	date = calendarDate(date)
	weekday := WeekdayOf(date)
	byID := make(map[string]ScheduleBlock, len(blocks))
	var out []ScheduledClass
	index := make(map[string]int)
	for _, b := range blocks {
		byID[b.id] = b
		if b.weekday != weekday {
			continue
		}
		index[b.id] = len(out)
		out = append(out, RegularClass(b))
	}
	for i := range overrides {
		o := overrides[i]
		if !o.date.Equal(date) {
			continue
		}
		at, ok := index[o.blockID]
		if !ok {
			continue
		}
		applyOverride(out, index, at, &o, byID)
	}
	sortClasses(out)
	return out
}

func applyOverride(out []ScheduledClass, index map[string]int, at int, o *ScheduleOverride, byID map[string]ScheduleBlock) {
	switch o.kind {
	case OverrideSubstitute:
		out[at].TeacherID, out[at].Status, out[at].Override = o.teacherID, ClassSubstitute, o
	case OverrideCancel:
		out[at].Status, out[at].Override = ClassCancelled, o
	case OverrideSwap:
		other, ok := index[o.swapBlockID]
		if !ok {
			return
		}
		mine, theirs := byID[o.blockID].time, byID[o.swapBlockID].time
		out[at].Time, out[at].Status, out[at].Override = theirs, ClassSwapped, o
		out[other].Time, out[other].Status, out[other].Override = mine, ClassSwapped, o
	}
}

// ClashKind says who would be double-booked.
type ClashKind string

// The kinds of clash.
const (
	ClashTeacher ClashKind = "teacher"
	ClashGroup   ClashKind = "group"
)

// FindClash returns the first class of day that candidate collides with: the
// same teacher or the same group at an overlapping time. Cancelled classes and
// the candidate's own block never clash.
func FindClash(day []ScheduledClass, candidate ScheduledClass) (ScheduledClass, ClashKind, bool) {
	if candidate.Status == ClassCancelled {
		return ScheduledClass{}, "", false
	}
	for _, c := range day {
		if c.Status == ClassCancelled || c.Block.id == candidate.Block.id || !c.Time.Overlaps(candidate.Time) {
			continue
		}
		if c.TeacherID == candidate.TeacherID {
			return c, ClashTeacher, true
		}
		if c.Block.groupID == candidate.Block.groupID {
			return c, ClashGroup, true
		}
	}
	return ScheduledClass{}, "", false
}

// FirstClashInDay returns the first pair of classes of day that collide.
func FirstClashInDay(day []ScheduledClass) (ScheduledClass, ScheduledClass, ClashKind, bool) {
	for i, c := range day {
		if other, kind, found := FindClash(day[i+1:], c); found {
			return c, other, kind, true
		}
	}
	return ScheduledClass{}, ScheduledClass{}, "", false
}

// ScheduleNames resolves ids to the names a person reads.
type ScheduleNames interface {
	Teacher(id string) string
	Subject(id string) string
	Group(id string) string
}

// ClashError is the refusal naming the class something collides with:
// "Ana Ruiz ya da Matemáticas a 3.º A el lunes de 08:00 a 08:50".
func ClashError(kind ClashKind, other ScheduledClass, n ScheduleNames) error {
	if kind == ClashGroup {
		return refusal(glerrors.KindConflict, CodeScheduleGroupBusy, groupBusySentence(other, n))
	}
	return refusal(glerrors.KindConflict, CodeScheduleTeacherBusy, teacherBusySentence(other, n))
}

// OverrideConflictError is the refusal for a base edit that would break a
// change recorded for a date: on that date it would collide with other.
func OverrideConflictError(date time.Time, kind ClashKind, other ScheduledClass, n ScheduleNames) error {
	sentence := teacherBusySentence(other, n)
	if kind == ClashGroup {
		sentence = groupBusySentence(other, n)
	}
	return refusal(glerrors.KindConflict, CodeScheduleOverrideConflict,
		fmt.Sprintf("Con los cambios del %s chocaría: %s", FormatSpanishDate(date), sentence))
}

func teacherBusySentence(c ScheduledClass, n ScheduleNames) string {
	return fmt.Sprintf("%s ya da %s a %s el %s de %s a %s", n.Teacher(c.TeacherID), n.Subject(c.Block.subjectID),
		n.Group(c.Block.groupID), WeekdayName(c.Block.weekday), FormatClock(c.Time.start), FormatClock(c.Time.end))
}

func groupBusySentence(c ScheduledClass, n ScheduleNames) string {
	return fmt.Sprintf("%s ya tiene %s con %s el %s de %s a %s", n.Group(c.Block.groupID), n.Subject(c.Block.subjectID),
		n.Teacher(c.TeacherID), WeekdayName(c.Block.weekday), FormatClock(c.Time.start), FormatClock(c.Time.end))
}

// --- substitutes ------------------------------------------------------------

// TeacherMatch is how well a free teacher fits a class.
type TeacherMatch string

// The fits, best first.
const (
	MatchSubject TeacherMatch = "subject"
	MatchGrade   TeacherMatch = "grade"
	MatchOther   TeacherMatch = "other"
)

var matchRank = map[TeacherMatch]int{MatchSubject: 0, MatchGrade: 1, MatchOther: 2}

// AvailableTeacher is a teacher free in a slot and how well they fit.
type AvailableTeacher struct {
	Teacher Teacher
	Match   TeacherMatch
}

// AvailableTeachers lists the teachers with no class in slot on day (the
// effective day: substitutes are busy where they cover), leaving excludeBlockID
// out of the busy check. Those who teach the subject (sameSubject, by any of
// their assignments) come first, then those who teach in the grade
// (sameGrade, by group), then the rest; by last then first name within each.
func AvailableTeachers(teachers []Teacher, day []ScheduledClass, slot ClassTime, excludeBlockID string,
	assignments []TeachingAssignment, sameSubject, sameGrade func(id string) bool,
) []AvailableTeacher {
	busy := make(map[string]bool)
	for _, c := range day {
		if c.Status != ClassCancelled && c.Block.id != excludeBlockID && c.Time.Overlaps(slot) {
			busy[c.TeacherID] = true
		}
	}
	match := make(map[string]TeacherMatch)
	for _, a := range assignments {
		switch {
		case sameSubject(a.subjectID):
			match[a.teacherID] = MatchSubject
		case sameGrade(a.groupID) && match[a.teacherID] != MatchSubject:
			match[a.teacherID] = MatchGrade
		}
	}
	out := make([]AvailableTeacher, 0, len(teachers))
	for _, t := range teachers {
		if busy[t.id] {
			continue
		}
		m, ok := match[t.id]
		if !ok {
			m = MatchOther
		}
		out = append(out, AvailableTeacher{Teacher: t, Match: m})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if ri, rj := matchRank[out[i].Match], matchRank[out[j].Match]; ri != rj {
			return ri < rj
		}
		if out[i].Teacher.lastName != out[j].Teacher.lastName {
			return out[i].Teacher.lastName < out[j].Teacher.lastName
		}
		return out[i].Teacher.firstName < out[j].Teacher.firstName
	})
	return out
}

// --- history ----------------------------------------------------------------

// ScheduleAction names an entry of the timetable's history.
type ScheduleAction string

// The history's actions.
const (
	ActionBlockCreated    ScheduleAction = "block_created"
	ActionBlockUpdated    ScheduleAction = "block_updated"
	ActionBlockDeleted    ScheduleAction = "block_deleted"
	ActionOverrideCreated ScheduleAction = "override_created"
	ActionOverrideDeleted ScheduleAction = "override_deleted"
)

// ScheduleEvent is one entry of the timetable's history: who, when, what and
// why. The summary is written at the time, with the names of the time.
type ScheduleEvent struct {
	ID         string
	TenantID   string
	CycleID    string
	At         time.Time
	By         Actor
	Action     ScheduleAction
	BlockID    string
	OverrideID string
	Date       time.Time // zero unless the entry is about a date-specific change
	Reason     string
	Summary    string
}

func blockLine(b ScheduleBlock, n ScheduleNames) string {
	return fmt.Sprintf("%s, %s, %s %s, %s", n.Subject(b.subjectID), n.Group(b.groupID), WeekdayName(b.weekday), b.time, n.Teacher(b.teacherID))
}

// BlockSummary is "<prefix>: Matemáticas, 3.º A, lunes 08:00–08:50, Ana Ruiz".
func BlockSummary(prefix string, b ScheduleBlock, n ScheduleNames) string {
	return prefix + ": " + blockLine(b, n)
}

// BlockChangeSummary describes a base edit, before → after.
func BlockChangeSummary(before, after ScheduleBlock, n ScheduleNames) string {
	return fmt.Sprintf("Clase cambiada: %s → %s %s, %s", blockLine(before, n), WeekdayName(after.weekday), after.time, n.Teacher(after.teacherID))
}

// OverrideSummary describes a date-specific change; swapped is the other class
// of a swap (nil otherwise).
func OverrideSummary(o ScheduleOverride, b ScheduleBlock, swapped *ScheduleBlock, n ScheduleNames) string {
	date := FormatSpanishDate(o.date)
	switch o.kind {
	case OverrideSubstitute:
		return fmt.Sprintf("Suplencia el %s: %s, %s, %s — %s en lugar de %s", date, n.Subject(b.subjectID), n.Group(b.groupID),
			b.time, n.Teacher(o.teacherID), n.Teacher(b.teacherID))
	case OverrideSwap:
		if swapped == nil {
			return fmt.Sprintf("Intercambio el %s, %s: %s (%s)", date, n.Group(b.groupID), n.Subject(b.subjectID), b.time)
		}
		return fmt.Sprintf("Intercambio el %s, %s: %s (%s) ↔ %s (%s)", date, n.Group(b.groupID), n.Subject(b.subjectID), b.time,
			n.Subject(swapped.subjectID), swapped.time)
	default:
		return fmt.Sprintf("Clase cancelada el %s: %s, %s, %s", date, n.Subject(b.subjectID), n.Group(b.groupID), b.time)
	}
}
