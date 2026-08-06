package domain

import (
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Grade value bounds. A grade is a numeric mark on the inclusive [0, 10] scale
// (the SEP-level qualitative Strategy is a later phase).
const (
	minGradeValue = 0.0
	maxGradeValue = 10.0
)

// GradeEntry is one student's numeric grade for a subject in a term. All
// identity references (enrollment, subject, term, teacher) are cross-service
// UUIDs — plain columns validated by RPC at the use-case boundary, never
// cross-service FKs. subjectID and teacherID are DERIVED from the teaching
// assignment by the use case; they are never client-supplied. The entry is
// immutable; WithValue returns a re-validated copy for the upsert-update path.
//
// capturedAt is the instant the grade was captured/edited. It is NOT the future
// boleta "published_at": publication is a later phase, so this timestamp is not
// persisted to the reserved published_at column — the adapter leaves that NULL
// and lets the DB manage created_at/updated_at.
type GradeEntry struct {
	id           string
	tenantID     string
	enrollmentID string
	subjectID    string
	termID       string
	teacherID    string
	value        float64
	capturedAt   time.Time
}

// NewGradeEntry builds a validated GradeEntry. Every identity reference and the
// tenant are required, and value must fall on the inclusive [0, 10] scale
// (out-of-range is KindInvalid). capturedAt stamps when the grade was captured.
func NewGradeEntry(id, tenantID, enrollmentID, subjectID, termID, teacherID string, value float64, capturedAt time.Time) (GradeEntry, error) {
	if strings.TrimSpace(id) == "" {
		return GradeEntry{}, glerrors.New(glerrors.KindInvalid, "grade entry id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return GradeEntry{}, glerrors.New(glerrors.KindInvalid, "tenant id is required")
	}
	if strings.TrimSpace(enrollmentID) == "" {
		return GradeEntry{}, glerrors.New(glerrors.KindInvalid, "enrollment id is required")
	}
	if strings.TrimSpace(subjectID) == "" {
		return GradeEntry{}, glerrors.New(glerrors.KindInvalid, "subject id is required")
	}
	if strings.TrimSpace(termID) == "" {
		return GradeEntry{}, glerrors.New(glerrors.KindInvalid, "term id is required")
	}
	if strings.TrimSpace(teacherID) == "" {
		return GradeEntry{}, glerrors.New(glerrors.KindInvalid, "teacher id is required")
	}
	if value < minGradeValue || value > maxGradeValue {
		return GradeEntry{}, glerrors.New(glerrors.KindInvalid, "grade value must be between 0 and 10")
	}
	return GradeEntry{
		id:           id,
		tenantID:     tenantID,
		enrollmentID: enrollmentID,
		subjectID:    subjectID,
		termID:       termID,
		teacherID:    teacherID,
		value:        value,
		capturedAt:   capturedAt,
	}, nil
}

// WithValue returns a re-validated copy of the entry carrying the new value,
// leaving the receiver unchanged. Used by the capture use case on the
// upsert-update path (re-capturing an existing student·subject·term key).
func (e GradeEntry) WithValue(v float64) (GradeEntry, error) {
	if v < minGradeValue || v > maxGradeValue {
		return GradeEntry{}, glerrors.New(glerrors.KindInvalid, "grade value must be between 0 and 10")
	}
	e.value = v
	return e, nil
}

// Accessors.
func (e GradeEntry) ID() string            { return e.id }
func (e GradeEntry) TenantID() string      { return e.tenantID }
func (e GradeEntry) EnrollmentID() string  { return e.enrollmentID }
func (e GradeEntry) SubjectID() string     { return e.subjectID }
func (e GradeEntry) TermID() string        { return e.termID }
func (e GradeEntry) TeacherID() string     { return e.teacherID }
func (e GradeEntry) Value() float64        { return e.value }
func (e GradeEntry) CapturedAt() time.Time { return e.capturedAt }
