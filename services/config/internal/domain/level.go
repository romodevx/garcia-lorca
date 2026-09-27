package domain

import (
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// EnrollmentGranularity is the hybrid-model dial: whether students enroll into a
// level once per cycle or once per term. It is a per-level property.
type EnrollmentGranularity uint8

const (
	// GranularityUnspecified is the zero value; not a valid granularity.
	GranularityUnspecified EnrollmentGranularity = iota
	// GranularityCycle: a single enrollment covers the whole cycle.
	GranularityCycle
	// GranularityTerm: enrollment happens per term.
	GranularityTerm
)

var granularityNames = map[EnrollmentGranularity]string{
	GranularityCycle: "cycle",
	GranularityTerm:  "term",
}

// String returns the canonical granularity name ("" for unspecified).
func (g EnrollmentGranularity) String() string { return granularityNames[g] }

// Valid reports whether the granularity is a real value (not unspecified).
func (g EnrollmentGranularity) Valid() bool {
	return g == GranularityCycle || g == GranularityTerm
}

// ParseEnrollmentGranularity resolves a name (case-insensitive) to a value.
func ParseEnrollmentGranularity(raw string) (EnrollmentGranularity, error) {
	key := strings.ToLower(strings.TrimSpace(raw))
	for gran, name := range granularityNames {
		if name == key {
			return gran, nil
		}
	}
	return GranularityUnspecified, glerrors.New(glerrors.KindInvalid, "unknown enrollment granularity: "+raw)
}

// Level is a SEP education level in the catalog (e.g. Primaria). It carries the
// per-level enrollment granularity that drives the hybrid cycle model.
type Level struct {
	id          string
	tenantID    string
	name        string
	sepCode     string
	granularity EnrollmentGranularity
	attendance  AttendanceMode
	grading     GradingScale
}

// NewLevel constructs a level. `id`, `tenantID`, and `name` are required, and
// `gran` must be a known granularity. The attendance mode starts at the default
// for the level's name (DefaultAttendanceMode); WithAttendanceMode changes it.
// Likewise the grading scale starts at DefaultGradingScale; WithGrading changes it.
func NewLevel(id, tenantID, name, sepCode string, gran EnrollmentGranularity) (Level, error) {
	if strings.TrimSpace(id) == "" {
		return Level{}, glerrors.New(glerrors.KindInvalid, "level id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return Level{}, glerrors.New(glerrors.KindInvalid, "level tenant id is required")
	}
	if strings.TrimSpace(name) == "" {
		return Level{}, glerrors.New(glerrors.KindInvalid, "level name is required")
	}
	if !gran.Valid() {
		return Level{}, glerrors.New(glerrors.KindInvalid, "level enrollment granularity is invalid")
	}
	return Level{
		id:          id,
		tenantID:    tenantID,
		name:        name,
		sepCode:     sepCode,
		granularity: gran,
		attendance:  DefaultAttendanceMode(name),
		grading:     DefaultGradingScale(name),
	}, nil
}

// ID returns the stable level identifier.
func (l Level) ID() string { return l.id }

// TenantID returns the tenant the level belongs to.
func (l Level) TenantID() string { return l.tenantID }

// Name returns the human-readable level name (e.g. "Primaria").
func (l Level) Name() string { return l.name }

// SEPCode returns the SEP catalog code for the level.
func (l Level) SEPCode() string { return l.sepCode }

// Granularity returns the per-level enrollment granularity.
func (l Level) Granularity() EnrollmentGranularity { return l.granularity }

// AttendanceMode returns how the level's groups take attendance.
func (l Level) AttendanceMode() AttendanceMode { return l.attendance }

// WithAttendanceMode returns a copy of the level taking attendance in mode. An
// unknown mode is KindInvalid; the receiver is never modified.
func (l Level) WithAttendanceMode(mode AttendanceMode) (Level, error) {
	if !mode.Valid() {
		return Level{}, glerrors.New(glerrors.KindInvalid, "level attendance mode is invalid")
	}
	l.attendance = mode
	return l, nil
}

// Grading returns the level's numeric grading scale.
func (l Level) Grading() GradingScale { return l.grading }

// WithGrading returns a copy of the level grading on scale. A scale that breaks
// the invariant (including the zero value) is KindInvalid; the receiver is never
// modified.
func (l Level) WithGrading(scale GradingScale) (Level, error) {
	if err := scale.validate(); err != nil {
		return Level{}, err
	}
	l.grading = scale
	return l, nil
}
