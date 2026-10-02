package usecase_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

func TestRenameSubject(t *testing.T) {
	svc, repo := newService(t)
	l := seedLevel(t, svc)
	g := seedGradeLevel(t, svc, l.ID())
	s, err := svc.CreateSubject(ctx(), g.ID(), "Matematicas")
	if err != nil {
		t.Fatalf("CreateSubject: %v", err)
	}

	got, err := svc.RenameSubject(ctx(), s.ID(), "Matemáticas")
	if err != nil {
		t.Fatalf("RenameSubject: %v", err)
	}
	if got.ID() != s.ID() || got.GradeLevelID() != g.ID() || got.Name() != "Matemáticas" {
		t.Errorf("renamed = %+v", got)
	}
	if repo.subjects[s.ID()].Name() != "Matemáticas" {
		t.Errorf("the rename must be persisted, have %q", repo.subjects[s.ID()].Name())
	}
}

func TestRenameSubjectRefusals(t *testing.T) {
	svc, repo := newService(t)
	l := seedLevel(t, svc)
	g := seedGradeLevel(t, svc, l.ID())
	s, _ := svc.CreateSubject(ctx(), g.ID(), "Matemáticas")

	if _, err := svc.RenameSubject(ctx(), s.ID(), "  "); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("blank name: want KindInvalid, got %v", err)
	}
	if _, err := svc.RenameSubject(ctx(), "nope", "Español"); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("unknown subject: want KindNotFound, got %v", err)
	}
	if repo.renames != 0 {
		t.Errorf("a refused rename must not reach the repository, %d writes", repo.renames)
	}
}

func TestDeleteSubject(t *testing.T) {
	svc, repo := newService(t)
	l := seedLevel(t, svc)
	g := seedGradeLevel(t, svc, l.ID())
	s, _ := svc.CreateSubject(ctx(), g.ID(), "Artes")

	if err := svc.DeleteSubject(ctx(), s.ID()); err != nil {
		t.Fatalf("DeleteSubject: %v", err)
	}
	if _, ok := repo.subjects[s.ID()]; ok {
		t.Error("the subject must be gone")
	}
	if err := svc.DeleteSubject(ctx(), s.ID()); glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Errorf("second delete: want KindNotFound, got %v", err)
	}
}

func TestDeleteAnAssignedSubjectIsConflict(t *testing.T) {
	svc, repo := newService(t)
	grp, subj, te := seedKnotParents(t, svc)
	if _, err := svc.CreateTeachingAssignment(ctx(), grp.ID(), subj.ID(), te.ID()); err != nil {
		t.Fatalf("CreateTeachingAssignment: %v", err)
	}

	if err := svc.DeleteSubject(ctx(), subj.ID()); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("assigned subject: want KindConflict, got %v", err)
	}
	if _, ok := repo.subjects[subj.ID()]; !ok {
		t.Error("a refused delete must keep the subject")
	}
}
