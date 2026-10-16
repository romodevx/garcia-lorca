package domain

import (
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// EnrollmentStatus is the lifecycle state of an enrollment (a student's place in
// a group of a cycle). It is a domain type (no proto import); the adapter maps it
// to/from the wire enum.
type EnrollmentStatus uint8

const (
	// EnrollmentStatusUnspecified is the zero value; not a valid status.
	EnrollmentStatusUnspecified EnrollmentStatus = iota
	// EnrollmentStatusActive is the live enrollment (can transfer or withdraw).
	EnrollmentStatusActive
	// EnrollmentStatusTransferred marks an enrollment superseded by a move.
	EnrollmentStatusTransferred
	// EnrollmentStatusWithdrawn marks a terminated enrollment.
	EnrollmentStatusWithdrawn
)

var enrollmentStatusNames = map[EnrollmentStatus]string{
	EnrollmentStatusActive:      "active",
	EnrollmentStatusTransferred: "transferred",
	EnrollmentStatusWithdrawn:   "withdrawn",
}

// Valid reports whether the status is a real enrollment status (not unspecified).
func (s EnrollmentStatus) Valid() bool {
	return s >= EnrollmentStatusActive && s <= EnrollmentStatusWithdrawn
}

// String returns the canonical status name ("" for unspecified).
func (s EnrollmentStatus) String() string { return enrollmentStatusNames[s] }

// ParseEnrollmentStatus resolves a status name to an EnrollmentStatus.
func ParseEnrollmentStatus(raw string) (EnrollmentStatus, error) {
	key := strings.ToLower(strings.TrimSpace(raw))
	for status, name := range enrollmentStatusNames {
		if name == key {
			return status, nil
		}
	}
	return EnrollmentStatusUnspecified, glerrors.New(glerrors.KindInvalid, "unknown enrollment status: "+raw)
}

// Enrollment is the join placing a student into a group of a cycle. cycle_id,
// group_id and term_id are cross-service references to the config bounded context
// (plain UUIDs, RPC-validated by the use case — never cross-service FKs). term_id
// is optional. The cycle is derived from the group at enroll time, so an
// Enrollment always carries a group and its cycle together.
type Enrollment struct {
	id               string
	tenantID         string
	studentID        string
	cycleID          string
	groupID          string
	termID           string
	status           EnrollmentStatus
	enrolledAt       time.Time
	endedAt          time.Time // zero while active
	withdrawnOn      time.Time
	withdrawalReason string
	origin           EnrollmentOrigin
}

// EnrollmentOrigin says how a placement was made. Finance reads it: an
// imported pupil was enrolled and paid in the previous system, so their
// inscription is never billed here (ADR 0033).
type EnrollmentOrigin string

// The origins an enrollment may have.
const (
	EnrollmentOriginManual EnrollmentOrigin = "manual"
	EnrollmentOriginImport EnrollmentOrigin = "import"
)

// ParseEnrollmentOrigin resolves a stored origin; empty is manual (every
// placement made before the column existed).
func ParseEnrollmentOrigin(raw string) (EnrollmentOrigin, error) {
	switch o := EnrollmentOrigin(strings.ToLower(strings.TrimSpace(raw))); o {
	case "", EnrollmentOriginManual:
		return EnrollmentOriginManual, nil
	case EnrollmentOriginImport:
		return o, nil
	default:
		return "", glerrors.New(glerrors.KindInvalid, "unknown enrollment origin: "+raw)
	}
}

// WithOrigin returns the enrollment marked with how it was made.
func (e Enrollment) WithOrigin(o EnrollmentOrigin) Enrollment {
	e.origin = o
	return e
}

// Origin is how the placement was made; manual unless marked otherwise.
func (e Enrollment) Origin() EnrollmentOrigin {
	if e.origin == "" {
		return EnrollmentOriginManual
	}
	return e.origin
}

// NewEnrollment builds a validated Enrollment. All identity references except
// termID are required, and the status must be a real enrollment status.
func NewEnrollment(id, tenantID, studentID, cycleID, groupID, termID string, status EnrollmentStatus, enrolledAt time.Time) (Enrollment, error) {
	if strings.TrimSpace(id) == "" {
		return Enrollment{}, glerrors.New(glerrors.KindInvalid, "enrollment id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return Enrollment{}, glerrors.New(glerrors.KindInvalid, "tenant id is required")
	}
	if strings.TrimSpace(studentID) == "" {
		return Enrollment{}, glerrors.New(glerrors.KindInvalid, "student id is required")
	}
	if strings.TrimSpace(cycleID) == "" {
		return Enrollment{}, glerrors.New(glerrors.KindInvalid, "cycle id is required")
	}
	if strings.TrimSpace(groupID) == "" {
		return Enrollment{}, glerrors.New(glerrors.KindInvalid, "group id is required")
	}
	if !status.Valid() {
		return Enrollment{}, glerrors.New(glerrors.KindInvalid, "a valid enrollment status is required")
	}
	return Enrollment{
		id:         id,
		tenantID:   tenantID,
		studentID:  studentID,
		cycleID:    cycleID,
		groupID:    groupID,
		termID:     termID,
		status:     status,
		enrolledAt: enrolledAt,
	}, nil
}

// WithEndedAt records when the enrollment stopped being active (its
// status-change instant). An active enrollment has not ended, so it is returned
// unchanged.
func (e Enrollment) WithEndedAt(t time.Time) Enrollment {
	if e.status == EnrollmentStatusActive {
		return e
	}
	e.endedAt = t
	return e
}

// CanTransfer reports whether the enrollment may be transferred to another group.
func (e Enrollment) CanTransfer() bool { return e.status == EnrollmentStatusActive }

// CanWithdraw reports whether the enrollment may be withdrawn.
func (e Enrollment) CanWithdraw() bool { return e.status == EnrollmentStatusActive }

// MaxEnrollmentWithdrawalReason is the largest allowed reason, in Unicode characters.
const MaxEnrollmentWithdrawalReason = 500

// Withdraw ends an active enrollment on a UTC calendar day. A zero date means
// the caller's current day, supplied explicitly for deterministic behavior.
func (e Enrollment) Withdraw(on time.Time, reason string, now time.Time) (Enrollment, error) {
	if !e.CanWithdraw() {
		return Enrollment{}, glerrors.New(glerrors.KindConflict, "enrollment is not active")
	}
	today := now.UTC()
	if on.IsZero() {
		on = today
	}
	day := utcCalendarDay(on)
	if day.After(utcCalendarDay(today)) {
		return Enrollment{}, glerrors.New(glerrors.KindInvalid, "withdrawal date cannot be in the future")
	}
	if day.Before(utcCalendarDay(e.enrolledAt)) {
		return Enrollment{}, glerrors.New(glerrors.KindInvalid, "withdrawal date cannot precede enrollment")
	}
	trimmed := strings.TrimSpace(reason)
	if len([]rune(trimmed)) > MaxEnrollmentWithdrawalReason {
		return Enrollment{}, glerrors.New(glerrors.KindInvalid, "withdrawal reason exceeds 500 characters")
	}
	e.status = EnrollmentStatusWithdrawn
	e.withdrawnOn = day
	e.withdrawalReason = trimmed
	e.endedAt = day
	return e, nil
}

// WithWithdrawal restores persisted withdrawal details without revalidating
// historical data. It is intended for persistence adapters only.
func (e Enrollment) WithWithdrawal(on time.Time, reason string) Enrollment {
	e.withdrawnOn = utcCalendarDay(on)
	e.withdrawalReason = reason
	if e.status == EnrollmentStatusWithdrawn {
		e.endedAt = e.withdrawnOn
	}
	return e
}

func utcCalendarDay(t time.Time) time.Time {
	year, month, day := t.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// Accessors.
func (e Enrollment) ID() string               { return e.id }
func (e Enrollment) TenantID() string         { return e.tenantID }
func (e Enrollment) StudentID() string        { return e.studentID }
func (e Enrollment) CycleID() string          { return e.cycleID }
func (e Enrollment) GroupID() string          { return e.groupID }
func (e Enrollment) TermID() string           { return e.termID }
func (e Enrollment) Status() EnrollmentStatus { return e.status }
func (e Enrollment) EnrolledAt() time.Time    { return e.enrolledAt }
func (e Enrollment) EndedAt() time.Time       { return e.endedAt }
func (e Enrollment) WithdrawnOn() time.Time   { return e.withdrawnOn }
func (e Enrollment) WithdrawalReason() string { return e.withdrawalReason }
