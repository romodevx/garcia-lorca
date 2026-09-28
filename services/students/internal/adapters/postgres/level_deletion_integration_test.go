//go:build integration

// Integration tests for students' half of the level trash (ADR 0021): the
// enrollments in the level's groups move to students_deletion_archive and back,
// the pupils are KEPT with their level cleared and restored, conflicts are
// skipped and reported, the purge honours the retention, and RLS keeps one
// school's trash from another.
package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
)

// Tenant L owns every row of this file, so the suite's other tests never see it.
const tenantL = "77777777-7777-7777-7777-777777777777"

const (
	ldLevel   = "e0000000-0000-0000-0000-0000000000a1"
	ldNewLvl  = "e0000000-0000-0000-0000-0000000000a2"
	ldGroup   = "e0000000-0000-0000-0000-0000000000b1"
	ldOther   = "e0000000-0000-0000-0000-0000000000b2"
	ldCycle   = "e0000000-0000-0000-0000-0000000000c1"
	ldStuA    = "e0000000-0000-0000-0000-0000000000d1"
	ldStuB    = "e0000000-0000-0000-0000-0000000000d2"
	ldStuC    = "e0000000-0000-0000-0000-0000000000d3" // another level, untouched
	ldEnrA    = "e0000000-0000-0000-0000-0000000000e1"
	ldEnrB    = "e0000000-0000-0000-0000-0000000000e2"
	ldEnrC    = "e0000000-0000-0000-0000-0000000000e3"
	ldTransfA = "e0000000-0000-0000-0000-0000000000f1"
)

func ldScope() levelarchive.Scope {
	return levelarchive.Scope{LevelID: ldLevel, GroupIDs: []string{ldGroup}}
}

// seedLevelPeople rebuilds tenant L: two pupils of the doomed level enrolled in
// its group (one with a transfer history), a guardian contact, and a third pupil
// of another level in another group.
func seedLevelPeople(t *testing.T) {
	t.Helper()
	for _, stmt := range []string{
		"DELETE FROM students_deletion_archive WHERE tenant_id = @t",
		"DELETE FROM enrollment_transfers WHERE tenant_id = @t",
		"DELETE FROM enrollments WHERE tenant_id = @t",
		"DELETE FROM guardian_students WHERE tenant_id = @t",
		"DELETE FROM students WHERE tenant_id = @t",
	} {
		require.NoError(t, superDB.Exec(stmt, map[string]any{"t": tenantL}).Error, stmt)
	}
	seed := []string{
		`INSERT INTO students (id, tenant_id, curp, first_name, last_name, level_id, level_name, grade_group) VALUES
			('` + ldStuA + `', @t, 'LDAA050312MDFRNN01', 'Ana', 'Uno', '` + ldLevel + `', 'Primaria', '1° A'),
			('` + ldStuB + `', @t, 'LDBB050312MDFRNN01', 'Beto', 'Dos', '` + ldLevel + `', 'Primaria', '1° A'),
			('` + ldStuC + `', @t, 'LDCC050312MDFRNN01', 'Caro', 'Tres', '` + ldNewLvl + `', 'Secundaria', '1° A')`,
		`INSERT INTO enrollments (id, tenant_id, student_id, cycle_id, group_id) VALUES
			('` + ldEnrA + `', @t, '` + ldStuA + `', '` + ldCycle + `', '` + ldGroup + `'),
			('` + ldEnrB + `', @t, '` + ldStuB + `', '` + ldCycle + `', '` + ldGroup + `'),
			('` + ldEnrC + `', @t, '` + ldStuC + `', '` + ldCycle + `', '` + ldOther + `')`,
		`INSERT INTO enrollment_transfers (id, tenant_id, enrollment_id, from_group_id, to_group_id, reason) VALUES
			('` + ldTransfA + `', @t, '` + ldEnrA + `', '` + ldOther + `', '` + ldGroup + `', 'cambio')`,
		`INSERT INTO guardian_students (tenant_id, student_id, full_name) VALUES (@t, '` + ldStuA + `', 'Mamá de Ana')`,
	}
	for _, stmt := range seed {
		require.NoError(t, superDB.Exec(stmt, map[string]any{"t": tenantL}).Error, stmt)
	}
}

func countL(t *testing.T, table, where string, args ...any) int64 {
	t.Helper()
	var n int64
	q := "SELECT count(*) FROM " + table + " WHERE tenant_id = '" + tenantL + "'"
	if where != "" {
		q += " AND " + where
	}
	require.NoError(t, superDB.Raw(q, args...).Scan(&n).Error)
	return n
}

type pupilLevel struct {
	LevelID    *string
	LevelName  string
	Level      *string
	GradeGroup string
}

func levelOf(t *testing.T, id string) pupilLevel {
	t.Helper()
	var p pupilLevel
	require.NoError(t, superDB.Raw("SELECT level_id::text AS level_id, level_name, level, grade_group FROM students WHERE id = ?", id).Scan(&p).Error)
	return p
}

var wantPeopleCounts = levelarchive.Counts{"students": 2, "enrollments": 2, "enrollmentTransfers": 1}

func TestLevelDeletionPreviewCountsThePeopleAndChangesNothing(t *testing.T) {
	seedLevelPeople(t)
	repo := appRepo(t, tenantL)

	got, err := repo.PreviewLevelDeletion(context.Background(), ldScope())
	require.NoError(t, err)
	assert.Equal(t, wantPeopleCounts, got.Counts)
	assert.Equal(t, []string{ldStuA, ldStuB}, got.StudentIDs)
	assert.Equal(t, []string{ldEnrA, ldEnrB}, got.EnrollmentIDs)
	assert.Equal(t, int64(3), countL(t, "enrollments", ""), "a preview changes nothing")
}

func TestLevelDeletionArchiveKeepsPupilsAndRestoresThem(t *testing.T) {
	seedLevelPeople(t)
	ctx := context.Background()
	repo := appRepo(t, tenantL)
	const batch = "e0000000-0000-0000-0000-00000000b001"

	counts, err := repo.ArchiveLevel(ctx, batch, ldScope())
	require.NoError(t, err)
	assert.Equal(t, wantPeopleCounts, counts)

	assert.Equal(t, int64(3), countL(t, "students", ""), "pupils are kept")
	a := levelOf(t, ldStuA)
	assert.Nil(t, a.LevelID, "sin nivel")
	assert.Empty(t, a.LevelName)
	assert.Nil(t, a.Level)
	assert.Empty(t, a.GradeGroup, "sin grado ni grupo")
	assert.Equal(t, int64(1), countL(t, "guardian_students", "student_id = ?", ldStuA), "contacts untouched")
	assert.Zero(t, countL(t, "enrollments", "group_id = ?", ldGroup))
	assert.Zero(t, countL(t, "enrollment_transfers", ""))
	assert.Equal(t, int64(1), countL(t, "enrollments", "id = ?", ldEnrC), "another level's enrollment stays")
	assert.Equal(t, "Secundaria", levelOf(t, ldStuC).LevelName)

	again, err := repo.ArchiveLevel(ctx, batch, ldScope())
	require.NoError(t, err, "a retried step answers its first result")
	assert.Equal(t, counts, again)

	res, err := repo.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	rep := res.Report
	assert.Equal(t, wantPeopleCounts, rep.Restored)
	assert.Empty(t, rep.Skipped)
	assert.Empty(t, res.UnrestoredEnrollmentIDs)
	assert.Zero(t, countL(t, "students_deletion_archive", "batch_id = ? AND table_name <> '_batch'", batch),
		"everything went back, the pupils' snapshot included")
	a = levelOf(t, ldStuA)
	require.NotNil(t, a.LevelID)
	assert.Equal(t, ldLevel, *a.LevelID)
	assert.Equal(t, "Primaria", a.LevelName)
	assert.Equal(t, "1° A", a.GradeGroup)
	assert.Equal(t, int64(2), countL(t, "enrollments", "group_id = ?", ldGroup))
	assert.Equal(t, int64(1), countL(t, "enrollment_transfers", "id = ?", ldTransfA))

	rep2, err := repo.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err, "a retried restore answers its first report")
	assert.Equal(t, rep.Restored, rep2.Report.Restored)
}

func TestLevelDeletionRestoreKeepsWhatChangedMeanwhile(t *testing.T) {
	seedLevelPeople(t)
	ctx := context.Background()
	repo := appRepo(t, tenantL)
	const batch = "e0000000-0000-0000-0000-00000000b002"
	_, err := repo.ArchiveLevel(ctx, batch, ldScope())
	require.NoError(t, err)

	// While the level sat in the trash: Ana got a new level, Beto was
	// re-enrolled in another group of the same cycle.
	require.NoError(t, superDB.Exec("UPDATE students SET level_id = ?, level_name = 'Secundaria' WHERE id = ?", ldNewLvl, ldStuA).Error)
	require.NoError(t, superDB.Exec(`INSERT INTO enrollments (tenant_id, student_id, cycle_id, group_id) VALUES (?, ?, ?, ?)`,
		tenantL, ldStuB, ldCycle, ldOther).Error)

	res, err := repo.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	rep := res.Report
	assert.Equal(t, []string{ldEnrB}, res.UnrestoredEnrollmentIDs,
		"grades and attendance must keep Beto's enrollment's rows archived")
	assert.Equal(t, int64(1), rep.Restored["students"])
	assert.Equal(t, int64(1), rep.Restored["enrollments"])
	skipped := map[string]int64{}
	for _, s := range rep.Skipped {
		assert.NotEmpty(t, s.Reason)
		skipped[s.Kind] = s.Count
	}
	assert.Equal(t, map[string]int64{"students": 1, "enrollments": 1}, skipped)
	assert.Equal(t, "Secundaria", levelOf(t, ldStuA).LevelName, "the new level wins")
	assert.Equal(t, int64(1), countL(t, "enrollments", "student_id = ? AND group_id = ?", ldStuB, ldOther), "the new enrollment wins")
	assert.Equal(t, int64(1), countL(t, "students_deletion_archive", "batch_id = ? AND table_name = 'enrollments'", batch),
		"the skipped enrollment stays in the trash")

	// The pupils' snapshot is final ("keep the new value"): a retry never
	// re-applies it, even to a pupil whose level was cleared again since.
	require.NoError(t, superDB.Exec("UPDATE students SET level_id = NULL, level_name = '' WHERE id = ?", ldStuA).Error)
	again, err := repo.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	assert.Nil(t, levelOf(t, ldStuA).LevelID, "the snapshot was consumed by the first restore")
	assert.Equal(t, []string{ldEnrB}, again.UnrestoredEnrollmentIDs)

	// Once the clash is gone, the retry brings Beto's enrollment back.
	require.NoError(t, superDB.Exec("DELETE FROM enrollments WHERE student_id = ? AND group_id = ?", ldStuB, ldOther).Error)
	last, err := repo.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	assert.Empty(t, last.UnrestoredEnrollmentIDs)
	assert.Equal(t, int64(1), countL(t, "enrollments", "id = ?", ldEnrB))
	assert.Zero(t, countL(t, "students_deletion_archive", "batch_id = ? AND table_name <> '_batch'", batch))
}

func TestLevelDeletionPurgeBatchTouchesOnlyThatBatch(t *testing.T) {
	seedLevelPeople(t)
	ctx := context.Background()
	repo := appRepo(t, tenantL)
	const doomed, kept = "e0000000-0000-0000-0000-00000000b006", "e0000000-0000-0000-0000-00000000b007"
	_, err := repo.ArchiveLevel(ctx, doomed, ldScope())
	require.NoError(t, err)
	_, err = repo.ArchiveLevel(ctx, kept, levelarchive.Scope{LevelID: ldNewLvl, GroupIDs: []string{ldOther}})
	require.NoError(t, err)

	n, err := repo.PurgeLevelBatch(ctx, doomed)
	require.NoError(t, err)
	assert.Positive(t, n)
	assert.Zero(t, countL(t, "students_deletion_archive", "batch_id = ?", doomed))
	assert.Positive(t, countL(t, "students_deletion_archive", "batch_id = ?", kept))

	_, err = repo.RestoreLevelBatch(ctx, doomed)
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err), "a purged batch is NotFound, never an empty success")
	n, err = repo.PurgeLevelBatch(ctx, doomed)
	require.NoError(t, err)
	assert.Zero(t, n, "purging twice is harmless")
}

func TestLevelDeletionPurgeHonoursTheRetention(t *testing.T) {
	seedLevelPeople(t)
	ctx := context.Background()
	repo := appRepo(t, tenantL)
	const batch = "e0000000-0000-0000-0000-00000000b003"
	_, err := repo.ArchiveLevel(ctx, batch, ldScope())
	require.NoError(t, err)

	cutoff := levelarchive.Cutoff(time.Now())
	purged, err := repo.PurgeLevelArchives(ctx, cutoff)
	require.NoError(t, err)
	assert.Zero(t, purged, "fresh trash is never purged")

	require.NoError(t, superDB.Exec("UPDATE students_deletion_archive SET created_at = now() - interval '8 days' WHERE batch_id = ?", batch).Error)
	purged, err = repo.PurgeLevelArchives(ctx, cutoff)
	require.NoError(t, err)
	assert.Positive(t, purged)
	assert.Zero(t, countL(t, "students_deletion_archive", "batch_id = ?", batch))
}

func TestLevelDeletionTrashIsTenantIsolated(t *testing.T) {
	seedLevelPeople(t)
	ctx := context.Background()
	const batch = "e0000000-0000-0000-0000-00000000b004"
	_, err := appRepo(t, tenantL).ArchiveLevel(ctx, batch, ldScope())
	require.NoError(t, err)

	repoB := appRepo(t, tenantB)
	_, err = repoB.RestoreLevelBatch(ctx, batch)
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err), "B cannot restore L's batch: to B it does not exist")
	assert.Nil(t, levelOf(t, ldStuA).LevelID, "L's pupil is still cleared")

	counts, err := repoB.ArchiveLevel(ctx, "e0000000-0000-0000-0000-00000000b005", levelarchive.Scope{LevelID: ldNewLvl, GroupIDs: []string{ldOther}})
	require.NoError(t, err)
	assert.Zero(t, counts["enrollments"]+counts["students"], "B cannot reach L's rows")
	assert.Equal(t, int64(1), countL(t, "enrollments", "id = ?", ldEnrC))

	db, err := open(appDSN)
	require.NoError(t, err)
	t.Cleanup(func() {
		if s, err := db.DB(); err == nil {
			_ = s.Close()
		}
	})
	var visible int64
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantB).Error; err != nil {
			return err
		}
		return tx.Raw("SELECT count(*) FROM students_deletion_archive WHERE batch_id = ?", batch).Scan(&visible).Error
	}))
	assert.Zero(t, visible, "RLS hides L's archive rows from B")
}
