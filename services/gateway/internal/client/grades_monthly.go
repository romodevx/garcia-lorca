package client

import (
	"context"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
)

// MonthlyGradeInput is one autosaved cell. Value is the decimal text exactly as
// the client sent it; the grades service judges it on the level's scale.
type MonthlyGradeInput struct {
	AssignmentID string
	Month        string // "YYYY-MM"
	EnrollmentID string
	Value        string
	CapturedBy   string // the authenticated user id
}

// MonthlyGrade is one captured monthly grade. Value is canonical decimal text
// ("9.5", "10").
type MonthlyGrade struct {
	EnrollmentID string
	SubjectID    string
	Month        string
	Value        string
	UpdatedAt    time.Time
}

// MonthlySheetRow is one active enrollment of the sheet; Value "" = no grade.
type MonthlySheetRow struct {
	EnrollmentID string
	StudentID    string
	Value        string
	UpdatedAt    time.Time
}

// MonthlySheet is the capture grid of one assignment for one month.
type MonthlySheet struct {
	AssignmentID   string
	Month          string
	Months         []string
	Scale          GradeScale
	GradingEnabled bool
	Editable       bool
	ReadOnlyReason string
	Rows           []MonthlySheetRow
}

// EnrollmentMonthlyGrades is one enrollment's monthly grades for its cycle.
type EnrollmentMonthlyGrades struct {
	CycleID        string
	CycleName      string
	Months         []string
	Scale          GradeScale
	GradingEnabled bool
	SubjectIDs     []string
	Grades         []MonthlyGrade
}

// UpsertMonthlyGrade records or corrects one cell. Refusals (scale, decimals,
// month, level) come back InvalidArgument with the user's sentence; an unknown
// assignment is NotFound.
func (c *GradesClient) UpsertMonthlyGrade(ctx context.Context, in MonthlyGradeInput) (MonthlyGrade, error) {
	res, err := c.rpc.UpsertMonthlyGrade(ctx, connect.NewRequest(&gradesv1.UpsertMonthlyGradeRequest{
		AssignmentId: in.AssignmentID, Month: in.Month, EnrollmentId: in.EnrollmentID, Value: in.Value, CapturedBy: in.CapturedBy,
	}))
	if err != nil {
		return MonthlyGrade{}, err
	}
	return toMonthlyGrade(res.Msg.GetGrade()), nil
}

// ClearMonthlyGrade empties one cell (clearing an empty cell succeeds).
func (c *GradesClient) ClearMonthlyGrade(ctx context.Context, assignmentID, month, enrollmentID string) error {
	_, err := c.rpc.ClearMonthlyGrade(ctx, connect.NewRequest(&gradesv1.ClearMonthlyGradeRequest{
		AssignmentId: assignmentID, Month: month, EnrollmentId: enrollmentID,
	}))
	return err
}

// GetMonthlySheet reads the capture grid; month "" lets the service pick the
// current month clamped into the cycle.
func (c *GradesClient) GetMonthlySheet(ctx context.Context, assignmentID, month string) (MonthlySheet, error) {
	res, err := c.rpc.GetMonthlySheet(ctx, connect.NewRequest(&gradesv1.GetMonthlySheetRequest{AssignmentId: assignmentID, Month: month}))
	if err != nil {
		return MonthlySheet{}, err
	}
	m := res.Msg
	rows := make([]MonthlySheetRow, 0, len(m.GetRows()))
	for _, r := range m.GetRows() {
		rows = append(rows, MonthlySheetRow{EnrollmentID: r.GetEnrollmentId(), StudentID: r.GetStudentId(), Value: r.GetValue(), UpdatedAt: timeOrZero(r.GetUpdatedAt())})
	}
	return MonthlySheet{
		AssignmentID: m.GetAssignmentId(), Month: m.GetMonth(), Months: m.GetMonths(), Scale: toGradeScale(m.GetScale()),
		GradingEnabled: m.GetGradingEnabled(), Editable: m.GetEditable(), ReadOnlyReason: m.GetReadOnlyReason(), Rows: rows,
	}, nil
}

// GetEnrollmentMonthlyGrades reads one enrollment's monthly grades.
func (c *GradesClient) GetEnrollmentMonthlyGrades(ctx context.Context, enrollmentID string) (EnrollmentMonthlyGrades, error) {
	res, err := c.rpc.GetEnrollmentMonthlyGrades(ctx, connect.NewRequest(&gradesv1.GetEnrollmentMonthlyGradesRequest{EnrollmentId: enrollmentID}))
	if err != nil {
		return EnrollmentMonthlyGrades{}, err
	}
	m := res.Msg
	grades := make([]MonthlyGrade, 0, len(m.GetGrades()))
	for _, g := range m.GetGrades() {
		grades = append(grades, toMonthlyGrade(g))
	}
	return EnrollmentMonthlyGrades{
		CycleID: m.GetCycleId(), CycleName: m.GetCycleName(), Months: m.GetMonths(), Scale: toGradeScale(m.GetScale()),
		GradingEnabled: m.GetGradingEnabled(), SubjectIDs: m.GetSubjectIds(), Grades: grades,
	}, nil
}

func toMonthlyGrade(g *gradesv1.MonthlyGrade) MonthlyGrade {
	return MonthlyGrade{EnrollmentID: g.GetEnrollmentId(), SubjectID: g.GetSubjectId(), Month: g.GetMonth(), Value: g.GetValue(), UpdatedAt: timeOrZero(g.GetUpdatedAt())}
}

func toGradeScale(s *gradesv1.GradingScale) GradeScale {
	return GradeScale{
		MinHundredths:     int(s.GetMinHundredths()),
		MaxHundredths:     int(s.GetMaxHundredths()),
		Decimals:          int(s.GetDecimals()),
		PassingHundredths: int(s.GetPassingHundredths()),
	}
}

// timeOrZero keeps an unset timestamp as the zero time, never the Unix epoch.
func timeOrZero(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime().UTC()
}
