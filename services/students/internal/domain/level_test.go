package domain_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

// A level is whatever the school defined in its catalog — Preescolar, Maternal,
// a level no fixed list ever had — as long as it points at one.
func TestNewLevelAcceptsAnyNamedCatalogLevel(t *testing.T) {
	for _, name := range []string{"Preescolar", "Maternal", "Primaria", "Bachillerato técnico"} {
		l, err := domain.NewLevel("lvl-1", "  "+name+" ")
		if err != nil {
			t.Fatalf("NewLevel(%q): %v", name, err)
		}
		if l.ID() != "lvl-1" || l.Name() != name {
			t.Errorf("NewLevel(%q) = (%q, %q)", name, l.ID(), l.Name())
		}
	}
}

func TestNewLevelRequiresAnIDAndAName(t *testing.T) {
	for _, c := range [][2]string{{"", "Primaria"}, {"lvl-1", " "}} {
		if _, err := domain.NewLevel(c[0], c[1]); glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Errorf("NewLevel(%q, %q): want KindInvalid, got %v", c[0], c[1], err)
		}
	}
}

// A pupil stored before the catalog went free may have a name and no id (its
// old key matched no config level). It is restored as it is, never re-judged.
func TestRestoreLevelKeepsALegacyUnlinkedLevel(t *testing.T) {
	l := domain.RestoreLevel("", "Maternal")
	if l.ID() != "" || l.Name() != "Maternal" {
		t.Errorf("RestoreLevel = (%q, %q)", l.ID(), l.Name())
	}
}
