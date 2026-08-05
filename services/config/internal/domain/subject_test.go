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
