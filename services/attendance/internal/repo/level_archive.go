package repo

import (
	"context"
	"time"

	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/pkg/levelarchive/gormarchive"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

// Compile-time proof the adapter satisfies the level-archive seam.
var _ service.LevelArchiveRepo = (*Repo)(nil)

// levelTrash is attendance's half of the deletion archive (migration 0005).
var levelTrash = gormarchive.New("attendance_deletion_archive")

// The kinds attendance reports (ADR 0021).
const (
	kindMarks    = "attendanceMarks"
	kindSessions = "attendanceSessions"
)

// levelSteps is the deletion: a mark goes when it belongs to one of the level's
// enrollments, was taken in one of its groups (a pupil who moved out of the
// level keeps no mark stamped with a deleted group) or is for one of its
// subjects; a pass list goes with its group or subject. Marks first: nothing
// references a session, but the order is the restore's reverse.
func levelSteps(s levelarchive.Scope) []gormarchive.Step {
	return []gormarchive.Step{
		{Kind: kindMarks, Table: "attendance",
			Where: "enrollment_id IN ? OR group_id IN ? OR subject_id IN ?",
			Args:  []any{s.EnrollmentIDs, s.GroupIDs, s.SubjectIDs}},
		{Kind: kindSessions, Table: "attendance_sessions",
			Where: "group_id IN ? OR subject_id IN ?",
			Args:  []any{s.GroupIDs, s.SubjectIDs}},
	}
}

// restoreOrder is the way back: pass lists, then their marks. Neither has a
// parent in this service; a row whose unique key was taken again while the
// level sat in the trash (the same list, the same pupil and day) is skipped.
var restoreOrder = []struct{ kind, table string }{
	{kindSessions, "attendance_sessions"},
	{kindMarks, "attendance"},
}

// skipReason is the one sentence a skipped attendance row gets.
const skipReason = "ya hay otra lista o marca para ese día"

// PreviewLevelDeletion counts what archiving the scope would move, changing
// nothing.
func (r *Repo) PreviewLevelDeletion(ctx context.Context, s levelarchive.Scope) (levelarchive.Counts, error) {
	counts := levelarchive.Counts{}
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		for _, step := range levelSteps(s) {
			n, err := gormarchive.Count(tx, step)
			if err != nil {
				return glerrors.Wrap(glerrors.KindInternal, "counting the level's attendance", err)
			}
			counts[step.Kind] = n
		}
		return nil
	})
	return counts, err
}

// ArchiveLevel moves the scope's marks and pass lists into the trash, once per
// batch.
func (r *Repo) ArchiveLevel(ctx context.Context, batchID string, s levelarchive.Scope) (levelarchive.Counts, error) {
	var counts levelarchive.Counts
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		counts, err = levelTrash.ArchiveBatch(tx, batchID, func(tx *gorm.DB) (levelarchive.Counts, error) {
			out := levelarchive.Counts{}
			for i, step := range levelSteps(s) {
				n, err := levelTrash.Move(tx, batchID, i, step)
				if err != nil {
					return nil, glerrors.Wrap(glerrors.KindInternal, "archiving the level's attendance", err)
				}
				out[step.Kind] = n
			}
			return out, nil
		})
		return err
	})
	return counts, err
}

// RestoreLevelBatch puts a batch back, skipping and reporting what conflicts.
func (r *Repo) RestoreLevelBatch(ctx context.Context, batchID string) (levelarchive.Report, error) {
	var rep levelarchive.Report
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		rep, err = levelTrash.RestoreBatch(tx, batchID, func(tx *gorm.DB) (levelarchive.Report, error) {
			out := levelarchive.NewReport()
			for _, step := range restoreOrder {
				restored, skipped, err := levelTrash.Reinsert(tx, batchID, step.table)
				if err != nil {
					return out, glerrors.Wrap(glerrors.KindInternal, "restoring the level's attendance", err)
				}
				out.Record(step.kind, restored, skipped, skipReason)
			}
			return out, nil
		})
		return err
	})
	return rep, err
}

// PurgeLevelArchives drops every archive row filed before cutoff.
func (r *Repo) PurgeLevelArchives(ctx context.Context, cutoff time.Time) (int64, error) {
	var n int64
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		if n, err = levelTrash.Purge(tx, cutoff); err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "purging the attendance trash", err)
		}
		return nil
	})
	return n, err
}
