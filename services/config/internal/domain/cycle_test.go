package domain_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func TestNewCycleValidates(t *testing.T) {
	starts := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	ends := time.Date(2027, time.July, 15, 0, 0, 0, 0, time.UTC)

	_, err := domain.NewCycle("", "t1", "2026-2027", domain.CycleStatusDraft, starts, ends)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("empty id should be Invalid, got %v", glerrors.KindOf(err))
	}

	_, err = domain.NewCycle("c1", "", "2026-2027", domain.CycleStatusDraft, starts, ends)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("empty tenant id should be Invalid, got %v", glerrors.KindOf(err))
	}

	_, err = domain.NewCycle("c1", "t1", "  ", domain.CycleStatusDraft, starts, ends)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("empty name should be Invalid, got %v", glerrors.KindOf(err))
	}

	c, err := domain.NewCycle("c1", "t1", "2026-2027", domain.CycleStatusDraft, starts, ends)
	if err != nil {
		t.Fatalf("valid cycle: %v", err)
	}
	if c.ID() != "c1" || c.TenantID() != "t1" {
		t.Errorf("id/tenant accessors: %q %q", c.ID(), c.TenantID())
	}
	if c.Name() != "2026-2027" || c.Status() != domain.CycleStatusDraft {
		t.Errorf("name/status accessors: %q %v", c.Name(), c.Status())
	}
	if !c.StartsOn().Equal(starts) || !c.EndsOn().Equal(ends) {
		t.Errorf("date accessors: %v %v", c.StartsOn(), c.EndsOn())
	}

	_, err = domain.NewCycle("c1", "t1", "2026-2027", domain.CycleStatusDraft, ends, starts)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("ends_on before starts_on must be Invalid, got %v", glerrors.KindOf(err))
	}

	// An unspecified (non-Valid) status must be rejected: the constructor is the
	// last guard before the DB CHECK, so it closes the gap rather than letting an
	// opaque 23514 surface. The use case defaults Unspecified to Draft; a direct
	// NewCycle with Unspecified must still fail cleanly.
	_, err = domain.NewCycle("c1", "t1", "2026-2027", domain.CycleStatusUnspecified, starts, ends)
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("unspecified status must be Invalid, got %v", glerrors.KindOf(err))
	}

	// Every real status still constructs.
	for _, status := range []domain.CycleStatus{domain.CycleStatusDraft, domain.CycleStatusActive, domain.CycleStatusClosed} {
		if _, err := domain.NewCycle("c1", "t1", "2026-2027", status, starts, ends); err != nil {
			t.Errorf("valid status %v: %v", status, err)
		}
	}
}

func TestParseCycleStatus(t *testing.T) {
	tests := []struct {
		raw  string
		want domain.CycleStatus
	}{
		{"draft", domain.CycleStatusDraft},
		{"ACTIVE", domain.CycleStatusActive},
		{" closed ", domain.CycleStatusClosed},
	}
	for _, tt := range tests {
		got, err := domain.ParseCycleStatus(tt.raw)
		if err != nil {
			t.Fatalf("ParseCycleStatus(%q): %v", tt.raw, err)
		}
		if got != tt.want {
			t.Errorf("ParseCycleStatus(%q) = %v, want %v", tt.raw, got, tt.want)
		}
		if got.String() != tt.want.String() {
			t.Errorf("String() round-trip mismatch for %q", tt.raw)
		}
	}

	if _, err := domain.ParseCycleStatus("bogus"); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("unknown status must be Invalid, got %v", glerrors.KindOf(err))
	}
}
