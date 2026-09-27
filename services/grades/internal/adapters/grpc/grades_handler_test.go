package grpcadapter_test

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	grpcadapter "github.com/garcia-lorca/services/grades/internal/adapters/grpc"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
	"github.com/garcia-lorca/services/grades/internal/usecase"
)

// --- fakes (usecase-level ports) --------------------------------------------

type fakeAssignments struct {
	assignments map[string]ports.AssignmentRef
	terms       map[string]ports.TermRef
}

func (f *fakeAssignments) ByID(_ context.Context, id string) (ports.AssignmentRef, error) {
	a, ok := f.assignments[id]
	if !ok {
		return ports.AssignmentRef{}, glerrors.ErrNotFound
	}
	return a, nil
}

func (f *fakeAssignments) TermByID(_ context.Context, id string) (ports.TermRef, error) {
	tm, ok := f.terms[id]
	if !ok {
		return ports.TermRef{}, glerrors.ErrNotFound
	}
	return tm, nil
}

type fakeEnrollments struct {
	byID      map[string]ports.EnrollmentRef
	transfers []ports.TransferRef
}

func (f *fakeEnrollments) ByCycle(_ context.Context, cycle string) ([]ports.EnrollmentRef, error) {
	var out []ports.EnrollmentRef
	for _, e := range f.byID {
		if e.CycleID == cycle {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeEnrollments) TransfersIntoGroup(_ context.Context, group string) ([]ports.TransferRef, error) {
	var out []ports.TransferRef
	for _, t := range f.transfers {
		if t.ToGroupID == group {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeEnrollments) ByID(_ context.Context, id string) (ports.EnrollmentRef, error) {
	e, ok := f.byID[id]
	if !ok {
		return ports.EnrollmentRef{}, glerrors.ErrNotFound
	}
	return e, nil
}

func (f *fakeEnrollments) ByGroup(_ context.Context, groupID string) ([]ports.EnrollmentRef, error) {
	var out []ports.EnrollmentRef
	for _, e := range f.byID {
		if e.GroupID == groupID {
			out = append(out, e)
		}
	}
	return out, nil
}

type fakeEntries struct {
	rows map[string]domain.GradeEntry
}

func newFakeEntries() *fakeEntries { return &fakeEntries{rows: map[string]domain.GradeEntry{}} }

func key(enrollmentID, subjectID, termID string) string {
	return enrollmentID + "|" + subjectID + "|" + termID
}

func (f *fakeEntries) UpsertGradeEntry(_ context.Context, e domain.GradeEntry) (domain.GradeEntry, error) {
	f.rows[key(e.EnrollmentID(), e.SubjectID(), e.TermID())] = e
	return e, nil
}

func (f *fakeEntries) GetForKeys(_ context.Context, enrollmentIDs []string, subjectID, termID string) (map[string]domain.GradeEntry, error) {
	out := make(map[string]domain.GradeEntry, len(enrollmentIDs))
	for _, id := range enrollmentIDs {
		if e, ok := f.rows[key(id, subjectID, termID)]; ok {
			out[id] = e
		}
	}
	return out, nil
}

func (f *fakeEntries) ListGradeEntries(_ context.Context, enrollmentID, termID string) ([]domain.GradeEntry, error) {
	var out []domain.GradeEntry
	for _, e := range f.rows {
		if e.EnrollmentID() == enrollmentID && (termID == "" || e.TermID() == termID) {
			out = append(out, e)
		}
	}
	return out, nil
}

// --- fixtures ---------------------------------------------------------------

const (
	testTenant   = "tenant-1"
	assignmentID = "asg-1"
	groupID      = "grp-1"
	subjectID    = "subj-1"
	teacherID    = "teacher-1"
	cycleID      = "cyc-1"
	termID       = "term-1"
	enrollmentID = "enr-1"
)

func baseAssignments() *fakeAssignments {
	return &fakeAssignments{
		assignments: map[string]ports.AssignmentRef{
			assignmentID: {ID: assignmentID, GroupID: groupID, SubjectID: subjectID, TeacherID: teacherID, CycleID: cycleID},
		},
		terms: map[string]ports.TermRef{termID: {ID: termID, CycleID: cycleID, LevelID: levelID}},
	}
}

func activeEnrollment(id, studentID string) ports.EnrollmentRef {
	return ports.EnrollmentRef{ID: id, StudentID: studentID, GroupID: groupID, CycleID: cycleID, Status: "active"}
}

// monthlyNow is the monthly surface's clock: mid-October, so September and
// October are gradable and November is still in the future.
func monthlyNow() time.Time { return time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC) }

func newHandler(a ports.AssignmentReader, e ports.EnrollmentReader, r ports.GradeEntryRepository) *grpcadapter.GradesHandler {
	now := func() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) }
	seq := 0
	newID := func() string { seq++; return "ge-" + string(rune('0'+seq)) }
	catalog := newFakeCatalog()
	svc := usecase.NewGradesService(r, a, catalog, e, testTenant, newID, now)
	monthly := usecase.NewMonthlyGradesService(newFakeMonthly(), a, catalog, e, testTenant, newID, monthlyNow)
	return grpcadapter.NewGradesHandler(svc, monthly, usecase.NewHealthService("grades"))
}

// --- tests ------------------------------------------------------------------

func TestHealth(t *testing.T) {
	h := newHandler(baseAssignments(), &fakeEnrollments{byID: map[string]ports.EnrollmentRef{}}, newFakeEntries())
	res, err := h.Health(context.Background(), connect.NewRequest(&gradesv1.HealthRequest{}))
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if res.Msg.GetStatus() != "SERVING" {
		t.Errorf("status = %q, want SERVING", res.Msg.GetStatus())
	}
}

func TestCaptureGradeHappyLeavesPublishedAtNil(t *testing.T) {
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{enrollmentID: activeEnrollment(enrollmentID, "stu-1")}}
	h := newHandler(baseAssignments(), enr, newFakeEntries())

	res, err := h.CaptureGrade(context.Background(), connect.NewRequest(&gradesv1.CaptureGradeRequest{
		AssignmentId: assignmentID, EnrollmentId: enrollmentID, TermId: termID, Value: 8.5,
	}))
	if err != nil {
		t.Fatalf("CaptureGrade: %v", err)
	}
	entry := res.Msg.GetEntry()
	// subject/teacher are DERIVED from the assignment, never taken from the request.
	if entry.GetSubjectId() != subjectID || entry.GetTeacherId() != teacherID {
		t.Errorf("subject/teacher not derived: %+v", entry)
	}
	if entry.GetValue() != 8.5 || entry.GetEnrollmentId() != enrollmentID || entry.GetTermId() != termID {
		t.Errorf("entry mismatch: %+v", entry)
	}
	// published_at is reserved for the future boleta flow — never set on capture.
	if entry.GetPublishedAt() != nil {
		t.Errorf("published_at must be nil on capture, got %v", entry.GetPublishedAt())
	}
}

func TestCaptureGradeBadEnrollmentIsInvalidArgument(t *testing.T) {
	h := newHandler(baseAssignments(), &fakeEnrollments{byID: map[string]ports.EnrollmentRef{}}, newFakeEntries())
	_, err := h.CaptureGrade(context.Background(), connect.NewRequest(&gradesv1.CaptureGradeRequest{
		AssignmentId: assignmentID, EnrollmentId: "nope", TermId: termID, Value: 5,
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("want InvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestCaptureGradeBadTermIsInvalidArgument(t *testing.T) {
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{enrollmentID: activeEnrollment(enrollmentID, "stu-1")}}
	h := newHandler(baseAssignments(), enr, newFakeEntries())
	_, err := h.CaptureGrade(context.Background(), connect.NewRequest(&gradesv1.CaptureGradeRequest{
		AssignmentId: assignmentID, EnrollmentId: enrollmentID, TermId: "nope", Value: 5,
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("want InvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestCaptureGradeOutOfRangeValueIsInvalidArgument(t *testing.T) {
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{enrollmentID: activeEnrollment(enrollmentID, "stu-1")}}
	h := newHandler(baseAssignments(), enr, newFakeEntries())
	_, err := h.CaptureGrade(context.Background(), connect.NewRequest(&gradesv1.CaptureGradeRequest{
		AssignmentId: assignmentID, EnrollmentId: enrollmentID, TermId: termID, Value: 11,
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("want InvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestGetRosterReturnsRows(t *testing.T) {
	graded := activeEnrollment("enr-graded", "stu-1")
	ungraded := activeEnrollment("enr-ungraded", "stu-2")
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{graded.ID: graded, ungraded.ID: ungraded}}
	repo := newFakeEntries()
	h := newHandler(baseAssignments(), enr, repo)

	if _, err := h.CaptureGrade(context.Background(), connect.NewRequest(&gradesv1.CaptureGradeRequest{
		AssignmentId: assignmentID, EnrollmentId: graded.ID, TermId: termID, Value: 7.5,
	})); err != nil {
		t.Fatalf("seed capture: %v", err)
	}

	res, err := h.GetRoster(context.Background(), connect.NewRequest(&gradesv1.GetRosterRequest{
		AssignmentId: assignmentID, TermId: termID,
	}))
	if err != nil {
		t.Fatalf("GetRoster: %v", err)
	}
	rows := res.Msg.GetRows()
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	// Deterministic order by enrollment id.
	if rows[0].GetEnrollmentId() != "enr-graded" || !rows[0].GetHasValue() || rows[0].GetValue() != 7.5 {
		t.Errorf("graded row wrong: %+v", rows[0])
	}
	if rows[1].GetEnrollmentId() != "enr-ungraded" || rows[1].GetHasValue() {
		t.Errorf("ungraded row wrong: %+v", rows[1])
	}
}

func TestListGradeEntries(t *testing.T) {
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{enrollmentID: activeEnrollment(enrollmentID, "stu-1")}}
	repo := newFakeEntries()
	h := newHandler(baseAssignments(), enr, repo)
	if _, err := h.CaptureGrade(context.Background(), connect.NewRequest(&gradesv1.CaptureGradeRequest{
		AssignmentId: assignmentID, EnrollmentId: enrollmentID, TermId: termID, Value: 5,
	})); err != nil {
		t.Fatalf("capture: %v", err)
	}

	res, err := h.ListGradeEntries(context.Background(), connect.NewRequest(&gradesv1.ListGradeEntriesRequest{
		EnrollmentId: enrollmentID, TermId: termID,
	}))
	if err != nil {
		t.Fatalf("ListGradeEntries: %v", err)
	}
	entries := res.Msg.GetEntries()
	if len(entries) != 1 || entries[0].GetEnrollmentId() != enrollmentID || entries[0].GetPublishedAt() != nil {
		t.Errorf("list mismatch: %+v", entries)
	}
}
