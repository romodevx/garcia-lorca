package postgres

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/config/internal/domain"
)

// Catalog corrections (T8, ADR 0036): a grade or group captured wrong is
// renamed in place or deleted alone, and the levels take a school order.
//
// The deletes count what still hangs from the row WITH THE ROW LOCKED (FOR
// UPDATE) and delete in the same transaction. Every child insert checks its
// foreign key by taking FOR KEY SHARE on the parent, which FOR UPDATE blocks,
// so nothing can be attached between the count and the delete. The ON DELETE
// RESTRICT keys stay as the backstop.

// lockForUpdate is the row lock a count-then-delete holds.
var lockForUpdate = clause.Locking{Strength: "UPDATE"}

// groupNameKey is 0001's UNIQUE (tenant_id, cycle_id, grade_level_id, name).
const groupNameKey = "groups_tenant_id_cycle_id_grade_level_id_name_key"

// --- levels -----------------------------------------------------------------

// ReorderLevels rewrites sort_order from ids (first = 1) in ONE transaction.
// The tenant's level rows are locked first and ids must be exactly them
// (domain.CheckLevelOrder), so a half-applied or partial order cannot exist.
// A level created while this runs is not in ids and keeps the place its
// insert trigger gave it (after the last).
func (r *Repository) ReorderLevels(ctx context.Context, ids []string) ([]domain.Level, error) {
	var out []domain.Level
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var current []string
		if err := tx.Raw("SELECT id::text FROM levels ORDER BY id FOR UPDATE").Scan(&current).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "locking the levels", err)
		}
		if err := domain.CheckLevelOrder(current, ids); err != nil {
			return err
		}
		for i, id := range ids {
			place := i + 1
			if err := tx.Exec("UPDATE levels SET sort_order = ?, updated_at = now() WHERE id = ? AND sort_order <> ?",
				place, id, place).Error; err != nil {
				return glerrors.Wrap(glerrors.KindInternal, "reordering levels", err)
			}
		}
		var lerr error
		out, lerr = listLevels(tx)
		return lerr
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// --- grade levels -------------------------------------------------------------

// RenameGradeLevel changes a grade's name in place and returns the stored row.
// A malformed or unknown id is KindNotFound; a name the level already has,
// whatever its case (0006's unique index), is GRADE_NAME_TAKEN.
func (r *Repository) RenameGradeLevel(ctx context.Context, id, name string) (domain.GradeLevel, error) {
	if !isUUID(id) {
		return domain.GradeLevel{}, glerrors.ErrNotFound
	}
	var out domain.GradeLevel
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []gradeLevelModel
		res := tx.Model(&rows).Clauses(clause.Returning{}).
			Where("id = ?", id).
			Updates(map[string]any{"name": name, "updated_at": gorm.Expr("now()")})
		if isConstraint(res.Error, gradeNamePerLevelKey) {
			return domain.GradeNameTaken()
		}
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "renaming grade level", res.Error)
		}
		if len(rows) == 0 {
			return glerrors.ErrNotFound
		}
		var derr error
		out, derr = toDomainGradeLevel(rows[0])
		return derr
	})
	if err != nil {
		return domain.GradeLevel{}, err
	}
	return out, nil
}

// gradeUsageRow is what still hangs from a grade.
type gradeUsageRow struct {
	Groups            int64
	Subjects          int64
	EvaluationPeriods int64
}

// DeleteGradeLevel removes a grade that holds nothing; its level and sibling
// grades stay. With the grade locked it counts groups (any cycle), subjects
// and evaluation periods, and refuses with the count of each.
func (r *Repository) DeleteGradeLevel(ctx context.Context, id string) error {
	if !isUUID(id) {
		return glerrors.ErrNotFound
	}
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		var m gradeLevelModel
		if err := tx.Clauses(lockForUpdate).Where("id = ?", id).First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		if err := refuseUnderLevelBeingDeleted(tx, `SELECT status FROM levels WHERE id = ? FOR SHARE`, m.LevelID); err != nil {
			return err
		}
		var u gradeUsageRow
		if err := tx.Raw(`SELECT
				(SELECT count(*) FROM groups WHERE grade_level_id = @id)             AS groups,
				(SELECT count(*) FROM subjects WHERE grade_level_id = @id)           AS subjects,
				(SELECT count(*) FROM evaluation_periods WHERE grade_level_id = @id) AS evaluation_periods`,
			map[string]any{"id": id}).Scan(&u).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "counting what the grade holds", err)
		}
		usage := domain.GradeLevelUsage{Groups: u.Groups, Subjects: u.Subjects, EvaluationPeriods: u.EvaluationPeriods}
		if err := usage.Refusal(m.Name); err != nil {
			return err
		}
		if err := tx.Where("id = ?", id).Delete(&gradeLevelModel{}).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "deleting grade level", err)
		}
		return nil
	})
}

// --- groups -------------------------------------------------------------------

// RenameGroup changes a group's name in place and returns the stored row. A
// malformed or unknown id is KindNotFound; a name its grade already has in the
// cycle is GROUP_NAME_TAKEN.
func (r *Repository) RenameGroup(ctx context.Context, id, name string) (domain.Group, error) {
	if !isUUID(id) {
		return domain.Group{}, glerrors.ErrNotFound
	}
	var out domain.Group
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []groupModel
		res := tx.Model(&rows).Clauses(clause.Returning{}).
			Where("id = ?", id).
			Updates(map[string]any{"name": name, "updated_at": gorm.Expr("now()")})
		if isConstraint(res.Error, groupNameKey) {
			return domain.GroupNameTaken()
		}
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "renaming group", res.Error)
		}
		if len(rows) == 0 {
			return glerrors.ErrNotFound
		}
		var derr error
		out, derr = toDomainGroup(rows[0])
		return derr
	})
	if err != nil {
		return domain.Group{}, err
	}
	return out, nil
}

// groupUsageRow is what config keeps of a group.
type groupUsageRow struct {
	TeachingAssignments int64
	ScheduleBlocks      int64
}

// groupUsageSQL counts a group's teaching assignments and timetable blocks.
const groupUsageSQL = `SELECT
	(SELECT count(*) FROM teaching_assignments WHERE group_id = @id) AS teaching_assignments,
	(SELECT count(*) FROM schedule_blocks WHERE group_id = @id)      AS schedule_blocks`

func countGroupUsage(tx *gorm.DB, id string) (domain.GroupUsage, error) {
	var u groupUsageRow
	if err := tx.Raw(groupUsageSQL, map[string]any{"id": id}).Scan(&u).Error; err != nil {
		return domain.GroupUsage{}, glerrors.Wrap(glerrors.KindInternal, "counting what the group holds", err)
	}
	return domain.GroupUsage{TeachingAssignments: u.TeachingAssignments, ScheduleBlocks: u.ScheduleBlocks}, nil
}

// GroupUsage counts config's references to a group. A malformed, unknown or
// other tenant's id is KindNotFound — never zeros, which would read as "free".
func (r *Repository) GroupUsage(ctx context.Context, id string) (domain.GroupUsage, error) {
	if !isUUID(id) {
		return domain.GroupUsage{}, glerrors.ErrNotFound
	}
	var out domain.GroupUsage
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m groupModel
		if err := tx.Where("id = ?", id).First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		var cerr error
		out, cerr = countGroupUsage(tx, id)
		return cerr
	})
	if err != nil {
		return domain.GroupUsage{}, err
	}
	return out, nil
}

// DeleteGroup removes a group config keeps nothing of; its grade stays. With
// the group locked it counts teaching assignments and timetable blocks and
// refuses with both counts.
func (r *Repository) DeleteGroup(ctx context.Context, id string) error {
	if !isUUID(id) {
		return glerrors.ErrNotFound
	}
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		var m groupModel
		if err := tx.Clauses(lockForUpdate).Where("id = ?", id).First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		if err := refuseUnderLevelBeingDeleted(tx, `SELECT l.status FROM levels l
				JOIN grade_levels g ON g.level_id = l.id WHERE g.id = ? FOR SHARE OF l`, m.GradeLevelID); err != nil {
			return err
		}
		usage, err := countGroupUsage(tx, id)
		if err != nil {
			return err
		}
		if err := usage.Refusal(m.Name); err != nil {
			return err
		}
		if err := tx.Where("id = ?", id).Delete(&groupModel{}).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "deleting group", err)
		}
		return nil
	})
}

// refuseUnderLevelBeingDeleted refuses a hand delete under a level in the
// deletion saga (security review of T8). The freeze triggers of migration 0012
// watch INSERT and UPDATE only — the saga itself deletes these rows — so a
// grade or group deleted by hand meanwhile would come back on a restore. The
// share lock keeps the saga from starting until this transaction ends.
func refuseUnderLevelBeingDeleted(tx *gorm.DB, statusQuery string, arg string) error {
	var status string
	if err := tx.Raw(statusQuery, arg).Scan(&status).Error; err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "reading the level's status", err)
	}
	if status == levelStatusDeleting {
		return levelarchive.LevelBeingDeleted()
	}
	return nil
}
