package usecase_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

// seedTeacher creates a valid teacher for assignment tests.
func seedTeacher(t *testing.T, svc *usecase.CatalogService) domain.Teacher {
	t.Helper()
	te, err := svc.CreateTeacher(ctx(), "Ada", "Lovelace", "ada@example.com", "")
	if err != nil {
		t.Fatalf("seed teacher: %v", err)
	}
	return te
}

// --- Teacher ---------------------------------------------------------------

func TestCreateTeacher(t *testing.T) {
	svc, repo := newService(t)
	te, err := svc.CreateTeacher(ctx(), "Ada", "Lovelace", "ada@example.com", "")
	if err != nil {
		t.Fatalf("CreateTeacher: %v", err)
	}
	if te.ID() == "" || te.TenantID() != tenant || te.Email() != "ada@example.com" {
		t.Errorf("teacher wrong: %+v", te)
	}
	if te.HasUser() {
		t.Errorf("expected no IAM user link, got UserID=%q", te.UserID())
	}
	if len(repo.teachers) != 1 {
		t.Errorf("expected 1 persisted teacher, got %d", len(repo.teachers))
	}
}

func TestCreateTeacherInvalidArg(t *testing.T) {
	svc, repo := newService(t)
	// malformed email → constructor KindInvalid.
	if _, err := svc.CreateTeacher(ctx(), "Ada", "Lovelace", "not-an-email", ""); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("bad email: want KindInvalid, got %v", err)
	}
	if len(repo.teachers) != 0 {
		t.Errorf("nothing should be persisted on invalid input, got %d", len(repo.teachers))
	}
}

func TestCreateTeacherDuplicateEmailIsConflict(t *testing.T) {
	svc, _ := newService(t)
	seedTeacher(t, svc)
	if _, err := svc.CreateTeacher(ctx(), "Grace", "Hopper", "ada@example.com", ""); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("duplicate email: want KindConflict, got %v", err)
	}
}

func TestListTeachers(t *testing.T) {
	svc, _ := newService(t)
	seedTeacher(t, svc)
	got, err := svc.ListTeachers(ctx())
	if err != nil || len(got) != 1 {
		t.Errorf("ListTeachers: %d %v", len(got), err)
	}
}

// --- TeachingAssignment (the knot) -----------------------------------------

// seedKnotParents builds a full, self-consistent parent chain: a cycle, a level,
// a grade level, a group under (cycle, grade level), a subject under that same
// grade level, and a teacher. It returns the group, the subject, and the teacher
// ready for a valid assignment.
func seedKnotParents(t *testing.T, svc *usecase.CatalogService) (domain.Group, domain.Subject, domain.Teacher) {
	t.Helper()
	c := seedCycle(t, svc)
	l := seedLevel(t, svc)
	gl := seedGradeLevel(t, svc, l.ID())
	grp, err := svc.CreateGroup(ctx(), c.ID(), gl.ID(), "A", 30)
	if err != nil {
		t.Fatalf("seed group: %v", err)
	}
	subj, err := svc.CreateSubject(ctx(), gl.ID(), "Matemáticas")
	if err != nil {
		t.Fatalf("seed subject: %v", err)
	}
	te := seedTeacher(t, svc)
	return grp, subj, te
}

func TestCreateTeachingAssignmentDerivesCycleFromGroup(t *testing.T) {
	svc, repo := newService(t)
	grp, subj, te := seedKnotParents(t, svc)

	ta, err := svc.CreateTeachingAssignment(ctx(), grp.ID(), subj.ID(), te.ID())
	if err != nil {
		t.Fatalf("CreateTeachingAssignment: %v", err)
	}
	if ta.ID() == "" || ta.TenantID() != tenant {
		t.Errorf("assignment identity wrong: %+v", ta)
	}
	if ta.GroupID() != grp.ID() || ta.SubjectID() != subj.ID() || ta.TeacherID() != te.ID() {
		t.Errorf("assignment references wrong: %+v", ta)
	}
	// The cycle is DERIVED from the group, never supplied by the caller.
	if ta.CycleID() != grp.CycleID() {
		t.Errorf("cycle not derived from group: assignment cycle=%q group cycle=%q", ta.CycleID(), grp.CycleID())
	}
	if len(repo.assignments) != 1 {
		t.Errorf("expected 1 persisted assignment, got %d", len(repo.assignments))
	}
	// The persisted assignment carries the derived cycle too.
	if got := repo.assignments[ta.ID()]; got.CycleID() != grp.CycleID() {
		t.Errorf("persisted cycle=%q, want group's cycle %q", got.CycleID(), grp.CycleID())
	}
}

func TestCreateTeachingAssignmentMissingGroupIsInvalid(t *testing.T) {
	svc, repo := newService(t)
	_, subj, te := seedKnotParents(t, svc)
	_, err := svc.CreateTeachingAssignment(ctx(), "no-such-group", subj.ID(), te.ID())
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("missing group: want KindInvalid, got %v", err)
	}
	if len(repo.assignments) != 0 {
		t.Errorf("nothing persisted when group missing, got %d", len(repo.assignments))
	}
}

func TestCreateTeachingAssignmentMissingSubjectIsInvalid(t *testing.T) {
	svc, repo := newService(t)
	grp, _, te := seedKnotParents(t, svc)
	_, err := svc.CreateTeachingAssignment(ctx(), grp.ID(), "no-such-subject", te.ID())
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("missing subject: want KindInvalid, got %v", err)
	}
	if len(repo.assignments) != 0 {
		t.Errorf("nothing persisted when subject missing, got %d", len(repo.assignments))
	}
}

func TestCreateTeachingAssignmentGradeLevelMismatchIsInvalid(t *testing.T) {
	svc, repo := newService(t)
	c := seedCycle(t, svc)
	l := seedLevel(t, svc)
	// Two distinct grade levels under the same level.
	gl1 := seedGradeLevel(t, svc, l.ID())
	gl2, err := svc.CreateGradeLevel(ctx(), l.ID(), "2° Primaria", 2)
	if err != nil {
		t.Fatalf("second grade level: %v", err)
	}
	// Group lives under gl1; subject lives under gl2 — a mismatch.
	grp, err := svc.CreateGroup(ctx(), c.ID(), gl1.ID(), "A", 30)
	if err != nil {
		t.Fatalf("group: %v", err)
	}
	subj, err := svc.CreateSubject(ctx(), gl2.ID(), "Historia")
	if err != nil {
		t.Fatalf("subject: %v", err)
	}
	te := seedTeacher(t, svc)

	_, err = svc.CreateTeachingAssignment(ctx(), grp.ID(), subj.ID(), te.ID())
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("grade-level mismatch: want KindInvalid, got %v", err)
	}
	if len(repo.assignments) != 0 {
		t.Errorf("nothing persisted on grade-level mismatch, got %d", len(repo.assignments))
	}
}

func TestCreateTeachingAssignmentMissingTeacherIsInvalid(t *testing.T) {
	svc, repo := newService(t)
	grp, subj, _ := seedKnotParents(t, svc)
	_, err := svc.CreateTeachingAssignment(ctx(), grp.ID(), subj.ID(), "no-such-teacher")
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("missing teacher: want KindInvalid, got %v", err)
	}
	if len(repo.assignments) != 0 {
		t.Errorf("nothing persisted when teacher missing, got %d", len(repo.assignments))
	}
}

func TestCreateTeachingAssignmentDuplicateIsConflict(t *testing.T) {
	svc, _ := newService(t)
	grp, subj, te := seedKnotParents(t, svc)
	if _, err := svc.CreateTeachingAssignment(ctx(), grp.ID(), subj.ID(), te.ID()); err != nil {
		t.Fatalf("first assignment: %v", err)
	}
	// A second teacher assigned to the same (cycle, group, subject) collides.
	te2, err := svc.CreateTeacher(ctx(), "Grace", "Hopper", "grace@example.com", "")
	if err != nil {
		t.Fatalf("second teacher: %v", err)
	}
	if _, err := svc.CreateTeachingAssignment(ctx(), grp.ID(), subj.ID(), te2.ID()); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("duplicate (cycle,group,subject): want KindConflict, got %v", err)
	}
}

func TestListTeachingAssignmentsFilters(t *testing.T) {
	svc, _ := newService(t)
	grp, subj, te := seedKnotParents(t, svc)
	// A second subject under the same grade level, so a second assignment shares
	// the cycle + group + teacher but differs by subject.
	subj2, err := svc.CreateSubject(ctx(), subj.GradeLevelID(), "Español")
	if err != nil {
		t.Fatalf("second subject: %v", err)
	}
	if _, err := svc.CreateTeachingAssignment(ctx(), grp.ID(), subj.ID(), te.ID()); err != nil {
		t.Fatalf("assignment 1: %v", err)
	}
	if _, err := svc.CreateTeachingAssignment(ctx(), grp.ID(), subj2.ID(), te.ID()); err != nil {
		t.Fatalf("assignment 2: %v", err)
	}

	all, err := svc.ListTeachingAssignments(ctx(), "", "", "")
	if err != nil || len(all) != 2 {
		t.Errorf("ListTeachingAssignments all: %d %v", len(all), err)
	}
	byCycle, err := svc.ListTeachingAssignments(ctx(), grp.CycleID(), "", "")
	if err != nil || len(byCycle) != 2 {
		t.Errorf("filter by cycle: %d %v", len(byCycle), err)
	}
	byTeacher, err := svc.ListTeachingAssignments(ctx(), "", "", te.ID())
	if err != nil || len(byTeacher) != 2 {
		t.Errorf("filter by teacher: %d %v", len(byTeacher), err)
	}
	none, err := svc.ListTeachingAssignments(ctx(), "", "other-group", "")
	if err != nil || len(none) != 0 {
		t.Errorf("filter by unknown group: %d %v", len(none), err)
	}
}
