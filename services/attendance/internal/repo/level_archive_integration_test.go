//go:build integration

// Integration tests for attendance's half of the level-deletion trash (ADR
// 0021): the level's marks and pass lists move into attendance_deletion_archive
// and back, a retried batch changes nothing more, a conflict is skipped and
// reported, the purge honours the retention and RLS keeps schools apart.
package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/attendance/internal/repo"
)

// Tenant C owns every row of this file.
const tenantC = "33333333-3333-3333-3333-333333333333"

const (
	laGroup      = "c0000000-0000-0000-0000-0000000000a1" // a group of the doomed level
	laSubject    = "c0000000-0000-0000-0000-0000000000a2" // a subject of the doomed level
	laEnrollment = "c0000000-0000-0000-0000-0000000000a3" // an enrollment in laGroup
	laMovedOut   = "c0000000-0000-0000-0000-0000000000a4" // moved to another level; old mark kept laGroup
	otherGroup   = "c0000000-0000-0000-0000-0000000000b1"
	otherEnroll  = "c0000000-0000-0000-0000-0000000000b3"
	laTaker      = "c0000000-0000-0000-0000-0000000000cc"
)

func laScope() levelarchive.Scope {
	return levelarchive.Scope{
		LevelID:       "c0000000-0000-0000-0000-0000000000f0",
		GroupIDs:      []string{laGroup},
		SubjectIDs:    []string{laSubject},
		EnrollmentIDs: []string{laEnrollment},
	}
}

// archiveRepo builds the repository as the non-owner app role, closing its
// pool with the test.
func archiveRepo(t *testing.T, tenant string) (*repo.Repo, *gorm.DB) {
	t.Helper()
	db, err := open(appDSN)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return repo.New(db, tenant), db
}

// seedLevelArchive wipes tenant C and seeds, as the superuser:
//   - laEnrollment: a daily mark in laGroup and a per-class mark of laSubject;
//   - laMovedOut: an old mark stamped with laGroup (it left the level);
//   - otherEnroll: a mark in otherGroup (a bystander level);
//   - two sessions of laGroup (daily + laSubject) and one of otherGroup.
func seedLevelArchive(t *testing.T) {
	t.Helper()
	for _, stmt := range []string{
		"DELETE FROM attendance_deletion_archive WHERE tenant_id = ?",
		"DELETE FROM attendance WHERE tenant_id = ?",
		"DELETE FROM attendance_sessions WHERE tenant_id = ?",
	} {
		require.NoError(t, superDB.Exec(stmt, tenantC).Error)
	}
	marks := `INSERT INTO attendance (tenant_id, enrollment_id, date, status, group_id, subject_id) VALUES
		(?, ?, '2026-09-01', 'absent', ?, NULL),
		(?, ?, '2026-09-01', 'late', ?, ?),
		(?, ?, '2026-08-31', 'absent', ?, NULL),
		(?, ?, '2026-09-01', 'absent', ?, NULL)`
	require.NoError(t, superDB.Exec(marks,
		tenantC, laEnrollment, laGroup,
		tenantC, laEnrollment, laGroup, laSubject,
		tenantC, laMovedOut, laGroup,
		tenantC, otherEnroll, otherGroup).Error)
	sessions := `INSERT INTO attendance_sessions (tenant_id, group_id, date, subject_id, taken_by, taken_by_name) VALUES
		(?, ?, '2026-09-01', NULL, ?, 'Gabriela'),
		(?, ?, '2026-09-01', ?, ?, 'Gabriela'),
		(?, ?, '2026-09-01', NULL, ?, 'Gabriela')`
	require.NoError(t, superDB.Exec(sessions,
		tenantC, laGroup, laTaker,
		tenantC, laGroup, laSubject, laTaker,
		tenantC, otherGroup, laTaker).Error)
}

func countC(t *testing.T, table, where string, args ...any) int64 {
	t.Helper()
	var n int64
	q := "SELECT count(*) FROM " + table + " WHERE tenant_id = '" + tenantC + "'"
	if where != "" {
		q += " AND " + where
	}
	require.NoError(t, superDB.Raw(q, args...).Scan(&n).Error)
	return n
}

var wantAttendanceCounts = levelarchive.Counts{"attendanceMarks": 3, "attendanceSessions": 2}

func TestLevelArchivePreviewCountsWithoutChanging(t *testing.T) {
	seedLevelArchive(t)
	r, _ := archiveRepo(t, tenantC)

	got, err := r.PreviewLevelDeletion(context.Background(), laScope())
	require.NoError(t, err)
	assert.Equal(t, wantAttendanceCounts, got)
	assert.Equal(t, int64(4), countC(t, "attendance", ""), "a preview changes nothing")

	empty, err := r.PreviewLevelDeletion(context.Background(), levelarchive.Scope{LevelID: laScope().LevelID})
	require.NoError(t, err)
	assert.Equal(t, levelarchive.Counts{"attendanceMarks": 0, "attendanceSessions": 0}, empty,
		"an empty scope reaches nothing, and every kind is still reported")
}

func TestLevelArchiveRoundTripIsIdempotent(t *testing.T) {
	seedLevelArchive(t)
	ctx := context.Background()
	r, _ := archiveRepo(t, tenantC)
	const batch = "c0000000-0000-0000-0000-00000000b001"

	counts, err := r.ArchiveLevel(ctx, batch, laScope())
	require.NoError(t, err)
	assert.Equal(t, wantAttendanceCounts, counts)
	assert.Equal(t, int64(1), countC(t, "attendance", ""), "only the bystander's mark stays")
	assert.Equal(t, int64(1), countC(t, "attendance", "enrollment_id = ?", otherEnroll))
	assert.Equal(t, int64(1), countC(t, "attendance_sessions", "group_id = ?", otherGroup))

	// A mark taken in the bystander group after the deletion must not be swept
	// up by a retry of the same batch.
	require.NoError(t, superDB.Exec(`INSERT INTO attendance (tenant_id, enrollment_id, date, status, group_id)
		VALUES (?, ?, '2026-09-02', 'absent', ?)`, tenantC, laEnrollment, laGroup).Error)
	again, err := r.ArchiveLevel(ctx, batch, laScope())
	require.NoError(t, err)
	assert.Equal(t, counts, again, "a retried batch answers its first counts")
	assert.Equal(t, int64(1), countC(t, "attendance", "date = '2026-09-02'"), "and moves nothing more")
	require.NoError(t, superDB.Exec("DELETE FROM attendance WHERE tenant_id = ? AND date = '2026-09-02'", tenantC).Error)

	rep, err := r.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	assert.Equal(t, wantAttendanceCounts, rep.Restored)
	assert.Empty(t, rep.Skipped)
	assert.Equal(t, int64(4), countC(t, "attendance", ""))
	assert.Equal(t, int64(3), countC(t, "attendance_sessions", ""))
	assert.Equal(t, int64(1), countC(t, "attendance", "enrollment_id = ? AND subject_id = ? AND status = 'late'", laEnrollment, laSubject),
		"a restored mark keeps every column")

	rep2, err := r.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	assert.Equal(t, rep.Restored, rep2.Restored, "a retried restore answers its first report")
	assert.Equal(t, int64(4), countC(t, "attendance", ""))
}

func TestLevelArchiveRestoreSkipsAndReportsConflicts(t *testing.T) {
	seedLevelArchive(t)
	ctx := context.Background()
	r, _ := archiveRepo(t, tenantC)
	const batch = "c0000000-0000-0000-0000-00000000b002"
	_, err := r.ArchiveLevel(ctx, batch, laScope())
	require.NoError(t, err)

	// While the level sat in the trash, the same list was taken again.
	require.NoError(t, superDB.Exec(`INSERT INTO attendance (tenant_id, enrollment_id, date, status, group_id)
		VALUES (?, ?, '2026-09-01', 'excused', ?)`, tenantC, laEnrollment, laGroup).Error)
	require.NoError(t, superDB.Exec(`INSERT INTO attendance_sessions (tenant_id, group_id, date, subject_id, taken_by, taken_by_name)
		VALUES (?, ?, '2026-09-01', NULL, ?, 'Otra')`, tenantC, laGroup, laTaker).Error)

	rep, err := r.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	assert.Equal(t, int64(2), rep.Restored["attendanceMarks"])
	assert.Equal(t, int64(1), rep.Restored["attendanceSessions"])
	require.Len(t, rep.Skipped, 2)
	for _, s := range rep.Skipped {
		assert.Equal(t, int64(1), s.Count, s.Kind)
		assert.NotEmpty(t, s.Reason)
	}
	assert.Equal(t, int64(1), countC(t, "attendance", "enrollment_id = ? AND date = '2026-09-01' AND subject_id IS NULL AND status = 'excused'", laEnrollment),
		"the newer mark wins")
}

func TestLevelArchivePurgeHonoursTheRetention(t *testing.T) {
	seedLevelArchive(t)
	ctx := context.Background()
	r, _ := archiveRepo(t, tenantC)
	const batch = "c0000000-0000-0000-0000-00000000b003"
	_, err := r.ArchiveLevel(ctx, batch, laScope())
	require.NoError(t, err)

	n, err := r.PurgeLevelArchives(ctx, levelarchive.Cutoff(time.Now()))
	require.NoError(t, err)
	assert.Zero(t, n, "fresh trash is never purged")

	require.NoError(t, superDB.Exec("UPDATE attendance_deletion_archive SET created_at = now() - interval '8 days' WHERE batch_id = ?", batch).Error)
	n, err = r.PurgeLevelArchives(ctx, levelarchive.Cutoff(time.Now()))
	require.NoError(t, err)
	assert.Positive(t, n)
	assert.Zero(t, countC(t, "attendance_deletion_archive", "batch_id = ?", batch))

	rep, err := r.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	assert.Empty(t, rep.Restored, "a purged batch restores nothing")
}

func TestLevelArchiveIsTenantIsolated(t *testing.T) {
	seedLevelArchive(t)
	ctx := context.Background()
	rC, _ := archiveRepo(t, tenantC)
	const batch = "c0000000-0000-0000-0000-00000000b004"
	_, err := rC.ArchiveLevel(ctx, batch, laScope())
	require.NoError(t, err)

	rB, dbB := archiveRepo(t, tenantB)
	rep, err := rB.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	assert.Empty(t, rep.Restored, "B cannot restore C's batch")
	assert.Equal(t, int64(1), countC(t, "attendance", ""), "C's marks stay in C's trash")

	counts, err := rB.ArchiveLevel(ctx, "c0000000-0000-0000-0000-00000000b005", laScope())
	require.NoError(t, err)
	assert.Equal(t, levelarchive.Counts{"attendanceMarks": 0, "attendanceSessions": 0}, counts, "B reaches none of C's rows")
	assert.Equal(t, int64(1), countC(t, "attendance", "enrollment_id = ?", otherEnroll))

	var visible int64
	require.NoError(t, dbB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantB).Error; err != nil {
			return err
		}
		return tx.Raw("SELECT count(*) FROM attendance_deletion_archive WHERE batch_id = ?", batch).Scan(&visible).Error
	}))
	assert.Zero(t, visible, "RLS hides C's archive rows from B")
}
