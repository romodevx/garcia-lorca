package domain

import (
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// GradingScale is the numeric scale of a level (owned by config, T6), held in
// exact hundredths (5.00 = 500). It judges the text a teacher typed with exact
// decimal arithmetic: a float64 cannot tell 9.95 from 9.949999…, so no float
// ever takes part in deciding whether a grade is in range or has too many
// decimals.
type GradingScale struct {
	min      int
	max      int
	passing  int
	decimals int
}

// Hard bounds of any scale: the column stores hundredths up to 100.00.
const (
	hundredthsPerUnit = 100
	scaleCeiling      = 100 * hundredthsPerUnit
	maxScaleDecimals  = 2
	decimalBase       = 10
)

// NewGradingScale builds a validated scale: 0 <= min < passing <= max <= 100.00
// and 0 <= decimals <= 2 (the same invariant config enforces).
func NewGradingScale(minHundredths, maxHundredths, passingHundredths, decimals int) (GradingScale, error) {
	if minHundredths < 0 || maxHundredths > scaleCeiling ||
		minHundredths >= passingHundredths || passingHundredths > maxHundredths ||
		decimals < 0 || decimals > maxScaleDecimals {
		return GradingScale{}, glerrors.New(glerrors.KindInvalid, "grading scale is inconsistent")
	}
	return GradingScale{min: minHundredths, max: maxHundredths, passing: passingHundredths, decimals: decimals}, nil
}

// MinHundredths is the lowest mark, in hundredths.
func (s GradingScale) MinHundredths() int { return s.min }

// MaxHundredths is the highest mark, in hundredths.
func (s GradingScale) MaxHundredths() int { return s.max }

// PassingHundredths is the lowest passing mark, in hundredths.
func (s GradingScale) PassingHundredths() int { return s.passing }

// Decimals is how many decimal places a mark may carry.
func (s GradingScale) Decimals() int { return s.decimals }

// ParseValue turns the decimal text a teacher typed into a GradeValue on this
// scale. The text is read as an exact rational (big.Rat), then checked against
// the range first and the allowed decimals second, so 10.05 on a 5–10 scale
// reads "out of range" and 9.95 with one decimal reads "too many decimals".
// Each refusal is KindInvalid with a sentence for the user.
func (s GradingScale) ParseValue(text string) (GradeValue, error) {
	r, ok := parseDecimal(text)
	if !ok {
		return GradeValue{}, Refusal(CodeGradeNotNumber, "La calificación debe ser un número")
	}
	if r.Cmp(ratHundredths(s.min)) < 0 || r.Cmp(ratHundredths(s.max)) > 0 {
		return GradeValue{}, Refusal(CodeGradeOutOfRange, fmt.Sprintf(
			"La calificación debe estar entre %s y %s", formatHundredths(s.min), formatHundredths(s.max)))
	}
	if !hasAtMostDecimals(r, s.decimals) {
		return GradeValue{}, Refusal(CodeGradeDecimals, decimalsMessage(s.decimals))
	}
	scaled := new(big.Rat).Mul(r, big.NewRat(hundredthsPerUnit, 1))
	return GradeValue{hundredths: int(scaled.Num().Int64())}, nil
}

// Admits reports whether a stored value fits this scale: inside the range and
// with no more decimals than it allows — what ParseValue would have accepted.
func (s GradingScale) Admits(v GradeValue) bool {
	h := v.hundredths
	step := stepFor(s.decimals)
	return h >= s.min && h <= s.max && h%step == 0
}

// stepFor is the hundredths granularity a number of decimals allows (0 → 100,
// 1 → 10, 2 → 1).
func stepFor(decimals int) int {
	step := hundredthsPerUnit
	for i := 0; i < decimals; i++ {
		step /= decimalBase
	}
	return step
}

// maxDecimalText caps the typed text: no grade needs more than 16 characters,
// and a bound keeps big.Rat from ever doing unbounded work.
const maxDecimalText = 16

// plainDecimal is the only shape a grade may be typed in: optional minus,
// digits, optional fraction. Exponent ("1e9"), hex ("0x1p3"), binary ("0b1")
// and fraction ("1/2") forms are refused BEFORE big.Rat sees them — SetString
// would accept "1e1000000000" and build a billion-digit number.
var plainDecimal = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

// parseDecimal reads a plain decimal exactly.
func parseDecimal(text string) (*big.Rat, bool) {
	text = strings.TrimSpace(text)
	if len(text) > maxDecimalText || !plainDecimal.MatchString(text) {
		return nil, false
	}
	return new(big.Rat).SetString(text)
}

// hasAtMostDecimals reports whether r * 10^decimals is a whole number.
func hasAtMostDecimals(r *big.Rat, decimals int) bool {
	factor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	return new(big.Rat).Mul(r, new(big.Rat).SetInt(factor)).IsInt()
}

func ratHundredths(h int) *big.Rat { return big.NewRat(int64(h), hundredthsPerUnit) }

func decimalsMessage(decimals int) string {
	switch decimals {
	case 0:
		return "Solo se permiten calificaciones enteras"
	case 1:
		return "Solo se permite 1 decimal"
	default:
		return fmt.Sprintf("Solo se permiten %d decimales", decimals)
	}
}

// GradeValue is a validated mark in exact hundredths.
type GradeValue struct {
	hundredths int
}

// GradeValueFromHundredths rebuilds a stored mark (already validated on write).
func GradeValueFromHundredths(h int) GradeValue { return GradeValue{hundredths: h} }

// Hundredths is the mark in hundredths (9.5 = 950).
func (v GradeValue) Hundredths() int { return v.hundredths }

// String is the canonical decimal text, trailing zeros dropped ("9.5", "10").
func (v GradeValue) String() string { return formatHundredths(v.hundredths) }

// formatHundredths prints hundredths as the shortest exact decimal.
func formatHundredths(h int) string {
	sign := ""
	if h < 0 {
		sign, h = "-", -h
	}
	whole, frac := h/hundredthsPerUnit, h%hundredthsPerUnit
	if frac == 0 {
		return sign + strconv.Itoa(whole)
	}
	return sign + strings.TrimRight(fmt.Sprintf("%d.%02d", whole, frac), "0")
}
