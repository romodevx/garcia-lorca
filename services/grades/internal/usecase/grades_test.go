package usecase_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
	"github.com/garcia-lorca/services/grades/internal/usecase"
)

// --- fakes -------------------------------------------------------------------

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

func (f *fakeEnrollments) TransfersIntoGroup(_ context.Context, groupID string) ([]ports.TransferRef, error) {
	var out []ports.TransferRef
	for _, t := range f.transfers {
		if t.ToGroupID == groupID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeEnrollments) ByCycle(_ context.Context, cycleID string) ([]ports.EnrollmentRef, error) {
	var out []ports.EnrollmentRef
	for _, e := range f.byID {
		if e.CycleID == cycleID {
			out = append(out, e)
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

// fakeEntries is an in-memory upsert store keyed by (enrollment, subject, term).
// It records how GetForKeys is called so the roster test can prove the lookup is
// batched into exactly ONE query (no N+1).
type fakeEntries struct {
	rows map[string]domain.GradeEntry

	getForKeysCalls int
	lastKeyIDs      []string
	lastSubjectID   string
	lastTermID      string
}

func newFakeEntries() *fakeEntries { return &fakeEntries{rows: map[string]domain.GradeEntry{}} }

func entryKey(enrollmentID, subjectID, termID string) string {
	return enrollmentID + "|" + subjectID + "|" + termID
}

func (f *fakeEntries) UpsertGradeEntry(_ context.Context, e domain.GradeEntry) (domain.GradeEntry, error) {
	f.rows[entryKey(e.EnrollmentID(), e.SubjectID(), e.TermID())] = e
	return e, nil
}

func (f *fakeEntries) GetForKeys(_ context.Context, enrollmentIDs []string, subjectID, termID string) (map[string]domain.GradeEntry, error) {
	f.getForKeysCalls++
	f.lastKeyIDs = append([]string(nil), enrollmentIDs...)
	f.lastSubjectID = subjectID
	f.lastTermID = termID

	out := make(map[string]domain.GradeEntry, len(enrollmentIDs))
	for _, id := range enrollmentIDs {
		if e, ok := f.rows[entryKey(id, subjectID, termID)]; ok {
			out[id] = e
		}
	}
	return out, nil
}

// getForKey is a test-only convenience read (not part of the repository port,
// which now batches via GetForKeys) used to assert what CaptureGrade persisted.
func (f *fakeEntries) getForKey(enrollmentID, subjectID, termID string) (domain.GradeEntry, bool) {
	e, ok := f.rows[entryKey(enrollmentID, subjectID, termID)]
	return e, ok
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

// --- fixtures ----------------------------------------------------------------

const (
	testTenant     = "tenant-1"
	assignmentID   = "asg-1"
	groupID        = "grp-1"
	subjectID      = "subj-1"
	teacherID      = "teacher-1"
	cycleID        = "cyc-1"
	termID         = "term-1"
	enrollmentID   = "enr-1"
	otherStudentID = "stu-2"
)

func baseAssignments() *fakeAssignments {
	return &fakeAssignments{
		assignments: map[string]ports.AssignmentRef{
			assignmentID: {ID: assignmentID, GroupID: groupID, SubjectID: subjectID, TeacherID: teacherID, CycleID: cycleID},
		},
		terms: map[string]ports.TermRef{
			termID: {ID: termID, CycleID: cycleID, LevelID: levelPrimaria},
		},
	}
}

func activeEnrollment(id, studentID string) ports.EnrollmentRef {
	return ports.EnrollmentRef{ID: id, StudentID: studentID, GroupID: groupID, CycleID: cycleID, Status: "active"}
}

func idSet(ids []string) map[string]bool {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}

func newService(a ports.AssignmentReader, e ports.EnrollmentReader, r ports.GradeEntryRepository) *usecase.GradesService {
	fixedNow := func() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) }
	seq := 0
	newID := func() string { seq++; return "ge-" + string(rune('0'+seq)) }
	return usecase.NewGradesService(r, a, legacyCatalog(), e, testTenant, newID, fixedNow)
}

// --- CaptureGrade ------------------------------------------------------------

func TestCaptureGradeHappyDerivesSubjectAndTeacher(t *testing.T) {
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{enrollmentID: activeEnrollment(enrollmentID, "stu-1")}}
	repo := newFakeEntries()
	svc := newService(baseAssignments(), enr, repo)

	got, err := svc.CaptureGrade(context.Background(), assignmentID, enrollmentID, termID, 8.5)
	if err != nil {
		t.Fatalf("CaptureGrade: %v", err)
	}
	// Subject and teacher are DERIVED from the assignment, never client input.
	if got.SubjectID() != subjectID || got.TeacherID() != teacherID {
		t.Errorf("subject/teacher not derived from assignment: subj=%q teacher=%q", got.SubjectID(), got.TeacherID())
	}
	if got.EnrollmentID() != enrollmentID || got.TermID() != termID || got.Value() != 8.5 || got.TenantID() != testTenant {
		t.Errorf("stored entry mismatch: %+v", got)
	}
	stored, ok := repo.getForKey(enrollmentID, subjectID, termID)
	if !ok || stored.Value() != 8.5 {
		t.Errorf("value not persisted: %+v, ok=%v", stored, ok)
	}
}

func TestCaptureGradeMissingAssignment(t *testing.T) {
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{enrollmentID: activeEnrollment(enrollmentID, "stu-1")}}
	svc := newService(baseAssignments(), enr, newFakeEntries())

	_, err := svc.CaptureGrade(context.Background(), "nope", enrollmentID, termID, 5)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v", err)
	}
}

func TestCaptureGradeMissingEnrollment(t *testing.T) {
	svc := newService(baseAssignments(), &fakeEnrollments{byID: map[string]ports.EnrollmentRef{}}, newFakeEntries())

	_, err := svc.CaptureGrade(context.Background(), assignmentID, "nope", termID, 5)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v", err)
	}
}

func TestCaptureGradeMissingTerm(t *testing.T) {
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{enrollmentID: activeEnrollment(enrollmentID, "stu-1")}}
	svc := newService(baseAssignments(), enr, newFakeEntries())

	_, err := svc.CaptureGrade(context.Background(), assignmentID, enrollmentID, "nope", 5)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v", err)
	}
}

func TestCaptureGradeEnrollmentNotInGroup(t *testing.T) {
	other := ports.EnrollmentRef{ID: enrollmentID, StudentID: "stu-1", GroupID: "grp-other", CycleID: cycleID, Status: "active"}
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{enrollmentID: other}}
	svc := newService(baseAssignments(), enr, newFakeEntries())

	_, err := svc.CaptureGrade(context.Background(), assignmentID, enrollmentID, termID, 5)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v", err)
	}
}

func TestCaptureGradeNonActiveEnrollment(t *testing.T) {
	inactive := ports.EnrollmentRef{ID: enrollmentID, StudentID: "stu-1", GroupID: groupID, CycleID: cycleID, Status: "withdrawn"}
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{enrollmentID: inactive}}
	svc := newService(baseAssignments(), enr, newFakeEntries())

	_, err := svc.CaptureGrade(context.Background(), assignmentID, enrollmentID, termID, 5)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v", err)
	}
}

func TestCaptureGradeEnrollmentWrongCycle(t *testing.T) {
	wrong := ports.EnrollmentRef{ID: enrollmentID, StudentID: "stu-1", GroupID: groupID, CycleID: "cyc-other", Status: "active"}
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{enrollmentID: wrong}}
	svc := newService(baseAssignments(), enr, newFakeEntries())

	_, err := svc.CaptureGrade(context.Background(), assignmentID, enrollmentID, termID, 5)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v", err)
	}
}

func TestCaptureGradeTermNotInCycle(t *testing.T) {
	asg := baseAssignments()
	asg.terms["term-other"] = ports.TermRef{ID: "term-other", CycleID: "cyc-other"}
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{enrollmentID: activeEnrollment(enrollmentID, "stu-1")}}
	svc := newService(asg, enr, newFakeEntries())

	_, err := svc.CaptureGrade(context.Background(), assignmentID, enrollmentID, "term-other", 5)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v", err)
	}
}

func TestCaptureGradeOutOfRangeStoresNothing(t *testing.T) {
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{enrollmentID: activeEnrollment(enrollmentID, "stu-1")}}
	repo := newFakeEntries()
	svc := newService(baseAssignments(), enr, repo)

	_, err := svc.CaptureGrade(context.Background(), assignmentID, enrollmentID, termID, 10.01)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v", err)
	}
	if len(repo.rows) != 0 {
		t.Errorf("nothing must be stored on an invalid value, got %d rows", len(repo.rows))
	}
}

func TestCaptureGradeReCaptureUpdatesValue(t *testing.T) {
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{enrollmentID: activeEnrollment(enrollmentID, "stu-1")}}
	repo := newFakeEntries()
	svc := newService(baseAssignments(), enr, repo)

	if _, err := svc.CaptureGrade(context.Background(), assignmentID, enrollmentID, termID, 6); err != nil {
		t.Fatalf("first capture: %v", err)
	}
	if _, err := svc.CaptureGrade(context.Background(), assignmentID, enrollmentID, termID, 9); err != nil {
		t.Fatalf("re-capture: %v", err)
	}
	if len(repo.rows) != 1 {
		t.Errorf("re-capture must upsert one row, got %d", len(repo.rows))
	}
	stored, _ := repo.getForKey(enrollmentID, subjectID, termID)
	if stored.Value() != 9 {
		t.Errorf("re-capture did not update value: %v", stored.Value())
	}
}

// --- Roster ------------------------------------------------------------------

func TestRosterActiveEnrollmentsWithAndWithoutValue(t *testing.T) {
	graded := activeEnrollment("enr-graded", "stu-1")
	ungraded := activeEnrollment("enr-ungraded", "stu-2")
	inactive := ports.EnrollmentRef{ID: "enr-inactive", StudentID: "stu-3", GroupID: groupID, CycleID: cycleID, Status: "withdrawn"}
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{
		graded.ID: graded, ungraded.ID: ungraded, inactive.ID: inactive,
	}}
	repo := newFakeEntries()
	svc := newService(baseAssignments(), enr, repo)

	// Pre-existing grade for the graded enrollment only.
	if _, err := svc.CaptureGrade(context.Background(), assignmentID, graded.ID, termID, 7.5); err != nil {
		t.Fatalf("seed capture: %v", err)
	}

	rows, err := svc.Roster(context.Background(), assignmentID, termID)
	if err != nil {
		t.Fatalf("Roster: %v", err)
	}

	// The roster must batch grade lookups into exactly ONE call regardless of the
	// number of active enrollments (no N+1).
	if repo.getForKeysCalls != 1 {
		t.Fatalf("Roster must batch grade lookups into ONE GetForKeys call, got %d", repo.getForKeysCalls)
	}
	if repo.lastSubjectID != subjectID || repo.lastTermID != termID {
		t.Errorf("GetForKeys called with wrong subject/term: subj=%q term=%q", repo.lastSubjectID, repo.lastTermID)
	}
	if got := idSet(repo.lastKeyIDs); !got["enr-graded"] || !got["enr-ungraded"] || len(got) != 2 {
		t.Errorf("GetForKeys called with wrong enrollment ids (want only active enr-graded, enr-ungraded): %v", repo.lastKeyIDs)
	}

	if len(rows) != 2 {
		t.Fatalf("want 2 active rows (inactive excluded), got %d: %+v", len(rows), rows)
	}
	// Deterministic order by enrollmentID: enr-graded < enr-ungraded.
	if rows[0].EnrollmentID != "enr-graded" || rows[1].EnrollmentID != "enr-ungraded" {
		t.Fatalf("rows not ordered by enrollmentID: %+v", rows)
	}
	if !rows[0].HasValue || rows[0].Value != 7.5 || rows[0].StudentID != "stu-1" {
		t.Errorf("graded row wrong: %+v", rows[0])
	}
	if rows[1].HasValue || rows[1].Value != 0 || rows[1].StudentID != "stu-2" {
		t.Errorf("ungraded row wrong: %+v", rows[1])
	}
}

func TestRosterTermNotInCycle(t *testing.T) {
	asg := baseAssignments()
	asg.terms["term-other"] = ports.TermRef{ID: "term-other", CycleID: "cyc-other"}
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{enrollmentID: activeEnrollment(enrollmentID, "stu-1")}}
	svc := newService(asg, enr, newFakeEntries())

	_, err := svc.Roster(context.Background(), assignmentID, "term-other")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("want KindInvalid, got %v", err)
	}
}

// --- ListGradeEntries --------------------------------------------------------

func TestListGradeEntriesPassthrough(t *testing.T) {
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{enrollmentID: activeEnrollment(enrollmentID, "stu-1")}}
	repo := newFakeEntries()
	svc := newService(baseAssignments(), enr, repo)
	if _, err := svc.CaptureGrade(context.Background(), assignmentID, enrollmentID, termID, 5); err != nil {
		t.Fatalf("capture: %v", err)
	}

	got, err := svc.ListGradeEntries(context.Background(), enrollmentID, termID)
	if err != nil {
		t.Fatalf("ListGradeEntries: %v", err)
	}
	if len(got) != 1 || got[0].EnrollmentID() != enrollmentID {
		t.Errorf("passthrough mismatch: %+v", got)
	}
}

// legacyCatalog places the test group in Primaria for the term-level check.
func legacyCatalog() *fakeCatalog {
	return &fakeCatalog{levelByGroup: map[string]ports.LevelRef{groupID: {ID: levelPrimaria, GradingEnabled: true}}}
}

// A term belongs to a cycle AND a level: a Secundaria term must not grade a
// Primaria group even inside the same cycle (T2 QA finding).
func TestLegacyCaptureAndRosterRejectATermOfAnotherLevel(t *testing.T) {
	asg := baseAssignments()
	asg.terms["term-sec"] = ports.TermRef{ID: "term-sec", CycleID: cycleID, LevelID: "lvl-sec"}
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{enrollmentID: activeEnrollment(enrollmentID, "stu-1")}}
	repo := newFakeEntries()
	svc := newService(asg, enr, repo)

	_, err := svc.CaptureGrade(context.Background(), assignmentID, enrollmentID, "term-sec", 8)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("capture with another level's term: want KindInvalid, got %v", err)
	}
	if len(repo.rows) != 0 {
		t.Error("a refused capture must not be stored")
	}
	if _, err := svc.Roster(context.Background(), assignmentID, "term-sec"); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("roster with another level's term: want KindInvalid, got %v", err)
	}
	if _, err := svc.CaptureGrade(context.Background(), assignmentID, enrollmentID, termID, 8); err != nil {
		t.Errorf("the group's own level term must still capture: %v", err)
	}
}

// The legacy term check needs only the group's level ID: a level whose monthly
// scale is unusable must not stop term capture (review finding 4).
func TestLegacyCaptureIgnoresTheMonthlyScale(t *testing.T) {
	asg := baseAssignments()
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{enrollmentID: activeEnrollment(enrollmentID, "stu-1")}}
	repo := newFakeEntries()
	catalog := legacyCatalog()
	catalog.scaleErr = glerrors.New(glerrors.KindInvalid, "La escala de calificación del nivel no está configurada")
	svc := usecase.NewGradesService(repo, asg, catalog, enr, testTenant, func() string { return "ge-1" }, func() time.Time { return time.Time{} })

	if _, err := svc.CaptureGrade(context.Background(), assignmentID, enrollmentID, termID, 8); err != nil {
		t.Fatalf("legacy capture must not depend on the scale: %v", err)
	}
	if _, err := svc.Roster(context.Background(), assignmentID, termID); err != nil {
		t.Fatalf("legacy roster must not depend on the scale: %v", err)
	}
}
