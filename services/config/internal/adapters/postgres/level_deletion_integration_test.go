//go:build integration

// Integration tests for deleting a level into config's trash (ADR 0021): the
// structure moves to config_deletion_archive and back, the batch is recorded in
// level_deletions, conflicts are skipped and reported, the purge honours the
// retention, and RLS keeps one school's trash from another.
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
	pgadapter "github.com/garcia-lorca/services/config/internal/adapters/postgres"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
)

// Tenant F owns every row of this file, so the suite's other tests never see it.
const tenantF = "66666666-6666-6666-6666-666666666666"

// The doomed level L and a bystander level M, with a full structure each.
const (
	ldCycle    = "f0000000-0000-0000-0000-000000000001"
	ldLevel    = "f0000000-0000-0000-0000-0000000000a1"
	ldGrade    = "f0000000-0000-0000-0000-0000000000a2"
	ldSubject  = "f0000000-0000-0000-0000-0000000000a3"
	ldGroup    = "f0000000-0000-0000-0000-0000000000a4"
	ldTerm     = "f0000000-0000-0000-0000-0000000000a5"
	ldTeacher  = "f0000000-0000-0000-0000-0000000000a6"
	ldAssign   = "f0000000-0000-0000-0000-0000000000a7"
	ldEvLevel  = "f0000000-0000-0000-0000-0000000000a8"
	ldEvGroup  = "f0000000-0000-0000-0000-0000000000a9"
	ldEvSchool = "f0000000-0000-0000-0000-0000000000aa"
	mLevel     = "f0000000-0000-0000-0000-0000000000b1"
	mGrade     = "f0000000-0000-0000-0000-0000000000b2"
	mGroup     = "f0000000-0000-0000-0000-0000000000b4"
	deleter    = "f0000000-0000-0000-0000-0000000000cc"
)

func superDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := open(superDSNFor(dbName))
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// seedLevelDeletion builds, as the superuser, tenant F's two levels. It wipes
// tenant F first so each test starts from the same school.
func seedLevelDeletion(t *testing.T) {
	t.Helper()
	db := superDB(t)
	for _, stmt := range []string{
		"DELETE FROM config_deletion_archive WHERE tenant_id = @t",
		"DELETE FROM level_deletions WHERE tenant_id = @t",
		"DELETE FROM school_events WHERE tenant_id = @t",
		"DELETE FROM teaching_assignments WHERE tenant_id = @t",
		"DELETE FROM term_months WHERE tenant_id = @t",
		"DELETE FROM terms WHERE tenant_id = @t",
		"DELETE FROM groups WHERE tenant_id = @t",
		"DELETE FROM subjects WHERE tenant_id = @t",
		"DELETE FROM grade_levels WHERE tenant_id = @t",
		"DELETE FROM teachers WHERE tenant_id = @t",
		"DELETE FROM levels WHERE tenant_id = @t",
		"DELETE FROM cycles WHERE tenant_id = @t",
	} {
		require.NoError(t, db.Exec(stmt, map[string]any{"t": tenantF}).Error, stmt)
	}
	seed := []string{
		`INSERT INTO cycles (id, tenant_id, name, status, starts_on, ends_on) VALUES ('` + ldCycle + `', @t, '2026-2027', 'active', '2026-08-01', '2027-07-15')`,
		`INSERT INTO levels (id, tenant_id, name, enrollment_granularity) VALUES ('` + ldLevel + `', @t, 'Primaria', 'CYCLE'), ('` + mLevel + `', @t, 'Secundaria', 'CYCLE')`,
		`INSERT INTO grade_levels (id, tenant_id, level_id, name, ordinal) VALUES ('` + ldGrade + `', @t, '` + ldLevel + `', '1°', 1), ('` + mGrade + `', @t, '` + mLevel + `', '1°', 1)`,
		`INSERT INTO subjects (id, tenant_id, grade_level_id, name) VALUES ('` + ldSubject + `', @t, '` + ldGrade + `', 'Matemáticas')`,
		`INSERT INTO groups (id, tenant_id, cycle_id, grade_level_id, name) VALUES ('` + ldGroup + `', @t, '` + ldCycle + `', '` + ldGrade + `', 'A'), ('` + mGroup + `', @t, '` + ldCycle + `', '` + mGrade + `', 'A')`,
		`INSERT INTO terms (id, tenant_id, cycle_id, level_id, name, ordinal, starts_on, ends_on) VALUES ('` + ldTerm + `', @t, '` + ldCycle + `', '` + ldLevel + `', 'T1', 1, '2026-09-01', '2026-11-30')`,
		`INSERT INTO term_months (tenant_id, term_id, cycle_id, level_id, month) VALUES (@t, '` + ldTerm + `', '` + ldCycle + `', '` + ldLevel + `', '2026-09-01'), (@t, '` + ldTerm + `', '` + ldCycle + `', '` + ldLevel + `', '2026-10-01')`,
		`INSERT INTO teachers (id, tenant_id, first_name, last_name, email) VALUES ('` + ldTeacher + `', @t, 'Ana', 'Ruiz', 'ana@f.mx')`,
		`INSERT INTO teaching_assignments (id, tenant_id, cycle_id, group_id, subject_id, teacher_id) VALUES ('` + ldAssign + `', @t, '` + ldCycle + `', '` + ldGroup + `', '` + ldSubject + `', '` + ldTeacher + `')`,
		`INSERT INTO school_events (id, tenant_id, cycle_id, title, starts_on, ends_on, level_id, group_id) VALUES
			('` + ldEvLevel + `', @t, '` + ldCycle + `', 'Festival de primaria', '2026-10-01', '2026-10-01', '` + ldLevel + `', NULL),
			('` + ldEvGroup + `', @t, '` + ldCycle + `', 'Salida 1A', '2026-10-02', '2026-10-02', NULL, '` + ldGroup + `'),
			('` + ldEvSchool + `', @t, '` + ldCycle + `', 'Honores', '2026-10-03', '2026-10-03', NULL, NULL)`,
	}
	for _, stmt := range seed {
		require.NoError(t, db.Exec(stmt, map[string]any{"t": tenantF}).Error, stmt)
	}
}

// countRows counts tenant F's rows of a table as the superuser (RLS-blind).
func countRows(t *testing.T, table, where string, args ...any) int64 {
	t.Helper()
	var n int64
	q := "SELECT count(*) FROM " + table + " WHERE tenant_id = '" + tenantF + "'"
	if where != "" {
		q += " AND " + where
	}
	require.NoError(t, superDB(t).Raw(q, args...).Scan(&n).Error)
	return n
}

var wantLevelCounts = levelarchive.Counts{
	"levels": 1, "gradeLevels": 1, "subjects": 1, "groups": 1, "terms": 1,
	"termMonths": 2, "teachingAssignments": 1, "schoolEvents": 2,
}

func archiveInput(batch string) ports.ArchiveLevelInput {
	return ports.ArchiveLevelInput{
		BatchID: batch, LevelID: ldLevel, DeletedBy: deleter, DeletedByName: "Laura Díaz",
		Summary: levelarchive.Counts{"students": 3, "enrollments": 3},
		At:      time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC),
	}
}

func TestLevelDeletionPreviewResolvesTheStructureAcrossCycles(t *testing.T) {
	seedLevelDeletion(t)
	repo := pgadapter.NewRepository(appDB(t), tenantF)

	got, err := repo.PreviewLevelDeletion(context.Background(), ldLevel)
	require.NoError(t, err)

	assert.Equal(t, "Primaria", got.LevelName)
	assert.Equal(t, []string{ldGrade}, got.GradeLevelIDs)
	assert.Equal(t, []string{ldGroup}, got.GroupIDs, "the bystander level's group is not in scope")
	assert.Equal(t, []string{ldSubject}, got.SubjectIDs)
	assert.Equal(t, []string{ldTerm}, got.TermIDs)
	assert.Equal(t, wantLevelCounts, got.Counts)
	assert.Equal(t, int64(1), countRows(t, "levels", "id = ?", ldLevel), "a preview changes nothing")

	_, err = repo.PreviewLevelDeletion(context.Background(), "f0000000-0000-0000-0000-00000000dead")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}

func TestLevelDeletionArchiveRestoreRoundTrip(t *testing.T) {
	seedLevelDeletion(t)
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantF)
	const batch = "f0000000-0000-0000-0000-00000000b001"

	counts, rec, err := repo.ArchiveLevel(ctx, archiveInput(batch))
	require.NoError(t, err)
	assert.Equal(t, wantLevelCounts, counts)
	assert.Equal(t, domain.LevelDeletionDeleted, rec.Status)
	assert.Equal(t, "Primaria", rec.LevelName)
	assert.Equal(t, int64(3), rec.Counts["students"], "the saga's summary is recorded")
	assert.Equal(t, int64(2), rec.Counts["schoolEvents"], "config's own counts are merged in")
	assert.True(t, rec.RestorableUntil.Equal(rec.DeletedAt.Add(7*24*time.Hour)))

	assert.Zero(t, countRows(t, "levels", "id = ?", ldLevel))
	assert.Zero(t, countRows(t, "groups", "id = ?", ldGroup))
	assert.Zero(t, countRows(t, "term_months", "level_id = ?", ldLevel))
	assert.Zero(t, countRows(t, "school_events", "id IN ?", []string{ldEvLevel, ldEvGroup}))
	assert.Equal(t, int64(1), countRows(t, "school_events", "id = ?", ldEvSchool), "a whole-school event stays")
	assert.Equal(t, int64(1), countRows(t, "levels", "id = ?", mLevel), "the bystander level stays")
	assert.Equal(t, int64(1), countRows(t, "teachers", "id = ?", ldTeacher), "teachers are people, never archived")

	again, rec2, err := repo.ArchiveLevel(ctx, archiveInput(batch))
	require.NoError(t, err, "a retried step answers its first result")
	assert.Equal(t, counts, again)
	assert.Equal(t, rec.BatchID, rec2.BatchID)

	trash, err := repo.ListLevelDeletions(ctx)
	require.NoError(t, err)
	require.Len(t, trash, 1)
	assert.Equal(t, batch, trash[0].BatchID)

	rep, err := repo.RestoreLevelBatch(ctx, batch, rec.DeletedAt.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, wantLevelCounts, rep.Restored)
	assert.Empty(t, rep.Skipped)
	assert.Equal(t, int64(1), countRows(t, "levels", "id = ? AND name = 'Primaria'", ldLevel))
	assert.Equal(t, int64(2), countRows(t, "term_months", "level_id = ?", ldLevel))
	assert.Equal(t, int64(1), countRows(t, "teaching_assignments", "id = ?", ldAssign))

	rep2, err := repo.RestoreLevelBatch(ctx, batch, rec.DeletedAt.Add(time.Hour))
	require.NoError(t, err, "a retried restore answers its first report")
	assert.Equal(t, rep.Restored, rep2.Restored)

	done, err := repo.MarkLevelDeletionRestored(ctx, batch)
	require.NoError(t, err)
	assert.Equal(t, domain.LevelDeletionRestored, done.Status)
	trash, err = repo.ListLevelDeletions(ctx)
	require.NoError(t, err)
	assert.Empty(t, trash, "a restored batch leaves the trash")
}

func TestLevelDeletionRestoreRefusesATakenName(t *testing.T) {
	seedLevelDeletion(t)
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantF)
	const batch = "f0000000-0000-0000-0000-00000000b002"
	_, rec, err := repo.ArchiveLevel(ctx, archiveInput(batch))
	require.NoError(t, err)

	require.NoError(t, superDB(t).Exec(
		"INSERT INTO levels (tenant_id, name, enrollment_granularity) VALUES (?, 'Primaria', 'CYCLE')", tenantF).Error)

	_, err = repo.RestoreLevelBatch(ctx, batch, rec.DeletedAt.Add(time.Hour))
	assert.Equal(t, domain.CodeLevelNameTaken, domain.RefusalCodeOf(err))
	assert.Zero(t, countRows(t, "grade_levels", "id = ?", ldGrade), "a refused restore restores nothing")
	assert.Equal(t, int64(1), countRows(t, "config_deletion_archive", "batch_id = ? AND table_name = 'levels'", batch),
		"the batch stays in the trash for a retry")
}

func TestLevelDeletionRestoreSkipsWhatLostItsParent(t *testing.T) {
	seedLevelDeletion(t)
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantF)
	const batch = "f0000000-0000-0000-0000-00000000b003"
	_, rec, err := repo.ArchiveLevel(ctx, archiveInput(batch))
	require.NoError(t, err)

	// While the level sat in the trash, the teacher left the school.
	require.NoError(t, superDB(t).Exec("DELETE FROM teachers WHERE id = ?", ldTeacher).Error)

	rep, err := repo.RestoreLevelBatch(ctx, batch, rec.DeletedAt.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(0), rep.Restored["teachingAssignments"])
	require.Len(t, rep.Skipped, 1)
	assert.Equal(t, "teachingAssignments", rep.Skipped[0].Kind)
	assert.Equal(t, int64(1), rep.Skipped[0].Count)
	assert.NotEmpty(t, rep.Skipped[0].Reason)
	assert.Equal(t, int64(1), countRows(t, "groups", "id = ?", ldGroup), "the rest came back")
}

func TestLevelDeletionRestoreRefusesAnExpiredBatch(t *testing.T) {
	seedLevelDeletion(t)
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantF)
	const batch = "f0000000-0000-0000-0000-00000000b004"
	_, rec, err := repo.ArchiveLevel(ctx, archiveInput(batch))
	require.NoError(t, err)

	_, err = repo.RestoreLevelBatch(ctx, batch, rec.RestorableUntil.Add(time.Minute))
	assert.Equal(t, domain.CodeTrashExpired, domain.RefusalCodeOf(err))
}

func TestLevelDeletionPurgeHonoursTheRetention(t *testing.T) {
	seedLevelDeletion(t)
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantF)
	const batch = "f0000000-0000-0000-0000-00000000b005"
	_, _, err := repo.ArchiveLevel(ctx, archiveInput(batch))
	require.NoError(t, err)

	purged, err := repo.PurgeLevelArchives(ctx, time.Now().Add(-levelarchive.Retention))
	require.NoError(t, err)
	assert.Zero(t, purged, "fresh trash is never purged")

	db := superDB(t)
	require.NoError(t, db.Exec("UPDATE config_deletion_archive SET created_at = now() - interval '8 days' WHERE batch_id = ?", batch).Error)
	require.NoError(t, db.Exec("UPDATE level_deletions SET deleted_at = now() - interval '8 days', restorable_until = now() - interval '1 day' WHERE batch_id = ?", batch).Error)

	purged, err = repo.PurgeLevelArchives(ctx, time.Now().Add(-levelarchive.Retention))
	require.NoError(t, err)
	assert.Positive(t, purged)
	assert.Zero(t, countRows(t, "config_deletion_archive", "batch_id = ?", batch))
	got, err := repo.GetLevelDeletion(ctx, batch)
	require.NoError(t, err)
	assert.Equal(t, domain.LevelDeletionPurged, got.Status)
}

func TestLevelDeletionTrashIsTenantIsolated(t *testing.T) {
	seedLevelDeletion(t)
	ctx := context.Background()
	repoF := pgadapter.NewRepository(appDB(t), tenantF)
	const batch = "f0000000-0000-0000-0000-00000000b006"
	_, _, err := repoF.ArchiveLevel(ctx, archiveInput(batch))
	require.NoError(t, err)

	repoB := pgadapter.NewRepository(appDB(t), tenantB)
	trash, err := repoB.ListLevelDeletions(ctx)
	require.NoError(t, err)
	assert.Empty(t, trash, "another school sees none of F's trash")
	_, err = repoB.GetLevelDeletion(ctx, batch)
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
	rep, err := repoB.RestoreLevelBatch(ctx, batch, time.Now())
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err), "B cannot restore F's batch: %+v", rep)
	assert.Equal(t, int64(0), countRows(t, "levels", "id = ?", ldLevel), "F's level is still in F's trash")

	var visible int64
	require.NoError(t, appDB(t).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantB).Error; err != nil {
			return err
		}
		return tx.Raw("SELECT count(*) FROM config_deletion_archive").Scan(&visible).Error
	}))
	assert.Zero(t, visible, "RLS hides F's archive rows from B")
}
