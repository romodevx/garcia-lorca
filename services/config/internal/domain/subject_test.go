package domain_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func TestNewSubjectValidates(t *testing.T) {
	tests := []struct {
		name         string
		id           string
		tenantID     string
		gradeLevelID string
		subjName     string
		wantKind     glerrors.Kind
	}{
		{"empty id", "", "t1", "g1", "Matemáticas", glerrors.KindInvalid},
		{"empty tenant", "s1", "", "g1", "Matemáticas", glerrors.KindInvalid},
		{"empty grade level id", "s1", "t1", "", "Matemáticas", glerrors.KindInvalid},
		{"empty name", "s1", "t1", "g1", "  ", glerrors.KindInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewSubject(tt.id, tt.tenantID, tt.gradeLevelID, tt.subjName)
			if glerrors.KindOf(err) != tt.wantKind {
				t.Fatalf("want %v, got %v", tt.wantKind, glerrors.KindOf(err))
			}
		})
	}

	s, err := domain.NewSubject("s1", "t1", "g1", "Matemáticas")
	if err != nil {
		t.Fatalf("valid subject: %v", err)
	}
	if s.ID() != "s1" || s.TenantID() != "t1" || s.GradeLevelID() != "g1" || s.Name() != "Matemáticas" {
		t.Errorf("accessors: %q %q %q %q", s.ID(), s.TenantID(), s.GradeLevelID(), s.Name())
	}
}

func TestSubjectRenamedKeepsIdentityAndGrade(t *testing.T) {
	s, _ := domain.NewSubject("s1", "t1", "g1", "Matemáticas")

	got, err := s.Renamed("Matemáticas I")
	if err != nil {
		t.Fatalf("Renamed: %v", err)
	}
	if got.ID() != "s1" || got.TenantID() != "t1" || got.GradeLevelID() != "g1" || got.Name() != "Matemáticas I" {
		t.Errorf("renamed = %q %q %q %q", got.ID(), got.TenantID(), got.GradeLevelID(), got.Name())
	}
	if s.Name() != "Matemáticas" {
		t.Errorf("the original must not change, now %q", s.Name())
	}
	if _, err := s.Renamed("   "); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("a blank name must be KindInvalid, got %v", err)
	}
}

func TestSubjectNameIsStoredTrimmed(t *testing.T) {
	s, err := domain.NewSubject("s1", "t1", "g1", "  Lengua \t")
	if err != nil {
		t.Fatalf("NewSubject: %v", err)
	}
	if s.Name() != "Lengua" {
		t.Errorf("name = %q, want it trimmed", s.Name())
	}
	r, _ := s.Renamed(" Inglés ")
	if r.Name() != "Inglés" {
		t.Errorf("renamed = %q, want it trimmed", r.Name())
	}
}
