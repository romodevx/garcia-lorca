package grpcadapter

import (
	"context"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
	"github.com/garcia-lorca/services/grades/internal/usecase"
)

// dayLayout is how a calendar day travels on the wire ("2026-08-24").
const dayLayout = "2006-01-02"

// UpsertPeriodGrade records or corrects one period cell (T15). The value
// travels as decimal text and is judged exactly — a whole number — by the use
// case.
func (h *GradesHandler) UpsertPeriodGrade(ctx context.Context, req *connect.Request[gradesv1.UpsertPeriodGradeRequest]) (*connect.Response[gradesv1.UpsertPeriodGradeResponse], error) {
	m := req.Msg
	g, err := h.periodGrades.Upsert(ctx, usecase.UpsertPeriodGradeInput{
		AssignmentID: m.GetAssignmentId(), PeriodID: m.GetPeriodId(), EnrollmentID: m.GetEnrollmentId(),
		Value: m.GetValue(), CapturedBy: m.GetCapturedBy(), CapturedByName: m.GetCapturedByName(),
		Scope: scopeOf(m.GetSchoolWide()),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&gradesv1.UpsertPeriodGradeResponse{Grade: toProtoPeriodGrade(g)}), nil
}

// ClearPeriodGrade empties one period cell (idempotent).
func (h *GradesHandler) ClearPeriodGrade(ctx context.Context, req *connect.Request[gradesv1.ClearPeriodGradeRequest]) (*connect.Response[gradesv1.ClearPeriodGradeResponse], error) {
	m := req.Msg
	if err := h.periodGrades.Clear(ctx, scopeOf(m.GetSchoolWide()), m.GetAssignmentId(), m.GetPeriodId(), m.GetEnrollmentId()); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&gradesv1.ClearPeriodGradeResponse{}), nil
}

// GetPeriodSheet returns the capture grid of one assignment for one period.
func (h *GradesHandler) GetPeriodSheet(ctx context.Context, req *connect.Request[gradesv1.GetPeriodSheetRequest]) (*connect.Response[gradesv1.GetPeriodSheetResponse], error) {
	sheet, err := h.periodGrades.Sheet(ctx, scopeOf(req.Msg.GetSchoolWide()), req.Msg.GetAssignmentId(), req.Msg.GetPeriodId())
	if err != nil {
		return nil, connectErr(err)
	}
	rows := make([]*gradesv1.PeriodSheetRow, 0, len(sheet.Rows))
	for _, r := range sheet.Rows {
		row := &gradesv1.PeriodSheetRow{
			EnrollmentId: r.EnrollmentID, StudentId: r.StudentID, Editable: r.Editable, ReadOnlyReason: r.ReadOnlyReason,
		}
		if r.HasValue {
			row.Value, row.UpdatedAt = r.Value.String(), timestampOrNil(r.UpdatedAt)
			if r.Letter != "" {
				row.Value = string(r.Letter)
			}
		}
		rows = append(rows, row)
	}
	closed := sheet.ClosedPeriodIDs
	if closed == nil {
		closed = []string{}
	}
	return connect.NewResponse(&gradesv1.GetPeriodSheetResponse{
		AssignmentId:    sheet.AssignmentID,
		PeriodId:        sheet.PeriodID,
		Periods:         toProtoPeriods(sheet.Periods),
		Scale:           toProtoScale(sheet.Scale),
		GradingEnabled:  sheet.GradingEnabled,
		GradingLetters:  sheet.GradingLetters,
		Editable:        sheet.Editable,
		ReadOnlyReason:  sheet.ReadOnlyReason,
		Rows:            rows,
		ClosedPeriodIds: closed,
		CurrentPeriodId: sheet.CurrentPeriodID,
		Today:           dayText(sheet.Today),
	}), nil
}

// GetEnrollmentPeriodGrades returns one enrollment's period grades for its cycle.
func (h *GradesHandler) GetEnrollmentPeriodGrades(ctx context.Context, req *connect.Request[gradesv1.GetEnrollmentPeriodGradesRequest]) (*connect.Response[gradesv1.GetEnrollmentPeriodGradesResponse], error) {
	got, err := h.periodGrades.ForEnrollment(ctx, req.Msg.GetEnrollmentId())
	if err != nil {
		return nil, connectErr(err)
	}
	grades := make([]*gradesv1.PeriodGrade, 0, len(got.Grades))
	for _, g := range got.Grades {
		grades = append(grades, toProtoPeriodGrade(g))
	}
	return connect.NewResponse(&gradesv1.GetEnrollmentPeriodGradesResponse{
		CycleId: got.CycleID, CycleName: got.CycleName, Periods: toProtoPeriods(got.Periods),
		Scale: toProtoScale(got.Scale), GradingEnabled: got.GradingEnabled, GradingLetters: got.GradingLetters,
		SubjectIds: got.SubjectIDs, Grades: grades,
	}), nil
}

// PeriodsWithGrades answers which of the requested periods hold a grade.
func (h *GradesHandler) PeriodsWithGrades(ctx context.Context, req *connect.Request[gradesv1.PeriodsWithGradesRequest]) (*connect.Response[gradesv1.PeriodsWithGradesResponse], error) {
	ids, err := h.periodGrades.PeriodsWithGrades(ctx, req.Msg.GetPeriodIds())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&gradesv1.PeriodsWithGradesResponse{PeriodIds: ids}), nil
}

// CountScaleViolations counts the period grades of the level's active cycle a
// proposed scale would refuse. An inconsistent enabled scale is
// InvalidArgument; a disabling request ignores the scale.
func (h *GradesHandler) CountScaleViolations(ctx context.Context, req *connect.Request[gradesv1.CountScaleViolationsRequest]) (*connect.Response[gradesv1.CountScaleViolationsResponse], error) {
	m := req.Msg
	check := ports.ScaleCheck{GradingEnabled: m.GetGradingEnabled(), GradingLetters: m.GetGradingLetters() && !m.GetGradingEnabled()}
	if check.GradingEnabled {
		sc := m.GetScale()
		scale, err := domain.NewGradingScale(int(sc.GetMinHundredths()), int(sc.GetMaxHundredths()), int(sc.GetPassingHundredths()), int(sc.GetDecimals()))
		if err != nil {
			return nil, connectErr(err)
		}
		check.Scale = scale
	}
	n, err := h.periodGrades.CountScaleViolations(ctx, m.GetLevelId(), check)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&gradesv1.CountScaleViolationsResponse{Violations: int32(n)}), nil
}

// scopeOf maps the wire flag to the use case's scope: only the director's
// routes set school_wide.
func scopeOf(schoolWide bool) usecase.CaptureScope {
	if schoolWide {
		return usecase.ScopeSchool
	}
	return usecase.ScopeTeacher
}

func toProtoPeriodGrade(g domain.PeriodGrade) *gradesv1.PeriodGrade {
	return &gradesv1.PeriodGrade{
		EnrollmentId: g.EnrollmentID(), SubjectId: g.SubjectID(), PeriodId: g.PeriodID(),
		Value: g.Text(), UpdatedAt: timestampOrNil(g.UpdatedAt()), CapturedByName: g.CapturedByName(),
	}
}

func toProtoPeriods(periods []domain.EvaluationPeriod) []*gradesv1.EvaluationPeriodRef {
	out := make([]*gradesv1.EvaluationPeriodRef, 0, len(periods))
	for _, p := range periods {
		out = append(out, &gradesv1.EvaluationPeriodRef{
			Id: p.ID, Ordinal: int32(p.Ordinal), Name: p.Name, StartsOn: dayText(p.StartsOn), EndsOn: dayText(p.EndsOn),
		})
	}
	return out
}

func toProtoScale(s domain.GradingScale) *gradesv1.GradingScale {
	return &gradesv1.GradingScale{
		MinHundredths:     int32(s.MinHundredths()),
		MaxHundredths:     int32(s.MaxHundredths()),
		Decimals:          int32(s.Decimals()),
		PassingHundredths: int32(s.PassingHundredths()),
	}
}

// dayText is a calendar day as "YYYY-MM-DD"; "" for the zero time.
func dayText(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(dayLayout)
}

func monthStrings(months []domain.Month) []string {
	out := make([]string, 0, len(months))
	for _, m := range months {
		out = append(out, m.String())
	}
	return out
}

func timestampOrNil(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}
