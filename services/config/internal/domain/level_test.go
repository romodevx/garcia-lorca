package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func TestNewLevelValidates(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		tenantID string
		lvlName  string
		sepCode  string
		gran     domain.EnrollmentGranularity
		wantKind glerrors.Kind
	}{
		{"empty id", "", "t1", "Primaria", "PRIM", domain.GranularityCycle, glerrors.KindInvalid},
		{"empty tenant", "l1", "", "Primaria", "PRIM", domain.GranularityCycle, glerrors.KindInvalid},
		{"empty name", "l1", "t1", "  ", "PRIM", domain.GranularityCycle, glerrors.KindInvalid},
		{"unknown granularity", "l1", "t1", "Primaria", "PRIM", domain.EnrollmentGranularity(99), glerrors.KindInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewLevel(tt.id, tt.tenantID, tt.lvlName, tt.sepCode, tt.gran)
			if glerrors.KindOf(err) != tt.wantKind {
				t.Fatalf("want %v, got %v", tt.wantKind, glerrors.KindOf(err))
			}
		})
	}

	l, err := domain.NewLevel("l1", "t1", "Primaria", "PRIM", domain.GranularityTerm)
	if err != nil {
		t.Fatalf("valid level: %v", err)
	}
	if l.ID() != "l1" || l.TenantID() != "t1" || l.Name() != "Primaria" || l.SEPCode() != "PRIM" {
		t.Errorf("accessors: %q %q %q %q", l.ID(), l.TenantID(), l.Name(), l.SEPCode())
	}
	if l.Granularity() != domain.GranularityTerm {
		t.Errorf("granularity accessor: %v", l.Granularity())
	}
}

func TestParseEnrollmentGranularity(t *testing.T) {
	tests := []struct {
		raw  string
		want domain.EnrollmentGranularity
	}{
		{"cycle", domain.GranularityCycle},
		{"TERM", domain.GranularityTerm},
		{" cycle ", domain.GranularityCycle},
	}
	for _, tt := range tests {
		got, err := domain.ParseEnrollmentGranularity(tt.raw)
		if err != nil {
			t.Fatalf("ParseEnrollmentGranularity(%q): %v", tt.raw, err)
		}
		if got != tt.want {
			t.Errorf("ParseEnrollmentGranularity(%q) = %v, want %v", tt.raw, got, tt.want)
		}
		if got.String() != tt.want.String() {
			t.Errorf("String() round-trip mismatch for %q", tt.raw)
		}
	}

	if _, err := domain.ParseEnrollmentGranularity("bogus"); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("unknown granularity must be Invalid, got %v", glerrors.KindOf(err))
	}
}

func TestLevelDeletingIsCarriedNotChosen(t *testing.T) {
	l, err := domain.NewLevel("l1", "t1", "Primaria", "", domain.GranularityCycle)
	require.NoError(t, err)
	assert.False(t, l.Deleting(), "a new level is active")
	d := l.WithDeleting(true)
	assert.True(t, d.Deleting())
	assert.False(t, l.Deleting(), "levels are values")
}
