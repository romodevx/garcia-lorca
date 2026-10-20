package domain

import glerrors "github.com/garcia-lorca/pkg/errors"

// GradingScale is how a level grades numerically (T6). Marks are held as exact
// hundredths (5.00 = 500) so no float ever sits on a grade boundary. A disabled
// scale means the level takes no numeric capture (early years); its numbers are
// kept so re-enabling restores the school's configuration.
//
// A level that does not grade with numbers may grade with letters instead
// (A Excelente, B Bien, C Suficiente, D Requiere apoyo — preescolar): letters
// says so. Numbers and letters never hold at once.
//
// Invariant, enforced by NewGradingScale and mirrored by the levels CHECKs:
// 0 <= min < passing <= max <= 100.00 and 0 <= decimals <= 2.
type GradingScale struct {
	enabled  bool
	letters  bool
	min      int
	max      int
	passing  int
	decimals int
}

// The school's default scale and the hard bounds any scale must respect.
// Grades are whole numbers (owner decision, T10 ISSUE-011): a new level starts
// «Sin decimales»; migrations/config/0021 gives the column the same default.
const (
	defaultGradeMin      = 500
	defaultGradeMax      = 1000
	defaultGradePassing  = 600
	defaultGradeDecimals = 0

	gradeCeiling     = 10000 // 100.00: the largest mark numeric(5,2) is allowed to hold
	maxGradeDecimals = 2     // the mark column stores hundredths
)

// NewGradingScale builds a validated scale from hundredths. A whole-number
// scale (decimals 0) also takes whole-number bounds: a passing mark of 6.5 is
// one no captured grade can ever equal (ISSUE-T14-005).
func NewGradingScale(enabled bool, minHundredths, maxHundredths, passingHundredths, decimals int) (GradingScale, error) {
	s, err := RestoreGradingScale(enabled, minHundredths, maxHundredths, passingHundredths, decimals)
	if err != nil {
		return GradingScale{}, err
	}
	if s.decimals == 0 && !s.wholeBounds() {
		return GradingScale{}, glerrors.New(glerrors.KindInvalid, "Sin decimales, la mínima, la máxima y la aprobatoria son números enteros")
	}
	return s, nil
}

// RestoreGradingScale rebuilds a stored scale. It skips the whole-number rule,
// which is newer than some rows: refusing one on read would hide its level.
func RestoreGradingScale(enabled bool, minHundredths, maxHundredths, passingHundredths, decimals int) (GradingScale, error) {
	s := GradingScale{enabled: enabled, min: minHundredths, max: maxHundredths, passing: passingHundredths, decimals: decimals}
	if err := s.validate(); err != nil {
		return GradingScale{}, err
	}
	return s, nil
}

const hundredthsPerMark = 100

func (s GradingScale) wholeBounds() bool {
	return s.min%hundredthsPerMark == 0 && s.max%hundredthsPerMark == 0 && s.passing%hundredthsPerMark == 0
}

func (s GradingScale) validate() error {
	if s.enabled && s.letters {
		return glerrors.New(glerrors.KindInvalid, "Un nivel califica con números o con letras, no con ambos")
	}
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

// earlyYearsLevelNames are the levels that take no numeric grades. Keys are
// folded names (see foldLevelName); migrations/config/0009 applies the same
// list to existing rows.
var earlyYearsLevelNames = map[string]bool{
	"preescolar": true,
	"kinder":     true,
	"maternal":   true,
}

// letterLevelNames are the early-years levels that grade with letters (owner,
// 20-oct): preescolar does, maternal takes no grades. migrations/config/0024
// applies the same list to existing rows.
var letterLevelNames = map[string]bool{
	"preescolar": true,
	"kinder":     true,
}

// DefaultGradingScale is the scale a level of this name starts with: 5 to 10,
// whole numbers, 6 to pass — disabled for the early-years levels, which grade
// with letters when they are preescolar.
func DefaultGradingScale(levelName string) GradingScale {
	folded := foldLevelName(levelName)
	return GradingScale{
		enabled:  !earlyYearsLevelNames[folded],
		letters:  letterLevelNames[folded],
		min:      defaultGradeMin,
		max:      defaultGradeMax,
		passing:  defaultGradePassing,
		decimals: defaultGradeDecimals,
	}
}

// Enabled reports whether the level captures numeric grades at all.
func (s GradingScale) Enabled() bool { return s.enabled }

// Letters reports whether the level grades with letters (A–D) instead.
func (s GradingScale) Letters() bool { return s.letters }

// WithLetters returns a copy grading with letters, or not. Letters on a scale
// that grades with numbers is KindInvalid; the receiver is never modified.
func (s GradingScale) WithLetters(on bool) (GradingScale, error) {
	s.letters = on
	if err := s.validate(); err != nil {
		return GradingScale{}, err
	}
	return s, nil
}

// MinHundredths is the lowest mark, in hundredths.
func (s GradingScale) MinHundredths() int { return s.min }

// MaxHundredths is the highest mark, in hundredths.
func (s GradingScale) MaxHundredths() int { return s.max }

// PassingHundredths is the lowest passing mark, in hundredths.
func (s GradingScale) PassingHundredths() int { return s.passing }

// Decimals is how many decimal places a captured mark may carry.
func (s GradingScale) Decimals() int { return s.decimals }
