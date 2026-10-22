// Package domain holds the rich domain model for the students bounded context:
// the Student entity, the admission state machine, and value objects (CURP,
// Level). It imports NO framework — not GORM, not Connect. Dependencies point
// inward toward it.
package domain

import (
	"regexp"
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// curpPattern validates the structure of a CURP (Clave Única de Registro de
// Población): 4 name letters, YYMMDD birthdate, sex (H/M), 2 state letters,
// 3 internal consonants, a homonym digit/letter, and a check digit. This is a
// shape check (not the official check-digit algorithm), enough as a boundary
// guard; the authoritative uniqueness constraint lives in the database.
var curpPattern = regexp.MustCompile(
	`^[A-Z][AEIOUX][A-Z]{2}` +
		`[0-9]{2}(0[1-9]|1[0-2])(0[1-9]|[12][0-9]|3[01])` +
		`[HM]` +
		`[A-Z]{2}[B-DF-HJ-NP-TV-Z]{3}[A-Z0-9][0-9]$`,
)

// ErrCURPTaken is another pupil of the school already holding the CURP.
var ErrCURPTaken = glerrors.New(glerrors.KindConflict, "a student with this CURP already exists")

// CURP is a normalized, validated Mexican national id value object, compared by
// value and unique per tenant.
type CURP string

// NewCURP trims, uppercases, and validates a CURP's structure.
func NewCURP(raw string) (CURP, error) {
	normalized := strings.ToUpper(strings.TrimSpace(raw))
	if len(normalized) != 18 || !curpPattern.MatchString(normalized) {
		return "", glerrors.New(glerrors.KindInvalid, "invalid CURP format")
	}
	return CURP(normalized), nil
}

// IsZero reports a pupil without a CURP: only a draft may be one.
func (c CURP) IsZero() bool { return c == "" }

// String returns the normalized CURP.
func (c CURP) String() string { return string(c) }
