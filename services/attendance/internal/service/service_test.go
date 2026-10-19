package service_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

// --- fakes -------------------------------------------------------------------

// fakeRepo is an in-memory attendance store with the REAL reconcile semantics:
// ReconcileList hands the plan the list's current marks (scoped to the given
// enrollments, date and subject), applies the changes it returns — upserts keyed
// by enrollment·date·subject, deletes by id — and upserts the list's session. It
// also records how it was called, which is how the "one batched read, no N+1",
// "one atomic reconcile" and "reconcile spans the whole active roster" claims
// are checked.
type fakeRepo struct {
	rows     map[string]service.AttendanceRecord // enrollmentID|date|subject → record
	sessions map[string]service.Session          // group|date|subject → session

	marksForDateCalls int
	lastMarksIDs      []string
	lastMarksDate     time.Time
	lastMarksSubject  string

	reconcileCalls    int
	lastReconcileIDs  []string
	lastReconcileRecs []service.AttendanceRecord // rows written (inserts + updates)
	lastChanges       service.ListChanges
	lastTaker         service.Taker
	lastKey           service.ListKey

	listCalls          int
	lastListEnrollment string
	lastListFrom       time.Time
	lastListTo         time.Time
	listResult         []service.AttendanceRecord

	sessionsCalls     int
	lastSessionGroups []string
	lastSessionFrom   time.Time
	lastSessionTo     time.Time
	sessionsResult    []service.TakenList

	// held marks pupil·day·subject keys (subjectDayKey) another group's list
	// holds: like the real repository, a planned insert for one is not stored
	// and is reported in ListChanges.HeldElsewhere.
	held map[string]bool
	// owners records which group's list stored each row (subjectDayKey →
	// group), as the real repository's group_id does (attendance/0004): a row
	// stored by one group's list is held elsewhere for every other group's.
	// A seeded row with no owner is legacy and belongs to every list.
	owners map[string]string

	historyCalls     int
	lastHistoryGroup string
	lastHistoryIDs   []string
	lastHistoryFrom  time.Time
	lastHistoryTo    time.Time
	history          service.GroupHistory

	err error // forced failure, returned by every method when set
}

func (f *fakeRepo) GroupHistory(_ context.Context, groupID string, enrollmentIDs []string, from, to time.Time) (service.GroupHistory, error) {
	f.historyCalls++
	f.lastHistoryGroup, f.lastHistoryIDs = groupID, append([]string(nil), enrollmentIDs...)
	f.lastHistoryFrom, f.lastHistoryTo = from, to
	if f.err != nil {
		return service.GroupHistory{}, f.err
	}
	return f.history, nil
}

func (f *fakeRepo) ListSessions(_ context.Context, groupIDs []string, from, to time.Time) ([]service.TakenList, error) {
	f.sessionsCalls++
	f.lastSessionGroups = append([]string(nil), groupIDs...)
	f.lastSessionFrom, f.lastSessionTo = from, to
	if f.err != nil {
		return nil, f.err
	}
	return f.sessionsResult, nil
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{rows: map[string]service.AttendanceRecord{}, sessions: map[string]service.Session{}, owners: map[string]string{}}
}

// heldElsewhere reports whether another group's list holds the pupil·day·subject
// of key: flagged in held, or stored by a different group's list.
func (f *fakeRepo) heldElsewhere(enrollmentID string, key service.ListKey) bool {
	k := subjectDayKey(enrollmentID, key.Date, key.SubjectID)
	owner, owned := f.owners[k]
	return f.held[k] || (owned && owner != key.GroupID)
}

// dayKey keys a DAILY-list mark (no subject); subjectDayKey a per-class one.
func dayKey(enrollmentID string, date time.Time) string {
	return subjectDayKey(enrollmentID, date, "")
}

func subjectDayKey(enrollmentID string, date time.Time, subjectID string) string {
	return enrollmentID + "|" + date.Format(dateLayout) + "|" + subjectID
}

func sessionKey(k service.ListKey) string {
	return k.GroupID + "|" + k.Date.Format(dateLayout) + "|" + k.SubjectID
}

func (f *fakeRepo) MarksForList(_ context.Context, enrollmentIDs []string, key service.ListKey) (service.ListMarks, error) {
	f.marksForDateCalls++
	f.lastMarksIDs = append([]string(nil), enrollmentIDs...)
	f.lastMarksDate = key.Date
	f.lastMarksSubject = key.SubjectID
	if f.err != nil {
		return service.ListMarks{}, f.err
	}
	out := service.ListMarks{Own: f.current(enrollmentIDs, key), HeldElsewhere: map[string]bool{}}
	for _, id := range enrollmentIDs {
		if f.heldElsewhere(id, key) {
			out.HeldElsewhere[id] = true
		}
	}
	return out, nil
}

func (f *fakeRepo) current(enrollmentIDs []string, key service.ListKey) map[string]service.AttendanceRecord {
	out := make(map[string]service.AttendanceRecord, len(enrollmentIDs))
	for _, id := range enrollmentIDs {
		if f.heldElsewhere(id, key) {
			continue
		}
		if rec, ok := f.rows[subjectDayKey(id, key.Date, key.SubjectID)]; ok {
			out[id] = rec
		}
	}
	return out
}

func (f *fakeRepo) SessionFor(_ context.Context, key service.ListKey) (service.Session, bool, error) {
	if f.err != nil {
		return service.Session{}, false, f.err
	}
	s, ok := f.sessions[sessionKey(key)]
	return s, ok, nil
}

func (f *fakeRepo) ReconcileList(_ context.Context, key service.ListKey, enrollmentIDs []string, taker service.Taker, plan service.PlanFunc) (service.ListChanges, error) {
	f.reconcileCalls++
	f.lastKey = key
	f.lastTaker = taker
	f.lastReconcileIDs = append([]string(nil), enrollmentIDs...)
	if f.err != nil {
		return service.ListChanges{}, f.err
	}
	current := f.current(enrollmentIDs, key)
	changes, err := plan(current)
	if err != nil {
		return service.ListChanges{}, err
	}
	// lastReconcileRecs is what the plan asked to write (inserts + updates).
	f.lastReconcileRecs = nil
	f.lastReconcileRecs = append(f.lastReconcileRecs, changes.Inserts...)
	for _, u := range changes.Updates {
		f.lastReconcileRecs = append(f.lastReconcileRecs, u.Record)
	}
	// Like the real upsert's RETURNING, report the STORED revision: a new row
	// starts at 1 and every in-place write bumps the stored one.
	stored := service.ListChanges{Deletes: changes.Deletes}
	for _, rec := range changes.Inserts {
		if f.heldElsewhere(rec.EnrollmentID, key) {
			stored.HeldElsewhere = append(stored.HeldElsewhere, rec.EnrollmentID)
			continue
		}
		rec.Revision = 1
		stored.Inserts = append(stored.Inserts, rec)
		f.rows[subjectDayKey(rec.EnrollmentID, rec.Date, rec.SubjectID)] = rec
		f.owners[subjectDayKey(rec.EnrollmentID, rec.Date, rec.SubjectID)] = key.GroupID
	}
	for _, u := range changes.Updates {
		u.Record.Revision = current[u.Record.EnrollmentID].Revision + 1
		stored.Updates = append(stored.Updates, u)
		f.rows[subjectDayKey(u.Record.EnrollmentID, u.Record.Date, u.Record.SubjectID)] = u.Record
	}
	f.lastChanges = changes
	for _, rec := range changes.Deletes {
		delete(f.rows, subjectDayKey(rec.EnrollmentID, rec.Date, rec.SubjectID))
		delete(f.owners, subjectDayKey(rec.EnrollmentID, rec.Date, rec.SubjectID))
	}
	sk := sessionKey(key)
	s, ok := f.sessions[sk]
	if !ok {
		s = service.Session{TakenAt: sessionTakenAt, TakenByName: taker.Name}
	}
	s.UpdatedAt = sessionTakenAt.Add(time.Duration(f.reconcileCalls) * time.Minute)
	f.sessions[sk] = s
	return stored, nil
}

func (f *fakeRepo) ListByEnrollment(_ context.Context, enrollmentID string, from, to time.Time) ([]service.AttendanceRecord, error) {
	f.listCalls++
	f.lastListEnrollment = enrollmentID
	f.lastListFrom = from
	f.lastListTo = to
	if f.err != nil {
		return nil, f.err
	}
	return f.listResult, nil
}

// fakeEnrollments stands in for the students service: every enrollment ever
// in a group and the group's moves, in the arbitrary order an RPC would
// return them.
type fakeEnrollments struct {
	byGroup map[string][]service.EnrollmentRef
	moves   map[string][]service.Move
	err     error
}

func (f *fakeEnrollments) MovesOf(_ context.Context, groupID string) ([]service.Move, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.moves[groupID], nil
}

func (f *fakeEnrollments) EverInGroup(_ context.Context, groupID string) ([]service.EnrollmentRef, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byGroup[groupID], nil
}

// fakeGroups stands in for the config service: a group either exists or is a
// NotFound (existence is all attendance needs).
type fakeGroups struct {
	existing map[string]bool
	deleting map[string]bool // the group's level is being deleted (ADR 0021)
	err      error
}

func (f *fakeGroups) ByID(_ context.Context, id string) error {
	if f.err != nil {
		return f.err
	}
	if !f.existing[id] {
		return glerrors.ErrNotFound
	}
	return nil
}

func (f *fakeGroups) WritableByID(ctx context.Context, id string) error {
	if err := f.ByID(ctx, id); err != nil {
		return err
	}
	if f.deleting[id] {
		return levelarchive.LevelBeingDeleted()
	}
	return nil
}

// spyPublisher records the events emitted for a saved day. Publishing is
// best-effort, so err lets a test prove a broken bus never fails the save.
type spyPublisher struct {
	absences []service.AbsenceEvent
	marked   []service.MarkedEvent
	err      error
}

func (s *spyPublisher) PublishAbsenceRecorded(_ context.Context, e service.AbsenceEvent) error {
	s.absences = append(s.absences, e)
	return s.err
}

func (s *spyPublisher) PublishAttendanceMarked(_ context.Context, e service.MarkedEvent) error {
	s.marked = append(s.marked, e)
	return s.err
}

// --- fixtures ----------------------------------------------------------------

const (
	dateLayout = "2006-01-02"

	testTenant = "tenant-1"
	groupID    = "grp-1"
	otherGroup = "grp-2"
	schoolDay  = "2026-08-05"

	// Enrollment ids are deliberately NOT in the order the reader returns them,
	// so the roster's sort is actually exercised.
	enrollmentAna   = "enr-a"
	enrollmentBeto  = "enr-b"
	enrollmentCruz  = "enr-c"
	enrollmentGone  = "enr-d" // withdrawn: never on the roster
	enrollmentAlien = "enr-x" // enrolled in another group

	// ListAttendance validates its enrollment id as a UUID at the boundary (the
	// column is a uuid), so its tests use a real one rather than the readable
	// roster fixtures above.
	enrollmentUUID = "3f2504e0-4f89-41d3-9a0c-0305e82c3301"

	studentAna  = "stu-a"
	studentBeto = "stu-b"
	studentCruz = "stu-c"
)

// testTaker is who saves in the tests that are not about the taker itself.
var testTaker = service.Taker{UserID: "9b2f7c1e-0000-4000-8000-000000000001", Name: "Gabriela Rojas"}

// sessionTakenAt is the instant the fake stamps on a list's first save.
var sessionTakenAt = time.Date(2026, 8, 5, 14, 0, 0, 0, time.UTC)

// save is SaveDailyAttendance on the DAILY list (no subject) by testTaker.
func save(svc *service.AttendanceService, ctx context.Context, group, date string, marks []service.Mark) (int, int, error) {
	res, err := svc.SaveDailyAttendance(ctx, service.SaveRequest{
		GroupID: group, Date: date, Marks: marks, TakenBy: testTaker.UserID, TakenByName: testTaker.Name,
	})
	return res.AbsentCount, res.MarkedCount, err
}

// roster is GetDailyRoster on the DAILY list, returning only its rows.
func roster(svc *service.AttendanceService, ctx context.Context, group, date string) ([]service.RosterRow, error) {
	r, err := svc.GetDailyRoster(ctx, group, date, "")
	return r.Rows, err
}

// maxNote is the longest note the service accepts; one more character is a
// client error (the column carries the same CHECK).
const maxNote = 500

// mintedIDs is the deterministic id sequence the service draws from, so tests
// can assert exactly which record got which id.
func fixedIDs(ids ...string) func() string {
	i := -1
	return func() string {
		i++
		if i < len(ids) {
			return ids[i]
		}
		return "overflow"
	}
}

func day(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(dateLayout, s)
	if err != nil {
		t.Fatalf("parse fixture date %q: %v", s, err)
	}
	return d
}

// newService wires the service over the given seams with a deterministic id
// source and a silent logger (see newLoggingService when the log itself is
// under test).
func newService(t *testing.T, repo *fakeRepo, pub service.Publisher, ids ...string) *service.AttendanceService {
	t.Helper()
	return newLoggingService(t, repo, pub, slog.New(slog.DiscardHandler), ids...)
}

// newLoggingService wires the service over the given seams with a deterministic
// id source. The roster group holds three active enrollments plus one
// withdrawn, and a second group holds an unrelated enrollment.
func newLoggingService(t *testing.T, repo *fakeRepo, pub service.Publisher, log *slog.Logger, ids ...string) *service.AttendanceService {
	t.Helper()
	return newServiceWith(t, repo, pub, log, ids)
}

// newServiceWith is newLoggingService with service options (e.g. the school
// calendar seam).
func newServiceWith(t *testing.T, repo *fakeRepo, pub service.Publisher, log *slog.Logger, ids []string, opts ...service.Option) *service.AttendanceService {
	t.Helper()
	enrollments := &fakeEnrollments{byGroup: map[string][]service.EnrollmentRef{
		groupID: {
			{ID: enrollmentCruz, StudentID: studentCruz, GroupID: groupID, Status: "active"},
			{ID: enrollmentAna, StudentID: studentAna, GroupID: groupID, Status: "active"},
			{ID: enrollmentGone, StudentID: "stu-d", GroupID: groupID, Status: "withdrawn"},
			{ID: enrollmentBeto, StudentID: studentBeto, GroupID: groupID, Status: "active"},
		},
		otherGroup: {
			{ID: enrollmentAlien, StudentID: "stu-x", GroupID: otherGroup, Status: "active"},
		},
	}}
	groups := &fakeGroups{existing: map[string]bool{groupID: true, otherGroup: true}}
	if len(ids) == 0 {
		ids = []string{"rec-1", "rec-2", "rec-3"}
	}
	return service.NewAttendanceService(repo, enrollments, groups, pub, testTenant, fixedIDs(ids...), log, opts...)
}

// captureLogs returns a logger writing into the buffer it also returns, so a
// test can assert on what was actually logged.
func captureLogs() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})), &buf
}

func wantInvalid(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("want a KindInvalid error, got nil")
	}
	if kind := glerrors.KindOf(err); kind != glerrors.KindInvalid {
		t.Fatalf("KindOf(%v) = %v, want KindInvalid", err, kind)
	}
}

// --- GetDailyRoster ----------------------------------------------------------

// TestGetDailyRosterDefaultsPresentAndOverlaysMarks is the default-present model
// in one assertion: every ACTIVE enrollment gets a row, a stored exception
// overrides the default, everything else reads "present", the withdrawn
// enrollment is absent from the roster, and the rows come back sorted by
// enrollment id. The marks are resolved in exactly ONE batched repo call.
func TestGetDailyRosterDefaultsPresentAndOverlaysMarks(t *testing.T) {
	repo := newFakeRepo()
	date := day(t, schoolDay)
	repo.rows[dayKey(enrollmentBeto, date)] = service.AttendanceRecord{
		ID: "rec-old", EnrollmentID: enrollmentBeto, Date: date, Status: "absent",
	}
	repo.rows[dayKey(enrollmentCruz, date)] = service.AttendanceRecord{
		ID: "rec-old-2", EnrollmentID: enrollmentCruz, Date: date, Status: "late", Note: "traffic",
	}
	svc := newService(t, repo, nil)

	rows, err := roster(svc, context.Background(), groupID, schoolDay)
	if err != nil {
		t.Fatalf("GetDailyRoster: %v", err)
	}

	want := []service.RosterRow{
		{EnrollmentID: enrollmentAna, StudentID: studentAna, Status: "present"},
		{EnrollmentID: enrollmentBeto, StudentID: studentBeto, Status: "absent"},
		{EnrollmentID: enrollmentCruz, StudentID: studentCruz, Status: "late", Note: "traffic"}, // the note round-trips
	}
	if len(rows) != len(want) {
		t.Fatalf("roster has %d rows, want %d: %+v", len(rows), len(want), rows)
	}
	for i, w := range want {
		if rows[i] != w {
			t.Errorf("row %d = %+v, want %+v", i, rows[i], w)
		}
	}
	if repo.marksForDateCalls != 1 {
		t.Errorf("MarksForDate called %d times, want exactly 1 (batched, no N+1)", repo.marksForDateCalls)
	}
	if len(repo.lastMarksIDs) != 3 {
		t.Errorf("batched read got %d ids, want the 3 active enrollments: %v", len(repo.lastMarksIDs), repo.lastMarksIDs)
	}
	if !repo.lastMarksDate.Equal(date) {
		t.Errorf("batched read date = %v, want %v", repo.lastMarksDate, date)
	}
}

// TestGetDailyRosterRejectsBadInput proves a missing group and a malformed date
// are client errors (KindInvalid) — a config NotFound is never leaked upward as
// a NotFound.
func TestGetDailyRosterRejectsBadInput(t *testing.T) {
	tests := []struct {
		name    string
		groupID string
		date    string
	}{
		{name: "unknown group", groupID: "grp-nope", date: schoolDay},
		{name: "malformed date", groupID: groupID, date: "05/08/2026"},
		{name: "empty date", groupID: groupID, date: ""},
		{name: "date with time", groupID: groupID, date: "2026-08-05T10:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc := newService(t, repo, nil)

			_, err := roster(svc, context.Background(), tt.groupID, tt.date)
			wantInvalid(t, err)
			if repo.marksForDateCalls != 0 {
				t.Errorf("repo was queried despite invalid input (%d calls)", repo.marksForDateCalls)
			}
		})
	}
}

// TestMalformedDateIsCheckedBeforeTheGroupRPC proves the cheap pure check runs
// first: a request that is wrong in BOTH the date and the group reports the date
// (and never pays for the config round trip).
func TestMalformedDateIsCheckedBeforeTheGroupRPC(t *testing.T) {
	const badDate = "05/08/2026"
	calls := &countingGroups{}

	t.Run("GetDailyRoster", func(t *testing.T) {
		svc := newRosterService(t, newFakeRepo(), calls)
		_, err := roster(svc, context.Background(), "grp-nope", badDate)
		wantDateError(t, err, calls)
	})
	t.Run("SaveDailyAttendance", func(t *testing.T) {
		calls.calls = 0
		svc := newRosterService(t, newFakeRepo(), calls)
		_, _, err := save(svc, context.Background(), "grp-nope", badDate, nil)
		wantDateError(t, err, calls)
	})
}

// countingGroups is a GroupReader that records whether it was consulted at all.
type countingGroups struct{ calls int }

func (c *countingGroups) ByID(context.Context, string) error {
	c.calls++
	return glerrors.ErrNotFound
}

func (c *countingGroups) WritableByID(ctx context.Context, id string) error { return c.ByID(ctx, id) }

func newRosterService(t *testing.T, repo *fakeRepo, groups service.GroupReader) *service.AttendanceService {
	t.Helper()
	return service.NewAttendanceService(
		repo, &fakeEnrollments{}, groups, nil, testTenant, fixedIDs(), slog.New(slog.DiscardHandler))
}

func wantDateError(t *testing.T, err error, groups *countingGroups) {
	t.Helper()
	wantInvalid(t, err)
	if !strings.Contains(err.Error(), "ISO date") {
		t.Errorf("error = %q, want the date error (the pure check runs before the RPC)", err)
	}
	if groups.calls != 0 {
		t.Errorf("the group RPC was called %d times despite a malformed date", groups.calls)
	}
}

// TestGetDailyRosterPropagatesRepoFailure proves an infrastructure failure is
// propagated unchanged, not swallowed into an empty roster.
func TestGetDailyRosterPropagatesRepoFailure(t *testing.T) {
	repo := newFakeRepo()
	repo.err = errors.New("connection reset")
	svc := newService(t, repo, nil)

	if _, err := roster(svc, context.Background(), groupID, schoolDay); err == nil {
		t.Fatal("want the repo failure to propagate, got nil")
	}
}

// --- SaveDailyAttendance -----------------------------------------------------

// TestSaveDailyAttendanceStoresExceptions proves a save stores ONLY the
// exceptions, mints an id per record, stamps the parsed date, and reconciles
// against the FULL active roster (that id set is what makes unlisted students
// present again).
func TestSaveDailyAttendanceStoresExceptions(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(t, repo, nil, "rec-1", "rec-2")
	date := day(t, schoolDay)

	absent, marked, err := save(svc, context.Background(), groupID, schoolDay, []service.Mark{
		{EnrollmentID: enrollmentAna, Status: "absent"},
		{EnrollmentID: enrollmentBeto, Status: "absent", Note: "sin justificar"},
	})
	if err != nil {
		t.Fatalf("SaveDailyAttendance: %v", err)
	}
	if absent != 2 || marked != 2 {
		t.Errorf("counts = (absent %d, marked %d), want (2, 2)", absent, marked)
	}
	if repo.reconcileCalls != 1 {
		t.Fatalf("ReconcileDay called %d times, want exactly 1 (one atomic day)", repo.reconcileCalls)
	}
	if len(repo.lastReconcileIDs) != 3 {
		t.Errorf("reconcile scope = %v, want the 3 active enrollments", repo.lastReconcileIDs)
	}
	want := []service.AttendanceRecord{
		{ID: "rec-1", EnrollmentID: enrollmentAna, Date: date, Status: "absent"},
		{ID: "rec-2", EnrollmentID: enrollmentBeto, Date: date, Status: "absent", Note: "sin justificar"},
	}
	if len(repo.lastReconcileRecs) != len(want) {
		t.Fatalf("stored %d records, want %d: %+v", len(repo.lastReconcileRecs), len(want), repo.lastReconcileRecs)
	}
	for i, w := range want {
		if repo.lastReconcileRecs[i] != w {
			t.Errorf("record %d = %+v, want %+v", i, repo.lastReconcileRecs[i], w)
		}
	}
}

// TestSaveDailyAttendanceReSaveClearsDroppedMarks is the reconcile contract: the
// second save carries a DIFFERENT exception set, so the student dropped from it
// goes back to present (their row is gone) without any explicit delete call.
func TestSaveDailyAttendanceReSaveClearsDroppedMarks(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(t, repo, nil, "rec-1", "rec-2", "rec-3")
	ctx := context.Background()

	if _, _, err := save(svc, ctx, groupID, schoolDay, []service.Mark{
		{EnrollmentID: enrollmentAna, Status: "absent"},
		{EnrollmentID: enrollmentBeto, Status: "absent"},
	}); err != nil {
		t.Fatalf("first save: %v", err)
	}

	absent, marked, err := save(svc, ctx, groupID, schoolDay, []service.Mark{
		{EnrollmentID: enrollmentBeto, Status: "late", Note: "llegó 8:15"},
	})
	if err != nil {
		t.Fatalf("re-save: %v", err)
	}
	if absent != 0 || marked != 1 {
		t.Errorf("counts = (absent %d, marked %d), want (0, 1)", absent, marked)
	}

	rows, err := roster(svc, ctx, groupID, schoolDay)
	if err != nil {
		t.Fatalf("roster after re-save: %v", err)
	}
	got := map[string]string{}
	for _, r := range rows {
		got[r.EnrollmentID] = r.Status
	}
	if got[enrollmentAna] != "present" {
		t.Errorf("Ana was dropped from the exception set: status = %q, want %q", got[enrollmentAna], "present")
	}
	if got[enrollmentBeto] != "late" {
		t.Errorf("Beto status = %q, want %q", got[enrollmentBeto], "late")
	}
}

// TestSaveDailyAttendanceDropsPresentMarks proves "present" is not stored: it is
// the way a client CLEARS a mark, so the day ends with zero records while the
// reconcile still spans the whole roster.
func TestSaveDailyAttendanceDropsPresentMarks(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(t, repo, nil)
	ctx := context.Background()

	if _, _, err := save(svc, ctx, groupID, schoolDay, []service.Mark{
		{EnrollmentID: enrollmentAna, Status: "absent"},
	}); err != nil {
		t.Fatalf("seed save: %v", err)
	}

	absent, marked, err := save(svc, ctx, groupID, schoolDay, []service.Mark{
		{EnrollmentID: enrollmentAna, Status: "present"},
	})
	if err != nil {
		t.Fatalf("clearing save: %v", err)
	}
	if absent != 0 || marked != 0 {
		t.Errorf("counts = (absent %d, marked %d), want (0, 0) — present is never stored", absent, marked)
	}
	if len(repo.lastReconcileRecs) != 0 {
		t.Errorf("present mark was persisted: %+v", repo.lastReconcileRecs)
	}
	if len(repo.rows) != 0 {
		t.Errorf("the stored mark was not cleared: %+v", repo.rows)
	}
}

// TestSaveDailyAttendanceRejectsBadInput proves every client mistake is a
// KindInvalid and that NOTHING is written when the batch is rejected (the day is
// reconciled all-or-nothing).
func TestSaveDailyAttendanceRejectsBadInput(t *testing.T) {
	tests := []struct {
		name    string
		groupID string
		date    string
		marks   []service.Mark
	}{
		{
			name:    "unknown group",
			groupID: "grp-nope",
			date:    schoolDay,
			marks:   []service.Mark{{EnrollmentID: enrollmentAna, Status: "absent"}},
		},
		{
			name:    "malformed date",
			groupID: groupID,
			date:    "2026-13-40",
			marks:   []service.Mark{{EnrollmentID: enrollmentAna, Status: "absent"}},
		},
		{
			name:    "enrollment belongs to another group",
			groupID: groupID,
			date:    schoolDay,
			marks:   []service.Mark{{EnrollmentID: enrollmentAlien, Status: "absent"}},
		},
		{
			name:    "enrollment is not active",
			groupID: groupID,
			date:    schoolDay,
			marks:   []service.Mark{{EnrollmentID: enrollmentGone, Status: "absent"}},
		},
		{
			name:    "unknown enrollment",
			groupID: groupID,
			date:    schoolDay,
			marks:   []service.Mark{{EnrollmentID: "enr-ghost", Status: "absent"}},
		},
		{
			name:    "unknown status",
			groupID: groupID,
			date:    schoolDay,
			marks:   []service.Mark{{EnrollmentID: enrollmentAna, Status: "tarde"}},
		},
		{
			name:    "empty status",
			groupID: groupID,
			date:    schoolDay,
			marks:   []service.Mark{{EnrollmentID: enrollmentAna, Status: ""}},
		},
		{
			name:    "two marks for the same enrollment",
			groupID: groupID,
			date:    schoolDay,
			marks: []service.Mark{
				{EnrollmentID: enrollmentAna, Status: "absent"},
				{EnrollmentID: enrollmentAna, Status: "late"},
			},
		},
		{
			name:    "note longer than the cap",
			groupID: groupID,
			date:    schoolDay,
			marks: []service.Mark{
				{EnrollmentID: enrollmentAna, Status: "excused", Note: strings.Repeat("x", maxNote+1)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc := newService(t, repo, nil)

			absent, marked, err := save(svc, context.Background(), tt.groupID, tt.date, tt.marks)
			wantInvalid(t, err)
			if absent != 0 || marked != 0 {
				t.Errorf("rejected save reported counts (%d, %d), want (0, 0)", absent, marked)
			}
			if repo.reconcileCalls != 0 {
				t.Errorf("rejected save still wrote (%d reconcile calls)", repo.reconcileCalls)
			}
		})
	}
}

// TestSaveDailyAttendancePublishesEvents proves the events carry the student and
// tenant the consumers need: one AbsenceRecorded per ABSENT record (comms M10
// only cares about absences) and one AttendanceMarked per stored exception.
func TestSaveDailyAttendancePublishesEvents(t *testing.T) {
	repo := newFakeRepo()
	pub := &spyPublisher{}
	svc := newService(t, repo, pub, "rec-1", "rec-2")

	if _, _, err := save(svc, context.Background(), groupID, schoolDay, []service.Mark{
		{EnrollmentID: enrollmentAna, Status: "absent"},
		{EnrollmentID: enrollmentBeto, Status: "late"},
	}); err != nil {
		t.Fatalf("SaveDailyAttendance: %v", err)
	}

	wantAbsences := []service.AbsenceEvent{{
		RecordID: "rec-1", EnrollmentID: enrollmentAna, StudentID: studentAna,
		TenantID: testTenant, Date: schoolDay, MarkVersion: 1,
	}}
	if len(pub.absences) != 1 || pub.absences[0] != wantAbsences[0] {
		t.Errorf("absences = %+v, want %+v", pub.absences, wantAbsences)
	}
	wantMarked := []service.MarkedEvent{
		{RecordID: "rec-1", EnrollmentID: enrollmentAna, StudentID: studentAna, TenantID: testTenant, Date: schoolDay, Status: "absent", MarkVersion: 1},
		{RecordID: "rec-2", EnrollmentID: enrollmentBeto, StudentID: studentBeto, TenantID: testTenant, Date: schoolDay, Status: "late", MarkVersion: 1},
	}
	if len(pub.marked) != len(wantMarked) {
		t.Fatalf("marked events = %+v, want %+v", pub.marked, wantMarked)
	}
	for i, w := range wantMarked {
		if pub.marked[i] != w {
			t.Errorf("marked event %d = %+v, want %+v", i, pub.marked[i], w)
		}
	}
}

// TestSaveDailyAttendancePublishingIsBestEffort proves the save succeeds with no
// publisher wired at all AND when the bus is broken: attendance is already
// durable, and losing a notification must never fail the teacher's save. A
// broken bus is best-effort, not silent: it must leave a warning behind, so a
// dropped notification is diagnosable instead of invisible.
func TestSaveDailyAttendancePublishingIsBestEffort(t *testing.T) {
	tests := []struct {
		name    string
		pub     service.Publisher
		wantLog bool
	}{
		{name: "no publisher wired", pub: nil},
		{name: "broken bus", pub: &spyPublisher{err: errors.New("pubsub unavailable")}, wantLog: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			log, logs := captureLogs()
			svc := newLoggingService(t, repo, tt.pub, log, "rec-1")

			absent, marked, err := save(svc, context.Background(), groupID, schoolDay, []service.Mark{
				{EnrollmentID: enrollmentAna, Status: "absent"},
			})
			if err != nil {
				t.Fatalf("publishing must not fail the save: %v", err)
			}
			if absent != 1 || marked != 1 {
				t.Errorf("counts = (%d, %d), want (1, 1)", absent, marked)
			}

			logged := logs.String()
			if !tt.wantLog {
				if logged != "" {
					t.Errorf("a healthy publish must log nothing, got %q", logged)
				}
				return
			}
			if !strings.Contains(logged, "attendance event not published") {
				t.Errorf("a dropped event must be logged, got %q", logged)
			}
			if !strings.Contains(logged, "pubsub unavailable") {
				t.Errorf("the log must carry the publisher error, got %q", logged)
			}
			if !strings.Contains(logged, "rec-1") {
				t.Errorf("the log must identify the record, got %q", logged)
			}
		})
	}
}

// TestSaveDailyAttendancePropagatesRepoFailure proves a failed reconcile is
// surfaced and no event is published for a day that was never stored.
func TestSaveDailyAttendancePropagatesRepoFailure(t *testing.T) {
	repo := newFakeRepo()
	repo.err = errors.New("deadlock detected")
	pub := &spyPublisher{}
	svc := newService(t, repo, pub)

	if _, _, err := save(svc, context.Background(), groupID, schoolDay, []service.Mark{
		{EnrollmentID: enrollmentAna, Status: "absent"},
	}); err == nil {
		t.Fatal("want the reconcile failure to propagate, got nil")
	}
	if len(pub.absences) != 0 || len(pub.marked) != 0 {
		t.Errorf("events published for a day that was not stored: %+v / %+v", pub.absences, pub.marked)
	}
}

// TestSaveDailyAttendanceAcceptsEveryStoredStatus pins the accepted vocabulary
// and which of them count as an absence.
func TestSaveDailyAttendanceAcceptsEveryStoredStatus(t *testing.T) {
	tests := []struct {
		status     string
		wantAbsent int
	}{
		{status: "absent", wantAbsent: 1},
		{status: "late", wantAbsent: 0},
		{status: "excused", wantAbsent: 0},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			repo := newFakeRepo()
			pub := &spyPublisher{}
			svc := newService(t, repo, pub)

			absent, marked, err := save(svc, context.Background(), groupID, schoolDay, []service.Mark{
				{EnrollmentID: enrollmentAna, Status: tt.status},
			})
			if err != nil {
				t.Fatalf("status %q must be accepted: %v", tt.status, err)
			}
			if marked != 1 {
				t.Errorf("marked = %d, want 1", marked)
			}
			if absent != tt.wantAbsent {
				t.Errorf("absent = %d, want %d", absent, tt.wantAbsent)
			}
			if len(pub.absences) != tt.wantAbsent {
				t.Errorf("AbsenceRecorded events = %d, want %d", len(pub.absences), tt.wantAbsent)
			}
		})
	}
}

// TestSaveDailyAttendanceCapsNoteLength pins the free-text boundary: a note of
// exactly the cap is stored, one character more is rejected before anything is
// written. Without it an unbounded note reaches a text column straight from a
// teacher's browser. The cap counts CHARACTERS, like the column's CHECK
// (length()) and the web's counter: an accented note is not cut at half length
// because "é" is two bytes.
func TestSaveDailyAttendanceCapsNoteLength(t *testing.T) {
	const accented = "é"
	tests := []struct {
		name       string
		char       string
		length     int
		wantStored bool
	}{
		{name: "at the cap", char: "x", length: maxNote, wantStored: true},
		{name: "one over the cap", char: "x", length: maxNote + 1},
		{name: "400 accented characters (800 bytes)", char: accented, length: 400, wantStored: true},
		{name: "accented at the cap", char: accented, length: maxNote, wantStored: true},
		{name: "accented one over the cap", char: accented, length: maxNote + 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc := newService(t, repo, nil)

			_, marked, err := save(svc, context.Background(), groupID, schoolDay, []service.Mark{
				{EnrollmentID: enrollmentAna, Status: "excused", Note: strings.Repeat(tt.char, tt.length)},
			})
			if !tt.wantStored {
				wantInvalid(t, err)
				if repo.reconcileCalls != 0 {
					t.Errorf("an oversized note still reached the repo (%d calls)", repo.reconcileCalls)
				}
				return
			}
			if err != nil {
				t.Fatalf("a note of %d characters must be accepted: %v", tt.length, err)
			}
			if marked != 1 {
				t.Errorf("marked = %d, want 1", marked)
			}
		})
	}
}

// TestSaveDailyAttendanceEmptyMarksClearsTheDay proves an empty exception set is
// a legitimate "everybody present" save: it still reconciles the roster (wiping
// any previous marks) and reports zero counts.
func TestSaveDailyAttendanceEmptyMarksClearsTheDay(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(t, repo, nil)
	ctx := context.Background()

	if _, _, err := save(svc, ctx, groupID, schoolDay, []service.Mark{
		{EnrollmentID: enrollmentAna, Status: "absent"},
	}); err != nil {
		t.Fatalf("seed save: %v", err)
	}

	absent, marked, err := save(svc, ctx, groupID, schoolDay, nil)
	if err != nil {
		t.Fatalf("empty save: %v", err)
	}
	if absent != 0 || marked != 0 {
		t.Errorf("counts = (%d, %d), want (0, 0)", absent, marked)
	}
	if len(repo.rows) != 0 {
		t.Errorf("previous marks survived an all-present save: %+v", repo.rows)
	}
}

// --- ListAttendance ----------------------------------------------------------

// TestListAttendanceParsesOptionalBounds proves each ISO bound is parsed and an
// empty bound becomes the zero time (the repo's "skip this bound" signal).
func TestListAttendanceParsesOptionalBounds(t *testing.T) {
	tests := []struct {
		name     string
		from     string
		to       string
		wantFrom string
		wantTo   string
	}{
		{name: "both bounds", from: "2026-08-01", to: "2026-08-31", wantFrom: "2026-08-01", wantTo: "2026-08-31"},
		{name: "open end", from: "2026-08-01", to: "", wantFrom: "2026-08-01"},
		{name: "open start", from: "", to: "2026-08-31", wantTo: "2026-08-31"},
		{name: "unbounded", from: "", to: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			repo.listResult = []service.AttendanceRecord{{ID: "rec-1", EnrollmentID: enrollmentUUID, Status: "absent"}}
			svc := newService(t, repo, nil)

			got, err := svc.ListAttendance(context.Background(), enrollmentUUID, tt.from, tt.to)
			if err != nil {
				t.Fatalf("ListAttendance: %v", err)
			}
			if len(got) != 1 || got[0].ID != "rec-1" {
				t.Errorf("records = %+v, want the repo's rows verbatim", got)
			}
			if repo.lastListEnrollment != enrollmentUUID {
				t.Errorf("enrollment = %q, want %q", repo.lastListEnrollment, enrollmentUUID)
			}
			assertBound(t, "from", repo.lastListFrom, tt.wantFrom)
			assertBound(t, "to", repo.lastListTo, tt.wantTo)
		})
	}
}

// TestListAttendanceRejectsMalformedBounds proves a bad bound is a client error,
// never a silently unbounded query.
func TestListAttendanceRejectsMalformedBounds(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
	}{
		{name: "bad from", from: "01-08-2026", to: ""},
		{name: "bad to", from: "", to: "2026-8-1x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc := newService(t, repo, nil)

			_, err := svc.ListAttendance(context.Background(), enrollmentUUID, tt.from, tt.to)
			wantInvalid(t, err)
			if repo.listCalls != 0 {
				t.Errorf("repo queried despite a malformed bound (%d calls)", repo.listCalls)
			}
		})
	}
}

// TestListAttendanceRejectsMalformedEnrollmentID proves the id is validated at
// the boundary instead of reaching Postgres as a broken uuid literal: without
// this, `?enrollmentId=abc` is a 500 (an infrastructure error for what is plainly
// a client mistake).
func TestListAttendanceRejectsMalformedEnrollmentID(t *testing.T) {
	tests := []struct {
		name         string
		enrollmentID string
	}{
		{name: "not a uuid", enrollmentID: "abc"},
		{name: "empty", enrollmentID: ""},
		{name: "sql-ish", enrollmentID: "' OR 1=1 --"},
		{name: "truncated uuid", enrollmentID: "3f2504e0-4f89-41d3-9a0c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc := newService(t, repo, nil)

			_, err := svc.ListAttendance(context.Background(), tt.enrollmentID, "", "")
			wantInvalid(t, err)
			if repo.listCalls != 0 {
				t.Errorf("repo queried with a malformed enrollment id (%d calls)", repo.listCalls)
			}
		})
	}
}

// assertBound checks a parsed bound: an empty want means the zero time.
func assertBound(t *testing.T, name string, got time.Time, want string) {
	t.Helper()
	if want == "" {
		if !got.IsZero() {
			t.Errorf("%s bound = %v, want the zero time (unbounded)", name, got)
		}
		return
	}
	if got.Format(dateLayout) != want {
		t.Errorf("%s bound = %v, want %s", name, got, want)
	}
}

// Review finding 4: while a level's deletion runs, a list saved for one of its
// groups is refused with LEVEL_BEING_DELETED (409) — a mark written after
// attendance's archive step would otherwise survive the level. The roster
// still reads.
func TestSaveRefusesAGroupWhoseLevelIsBeingDeleted(t *testing.T) {
	repo := newFakeRepo()
	enrollments := &fakeEnrollments{byGroup: map[string][]service.EnrollmentRef{
		groupID: {{ID: enrollmentAna, StudentID: studentAna, GroupID: groupID, Status: "active"}},
	}}
	groups := &fakeGroups{existing: map[string]bool{groupID: true}, deleting: map[string]bool{groupID: true}}
	svc := service.NewAttendanceService(repo, enrollments, groups, nil, testTenant, fixedIDs("rec-1"), slog.New(slog.DiscardHandler))
	ctx := context.Background()

	_, _, err := save(svc, ctx, groupID, schoolDay, []service.Mark{{EnrollmentID: enrollmentAna, Status: "absent"}})
	if code := levelarchive.CodeOf(err); code != levelarchive.CodeLevelBeingDeleted {
		t.Fatalf("CodeOf(%v) = %q, want %q", err, code, levelarchive.CodeLevelBeingDeleted)
	}
	if kind := glerrors.KindOf(err); kind != glerrors.KindConflict {
		t.Fatalf("KindOf = %v, want KindConflict", kind)
	}
	if len(repo.rows) != 0 || len(repo.sessions) != 0 {
		t.Fatal("a refused list must store nothing")
	}
	if _, err := roster(svc, ctx, groupID, schoolDay); err != nil {
		t.Fatalf("the roster still reads: %v", err)
	}
}
