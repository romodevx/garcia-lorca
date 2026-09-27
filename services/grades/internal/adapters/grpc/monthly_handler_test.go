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

func (f *fakeCatalog) CycleByID(_ context.Context, id string) (ports.CycleRef, error) {
	if id != cycleID {
		return ports.CycleRef{}, glerrors.ErrNotFound
	}
	return f.cycle, nil
}

func (f *fakeCatalog) AssignmentsByGroup(_ context.Context, _, _ string) ([]ports.AssignmentRef, error) {
	return []ports.AssignmentRef{{ID: assignmentID, GroupID: groupID, SubjectID: subjectID, TeacherID: teacherID, CycleID: cycleID}}, nil
}

type fakeMonthly struct {
	rows map[string]domain.MonthlyGrade
}

func newFakeMonthly() *fakeMonthly { return &fakeMonthly{rows: map[string]domain.MonthlyGrade{}} }

func (f *fakeMonthly) UpsertMonthlyGrade(_ context.Context, g domain.MonthlyGrade) (domain.MonthlyGrade, error) {
	f.rows[g.EnrollmentID()+g.Month().String()] = g
	return g, nil
}

func (f *fakeMonthly) DeleteMonthlyGrade(_ context.Context, enr, _ string, m domain.Month) error {
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
