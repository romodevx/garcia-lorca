package domain_test

import (
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

func TestParseLevel(t *testing.T) {
	tests := map[string]domain.Level{
		"kinder":       domain.LevelKinder,
		"Kínder":       domain.LevelKinder,
		"PRIMARIA":     domain.LevelPrimaria,
		"secundaria":   domain.LevelSecundaria,
		"preparatoria": domain.LevelPreparatoria,
	}
	for in, want := range tests {
		got, err := domain.ParseLevel(in)
		if err != nil {
			t.Errorf("ParseLevel(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseLevelRejectsUnknown(t *testing.T) {
	for _, in := range []string{"", "universidad", "primary"} {
		if _, err := domain.ParseLevel(in); err == nil || glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Errorf("ParseLevel(%q): want KindInvalid, got %v", in, err)
		}
	}
}

func TestLevelValidAndString(t *testing.T) {
	if domain.LevelUnspecified.Valid() {
		t.Error("unspecified level must be invalid")
	}
	if !domain.LevelPrimaria.Valid() {
		t.Error("primaria must be valid")
	}
	if domain.LevelPreparatoria.String() != "preparatoria" {
		t.Errorf("String() = %q", domain.LevelPreparatoria.String())
	}
}
