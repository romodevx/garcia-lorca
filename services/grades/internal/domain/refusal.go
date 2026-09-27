package domain

import (
	"errors"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// RefusalCode is the machine-readable name of a monthly-grade refusal. The
// sentence is for the person; the code is for the screen, which must react to
// a refusal (mark a cell, lock a row) without parsing Spanish.
type RefusalCode string

// The refusal codes the monthly grade surface answers with. The gateway
// forwards them as the X-Error-Code response header.
const (
	CodeGradeOutOfRange       RefusalCode = "GRADE_OUT_OF_RANGE"
	CodeGradeDecimals         RefusalCode = "GRADE_DECIMALS"
	CodeGradeNotNumber        RefusalCode = "GRADE_NOT_NUMBER"
	CodeMonthOutsideCycle     RefusalCode = "MONTH_OUTSIDE_CYCLE"
	CodeLevelNotGraded        RefusalCode = "LEVEL_NOT_GRADED"
	CodeEnrollmentNotInGroup  RefusalCode = "ENROLLMENT_NOT_IN_GROUP"
	CodeMonthPreviousGroup    RefusalCode = "MONTH_PREVIOUS_GROUP"
	CodeMonthInFuture         RefusalCode = "MONTH_IN_FUTURE"
	CodeLevelScaleUnavailable RefusalCode = "LEVEL_SCALE_NOT_CONFIGURED"
)

// RefusalError is a KindInvalid domain error that also names its RefusalCode.
// It unwraps to the *glerrors.Error, so KindOf and SafeMessage read it as any
// other typed error.
type RefusalError struct {
	Code RefusalCode
	err  *glerrors.Error
}

// Refusal builds a KindInvalid error with a sentence for the user and a code.
func Refusal(code RefusalCode, msg string) error {
	return &RefusalError{Code: code, err: glerrors.New(glerrors.KindInvalid, msg)}
}

// Error implements error.
func (e *RefusalError) Error() string { return e.err.Error() }

// Unwrap exposes the typed error for errors.As / KindOf / SafeMessage.
func (e *RefusalError) Unwrap() error { return e.err }

// RefusalCodeOf is err's refusal code, or "" when it carries none.
func RefusalCodeOf(err error) RefusalCode {
	var r *RefusalError
	if errors.As(err, &r) {
		return r.Code
	}
	return ""
}
