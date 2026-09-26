package postgres

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
)

// ApplyTeachingLoad reads the snapshot a teaching load is decided against and
// writes the decision, all in ONE tenant-bound transaction.
//
// The occupied slots are read FOR UPDATE (ordered by id, so two concurrent
// loads lock in the same order and cannot deadlock each other): a load that
// decided "reassign" or "conflict" for a slot cannot see that slot change hands
// before it commits. A FREE slot has no row to lock; if a concurrent writer
// fills it first, the insert trips the one-teacher-per-slot UNIQUE, surfaces as
// KindConflict, and rolls the whole load back — the caller retries against the
// new state instead of silently getting half a load.
func (r *Repository) ApplyTeachingLoad(ctx context.Context, teacherID string, slots []domain.TeachingSlot,
	plan func(ports.TeachingLoadSnapshot) (ports.TeachingLoadWrite, error)) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		snap, err := loadTeachingSnapshot(tx, teacherID, slots)
		if err != nil {
			return err
		}
		write, err := plan(snap)
		if err != nil {
			return err
		}
		return writeTeachingLoad(tx, write)
	})
}

// loadTeachingSnapshot reads the teacher, the referenced groups and subjects,
// the assignments occupying the requested slots (locked), and their holders.
func loadTeachingSnapshot(tx *gorm.DB, teacherID string, slots []domain.TeachingSlot) (ports.TeachingLoadSnapshot, error) {
	snap := ports.TeachingLoadSnapshot{}
	var err error
	if snap.Teacher, snap.TeacherFound, err = findTeacher(tx, teacherID); err != nil {
		return ports.TeachingLoadSnapshot{}, err
	}
	groupIDs, subjectIDs, pairs := slotKeys(slots)
	if snap.Groups, err = groupsByID(tx, groupIDs); err != nil {
		return ports.TeachingLoadSnapshot{}, err
	}
	if snap.Subjects, err = subjectsByID(tx, subjectIDs); err != nil {
		return ports.TeachingLoadSnapshot{}, err
	}
	if snap.Occupied, err = occupiedSlots(tx, pairs); err != nil {
		return ports.TeachingLoadSnapshot{}, err
	}
	holderIDs := make([]string, 0, len(snap.Occupied))
	for _, a := range snap.Occupied {
		holderIDs = append(holderIDs, a.TeacherID())
	}
	if snap.Holders, err = teachersByID(tx, holderIDs); err != nil {
		return ports.TeachingLoadSnapshot{}, err
	}
	return snap, nil
}

// slotKeys splits the slots into distinct group ids, distinct subject ids, and
// the (group_id, subject_id) tuples for the row-value IN.
func slotKeys(slots []domain.TeachingSlot) (groupIDs, subjectIDs []string, pairs [][]any) {
	seenGroup := make(map[string]bool, len(slots))
	seenSubject := make(map[string]bool, len(slots))
	pairs = make([][]any, 0, len(slots))
	for _, s := range slots {
		if !seenGroup[s.GroupID] {
			seenGroup[s.GroupID] = true
			groupIDs = append(groupIDs, s.GroupID)
		}
		if !seenSubject[s.SubjectID] {
			seenSubject[s.SubjectID] = true
			subjectIDs = append(subjectIDs, s.SubjectID)
		}
		pairs = append(pairs, []any{s.GroupID, s.SubjectID})
	}
	return groupIDs, subjectIDs, pairs
}

func findTeacher(tx *gorm.DB, id string) (domain.Teacher, bool, error) {
	var m teacherModel
	err := tx.Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Teacher{}, false, nil
	}
	if err != nil {
		return domain.Teacher{}, false, glerrors.Wrap(glerrors.KindInternal, "loading teacher", err)
	}
	te, err := toDomainTeacher(m)
	if err != nil {
		return domain.Teacher{}, false, err
	}
	return te, true, nil
}

func groupsByID(tx *gorm.DB, ids []string) (map[string]domain.Group, error) {
	out := make(map[string]domain.Group, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []groupModel
	if err := tx.Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "loading groups", err)
	}
	for _, m := range rows {
		g, err := toDomainGroup(m)
		if err != nil {
			return nil, err
		}
		out[g.ID()] = g
	}
	return out, nil
}

func subjectsByID(tx *gorm.DB, ids []string) (map[string]domain.Subject, error) {
	out := make(map[string]domain.Subject, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []subjectModel
	if err := tx.Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "loading subjects", err)
	}
	for _, m := range rows {
		s, err := toDomainSubject(m)
		if err != nil {
			return nil, err
		}
		out[s.ID()] = s
	}
	return out, nil
}

func teachersByID(tx *gorm.DB, ids []string) (map[string]domain.Teacher, error) {
	out := make(map[string]domain.Teacher, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []teacherModel
	if err := tx.Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "loading teachers", err)
	}
	for _, m := range rows {
		te, err := toDomainTeacher(m)
		if err != nil {
			return nil, err
		}
		out[te.ID()] = te
	}
	return out, nil
}

// occupiedSlots locks and returns the assignments sitting on the requested
// (group, subject) slots. The group fixes the cycle, so (group_id, subject_id)
// identifies a slot as precisely as the UNIQUE's (cycle, group, subject).
func occupiedSlots(tx *gorm.DB, pairs [][]any) (map[domain.TeachingSlot]domain.TeachingAssignment, error) {
	out := make(map[domain.TeachingSlot]domain.TeachingAssignment, len(pairs))
	if len(pairs) == 0 {
		return out, nil
	}
	var rows []teachingAssignmentModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("(group_id, subject_id) IN ?", pairs).
		Order("id").Find(&rows).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "locking occupied teaching slots", err)
	}
	for _, m := range rows {
		a, err := toDomainTeachingAssignment(m)
		if err != nil {
			return nil, err
		}
		out[a.Slot()] = a
	}
	return out, nil
}

// writeTeachingLoad inserts the new assignments (in lock order) and moves the
// reassigned ones.
func writeTeachingLoad(tx *gorm.DB, w ports.TeachingLoadWrite) error {
	if len(w.Create) > 0 {
		models := make([]teachingAssignmentModel, 0, len(w.Create))
		for _, a := range inLockOrder(w.Create) {
			models = append(models, fromDomainTeachingAssignment(a))
		}
		if err := tx.Create(&models).Error; err != nil {
			return translateCreateErr(err, "creating teaching load", "a teacher is already assigned to this group and subject in the cycle")
		}
	}
	for _, a := range w.Reassign {
		if err := setAssignmentTeacher(tx, a.ID(), a.TeacherID()); err != nil {
			return err
		}
	}
	return nil
}

// setAssignmentTeacher moves one assignment to teacherID. No row is
// KindNotFound; a teacher that does not exist trips the FK (KindInvalid).
func setAssignmentTeacher(tx *gorm.DB, id, teacherID string) error {
	res := tx.Model(&teachingAssignmentModel{}).
		Where("id = ?", id).
		Updates(map[string]any{"teacher_id": teacherID, "updated_at": gorm.Expr("now()")})
	if res.Error != nil {
		return translateCreateErr(res.Error, "reassigning teaching assignment", "a teacher is already assigned to this group and subject in the cycle")
	}
	if res.RowsAffected == 0 {
		return glerrors.ErrNotFound
	}
	return nil
}

// UpdateTeachingAssignmentTeacher moves one assignment to teacherID.
func (r *Repository) UpdateTeachingAssignmentTeacher(ctx context.Context, id, teacherID string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		return setAssignmentTeacher(tx, id, teacherID)
	})
}

// DeleteTeachingAssignment removes one assignment. An id that matches no row
// (or another tenant's row, hidden by RLS) is KindNotFound, so the caller
// learns whether the slot was actually freed.
func (r *Repository) DeleteTeachingAssignment(ctx context.Context, id string) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Where("id = ?", id).Delete(&teachingAssignmentModel{})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "deleting teaching assignment", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
	})
}

// inLockOrder returns a copy of as sorted by (group_id, subject_id): the key of
// the one-teacher-per-slot UNIQUE (the group fixes the cycle). Inserting in
// that order makes every concurrent load take the index locks in the same
// order, so overlapping loads queue instead of deadlocking. The caller's slice
// keeps its request order.
func inLockOrder(as []domain.TeachingAssignment) []domain.TeachingAssignment {
	out := slices.Clone(as)
	slices.SortFunc(out, func(a, b domain.TeachingAssignment) int {
		return cmp.Or(strings.Compare(a.GroupID(), b.GroupID()), strings.Compare(a.SubjectID(), b.SubjectID()))
	})
	return out
}
