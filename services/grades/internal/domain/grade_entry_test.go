package domain_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/domain"
)

func validGradeEntry(t *testing.T, value float64) domain.GradeEntry {
	t.Helper()
	e, err := domain.NewGradeEntry("ge-1", "t1", "enr-1", "subj-1", "term-1", "teacher-1",
		value, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewGradeEntry(%v): %v", value, err)
	}
	return e
}

func TestNewGradeEntryAccessors(t *testing.T) {
	at := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	e, err := domain.NewGradeEntry("ge-1", "t1", "enr-1", "subj-1", "term-1", "teacher-1", 8.5, at)
	if err != nil {
		t.Fatalf("NewGradeEntry: %v", err)
	}
	if e.ID() != "ge-1" || e.TenantID() != "t1" || e.EnrollmentID() != "enr-1" ||
		e.SubjectID() != "subj-1" || e.TermID() != "term-1" || e.TeacherID() != "teacher-1" ||
		e.Value() != 8.5 || !e.CapturedAt().Equal(at) {
		t.Errorf("accessors mismatch: %+v", e)
	}
}

func TestNewGradeEntryValueBoundaries(t *testing.T) {
	// 0 and 10 are inclusive bounds and must be accepted.
	if got := validGradeEntry(t, 0).Value(); got != 0 {
		t.Errorf("value 0 accepted, got %v", got)
	}
	if got := validGradeEntry(t, 10).Value(); got != 10 {
		t.Errorf("value 10 accepted, got %v", got)
	}
}

func TestNewGradeEntryValueOutOfRange(t *testing.T) {
	at := time.Now()
	for _, value := range []float64{-0.1, 10.01} {
		_, err := domain.NewGradeEntry("ge-1", "t1", "enr-1", "subj-1", "term-1", "teacher-1", value, at)
		if glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Errorf("value %v: want KindInvalid, got %v", value, err)
		}
	}
}

func TestNewGradeEntryValidation(t *testing.T) {
	at := time.Now()
	cases := []struct {
		name                                                     string
		id, tenantID, enrollmentID, subjectID, termID, teacherID string
	}{
		{"empty id", "", "t1", "enr-1", "subj-1", "term-1", "teacher-1"},
		{"empty tenant", "ge-1", "", "enr-1", "subj-1", "term-1", "teacher-1"},
		{"empty enrollment", "ge-1", "t1", "", "subj-1", "term-1", "teacher-1"},
		{"empty subject", "ge-1", "t1", "enr-1", "", "term-1", "teacher-1"},
		{"empty term", "ge-1", "t1", "enr-1", "subj-1", "", "teacher-1"},
		{"empty teacher", "ge-1", "t1", "enr-1", "subj-1", "term-1", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewGradeEntry(tc.id, tc.tenantID, tc.enrollmentID, tc.subjectID, tc.termID, tc.teacherID, 5, at)
			if glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("want KindInvalid, got %v", err)
			}
		})
	}
}

func TestGradeEntryWithValue(t *testing.T) {
	e := validGradeEntry(t, 5)
	updated, err := e.WithValue(9.25)
	if err != nil {
		t.Fatalf("WithValue: %v", err)
	}
	if updated.Value() != 9.25 {
		t.Errorf("WithValue did not update: %v", updated.Value())
	}
	// Identity and the rest are preserved; the source is immutable.
	if updated.ID() != e.ID() || updated.SubjectID() != e.SubjectID() ||
		updated.TeacherID() != e.TeacherID() || e.Value() != 5 {
		t.Errorf("WithValue mutated identity or source: %+v (src %+v)", updated, e)
	}
	if _, err := e.WithValue(10.01); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("WithValue out-of-range: want KindInvalid, got %v", err)
	}
}
