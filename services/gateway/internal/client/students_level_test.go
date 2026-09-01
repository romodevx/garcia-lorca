package client_test

import (
	"testing"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/services/gateway/internal/client"
)

// TestLevelNamesAreReadAsTolerantlyAsTheServiceReadsThem closes a gap between two
// places that decide the same thing.
//
// The students DOMAIN folds case and accents — it maps "Kínder" to kinder on
// purpose, because that is how a school writes it. The edge did an exact match
// against lowercase names, so it turned "Kínder" into UNSPECIFIED and answered
// 400 for a level the service two hops away would have accepted. The panel had
// no way to know: it was sending the school's own level name.
//
// Two layers parsing the same value, one more strictly than the other, is the
// shape of the bug. The edge is now as tolerant as the service.
func TestLevelNamesAreReadAsTolerantlyAsTheServiceReadsThem(t *testing.T) {
	cases := map[string]studentsv1.Level{
		"kinder":       studentsv1.Level_LEVEL_KINDER,
		"Kínder":       studentsv1.Level_LEVEL_KINDER,
		"KÍNDER":       studentsv1.Level_LEVEL_KINDER,
		"  Primaria ":  studentsv1.Level_LEVEL_PRIMARIA,
		"secundaria":   studentsv1.Level_LEVEL_SECUNDARIA,
		"Preparatoria": studentsv1.Level_LEVEL_PREPARATORIA,
		// Maternal is a real level this school teaches. It used to map to
		// UNSPECIFIED with a comment calling it "a level a school invented",
		// which meant the school could not enrol its youngest pupils at all.
		"Maternal": studentsv1.Level_LEVEL_MATERNAL,
		"maternal": studentsv1.Level_LEVEL_MATERNAL,
		"":         studentsv1.Level_LEVEL_UNSPECIFIED,
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			if got := client.LevelFromName(name); got != want {
				t.Errorf("LevelFromName(%q) = %v, want %v", name, got, want)
			}
		})
	}
}
