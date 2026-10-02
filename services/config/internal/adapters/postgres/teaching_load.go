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
	if snap.Groups, err = byIDs(tx, groupIDs, toDomainGroup, "groups"); err != nil {
		return ports.TeachingLoadSnapshot{}, err
	}
	if snap.Subjects, err = byIDs(tx, subjectIDs, toDomainSubject, "subjects"); err != nil {
		return ports.TeachingLoadSnapshot{}, err
	}
	if snap.Occupied, err = occupiedSlots(tx, pairs); err != nil {
		return ports.TeachingLoadSnapshot{}, err
	}
	holderIDs := make([]string, 0, len(snap.Occupied))
	for _, a := range snap.Occupied {
		holderIDs = append(holderIDs, a.TeacherID())
	}
	if snap.Holders, err = byIDs(tx, holderIDs, toDomainTeacher, "teachers"); err != nil {
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

// byIDs loads the rows whose id is in ids and maps them to domain values keyed
// by id. An empty ids slice answers an empty map without a query. what names
// the rows in the wrapped error ("groups", "subjects", ...).
func byIDs[M any, D interface{ ID() string }](tx *gorm.DB, ids []string, toDomain func(M) (D, error), what string) (map[string]D, error) {
	out := make(map[string]D, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []M
	if err := tx.Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, glerrors.Wrap(glerrors.KindInternal, "loading "+what, err)
	}
	for _, m := range rows {
		d, err := toDomain(m)
		if err != nil {
			return nil, err
		}
		out[d.ID()] = d
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
		if pgCode(res.Error) == exclusionViolation {
			// 0017: the assignment's classes move to the new teacher (FK ON
			// UPDATE CASCADE) and the timetable refuses a double-booking.
			return domain.ScheduleRefusal(glerrors.KindConflict, domain.CodeScheduleTeacherBusy,
				"El docente nuevo ya tiene clase a la hora de las clases de esta asignación en el horario; muévelas antes de reasignar")
		}
		return translateCreateErr(res.Error, "reassigning teaching assignment", "a teacher is already assigned to this group and subject in the cycle")
	}
	if res.RowsAffected == 0 {
		return glerrors.ErrNotFound
	}
	return nil
}

// ReassignTeachingAssignmentTx locks the assignment, checks the target teacher
// and moves the row, all in one tenant-bound transaction: a concurrent
// reassign or delete of the same slot waits on the row lock instead of racing
// the decision. An unknown id (or another tenant's, hidden by RLS) is
// KindNotFound.
func (r *Repository) ReassignTeachingAssignmentTx(ctx context.Context, id, teacherID string,
	decide func(domain.TeachingAssignment, bool) (bool, error)) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		a, err := lockTeachingAssignment(tx, id)
		if err != nil {
			return err
		}
		_, teacherFound, err := findTeacher(tx, teacherID)
		if err != nil {
			return err
		}
		write, err := decide(a, teacherFound)
		if err != nil || !write {
			return err
		}
		return setAssignmentTeacher(tx, id, teacherID)
	})
}

// lockTeachingAssignment reads one assignment FOR UPDATE, or KindNotFound.
func lockTeachingAssignment(tx *gorm.DB, id string) (domain.TeachingAssignment, error) {
	var m teachingAssignmentModel
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.TeachingAssignment{}, glerrors.ErrNotFound
	}
	if err != nil {
		return domain.TeachingAssignment{}, translateCreateErr(err, "locking teaching assignment", "the assignment changed concurrently")
	}
	return toDomainTeachingAssignment(m)
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
