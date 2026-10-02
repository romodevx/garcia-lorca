package client

import (
	"context"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
)

// EvaluationPeriodRef is an evaluation period of a grade (config, T14) as the
// grades service names it; dates are inclusive "YYYY-MM-DD".
type EvaluationPeriodRef struct {
	ID       string
	Ordinal  int
	Name     string
	StartsOn string
	EndsOn   string
}

// PeriodGradeInput is one autosaved cell (T15). Value is the decimal text
// exactly as the client sent it; the grades service judges it — a whole number
// on the level's scale.
type PeriodGradeInput struct {
	AssignmentID   string
	PeriodID       string
	EnrollmentID   string
	Value          string
	CapturedBy     string // the authenticated user id
	CapturedByName string // the authenticated user's name (audit)
	SchoolWide     bool   // the director's routes: periods of a previous group stay writable
}

// PeriodCell names one cell to clear.
type PeriodCell struct {
	AssignmentID string
	PeriodID     string
	EnrollmentID string
	SchoolWide   bool
}

// PeriodSheetQuery names the sheet to read; PeriodID "" lets the service pick
// the period of the school's day.
type PeriodSheetQuery struct {
	AssignmentID string
	PeriodID     string
	SchoolWide   bool
}

// PeriodGrade is one captured period grade. Value is canonical decimal text
// of a whole number ("9", "10").
type PeriodGrade struct {
	EnrollmentID string
	SubjectID    string
	PeriodID     string
	Value        string
	UpdatedAt    time.Time
}

// PeriodSheetRow is one active enrollment of the sheet; Value "" = no grade.
// Editable is whether the caller may write this row's cell; ReadOnlyReason
// says why not.
type PeriodSheetRow struct {
	EnrollmentID   string
	StudentID      string
	Value          string
	UpdatedAt      time.Time
	Editable       bool
	ReadOnlyReason string
}

// PeriodSheet is the capture grid of one assignment for one period.
type PeriodSheet struct {
	AssignmentID    string
	PeriodID        string
	Periods         []EvaluationPeriodRef
	CurrentPeriodID string
	Today           string
	Scale           GradeScale
	GradingEnabled  bool
	Editable        bool
	ReadOnlyReason  string
	Rows            []PeriodSheetRow
	// ClosedPeriodIDs are the periods whose level term is CLOSED.
	ClosedPeriodIDs []string
}

// EnrollmentPeriodGrades is one enrollment's period grades for its cycle.
type EnrollmentPeriodGrades struct {
	CycleID        string
	CycleName      string
	Periods        []EvaluationPeriodRef
	Scale          GradeScale
	GradingEnabled bool
	SubjectIDs     []string
	Grades         []PeriodGrade
}

// UpsertPeriodGrade records or corrects one cell. Refusals come back
// InvalidArgument (or AlreadyExists for a closed term) with the user's
// sentence and an X-Error-Code; an unknown assignment is NotFound.
func (c *GradesClient) UpsertPeriodGrade(ctx context.Context, in PeriodGradeInput) (PeriodGrade, error) {
	res, err := c.rpc.UpsertPeriodGrade(ctx, connect.NewRequest(&gradesv1.UpsertPeriodGradeRequest{
		AssignmentId: in.AssignmentID, PeriodId: in.PeriodID, EnrollmentId: in.EnrollmentID, Value: in.Value,
		CapturedBy: in.CapturedBy, CapturedByName: in.CapturedByName, SchoolWide: in.SchoolWide,
	}))
	if err != nil {
		return PeriodGrade{}, err
	}
	return toPeriodGrade(res.Msg.GetGrade()), nil
}

// ClearPeriodGrade empties one cell (clearing an empty cell succeeds).
func (c *GradesClient) ClearPeriodGrade(ctx context.Context, cell PeriodCell) error {
	_, err := c.rpc.ClearPeriodGrade(ctx, connect.NewRequest(&gradesv1.ClearPeriodGradeRequest{
		AssignmentId: cell.AssignmentID, PeriodId: cell.PeriodID, EnrollmentId: cell.EnrollmentID, SchoolWide: cell.SchoolWide,
	}))
	return err
}

// GetPeriodSheet reads the capture grid.
func (c *GradesClient) GetPeriodSheet(ctx context.Context, q PeriodSheetQuery) (PeriodSheet, error) {
	res, err := c.rpc.GetPeriodSheet(ctx, connect.NewRequest(&gradesv1.GetPeriodSheetRequest{
		AssignmentId: q.AssignmentID, PeriodId: q.PeriodID, SchoolWide: q.SchoolWide,
	}))
	if err != nil {
		return PeriodSheet{}, err
	}
	m := res.Msg
	rows := make([]PeriodSheetRow, 0, len(m.GetRows()))
	for _, r := range m.GetRows() {
		rows = append(rows, PeriodSheetRow{
			EnrollmentID: r.GetEnrollmentId(), StudentID: r.GetStudentId(), Value: r.GetValue(), UpdatedAt: timeOrZero(r.GetUpdatedAt()),
			Editable: r.GetEditable(), ReadOnlyReason: r.GetReadOnlyReason(),
		})
	}
	return PeriodSheet{
		AssignmentID: m.GetAssignmentId(), PeriodID: m.GetPeriodId(), Periods: toPeriodRefs(m.GetPeriods()),
		CurrentPeriodID: m.GetCurrentPeriodId(), Today: m.GetToday(), Scale: toGradeScale(m.GetScale()),
		GradingEnabled: m.GetGradingEnabled(), Editable: m.GetEditable(), ReadOnlyReason: m.GetReadOnlyReason(), Rows: rows,
		ClosedPeriodIDs: m.GetClosedPeriodIds(),
	}, nil
}

// GetEnrollmentPeriodGrades reads one enrollment's period grades.
func (c *GradesClient) GetEnrollmentPeriodGrades(ctx context.Context, enrollmentID string) (EnrollmentPeriodGrades, error) {
	res, err := c.rpc.GetEnrollmentPeriodGrades(ctx, connect.NewRequest(&gradesv1.GetEnrollmentPeriodGradesRequest{EnrollmentId: enrollmentID}))
	if err != nil {
		return EnrollmentPeriodGrades{}, err
	}
	m := res.Msg
	grades := make([]PeriodGrade, 0, len(m.GetGrades()))
	for _, g := range m.GetGrades() {
		grades = append(grades, toPeriodGrade(g))
	}
	return EnrollmentPeriodGrades{
		CycleID: m.GetCycleId(), CycleName: m.GetCycleName(), Periods: toPeriodRefs(m.GetPeriods()), Scale: toGradeScale(m.GetScale()),
		GradingEnabled: m.GetGradingEnabled(), SubjectIDs: m.GetSubjectIds(), Grades: grades,
	}, nil
}

// PeriodsWithGrades asks which of these evaluation periods hold a grade (one
// call). The gateway asks before config removes a period.
func (c *GradesClient) PeriodsWithGrades(ctx context.Context, periodIDs []string) ([]string, error) {
	res, err := c.rpc.PeriodsWithGrades(ctx, connect.NewRequest(&gradesv1.PeriodsWithGradesRequest{PeriodIds: periodIDs}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetPeriodIds(), nil
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

func toPeriodGrade(g *gradesv1.PeriodGrade) PeriodGrade {
	return PeriodGrade{EnrollmentID: g.GetEnrollmentId(), SubjectID: g.GetSubjectId(), PeriodID: g.GetPeriodId(), Value: g.GetValue(), UpdatedAt: timeOrZero(g.GetUpdatedAt())}
}

func toPeriodRefs(ps []*gradesv1.EvaluationPeriodRef) []EvaluationPeriodRef {
	out := make([]EvaluationPeriodRef, 0, len(ps))
	for _, p := range ps {
		out = append(out, EvaluationPeriodRef{ID: p.GetId(), Ordinal: int(p.GetOrdinal()), Name: p.GetName(), StartsOn: p.GetStartsOn(), EndsOn: p.GetEndsOn()})
	}
	return out
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
