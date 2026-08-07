package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

// --- fakes -------------------------------------------------------------------

// fakeRepo is an in-memory attendance store with the REAL reconcile semantics
// (delete the day's marks for the given enrollments, then insert the new set),
// so the tests can prove a re-save clears the marks that were dropped from the
// exception list. It also records how it was called, which is how the "one
// batched read, no N+1" and "reconcile spans the whole active roster" claims are
// checked.
type fakeRepo struct {
	rows map[string]service.AttendanceRecord // enrollmentID|date → record

	marksForDateCalls int
	lastMarksIDs      []string
	lastMarksDate     time.Time

	reconcileCalls    int
	lastReconcileIDs  []string
	lastReconcileRecs []service.AttendanceRecord

	listCalls          int
	lastListEnrollment string
	lastListFrom       time.Time
	lastListTo         time.Time
	listResult         []service.AttendanceRecord

	err error // forced failure, returned by every method when set
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{rows: map[string]service.AttendanceRecord{}}
}

func dayKey(enrollmentID string, date time.Time) string {
	return enrollmentID + "|" + date.Format(dateLayout)
}

func (f *fakeRepo) MarksForDate(_ context.Context, enrollmentIDs []string, date time.Time) (map[string]service.AttendanceRecord, error) {
	f.marksForDateCalls++
	f.lastMarksIDs = append([]string(nil), enrollmentIDs...)
	f.lastMarksDate = date
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]service.AttendanceRecord, len(enrollmentIDs))
	for _, id := range enrollmentIDs {
		if rec, ok := f.rows[dayKey(id, date)]; ok {
			out[id] = rec
		}
	}
	return out, nil
}

func (f *fakeRepo) ReconcileDay(_ context.Context, enrollmentIDs []string, date time.Time, records []service.AttendanceRecord) error {
	f.reconcileCalls++
	f.lastReconcileIDs = append([]string(nil), enrollmentIDs...)
	f.lastReconcileRecs = append([]service.AttendanceRecord(nil), records...)
	if f.err != nil {
		return f.err
	}
	for _, id := range enrollmentIDs {
		delete(f.rows, dayKey(id, date))
	}
	for _, rec := range records {
		f.rows[dayKey(rec.EnrollmentID, rec.Date)] = rec
	}
	return nil
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

// fakeEnrollments stands in for the students service: the enrollments of a
// group, in the arbitrary order an RPC would return them.
type fakeEnrollments struct {
	byGroup map[string][]service.EnrollmentRef
	err     error
}

func (f *fakeEnrollments) ByGroup(_ context.Context, groupID string) ([]service.EnrollmentRef, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byGroup[groupID], nil
}

// fakeGroups stands in for the config service: a group either exists or is a
// NotFound (existence is all attendance needs).
type fakeGroups struct {
	existing map[string]bool
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

	studentAna  = "stu-a"
	studentBeto = "stu-b"
	studentCruz = "stu-c"
)

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
// source. The roster group holds three active enrollments plus one withdrawn,
// and a second group holds an unrelated enrollment.
func newService(t *testing.T, repo *fakeRepo, pub service.Publisher, ids ...string) *service.AttendanceService {
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
	return service.NewAttendanceService(repo, enrollments, groups, pub, testTenant, fixedIDs(ids...))
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

	rows, err := svc.GetDailyRoster(context.Background(), groupID, schoolDay)
	if err != nil {
		t.Fatalf("GetDailyRoster: %v", err)
	}

	want := []service.RosterRow{
		{EnrollmentID: enrollmentAna, StudentID: studentAna, Status: "present"},
		{EnrollmentID: enrollmentBeto, StudentID: studentBeto, Status: "absent"},
		{EnrollmentID: enrollmentCruz, StudentID: studentCruz, Status: "late"},
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

			_, err := svc.GetDailyRoster(context.Background(), tt.groupID, tt.date)
			wantInvalid(t, err)
			if repo.marksForDateCalls != 0 {
				t.Errorf("repo was queried despite invalid input (%d calls)", repo.marksForDateCalls)
			}
		})
	}
}

// TestGetDailyRosterPropagatesRepoFailure proves an infrastructure failure is
// propagated unchanged, not swallowed into an empty roster.
func TestGetDailyRosterPropagatesRepoFailure(t *testing.T) {
	repo := newFakeRepo()
	repo.err = errors.New("connection reset")
	svc := newService(t, repo, nil)

	if _, err := svc.GetDailyRoster(context.Background(), groupID, schoolDay); err == nil {
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

	absent, marked, err := svc.SaveDailyAttendance(context.Background(), groupID, schoolDay, []service.Mark{
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

	if _, _, err := svc.SaveDailyAttendance(ctx, groupID, schoolDay, []service.Mark{
		{EnrollmentID: enrollmentAna, Status: "absent"},
		{EnrollmentID: enrollmentBeto, Status: "absent"},
	}); err != nil {
		t.Fatalf("first save: %v", err)
	}

	absent, marked, err := svc.SaveDailyAttendance(ctx, groupID, schoolDay, []service.Mark{
		{EnrollmentID: enrollmentBeto, Status: "late", Note: "llegó 8:15"},
	})
	if err != nil {
		t.Fatalf("re-save: %v", err)
	}
	if absent != 0 || marked != 1 {
		t.Errorf("counts = (absent %d, marked %d), want (0, 1)", absent, marked)
	}

	rows, err := svc.GetDailyRoster(ctx, groupID, schoolDay)
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

	if _, _, err := svc.SaveDailyAttendance(ctx, groupID, schoolDay, []service.Mark{
		{EnrollmentID: enrollmentAna, Status: "absent"},
	}); err != nil {
		t.Fatalf("seed save: %v", err)
	}

	absent, marked, err := svc.SaveDailyAttendance(ctx, groupID, schoolDay, []service.Mark{
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc := newService(t, repo, nil)

			absent, marked, err := svc.SaveDailyAttendance(context.Background(), tt.groupID, tt.date, tt.marks)
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

	if _, _, err := svc.SaveDailyAttendance(context.Background(), groupID, schoolDay, []service.Mark{
		{EnrollmentID: enrollmentAna, Status: "absent"},
		{EnrollmentID: enrollmentBeto, Status: "late"},
	}); err != nil {
		t.Fatalf("SaveDailyAttendance: %v", err)
	}

	wantAbsences := []service.AbsenceEvent{{
		RecordID: "rec-1", EnrollmentID: enrollmentAna, StudentID: studentAna,
		TenantID: testTenant, Date: schoolDay,
	}}
	if len(pub.absences) != 1 || pub.absences[0] != wantAbsences[0] {
		t.Errorf("absences = %+v, want %+v", pub.absences, wantAbsences)
	}
	wantMarked := []service.MarkedEvent{
		{RecordID: "rec-1", EnrollmentID: enrollmentAna, StudentID: studentAna, TenantID: testTenant, Date: schoolDay, Status: "absent"},
		{RecordID: "rec-2", EnrollmentID: enrollmentBeto, StudentID: studentBeto, TenantID: testTenant, Date: schoolDay, Status: "late"},
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
// durable, and losing a notification must never fail the teacher's save.
func TestSaveDailyAttendancePublishingIsBestEffort(t *testing.T) {
	tests := []struct {
		name string
		pub  service.Publisher
	}{
		{name: "no publisher wired", pub: nil},
		{name: "broken bus", pub: &spyPublisher{err: errors.New("pubsub unavailable")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc := newService(t, repo, tt.pub)

			absent, marked, err := svc.SaveDailyAttendance(context.Background(), groupID, schoolDay, []service.Mark{
				{EnrollmentID: enrollmentAna, Status: "absent"},
			})
			if err != nil {
				t.Fatalf("publishing must not fail the save: %v", err)
			}
			if absent != 1 || marked != 1 {
				t.Errorf("counts = (%d, %d), want (1, 1)", absent, marked)
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

	if _, _, err := svc.SaveDailyAttendance(context.Background(), groupID, schoolDay, []service.Mark{
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

			absent, marked, err := svc.SaveDailyAttendance(context.Background(), groupID, schoolDay, []service.Mark{
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

// TestSaveDailyAttendanceEmptyMarksClearsTheDay proves an empty exception set is
// a legitimate "everybody present" save: it still reconciles the roster (wiping
// any previous marks) and reports zero counts.
func TestSaveDailyAttendanceEmptyMarksClearsTheDay(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(t, repo, nil)
	ctx := context.Background()

	if _, _, err := svc.SaveDailyAttendance(ctx, groupID, schoolDay, []service.Mark{
		{EnrollmentID: enrollmentAna, Status: "absent"},
	}); err != nil {
		t.Fatalf("seed save: %v", err)
	}

	absent, marked, err := svc.SaveDailyAttendance(ctx, groupID, schoolDay, nil)
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
			repo.listResult = []service.AttendanceRecord{{ID: "rec-1", EnrollmentID: enrollmentAna, Status: "absent"}}
			svc := newService(t, repo, nil)

			got, err := svc.ListAttendance(context.Background(), enrollmentAna, tt.from, tt.to)
			if err != nil {
				t.Fatalf("ListAttendance: %v", err)
			}
			if len(got) != 1 || got[0].ID != "rec-1" {
				t.Errorf("records = %+v, want the repo's rows verbatim", got)
			}
			if repo.lastListEnrollment != enrollmentAna {
				t.Errorf("enrollment = %q, want %q", repo.lastListEnrollment, enrollmentAna)
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

			_, err := svc.ListAttendance(context.Background(), enrollmentAna, tt.from, tt.to)
			wantInvalid(t, err)
			if repo.listCalls != 0 {
				t.Errorf("repo queried despite a malformed bound (%d calls)", repo.listCalls)
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
