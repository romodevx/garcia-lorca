package domain_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func TestNewGradeLevelValidates(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		tenantID string
		levelID  string
		glName   string
		ordinal  int
		wantKind glerrors.Kind
	}{
		{"empty id", "", "t1", "l1", "1° Primaria", 1, glerrors.KindInvalid},
		{"empty tenant", "g1", "", "l1", "1° Primaria", 1, glerrors.KindInvalid},
		{"empty level id", "g1", "t1", "", "1° Primaria", 1, glerrors.KindInvalid},
		{"empty name", "g1", "t1", "l1", "  ", 1, glerrors.KindInvalid},
		{"ordinal below one", "g1", "t1", "l1", "1° Primaria", 0, glerrors.KindInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewGradeLevel(tt.id, tt.tenantID, tt.levelID, tt.glName, tt.ordinal)
			if glerrors.KindOf(err) != tt.wantKind {
				t.Fatalf("want %v, got %v", tt.wantKind, glerrors.KindOf(err))
			}
		})
	}

	g, err := domain.NewGradeLevel("g1", "t1", "l1", "1° Primaria", 1)
	if err != nil {
		t.Fatalf("valid grade level: %v", err)
	}
	if g.ID() != "g1" || g.TenantID() != "t1" || g.LevelID() != "l1" || g.Name() != "1° Primaria" || g.Ordinal() != 1 {
		t.Errorf("accessors: %q %q %q %q %d", g.ID(), g.TenantID(), g.LevelID(), g.Name(), g.Ordinal())
	}
}
