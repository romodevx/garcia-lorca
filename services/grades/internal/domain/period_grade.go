package domain

import (
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// PeriodGrade is one student's grade for a subject in an evaluation period
// (T15). It replaces the monthly grade: monthly_grades stays in the database,
// untouched, and nothing reads or writes it any more.
//
// enrollment/subject/period are cross-service UUIDs validated by RPC; the
// subject is DERIVED from the teaching assignment, never client input.
// capturedBy/capturedByName are who last wrote the value (a director may
// correct a teacher's grade). The value was already judged as a whole number
// on the level's scale (GradingScale.ParseWholeValue). A grade of a level that
// grades with letters carries a Letter instead, and no value.
type PeriodGrade struct {
	id             string
	tenantID       string
	enrollmentID   string
	subjectID      string
	periodID       string
	capturedBy     string
	capturedByName string
	value          GradeValue
	letter         Letter
	updatedAt      time.Time
}

// PeriodGradeFields are the parts of a PeriodGrade.
type PeriodGradeFields struct {
	ID             string
	TenantID       string
	EnrollmentID   string
	SubjectID      string
	PeriodID       string
	CapturedBy     string
	CapturedByName string
	Value          GradeValue
	// Letter is set instead of Value on a level that grades with letters.
	Letter    Letter
	UpdatedAt time.Time
}

// NewPeriodGrade builds a validated PeriodGrade: every id is required
// (KindInvalid otherwise).
func NewPeriodGrade(f PeriodGradeFields) (PeriodGrade, error) {
	required := []struct{ value, what string }{
		{f.ID, "period grade id"},
		{f.TenantID, "tenant id"},
		{f.EnrollmentID, "enrollment id"},
		{f.SubjectID, "subject id"},
		{f.PeriodID, "period id"},
		{f.CapturedBy, "captured by"},
	}
	for _, r := range required {
		if strings.TrimSpace(r.value) == "" {
			return PeriodGrade{}, glerrors.New(glerrors.KindInvalid, r.what+" is required")
		}
	}
	return PeriodGrade{
		id: f.ID, tenantID: f.TenantID, enrollmentID: f.EnrollmentID, subjectID: f.SubjectID,
		periodID: f.PeriodID, capturedBy: f.CapturedBy, capturedByName: f.CapturedByName,
		value: f.Value, letter: f.Letter, updatedAt: f.UpdatedAt,
	}, nil
}

// Accessors.
func (g PeriodGrade) ID() string             { return g.id }
func (g PeriodGrade) TenantID() string       { return g.tenantID }
func (g PeriodGrade) EnrollmentID() string   { return g.enrollmentID }
func (g PeriodGrade) SubjectID() string      { return g.subjectID }
func (g PeriodGrade) PeriodID() string       { return g.periodID }
func (g PeriodGrade) CapturedBy() string     { return g.capturedBy }
func (g PeriodGrade) CapturedByName() string { return g.capturedByName }
func (g PeriodGrade) Value() GradeValue      { return g.value }
func (g PeriodGrade) UpdatedAt() time.Time   { return g.updatedAt }
func (g PeriodGrade) Letter() Letter         { return g.letter }

// IsLetter reports whether the grade is a letter rather than a number.
func (g PeriodGrade) IsLetter() bool { return g.letter != "" }

// Text is the grade as the wire and the report card carry it: "9" or "B".
func (g PeriodGrade) Text() string {
	if g.IsLetter() {
		return string(g.letter)
	}
	return g.value.String()
}
