//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

// Neutral references for the T7 tests (disjoint from the other fixtures: the
// container is shared and not reset between tests).
const (
	pCycle    = "c7000000-0000-0000-0000-000000000001"
	pLevel    = "17000000-0000-0000-0000-000000000001"
	pGroup    = "47000000-0000-0000-0000-000000000001"
	pSubjectA = "57000000-0000-0000-0000-000000000001"
	pSubjectB = "57000000-0000-0000-0000-000000000002"
	pDirector = "77000000-0000-0000-0000-000000000001"
)

var pClock = time.Date(2026, 12, 1, 10, 0, 0, 0, time.UTC)

func month(t *testing.T, raw string) domain.Month {
	t.Helper()
	m, err := domain.ParseMonth(raw)
	require.NoError(t, err)
	return m
}

func termMonths(t *testing.T) []domain.Month {
	return []domain.Month{month(t, "2026-09"), month(t, "2026-10"), month(t, "2026-11")}
}

func closedPeriod(t *testing.T, termID string) domain.PeriodClosure {
	t.Helper()
	c, err := domain.NewOpenPeriod(termID, pCycle, pLevel).Close(domain.Actor{ID: pDirector, Name: "Directora Ruiz"}, pClock, termMonths(t))
	require.NoError(t, err)
	return c
}

// cardColumns are a 2.º trimestre card's columns: periods 1 and 2.
var cardColumns = []domain.ReportCardPeriod{
	{ID: mPeriodOne, Ordinal: 1, Name: "1.er trimestre", StartsOn: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)},
	{ID: mPeriodTwo, Ordinal: 2, Name: "2.º trimestre", StartsOn: time.Date(2026, 12, 7, 0, 0, 0, 0, time.UTC)},
}

func reportCard(t *testing.T, id, tenant, termID, enrollment, student string, version int, grades []domain.PeriodGrade) domain.ReportCard {
	t.Helper()
	return reportCardOfTerm(t, id, tenant, termID, 2, enrollment, student, version, grades)
}

func reportCardOfTerm(t *testing.T, id, tenant, termID string, ordinal int, enrollment, student string, version int, grades []domain.PeriodGrade) domain.ReportCard {
	t.Helper()
	scale, err := domain.NewGradingScale(500, 1000, 600, 1)
	require.NoError(t, err)
	rc, err := domain.BuildReportCard(domain.ReportCardHeader{
		ID: id, TenantID: tenant, TermID: termID, CycleID: pCycle, LevelID: pLevel, GroupID: pGroup,
		EnrollmentID: enrollment, StudentID: student, Version: version, IssuedAt: pClock, IssuedBy: pDirector,
		StudentName: "Ana López Ñúñez", EnrollmentNumber: "2026-00042", GroupLabel: "1° A", LevelName: "Primaria",
		CycleName: "2026-2027", TermName: "2.º trimestre", TermOrdinal: ordinal, Periods: cardColumns, Scale: scale,
	}, []domain.ReportCardSubject{{ID: pSubjectA, Name: "Matemáticas"}, {ID: pSubjectB, Name: "Español"}}, grades)
	require.NoError(t, err)
	return rc
}

// The closure upserts in place: saving it closed, then reopened, then closed
// again on the same term keeps one row whose every column follows — the path
// every real period walks after its first close.
func TestPeriodClosureUpsertsTwice(t *testing.T) {
	const term = "70700000-0000-0000-0000-000000000001"
	repo := appRepo(t, tenantA)
	ctx := context.Background()

	err := repo.WithPeriodLock(ctx, term, func(ctx context.Context, tx ports.PeriodTx) error {
		_, found, err := tx.Closure(ctx)
		require.NoError(t, err)
		assert.False(t, found, "a term never closed has no row")
		return tx.SaveClosure(ctx, closedPeriod(t, term))
	})
	require.NoError(t, err)

	got, err := repo.PeriodClosures(ctx, []string{term, "70700000-0000-0000-0000-0000000000ff"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.True(t, got[term].IsClosed())
	assert.Equal(t, 1, got[term].Version())
	assert.Equal(t, "Directora Ruiz", got[term].ClosedByName())
	assert.Equal(t, pDirector, got[term].ClosedBy())
	assert.True(t, got[term].ClosedAt().Equal(pClock))
	assert.Equal(t, pCycle, got[term].CycleID())
	assert.Equal(t, []string{"2026-09", "2026-10", "2026-11"}, monthsText(got[term].Months()), "the months snapshot is stored")

	reopenAt := pClock.Add(time.Hour)
	require.NoError(t, repo.WithPeriodLock(ctx, term, func(ctx context.Context, tx ports.PeriodTx) error {
		c, found, err := tx.Closure(ctx)
		require.NoError(t, err)
		require.True(t, found)
		reopened, err := c.Reopen(reopenAt)
		require.NoError(t, err)
		return tx.SaveClosure(ctx, reopened)
	}))
	got, err = repo.PeriodClosures(ctx, []string{term})
	require.NoError(t, err)
	assert.False(t, got[term].IsClosed(), "the reopen must be written, not dropped by the upsert")
	assert.True(t, got[term].ReopenedAt().Equal(reopenAt))
	assert.Equal(t, 1, got[term].Version())

	require.NoError(t, repo.WithPeriodLock(ctx, term, func(ctx context.Context, tx ports.PeriodTx) error {
		c, _, err := tx.Closure(ctx)
		require.NoError(t, err)
		again, err := c.Close(domain.Actor{ID: pDirector, Name: "Otra Persona"}, reopenAt.Add(time.Hour), termMonths(t)[:2])
		require.NoError(t, err)
		return tx.SaveClosure(ctx, again)
	}))
	got, err = repo.PeriodClosures(ctx, []string{term})
	require.NoError(t, err)
	assert.Equal(t, 2, got[term].Version())
	assert.Equal(t, "Otra Persona", got[term].ClosedByName())
	assert.Equal(t, []string{"2026-09", "2026-10"}, monthsText(got[term].Months()), "a re-close rewrites the snapshot")

	empty, err := repo.PeriodClosures(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

// fn's error rolls the whole transaction back: no closure, no report card.
func TestWithPeriodLockRollsBack(t *testing.T) {
	const term = "70700000-0000-0000-0000-000000000002"
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	boom := errors.New("boom")
	err := repo.WithPeriodLock(ctx, term, func(ctx context.Context, tx ports.PeriodTx) error {
		require.NoError(t, tx.SaveClosure(ctx, closedPeriod(t, term)))
		require.NoError(t, tx.IssueReportCards(ctx, []domain.ReportCard{
			reportCard(t, "a7000000-0000-0000-0000-000000000002", tenantA, term, "e7000000-0000-0000-0000-000000000002", "57700000-0000-0000-0000-000000000002", 1, nil),
		}))
		return boom
	})
	assert.ErrorIs(t, err, boom)
	got, err := repo.PeriodClosures(ctx, []string{term})
	require.NoError(t, err)
	assert.Empty(t, got)
	cards, err := repo.LatestReportCards(ctx, ports.ReportCardFilter{TermID: term})
	require.NoError(t, err)
	assert.Empty(t, cards)
}

// Issuing v2 supersedes EVERY published v1 of the term — including the card
// of a pupil withdrawn between the two closes, who gets no v2 (ADR 0020): a
// re-close replaces the term's report cards as a whole, so a stale v1 never
// stays published beside the new version. v1 stays readable, superseded. The
// snapshot keeps names, months, exact month values and averages.
func TestIssueReportCardsSupersedes(t *testing.T) {
	const (
		term     = "70700000-0000-0000-0000-000000000003"
		enr      = "e7000000-0000-0000-0000-000000000003"
		enrOther = "e7000000-0000-0000-0000-000000000004"
		stu      = "57700000-0000-0000-0000-000000000003"
		stuOther = "57700000-0000-0000-0000-000000000004"
		v1ID     = "a7000000-0000-0000-0000-000000000031"
		v2ID     = "a7000000-0000-0000-0000-000000000032"
		otherID  = "a7000000-0000-0000-0000-000000000033"
	)
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	grades := []domain.PeriodGrade{
		periodGrade(t, "a7000000-0000-0000-0000-0000000000a1", tenantA, enr, pSubjectA, mPeriodOne, mUser, "9"),
		periodGrade(t, "a7000000-0000-0000-0000-0000000000a2", tenantA, enr, pSubjectA, mPeriodTwo, mUser, "8"),
	}
	require.NoError(t, repo.WithPeriodLock(ctx, term, func(ctx context.Context, tx ports.PeriodTx) error {
		return tx.IssueReportCards(ctx, []domain.ReportCard{
			reportCard(t, v1ID, tenantA, term, enr, stu, 1, grades),
			reportCard(t, otherID, tenantA, term, enrOther, stuOther, 1, nil),
		})
	}))
	require.NoError(t, repo.WithPeriodLock(ctx, term, func(ctx context.Context, tx ports.PeriodTx) error {
		return tx.IssueReportCards(ctx, []domain.ReportCard{reportCard(t, v2ID, tenantA, term, enr, stu, 2, grades[:1])})
	}))

	latest, err := repo.LatestReportCards(ctx, ports.ReportCardFilter{TermID: term})
	require.NoError(t, err)
	require.Len(t, latest, 1, "only the re-close's cards are published")
	assert.Equal(t, v2ID, latest[0].ID())
	withdrawn, err := repo.ReportCard(ctx, otherID)
	require.NoError(t, err)
	assert.Equal(t, domain.ReportCardSuperseded, withdrawn.Status(), "the withdrawn pupil's v1 is superseded, not left published")

	byStudent, err := repo.LatestReportCards(ctx, ports.ReportCardFilter{StudentID: stu})
	require.NoError(t, err)
	require.Len(t, byStudent, 1)
	assert.Equal(t, 2, byStudent[0].Version())
	assert.Equal(t, "9", byStudent[0].GeneralAverage().String())

	byGroup, err := repo.LatestReportCards(ctx, ports.ReportCardFilter{TermID: term, GroupID: pGroup})
	require.NoError(t, err)
	assert.Len(t, byGroup, 1)

	v1, err := repo.ReportCard(ctx, v1ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ReportCardSuperseded, v1.Status())
	h := v1.Header()
	assert.Equal(t, "Ana López Ñúñez", h.StudentName)
	assert.Equal(t, "2026-00042", h.EnrollmentNumber)
	assert.Equal(t, "1° A", h.GroupLabel)
	assert.Equal(t, "Primaria", h.LevelName)
	assert.Equal(t, "2026-2027", h.CycleName)
	assert.Equal(t, "2.º trimestre", h.TermName)
	assert.Equal(t, 2, h.TermOrdinal, "the term's order is part of the snapshot")
	assert.Empty(t, h.Months, "a T15 card has no month columns")
	assert.Equal(t, cardColumns, h.Periods, "the period columns are part of the snapshot")
	assert.Equal(t, 500, h.Scale.MinHundredths())
	assert.Equal(t, 600, h.Scale.PassingHundredths())
	assert.Equal(t, 1, h.Scale.Decimals())
	assert.True(t, h.IssuedAt.Equal(pClock))
	assert.Equal(t, "8.5", v1.GeneralAverage().String())
	require.Len(t, v1.Lines(), 2)
	assert.Equal(t, "Matemáticas", v1.Lines()[0].SubjectName)
	assert.Equal(t, 1, v1.Lines()[0].Position)
	assert.Equal(t, map[string]string{mPeriodOne: "9", mPeriodTwo: "8"}, v1.Lines()[0].PeriodValueStrings())
	assert.Equal(t, "8.5", v1.Lines()[0].Average.String())
	assert.False(t, v1.Lines()[1].Average.Present(), "NULL stays no average, never 0")
	assert.Empty(t, v1.Lines()[1].PeriodValues)

	v2, err := repo.ReportCard(ctx, v2ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ReportCardPublished, v2.Status())

	_, err = repo.ReportCard(ctx, "a7000000-0000-0000-0000-0000000000ff")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}

func monthsText(ms []domain.Month) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.String())
	}
	return out
}

// A period-grade write guarded by a CLOSED term is refused with PERIOD_LOCKED
// inside its own transaction; an open term, or no term, lets it through.
func TestPeriodGradeWriteRespectsTheClosure(t *testing.T) {
	const (
		term = "70700000-0000-0000-0000-000000000005"
		enr  = "e7000000-0000-0000-0000-000000000005"
	)
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	lock := ports.PeriodLock{TermID: term}

	_, err := repo.UpsertPeriodGrade(ctx, periodGrade(t, "a7000000-0000-0000-0000-000000000051", tenantA, enr, mSubject, mPeriodOne, mUser, "9"), lock)
	require.NoError(t, err, "a term never closed is open")

	require.NoError(t, repo.WithPeriodLock(ctx, term, func(ctx context.Context, tx ports.PeriodTx) error {
		return tx.SaveClosure(ctx, closedPeriod(t, term))
	}))
	_, err = repo.UpsertPeriodGrade(ctx, periodGrade(t, "a7000000-0000-0000-0000-000000000052", tenantA, enr, mSubject, mPeriodOne, mUser, "5"), lock)
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Equal(t, domain.CodePeriodLocked, domain.RefusalCodeOf(err))
	err = repo.DeletePeriodGrade(ctx, enr, mSubject, mPeriodOne, lock)
	assert.Equal(t, domain.CodePeriodLocked, domain.RefusalCodeOf(err))

	stored, err := repo.PeriodGradesForEnrollment(ctx, enr)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, 900, stored[0].Value().Hundredths(), "the locked write and delete changed nothing")

	_, err = repo.UpsertPeriodGrade(ctx, periodGrade(t, "a7000000-0000-0000-0000-000000000053", tenantA, enr, mSubject, mPeriodThree, mUser, "7"), ports.PeriodLock{})
	require.NoError(t, err, "a period no term maps is not guarded")
}

// A grade write that arrives while a close holds the term waits for it, then
// sees the closure and is refused: nothing slips between the snapshot and the
// closure.
func TestPeriodGradeWriteWaitsForAClose(t *testing.T) {
	const (
		term = "70700000-0000-0000-0000-000000000006"
		enr  = "e7000000-0000-0000-0000-000000000006"
	)
	repo := appRepo(t, tenantA)
	writer := appRepo(t, tenantA)
	ctx := context.Background()

	started := make(chan struct{})
	writeErr := make(chan error, 1)
	require.NoError(t, repo.WithPeriodLock(ctx, term, func(ctx context.Context, tx ports.PeriodTx) error {
		if err := tx.SaveClosure(ctx, closedPeriod(t, term)); err != nil {
			return err
		}
		go func() {
			close(started)
			_, err := writer.UpsertPeriodGrade(context.Background(),
				periodGrade(t, "a7000000-0000-0000-0000-000000000061", tenantA, enr, mSubject, mPeriodTwo, mUser, "8"),
				ports.PeriodLock{TermID: term})
			writeErr <- err
		}()
		<-started
		time.Sleep(150 * time.Millisecond) // the writer is now queued on the lock
		return nil
	}))
	select {
	case err := <-writeErr:
		assert.Equal(t, domain.CodePeriodLocked, domain.RefusalCodeOf(err), "err = %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the write never finished")
	}
	assert.Equal(t, int64(0), countPeriodGrades(t, tenantA, enr))
}

// A report card issued BEFORE T15 (month columns, one-decimal averages) reads
// back exactly as it was issued: its months, month values and averages.
func TestLegacyMonthlyReportCardReadsBackAsIssued(t *testing.T) {
	const (
		cardID = "a7000000-0000-0000-0000-0000000000c1"
		term   = "70700000-0000-0000-0000-0000000000c1"
		enr    = "e7000000-0000-0000-0000-0000000000c1"
		stu    = "57700000-0000-0000-0000-0000000000c1"
	)
	require.NoError(t, superDB.Exec(`INSERT INTO report_cards (id, tenant_id, term_id, cycle_id, level_id, group_id, enrollment_id, student_id,
		version, status, issued_at, issued_by, student_name, term_name, months, general_average,
		scale_min_hundredths, scale_max_hundredths, scale_decimals, scale_passing_hundredths)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, 'published', now(), ?, 'Ana', '1.er trimestre', '{2026-09,2026-10}', 8.50, 500, 1000, 1, 600)`,
		cardID, tenantA, term, pCycle, pLevel, pGroup, enr, stu, pDirector).Error)
	require.NoError(t, superDB.Exec(`INSERT INTO report_card_lines (report_card_id, tenant_id, subject_id, subject_name, position, month_values, term_average)
		VALUES (?, ?, ?, 'Matemáticas', 1, '{"2026-09":"8.4","2026-10":"8.5"}', 8.50)`, cardID, tenantA, pSubjectA).Error)

	rc, err := appRepo(t, tenantA).ReportCard(context.Background(), cardID)
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-09", "2026-10"}, monthsText(rc.Header().Months))
	assert.Empty(t, rc.Header().Periods)
	assert.Equal(t, "8.5", rc.GeneralAverage().String())
	require.Len(t, rc.Lines(), 1)
	assert.Equal(t, map[string]string{"2026-09": "8.4", "2026-10": "8.5"}, rc.Lines()[0].MonthValueStrings())
	assert.Empty(t, rc.Lines()[0].PeriodValues)
	assert.Equal(t, "8.5", rc.Lines()[0].Average.String())
}

// RLS isolation for the non-owner app role on all three T7 tables: tenant A
// neither reads nor supersedes tenant B's closures, report cards or lines, and
// cannot write a line under tenant B.
func TestRLSIsolatesPeriodTables(t *testing.T) {
	const (
		term = "70700000-0000-0000-0000-000000000008"
		enr  = "e7000000-0000-0000-0000-000000000008"
		stu  = "57700000-0000-0000-0000-000000000008"
		bID  = "b7000000-0000-0000-0000-000000000081"
		aID  = "a7000000-0000-0000-0000-000000000081"
	)
	ctx := context.Background()
	b := appRepo(t, tenantB)
	require.NoError(t, b.WithPeriodLock(ctx, term, func(ctx context.Context, tx ports.PeriodTx) error {
		if err := tx.SaveClosure(ctx, closedPeriod(t, term)); err != nil {
			return err
		}
		return tx.IssueReportCards(ctx, []domain.ReportCard{reportCard(t, bID, tenantB, term, enr, stu, 1,
			[]domain.PeriodGrade{periodGrade(t, "b7000000-0000-0000-0000-0000000000b1", tenantB, enr, pSubjectA, mPeriodOne, mUser, "9")})})
	}))

	a := appRepo(t, tenantA)
	closures, err := a.PeriodClosures(ctx, []string{term})
	require.NoError(t, err)
	assert.Empty(t, closures, "tenant A read tenant B's closure")
	cards, err := a.LatestReportCards(ctx, ports.ReportCardFilter{TermID: term})
	require.NoError(t, err)
	assert.Empty(t, cards)
	_, err = a.ReportCard(ctx, bID)
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))

	// Tenant A issuing a card for the same enrollment neither supersedes B's
	// nor collides with it.
	require.NoError(t, a.WithPeriodLock(ctx, term, func(ctx context.Context, tx ports.PeriodTx) error {
		_, found, err := tx.Closure(ctx)
		require.NoError(t, err)
		assert.False(t, found, "tenant A saw tenant B's closure under the lock")
		return tx.IssueReportCards(ctx, []domain.ReportCard{reportCard(t, aID, tenantA, term, enr, stu, 1, nil)})
	}))
	still, err := b.ReportCard(ctx, bID)
	require.NoError(t, err)
	assert.Equal(t, domain.ReportCardPublished, still.Status())
	require.Len(t, still.Lines(), 2)

	// A line under tenant B written with tenant A bound is refused by the policy.
	db, err := open(appDSN)
	require.NoError(t, err)
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantA).Error; err != nil {
			return err
		}
		return tx.Exec("INSERT INTO report_card_lines (report_card_id, tenant_id, subject_id, subject_name, position) VALUES (?, ?, ?, 'X', 9)",
			aID, tenantB, pSubjectB).Error
	})
	assert.Error(t, err, "a line under another tenant must be refused")
	var n int64
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantA).Error; err != nil {
			return err
		}
		return tx.Table("report_card_lines").Where("report_card_id = ?", bID).Count(&n).Error
	}))
	assert.Equal(t, int64(0), n, "tenant A read tenant B's lines")
}

// The legacy per-term capture (CaptureGrade → UpsertGradeEntry) honours the
// period lock too: its term closed, the upsert is refused with MONTH_LOCKED
// under the same shared advisory lock a period-grade write takes.
func TestGradeEntryWriteRespectsTheClosure(t *testing.T) {
	const (
		term = "70700000-0000-0000-0000-000000000009"
		enr  = "e7000000-0000-0000-0000-000000000009"
	)
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	entry := func(id string, value float64) domain.GradeEntry {
		e, err := domain.NewGradeEntry(id, tenantA, enr, subjectOne, term, teacherOne, value, captureClock)
		require.NoError(t, err)
		return e
	}
	_, err := repo.UpsertGradeEntry(ctx, entry("a7000000-0000-0000-0000-000000000091", 9))
	require.NoError(t, err, "a term never closed is open")

	require.NoError(t, repo.WithPeriodLock(ctx, term, func(ctx context.Context, tx ports.PeriodTx) error {
		return tx.SaveClosure(ctx, closedPeriod(t, term))
	}))
	_, err = repo.UpsertGradeEntry(ctx, entry("a7000000-0000-0000-0000-000000000092", 5))
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Equal(t, domain.CodeMonthLocked, domain.RefusalCodeOf(err))
	assert.Equal(t, "El periodo está cerrado", glerrors.SafeMessage(err))

	stored, err := repo.ListGradeEntries(ctx, enr, term)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.InDelta(t, 9.0, stored[0].Value(), 0.001, "the locked write changed nothing")
}

// A re-close compares against the term's published cards: read under the
// lock, lines included, exactly as they were issued (so an unchanged pupil
// compares equal), and never a superseded card.
func TestPublishedReportCardsReadsTheTermsCardsWithLines(t *testing.T) {
	const (
		term  = "70700000-0000-0000-0000-000000000009"
		other = "70700000-0000-0000-0000-00000000000a"
		enr   = "e7000000-0000-0000-0000-000000000009"
		stu   = "57700000-0000-0000-0000-000000000009"
		v1ID  = "a7000000-0000-0000-0000-000000000091"
		v2ID  = "a7000000-0000-0000-0000-000000000092"
		oID   = "a7000000-0000-0000-0000-000000000093"
	)
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	grades := []domain.PeriodGrade{
		periodGrade(t, "a7000000-0000-0000-0000-0000000000b1", tenantA, enr, pSubjectA, mPeriodOne, mUser, "9"),
		periodGrade(t, "a7000000-0000-0000-0000-0000000000b2", tenantA, enr, pSubjectB, mPeriodTwo, mUser, "8"),
		periodGrade(t, "a7000000-0000-0000-0000-0000000000b3", tenantA, enr, pSubjectB, mPeriodOne, mUser, "9"),
	}
	issue := func(termID string, cards ...domain.ReportCard) {
		require.NoError(t, repo.WithPeriodLock(ctx, termID, func(ctx context.Context, tx ports.PeriodTx) error {
			return tx.IssueReportCards(ctx, cards)
		}))
	}
	issue(term, reportCard(t, v1ID, tenantA, term, enr, stu, 1, grades))
	issue(other, reportCard(t, oID, tenantA, other, enr, stu, 1, nil))
	v2 := reportCard(t, v2ID, tenantA, term, enr, stu, 2, grades)
	issue(term, v2)

	var published []domain.ReportCard
	require.NoError(t, repo.WithPeriodLock(ctx, term, func(ctx context.Context, tx ports.PeriodTx) error {
		var err error
		published, err = tx.PublishedReportCards(ctx)
		return err
	}))

	require.Len(t, published, 1, "only this term's published card; v1 is superseded, the other term is not ours")
	assert.Equal(t, v2ID, published[0].ID())
	require.Len(t, published[0].Lines(), 2)
	assert.True(t, reportCard(t, "a7000000-0000-0000-0000-000000000094", tenantA, term, enr, stu, 3, grades).SameGradesAs(published[0]),
		"a stored card round-trips to the same grades")
}

// A pupil's list spans terms: the latest published card of EACH term, each
// with its frozen term ordinal; a superseded version never shows.
func TestLatestReportCardsOfAStudentAcrossTerms(t *testing.T) {
	const (
		termOne = "70700000-0000-0000-0000-000000000081"
		termTwo = "70700000-0000-0000-0000-000000000082"
		enr     = "e7000000-0000-0000-0000-000000000081"
		stu     = "57700000-0000-0000-0000-000000000081"
		t1v1    = "a7000000-0000-0000-0000-000000000811"
		t1v2    = "a7000000-0000-0000-0000-000000000812"
		t2v1    = "a7000000-0000-0000-0000-000000000821"
		t2v2    = "a7000000-0000-0000-0000-000000000822"
	)
	repo := appRepo(t, tenantA)
	ctx := context.Background()
	issue := func(term string, cards ...domain.ReportCard) {
		require.NoError(t, repo.WithPeriodLock(ctx, term, func(ctx context.Context, tx ports.PeriodTx) error {
			return tx.IssueReportCards(ctx, cards)
		}))
	}
	issue(termOne, reportCardOfTerm(t, t1v1, tenantA, termOne, 1, enr, stu, 1, nil))
	issue(termTwo, reportCardOfTerm(t, t2v1, tenantA, termTwo, 2, enr, stu, 1, nil))
	issue(termTwo, reportCardOfTerm(t, t2v2, tenantA, termTwo, 2, enr, stu, 2, nil))
	issue(termOne, reportCardOfTerm(t, t1v2, tenantA, termOne, 1, enr, stu, 2, nil))

	cards, err := repo.LatestReportCards(ctx, ports.ReportCardFilter{StudentID: stu})
	require.NoError(t, err)

	ordinalByID := map[string]int{}
	for _, c := range cards {
		ordinalByID[c.ID()] = c.Header().TermOrdinal
	}
	assert.Equal(t, map[string]int{t1v2: 1, t2v2: 2}, ordinalByID)
}
