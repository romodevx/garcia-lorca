//go:build integration

// Integration tests for grades' half of the level trash (ADR 0021): a deleted
// level's monthly grades, legacy entries, period closures, report cards (+
// lines) and outbox rows move to grades_deletion_archive and back; retries are
// idempotent; conflicts are skipped and reported; the purge honours the
// retention; RLS keeps one school's trash from another.
package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/garcia-lorca/pkg/levelarchive"
)

// Tenant G owns every row of this file, so the suite's other tests never see it.
const tenantG = "77777777-7777-7777-7777-777777777777"

// The doomed level's ids (…a*) and a bystander level's (…b*).
const (
	laLevel      = "c0000000-0000-0000-0000-0000000000a1"
	laGroup      = "c0000000-0000-0000-0000-0000000000a2"
	laSubject    = "c0000000-0000-0000-0000-0000000000a3"
	laTerm       = "c0000000-0000-0000-0000-0000000000a4"
	laEnrollment = "c0000000-0000-0000-0000-0000000000a5"
	laCard       = "c0000000-0000-0000-0000-0000000000a6"
	byLevel      = "c0000000-0000-0000-0000-0000000000b1"
	byGroup      = "c0000000-0000-0000-0000-0000000000b2"
	bySubject    = "c0000000-0000-0000-0000-0000000000b3"
	byTerm       = "c0000000-0000-0000-0000-0000000000b4"
	byEnrollment = "c0000000-0000-0000-0000-0000000000b5"
	byCard       = "c0000000-0000-0000-0000-0000000000b6"
	laCycle      = "c0000000-0000-0000-0000-0000000000c1"
	laPerson     = "c0000000-0000-0000-0000-0000000000c2"
)

var laScope = levelarchive.Scope{
	LevelID: laLevel, GroupIDs: []string{laGroup}, SubjectIDs: []string{laSubject},
	TermIDs: []string{laTerm}, EnrollmentIDs: []string{laEnrollment},
}

var wantGradesCounts = levelarchive.Counts{
	"monthlyGrades": 1, "gradeEntries": 1, "periodClosures": 1,
	"reportCards": 1, "reportCardLines": 1, "gradesOutbox": 1,
}

// gradesTables are the tables the level trash reaches, with the tenant filter.
var gradesTables = []string{
	"report_card_lines", "report_cards", "period_closures", "monthly_grades", "grade_entries", "grades_outbox",
}

// seedGradesLevel wipes tenant G and builds one row of every kind for the
// doomed level and for the bystander, as the superuser.
func seedGradesLevel(t *testing.T) {
	t.Helper()
	for _, table := range append([]string{"grades_deletion_archive"}, gradesTables...) {
		require.NoError(t, superDB.Exec("DELETE FROM "+table+" WHERE tenant_id = ?", tenantG).Error)
	}
	for _, s := range []struct{ enr, subj, term, level, group, card string }{
		{laEnrollment, laSubject, laTerm, laLevel, laGroup, laCard},
		{byEnrollment, bySubject, byTerm, byLevel, byGroup, byCard},
	} {
		for _, stmt := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO monthly_grades (tenant_id, enrollment_id, subject_id, teacher_id, month, value, captured_by)
			  VALUES (?, ?, ?, ?, '2026-09-01', 8.5, ?)`, []any{tenantG, s.enr, s.subj, laPerson, laPerson}},
			{`INSERT INTO grade_entries (tenant_id, enrollment_id, subject_id, term_id, teacher_id, value)
			  VALUES (?, ?, ?, ?, ?, 9)`, []any{tenantG, s.enr, s.subj, s.term, laPerson}},
			{`INSERT INTO period_closures (tenant_id, term_id, cycle_id, level_id, status, version, closed_at)
			  VALUES (?, ?, ?, ?, 'closed', 1, now())`, []any{tenantG, s.term, laCycle, s.level}},
			{`INSERT INTO report_cards (id, tenant_id, term_id, cycle_id, level_id, group_id, enrollment_id, student_id,
			    version, status, issued_at, issued_by, student_name, months, scale_min_hundredths, scale_max_hundredths,
			    scale_decimals, scale_passing_hundredths)
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, 'published', now(), ?, 'Ana', '{2026-09}', 500, 1000, 1, 600)`,
				[]any{s.card, tenantG, s.term, laCycle, s.level, s.group, s.enr, laPerson, laPerson}},
			{`INSERT INTO report_card_lines (report_card_id, tenant_id, subject_id, subject_name, position)
			  VALUES (?, ?, ?, 'Matemáticas', 1)`, []any{s.card, tenantG, s.subj}},
			{`INSERT INTO grades_outbox (tenant_id, event_type, payload)
			  VALUES (?, 'GradesPublished', jsonb_build_object('enrollmentId', ?::text, 'termId', ?::text))`,
				[]any{tenantG, s.enr, s.term}},
		} {
			require.NoError(t, superDB.Exec(stmt.sql, stmt.args...).Error, stmt.sql)
		}
	}
}

// gradesSnapshot counts tenant G's rows per table, RLS-blind.
func gradesSnapshot(t *testing.T) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	for _, table := range gradesTables {
		var n int64
		require.NoError(t, superDB.Raw("SELECT count(*) FROM "+table+" WHERE tenant_id = ?", tenantG).Scan(&n).Error)
		out[table] = n
	}
	return out
}

func TestGradesLevelPreviewCountsWithoutChangingAnything(t *testing.T) {
	seedGradesLevel(t)
	before := gradesSnapshot(t)

	got, err := appRepo(t, tenantG).PreviewLevelDeletion(context.Background(), laScope)
	require.NoError(t, err)
	assert.Equal(t, wantGradesCounts, got)
	assert.Equal(t, before, gradesSnapshot(t))
}

func TestGradesLevelArchiveRestoreRoundTrip(t *testing.T) {
	seedGradesLevel(t)
	ctx := context.Background()
	repo := appRepo(t, tenantG)
	const batch = "c0000000-0000-0000-0000-00000000f001"
	before := gradesSnapshot(t)

	counts, err := repo.ArchiveLevel(ctx, batch, laScope)
	require.NoError(t, err)
	assert.Equal(t, wantGradesCounts, counts)
	after := gradesSnapshot(t)
	for _, table := range gradesTables {
		assert.Equal(t, before[table]-1, after[table], "%s lost exactly the doomed level's row", table)
	}

	// A retry of the same batch answers the first counts and moves nothing new,
	// even if a new in-scope grade was captured meanwhile.
	require.NoError(t, superDB.Exec(`INSERT INTO monthly_grades (tenant_id, enrollment_id, subject_id, teacher_id, month, value, captured_by)
		VALUES (?, ?, ?, ?, '2026-10-01', 7, ?)`, tenantG, laEnrollment, laSubject, laPerson, laPerson).Error)
	again, err := repo.ArchiveLevel(ctx, batch, laScope)
	require.NoError(t, err)
	assert.Equal(t, counts, again)
	assert.Equal(t, after["monthly_grades"]+1, gradesSnapshot(t)["monthly_grades"])
	require.NoError(t, superDB.Exec("DELETE FROM monthly_grades WHERE tenant_id = ? AND month = '2026-10-01'", tenantG).Error)

	rep, err := repo.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	assert.Equal(t, wantGradesCounts, rep.Restored)
	assert.Empty(t, rep.Skipped)
	assert.Equal(t, before, gradesSnapshot(t))

	rep2, err := repo.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err, "a retried restore answers its first report")
	assert.Equal(t, rep.Restored, rep2.Restored)
	assert.Equal(t, before, gradesSnapshot(t))
}

func TestGradesLevelRestoreSkipsAndReportsConflicts(t *testing.T) {
	seedGradesLevel(t)
	ctx := context.Background()
	repo := appRepo(t, tenantG)
	const batch = "c0000000-0000-0000-0000-00000000f002"
	_, err := repo.ArchiveLevel(ctx, batch, laScope)
	require.NoError(t, err)

	// While the level sat in the trash, the same cell was captured again.
	require.NoError(t, superDB.Exec(`INSERT INTO monthly_grades (tenant_id, enrollment_id, subject_id, teacher_id, month, value, captured_by)
		VALUES (?, ?, ?, ?, '2026-09-01', 6, ?)`, tenantG, laEnrollment, laSubject, laPerson, laPerson).Error)

	rep, err := repo.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	assert.Equal(t, int64(0), rep.Restored["monthlyGrades"])
	require.Len(t, rep.Skipped, 1)
	assert.Equal(t, "monthlyGrades", rep.Skipped[0].Kind)
	assert.Equal(t, int64(1), rep.Skipped[0].Count)
	assert.NotEmpty(t, rep.Skipped[0].Reason)
	assert.Equal(t, int64(1), rep.Restored["reportCards"], "the rest came back")

	var value float64
	require.NoError(t, superDB.Raw("SELECT value FROM monthly_grades WHERE tenant_id = ? AND enrollment_id = ?",
		tenantG, laEnrollment).Scan(&value).Error)
	assert.Equal(t, 6.0, value, "the newer capture wins")
}

func TestGradesLevelPurgeHonoursTheRetention(t *testing.T) {
	seedGradesLevel(t)
	ctx := context.Background()
	repo := appRepo(t, tenantG)
	const batch = "c0000000-0000-0000-0000-00000000f003"
	_, err := repo.ArchiveLevel(ctx, batch, laScope)
	require.NoError(t, err)

	purged, err := repo.PurgeLevelArchives(ctx, levelarchive.Cutoff(time.Now()))
	require.NoError(t, err)
	assert.Zero(t, purged, "fresh trash is never purged")

	require.NoError(t, superDB.Exec("UPDATE grades_deletion_archive SET created_at = now() - interval '8 days' WHERE batch_id = ?", batch).Error)
	purged, err = repo.PurgeLevelArchives(ctx, levelarchive.Cutoff(time.Now()))
	require.NoError(t, err)
	assert.Positive(t, purged)
	var left int64
	require.NoError(t, superDB.Raw("SELECT count(*) FROM grades_deletion_archive WHERE batch_id = ?", batch).Scan(&left).Error)
	assert.Zero(t, left)
}

func TestGradesLevelTrashIsTenantIsolated(t *testing.T) {
	seedGradesLevel(t)
	ctx := context.Background()
	const batch = "c0000000-0000-0000-0000-00000000f004"
	_, err := appRepo(t, tenantG).ArchiveLevel(ctx, batch, laScope)
	require.NoError(t, err)
	afterArchive := gradesSnapshot(t)

	other := appRepo(t, tenantB)
	counts, err := other.PreviewLevelDeletion(ctx, laScope)
	require.NoError(t, err)
	for kind, n := range counts {
		assert.Zero(t, n, "tenant B sees none of G's %s", kind)
	}
	rep, err := other.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	assert.Empty(t, rep.Restored, "B cannot restore G's batch")
	assert.Equal(t, afterArchive, gradesSnapshot(t))

	db, err := open(appDSN)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	var visible int64
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantB).Error; err != nil {
			return err
		}
		return tx.Raw("SELECT count(*) FROM grades_deletion_archive").Scan(&visible).Error
	}))
	assert.Zero(t, visible, "RLS hides G's archive rows from B")
}
