package domain

import (
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// AttendanceMode is how a level's groups take attendance (T3): one pass list per
// group per day, which any of the group's teachers may take (daily), or one per
// group, subject and day, taken only by that subject's teacher (per class).
type AttendanceMode uint8

const (
	// AttendanceModeUnspecified is the zero value; not a valid mode.
	AttendanceModeUnspecified AttendanceMode = iota
	// AttendanceDaily: one list per group per day.
	AttendanceDaily
	// AttendancePerClass: one list per group, subject and day.
	AttendancePerClass
)

var attendanceModeNames = map[AttendanceMode]string{
	AttendanceDaily:    "daily",
	AttendancePerClass: "per_class",
}

// String returns the canonical mode name, the one the levels table stores
// ("" for unspecified).
func (m AttendanceMode) String() string { return attendanceModeNames[m] }

// Valid reports whether the mode is a real value (not unspecified).
func (m AttendanceMode) Valid() bool {
	return m == AttendanceDaily || m == AttendancePerClass
}

// ParseAttendanceMode resolves a name (case-insensitive, trimmed) to a mode.
func ParseAttendanceMode(raw string) (AttendanceMode, error) {
	key := strings.ToLower(strings.TrimSpace(raw))
	for mode, name := range attendanceModeNames {
		if name == key {
			return mode, nil
		}
	}
	return AttendanceModeUnspecified, glerrors.New(glerrors.KindInvalid, "unknown attendance mode: "+raw)
}

// perClassLevelNames are the SEP levels taught by one teacher per subject, which
// therefore take attendance per class by default. Keys are folded names (see
// foldLevelName). migrations/config/0008 applies the same list to existing rows.
var perClassLevelNames = map[string]bool{
	"secundaria":   true,
	"preparatoria": true,
	"bachillerato": true,
}

var accentFolder = strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u")

// foldLevelName lowercases, trims and strips Spanish accents from a level name.
func foldLevelName(name string) string {
	return accentFolder.Replace(strings.ToLower(strings.TrimSpace(name)))
}

// DefaultAttendanceMode is the mode a level of this name starts with: per class
// for secundaria/preparatoria/bachillerato, daily for everything else.
func DefaultAttendanceMode(levelName string) AttendanceMode {
	if perClassLevelNames[foldLevelName(levelName)] {
		return AttendancePerClass
	}
	return AttendanceDaily
}
