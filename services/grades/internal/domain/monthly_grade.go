package domain

import (
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// MonthlyGrade is one student's grade for a subject in a calendar month (T6).
// The school captures monthly; the legacy term-based GradeEntry stays as it was
// and is no longer written.
//
// enrollment/subject/teacher are cross-service UUIDs validated by RPC; subject
// and teacher are DERIVED from the teaching assignment, never client input.
// capturedBy is the authenticated user who last wrote the value (a director may
// correct a teacher's grade). The value was already judged against the level's
// scale (GradingScale.ParseValue) — the entity only carries it.
type MonthlyGrade struct {
	id           string
	tenantID     string
	enrollmentID string
	subjectID    string
	teacherID    string
	capturedBy   string
	month        Month
	value        GradeValue
	updatedAt    time.Time
}

// MonthlyGradeFields are the parts of a MonthlyGrade (a parameter object: nine
// positional strings would invite a swapped argument).
type MonthlyGradeFields struct {
	ID           string
	TenantID     string
	EnrollmentID string
	SubjectID    string
	TeacherID    string
	CapturedBy   string
	Month        Month
	Value        GradeValue
	UpdatedAt    time.Time
}

// NewMonthlyGrade builds a validated MonthlyGrade: every id and the month are
// required (KindInvalid otherwise).
func NewMonthlyGrade(f MonthlyGradeFields) (MonthlyGrade, error) {
	required := []struct{ value, what string }{
		{f.ID, "monthly grade id"},
		{f.TenantID, "tenant id"},
		{f.EnrollmentID, "enrollment id"},
		{f.SubjectID, "subject id"},
		{f.TeacherID, "teacher id"},
		{f.CapturedBy, "captured by"},
	}
	for _, r := range required {
		if strings.TrimSpace(r.value) == "" {
			return MonthlyGrade{}, glerrors.New(glerrors.KindInvalid, r.what+" is required")
		}
	}
	if f.Month.IsZero() {
		return MonthlyGrade{}, glerrors.New(glerrors.KindInvalid, "month is required")
	}
	return MonthlyGrade{
		id:           f.ID,
		tenantID:     f.TenantID,
		enrollmentID: f.EnrollmentID,
		subjectID:    f.SubjectID,
		teacherID:    f.TeacherID,
		capturedBy:   f.CapturedBy,
		month:        f.Month,
		value:        f.Value,
		updatedAt:    f.UpdatedAt,
	}, nil
}

// Accessors.
func (g MonthlyGrade) ID() string           { return g.id }
func (g MonthlyGrade) TenantID() string     { return g.tenantID }
func (g MonthlyGrade) EnrollmentID() string { return g.enrollmentID }
func (g MonthlyGrade) SubjectID() string    { return g.subjectID }
func (g MonthlyGrade) TeacherID() string    { return g.teacherID }
func (g MonthlyGrade) CapturedBy() string   { return g.capturedBy }
func (g MonthlyGrade) Month() Month         { return g.month }
func (g MonthlyGrade) Value() GradeValue    { return g.value }
func (g MonthlyGrade) UpdatedAt() time.Time { return g.updatedAt }
