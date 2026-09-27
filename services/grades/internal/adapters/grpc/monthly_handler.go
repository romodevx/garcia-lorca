package grpcadapter

import (
	"context"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/usecase"
)

// UpsertMonthlyGrade records or corrects one monthly cell. The value travels as
// decimal text and is judged exactly by the use case.
func (h *GradesHandler) UpsertMonthlyGrade(ctx context.Context, req *connect.Request[gradesv1.UpsertMonthlyGradeRequest]) (*connect.Response[gradesv1.UpsertMonthlyGradeResponse], error) {
	m := req.Msg
	g, err := h.monthly.Upsert(ctx, usecase.UpsertMonthlyInput{
		AssignmentID: m.GetAssignmentId(), Month: m.GetMonth(), EnrollmentID: m.GetEnrollmentId(),
		Value: m.GetValue(), CapturedBy: m.GetCapturedBy(),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&gradesv1.UpsertMonthlyGradeResponse{Grade: toProtoMonthlyGrade(g)}), nil
}

// ClearMonthlyGrade empties one monthly cell (idempotent).
func (h *GradesHandler) ClearMonthlyGrade(ctx context.Context, req *connect.Request[gradesv1.ClearMonthlyGradeRequest]) (*connect.Response[gradesv1.ClearMonthlyGradeResponse], error) {
	m := req.Msg
	if err := h.monthly.Clear(ctx, m.GetAssignmentId(), m.GetMonth(), m.GetEnrollmentId()); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&gradesv1.ClearMonthlyGradeResponse{}), nil
}

// GetMonthlySheet returns the capture grid of one assignment for one month.
func (h *GradesHandler) GetMonthlySheet(ctx context.Context, req *connect.Request[gradesv1.GetMonthlySheetRequest]) (*connect.Response[gradesv1.GetMonthlySheetResponse], error) {
	sheet, err := h.monthly.Sheet(ctx, req.Msg.GetAssignmentId(), req.Msg.GetMonth())
	if err != nil {
		return nil, connectErr(err)
	}
	rows := make([]*gradesv1.MonthlySheetRow, 0, len(sheet.Rows))
	for _, r := range sheet.Rows {
		row := &gradesv1.MonthlySheetRow{EnrollmentId: r.EnrollmentID, StudentId: r.StudentID}
		if r.HasValue {
			row.Value, row.UpdatedAt = r.Value.String(), timestampOrNil(r.UpdatedAt)
		}
		rows = append(rows, row)
	}
	return connect.NewResponse(&gradesv1.GetMonthlySheetResponse{
		AssignmentId:   sheet.AssignmentID,
		Month:          sheet.Month.String(),
		Months:         monthStrings(sheet.Months),
		Scale:          toProtoScale(sheet.Scale),
		GradingEnabled: sheet.GradingEnabled,
		Editable:       sheet.Editable,
		ReadOnlyReason: sheet.ReadOnlyReason,
		Rows:           rows,
	}), nil
}

// GetEnrollmentMonthlyGrades returns one enrollment's monthly grades for its cycle.
func (h *GradesHandler) GetEnrollmentMonthlyGrades(ctx context.Context, req *connect.Request[gradesv1.GetEnrollmentMonthlyGradesRequest]) (*connect.Response[gradesv1.GetEnrollmentMonthlyGradesResponse], error) {
	got, err := h.monthly.ForEnrollment(ctx, req.Msg.GetEnrollmentId())
	if err != nil {
		return nil, connectErr(err)
	}
	grades := make([]*gradesv1.MonthlyGrade, 0, len(got.Grades))
	for _, g := range got.Grades {
		grades = append(grades, toProtoMonthlyGrade(g))
	}
	return connect.NewResponse(&gradesv1.GetEnrollmentMonthlyGradesResponse{
		CycleId:        got.CycleID,
		CycleName:      got.CycleName,
		Months:         monthStrings(got.Months),
		Scale:          toProtoScale(got.Scale),
		GradingEnabled: got.GradingEnabled,
		SubjectIds:     got.SubjectIDs,
		Grades:         grades,
	}), nil
}

func toProtoMonthlyGrade(g domain.MonthlyGrade) *gradesv1.MonthlyGrade {
	return &gradesv1.MonthlyGrade{
		EnrollmentId: g.EnrollmentID(),
		SubjectId:    g.SubjectID(),
		Month:        g.Month().String(),
		Value:        g.Value().String(),
		UpdatedAt:    timestampOrNil(g.UpdatedAt()),
	}
}

func toProtoScale(s domain.GradingScale) *gradesv1.GradingScale {
	return &gradesv1.GradingScale{
		MinHundredths:     int32(s.MinHundredths()),
		MaxHundredths:     int32(s.MaxHundredths()),
		Decimals:          int32(s.Decimals()),
		PassingHundredths: int32(s.PassingHundredths()),
	}
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
