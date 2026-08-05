package domain_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func TestNewGroupValidates(t *testing.T) {
	tests := []struct {
		name         string
		id           string
		tenantID     string
		cycleID      string
		gradeLevelID string
		grpName      string
		capacity     int
		wantKind     glerrors.Kind
	}{
		{"empty id", "", "t1", "c1", "g1", "A", 30, glerrors.KindInvalid},
		{"empty tenant", "gr1", "", "c1", "g1", "A", 30, glerrors.KindInvalid},
		{"empty cycle id", "gr1", "t1", "", "g1", "A", 30, glerrors.KindInvalid},
		{"empty grade level id", "gr1", "t1", "c1", "", "A", 30, glerrors.KindInvalid},
		{"empty name", "gr1", "t1", "c1", "g1", "  ", 30, glerrors.KindInvalid},
		{"negative capacity", "gr1", "t1", "c1", "g1", "A", -1, glerrors.KindInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewGroup(tt.id, tt.tenantID, tt.cycleID, tt.gradeLevelID, tt.grpName, tt.capacity)
			if glerrors.KindOf(err) != tt.wantKind {
				t.Fatalf("want %v, got %v", tt.wantKind, glerrors.KindOf(err))
			}
		})
	}

	// capacity == 0 is valid (unbounded / not yet set).
	gr, err := domain.NewGroup("gr1", "t1", "c1", "g1", "A", 0)
	if err != nil {
		t.Fatalf("valid group with zero capacity: %v", err)
	}
	if gr.ID() != "gr1" || gr.TenantID() != "t1" || gr.CycleID() != "c1" || gr.GradeLevelID() != "g1" {
		t.Errorf("id accessors: %q %q %q %q", gr.ID(), gr.TenantID(), gr.CycleID(), gr.GradeLevelID())
	}
	if gr.Name() != "A" || gr.Capacity() != 0 {
		t.Errorf("value accessors: %q %d", gr.Name(), gr.Capacity())
	}
}
