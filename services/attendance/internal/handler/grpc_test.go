package handler_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	attendancev1 "github.com/garcia-lorca/gen/go/garcialorca/attendance/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/attendance/internal/handler"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

// --- fakes (service-level seams) ---------------------------------------------

// fakeRepo is an in-memory store with the real reconcile semantics (the
// service's own DiffList applied to a map), so the handler tests exercise the
// same default-present behaviour the service defines.
type fakeRepo struct {
	rows     map[string]service.AttendanceRecord // enrollmentID|date|subject → record
	sessions map[string]service.Session
	list     []service.AttendanceRecord
	taker    service.Taker
	taken    []service.TakenList
}

func (f *fakeRepo) ListSessions(_ context.Context, _ []string, _, _ time.Time) ([]service.TakenList, error) {
	return f.taken, nil
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{rows: map[string]service.AttendanceRecord{}, sessions: map[string]service.Session{}}
}

func markKey(enrollmentID string, key service.ListKey) string {
	return enrollmentID + "|" + key.Date.Format("2006-01-02") + "|" + key.SubjectID
}

func listKey(key service.ListKey) string {
	return key.GroupID + "|" + key.Date.Format("2006-01-02") + "|" + key.SubjectID
}

func (f *fakeRepo) MarksForList(_ context.Context, enrollmentIDs []string, key service.ListKey) (map[string]service.AttendanceRecord, error) {
	out := make(map[string]service.AttendanceRecord, len(enrollmentIDs))
	for _, id := range enrollmentIDs {
		if rec, ok := f.rows[markKey(id, key)]; ok {
			out[id] = rec
		}
	}
	return out, nil
}

func (f *fakeRepo) SessionFor(_ context.Context, key service.ListKey) (service.Session, bool, error) {
	s, ok := f.sessions[listKey(key)]
	return s, ok, nil
}

func (f *fakeRepo) ReconcileList(ctx context.Context, key service.ListKey, enrollmentIDs []string, taker service.Taker, plan service.PlanFunc) (service.ListChanges, error) {
	current, _ := f.MarksForList(ctx, enrollmentIDs, key)
	changes, err := plan(current)
	if err != nil {
		return service.ListChanges{}, err
	}
	for _, rec := range changes.Deletes {
		delete(f.rows, markKey(rec.EnrollmentID, key))
	}
	for _, rec := range changes.Inserts {
		f.rows[markKey(rec.EnrollmentID, key)] = rec
	}
	for _, u := range changes.Updates {
		f.rows[markKey(u.Record.EnrollmentID, key)] = u.Record
	}
	f.taker = taker
	if _, ok := f.sessions[listKey(key)]; !ok {
		f.sessions[listKey(key)] = service.Session{TakenAt: takenAt, TakenByName: taker.Name, UpdatedAt: takenAt}
	}
	return changes, nil
}

func (f *fakeRepo) ListByEnrollment(_ context.Context, _ string, _, _ time.Time) ([]service.AttendanceRecord, error) {
	return f.list, nil
}

type fakeEnrollments struct {
	byGroup map[string][]service.EnrollmentRef
}

func (f *fakeEnrollments) ByGroup(_ context.Context, groupID string) ([]service.EnrollmentRef, error) {
	return f.byGroup[groupID], nil
}

type fakeGroups struct{ existing map[string]bool }

func (f *fakeGroups) ByID(_ context.Context, id string) error {
	if !f.existing[id] {
		return glerrors.ErrNotFound
	}
	return nil
}

// --- fixtures -----------------------------------------------------------------

const (
	testTenant = "tenant-1"
	groupID    = "grp-1"
	testDate   = "2026-09-15"

	// ListAttendance validates its enrollment id as a UUID at the boundary, so
	// its tests carry a real one (the readable ids above never reach it).
	enrollmentUUID = "3f2504e0-4f89-41d3-9a0c-0305e82c3301"

	takerID     = "8f000000-0000-4000-8000-000000000001"
	subjectUUID = "5b000000-0000-4000-8000-00000000000a"
)

var takenAt = time.Date(2026, 9, 15, 14, 30, 0, 0, time.UTC)

func activeEnrollment(id, studentID string) service.EnrollmentRef {
	return service.EnrollmentRef{ID: id, StudentID: studentID, GroupID: groupID, Status: "active"}
}

func newHandler(repo *fakeRepo, enrollments []service.EnrollmentRef) *handler.AttendanceHandler {
	enr := &fakeEnrollments{byGroup: map[string][]service.EnrollmentRef{groupID: enrollments}}
	groups := &fakeGroups{existing: map[string]bool{groupID: true}}
	seq := 0
	newID := func() string { seq++; return "rec-" + string(rune('0'+seq)) }
	svc := service.NewAttendanceService(repo, enr, groups, nil, testTenant, newID, slog.New(slog.DiscardHandler))
	return handler.NewAttendanceHandler(svc)
}

// --- tests ---------------------------------------------------------------------

func TestHealth(t *testing.T) {
	h := newHandler(newFakeRepo(), nil)
	res, err := h.Health(context.Background(), connect.NewRequest(&attendancev1.HealthRequest{}))
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if res.Msg.GetStatus() != "SERVING" {
		t.Errorf("status = %q, want SERVING", res.Msg.GetStatus())
	}
}

func TestGetDailyRosterDefaultsToPresent(t *testing.T) {
	h := newHandler(newFakeRepo(), []service.EnrollmentRef{
		activeEnrollment("enr-1", "stu-1"),
		activeEnrollment("enr-2", "stu-2"),
	})

	res, err := h.GetDailyRoster(context.Background(), connect.NewRequest(&attendancev1.GetDailyRosterRequest{
		GroupId: groupID, Date: testDate,
	}))
	if err != nil {
		t.Fatalf("GetDailyRoster: %v", err)
	}
	rows := res.Msg.GetRows()
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	// Deterministic order by enrollment id; no stored mark reads back present.
	if rows[0].GetEnrollmentId() != "enr-1" || rows[0].GetStudentId() != "stu-1" || rows[0].GetStatus() != "present" {
		t.Errorf("row 0 = %+v", rows[0])
	}
	if rows[1].GetEnrollmentId() != "enr-2" || rows[1].GetStatus() != "present" {
		t.Errorf("row 1 = %+v", rows[1])
	}
}

func TestGetDailyRosterUnknownGroupIsInvalidArgument(t *testing.T) {
	h := newHandler(newFakeRepo(), nil)
	_, err := h.GetDailyRoster(context.Background(), connect.NewRequest(&attendancev1.GetDailyRosterRequest{
		GroupId: "nope", Date: testDate,
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("want InvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestGetDailyRosterBadDateIsInvalidArgument(t *testing.T) {
	h := newHandler(newFakeRepo(), nil)
	_, err := h.GetDailyRoster(context.Background(), connect.NewRequest(&attendancev1.GetDailyRosterRequest{
		GroupId: groupID, Date: "15/09/2026",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("want InvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestSaveDailyAttendanceCountsAndReflectsInRoster(t *testing.T) {
	repo := newFakeRepo()
	h := newHandler(repo, []service.EnrollmentRef{
		activeEnrollment("enr-1", "stu-1"),
		activeEnrollment("enr-2", "stu-2"),
		activeEnrollment("enr-3", "stu-3"),
	})

	res, err := h.SaveDailyAttendance(context.Background(), connect.NewRequest(&attendancev1.SaveDailyAttendanceRequest{
		GroupId: groupID, Date: testDate, TakenBy: takerID, TakenByName: "Gabriela Rojas",
		Marks: []*attendancev1.Mark{
			{EnrollmentId: "enr-1", Status: "absent", Note: "sin aviso"},
			{EnrollmentId: "enr-2", Status: "late"},
		},
	}))
	if err != nil {
		t.Fatalf("SaveDailyAttendance: %v", err)
	}
	if res.Msg.GetAbsentCount() != 1 || res.Msg.GetMarkedCount() != 2 {
		t.Errorf("counts = absent %d marked %d, want 1/2", res.Msg.GetAbsentCount(), res.Msg.GetMarkedCount())
	}

	roster, err := h.GetDailyRoster(context.Background(), connect.NewRequest(&attendancev1.GetDailyRosterRequest{
		GroupId: groupID, Date: testDate,
	}))
	if err != nil {
		t.Fatalf("GetDailyRoster: %v", err)
	}
	rows := roster.Msg.GetRows()
	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(rows))
	}
	if rows[0].GetStatus() != "absent" || rows[1].GetStatus() != "late" || rows[2].GetStatus() != "present" {
		t.Errorf("statuses = %q/%q/%q", rows[0].GetStatus(), rows[1].GetStatus(), rows[2].GetStatus())
	}
}

func TestSaveDailyAttendanceMarkOutsideGroupIsInvalidArgument(t *testing.T) {
	h := newHandler(newFakeRepo(), []service.EnrollmentRef{activeEnrollment("enr-1", "stu-1")})
	_, err := h.SaveDailyAttendance(context.Background(), connect.NewRequest(&attendancev1.SaveDailyAttendanceRequest{
		GroupId: groupID, Date: testDate, TakenBy: takerID, TakenByName: "Gabriela Rojas",
		Marks: []*attendancev1.Mark{{EnrollmentId: "intruder", Status: "absent"}},
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("want InvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestListAttendanceFormatsDatesAsISO(t *testing.T) {
	repo := newFakeRepo()
	repo.list = []service.AttendanceRecord{{
		ID:           "rec-1",
		EnrollmentID: enrollmentUUID,
		Date:         time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC),
		Status:       "absent",
		Note:         "sin aviso",
	}}
	h := newHandler(repo, nil)

	res, err := h.ListAttendance(context.Background(), connect.NewRequest(&attendancev1.ListAttendanceRequest{
		EnrollmentId: enrollmentUUID, FromDate: "2026-09-01", ToDate: "2026-09-30",
	}))
	if err != nil {
		t.Fatalf("ListAttendance: %v", err)
	}
	records := res.Msg.GetRecords()
	if len(records) != 1 {
		t.Fatalf("want 1 record, got %d", len(records))
	}
	rec := records[0]
	// The wire carries a school day, never a timestamp.
	if rec.GetDate() != "2026-09-15" {
		t.Errorf("date = %q, want 2026-09-15", rec.GetDate())
	}
	if rec.GetId() != "rec-1" || rec.GetEnrollmentId() != enrollmentUUID || rec.GetStatus() != "absent" || rec.GetNote() != "sin aviso" {
		t.Errorf("record = %+v", rec)
	}
}

func TestListAttendanceBadBoundIsInvalidArgument(t *testing.T) {
	h := newHandler(newFakeRepo(), nil)
	_, err := h.ListAttendance(context.Background(), connect.NewRequest(&attendancev1.ListAttendanceRequest{
		EnrollmentId: enrollmentUUID, FromDate: "septiembre",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("want InvalidArgument, got %v", connect.CodeOf(err))
	}
}

// A malformed enrollment id is a client error at the edge too: it must map to
// InvalidArgument, never to the Internal a broken uuid literal would raise in
// Postgres.
func TestListAttendanceMalformedEnrollmentIDIsInvalidArgument(t *testing.T) {
	repo := newFakeRepo()
	h := newHandler(repo, nil)
	_, err := h.ListAttendance(context.Background(), connect.NewRequest(&attendancev1.ListAttendanceRequest{
		EnrollmentId: "abc",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("want InvalidArgument, got %v", connect.CodeOf(err))
	}
}

// T3 wire contract: a per-class save carries its subject and taker through, the
// roster returns notes and the session, and records carry their subject.
func TestPerClassListRoundTripsSubjectNoteAndSession(t *testing.T) {
	repo := newFakeRepo()
	h := newHandler(repo, []service.EnrollmentRef{activeEnrollment("enr-1", "stu-1")})
	ctx := context.Background()

	before, err := h.GetDailyRoster(ctx, connect.NewRequest(&attendancev1.GetDailyRosterRequest{
		GroupId: groupID, Date: testDate, SubjectId: subjectUUID,
	}))
	if err != nil {
		t.Fatalf("GetDailyRoster: %v", err)
	}
	if before.Msg.GetSession() != nil {
		t.Errorf("an untaken list must have no session, got %v", before.Msg.GetSession())
	}

	if _, err := h.SaveDailyAttendance(ctx, connect.NewRequest(&attendancev1.SaveDailyAttendanceRequest{
		GroupId: groupID, Date: testDate, SubjectId: subjectUUID,
		TakenBy: takerID, TakenByName: "Gabriela Rojas",
		Marks: []*attendancev1.Mark{{EnrollmentId: "enr-1", Status: "excused", Note: "cita médica"}},
	})); err != nil {
		t.Fatalf("SaveDailyAttendance: %v", err)
	}
	if repo.taker.UserID != takerID || repo.taker.Name != "Gabriela Rojas" {
		t.Errorf("taker = %+v", repo.taker)
	}

	after, err := h.GetDailyRoster(ctx, connect.NewRequest(&attendancev1.GetDailyRosterRequest{
		GroupId: groupID, Date: testDate, SubjectId: subjectUUID,
	}))
	if err != nil {
		t.Fatalf("GetDailyRoster: %v", err)
	}
	row := after.Msg.GetRows()[0]
	if row.GetStatus() != "excused" || row.GetNote() != "cita médica" {
		t.Errorf("row = %v, want excused with its note", row)
	}
	session := after.Msg.GetSession()
	if session.GetTakenByName() != "Gabriela Rojas" || !session.GetTakenAt().AsTime().Equal(takenAt) || !session.GetUpdatedAt().AsTime().Equal(takenAt) {
		t.Errorf("session = %v", session)
	}

	daily, err := h.GetDailyRoster(ctx, connect.NewRequest(&attendancev1.GetDailyRosterRequest{GroupId: groupID, Date: testDate}))
	if err != nil {
		t.Fatalf("GetDailyRoster daily: %v", err)
	}
	if daily.Msg.GetRows()[0].GetStatus() != "present" || daily.Msg.GetSession() != nil {
		t.Errorf("the daily list must not see the per-class save: %v", daily.Msg)
	}
}

func TestSaveDailyAttendanceWithoutTakerIsInvalidArgument(t *testing.T) {
	h := newHandler(newFakeRepo(), []service.EnrollmentRef{activeEnrollment("enr-1", "stu-1")})
	_, err := h.SaveDailyAttendance(context.Background(), connect.NewRequest(&attendancev1.SaveDailyAttendanceRequest{
		GroupId: groupID, Date: testDate,
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("want InvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestListAttendanceCarriesSubject(t *testing.T) {
	repo := newFakeRepo()
	repo.list = []service.AttendanceRecord{{ID: "r1", EnrollmentID: enrollmentUUID, Date: takenAt, SubjectID: subjectUUID, Status: "absent"}}
	h := newHandler(repo, nil)
	res, err := h.ListAttendance(context.Background(), connect.NewRequest(&attendancev1.ListAttendanceRequest{EnrollmentId: enrollmentUUID}))
	if err != nil {
		t.Fatalf("ListAttendance: %v", err)
	}
	if res.Msg.GetRecords()[0].GetSubjectId() != subjectUUID {
		t.Errorf("record = %v, want its subject", res.Msg.GetRecords()[0])
	}
}
