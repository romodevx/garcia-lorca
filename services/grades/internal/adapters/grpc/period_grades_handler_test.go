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
	"github.com/garcia-lorca/services/grades/internal/usecase"
)

const (
	levelID = "lvl-1"
	gradeID = "gl-1"
	period1 = "p1"
	period2 = "p2"
)

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
		level: ports.LevelRef{ID: levelID, GradeLevelID: gradeID, GradingEnabled: true, Scale: scale},
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

// fakeEvalPeriods answers the grade's two trimesters.
type fakeEvalPeriods struct{}

func (fakeEvalPeriods) EvaluationPeriods(_ context.Context, _, _, grade string) ([]domain.EvaluationPeriod, error) {
	if grade != gradeID {
		return nil, nil
	}
	return []domain.EvaluationPeriod{
		{ID: period1, GradeLevelID: gradeID, Ordinal: 1, Name: "1.er trimestre",
			StartsOn: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC), EndsOn: time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)},
		{ID: period2, GradeLevelID: gradeID, Ordinal: 2, Name: "2.º trimestre",
			StartsOn: time.Date(2026, 12, 7, 0, 0, 0, 0, time.UTC), EndsOn: time.Date(2027, 3, 26, 0, 0, 0, 0, time.UTC)},
	}, nil
}

type fakePeriodGrades struct {
	rows     map[string]domain.PeriodGrade
	closures map[string]domain.PeriodClosure
}

func newFakePeriodGrades() *fakePeriodGrades {
	return &fakePeriodGrades{rows: map[string]domain.PeriodGrade{}, closures: map[string]domain.PeriodClosure{}}
}

func (f *fakePeriodGrades) PeriodClosures(_ context.Context, ids []string) (map[string]domain.PeriodClosure, error) {
	out := map[string]domain.PeriodClosure{}
	for _, id := range ids {
		if c, ok := f.closures[id]; ok {
			out[id] = c
		}
	}
	return out, nil
}

func (f *fakePeriodGrades) UpsertPeriodGrade(_ context.Context, g domain.PeriodGrade, lock ports.PeriodLock) (domain.PeriodGrade, error) {
	if c, ok := f.closures[lock.TermID]; ok && c.IsClosed() {
		return domain.PeriodGrade{}, domain.Conflict(domain.CodePeriodLocked, "El periodo está cerrado")
	}
	f.rows[g.EnrollmentID()+g.PeriodID()] = g
	return g, nil
}

func (f *fakePeriodGrades) DeletePeriodGrade(_ context.Context, enr, _, period string, _ ports.PeriodLock) error {
	delete(f.rows, enr+period)
	return nil
}

func (f *fakePeriodGrades) PeriodGradesForSheet(_ context.Context, ids []string, _, period string) (map[string]domain.PeriodGrade, error) {
	out := map[string]domain.PeriodGrade{}
	for _, id := range ids {
		if g, ok := f.rows[id+period]; ok {
			out[id] = g
		}
	}
	return out, nil
}

func (f *fakePeriodGrades) PeriodGradesForEnrollment(_ context.Context, enr string) ([]domain.PeriodGrade, error) {
	var out []domain.PeriodGrade
	for _, g := range f.rows {
		if g.EnrollmentID() == enr {
			out = append(out, g)
		}
	}
	return out, nil
}

func (f *fakePeriodGrades) PeriodGradesForEnrollments(context.Context, []string, []string) ([]domain.PeriodGrade, error) {
	return nil, nil
}

func (f *fakePeriodGrades) PeriodsWithGrades(_ context.Context, ids []string) ([]string, error) {
	out := []string{}
	for _, id := range ids {
		for _, g := range f.rows {
			if g.PeriodID() == id {
				out = append(out, id)
				break
			}
		}
	}
	return out, nil
}

func (f *fakePeriodGrades) CountScaleViolations(_ context.Context, ids []string, check ports.ScaleCheck) (int, error) {
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

// gradesNow is the period surface's clock: mid-October, inside the first
// trimester (the second has not started).
func gradesNow() time.Time { return time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC) }

func newPeriodGradesService(a ports.AssignmentReader, e ports.EnrollmentReader, repo ports.PeriodGradeRepository) *usecase.PeriodGradesService {
	return usecase.NewPeriodGradesService(usecase.PeriodGradesDeps{
		Grades: repo, Assignments: a, Catalog: newFakeCatalog(), Periods: fakeEvalPeriods{}, Enrollments: e,
		TenantID: testTenant, NewID: func() string { return "pg-1" }, Now: gradesNow,
	})
}

func periodGradesHandler(t *testing.T) *grpcadapter.GradesHandler {
	t.Helper()
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{enrollmentID: activeEnrollment(enrollmentID, "stu-1")}}
	return newHandler(baseAssignments(), enr, newFakeEntries())
}

func TestPeriodGradeRPCsRoundTrip(t *testing.T) {
	h := periodGradesHandler(t)
	ctx := context.Background()

	up, err := h.UpsertPeriodGrade(ctx, connect.NewRequest(&gradesv1.UpsertPeriodGradeRequest{
		AssignmentId: assignmentID, PeriodId: period1, EnrollmentId: enrollmentID, Value: "9", CapturedBy: "user-1", CapturedByName: "Gabriela Rojas",
	}))
	require.NoError(t, err)
	g := up.Msg.GetGrade()
	assert.Equal(t, enrollmentID, g.GetEnrollmentId())
	assert.Equal(t, subjectID, g.GetSubjectId())
	assert.Equal(t, period1, g.GetPeriodId())
	assert.Equal(t, "9", g.GetValue())
	assert.Equal(t, "Gabriela Rojas", g.GetCapturedByName())
	assert.NotNil(t, g.GetUpdatedAt())

	sheet, err := h.GetPeriodSheet(ctx, connect.NewRequest(&gradesv1.GetPeriodSheetRequest{AssignmentId: assignmentID}))
	require.NoError(t, err)
	m := sheet.Msg
	assert.Equal(t, period1, m.GetPeriodId(), "the period of the school's day")
	assert.Equal(t, period1, m.GetCurrentPeriodId())
	assert.Equal(t, "2026-10-15", m.GetToday())
	require.Len(t, m.GetPeriods(), 2)
	assert.Equal(t, &gradesv1.EvaluationPeriodRef{Id: period2, Ordinal: 2, Name: "2.º trimestre", StartsOn: "2026-12-07", EndsOn: "2027-03-26"}, m.GetPeriods()[1])
	assert.Equal(t, &gradesv1.GradingScale{MinHundredths: 500, MaxHundredths: 1000, Decimals: 1, PassingHundredths: 600}, m.GetScale())
	assert.True(t, m.GetGradingEnabled())
	assert.True(t, m.GetEditable())
	assert.NotNil(t, m.GetClosedPeriodIds())
	require.Len(t, m.GetRows(), 1)
	assert.Equal(t, "9", m.GetRows()[0].GetValue())
	assert.Equal(t, "stu-1", m.GetRows()[0].GetStudentId())
	assert.True(t, m.GetRows()[0].GetEditable())

	fam, err := h.GetEnrollmentPeriodGrades(ctx, connect.NewRequest(&gradesv1.GetEnrollmentPeriodGradesRequest{EnrollmentId: enrollmentID}))
	require.NoError(t, err)
	assert.Equal(t, "2026-2027", fam.Msg.GetCycleName())
	assert.Len(t, fam.Msg.GetPeriods(), 2)
	assert.Equal(t, []string{subjectID}, fam.Msg.GetSubjectIds())
	require.Len(t, fam.Msg.GetGrades(), 1)
	assert.Equal(t, period1, fam.Msg.GetGrades()[0].GetPeriodId())

	with, err := h.PeriodsWithGrades(ctx, connect.NewRequest(&gradesv1.PeriodsWithGradesRequest{PeriodIds: []string{period1, period2}}))
	require.NoError(t, err)
	assert.Equal(t, []string{period1}, with.Msg.GetPeriodIds())

	_, err = h.ClearPeriodGrade(ctx, connect.NewRequest(&gradesv1.ClearPeriodGradeRequest{AssignmentId: assignmentID, PeriodId: period1, EnrollmentId: enrollmentID}))
	require.NoError(t, err)
	sheet, err = h.GetPeriodSheet(ctx, connect.NewRequest(&gradesv1.GetPeriodSheetRequest{AssignmentId: assignmentID, PeriodId: period1}))
	require.NoError(t, err)
	assert.Empty(t, sheet.Msg.GetRows()[0].GetValue(), "a cleared cell reads empty")
	assert.Nil(t, sheet.Msg.GetRows()[0].GetUpdatedAt())
}

// A refusal's sentence and machine code reach the edge (X-Error-Code meta).
func TestPeriodGradeRefusalsCarryTheirCode(t *testing.T) {
	h := periodGradesHandler(t)
	ctx := context.Background()
	cases := []struct {
		name    string
		period  string
		value   string
		code    string
		message string
	}{
		{"a decimal", period1, "9.5", "GRADE_NOT_INTEGER", "Solo se permiten calificaciones enteras"},
		{"out of range", period1, "11", "GRADE_OUT_OF_RANGE", "La calificación debe estar entre 5 y 10"},
		{"not started", period2, "9", "PERIOD_NOT_STARTED", "Aún no empieza ese periodo"},
		{"another grade's period", "p-other", "9", "PERIOD_NOT_IN_GRADE", "Ese periodo no pertenece al grado del grupo"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := h.UpsertPeriodGrade(ctx, connect.NewRequest(&gradesv1.UpsertPeriodGradeRequest{
				AssignmentId: assignmentID, PeriodId: c.period, EnrollmentId: enrollmentID, Value: c.value, CapturedBy: "user-1", SchoolWide: true,
			}))
			var ce *connect.Error
			require.ErrorAs(t, err, &ce)
			assert.Equal(t, connect.CodeInvalidArgument, ce.Code())
			assert.Equal(t, c.code, ce.Meta().Get(grpcadapter.ErrorCodeMeta))
			assert.Equal(t, c.message, ce.Message())
		})
	}

	_, err := h.GetPeriodSheet(ctx, connect.NewRequest(&gradesv1.GetPeriodSheetRequest{AssignmentId: "nope"}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	_, err = h.GetEnrollmentPeriodGrades(ctx, connect.NewRequest(&gradesv1.GetEnrollmentPeriodGradesRequest{EnrollmentId: "ghost"}))
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, connect.CodeNotFound, ce.Code())
	assert.Empty(t, ce.Meta().Get(grpcadapter.ErrorCodeMeta), "an uncoded error carries no code")
}

// A closed term locks its period: 409 PERIOD_LOCKED, and the sheet lists it.
func TestPeriodSheetCarriesClosedPeriods(t *testing.T) {
	asg := baseAssignments()
	asg.terms["term-t1"] = ports.TermRef{ID: "term-t1", CycleID: cycleID, LevelID: levelID, Ordinal: 1}
	repo := newFakePeriodGrades()
	closed, err := domain.NewOpenPeriod("term-t1", cycleID, levelID).Close(domain.Actor{ID: "dir"}, issuedAt, nil)
	require.NoError(t, err)
	repo.closures["term-t1"] = closed
	enr := &fakeEnrollments{byID: map[string]ports.EnrollmentRef{enrollmentID: activeEnrollment(enrollmentID, "stu-1")}}
	h := grpcadapter.NewGradesHandler(nil, newPeriodGradesService(asg, enr, repo), &stubPeriods{}, usecase.NewHealthService("grades"))

	res, err := h.GetPeriodSheet(context.Background(), connect.NewRequest(&gradesv1.GetPeriodSheetRequest{AssignmentId: assignmentID, PeriodId: period1}))
	require.NoError(t, err)
	assert.Equal(t, []string{period1}, res.Msg.GetClosedPeriodIds())
	assert.False(t, res.Msg.GetEditable())
	assert.Equal(t, "El periodo está cerrado", res.Msg.GetReadOnlyReason())

	_, err = h.UpsertPeriodGrade(context.Background(), connect.NewRequest(&gradesv1.UpsertPeriodGradeRequest{
		AssignmentId: assignmentID, PeriodId: period1, EnrollmentId: enrollmentID, Value: "9", CapturedBy: "u", SchoolWide: true,
	}))
	assert.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(err))
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, "PERIOD_LOCKED", ce.Meta().Get(grpcadapter.ErrorCodeMeta))
}

// school_wide lifts the previous-group rule; the sheet marks the row for a
// teacher.
func TestPeriodGradeRPCsHonourThePreviousGroupRule(t *testing.T) {
	enr := &fakeEnrollments{
		byID:      map[string]ports.EnrollmentRef{enrollmentID: activeEnrollment(enrollmentID, "stu-1")},
		transfers: []ports.TransferRef{{EnrollmentID: enrollmentID, ToGroupID: groupID, MovedAt: time.Date(2026, 12, 10, 15, 0, 0, 0, time.UTC)}},
	}
	svc := usecase.NewPeriodGradesService(usecase.PeriodGradesDeps{
		Grades: newFakePeriodGrades(), Assignments: baseAssignments(), Catalog: newFakeCatalog(), Periods: fakeEvalPeriods{},
		Enrollments: enr, TenantID: testTenant, NewID: func() string { return "pg-1" },
		Now: func() time.Time { return time.Date(2027, 1, 20, 15, 0, 0, 0, time.UTC) },
	})
	h := grpcadapter.NewGradesHandler(nil, svc, &stubPeriods{}, usecase.NewHealthService("grades"))
	ctx := context.Background()
	req := &gradesv1.UpsertPeriodGradeRequest{AssignmentId: assignmentID, PeriodId: period1, EnrollmentId: enrollmentID, Value: "9", CapturedBy: "user-1"}

	_, err := h.UpsertPeriodGrade(ctx, connect.NewRequest(req))
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, "PERIOD_PREVIOUS_GROUP", ce.Meta().Get(grpcadapter.ErrorCodeMeta))

	req.SchoolWide = true
	_, err = h.UpsertPeriodGrade(ctx, connect.NewRequest(req))
	require.NoError(t, err, "the director writes any period")

	sheet, err := h.GetPeriodSheet(ctx, connect.NewRequest(&gradesv1.GetPeriodSheetRequest{AssignmentId: assignmentID, PeriodId: period1}))
	require.NoError(t, err)
	row := sheet.Msg.GetRows()[0]
	assert.False(t, row.GetEditable())
	assert.Equal(t, "Ese periodo corresponde al grupo anterior del alumno", row.GetReadOnlyReason())
	assert.Equal(t, "9", row.GetValue(), "the earlier period is still readable")

	director, err := h.GetPeriodSheet(ctx, connect.NewRequest(&gradesv1.GetPeriodSheetRequest{AssignmentId: assignmentID, PeriodId: period1, SchoolWide: true}))
	require.NoError(t, err)
	assert.True(t, director.Msg.GetRows()[0].GetEditable())
}

// Monthly capture is retired (T15): its RPCs are no longer served, so nothing
// can write monthly_grades any more.
func TestMonthlyRPCsAreRetired(t *testing.T) {
	h := periodGradesHandler(t)
	ctx := context.Background()
	_, err := h.UpsertMonthlyGrade(ctx, connect.NewRequest(&gradesv1.UpsertMonthlyGradeRequest{
		AssignmentId: assignmentID, Month: "2026-09", EnrollmentId: enrollmentID, Value: "9", CapturedBy: "user-1",
	}))
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
	_, err = h.ClearMonthlyGrade(ctx, connect.NewRequest(&gradesv1.ClearMonthlyGradeRequest{AssignmentId: assignmentID, Month: "2026-09", EnrollmentId: enrollmentID}))
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
	_, err = h.GetMonthlySheet(ctx, connect.NewRequest(&gradesv1.GetMonthlySheetRequest{AssignmentId: assignmentID}))
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
	_, err = h.GetEnrollmentMonthlyGrades(ctx, connect.NewRequest(&gradesv1.GetEnrollmentMonthlyGradesRequest{EnrollmentId: enrollmentID}))
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}

func TestCountScaleViolationsRPC(t *testing.T) {
	h := periodGradesHandler(t)
	ctx := context.Background()
	_, err := h.UpsertPeriodGrade(ctx, connect.NewRequest(&gradesv1.UpsertPeriodGradeRequest{
		AssignmentId: assignmentID, PeriodId: period1, EnrollmentId: enrollmentID, Value: "5", CapturedBy: "user-1",
	}))
	require.NoError(t, err)

	tests := []struct {
		name     string
		req      *gradesv1.CountScaleViolationsRequest
		want     int32
		wantCode connect.Code
	}{
		{"loosen", &gradesv1.CountScaleViolationsRequest{LevelId: levelID, GradingEnabled: true, Scale: &gradesv1.GradingScale{MinHundredths: 0, MaxHundredths: 1000, PassingHundredths: 600, Decimals: 2}}, 0, 0},
		{"raise the minimum", &gradesv1.CountScaleViolationsRequest{LevelId: levelID, GradingEnabled: true, Scale: &gradesv1.GradingScale{MinHundredths: 600, MaxHundredths: 1000, PassingHundredths: 700, Decimals: 0}}, 1, 0},
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
