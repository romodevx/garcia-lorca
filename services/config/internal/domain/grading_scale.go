package domain

import glerrors "github.com/garcia-lorca/pkg/errors"

// GradingScale is how a level grades numerically (T6). Marks are held as exact
// hundredths (5.00 = 500) so no float ever sits on a grade boundary. A disabled
// scale means the level takes no numeric capture (early years); its numbers are
// kept so re-enabling restores the school's configuration.
//
// Invariant, enforced by NewGradingScale and mirrored by the levels CHECKs:
// 0 <= min < passing <= max <= 100.00 and 0 <= decimals <= 2.
type GradingScale struct {
	enabled  bool
	min      int
	max      int
	passing  int
	decimals int
}

// The school's default scale and the hard bounds any scale must respect.
const (
	defaultGradeMin      = 500
	defaultGradeMax      = 1000
	defaultGradePassing  = 600
	defaultGradeDecimals = 1

	gradeCeiling     = 10000 // 100.00: the largest mark numeric(5,2) is allowed to hold
	maxGradeDecimals = 2     // the mark column stores hundredths
)

// NewGradingScale builds a validated scale from hundredths.
func NewGradingScale(enabled bool, minHundredths, maxHundredths, passingHundredths, decimals int) (GradingScale, error) {
	s := GradingScale{enabled: enabled, min: minHundredths, max: maxHundredths, passing: passingHundredths, decimals: decimals}
	if err := s.validate(); err != nil {
		return GradingScale{}, err
	}
	return s, nil
}

func (s GradingScale) validate() error {
	if s.min < 0 || s.max > gradeCeiling {
		return glerrors.New(glerrors.KindInvalid, "La escala debe estar entre 0 y 100")
	}
	if s.min >= s.passing {
		return glerrors.New(glerrors.KindInvalid, "La calificación mínima debe ser menor que la aprobatoria")
	}
	if s.passing > s.max {
		return glerrors.New(glerrors.KindInvalid, "La calificación aprobatoria no puede ser mayor que la máxima")
	}
	if s.decimals < 0 || s.decimals > maxGradeDecimals {
		return glerrors.New(glerrors.KindInvalid, "Los decimales deben ser 0, 1 o 2")
	}
	return nil
}

// earlyYearsLevelNames are the levels that take no numeric grades (qualitative
// assessment comes later). Keys are folded names (see foldLevelName);
// migrations/config/0009 applies the same list to existing rows.
var earlyYearsLevelNames = map[string]bool{
	"preescolar": true,
	"kinder":     true,
	"maternal":   true,
}

// DefaultGradingScale is the scale a level of this name starts with: 5 to 10,
// one decimal, 6 to pass — disabled for the early-years levels.
func DefaultGradingScale(levelName string) GradingScale {
	return GradingScale{
		enabled:  !earlyYearsLevelNames[foldLevelName(levelName)],
		min:      defaultGradeMin,
		max:      defaultGradeMax,
		passing:  defaultGradePassing,
		decimals: defaultGradeDecimals,
	}
}

// Enabled reports whether the level captures numeric grades at all.
func (s GradingScale) Enabled() bool { return s.enabled }

// MinHundredths is the lowest mark, in hundredths.
func (s GradingScale) MinHundredths() int { return s.min }

// MaxHundredths is the highest mark, in hundredths.
func (s GradingScale) MaxHundredths() int { return s.max }

// PassingHundredths is the lowest passing mark, in hundredths.
func (s GradingScale) PassingHundredths() int { return s.passing }

// Decimals is how many decimal places a captured mark may carry.
func (s GradingScale) Decimals() int { return s.decimals }
