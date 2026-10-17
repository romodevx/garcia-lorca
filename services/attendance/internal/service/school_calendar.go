package service

import (
	"context"
	"errors"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// CodeNoClasses is the machine code of a save on a day without classes (T14):
// a weekend, a date outside the group's cycle, or a closed day of its calendar.
const CodeNoClasses = "NO_CLASSES"

// msgNoClasses is the refusal a person reads; a closed day adds its label.
const msgNoClasses = "Ese día no hay clases"

// SchoolDay is whether a date has classes for a group. Closed names the
// calendar period that closes it ("" for a weekend or a date outside the cycle).
type SchoolDay struct {
	Open   bool
	Closed string
}

// SchoolCalendar answers whether a group has classes on a date. The rule
// (weekday, inside the cycle, not closed) belongs to config's school calendar;
// attendance only asks (by RPC, ADR 0032).
type SchoolCalendar interface {
	SchoolDayOf(ctx context.Context, groupID string, date time.Time) (SchoolDay, error)
}

// WithSchoolCalendar wires the calendar every save is checked against (T14):
// without it no day is refused, so the composition root always passes one.
func WithSchoolCalendar(c SchoolCalendar) Option {
	return func(s *AttendanceService) { s.calendar = c }
}

// CodedError is a refusal that also names a machine code for the screen. It
// unwraps to the *glerrors.Error, so KindOf and SafeMessage read it.
type CodedError struct {
	Code string
	err  *glerrors.Error
}

// Error implements error.
func (e *CodedError) Error() string { return e.err.Error() }

// Unwrap exposes the typed error.
func (e *CodedError) Unwrap() error { return e.err }

// CodeOf is err's attendance machine code, or "" when it carries none.
func CodeOf(err error) string {
	var c *CodedError
	if errors.As(err, &c) {
		return c.Code
	}
	return ""
}

// noClassesError is the refusal of a save on a day without classes.
func noClassesError(day SchoolDay) error {
	msg := msgNoClasses + "."
	if day.Closed != "" {
		msg = msgNoClasses + " (" + day.Closed + ")."
	}
	return &CodedError{Code: CodeNoClasses, err: glerrors.New(glerrors.KindInvalid, msg)}
}

// schoolDayOf asks the calendar about key's day; with no calendar wired every
// day is open.
func (s *AttendanceService) schoolDayOf(ctx context.Context, key ListKey) (SchoolDay, error) {
	if s.calendar == nil {
		return SchoolDay{Open: true}, nil
	}
	return s.calendar.SchoolDayOf(ctx, key.GroupID, key.Date)
}

// requireSchoolDay refuses a write on a day without classes (NO_CLASSES).
func (s *AttendanceService) requireSchoolDay(ctx context.Context, key ListKey) error {
	day, err := s.schoolDayOf(ctx, key)
	if err != nil {
		return err
	}
	if !day.Open {
		return noClassesError(day)
	}
	return nil
}
