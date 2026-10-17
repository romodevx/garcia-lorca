package service

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// dateLayout is the ISO form every date crosses the boundary in: attendance is
// per school day, so no time-of-day is ever accepted or produced.
const dateLayout = "2006-01-02"

// The stored attendance vocabulary. Only the exceptions reach the database:
// statusPresent is what a client sends to CLEAR a mark (see the package doc).
const (
	statusPresent = "present"
	statusAbsent  = "absent"
	statusLate    = "late"
	statusExcused = "excused"
)

// enrollmentStatusActive is the only enrollment status that appears on a roster.
const enrollmentStatusActive = "active"

// maxNoteLength bounds, in CHARACTERS (runes, like the column CHECK length()
// and the web counter), the free-text justification a client may attach to a
// mark. The column carries the same CHECK, so the cap holds even for a writer
// that bypasses this layer.
const maxNoteLength = 500

// maxSubjectNameLength bounds, in characters, the per-class list label a save
// may carry onto its events (it is not stored).
const maxSubjectNameLength = 200

// Bounds of a ListSessions query: it serves a family's month view, so two whole
// months and a family's handful of groups are the widest honest read.
const (
	maxSessionRangeDays = 62
	maxSessionGroups    = 20
)

// maxTakerNameLength bounds the display-name snapshot stamped on a session; the
// column carries the same CHECK.
const maxTakerNameLength = 200

// --- seams -------------------------------------------------------------------
//
// The interfaces below are consumer-defined: attendance declares the narrow
// view it needs and the adapters (students/config Connect clients, the GORM
// repository, the Pub/Sub publisher) satisfy them from the outside. Attendance
// owns no enrollments and no groups — it validates both by RPC and stores only
// marks.

// EnrollmentRef is the attendance-owned view of a students enrollment: enough to
// place a student on a group's roster and know whether the enrollment is live.
// StudentID is carried into the roster and the published events; names are
// resolved in the web from the students list, so attendance stays free of PII.
type EnrollmentRef struct {
	ID        string
	StudentID string
	GroupID   string
	Status    string
	// EnrolledAt / EndedAt bound the enrollment (EndedAt zero while active):
	// the report counts a pupil only on lists taken in between (T12).
	EnrolledAt time.Time
	EndedAt    time.Time
}

// TransferIn is a move of an enrollment INTO a group. A transfer rewrites the
// enrollment's group in place, so this is how the report learns when a pupil
// joined the group.
type TransferIn struct {
	EnrollmentID string
	MovedAt      time.Time
}

// EnrollmentReader lists a group's enrollments from the students bounded context
// (by RPC). A missing group is reported as glerrors.ErrNotFound; any other error
// propagates unchanged.
type EnrollmentReader interface {
	ByGroup(ctx context.Context, groupID string) ([]EnrollmentRef, error)
	// TransfersInto lists every move into the group, in one read (T12).
	TransfersInto(ctx context.Context, groupID string) ([]TransferIn, error)
}

// GroupReader validates a group reference against the config bounded context (by
// RPC). Attendance needs existence only — the roster itself comes from students
// — so this returns nothing but an error: glerrors.ErrNotFound when the group
// does not exist.
type GroupReader interface {
	ByID(ctx context.Context, id string) error
	// WritableByID is ByID for a write: it also resolves the group's level and
	// refuses one whose deletion runs with levelarchive.LevelBeingDeleted (409
	// LEVEL_BEING_DELETED, ADR 0021).
	WritableByID(ctx context.Context, id string) error
}

// AttendanceRecord is one stored attendance exception: an enrollment, the school
// day it applies to, the list's subject ("" for a daily list), its status (never
// statusPresent) and an optional note.
type AttendanceRecord struct {
	ID           string
	EnrollmentID string
	Date         time.Time
	SubjectID    string
	Status       string
	Note         string
	// Revision is the stored row's version: 1 when inserted, bumped by every
	// in-place write. The repository reports it as STORED (never the plan's
	// guess); it is what a mark event's version is made of.
	Revision int64
}

// ListKey identifies one pass list: a group's school day and, for a per-class
// list, the subject. An empty SubjectID is the daily list. Which of the two a
// group uses is its level's attendance mode, enforced at the gateway: this
// service stores whichever key it is given.
type ListKey struct {
	GroupID   string
	Date      time.Time
	SubjectID string
}

// Session records that a list was taken: TakenAt/TakenByName are its first save,
// UpdatedAt its latest correction.
type Session struct {
	TakenAt     time.Time
	TakenByName string
	UpdatedAt   time.Time
}

// Taker is who saved a list: the iam user id and a display-name snapshot.
type Taker struct {
	UserID string
	Name   string
}

// RecordUpdate is a stored exception rewritten in place by a correction: Record
// carries the STORED id. StatusChanged tells a notifying change from a note-only
// edit.
type RecordUpdate struct {
	Record        AttendanceRecord
	StatusChanged bool
	// PreviousStatus is the stored status the update replaced ("" when the
	// repository could not know it: a planned insert that met a row).
	PreviousStatus string
}

// ListChanges is what a save does to a list: exceptions to insert, stored ones
// to update in place, and stored ones cleared back to present (deleted).
//
// HeldElsewhere is filled by the repository only: the enrollment ids whose
// planned mark was NOT stored because another group's list holds that
// pupil·day·subject (see the repository's ownership rule, attendance/0004).
type ListChanges struct {
	Inserts       []AttendanceRecord
	Updates       []RecordUpdate
	Deletes       []AttendanceRecord
	HeldElsewhere []string
}

// SaveResult summarizes the list AFTER a save, from what was stored rather
// than what was submitted: AbsentCount stored marks are absences, MarkedCount
// exceptions are stored for this list, and HeldElsewhere (ascending) are the
// submitted enrollments skipped because another group's list holds their mark.
type SaveResult struct {
	AbsentCount   int
	MarkedCount   int
	HeldElsewhere []string
}

// PlanFunc turns a list's CURRENT stored marks (keyed by enrollment id) into the
// changes to apply. The repository calls it inside the reconcile transaction, so
// the plan is computed against the rows it is about to change.
type PlanFunc func(current map[string]AttendanceRecord) (ListChanges, error)

// Repo stores the attendance marks. All access is tenant-scoped for RLS (the
// adapter binds app.current_tenant per transaction, as in students/grades).
type Repo interface {
	// MarksForList batch-loads the stored marks of many enrollments on one list
	// (key's date and subject) in a single query, so building a roster never fans
	// out into a lookup per student (N+1). The result is keyed by enrollment_id
	// and holds an entry ONLY for enrollments with a stored exception; present
	// students are simply absent from the map. An empty enrollmentIDs slice
	// yields an empty map without touching the database.
	MarksForList(ctx context.Context, enrollmentIDs []string, key ListKey) (map[string]AttendanceRecord, error)
	// SessionFor returns the list's session; found is false for a list nobody
	// has saved.
	SessionFor(ctx context.Context, key ListKey) (s Session, found bool, err error)
	// ReconcileList applies a save to one list in ONE transaction: it loads the
	// list's current marks for enrollmentIDs, asks plan for the changes, applies
	// them (inserts and in-place updates as one upsert, deletes by id) and
	// upserts the list's session for taker (the first taker is kept). It returns
	// the changes applied. Atomic on purpose — a partially reconciled list would
	// silently misreport who was present.
	ReconcileList(ctx context.Context, key ListKey, enrollmentIDs []string, taker Taker, plan PlanFunc) (ListChanges, error)
	// ListByEnrollment returns one enrollment's stored marks within an inclusive
	// date range, ordered by date. A zero from or to skips that bound.
	ListByEnrollment(ctx context.Context, enrollmentID string, from, to time.Time) ([]AttendanceRecord, error)
	// ListSessions returns the lists TAKEN for any of groupIDs between two
	// inclusive days, newest first, in one query.
	ListSessions(ctx context.Context, groupIDs []string, from, to time.Time) ([]TakenList, error)
	// GroupHistory reads, in ONE transaction, the days of the group's lists
	// taken between two inclusive days (one entry per list) and the exceptions
	// of enrollmentIDs on the group's lists in that span: marks stamped with
	// this group, plus legacy marks (no group, stored before lists carried one).
	GroupHistory(ctx context.Context, groupID string, enrollmentIDs []string, from, to time.Time) (GroupHistory, error)
}

// ExceptionCounts is how many of a pupil's marks were each stored exception.
type ExceptionCounts struct {
	Absent  int
	Late    int
	Excused int
}

// DayMark is one stored exception of a report's span.
type DayMark struct {
	EnrollmentID string
	Date         time.Time
	Status       string
}

// GroupHistory is what the repository reads for a report: the day of every
// list the group took, and the exceptions on them.
type GroupHistory struct {
	Lists []time.Time
	Marks []DayMark
}

// StudentCounts is one pupil's line of the report: the lists taken while they
// were in the group, and their exceptions on them.
type StudentCounts struct {
	EnrollmentID string
	StudentID    string
	// Active is false for a pupil who left the group during the range.
	Active bool
	Lists  int
	ExceptionCounts
}

// AttendanceSummary is a group's attendance over a range: the lists taken and
// one line per pupil who was in the group at some point of it, ordered by
// enrollment id.
type AttendanceSummary struct {
	ListsTaken int
	Students   []StudentCounts
}

// TakenList is one list that was taken: its group, school day and subject (""
// for the daily list).
type TakenList struct {
	GroupID   string
	Date      time.Time
	SubjectID string
}

// AbsenceEvent announces that a student was recorded absent on a date (feeds
// comms M10). Date is the ISO school day.
type AbsenceEvent struct {
	RecordID     string
	EnrollmentID string
	StudentID    string
	TenantID     string
	Date         string
	SubjectID    string
	SubjectName  string // the per-class list's label, "" for daily (never stored)
	MarkVersion  int64  // the mark's stored revision; with RecordID, the consumer's dedupe key
	// PreviousStatus is the exception this absence replaced (late → absent), ""
	// for a new absence.
	PreviousStatus string
}

// MarkedEvent announces that a student's mark CHANGED on a list: a new
// exception, a changed status, or — Status statusPresent — a stored exception
// cleared by a correction. It is the general companion of AbsenceEvent.
// SubjectID is empty for a daily list.
type MarkedEvent struct {
	RecordID     string
	EnrollmentID string
	StudentID    string
	TenantID     string
	Date         string
	Status       string
	SubjectID    string
	SubjectName  string // the per-class list's label, "" for daily (never stored)
	MarkVersion  int64  // the mark's stored revision; with RecordID, the consumer's dedupe key
	// PreviousStatus is the stored status this change replaced (an in-place
	// correction or a clear), "" for a new exception.
	PreviousStatus string
}

// Publisher announces stored attendance on the messaging boundary. Publishing is
// best-effort (see publishChanges): a nil Publisher disables it entirely.
type Publisher interface {
	PublishAbsenceRecorded(ctx context.Context, e AbsenceEvent) error
	PublishAttendanceMarked(ctx context.Context, e MarkedEvent) error
}

// --- API types ---------------------------------------------------------------

// RosterRow is one student's slot in a day's pass list: the enrollment, the
// student it belongs to, and the EFFECTIVE status for that day — the stored
// exception if there is one, statusPresent otherwise.
type RosterRow struct {
	EnrollmentID string
	StudentID    string
	Status       string
	Note         string
}

// Roster is one list's rows plus its session (nil when nobody has taken it).
type Roster struct {
	Rows    []RosterRow
	Session *Session
	// NoClasses is true when the date has no classes for the group (T14): the
	// list may be read, but a save is refused with NO_CLASSES.
	NoClasses bool
}

// SaveRequest is one save of a list: the group·day, the subject for a per-class
// list ("" for daily), the COMPLETE set of exceptions, and who is saving.
type SaveRequest struct {
	GroupID     string
	Date        string
	SubjectID   string
	Marks       []Mark
	TakenBy     string
	TakenByName string
	// SubjectName is the per-class list's display label, carried onto the
	// published events only (comms names the class). Ignored for a daily list.
	SubjectName string
}

// Mark is one submitted exception for a group·day. A Mark with statusPresent
// clears whatever was stored for that enrollment instead of storing a row.
type Mark struct {
	EnrollmentID string
	Status       string
	Note         string
}

// --- service -----------------------------------------------------------------

// AttendanceService builds a group's daily pass list and reconciles a saved day.
// It orchestrates three seams: the local Repo, the cross-service EnrollmentReader
// (students, by RPC) and GroupReader (config, by RPC) — all faked in tests — and
// announces stored marks through an optional Publisher. Ids come from an
// injected newID so tests are deterministic; the day itself always comes from the
// caller's date, never from a clock here.
//
//	Save: group (RPC) ─> active enrollments (RPC) ─> validate marks
//	      ─> reconcile the whole day in one tx ─> publish (best-effort)
type AttendanceService struct {
	repo        Repo
	enrollments EnrollmentReader
	groups      GroupReader
	publisher   Publisher
	tenantID    string
	newID       func() string
	log         *slog.Logger
	// school is the school's time zone: an enrollment's instants are read as
	// days on its calendar, the one the lists are taken on (default UTC).
	school *time.Location
	// calendar refuses saves on days without classes (T14); nil = none refused.
	calendar SchoolCalendar
}

// Option configures an AttendanceService (functional options).
type Option func(*AttendanceService)

// WithSchoolLocation sets the school's time zone, so a pupil enrolled at 18:00
// in Mexico City is in the group for that day's list, not the next one (T12).
func WithSchoolLocation(loc *time.Location) Option {
	return func(s *AttendanceService) { s.school = loc }
}

// NewAttendanceService wires the service. tenantID is the single-tenant scope for
// this phase and is stamped on every published event; publisher may be nil, which
// disables event publishing. log receives the best-effort publishing failures
// (the only thing this layer logs); a nil log falls back to slog.Default so a
// dropped event is never silent.
func NewAttendanceService(
	repo Repo,
	enrollments EnrollmentReader,
	groups GroupReader,
	publisher Publisher,
	tenantID string,
	newID func() string,
	log *slog.Logger,
	opts ...Option,
) *AttendanceService {
	if log == nil {
		log = slog.Default()
	}
	s := &AttendanceService{
		repo:        repo,
		enrollments: enrollments,
		groups:      groups,
		publisher:   publisher,
		tenantID:    tenantID,
		newID:       newID,
		log:         log,
		school:      time.UTC,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// GetDailyRoster returns one row per ACTIVE enrollment in the group for one list
// (the day's daily list, or a subject's per-class list), each carrying its
// effective status and note: the stored exception, or statusPresent when nothing
// is stored (default-present). Every mark is resolved in ONE batched read, and
// the list's session comes along (nil when nobody has taken it). Rows are
// ordered by enrollment id, deterministically.
func (s *AttendanceService) GetDailyRoster(ctx context.Context, groupID, dateStr, subjectID string) (Roster, error) {
	// The pure checks first: a malformed date or subject costs nothing to reject
	// and must not be reported as "group not found" when the request is wrong in
	// both.
	key, err := parseListKey(groupID, dateStr, subjectID)
	if err != nil {
		return Roster{}, err
	}
	if err := s.requireGroup(ctx, groupID); err != nil {
		return Roster{}, err
	}
	schoolDay, err := s.schoolDayOf(ctx, key)
	if err != nil {
		return Roster{}, err
	}
	enrollmentIDs, studentByEnrollment, err := s.activeRoster(ctx, groupID)
	if err != nil {
		return Roster{}, err
	}

	marks, err := s.repo.MarksForList(ctx, enrollmentIDs, key)
	if err != nil {
		return Roster{}, err
	}
	session, found, err := s.repo.SessionFor(ctx, key)
	if err != nil {
		return Roster{}, err
	}

	out := Roster{Rows: make([]RosterRow, 0, len(enrollmentIDs)), NoClasses: !schoolDay.Open}
	if found {
		out.Session = &session
	}
	for _, enrollmentID := range enrollmentIDs {
		row := RosterRow{
			EnrollmentID: enrollmentID,
			StudentID:    studentByEnrollment[enrollmentID],
			Status:       statusPresent,
		}
		if mark, ok := marks[enrollmentID]; ok {
			row.Status = mark.Status
			row.Note = mark.Note
		}
		out.Rows = append(out.Rows, row)
	}
	return out, nil
}

// SaveDailyAttendance reconciles one list: req.Marks is the COMPLETE set of
// exceptions, so every active enrollment missing from it — and every enrollment
// explicitly marked statusPresent — ends the call present. A correction changes
// the stored rows IN PLACE (their ids survive) and publishes events only for
// what changed (see DiffList), so re-saving never re-notifies the parents. The
// list's session is recorded for the taker. The result is counted from what the
// reconcile STORED (see SaveResult): a submitted mark the repository skipped
// because another group's list holds it is reported in HeldElsewhere, never
// counted. The batch is validated before anything is written, so a rejected
// save leaves the list untouched.
//
// Known limitation: a mark stored before attendance/0004 has no group
// (group_id NULL) and is treated as this list's own, so any list of its day
// may read, correct or clear it (ADR 0018). Such legacy rows exist only in
// local dev databases today.
func (s *AttendanceService) SaveDailyAttendance(ctx context.Context, req SaveRequest) (SaveResult, error) {
	// Cheap pure checks before the network round trip (see GetDailyRoster).
	key, err := parseListKey(req.GroupID, req.Date, req.SubjectID)
	if err != nil {
		return SaveResult{}, err
	}
	taker, err := parseTaker(req.TakenBy, req.TakenByName)
	if err != nil {
		return SaveResult{}, err
	}
	label, err := parseSubjectName(key.SubjectID, req.SubjectName)
	if err != nil {
		return SaveResult{}, err
	}
	if err := s.requireWritableGroup(ctx, req.GroupID); err != nil {
		return SaveResult{}, err
	}
	if err := s.requireSchoolDay(ctx, key); err != nil {
		return SaveResult{}, err
	}
	enrollmentIDs, studentByEnrollment, err := s.activeRoster(ctx, req.GroupID)
	if err != nil {
		return SaveResult{}, err
	}

	desired, err := s.recordsFor(req.Marks, studentByEnrollment, key)
	if err != nil {
		return SaveResult{}, err
	}

	changes, err := s.repo.ReconcileList(ctx, key, enrollmentIDs, taker, func(current map[string]AttendanceRecord) (ListChanges, error) {
		return DiffList(current, desired), nil
	})
	if err != nil {
		return SaveResult{}, err
	}

	s.publishChanges(ctx, changes, eventContext{students: studentByEnrollment, subjectName: label})
	return summarizeStored(desired, changes.HeldElsewhere), nil
}

// summarizeStored counts the list's stored state after a reconcile. desired is
// the complete set of exceptions the save asked for; every one of them ends
// stored (inserted, updated or already identical) EXCEPT those held by another
// group's list, so the stored state is desired minus held.
func summarizeStored(desired []AttendanceRecord, held []string) SaveResult {
	skipped := make(map[string]bool, len(held))
	for _, id := range held {
		skipped[id] = true
	}
	var out SaveResult
	for _, rec := range desired {
		if skipped[rec.EnrollmentID] {
			continue
		}
		out.MarkedCount++
		if rec.Status == statusAbsent {
			out.AbsentCount++
		}
	}
	if len(held) > 0 {
		out.HeldElsewhere = append([]string(nil), held...)
		sort.Strings(out.HeldElsewhere)
	}
	return out
}

// DiffList computes what a save does to a list. current is the list's stored
// marks keyed by enrollment id; desired is the complete set of exceptions the
// save asks for, each with a freshly minted id. A desired mark with no stored
// row is an INSERT; one whose status or note differs from the stored row is an
// UPDATE that keeps the STORED id (StatusChanged tells a notifying change from a
// note-only edit); an identical one produces nothing. A stored row missing from
// desired is a DELETE — the student is present again. Inserts and updates follow
// desired's order, deletes are ordered by enrollment id, so the result is
// deterministic.
func DiffList(current map[string]AttendanceRecord, desired []AttendanceRecord) ListChanges {
	var changes ListChanges
	kept := make(map[string]bool, len(desired))
	for _, want := range desired {
		kept[want.EnrollmentID] = true
		have, stored := current[want.EnrollmentID]
		switch {
		case !stored:
			changes.Inserts = append(changes.Inserts, want)
		case have.Status != want.Status || have.Note != want.Note:
			want.ID = have.ID
			changes.Updates = append(changes.Updates, RecordUpdate{Record: want, StatusChanged: have.Status != want.Status, PreviousStatus: have.Status})
		}
	}
	cleared := make([]string, 0, len(current))
	for enrollmentID := range current {
		if !kept[enrollmentID] {
			cleared = append(cleared, enrollmentID)
		}
	}
	sort.Strings(cleared)
	for _, enrollmentID := range cleared {
		changes.Deletes = append(changes.Deletes, current[enrollmentID])
	}
	return changes
}

// ListAttendance returns one enrollment's stored marks between two optional ISO
// bounds (an empty bound is unbounded). Only exceptions are stored, so present
// days are absent from the result by construction. The enrollment id is checked
// here rather than left to Postgres: the column is a uuid, so a malformed id
// would otherwise surface as an infrastructure error for a plain client mistake.
func (s *AttendanceService) ListAttendance(ctx context.Context, enrollmentID, fromStr, toStr string) ([]AttendanceRecord, error) {
	if err := uuid.Validate(enrollmentID); err != nil {
		return nil, glerrors.New(glerrors.KindInvalid, "invalid enrollment id")
	}
	from, err := parseOptionalDate(fromStr)
	if err != nil {
		return nil, err
	}
	to, err := parseOptionalDate(toStr)
	if err != nil {
		return nil, err
	}
	return s.repo.ListByEnrollment(ctx, enrollmentID, from, to)
}

// ListSessions returns which lists were taken for groupIDs between two required
// inclusive ISO days, newest first. The query is bounded at the boundary — at
// most maxSessionGroups distinct group UUIDs and maxSessionRangeDays days — so a
// caller can never turn it into a scan of the whole history.
func (s *AttendanceService) ListSessions(ctx context.Context, groupIDs []string, fromStr, toStr string) ([]TakenList, error) {
	groups, err := distinctGroupIDs(groupIDs)
	if err != nil {
		return nil, err
	}
	from, to, err := parseRange(fromStr, toStr, maxSessionRangeDays)
	if err != nil {
		return nil, err
	}
	return s.repo.ListSessions(ctx, groups, from, to)
}

// maxSummaryRangeDays bounds a report: a school year (leap day included).
const maxSummaryRangeDays = 366

// SummarizeAttendance is the T12 report's read: the group's lists taken between
// two required inclusive ISO days and, for every pupil who was in the group at
// some point of that span, the lists taken while they were in it — from
// enrolling, or their latest move into the group, to leaving it — and their
// exceptions on those lists. Present is what is left, so it is not computed
// here (default-present model). A mark outside a pupil's window (a legacy mark
// from the group they came from) is not this group's. The range is bounded at
// the boundary.
func (s *AttendanceService) SummarizeAttendance(ctx context.Context, groupID, fromStr, toStr string) (AttendanceSummary, error) {
	from, to, err := parseRange(fromStr, toStr, maxSummaryRangeDays)
	if err != nil {
		return AttendanceSummary{}, err
	}
	if err := s.requireGroup(ctx, groupID); err != nil {
		return AttendanceSummary{}, err
	}
	pupils, err := s.pupilsDuring(ctx, groupID, from, to)
	if err != nil {
		return AttendanceSummary{}, err
	}
	ids := make([]string, 0, len(pupils))
	for _, p := range pupils {
		ids = append(ids, p.enrollment.ID)
	}
	history, err := s.repo.GroupHistory(ctx, groupID, ids, from, to)
	if err != nil {
		return AttendanceSummary{}, err
	}
	return summarize(pupils, history), nil
}

// window is the span of days a pupil belonged to the group, clipped to a range.
type window struct{ from, to time.Time }

func (w window) holds(day time.Time) bool { return !day.Before(w.from) && !day.After(w.to) }

// pupilInGroup is an enrollment and the days of the range it was in the group.
type pupilInGroup struct {
	enrollment EnrollmentRef
	window     window
}

// pupilsDuring lists, by enrollment id, every enrollment of the group whose
// time in it overlaps [from, to]: it starts on the day of enrolling or of the
// latest move into the group, and ends on the day it ended (open while
// active). An inactive enrollment without an end is skipped: its span is
// unknown.
func (s *AttendanceService) pupilsDuring(ctx context.Context, groupID string, from, to time.Time) ([]pupilInGroup, error) {
	enrollments, err := s.enrollments.ByGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	moves, err := s.enrollments.TransfersInto(ctx, groupID)
	if err != nil {
		return nil, err
	}
	joined := make(map[string]time.Time, len(moves))
	for _, m := range moves {
		if day := s.schoolDay(m.MovedAt); day.After(joined[m.EnrollmentID]) {
			joined[m.EnrollmentID] = day
		}
	}
	out := make([]pupilInGroup, 0, len(enrollments))
	for _, enr := range enrollments {
		active := enr.Status == enrollmentStatusActive
		if !active && enr.EndedAt.IsZero() {
			continue
		}
		w := window{from: latest(from, s.schoolDay(enr.EnrolledAt), joined[enr.ID]), to: to}
		if !active {
			w.to = earliest(to, s.schoolDay(enr.EndedAt))
		}
		if w.from.After(w.to) {
			continue
		}
		out = append(out, pupilInGroup{enrollment: enr, window: w})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].enrollment.ID < out[j].enrollment.ID })
	return out, nil
}

// summarize counts each pupil's lists and exceptions inside their window.
func summarize(pupils []pupilInGroup, history GroupHistory) AttendanceSummary {
	marksOf := make(map[string][]DayMark, len(pupils))
	for _, m := range history.Marks {
		marksOf[m.EnrollmentID] = append(marksOf[m.EnrollmentID], m)
	}
	out := AttendanceSummary{ListsTaken: len(history.Lists), Students: make([]StudentCounts, 0, len(pupils))}
	for _, p := range pupils {
		line := StudentCounts{EnrollmentID: p.enrollment.ID, StudentID: p.enrollment.StudentID, Active: p.enrollment.Status == enrollmentStatusActive}
		for _, day := range history.Lists {
			if p.window.holds(day) {
				line.Lists++
			}
		}
		for _, m := range marksOf[p.enrollment.ID] {
			if p.window.holds(m.Date) {
				line.ExceptionCounts = line.add(m.Status)
			}
		}
		out.Students = append(out.Students, line)
	}
	return out
}

// add counts one more mark of status.
func (c ExceptionCounts) add(status string) ExceptionCounts {
	switch status {
	case statusAbsent:
		c.Absent++
	case statusLate:
		c.Late++
	case statusExcused:
		c.Excused++
	}
	return c
}

// schoolDay is an instant's day on the school's calendar, as the midnight-UTC
// date the lists are keyed by; the zero time stays zero.
func (s *AttendanceService) schoolDay(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	local := t.In(s.school)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

func latest(first time.Time, rest ...time.Time) time.Time {
	for _, t := range rest {
		if t.After(first) {
			first = t
		}
	}
	return first
}

func earliest(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

// parseRange parses two required inclusive ISO days spanning at most maxDays.
func parseRange(fromStr, toStr string, maxDays int) (time.Time, time.Time, error) {
	from, err := parseDate(fromStr)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	to, err := parseDate(toStr)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if to.Before(from) {
		return time.Time{}, time.Time{}, glerrors.New(glerrors.KindInvalid, "to_date is before from_date")
	}
	if days := int(to.Sub(from).Hours()/hoursPerDay) + 1; days > maxDays {
		return time.Time{}, time.Time{}, glerrors.New(glerrors.KindInvalid, fmt.Sprintf("the range spans more than %d days", maxDays))
	}
	return from, to, nil
}

// hoursPerDay converts a UTC-midnight difference to days (no DST in UTC).
const hoursPerDay = 24

// distinctGroupIDs validates a ListSessions group set: 1..maxSessionGroups
// UUIDs, duplicates collapsed in first-seen order.
func distinctGroupIDs(groupIDs []string) ([]string, error) {
	out := make([]string, 0, len(groupIDs))
	seen := make(map[string]bool, len(groupIDs))
	for _, id := range groupIDs {
		if uuid.Validate(id) != nil {
			return nil, glerrors.New(glerrors.KindInvalid, "invalid group id")
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if len(out) == 0 || len(out) > maxSessionGroups {
		return nil, glerrors.New(glerrors.KindInvalid, "between 1 and 20 group ids are required")
	}
	return out, nil
}

// parseSubjectName validates the list label a save carries onto its events:
// trimmed, at most maxSubjectNameLength characters, and dropped for a daily
// list (which has no class to name).
func parseSubjectName(subjectID, name string) (string, error) {
	if subjectID == "" {
		return "", nil
	}
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > maxSubjectNameLength {
		return "", glerrors.New(glerrors.KindInvalid, "subject_name is too long (at most 200 characters)")
	}
	return name, nil
}

// recordsFor validates the submitted marks against the day's active roster and
// projects the exceptions into storable records. A mark for an enrollment that is
// not on the roster, an unknown status, an over-long note or a second mark for
// the same enrollment (the stored key is one mark per enrollment·day) is a client
// error; a statusPresent mark is dropped, since being present is the absence of a
// record.
func (s *AttendanceService) recordsFor(marks []Mark, studentByEnrollment map[string]string, key ListKey) ([]AttendanceRecord, error) {
	records := make([]AttendanceRecord, 0, len(marks))
	seen := make(map[string]bool, len(marks))
	for _, mark := range marks {
		if _, onRoster := studentByEnrollment[mark.EnrollmentID]; !onRoster {
			return nil, glerrors.New(glerrors.KindInvalid, "enrollment not in group")
		}
		if !isStorableStatus(mark.Status) && mark.Status != statusPresent {
			return nil, glerrors.New(glerrors.KindInvalid, "invalid attendance status")
		}
		if utf8.RuneCountInString(mark.Note) > maxNoteLength {
			return nil, glerrors.New(glerrors.KindInvalid, "note is too long")
		}
		if seen[mark.EnrollmentID] {
			return nil, glerrors.New(glerrors.KindInvalid, "duplicate mark for enrollment")
		}
		seen[mark.EnrollmentID] = true

		if mark.Status == statusPresent {
			continue // present is never stored: the mark is being cleared
		}
		records = append(records, AttendanceRecord{
			ID:           s.newID(),
			EnrollmentID: mark.EnrollmentID,
			Date:         key.Date,
			SubjectID:    key.SubjectID,
			Status:       mark.Status,
			Note:         mark.Note,
		})
	}
	return records, nil
}

// publishChanges announces what a save CHANGED, and nothing else, so a
// correction never re-notifies the parents: one AttendanceMarked per inserted
// exception, per status change and — with statusPresent — per cleared exception;
// plus one AbsenceRecorded (what comms M10 listens for) whenever a mark BECAME
// absent. A note-only edit is silent. A nil publisher is a no-op and a failure
// never fails the call — the list is already durable, and losing a notification
// is strictly better than failing a teacher's save — but every dropped event is
// logged, so best-effort stays diagnosable instead of silent.
func (s *AttendanceService) publishChanges(ctx context.Context, changes ListChanges, ec eventContext) {
	if s.publisher == nil {
		return
	}
	for _, rec := range changes.Inserts {
		s.publishMark(ctx, rec, markChange{status: rec.Status, version: rec.Revision}, ec)
	}
	for _, u := range changes.Updates {
		if u.StatusChanged {
			s.publishMark(ctx, u.Record, markChange{status: u.Record.Status, previous: u.PreviousStatus, version: u.Record.Revision}, ec)
		}
	}
	for _, rec := range changes.Deletes {
		s.publishMark(ctx, rec, markChange{status: statusPresent, previous: rec.Status, version: clearedVersion(rec)}, ec)
	}
}

// eventContext is what every event of one save shares: the roster's student
// ids and the list's label.
type eventContext struct {
	students    map[string]string
	subjectName string
}

// clearedVersion is the version announcing that a stored mark was cleared back
// to present: one past its last stored revision. The row is gone, and a later
// exception for the same student·day is a new row with a new id, so no other
// event can ever carry this (record id, version) pair.
func clearedVersion(rec AttendanceRecord) int64 {
	return rec.Revision + 1
}

// markChange is what one published mark announces: its new status, the stored
// status it replaced ("" for a new exception) and its version.
type markChange struct {
	status, previous string
	version          int64
}

// publishMark announces one changed mark as its status at its version, plus the
// absence companion (same version) when the status is absent. The version is
// what makes a correction a NEW event to a consumer that dedupes on (record id,
// version), while the record id stays the stored one.
func (s *AttendanceService) publishMark(ctx context.Context, rec AttendanceRecord, change markChange, ec eventContext) {
	status, version := change.status, change.version
	day := rec.Date.Format(dateLayout)
	studentID := ec.students[rec.EnrollmentID]
	err := s.publisher.PublishAttendanceMarked(ctx, MarkedEvent{
		RecordID:       rec.ID,
		EnrollmentID:   rec.EnrollmentID,
		StudentID:      studentID,
		TenantID:       s.tenantID,
		Date:           day,
		Status:         status,
		SubjectID:      rec.SubjectID,
		SubjectName:    ec.subjectName,
		MarkVersion:    version,
		PreviousStatus: change.previous,
	})
	s.logDroppedEvent(err, rec.ID)
	if status != statusAbsent {
		return
	}
	err = s.publisher.PublishAbsenceRecorded(ctx, AbsenceEvent{
		RecordID:       rec.ID,
		EnrollmentID:   rec.EnrollmentID,
		StudentID:      studentID,
		TenantID:       s.tenantID,
		Date:           day,
		SubjectID:      rec.SubjectID,
		SubjectName:    ec.subjectName,
		MarkVersion:    version,
		PreviousStatus: change.previous,
	})
	s.logDroppedEvent(err, rec.ID)
}

// logDroppedEvent records a best-effort publish that did not make it to the bus.
// The record id is the handle an operator replays from; no PII is logged.
func (s *AttendanceService) logDroppedEvent(err error, recordID string) {
	if err == nil {
		return
	}
	s.log.Warn("attendance event not published",
		slog.Any("error", err),
		slog.String("record_id", recordID))
}

// activeRoster resolves a group's ACTIVE enrollments once: the sorted enrollment
// ids (the deterministic roster order, and the exact scope a day's reconcile
// replaces) and their student ids.
func (s *AttendanceService) activeRoster(ctx context.Context, groupID string) ([]string, map[string]string, error) {
	enrollments, err := s.enrollments.ByGroup(ctx, groupID)
	if err != nil {
		return nil, nil, err
	}
	enrollmentIDs := make([]string, 0, len(enrollments))
	studentByEnrollment := make(map[string]string, len(enrollments))
	for _, enr := range enrollments {
		if enr.Status != enrollmentStatusActive {
			continue
		}
		enrollmentIDs = append(enrollmentIDs, enr.ID)
		studentByEnrollment[enr.ID] = enr.StudentID
	}
	sort.Strings(enrollmentIDs)
	return enrollmentIDs, studentByEnrollment, nil
}

// requireGroup asserts the group exists, mapping a config NotFound to
// KindInvalid (a bad client reference, never a NotFound leak) and propagating
// any other error.
func (s *AttendanceService) requireGroup(ctx context.Context, groupID string) error {
	if err := s.groups.ByID(ctx, groupID); err != nil {
		if glerrors.KindOf(err) == glerrors.KindNotFound {
			return glerrors.New(glerrors.KindInvalid, "group not found")
		}
		return err
	}
	return nil
}

// requireWritableGroup is requireGroup for a write: a group whose level is
// being deleted is refused (LEVEL_BEING_DELETED), so no mark lands after
// attendance's archive step of the deletion.
func (s *AttendanceService) requireWritableGroup(ctx context.Context, groupID string) error {
	if err := s.groups.WritableByID(ctx, groupID); err != nil {
		if glerrors.KindOf(err) == glerrors.KindNotFound {
			return glerrors.New(glerrors.KindInvalid, "group not found")
		}
		return err
	}
	return nil
}

// isStorableStatus reports whether a status is one of the exceptions attendance
// actually stores (statusPresent is deliberately excluded).
func isStorableStatus(status string) bool {
	switch status {
	case statusAbsent, statusLate, statusExcused:
		return true
	default:
		return false
	}
}

// parseDate parses a required ISO school day. Anything else — a locale format, a
// timestamp, an empty string — is a client error.
func parseDate(value string) (time.Time, error) {
	date, err := time.Parse(dateLayout, value)
	if err != nil {
		return time.Time{}, glerrors.New(glerrors.KindInvalid, "date must be an ISO date (YYYY-MM-DD)")
	}
	return date, nil
}

// parseListKey validates a list's key at the boundary: an ISO date and, for a
// per-class list, a subject UUID (the column is a uuid, so a malformed one would
// otherwise surface as an infrastructure error for a plain client mistake).
func parseListKey(groupID, dateStr, subjectID string) (ListKey, error) {
	date, err := parseDate(dateStr)
	if err != nil {
		return ListKey{}, err
	}
	if subjectID != "" && uuid.Validate(subjectID) != nil {
		return ListKey{}, glerrors.New(glerrors.KindInvalid, "invalid subject id")
	}
	return ListKey{GroupID: groupID, Date: date, SubjectID: subjectID}, nil
}

// parseTaker validates who is saving: an iam user UUID and a non-blank display
// name of at most maxTakerNameLength characters, stored trimmed.
func parseTaker(userID, name string) (Taker, error) {
	if uuid.Validate(userID) != nil {
		return Taker{}, glerrors.New(glerrors.KindInvalid, "invalid taken_by user id")
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxTakerNameLength {
		return Taker{}, glerrors.New(glerrors.KindInvalid, "taken_by_name is required (at most 200 characters)")
	}
	return Taker{UserID: userID, Name: name}, nil
}

// parseOptionalDate parses an ISO bound, mapping an empty string to the zero
// time — the repository's "no bound" signal.
func parseOptionalDate(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return parseDate(value)
}
