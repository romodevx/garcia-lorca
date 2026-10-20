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

// Neutral cross-service references for the period-grade tests (disjoint from
// the grade_entries fixtures: the container is shared and not reset).
const (
	mEnrollmentA  = "e1000000-0000-0000-0000-000000000001"
	mEnrollmentB  = "e1000000-0000-0000-0000-000000000002"
	mEnrollmentC  = "e1000000-0000-0000-0000-000000000003"
	mEnrollmentRL = "e1000000-0000-0000-0000-000000000004"
	mSubject      = "51000000-0000-0000-0000-000000000001"
	mSubjectTwo   = "51000000-0000-0000-0000-000000000002"
	mUser         = "71000000-0000-0000-0000-000000000001"
	mDirector     = "71000000-0000-0000-0000-000000000002"
	mPeriodOne    = "0e000000-0000-0000-0000-000000000001"
	mPeriodTwo    = "0e000000-0000-0000-0000-000000000002"
	mPeriodThree  = "0e000000-0000-0000-0000-000000000003"
)

func periodGrade(t *testing.T, id, tenant, enrollment, subject, period, capturedBy, value string) domain.PeriodGrade {
	t.Helper()
	scale, err := domain.NewGradingScale(0, 10000, 600, 0)
	require.NoError(t, err)
	v, err := scale.ParseWholeValue(value)
	require.NoError(t, err)
	g, err := domain.NewPeriodGrade(domain.PeriodGradeFields{
		ID: id, TenantID: tenant, EnrollmentID: enrollment, SubjectID: subject, PeriodID: period,
		CapturedBy: capturedBy, CapturedByName: "Gabriela Rojas", Value: v, UpdatedAt: captureClock,
	})
	require.NoError(t, err)
	return g
}

// countPeriodGrades counts period_grades rows as the non-owner app role with
// the tenant bound, so it exercises RLS.
func countPeriodGrades(t *testing.T, tenant, enrollment string) int64 {
	t.Helper()
	db, err := open(appDSN)
	require.NoError(t, err)
	var n int64
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenant).Error; err != nil {
			return err
		}
		return tx.Table("period_grades").Where("enrollment_id = ?", enrollment).Count(&n).Error
	}))
	return n
}

// The upsert edits the cell in place: a second write on the same
// (enrollment, subject, period) keeps the row id, replaces value and who
// captured it, and bumps updated_at — the path every autosave after the first
// walks. The value round-trips exactly through numeric (no float).
func TestPeriodGradeUpsertTwiceEditsInPlace(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()

	first, err := repo.UpsertPeriodGrade(ctx, periodGrade(t, "a1000000-0000-0000-0000-000000000001", tenantA, mEnrollmentA, mSubject, mPeriodOne, mUser, "10"), ports.PeriodLock{})
	require.NoError(t, err)
	assert.Equal(t, "10", first.Value().String())
	assert.Equal(t, mPeriodOne, first.PeriodID())
	assert.Equal(t, "Gabriela Rojas", first.CapturedByName())
	assert.False(t, first.UpdatedAt().IsZero())

	time.Sleep(10 * time.Millisecond) // now() must move between the two transactions
	second, err := repo.UpsertPeriodGrade(ctx, periodGrade(t, "a1000000-0000-0000-0000-000000000099", tenantA, mEnrollmentA, mSubject, mPeriodOne, mDirector, "7"), ports.PeriodLock{})
	require.NoError(t, err)
	assert.Equal(t, first.ID(), second.ID(), "the upsert must keep the row")
	assert.Equal(t, 700, second.Value().Hundredths())
	assert.Equal(t, mDirector, second.CapturedBy())
	assert.True(t, second.UpdatedAt().After(first.UpdatedAt()), "updated_at must move on a correction")
	assert.Equal(t, int64(1), countPeriodGrades(t, tenantA, mEnrollmentA))

	// Another period is another cell.
	_, err = repo.UpsertPeriodGrade(ctx, periodGrade(t, "a1000000-0000-0000-0000-000000000002", tenantA, mEnrollmentA, mSubject, mPeriodTwo, mUser, "8"), ports.PeriodLock{})
	require.NoError(t, err)
	all, err := repo.PeriodGradesForEnrollment(ctx, mEnrollmentA)
	require.NoError(t, err)
	require.Len(t, all, 2)
	assert.Equal(t, mPeriodOne, all[0].PeriodID(), "ordered by period")
	assert.Equal(t, "8", all[1].Value().String())
}

// The sheet is one batched read: only the enrollments that have a grade for
// that subject and period come back, keyed by enrollment id.
func TestPeriodGradesForSheet(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	_, err := repo.UpsertPeriodGrade(ctx, periodGrade(t, "a1000000-0000-0000-0000-000000000010", tenantA, mEnrollmentB, mSubject, mPeriodThree, mUser, "6"), ports.PeriodLock{})
	require.NoError(t, err)
	_, err = repo.UpsertPeriodGrade(ctx, periodGrade(t, "a1000000-0000-0000-0000-000000000011", tenantA, mEnrollmentB, mSubjectTwo, mPeriodThree, mUser, "10"), ports.PeriodLock{})
	require.NoError(t, err)

	got, err := repo.PeriodGradesForSheet(ctx, []string{mEnrollmentB, mEnrollmentC}, mSubject, mPeriodThree)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "6", got[mEnrollmentB].Value().String())

	empty, err := repo.PeriodGradesForSheet(ctx, nil, mSubject, mPeriodThree)
	require.NoError(t, err)
	assert.Empty(t, empty)

	both, err := repo.PeriodGradesForEnrollments(ctx, []string{mEnrollmentB, mEnrollmentC}, []string{mPeriodThree})
	require.NoError(t, err)
	assert.Len(t, both, 2)
	none, err := repo.PeriodGradesForEnrollments(ctx, []string{mEnrollmentB}, nil)
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestPeriodGradeDeleteIsIdempotent(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	_, err := repo.UpsertPeriodGrade(ctx, periodGrade(t, "a1000000-0000-0000-0000-000000000020", tenantA, mEnrollmentC, mSubject, mPeriodTwo, mUser, "9"), ports.PeriodLock{})
	require.NoError(t, err)

	require.NoError(t, repo.DeletePeriodGrade(ctx, mEnrollmentC, mSubject, mPeriodTwo, ports.PeriodLock{}))
	assert.Equal(t, int64(0), countPeriodGrades(t, tenantA, mEnrollmentC))
	require.NoError(t, repo.DeletePeriodGrade(ctx, mEnrollmentC, mSubject, mPeriodTwo, ports.PeriodLock{}), "deleting an empty cell is not an error")
}

// The table refuses a value that is not a whole number or falls outside
// 0–100, and a second row for the same cell, even when a write bypasses the
// domain.
func TestPeriodGradeChecks(t *testing.T) {
	db, err := open(appDSN)
	require.NoError(t, err)
	insert := func(value string) error {
		return db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantA).Error; err != nil {
				return err
			}
			return tx.Exec("INSERT INTO period_grades (tenant_id, enrollment_id, subject_id, period_id, value, captured_by) VALUES (?, ?, ?, ?, ?, ?)",
				tenantA, mEnrollmentC, mSubjectTwo, mPeriodOne, value, mUser).Error
		})
	}
	for _, value := range []string{"9.5", "8.01", "100.5", "101", "-1"} {
		assert.Error(t, insert(value), "value %s must be refused", value)
	}
	require.NoError(t, insert("9.00"), "a whole number with zero decimals is a whole number")
	assert.Error(t, insert("8"), "one row per enrollment·subject·period")
}

// RLS isolation for the non-owner app role: tenant A can neither read, change
// nor delete tenant B's period grade.
func TestRLSIsolatesPeriodGrades(t *testing.T) {
	ctx := context.Background()
	b, err := appRepo(t, tenantB).UpsertPeriodGrade(ctx, periodGrade(t, "b1000000-0000-0000-0000-000000000001", tenantB, mEnrollmentRL, mSubject, mPeriodOne, mUser, "7"), ports.PeriodLock{})
	require.NoError(t, err)

	a := appRepo(t, tenantA)
	seen, err := a.PeriodGradesForEnrollment(ctx, mEnrollmentRL)
	require.NoError(t, err)
	assert.Empty(t, seen, "tenant A read tenant B's grade")
	sheet, err := a.PeriodGradesForSheet(ctx, []string{mEnrollmentRL}, mSubject, mPeriodOne)
	require.NoError(t, err)
	assert.Empty(t, sheet)

	require.NoError(t, a.DeletePeriodGrade(ctx, mEnrollmentRL, mSubject, mPeriodOne, ports.PeriodLock{}))
	_, err = a.UpsertPeriodGrade(ctx, periodGrade(t, "a1000000-0000-0000-0000-0000000000a1", tenantA, mEnrollmentRL, mSubject, mPeriodOne, mUser, "3"), ports.PeriodLock{})
	require.NoError(t, err)

	after, err := appRepo(t, tenantB).PeriodGradesForEnrollment(ctx, mEnrollmentRL)
	require.NoError(t, err)
	require.Len(t, after, 1)
	assert.Equal(t, b.ID(), after[0].ID())
	assert.Equal(t, 700, after[0].Value().Hundredths(), "tenant A's write or delete leaked into tenant B")
	assert.Equal(t, int64(1), countPeriodGrades(t, tenantA, mEnrollmentRL))
}

// PeriodsWithGrades answers which periods hold at least one grade (config's
// guard against removing a period with grades) — only the tenant's own rows.
func TestPeriodsWithGrades(t *testing.T) {
	const (
		periodUsed  = "0e000000-0000-0000-0000-000000000041"
		periodEmpty = "0e000000-0000-0000-0000-000000000042"
		periodOther = "0e000000-0000-0000-0000-000000000043"
	)
	ctx := context.Background()
	repo := appRepo(t, tenantA)
	for i, enr := range []string{mEnrollmentA, mEnrollmentB} {
		id := "a1000000-0000-0000-0000-00000000004" + string(rune('0'+i))
		_, err := repo.UpsertPeriodGrade(ctx, periodGrade(t, id, tenantA, enr, mSubject, periodUsed, mUser, "9"), ports.PeriodLock{})
		require.NoError(t, err)
	}
	_, err := appRepo(t, tenantB).UpsertPeriodGrade(ctx, periodGrade(t, "b1000000-0000-0000-0000-000000000043", tenantB, mEnrollmentA, mSubject, periodOther, mUser, "9"), ports.PeriodLock{})
	require.NoError(t, err)

	got, err := repo.PeriodsWithGrades(ctx, []string{periodEmpty, periodUsed, periodOther})
	require.NoError(t, err)
	assert.Equal(t, []string{periodUsed}, got, "once per period; another tenant's grades never count")

	none, err := repo.PeriodsWithGrades(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, none)
}

// CountScaleViolations counts, in one tenant-bound query, the named
// enrollments' period grades a proposed scale refuses: out of range, or —
// disabling grading — every grade. Grades are whole numbers, so decimals never
// count. Another tenant's grades never count.
func TestCountScaleViolations(t *testing.T) {
	const (
		enrV1 = "e1000000-0000-0000-0000-000000000051"
		enrV2 = "e1000000-0000-0000-0000-000000000052"
	)
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	for i, c := range []struct{ enr, period, value string }{
		{enrV1, mPeriodOne, "9"}, {enrV1, mPeriodTwo, "6"}, {enrV2, mPeriodOne, "10"}, {enrV2, mPeriodTwo, "7"},
	} {
		id := "a1000000-0000-0000-0000-00000000005" + string(rune('0'+i))
		_, err := repo.UpsertPeriodGrade(ctx, periodGrade(t, id, tenantA, c.enr, mSubject, c.period, mUser, c.value), ports.PeriodLock{})
		require.NoError(t, err)
	}
	_, err := repo.UpsertPeriodGrade(ctx, letterGrade(t, "a1000000-0000-0000-0000-00000000005a", enrV2, mSubjectTwo, mPeriodOne, domain.LetterA), ports.PeriodLock{})
	require.NoError(t, err)
	_, err = appRepo(t, tenantB).UpsertPeriodGrade(ctx, periodGrade(t, "a1000000-0000-0000-0000-000000000059", tenantB, enrV1, mSubject, mPeriodThree, mUser, "1"), ports.PeriodLock{})
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
		// One captured letter: a numeric scale refuses it whatever its bounds.
		{"loosen", both, ports.ScaleCheck{GradingEnabled: true, Scale: scale(0, 1000, 600, 2)}, 1},
		{"max 9.5", both, ports.ScaleCheck{GradingEnabled: true, Scale: scale(500, 950, 600, 2)}, 2},
		{"min 7", both, ports.ScaleCheck{GradingEnabled: true, Scale: scale(700, 1000, 800, 2)}, 2},
		{"whole numbers never violate decimals", both, ports.ScaleCheck{GradingEnabled: true, Scale: scale(500, 1000, 600, 0)}, 1},
		{"switch to letters refuses every number", both, ports.ScaleCheck{GradingLetters: true}, 4},
		{"disable", both, ports.ScaleCheck{}, 5},
		{"only the named", []string{enrV2}, ports.ScaleCheck{}, 3},
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

// Monthly grades captured before T15 stay in their table, untouched: the
// migration converts nothing and no code path writes them.
func TestMonthlyGradesStayUntouched(t *testing.T) {
	var n int64
	require.NoError(t, superDB.Raw("SELECT count(*) FROM information_schema.tables WHERE table_name = 'monthly_grades'").Scan(&n).Error)
	assert.Equal(t, int64(1), n, "the table is kept")
}
