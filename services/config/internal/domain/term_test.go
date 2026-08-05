package domain_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func TestNewTermValidates(t *testing.T) {
	starts := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	ends := time.Date(2026, time.December, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		id       string
		tenantID string
		cycleID  string
		levelID  string
		termName string
		ordinal  int
		starts   time.Time
		ends     time.Time
		wantKind glerrors.Kind
	}{
		{"empty id", "", "t1", "c1", "l1", "T1", 1, starts, ends, glerrors.KindInvalid},
		{"empty tenant", "tm1", "", "c1", "l1", "T1", 1, starts, ends, glerrors.KindInvalid},
		{"empty cycle id", "tm1", "t1", "", "l1", "T1", 1, starts, ends, glerrors.KindInvalid},
		{"empty level id", "tm1", "t1", "c1", "", "T1", 1, starts, ends, glerrors.KindInvalid},
		{"empty name", "tm1", "t1", "c1", "l1", "  ", 1, starts, ends, glerrors.KindInvalid},
		{"ordinal below one", "tm1", "t1", "c1", "l1", "T1", 0, starts, ends, glerrors.KindInvalid},
		{"ends before starts", "tm1", "t1", "c1", "l1", "T1", 1, ends, starts, glerrors.KindInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewTerm(tt.id, tt.tenantID, tt.cycleID, tt.levelID, tt.termName, tt.ordinal, tt.starts, tt.ends)
			if glerrors.KindOf(err) != tt.wantKind {
				t.Fatalf("want %v, got %v", tt.wantKind, glerrors.KindOf(err))
			}
		})
	}

	tm, err := domain.NewTerm("tm1", "t1", "c1", "l1", "T1", 1, starts, ends)
	if err != nil {
		t.Fatalf("valid term: %v", err)
	}
	if tm.ID() != "tm1" || tm.TenantID() != "t1" || tm.CycleID() != "c1" || tm.LevelID() != "l1" {
		t.Errorf("id accessors: %q %q %q %q", tm.ID(), tm.TenantID(), tm.CycleID(), tm.LevelID())
	}
	if tm.Name() != "T1" || tm.Ordinal() != 1 || !tm.StartsOn().Equal(starts) || !tm.EndsOn().Equal(ends) {
		t.Errorf("value accessors: %q %d %v %v", tm.Name(), tm.Ordinal(), tm.StartsOn(), tm.EndsOn())
	}
}
