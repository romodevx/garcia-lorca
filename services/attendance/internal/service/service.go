package service

import (
	"context"
	"log/slog"
	"sort"
	"time"

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

// maxNoteLength bounds the free-text justification a client may attach to a
// mark. The column carries the same CHECK, so the cap holds even for a writer
// that bypasses this layer.
const maxNoteLength = 500

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
}

// EnrollmentReader lists a group's enrollments from the students bounded context
// (by RPC). A missing group is reported as glerrors.ErrNotFound; any other error
// propagates unchanged.
type EnrollmentReader interface {
	ByGroup(ctx context.Context, groupID string) ([]EnrollmentRef, error)
}

// GroupReader validates a group reference against the config bounded context (by
// RPC). Attendance needs existence only — the roster itself comes from students
// — so this returns nothing but an error: glerrors.ErrNotFound when the group
// does not exist.
type GroupReader interface {
	ByID(ctx context.Context, id string) error
}

// AttendanceRecord is one stored attendance exception: an enrollment, the school
// day it applies to, its status (never statusPresent) and an optional note.
type AttendanceRecord struct {
	ID           string
	EnrollmentID string
	Date         time.Time
	Status       string
	Note         string
}

// Repo stores the attendance marks. All access is tenant-scoped for RLS (the
// adapter binds app.current_tenant per transaction, as in students/grades).
type Repo interface {
	// MarksForDate batch-loads the stored marks for many enrollments on one date
	// in a single query, so building a roster never fans out into a lookup per
	// student (N+1). The result is keyed by enrollment_id and holds an entry ONLY
	// for enrollments with a stored exception; present students are simply absent
	// from the map. An empty enrollmentIDs slice yields an empty map without
	// touching the database.
	MarksForDate(ctx context.Context, enrollmentIDs []string, date time.Time) (map[string]AttendanceRecord, error)
	// ReconcileDay makes the stored state of one group·day match records, in ONE
	// transaction: every mark held by enrollmentIDs on that date is dropped and
	// records are inserted in its place. It is atomic on purpose — a partially
	// reconciled day would silently misreport who was present.
	ReconcileDay(ctx context.Context, enrollmentIDs []string, date time.Time, records []AttendanceRecord) error
	// ListByEnrollment returns one enrollment's stored marks within an inclusive
	// date range, ordered by date. A zero from or to skips that bound.
	ListByEnrollment(ctx context.Context, enrollmentID string, from, to time.Time) ([]AttendanceRecord, error)
}

// AbsenceEvent announces that a student was recorded absent on a date (feeds
// comms M10). Date is the ISO school day.
type AbsenceEvent struct {
	RecordID     string
	EnrollmentID string
	StudentID    string
	TenantID     string
	Date         string
}

// MarkedEvent announces that an attendance exception was stored for a student on
// a date. It is the general companion of AbsenceEvent: Status is one of
// absent|late|excused.
type MarkedEvent struct {
	RecordID     string
	EnrollmentID string
	StudentID    string
	TenantID     string
	Date         string
	Status       string
}

// Publisher announces stored attendance on the messaging boundary. Publishing is
// best-effort (see publishDay): a nil Publisher disables it entirely.
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
) *AttendanceService {
	if log == nil {
		log = slog.Default()
	}
	return &AttendanceService{
		repo:        repo,
		enrollments: enrollments,
		groups:      groups,
		publisher:   publisher,
		tenantID:    tenantID,
		newID:       newID,
		log:         log,
	}
}

// GetDailyRoster returns one row per ACTIVE enrollment in the group for the given
// day, each carrying its effective status: the stored exception, or statusPresent
// when nothing is stored (default-present). Every mark is resolved in ONE batched
// read. Rows are ordered by enrollment id, deterministically.
func (s *AttendanceService) GetDailyRoster(ctx context.Context, groupID, dateStr string) ([]RosterRow, error) {
	// The pure check first: a malformed date costs nothing to reject and must not
	// be reported as "group not found" when the request is wrong in both.
	date, err := parseDate(dateStr)
	if err != nil {
		return nil, err
	}
	if err := s.requireGroup(ctx, groupID); err != nil {
		return nil, err
	}
	enrollmentIDs, studentByEnrollment, err := s.activeRoster(ctx, groupID)
	if err != nil {
		return nil, err
	}

	marks, err := s.repo.MarksForDate(ctx, enrollmentIDs, date)
	if err != nil {
		return nil, err
	}

	rows := make([]RosterRow, 0, len(enrollmentIDs))
	for _, enrollmentID := range enrollmentIDs {
		row := RosterRow{
			EnrollmentID: enrollmentID,
			StudentID:    studentByEnrollment[enrollmentID],
			Status:       statusPresent,
		}
		if mark, ok := marks[enrollmentID]; ok {
			row.Status = mark.Status
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// SaveDailyAttendance reconciles a whole group·day: marks is the COMPLETE set of
// exceptions for that day, so every active enrollment missing from it — and every
// enrollment explicitly marked statusPresent — ends the call present. It returns
// how many stored marks are absences and how many exceptions were stored in
// total. The batch is validated before anything is written, so a rejected save
// leaves the day untouched.
func (s *AttendanceService) SaveDailyAttendance(ctx context.Context, groupID, dateStr string, marks []Mark) (absentCount, markedCount int, err error) {
	// Cheap pure check before the network round trip (see GetDailyRoster).
	date, err := parseDate(dateStr)
	if err != nil {
		return 0, 0, err
	}
	if err := s.requireGroup(ctx, groupID); err != nil {
		return 0, 0, err
	}
	enrollmentIDs, studentByEnrollment, err := s.activeRoster(ctx, groupID)
	if err != nil {
		return 0, 0, err
	}

	records, err := s.recordsFor(marks, studentByEnrollment, date)
	if err != nil {
		return 0, 0, err
	}

	// One transaction replaces the whole day: dropped marks become present again.
	if err := s.repo.ReconcileDay(ctx, enrollmentIDs, date, records); err != nil {
		return 0, 0, err
	}

	s.publishDay(ctx, records, studentByEnrollment, date)

	for _, rec := range records {
		if rec.Status == statusAbsent {
			absentCount++
		}
	}
	return absentCount, len(records), nil
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

// recordsFor validates the submitted marks against the day's active roster and
// projects the exceptions into storable records. A mark for an enrollment that is
// not on the roster, an unknown status, an over-long note or a second mark for
// the same enrollment (the stored key is one mark per enrollment·day) is a client
// error; a statusPresent mark is dropped, since being present is the absence of a
// record.
func (s *AttendanceService) recordsFor(marks []Mark, studentByEnrollment map[string]string, date time.Time) ([]AttendanceRecord, error) {
	records := make([]AttendanceRecord, 0, len(marks))
	seen := make(map[string]bool, len(marks))
	for _, mark := range marks {
		if _, onRoster := studentByEnrollment[mark.EnrollmentID]; !onRoster {
			return nil, glerrors.New(glerrors.KindInvalid, "enrollment not in group")
		}
		if !isStorableStatus(mark.Status) && mark.Status != statusPresent {
			return nil, glerrors.New(glerrors.KindInvalid, "invalid attendance status")
		}
		if len(mark.Note) > maxNoteLength {
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
			Date:         date,
			Status:       mark.Status,
			Note:         mark.Note,
		})
	}
	return records, nil
}

// publishDay announces the marks that were just stored: one AttendanceMarked per
// exception and, for the absences, one AbsenceRecorded (what comms M10 listens
// for). A nil publisher is a no-op and a failure never fails the call — the day
// is already durable, and losing a notification is strictly better than failing a
// teacher's save — but every dropped event is logged, so best-effort stays
// diagnosable instead of silent.
func (s *AttendanceService) publishDay(ctx context.Context, records []AttendanceRecord, studentByEnrollment map[string]string, date time.Time) {
	if s.publisher == nil {
		return
	}
	day := date.Format(dateLayout)
	for _, rec := range records {
		studentID := studentByEnrollment[rec.EnrollmentID]
		err := s.publisher.PublishAttendanceMarked(ctx, MarkedEvent{
			RecordID:     rec.ID,
			EnrollmentID: rec.EnrollmentID,
			StudentID:    studentID,
			TenantID:     s.tenantID,
			Date:         day,
			Status:       rec.Status,
		})
		s.logDroppedEvent(err, rec.ID)
		if rec.Status != statusAbsent {
			continue
		}
		err = s.publisher.PublishAbsenceRecorded(ctx, AbsenceEvent{
			RecordID:     rec.ID,
			EnrollmentID: rec.EnrollmentID,
			StudentID:    studentID,
			TenantID:     s.tenantID,
			Date:         day,
		})
		s.logDroppedEvent(err, rec.ID)
	}
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

// parseOptionalDate parses an ISO bound, mapping an empty string to the zero
// time — the repository's "no bound" signal.
func parseOptionalDate(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return parseDate(value)
}
