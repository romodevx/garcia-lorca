package usecase

import (
	"context"
	"fmt"
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
)

// MaxTeachingLoadItems bounds one AssignTeachingLoad request. It is a
// TECHNICAL guard on the size of a single transaction (and of the row locks it
// takes), not a business rule: a teacher may hold any number of classes, spread
// over as many requests as needed.
const MaxTeachingLoadItems = 500

// TeachingLoadRequest gives one teacher a batch of (group, subject) slots.
type TeachingLoadRequest struct {
	TeacherID string
	Items     []domain.TeachingSlot
	// ReplaceConflicts hands slots another teacher holds to TeacherID instead
	// of reporting them as conflicts.
	ReplaceConflicts bool
}

// TeachingLoadConflict is a requested slot another teacher already holds.
type TeachingLoadConflict struct {
	Slot         domain.TeachingSlot
	AssignmentID string
	TeacherID    string
	TeacherName  string
}

// TeachingLoadResult says what happened to every item, in request order within
// each list.
type TeachingLoadResult struct {
	Created    []domain.TeachingAssignment
	Reassigned []domain.TeachingAssignment
	Skipped    []domain.TeachingSlot // already held by the teacher
	Conflicts  []TeachingLoadConflict
}

// AssignTeachingLoad gives a teacher a whole teaching load in one transaction.
//
// The request shape is checked first (a teacher, 1..MaxTeachingLoadItems items,
// no blank ids, no slot twice). Then, against a snapshot the repository reads
// under lock, every item is validated exactly as CreateTeachingAssignment does
// (group and subject exist, the subject is taught in the group's grade level,
// the teacher exists). One bad item rejects the whole request with a
// KindInvalid naming it as items[i], and nothing is written.
//
// Valid items are then classified: a free slot is created (cycle derived from
// the group), a slot the teacher already holds is skipped, and a slot another
// teacher holds is either reported as a conflict and left alone or, with
// ReplaceConflicts, reassigned to this teacher keeping its id. There is still
// one teacher per slot per cycle — replacing is explicit, never co-teaching.
func (s *CatalogService) AssignTeachingLoad(ctx context.Context, req TeachingLoadRequest) (TeachingLoadResult, error) {
	if err := validateTeachingLoadShape(req); err != nil {
		return TeachingLoadResult{}, err
	}
	var result TeachingLoadResult
	err := s.repo.ApplyTeachingLoad(ctx, req.TeacherID, req.Items, func(snap ports.TeachingLoadSnapshot) (ports.TeachingLoadWrite, error) {
		var perr error
		result, perr = s.planTeachingLoad(req, snap)
		if perr != nil {
			return ports.TeachingLoadWrite{}, perr
		}
		return ports.TeachingLoadWrite{Create: result.Created, Reassign: result.Reassigned}, nil
	})
	if err != nil {
		return TeachingLoadResult{}, err
	}
	return result, nil
}

// validateTeachingLoadShape checks what can be checked without the database.
func validateTeachingLoadShape(req TeachingLoadRequest) error {
	if strings.TrimSpace(req.TeacherID) == "" {
		return glerrors.New(glerrors.KindInvalid, "teacher id is required")
	}
	if len(req.Items) == 0 {
		return glerrors.New(glerrors.KindInvalid, "items must not be empty")
	}
	if len(req.Items) > MaxTeachingLoadItems {
		return glerrors.New(glerrors.KindInvalid, fmt.Sprintf("items must not exceed %d per request", MaxTeachingLoadItems))
	}
	seen := make(map[domain.TeachingSlot]int, len(req.Items))
	for i, item := range req.Items {
		if strings.TrimSpace(item.GroupID) == "" || strings.TrimSpace(item.SubjectID) == "" {
			return invalidItem(i, "group id and subject id are required")
		}
		if first, dup := seen[item]; dup {
			return invalidItem(i, fmt.Sprintf("repeats items[%d]", first))
		}
		seen[item] = i
	}
	return nil
}

// planTeachingLoad validates every item against the snapshot and decides, per
// item, create / skip / conflict / reassign. It is pure: it only reads snap.
func (s *CatalogService) planTeachingLoad(req TeachingLoadRequest, snap ports.TeachingLoadSnapshot) (TeachingLoadResult, error) {
	if !snap.TeacherFound {
		return TeachingLoadResult{}, glerrors.New(glerrors.KindInvalid, "teacher not found")
	}
	var out TeachingLoadResult
	for i, item := range req.Items {
		grp, err := teachableSlot(i, item, snap)
		if err != nil {
			return TeachingLoadResult{}, err
		}
		held, occupied := snap.Occupied[item]
		switch {
		case !occupied:
			a, err := domain.NewTeachingAssignment(s.newID(), s.tenant, grp.CycleID(), item.GroupID, item.SubjectID, req.TeacherID)
			if err != nil {
				return TeachingLoadResult{}, err
			}
			out.Created = append(out.Created, a)
		case held.HeldBy(req.TeacherID):
			out.Skipped = append(out.Skipped, item)
		case req.ReplaceConflicts:
			moved, err := held.ReassignTo(req.TeacherID)
			if err != nil {
				return TeachingLoadResult{}, err
			}
			out.Reassigned = append(out.Reassigned, moved)
		default:
			out.Conflicts = append(out.Conflicts, TeachingLoadConflict{
				Slot:         item,
				AssignmentID: held.ID(),
				TeacherID:    held.TeacherID(),
				TeacherName:  snap.Holders[held.TeacherID()].FullName(),
			})
		}
	}
	return out, nil
}

// teachableSlot checks one item's group and subject exist and agree on the
// grade level, returning the group (whose cycle the assignment inherits).
func teachableSlot(i int, item domain.TeachingSlot, snap ports.TeachingLoadSnapshot) (domain.Group, error) {
	grp, ok := snap.Groups[item.GroupID]
	if !ok {
		return domain.Group{}, invalidItem(i, "group not found")
	}
	subj, ok := snap.Subjects[item.SubjectID]
	if !ok {
		return domain.Group{}, invalidItem(i, "subject not found")
	}
	if subj.GradeLevelID() != grp.GradeLevelID() {
		return domain.Group{}, invalidItem(i, "subject does not belong to the group's grade level")
	}
	return grp, nil
}

// invalidItem names the offending item so the caller can point at it.
func invalidItem(i int, msg string) error {
	return glerrors.New(glerrors.KindInvalid, fmt.Sprintf("items[%d]: %s", i, msg))
}

// DeleteTeachingAssignment frees a slot (KindNotFound if the id is unknown).
func (s *CatalogService) DeleteTeachingAssignment(ctx context.Context, id string) error {
	return s.repo.DeleteTeachingAssignment(ctx, id)
}

// ReassignTeachingAssignment hands an existing slot to another teacher, keeping
// the assignment id. An unknown assignment is KindNotFound; an unknown teacher
// is KindInvalid (a precondition on the request body, as in
// CreateTeachingAssignment). Reassigning to the current teacher writes nothing.
// The read, the teacher check and the write share one transaction with the
// assignment locked, so a concurrent change cannot slip between them.
func (s *CatalogService) ReassignTeachingAssignment(ctx context.Context, id, teacherID string) (domain.TeachingAssignment, error) {
	var out domain.TeachingAssignment
	err := s.repo.ReassignTeachingAssignmentTx(ctx, id, teacherID, func(a domain.TeachingAssignment, teacherFound bool) (bool, error) {
		if a.HeldBy(teacherID) {
			out = a
			return false, nil
		}
		if !teacherFound {
			return false, glerrors.New(glerrors.KindInvalid, "teacher not found")
		}
		moved, err := a.ReassignTo(teacherID)
		if err != nil {
			return false, err
		}
		out = moved
		return true, nil
	})
	if err != nil {
		return domain.TeachingAssignment{}, err
	}
	return out, nil
}
