//go:build integration

// Integration tests for deleting a level into config's trash (ADR 0021): the
// structure moves to config_deletion_archive and back, the batch is recorded in
// level_deletions, conflicts are skipped and reported, the purge honours the
// retention, and RLS keeps one school's trash from another.
package postgres_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
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
		"DELETE FROM evaluation_periods WHERE tenant_id = @t",
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
		`INSERT INTO evaluation_periods (tenant_id, cycle_id, grade_level_id, ordinal, name, starts_on, ends_on, term_id) VALUES
			(@t, '` + ldCycle + `', '` + ldGrade + `', 1, 'T1', '2026-09-01', '2026-11-30', '` + ldTerm + `'),
			(@t, '` + ldCycle + `', '` + mGrade + `', 1, 'B1', '2026-09-01', '2026-10-31', NULL)`,
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
	"termMonths": 2, "teachingAssignments": 1, "schoolEvents": 2, "evaluationPeriods": 1,
}

func archiveInput(batch string) ports.ArchiveLevelInput {
	return ports.ArchiveLevelInput{
		BatchID: batch, LevelID: ldLevel,
		Summary: levelarchive.Counts{"students": 3, "enrollments": 3},
	}
}

// tokenHash is a well-formed confirm-token hash, distinct per batch.
func tokenHash(batch string) string {
	return fmt.Sprintf("%064x", sha256.Sum256([]byte(batch)))
}

func beginInput(batch string) ports.BeginLevelDeletionInput {
	return ports.BeginLevelDeletionInput{
		BatchID: batch, LevelID: ldLevel, TokenHash: tokenHash(batch),
		DeletedBy: deleter, DeletedByName: "Laura Díaz",
	}
}

// beginAndArchive runs config's two saga steps (the first and the last).
func beginAndArchive(t *testing.T, repo *pgadapter.Repository, batch string) (levelarchive.Counts, domain.LevelDeletion) {
	t.Helper()
	_, err := repo.BeginLevelDeletion(context.Background(), beginInput(batch))
	require.NoError(t, err)
	counts, rec, err := repo.ArchiveLevel(context.Background(), archiveInput(batch))
	require.NoError(t, err)
	return counts, rec
}

// levelStatus reads a level's status as the superuser.
func levelStatus(t *testing.T, id string) string {
	t.Helper()
	var st string
	require.NoError(t, superDB(t).Raw("SELECT status FROM levels WHERE id = ?", id).Scan(&st).Error)
	return st
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

	counts, rec := beginAndArchive(t, repo, batch)
	assert.Equal(t, wantLevelCounts, counts)
	assert.Equal(t, domain.LevelDeletionDeleted, rec.Status)
	assert.Equal(t, "Primaria", rec.LevelName)
	assert.Equal(t, int64(3), rec.Counts["students"], "the saga's summary is recorded")
	assert.Equal(t, int64(2), rec.Counts["schoolEvents"], "config's own counts are merged in")
	assert.True(t, rec.RestorableUntil.Equal(rec.DeletedAt.Add(7*24*time.Hour)))

	assert.Zero(t, countRows(t, "levels", "id = ?", ldLevel))
	assert.Zero(t, countRows(t, "groups", "id = ?", ldGroup))
	assert.Zero(t, countRows(t, "term_months", "level_id = ?", ldLevel))
	assert.Zero(t, countRows(t, "evaluation_periods", "grade_level_id = ?", ldGrade))
	assert.Equal(t, int64(1), countRows(t, "evaluation_periods", "grade_level_id = ?", mGrade), "the bystander's periods stay")
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

	rep, err := repo.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	assert.Equal(t, wantLevelCounts, rep.Restored)
	assert.Empty(t, rep.Skipped)
	assert.Equal(t, int64(1), countRows(t, "levels", "id = ? AND name = 'Primaria'", ldLevel))
	assert.Equal(t, "active", levelStatus(t, ldLevel), "a restored level is writable again, never frozen as it was archived")
	assert.Equal(t, int64(2), countRows(t, "term_months", "level_id = ?", ldLevel))
	assert.Equal(t, int64(1), countRows(t, "evaluation_periods", "grade_level_id = ? AND term_id = ?", ldGrade, ldTerm),
		"a period comes back with its term link")
	assert.Equal(t, int64(1), countRows(t, "teaching_assignments", "id = ?", ldAssign))

	rep2, err := repo.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err, "a retried restore answers its first report")
	assert.Equal(t, rep.Restored, rep2.Restored)

	done, err := repo.MarkLevelDeletionRestored(ctx, batch, false)
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
	beginAndArchive(t, repo, batch)

	require.NoError(t, superDB(t).Exec(
		"INSERT INTO levels (tenant_id, name, enrollment_granularity) VALUES (?, 'Primaria', 'CYCLE')", tenantF).Error)

	_, err := repo.RestoreLevelBatch(ctx, batch)
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
	beginAndArchive(t, repo, batch)

	// While the level sat in the trash, the teacher left the school.
	require.NoError(t, superDB(t).Exec("DELETE FROM teachers WHERE id = ?", ldTeacher).Error)

	rep, err := repo.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err)
	assert.Equal(t, int64(0), rep.Restored["teachingAssignments"])
	require.Len(t, rep.Skipped, 1)
	assert.Equal(t, "teachingAssignments", rep.Skipped[0].Kind)
	assert.Equal(t, int64(1), rep.Skipped[0].Count)
	assert.NotEmpty(t, rep.Skipped[0].Reason)
	assert.Equal(t, int64(1), countRows(t, "groups", "id = ?", ldGroup), "the rest came back")

	// Review finding 2/3: the teacher comes back; a retry finishes the batch —
	// no LEVEL_NAME_TAKEN from its own, already restored level.
	require.NoError(t, superDB(t).Exec(`INSERT INTO teachers (id, tenant_id, first_name, last_name, email)
		VALUES (?, ?, 'Ana', 'Ruiz', 'ana@f.mx')`, ldTeacher, tenantF).Error)
	again, err := repo.RestoreLevelBatch(ctx, batch)
	require.NoError(t, err, "a retry after a partial restore converges")
	assert.Empty(t, again.Skipped)
	assert.Equal(t, int64(1), again.Restored["teachingAssignments"])
	assert.Equal(t, int64(1), countRows(t, "teaching_assignments", "id = ?", ldAssign))
	assert.Zero(t, countRows(t, "config_deletion_archive", "batch_id = ? AND table_name <> '_batch'", batch))
}

func TestLevelDeletionRestoreRefusesAnExpiredBatch(t *testing.T) {
	seedLevelDeletion(t)
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantF)
	const batch = "f0000000-0000-0000-0000-00000000b004"
	beginAndArchive(t, repo, batch)

	// The window is the database's: move it into the past there.
	require.NoError(t, superDB(t).Exec("UPDATE level_deletions SET deleted_at = now() - interval '8 days', restorable_until = now() - interval '1 minute' WHERE batch_id = ?", batch).Error)
	_, err := repo.RestoreLevelBatch(ctx, batch)
	assert.Equal(t, domain.CodeTrashExpired, domain.RefusalCodeOf(err))
}

func TestLevelDeletionPurgeHonoursTheRetention(t *testing.T) {
	seedLevelDeletion(t)
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantF)
	const batch = "f0000000-0000-0000-0000-00000000b005"
	beginAndArchive(t, repo, batch)

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
	beginAndArchive(t, repoF, batch)

	repoB := pgadapter.NewRepository(appDB(t), tenantB)
	trash, err := repoB.ListLevelDeletions(ctx)
	require.NoError(t, err)
	assert.Empty(t, trash, "another school sees none of F's trash")
	_, err = repoB.GetLevelDeletion(ctx, batch)
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
	rep, err := repoB.RestoreLevelBatch(ctx, batch)
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

// --- review fixes: the lock, the single-use token, the one clock --------------

func TestBeginLevelDeletionLocksTheLevelOnTheDatabaseClock(t *testing.T) {
	seedLevelDeletion(t)
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantF)
	const batch = "f0000000-0000-0000-0000-00000000c001"

	before := time.Now()
	rec, err := repo.BeginLevelDeletion(ctx, beginInput(batch))
	require.NoError(t, err)
	assert.Equal(t, domain.LevelDeletionDeleting, rec.Status)
	assert.Equal(t, "Primaria", rec.LevelName)
	assert.WithinDuration(t, before, rec.DeletedAt, time.Minute, "deleted_at is the database's now()")
	assert.True(t, rec.RestorableUntil.Equal(rec.DeletedAt.Add(levelarchive.Retention)))
	assert.Equal(t, "deleting", levelStatus(t, ldLevel))

	st, err := repo.PreviewLevelDeletion(ctx, ldLevel)
	require.NoError(t, err)
	assert.True(t, st.Deleting, "the preview says the level is being deleted")

	again, err := repo.BeginLevelDeletion(ctx, beginInput(batch))
	require.NoError(t, err, "the same batch is idempotent")
	assert.Equal(t, rec.DeletedAt, again.DeletedAt)

	_, err = repo.BeginLevelDeletion(ctx, beginInput("f0000000-0000-0000-0000-00000000c002"))
	assert.Equal(t, domain.CodeDeletionInProgress, domain.RefusalCodeOf(err))
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))

	trash, err := repo.ListLevelDeletions(ctx)
	require.NoError(t, err)
	assert.Empty(t, trash, "a running saga is not in the trash yet")

	_, err = repo.BeginLevelDeletion(ctx, ports.BeginLevelDeletionInput{
		BatchID: "f0000000-0000-0000-0000-00000000c003", LevelID: "f0000000-0000-0000-0000-00000000dead",
		TokenHash: tokenHash("x"), DeletedBy: deleter, DeletedByName: "Laura",
	})
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}

func TestBeginLevelDeletionConcurrentDeletesRunOneSaga(t *testing.T) {
	seedLevelDeletion(t)
	ctx := context.Background()
	const racers = 8
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		won      int
		inFlight int
	)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			repo := pgadapter.NewRepository(appDB(t), tenantF)
			batch := fmt.Sprintf("f0000000-0000-0000-0000-0000000d%04d", i)
			_, err := repo.BeginLevelDeletion(ctx, beginInput(batch))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				won++
			case domain.RefusalCodeOf(err) == domain.CodeDeletionInProgress:
				inFlight++
			default:
				t.Errorf("racer %d: unexpected %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	assert.Equal(t, 1, won, "exactly one saga starts")
	assert.Equal(t, racers-1, inFlight, "every other request is told a deletion is in progress")
	assert.Equal(t, int64(1), countRows(t, "level_deletions", "status = 'deleting'"))
}

func TestConfirmTokenOpensOneSagaOnly(t *testing.T) {
	seedLevelDeletion(t)
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantF)
	const first, second = "f0000000-0000-0000-0000-00000000c011", "f0000000-0000-0000-0000-00000000c012"

	_, err := repo.BeginLevelDeletion(ctx, beginInput(first))
	require.NoError(t, err)
	cancelled, err := repo.CancelLevelDeletion(ctx, first)
	require.NoError(t, err)
	assert.Equal(t, domain.LevelDeletionCancelled, cancelled.Status)
	assert.Equal(t, "active", levelStatus(t, ldLevel), "a fully undone deletion unfreezes the level")

	in := beginInput(second)
	in.TokenHash = tokenHash(first)
	_, err = repo.BeginLevelDeletion(ctx, in)
	assert.Equal(t, domain.CodePreviewStale, domain.RefusalCodeOf(err), "a used token is stale")
	assert.Equal(t, "active", levelStatus(t, ldLevel))
}

func TestArchiveLevelNeedsABegunDeletion(t *testing.T) {
	seedLevelDeletion(t)
	repo := pgadapter.NewRepository(appDB(t), tenantF)
	_, _, err := repo.ArchiveLevel(context.Background(), archiveInput("f0000000-0000-0000-0000-00000000c021"))
	assert.Error(t, err)
	assert.Equal(t, int64(1), countRows(t, "levels", "id = ?", ldLevel), "nothing moved")
}

// Review finding 4: while the level is being deleted, nothing of its structure
// can be written — by any path. A bystander level is untouched.
func TestADeletingLevelRefusesStructureWrites(t *testing.T) {
	seedLevelDeletion(t)
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantF)
	_, err := repo.BeginLevelDeletion(ctx, beginInput("f0000000-0000-0000-0000-00000000c031"))
	require.NoError(t, err)

	writes := []struct{ name, sql string }{
		{"grade level", `INSERT INTO grade_levels (tenant_id, level_id, name, ordinal) VALUES (@t, @l, '2°', 2)`},
		{"subject", `INSERT INTO subjects (tenant_id, grade_level_id, name) VALUES (@t, @g, 'Español')`},
		{"group", `INSERT INTO groups (tenant_id, cycle_id, grade_level_id, name) VALUES (@t, @c, @g, 'B')`},
		{"term", `INSERT INTO terms (tenant_id, cycle_id, level_id, name, ordinal, starts_on, ends_on) VALUES (@t, @c, @l, 'T2', 2, '2026-12-01', '2027-02-28')`},
		{"term month", `INSERT INTO term_months (tenant_id, term_id, cycle_id, level_id, month) VALUES (@t, @term, @c, @l, '2026-11-01')`},
		{"evaluation period", `INSERT INTO evaluation_periods (tenant_id, cycle_id, grade_level_id, ordinal, name, starts_on, ends_on) VALUES (@t, @c, @g, 2, 'T2', '2026-12-01', '2027-02-28')`},
		{"teaching assignment", `UPDATE teaching_assignments SET teacher_id = teacher_id WHERE id = @a`},
		{"event of the level", `INSERT INTO school_events (tenant_id, cycle_id, title, starts_on, ends_on, level_id) VALUES (@t, @c, 'x', '2026-10-09', '2026-10-09', @l)`},
		{"event of a group", `INSERT INTO school_events (tenant_id, cycle_id, title, starts_on, ends_on, group_id) VALUES (@t, @c, 'x', '2026-10-09', '2026-10-09', @grp)`},
		{"the level itself", `UPDATE levels SET name = 'Primaria 2' WHERE id = @l`},
	}
	args := map[string]any{"t": tenantF, "l": ldLevel, "g": ldGrade, "c": ldCycle, "term": ldTerm, "a": ldAssign, "grp": ldGroup}
	for _, w := range writes {
		t.Run(w.name, func(t *testing.T) {
			err := appDB(t).Transaction(func(tx *gorm.DB) error {
				if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", tenantF).Error; err != nil {
					return err
				}
				return tx.Exec(w.sql, args).Error
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "LEVEL_BEING_DELETED")
		})
	}

	// Through the repository the refusal is the coded conflict.
	gl, err := domain.NewGradeLevel("f0000000-0000-0000-0000-0000000000a9", tenantF, ldLevel, "3°", 3)
	require.NoError(t, err)
	err = repo.CreateGradeLevel(ctx, gl)
	assert.Equal(t, levelarchive.CodeLevelBeingDeleted, levelarchive.CodeOf(err))
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))

	other, err := domain.NewGradeLevel("f0000000-0000-0000-0000-0000000000b9", tenantF, mLevel, "2°", 2)
	require.NoError(t, err)
	require.NoError(t, repo.CreateGradeLevel(ctx, other), "another level is not frozen")
}

func TestFailedPartialDeletionStaysFrozenInTheTrashUntilRestored(t *testing.T) {
	seedLevelDeletion(t)
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantF)
	const batch = "f0000000-0000-0000-0000-00000000c041"
	_, err := repo.BeginLevelDeletion(ctx, beginInput(batch))
	require.NoError(t, err)

	skipped := []levelarchive.Skip{{Kind: "enrollments", Count: 2, Reason: "ya inscrito en otro grupo"}}
	rec, err := repo.FailLevelDeletion(ctx, batch, []string{"grades", "students"}, skipped)
	require.NoError(t, err)
	assert.Equal(t, domain.LevelDeletionFailedPartial, rec.Status)
	assert.Equal(t, []string{"grades", "students"}, rec.PendingSteps)
	assert.Equal(t, skipped, rec.Skipped)
	assert.Equal(t, "deleting", levelStatus(t, ldLevel), "half a level stays frozen until it is restored")

	trash, err := repo.ListLevelDeletions(ctx)
	require.NoError(t, err)
	require.Len(t, trash, 1, "the director sees it in the trash")
	assert.Equal(t, domain.LevelDeletionFailedPartial, trash[0].Status)

	_, err = repo.RestoreLevelBatch(ctx, batch)
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err), "config never archived: NotFound, the gateway knows it was pending")

	done, err := repo.MarkLevelDeletionRestored(ctx, batch, true)
	require.NoError(t, err)
	assert.Equal(t, domain.LevelDeletionRestoredPartial, done.Status, "some rows did not fit: it stays in the trash")
	assert.Equal(t, "active", levelStatus(t, ldLevel))
	trash, err = repo.ListLevelDeletions(ctx)
	require.NoError(t, err)
	assert.Len(t, trash, 1)

	done, err = repo.MarkLevelDeletionRestored(ctx, batch, false)
	require.NoError(t, err)
	assert.Equal(t, domain.LevelDeletionRestored, done.Status)
}

func TestSweepFindsExpiredBatchesAndUnsticksDeadSagas(t *testing.T) {
	seedLevelDeletion(t)
	ctx := context.Background()
	repo := pgadapter.NewRepository(appDB(t), tenantF)
	const expired, stuck = "f0000000-0000-0000-0000-00000000c051", "f0000000-0000-0000-0000-00000000c052"
	beginAndArchive(t, repo, expired)
	db := superDB(t)
	require.NoError(t, db.Exec("UPDATE level_deletions SET deleted_at = now() - interval '8 days', restorable_until = now() - interval '1 day' WHERE batch_id = ?", expired).Error)

	// A gateway died mid-saga on the bystander level an hour ago.
	in := beginInput(stuck)
	in.LevelID = mLevel
	_, err := repo.BeginLevelDeletion(ctx, in)
	require.NoError(t, err)
	require.NoError(t, db.Exec("UPDATE level_deletions SET deleted_at = now() - interval '1 hour', restorable_until = now() + interval '6 days' WHERE batch_id = ?", stuck).Error)

	got, stalled, err := repo.SweepLevelDeletions(ctx, 30*time.Minute)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stalled)
	require.Len(t, got, 1)
	assert.Equal(t, expired, got[0].BatchID)

	st, err := repo.GetLevelDeletion(ctx, stuck)
	require.NoError(t, err)
	assert.Equal(t, domain.LevelDeletionFailedPartial, st.Status, "a dead saga lands in the trash, restorable")
	assert.ElementsMatch(t, domain.LevelDeletionSteps, st.PendingSteps)

	n, err := repo.PurgeLevelBatch(ctx, expired)
	require.NoError(t, err)
	assert.Positive(t, n)
	assert.Zero(t, countRows(t, "config_deletion_archive", "batch_id = ?", expired))
	done, err := repo.MarkLevelDeletionPurged(ctx, expired)
	require.NoError(t, err)
	assert.Equal(t, domain.LevelDeletionPurged, done.Status)

	got, _, err = repo.SweepLevelDeletions(ctx, 30*time.Minute)
	require.NoError(t, err)
	assert.Empty(t, got, "a purged batch is swept once")
}
