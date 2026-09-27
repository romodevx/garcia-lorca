//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

// Neutral cross-service references for the monthly tests (disjoint from the
// grade_entries fixtures: the container is shared and not reset).
const (
	mEnrollmentA  = "e1000000-0000-0000-0000-000000000001"
	mEnrollmentB  = "e1000000-0000-0000-0000-000000000002"
	mEnrollmentC  = "e1000000-0000-0000-0000-000000000003"
	mEnrollmentRL = "e1000000-0000-0000-0000-000000000004"
	mSubject      = "51000000-0000-0000-0000-000000000001"
	mSubjectTwo   = "51000000-0000-0000-0000-000000000002"
	mTeacher      = "61000000-0000-0000-0000-000000000001"
	mTeacherTwo   = "61000000-0000-0000-0000-000000000002"
	mUser         = "71000000-0000-0000-0000-000000000001"
	mDirector     = "71000000-0000-0000-0000-000000000002"
)

func month(t *testing.T, raw string) domain.Month {
	t.Helper()
	m, err := domain.ParseMonth(raw)
	require.NoError(t, err)
	return m
}

func monthlyGrade(t *testing.T, id, tenant, enrollment, subject, teacher, capturedBy, rawMonth, value string) domain.MonthlyGrade {
	t.Helper()
	scale, err := domain.NewGradingScale(0, 10000, 600, 2)
	require.NoError(t, err)
	v, err := scale.ParseValue(value)
	require.NoError(t, err)
	g, err := domain.NewMonthlyGrade(domain.MonthlyGradeFields{
		ID: id, TenantID: tenant, EnrollmentID: enrollment, SubjectID: subject, TeacherID: teacher,
		CapturedBy: capturedBy, Month: month(t, rawMonth), Value: v, UpdatedAt: captureClock,
	})
	require.NoError(t, err)
	return g
}

// countMonthly counts monthly_grades rows as the non-owner app role with the
// tenant bound, so it exercises RLS.
func countMonthly(t *testing.T, tenant, enrollment string) int64 {
	t.Helper()
	db, err := open(appDSN)
	require.NoError(t, err)
	var n int64
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenant).Error; err != nil {
			return err
		}
		return tx.Table("monthly_grades").Where("enrollment_id = ?", enrollment).Count(&n).Error
	}))
	return n
}

// The upsert edits the cell in place: a second write on the same
// (enrollment, subject, month) keeps the row id, replaces value, teacher and
// captured_by, and bumps updated_at — the path every autosave after the first
// walks. A 9.95 round-trips exactly (no float drift through numeric(5,2)).
func TestMonthlyUpsertTwiceEditsInPlace(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()

	first, err := repo.UpsertMonthlyGrade(ctx, monthlyGrade(t, "a1000000-0000-0000-0000-000000000001", tenantA, mEnrollmentA, mSubject, mTeacher, mUser, "2026-09", "9.95"))
	require.NoError(t, err)
	assert.Equal(t, 995, first.Value().Hundredths())
	assert.Equal(t, "2026-09", first.Month().String())
	assert.False(t, first.UpdatedAt().IsZero())

	time.Sleep(10 * time.Millisecond) // now() must move between the two transactions
	second, err := repo.UpsertMonthlyGrade(ctx, monthlyGrade(t, "a1000000-0000-0000-0000-000000000099", tenantA, mEnrollmentA, mSubject, mTeacherTwo, mDirector, "2026-09", "7"))
	require.NoError(t, err)
	assert.Equal(t, first.ID(), second.ID(), "the upsert must keep the row")
	assert.Equal(t, 700, second.Value().Hundredths())
	assert.Equal(t, mTeacherTwo, second.TeacherID())
	assert.Equal(t, mDirector, second.CapturedBy())
	assert.True(t, second.UpdatedAt().After(first.UpdatedAt()), "updated_at must move on a correction")
	assert.Equal(t, int64(1), countMonthly(t, tenantA, mEnrollmentA))

	// Another month is another cell.
	_, err = repo.UpsertMonthlyGrade(ctx, monthlyGrade(t, "a1000000-0000-0000-0000-000000000002", tenantA, mEnrollmentA, mSubject, mTeacher, mUser, "2026-10", "8.5"))
	require.NoError(t, err)
	all, err := repo.MonthlyGradesForEnrollment(ctx, mEnrollmentA)
	require.NoError(t, err)
	require.Len(t, all, 2)
	assert.Equal(t, "2026-09", all[0].Month().String(), "ordered by month")
	assert.Equal(t, "8.5", all[1].Value().String())
}

// The sheet is one batched read: only the enrollments that have a grade for
// that subject and month come back, keyed by enrollment id.
func TestMonthlyGradesForSheet(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	_, err := repo.UpsertMonthlyGrade(ctx, monthlyGrade(t, "a1000000-0000-0000-0000-000000000010", tenantA, mEnrollmentB, mSubject, mTeacher, mUser, "2026-11", "6"))
	require.NoError(t, err)
	_, err = repo.UpsertMonthlyGrade(ctx, monthlyGrade(t, "a1000000-0000-0000-0000-000000000011", tenantA, mEnrollmentB, mSubjectTwo, mTeacher, mUser, "2026-11", "10"))
	require.NoError(t, err)

	got, err := repo.MonthlyGradesForSheet(ctx, []string{mEnrollmentB, mEnrollmentC}, mSubject, month(t, "2026-11"))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "6", got[mEnrollmentB].Value().String())

	empty, err := repo.MonthlyGradesForSheet(ctx, nil, mSubject, month(t, "2026-11"))
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestMonthlyDeleteIsIdempotent(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	_, err := repo.UpsertMonthlyGrade(ctx, monthlyGrade(t, "a1000000-0000-0000-0000-000000000020", tenantA, mEnrollmentC, mSubject, mTeacher, mUser, "2026-12", "9"))
	require.NoError(t, err)

	require.NoError(t, repo.DeleteMonthlyGrade(ctx, mEnrollmentC, mSubject, month(t, "2026-12")))
	assert.Equal(t, int64(0), countMonthly(t, tenantA, mEnrollmentC))
	require.NoError(t, repo.DeleteMonthlyGrade(ctx, mEnrollmentC, mSubject, month(t, "2026-12")), "deleting an empty cell is not an error")
}

// The table refuses a month that is not a first day and a value outside 0–100,
// even when a write bypasses the domain.
func TestMonthlyChecks(t *testing.T) {
	db, err := open(appDSN)
	require.NoError(t, err)
	for _, stmt := range []struct{ month, value string }{{"2026-09-15", "9"}, {"2026-09-01", "100.5"}, {"2026-09-01", "-1"}} {
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantA).Error; err != nil {
				return err
			}
			return tx.Exec("INSERT INTO monthly_grades (tenant_id, enrollment_id, subject_id, teacher_id, month, value, captured_by) VALUES (?, ?, ?, ?, ?, ?, ?)",
				tenantA, mEnrollmentC, mSubjectTwo, mTeacher, stmt.month, stmt.value, mUser).Error
		})
		assert.Error(t, err, "month %s value %s must be refused", stmt.month, stmt.value)
	}
}

// RLS isolation for the non-owner app role: tenant A can neither read, change
// nor delete tenant B's monthly grade.
func TestRLSIsolatesMonthlyGrades(t *testing.T) {
	ctx := context.Background()
	b, err := appRepo(t, tenantB).UpsertMonthlyGrade(ctx, monthlyGrade(t, "b1000000-0000-0000-0000-000000000001", tenantB, mEnrollmentRL, mSubject, mTeacher, mUser, "2026-09", "7"))
	require.NoError(t, err)

	a := appRepo(t, tenantA)
	seen, err := a.MonthlyGradesForEnrollment(ctx, mEnrollmentRL)
	require.NoError(t, err)
	assert.Empty(t, seen, "tenant A read tenant B's grade")
	sheet, err := a.MonthlyGradesForSheet(ctx, []string{mEnrollmentRL}, mSubject, month(t, "2026-09"))
	require.NoError(t, err)
	assert.Empty(t, sheet)

	require.NoError(t, a.DeleteMonthlyGrade(ctx, mEnrollmentRL, mSubject, month(t, "2026-09")))
	_, err = a.UpsertMonthlyGrade(ctx, monthlyGrade(t, "a1000000-0000-0000-0000-0000000000a1", tenantA, mEnrollmentRL, mSubject, mTeacher, mUser, "2026-09", "3"))
	require.NoError(t, err)

	after, err := appRepo(t, tenantB).MonthlyGradesForEnrollment(ctx, mEnrollmentRL)
	require.NoError(t, err)
	require.Len(t, after, 1)
	assert.Equal(t, b.ID(), after[0].ID())
	assert.Equal(t, 700, after[0].Value().Hundredths(), "tenant A's write or delete leaked into tenant B")
	assert.Equal(t, int64(1), countMonthly(t, tenantA, mEnrollmentRL))
}

// CountScaleViolations counts, in one tenant-bound query, the named
// enrollments' grades a proposed scale refuses: out of range, too many
// decimals, or — disabling grading — every grade. Another tenant's grades never
// count.
func TestCountScaleViolations(t *testing.T) {
	const (
		enrV1 = "e1000000-0000-0000-0000-000000000051"
		enrV2 = "e1000000-0000-0000-0000-000000000052"
	)
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	for i, c := range []struct{ enr, month, value string }{
		{enrV1, "2026-09", "9.5"}, {enrV1, "2026-10", "6"}, {enrV2, "2026-09", "10"}, {enrV2, "2026-10", "7.25"},
	} {
		id := "a1000000-0000-0000-0000-00000000005" + string(rune('0'+i))
		_, err := repo.UpsertMonthlyGrade(ctx, monthlyGrade(t, id, tenantA, c.enr, mSubject, mTeacher, mUser, c.month, c.value))
		require.NoError(t, err)
	}
	_, err := appRepo(t, tenantB).UpsertMonthlyGrade(ctx, monthlyGrade(t, "a1000000-0000-0000-0000-000000000059", tenantB, enrV1, mSubject, mTeacher, mUser, "2026-11", "1"))
	require.NoError(t, err)

	scale := func(minH, maxH, passH, dec int) domain.GradingScale {
		s, err := domain.NewGradingScale(minH, maxH, passH, dec)
		require.NoError(t, err)
		return s
	}
	both := []string{enrV1, enrV2}
	tests := []struct {
		name  string
		ids   []string
		check ports.ScaleCheck
		want  int
	}{
		{"loosen", both, ports.ScaleCheck{GradingEnabled: true, Scale: scale(0, 1000, 600, 2)}, 0},
		{"max 9.5", both, ports.ScaleCheck{GradingEnabled: true, Scale: scale(500, 950, 600, 2)}, 1},
		{"min 7", both, ports.ScaleCheck{GradingEnabled: true, Scale: scale(700, 1000, 800, 2)}, 1},
		{"one decimal", both, ports.ScaleCheck{GradingEnabled: true, Scale: scale(500, 1000, 600, 1)}, 1},
		{"whole numbers", both, ports.ScaleCheck{GradingEnabled: true, Scale: scale(500, 1000, 600, 0)}, 2},
		{"disable", both, ports.ScaleCheck{}, 4},
		{"only the named", []string{enrV2}, ports.ScaleCheck{}, 2},
		{"none named", nil, ports.ScaleCheck{}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := repo.CountScaleViolations(ctx, tt.ids, tt.check)
			require.NoError(t, err)
			assert.Equal(t, tt.want, n)
		})
	}
}
