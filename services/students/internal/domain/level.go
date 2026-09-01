package domain

import (
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Level is the SEP education level. It is a domain type (no proto import); the
// adapter maps it to/from the wire enum.
type Level uint8

const (
	// LevelUnspecified is the zero value; not a valid level.
	LevelUnspecified Level = iota
	LevelKinder
	LevelPrimaria
	LevelSecundaria
	LevelPreparatoria
	// LevelMaternal is nursery, the level below kinder. Appended rather than
	// placed first because the wire enum's numbers are its format; pedagogical
	// order is a display concern.
	LevelMaternal
)

var levelNames = map[Level]string{
	LevelKinder:       "kinder",
	LevelPrimaria:     "primaria",
	LevelSecundaria:   "secundaria",
	LevelPreparatoria: "preparatoria",
	LevelMaternal:     "maternal",
}

// parseAliases maps normalized (folded) input to a Level, tolerating accents.
var parseAliases = map[string]Level{
	"kinder":       LevelKinder,
	"primaria":     LevelPrimaria,
	"secundaria":   LevelSecundaria,
	"preparatoria": LevelPreparatoria,
	"maternal":     LevelMaternal,
}

// ParseLevel resolves a level name (case/accent-insensitive) to a Level.
func ParseLevel(raw string) (Level, error) {
	key := strings.ToLower(strings.TrimSpace(raw))
	key = strings.ReplaceAll(key, "í", "i") // "kínder" → "kinder"
	if level, ok := parseAliases[key]; ok {
		return level, nil
	}
	return LevelUnspecified, glerrors.New(glerrors.KindInvalid, "unknown level: "+raw)
}

// Valid reports whether the level is a real SEP level (not unspecified).
func (l Level) Valid() bool { return l >= LevelKinder && l <= LevelPreparatoria }

// String returns the canonical level name ("" for unspecified).
func (l Level) String() string { return levelNames[l] }
