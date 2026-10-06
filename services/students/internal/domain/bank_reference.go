package domain

import (
	"strings"
	"unicode"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// maxBankReferenceDigits is the longest reference BBVA issues the school.
const maxBankReferenceDigits = 20

// BankReference is the pupil's BBVA deposit reference ("Matrícula Interna" in
// the previous system, ADR 0033 / O1): 1–20 digits, unique per school. The
// zero value means the pupil has none. It is NOT the matrícula, which the
// system gives on its own.
type BankReference string

// NewBankReference strips every space and validates the digits. Blank is a
// pupil without a reference; anything that is not 1–20 digits ("S/R", a
// dash) is KindInvalid.
func NewBankReference(raw string) (BankReference, error) {
	compact := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, raw)
	if compact == "" {
		return "", nil
	}
	if len(compact) > maxBankReferenceDigits || onlyDigits(compact) != compact {
		return "", glerrors.New(glerrors.KindInvalid, "the bank reference has one to twenty digits")
	}
	return BankReference(compact), nil
}

// IsZero reports whether the pupil has no reference.
func (b BankReference) IsZero() bool { return b == "" }

// String returns the digits ("" when none).
func (b BankReference) String() string { return string(b) }
