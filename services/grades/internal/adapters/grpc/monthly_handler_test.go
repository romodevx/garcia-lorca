package grpcadapter_test

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	grpcadapter "github.com/garcia-lorca/services/grades/internal/adapters/grpc"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

const levelID = "lvl-1"

type fakeCatalog struct {
	level ports.LevelRef
	cycle ports.CycleRef
}

func newFakeCatalog() *fakeCatalog {
	scale, err := domain.NewGradingScale(500, 1000, 600, 1)
	if err != nil {
		panic(err)
	}
	return &fakeCatalog{
		level: ports.LevelRef{ID: levelID, GradingEnabled: true, Scale: scale},
		cycle: ports.CycleRef{ID: cycleID, Name: "2026-2027",
			StartsOn: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), EndsOn: time.Date(2027, 7, 1, 0, 0, 0, 0, time.UTC)},
	}
}

func (f *fakeCatalog) LevelOfGroup(_ context.Context, g string) (ports.LevelRef, error) {
	if g != groupID {
		return ports.LevelRef{}, glerrors.ErrNotFound
	}
	return f.level, nil
}

func (f *fakeCatalog) LevelIDOfGroup(ctx context.Context, g string) (ports.LevelKey, error) {
	l, err := f.LevelOfGroup(ctx, g)
	return ports.LevelKey{ID: l.ID, Deleting: l.Deleting}, err
}

func (f *fakeCatalog) ActiveCycle(_ context.Context) (ports.CycleRef, bool, error) {
	return f.cycle, true, nil
}

func (f *fakeCatalog) GroupsOfLevel(_ context.Context, _, level string) ([]string, error) {
	if level != levelID {
		return nil, nil
	}
	return []string{groupID}, nil
}

func (f *fakeCatalog) CycleByID(_ context.Context, id string) (ports.CycleRef, error) {
	if id != cycleID {
		return ports.CycleRef{}, glerrors.ErrNotFound
	}
	return f.cycle, nil
}

func (f *fakeCatalog) AssignmentsByGroup(_ context.Context, _, _ string) ([]ports.AssignmentRef, error) {
	return []ports.AssignmentRef{{ID: assignmentID, GroupID: groupID, SubjectID: subjectID, TeacherID: teacherID, CycleID: cycleID}}, nil
}

func (f *fakeCatalog) LevelCatalog(context.Context, string, string) (ports.LevelCatalog, error) {
	return ports.LevelCatalog{}, glerrors.ErrNotFound
}

type fakeMonthly struct {
	rows     map[string]domain.MonthlyGrade
	closures map[string]domain.PeriodClosure
}

func newFakeMonthly() *fakeMonthly {
	return &fakeMonthly{rows: map[string]domain.MonthlyGrade{}, closures: map[string]domain.PeriodClosure{}}
}

func (f *fakeMonthly) PeriodClosures(_ context.Context, ids []string) (map[string]domain.PeriodClosure, error) {
	out := map[string]domain.PeriodClosure{}
	for _, id := range ids {
		if c, ok := f.closures[id]; ok {
			out[id] = c
		}
	}
	return out, nil
}

func (f *fakeMonthly) MonthlyGradesForEnrollments(context.Context, []string, []domain.Month) ([]domain.MonthlyGrade, error) {
	return nil, nil
}

func (f *fakeMonthly) UpsertMonthlyGrade(_ context.Context, g domain.MonthlyGrade, _ ports.PeriodLock) (domain.MonthlyGrade, error) {
	f.rows[g.EnrollmentID()+g.Month().String()] = g
	return g, nil
}

func (f *fakeMonthly) DeleteMonthlyGrade(_ context.Context, enr, _ string, m domain.Month, _ ports.PeriodLock) error {
	delete(f.rows, enr+m.String())
	return nil
}

func (f *fakeMonthly) MonthlyGradesForSheet(_ context.Context, ids []string, _ string, m domain.Month) (map[string]domain.MonthlyGrade, error) {
	out := map[string]domain.MonthlyGrade{}
	for _, id := range ids {
		if g, ok := f.rows[id+m.String()]; ok {
			out[id] = g
		}
	}
	return out, nil
}

func (f *fakeMonthly) CountScaleViolations(_ context.Context, ids []string, check ports.ScaleCheck) (int, error) {
	n := 0
	for _, id := range ids {
		for _, g := range f.rows {
			if g.EnrollmentID() == id && (!check.GradingEnabled || !check.Scale.Admits(g.Value())) {
				n++
			}
		}
	}
	return n, nil
}

func (f *fakeMonthly) MonthlyGradesForEnrollment(_ context.Context, enr string) ([]domain.MonthlyGrade, error) {
	var out []domain.MonthlyGrade
	for _, g := range f.rows {
		if g.EnrollmentID() == enr {
			out = append(out, g)
		}
	}
	return out, nil
}

func monthlyHandler(t *testing.T) *grpcadapter.GradesHandler {
	t.Helper()
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{enrollmentID: activeEnrollment(enrollmentID, "stu-1")}}
	return newHandler(baseAssignments(), enr, newFakeEntries())
}

func TestMonthlyRPCsRoundTrip(t *testing.T) {
	h := monthlyHandler(t)
	ctx := context.Background()

	up, err := h.UpsertMonthlyGrade(ctx, connect.NewRequest(&gradesv1.UpsertMonthlyGradeRequest{
		AssignmentId: assignmentID, Month: "2026-09", EnrollmentId: enrollmentID, Value: "9.0", CapturedBy: "user-1",
	}))
	require.NoError(t, err)
	g := up.Msg.GetGrade()
	assert.Equal(t, enrollmentID, g.GetEnrollmentId())
	assert.Equal(t, subjectID, g.GetSubjectId())
	assert.Equal(t, "2026-09", g.GetMonth())
	assert.Equal(t, "9", g.GetValue())
	assert.NotNil(t, g.GetUpdatedAt())

	sheet, err := h.GetMonthlySheet(ctx, connect.NewRequest(&gradesv1.GetMonthlySheetRequest{AssignmentId: assignmentID, Month: "2026-09"}))
	require.NoError(t, err)
	m := sheet.Msg
	assert.Equal(t, "2026-09", m.GetMonth())
	assert.Len(t, m.GetMonths(), 12)
	assert.Equal(t, "2026-08", m.GetMonths()[0])
	assert.Equal(t, &gradesv1.GradingScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600}, m.GetScale())
	assert.True(t, m.GetGradingEnabled())
	assert.True(t, m.GetEditable())
	require.Len(t, m.GetRows(), 1)
	assert.Equal(t, "9", m.GetRows()[0].GetValue())
	assert.Equal(t, "stu-1", m.GetRows()[0].GetStudentId())
	assert.NotNil(t, m.GetRows()[0].GetUpdatedAt())

	fam, err := h.GetEnrollmentMonthlyGrades(ctx, connect.NewRequest(&gradesv1.GetEnrollmentMonthlyGradesRequest{EnrollmentId: enrollmentID}))
	require.NoError(t, err)
	assert.Equal(t, "2026-2027", fam.Msg.GetCycleName())
	assert.Equal(t, []string{subjectID}, fam.Msg.GetSubjectIds())
	require.Len(t, fam.Msg.GetGrades(), 1)

	_, err = h.ClearMonthlyGrade(ctx, connect.NewRequest(&gradesv1.ClearMonthlyGradeRequest{AssignmentId: assignmentID, Month: "2026-09", EnrollmentId: enrollmentID}))
	require.NoError(t, err)
	sheet, err = h.GetMonthlySheet(ctx, connect.NewRequest(&gradesv1.GetMonthlySheetRequest{AssignmentId: assignmentID, Month: "2026-09"}))
	require.NoError(t, err)
	assert.Empty(t, sheet.Msg.GetRows()[0].GetValue(), "a cleared cell reads empty")
	assert.Nil(t, sheet.Msg.GetRows()[0].GetUpdatedAt())
}

func TestMonthlyRPCErrorCodes(t *testing.T) {
	h := monthlyHandler(t)
	ctx := context.Background()
	_, err := h.UpsertMonthlyGrade(ctx, connect.NewRequest(&gradesv1.UpsertMonthlyGradeRequest{
		AssignmentId: assignmentID, Month: "2026-09", EnrollmentId: enrollmentID, Value: "9.95", CapturedBy: "user-1",
	}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, "Solo se permite 1 decimal", ce.Message(), "the user's sentence reaches the edge")

	_, err = h.GetMonthlySheet(ctx, connect.NewRequest(&gradesv1.GetMonthlySheetRequest{AssignmentId: "nope"}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	_, err = h.ClearMonthlyGrade(ctx, connect.NewRequest(&gradesv1.ClearMonthlyGradeRequest{AssignmentId: assignmentID, Month: "2030-01", EnrollmentId: enrollmentID}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = h.GetEnrollmentMonthlyGrades(ctx, connect.NewRequest(&gradesv1.GetEnrollmentMonthlyGradesRequest{EnrollmentId: "ghost"}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

// A refusal's machine code rides as the X-Error-Code error metadata, which the
// gateway forwards as a response header.
func TestMonthlyRefusalsCarryTheirCodeAsMetadata(t *testing.T) {
	h := monthlyHandler(t)
	_, err := h.UpsertMonthlyGrade(context.Background(), connect.NewRequest(&gradesv1.UpsertMonthlyGradeRequest{
		AssignmentId: assignmentID, Month: "2026-09", EnrollmentId: enrollmentID, Value: "9.95", CapturedBy: "user-1",
	}))
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, "GRADE_DECIMALS", ce.Meta().Get("X-Error-Code"))

	_, err = h.ClearMonthlyGrade(context.Background(), connect.NewRequest(&gradesv1.ClearMonthlyGradeRequest{
		AssignmentId: assignmentID, Month: "2026-11", EnrollmentId: enrollmentID, SchoolWide: true,
	}))
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, connect.CodeInvalidArgument, ce.Code())
	assert.Equal(t, "MONTH_IN_FUTURE", ce.Meta().Get("X-Error-Code"), "a future month is refused even school-wide")

	_, err = h.GetEnrollmentMonthlyGrades(context.Background(), connect.NewRequest(&gradesv1.GetEnrollmentMonthlyGradesRequest{EnrollmentId: "ghost"}))
	require.ErrorAs(t, err, &ce)
	assert.Empty(t, ce.Meta().Get("X-Error-Code"), "an uncoded error carries no code")
}

// school_wide lifts the previous-group rule; the sheet marks the row for a
// teacher.
func TestMonthlyRPCsHonourThePreviousGroupRule(t *testing.T) {
	enr := &fakeEnrollments{
		byID:      map[string]ports.EnrollmentRef{enrollmentID: activeEnrollment(enrollmentID, "stu-1")},
		transfers: []ports.TransferRef{{EnrollmentID: enrollmentID, ToGroupID: groupID, MovedAt: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)}},
	}
	h := newHandler(baseAssignments(), enr, newFakeEntries())
	ctx := context.Background()
	req := &gradesv1.UpsertMonthlyGradeRequest{AssignmentId: assignmentID, Month: "2026-09", EnrollmentId: enrollmentID, Value: "9", CapturedBy: "user-1"}

	_, err := h.UpsertMonthlyGrade(ctx, connect.NewRequest(req))
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, connect.CodeInvalidArgument, ce.Code())
	assert.Equal(t, "Ese mes corresponde al grupo anterior del alumno", ce.Message())
	assert.Equal(t, "MONTH_PREVIOUS_GROUP", ce.Meta().Get("X-Error-Code"))

	_, err = h.ClearMonthlyGrade(ctx, connect.NewRequest(&gradesv1.ClearMonthlyGradeRequest{AssignmentId: assignmentID, Month: "2026-09", EnrollmentId: enrollmentID}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	req.SchoolWide = true
	_, err = h.UpsertMonthlyGrade(ctx, connect.NewRequest(req))
	require.NoError(t, err, "the director writes any month")

	sheet, err := h.GetMonthlySheet(ctx, connect.NewRequest(&gradesv1.GetMonthlySheetRequest{AssignmentId: assignmentID, Month: "2026-09"}))
	require.NoError(t, err)
	row := sheet.Msg.GetRows()[0]
	assert.False(t, row.GetEditable())
	assert.Equal(t, "Ese mes corresponde al grupo anterior del alumno", row.GetReadOnlyReason())
	assert.Equal(t, "9", row.GetValue(), "the earlier month is still readable")

	director, err := h.GetMonthlySheet(ctx, connect.NewRequest(&gradesv1.GetMonthlySheetRequest{AssignmentId: assignmentID, Month: "2026-09", SchoolWide: true}))
	require.NoError(t, err)
	assert.True(t, director.Msg.GetRows()[0].GetEditable())
}

func TestCountScaleViolationsRPC(t *testing.T) {
	h := monthlyHandler(t)
	ctx := context.Background()
	_, err := h.UpsertMonthlyGrade(ctx, connect.NewRequest(&gradesv1.UpsertMonthlyGradeRequest{
		AssignmentId: assignmentID, Month: "2026-09", EnrollmentId: enrollmentID, Value: "9.5", CapturedBy: "user-1",
	}))
	require.NoError(t, err)

	tests := []struct {
		name     string
		req      *gradesv1.CountScaleViolationsRequest
		want     int32
		wantCode connect.Code
	}{
		{"loosen", &gradesv1.CountScaleViolationsRequest{LevelId: levelID, GradingEnabled: true, Scale: &gradesv1.GradingScale{MinHundredths: 0, MaxHundredths: 1000, PassingHundredths: 600, Decimals: 2}}, 0, 0},
		{"tighten to whole numbers", &gradesv1.CountScaleViolationsRequest{LevelId: levelID, GradingEnabled: true, Scale: &gradesv1.GradingScale{MinHundredths: 500, MaxHundredths: 1000, PassingHundredths: 600, Decimals: 0}}, 1, 0},
		{"disable", &gradesv1.CountScaleViolationsRequest{LevelId: levelID}, 1, 0},
		{"inconsistent scale", &gradesv1.CountScaleViolationsRequest{LevelId: levelID, GradingEnabled: true, Scale: &gradesv1.GradingScale{MinHundredths: 900, MaxHundredths: 500}}, 0, connect.CodeInvalidArgument},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := h.CountScaleViolations(ctx, connect.NewRequest(tt.req))
			if tt.wantCode != 0 {
				assert.Equal(t, tt.wantCode, connect.CodeOf(err))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, res.Msg.GetViolations())
		})
	}
}
