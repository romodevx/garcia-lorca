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
	SchoolWide   bool   // the director's routes: months of a previous group stay writable
}

// MonthlyCell names one cell to clear.
type MonthlyCell struct {
	AssignmentID string
	Month        string
	EnrollmentID string
	SchoolWide   bool
}

// MonthlySheetQuery names the sheet to read; Month "" lets the service pick
// the current month clamped into the cycle.
type MonthlySheetQuery struct {
	AssignmentID string
	Month        string
	SchoolWide   bool
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
// Editable is whether the caller may write this row's cell; ReadOnlyReason
// says why not.
type MonthlySheetRow struct {
	EnrollmentID   string
	StudentID      string
	Value          string
	UpdatedAt      time.Time
	Editable       bool
	ReadOnlyReason string
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
		SchoolWide: in.SchoolWide,
	}))
	if err != nil {
		return MonthlyGrade{}, err
	}
	return toMonthlyGrade(res.Msg.GetGrade()), nil
}

// ClearMonthlyGrade empties one cell (clearing an empty cell succeeds).
func (c *GradesClient) ClearMonthlyGrade(ctx context.Context, cell MonthlyCell) error {
	_, err := c.rpc.ClearMonthlyGrade(ctx, connect.NewRequest(&gradesv1.ClearMonthlyGradeRequest{
		AssignmentId: cell.AssignmentID, Month: cell.Month, EnrollmentId: cell.EnrollmentID, SchoolWide: cell.SchoolWide,
	}))
	return err
}

// GetMonthlySheet reads the capture grid.
func (c *GradesClient) GetMonthlySheet(ctx context.Context, q MonthlySheetQuery) (MonthlySheet, error) {
	res, err := c.rpc.GetMonthlySheet(ctx, connect.NewRequest(&gradesv1.GetMonthlySheetRequest{
		AssignmentId: q.AssignmentID, Month: q.Month, SchoolWide: q.SchoolWide,
	}))
	if err != nil {
		return MonthlySheet{}, err
	}
	m := res.Msg
	rows := make([]MonthlySheetRow, 0, len(m.GetRows()))
	for _, r := range m.GetRows() {
		rows = append(rows, MonthlySheetRow{
			EnrollmentID: r.GetEnrollmentId(), StudentID: r.GetStudentId(), Value: r.GetValue(), UpdatedAt: timeOrZero(r.GetUpdatedAt()),
			Editable: r.GetEditable(), ReadOnlyReason: r.GetReadOnlyReason(),
		})
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

// CountScaleViolations counts the grades captured in the level's active cycle
// that the proposed grading would refuse (every grade when it disables
// grading).
func (c *GradesClient) CountScaleViolations(ctx context.Context, levelID string, g LevelGrading) (int, error) {
	res, err := c.rpc.CountScaleViolations(ctx, connect.NewRequest(&gradesv1.CountScaleViolationsRequest{
		LevelId: levelID, GradingEnabled: g.Enabled,
		Scale: &gradesv1.GradingScale{
			MinHundredths: int32(g.Scale.MinHundredths), MaxHundredths: int32(g.Scale.MaxHundredths),
			Decimals: int32(g.Scale.Decimals), PassingHundredths: int32(g.Scale.PassingHundredths),
		},
	}))
	if err != nil {
		return 0, err
	}
	return int(res.Msg.GetViolations()), nil
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
