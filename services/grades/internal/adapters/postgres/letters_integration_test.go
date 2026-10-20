//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

const (
	lEnrollment = "e1000000-0000-0000-0000-0000000000a1"
	lStudent    = "57700000-0000-0000-0000-0000000000a1"
)

func letterGrade(t *testing.T, id, enrollment, subject, period string, l domain.Letter) domain.PeriodGrade {
	t.Helper()
	g, err := domain.NewPeriodGrade(domain.PeriodGradeFields{
		ID: id, TenantID: tenantA, EnrollmentID: enrollment, SubjectID: subject, PeriodID: period,
		CapturedBy: mUser, CapturedByName: "Gabriela Rojas", Letter: l, UpdatedAt: captureClock,
	})
	require.NoError(t, err)
	return g
}

// A preescolar cell holds a letter: it round-trips, and a correction over the
// same cell replaces it in place (the second upsert is the path every autosave
// walks).
func TestPeriodGradeLetterUpsertsTwice(t *testing.T) {
	repo := appRepo(t, tenantA)
	ctx := context.Background()

	first, err := repo.UpsertPeriodGrade(ctx, letterGrade(t, "a1000000-0000-0000-0000-0000000000e1", lEnrollment, mSubject, mPeriodOne, domain.LetterB), ports.PeriodLock{})
	require.NoError(t, err)
	assert.Equal(t, domain.LetterB, first.Letter())

	second, err := repo.UpsertPeriodGrade(ctx, letterGrade(t, "a1000000-0000-0000-0000-0000000000e2", lEnrollment, mSubject, mPeriodOne, domain.LetterA), ports.PeriodLock{})
	require.NoError(t, err)
	assert.Equal(t, first.ID(), second.ID())
	assert.Equal(t, domain.LetterA, second.Letter())

	all, err := repo.PeriodGradesForEnrollment(ctx, lEnrollment)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, "A", all[0].Text())
}

// The table holds a number or a letter, exactly one, and only A–D.
func TestPeriodGradeLetterChecks(t *testing.T) {
	db, err := open(appDSN)
	require.NoError(t, err)
	insert := func(value, letter any) error {
		return db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantA).Error; err != nil {
				return err
			}
			return tx.Exec("INSERT INTO period_grades (tenant_id, enrollment_id, subject_id, period_id, value, letter, captured_by) VALUES (?, ?, ?, ?, ?, ?, ?)",
				tenantA, "e1000000-0000-0000-0000-0000000000a2", mSubjectTwo, mPeriodTwo, value, letter, mUser).Error
		})
	}
	assert.Error(t, insert(nil, nil), "neither number nor letter")
	assert.Error(t, insert("9", "A"), "both")
	assert.Error(t, insert(nil, "E"), "only A–D")
	assert.NoError(t, insert(nil, "D"))
}

// A letters card round-trips: the letters per period, no average.
func TestLettersReportCardRoundTrips(t *testing.T) {
	const term = "70700000-0000-0000-0000-0000000000a1"
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	grades := []domain.PeriodGrade{
		letterGrade(t, "a7000000-0000-0000-0000-0000000000c1", lEnrollment, pSubjectA, mPeriodOne, domain.LetterA),
		letterGrade(t, "a7000000-0000-0000-0000-0000000000c2", lEnrollment, pSubjectB, mPeriodTwo, domain.LetterC),
	}
	card := func(id string, version int) domain.ReportCard {
		rc, err := domain.BuildReportCard(domain.ReportCardHeader{
			ID: id, TenantID: tenantA, TermID: term, CycleID: pCycle, LevelID: pLevel, GroupID: pGroup,
			EnrollmentID: lEnrollment, StudentID: lStudent, Version: version, IssuedAt: pClock, IssuedBy: pDirector,
			StudentName: "Kim Ortiz", EnrollmentNumber: "2026-00005", GroupLabel: "K A", LevelName: "Preescolar",
			CycleName: "2026-2027", TermName: "2.º trimestre", TermOrdinal: 2, Periods: cardColumns, Letters: true,
		}, []domain.ReportCardSubject{{ID: pSubjectA, Name: "Lenguajes"}, {ID: pSubjectB, Name: "Ética"}}, grades)
		require.NoError(t, err)
		return rc
	}
	require.NoError(t, repo.WithPeriodLock(ctx, term, func(ctx context.Context, tx ports.PeriodTx) error {
		return tx.IssueReportCards(ctx, []domain.ReportCard{card("a7000000-0000-0000-0000-0000000000c9", 1)})
	}))
	var published []domain.ReportCard
	require.NoError(t, repo.WithPeriodLock(ctx, term, func(ctx context.Context, tx ports.PeriodTx) error {
		var err error
		published, err = tx.PublishedReportCards(ctx)
		return err
	}))
	require.Len(t, published, 1)
	got := published[0]
	assert.True(t, got.Header().Letters)
	assert.False(t, got.GeneralAverage().Present())
	assert.True(t, card("a7000000-0000-0000-0000-0000000000ca", 2).SameGradesAs(got), "a stored letters card reads back with its letters")
}
