package postgres

import (
	"context"
	"time"

	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/pkg/levelarchive/gormarchive"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// Compile-time proof the adapter satisfies the level-archive port.
var _ ports.LevelArchiveRepository = (*Repository)(nil)

// levelTrash is students' half of the deletion archive (migration 0016).
var levelTrash = gormarchive.New("students_deletion_archive")

// The kinds students reports.
const (
	kindStudents            = "students"
	kindEnrollments         = "enrollments"
	kindEnrollmentTransfers = "enrollmentTransfers"
)

// studentLevelSnapshot names the archived "before" values of the pupils whose
// level a deletion cleared. They are not deleted: only their reference is.
const studentLevelSnapshot = "students#level"

// Spanish reasons a restore gives for what it could not put back.
const (
	reasonNewLevel     = "el alumno ya tiene otro nivel"
	reasonReEnrolled   = "el alumno ya está inscrito en otro grupo de ese ciclo"
	reasonNoEnrollment = "su inscripción ya no existe"
)

// Predicates over the scope. Enrollments are found by the level's groups;
// pupils by the level itself.
const (
	enrollmentsOfGroups = "group_id IN ?"
	transfersOfGroups   = "enrollment_id IN (SELECT id FROM enrollments WHERE group_id IN ?)"
	pupilsOfLevel       = "level_id = ?"
)

// enrollmentSteps is the archived part, children first.
func enrollmentSteps(scope levelarchive.Scope) []gormarchive.Step {
	groups := []any{scope.GroupIDs}
	return []gormarchive.Step{
		{Kind: kindEnrollmentTransfers, Table: "enrollment_transfers", Where: transfersOfGroups, Args: groups},
		{Kind: kindEnrollments, Table: "enrollments", Where: enrollmentsOfGroups, Args: groups},
	}
}

// PreviewLevelDeletion counts and lists what the deletion would reach.
func (r *Repository) PreviewLevelDeletion(ctx context.Context, scope levelarchive.Scope) (ports.LevelPeople, error) {
	out := ports.LevelPeople{Counts: levelarchive.Counts{}}
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		for _, s := range enrollmentSteps(scope) {
			n, err := gormarchive.Count(tx, s)
			if err != nil {
				return glerrors.Wrap(glerrors.KindInternal, "counting the level's enrollments", err)
			}
			out.Counts[s.Kind] = n
		}
		if err := tx.Raw("SELECT id::text FROM students WHERE "+pupilsOfLevel+" ORDER BY id", scope.LevelID).
			Scan(&out.StudentIDs).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing the level's pupils", err)
		}
		if err := tx.Raw("SELECT id::text FROM enrollments WHERE "+enrollmentsOfGroups+" ORDER BY id", scope.GroupIDs).
			Scan(&out.EnrollmentIDs).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing the level's enrollments", err)
		}
		out.Counts[kindStudents] = int64(len(out.StudentIDs))
		return nil
	})
	return out, err
}

// ArchiveLevel archives the enrollments and clears the pupils' level, once per
// batch.
func (r *Repository) ArchiveLevel(ctx context.Context, batchID string, scope levelarchive.Scope) (levelarchive.Counts, error) {
	var counts levelarchive.Counts
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		counts, err = levelTrash.ArchiveBatch(tx, batchID, func(tx *gorm.DB) (levelarchive.Counts, error) {
			return archivePeople(tx, batchID, scope)
		})
		return err
	})
	return counts, err
}

func archivePeople(tx *gorm.DB, batchID string, scope levelarchive.Scope) (levelarchive.Counts, error) {
	counts := levelarchive.Counts{}
	steps := enrollmentSteps(scope)
	for i, s := range steps {
		n, err := levelTrash.Move(tx, batchID, i, s)
		if err != nil {
			return nil, glerrors.Wrap(glerrors.KindInternal, "archiving the level's enrollments", err)
		}
		counts[s.Kind] = n
	}
	if _, err := levelTrash.Snapshot(tx, batchID, len(steps), studentLevelSnapshot,
		"SELECT id, level_id, level_name, level, grade_group FROM students WHERE "+pupilsOfLevel, scope.LevelID); err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "filing the pupils' level", err)
	}
	res := tx.Exec("UPDATE students SET level_id = NULL, level_name = '', level = NULL, grade_group = '', updated_at = now() WHERE "+
		pupilsOfLevel, scope.LevelID)
	if res.Error != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "clearing the pupils' level", res.Error)
	}
	counts[kindStudents] = res.RowsAffected
	return counts, nil
}

// RestoreLevelBatch puts the batch back: pupils' level first, then their
// enrollments, then the enrollments' transfer history.
func (r *Repository) RestoreLevelBatch(ctx context.Context, batchID string) (levelarchive.Report, error) {
	var rep levelarchive.Report
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		rep, err = levelTrash.RestoreBatch(tx, batchID, func(tx *gorm.DB) (levelarchive.Report, error) {
			return restorePeople(tx, batchID)
		})
		return err
	})
	return rep, err
}

func restorePeople(tx *gorm.DB, batchID string) (levelarchive.Report, error) {
	rep := levelarchive.NewReport()
	filed, err := levelTrash.Archived(tx, batchID, studentLevelSnapshot)
	if err != nil {
		return rep, glerrors.Wrap(glerrors.KindInternal, "reading the pupils' level", err)
	}
	// Only a pupil still without a level gets the old one back: a level given
	// meanwhile is a newer decision and wins.
	res := tx.Exec(`UPDATE students s
		   SET level_id = (a."row"->>'level_id')::uuid, level_name = a."row"->>'level_name',
		       level = a."row"->>'level', grade_group = a."row"->>'grade_group', updated_at = now()
		  FROM `+levelTrash.Table()+` a
		 WHERE a.batch_id = ? AND a.table_name = ? AND s.id = (a."row"->>'id')::uuid AND s.level_id IS NULL`,
		batchID, studentLevelSnapshot)
	if res.Error != nil {
		return rep, glerrors.Wrap(glerrors.KindInternal, "restoring the pupils' level", res.Error)
	}
	rep.Record(kindStudents, res.RowsAffected, filed-res.RowsAffected, reasonNewLevel)

	restored, skipped, err := levelTrash.Reinsert(tx, batchID, "enrollments",
		gormarchive.Parent{Column: "student_id", Table: "students"})
	if err != nil {
		return rep, glerrors.Wrap(glerrors.KindInternal, "restoring the enrollments", err)
	}
	rep.Record(kindEnrollments, restored, skipped, reasonReEnrolled)

	restored, skipped, err = levelTrash.Reinsert(tx, batchID, "enrollment_transfers",
		gormarchive.Parent{Column: "enrollment_id", Table: "enrollments"})
	if err != nil {
		return rep, glerrors.Wrap(glerrors.KindInternal, "restoring the transfer history", err)
	}
	rep.Record(kindEnrollmentTransfers, restored, skipped, reasonNoEnrollment)
	return rep, nil
}

// PurgeLevelArchives drops archives filed before cutoff.
func (r *Repository) PurgeLevelArchives(ctx context.Context, cutoff time.Time) (int64, error) {
	var n int64
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		if n, err = levelTrash.Purge(tx, cutoff); err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "purging the trash", err)
		}
		return nil
	})
	return n, err
}
