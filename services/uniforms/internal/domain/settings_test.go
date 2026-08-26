package domain_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/uniforms/internal/domain"
)

// TestDefaultSettingsAreUsable: a school that has never opened the settings
// screen still gets a working shop, and the ten working days the owner asked for.
func TestDefaultSettingsAreUsable(t *testing.T) {
	s := domain.DefaultSettings("t-1")
	if s.PickupWindowBusinessDays() != 10 {
		t.Errorf("pickup window = %d, want 10", s.PickupWindowBusinessDays())
	}
	if !s.AutoCancelEnabled() {
		t.Error("holds should lapse by default; that is the whole point of a hold")
	}
	if s.MaxOpenOrdersPerStudent() < 1 || s.MaxLinesPerOrder() < 1 || s.MaxUnitsPerLine() < 1 {
		t.Errorf("the ceilings must be usable out of the box: %+v", s)
	}
}

func TestSettingsValidation(t *testing.T) {
	cases := []struct {
		name  string
		build func() (domain.Settings, error)
	}{
		{"window below one", func() (domain.Settings, error) {
			return domain.NewSettings("t-1", 0, true, 2, 5, 20, 5, 2)
		}},
		{"window beyond a term", func() (domain.Settings, error) {
			return domain.NewSettings("t-1", 91, true, 2, 5, 20, 5, 2)
		}},
		{"no tenant", func() (domain.Settings, error) {
			return domain.NewSettings("", 10, true, 2, 5, 20, 5, 2)
		}},
		{"zero orders per student", func() (domain.Settings, error) {
			return domain.NewSettings("t-1", 10, true, 2, 0, 20, 5, 2)
		}},
		{"zero lines per order", func() (domain.Settings, error) {
			return domain.NewSettings("t-1", 10, true, 2, 5, 0, 5, 2)
		}},
		{"zero units per line", func() (domain.Settings, error) {
			return domain.NewSettings("t-1", 10, true, 2, 5, 20, 0, 2)
		}},
		{"negative extensions", func() (domain.Settings, error) {
			return domain.NewSettings("t-1", 10, true, -1, 5, 20, 5, 2)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.build(); glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("want KindInvalid, got %v", err)
			}
		})
	}
}

// TestSettingsCeilingsAreWhatStopsAFreeHold spells out what these numbers are
// for. Without them one family can reserve the last unit of every size for ten
// working days, let them all lapse, and start again — at no cost, forever.
func TestSettingsCeilingsAreWhatStopsAFreeHold(t *testing.T) {
	s := domain.DefaultSettings("t-1")

	if err := s.CheckOrderShape(s.MaxLinesPerOrder()+1, 1); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("too many lines: want KindInvalid, got %v", err)
	}
	if err := s.CheckOrderShape(1, s.MaxUnitsPerLine()+1); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("too many units on a line: want KindInvalid, got %v", err)
	}
	if err := s.CheckOrderShape(1, 1); err != nil {
		t.Errorf("an ordinary order was refused: %v", err)
	}
	if err := s.CheckOpenOrders(s.MaxOpenOrdersPerStudent()); glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("at the ceiling: want KindConflict, got %v", err)
	}
	if err := s.CheckOpenOrders(s.MaxOpenOrdersPerStudent() - 1); err != nil {
		t.Errorf("below the ceiling was refused: %v", err)
	}
}
