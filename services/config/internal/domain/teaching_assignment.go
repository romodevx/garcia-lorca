package domain

import (
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// TeachingAssignment is the knot binding a teacher to a group and a subject
// within a cycle. The cross-aggregate relational invariants (group belongs to
// the cycle, subject is teachable for the group's grade level, teacher exists)
// are enforced by the use case, which can see the other aggregates; the domain
// only guarantees every reference is present.
type TeachingAssignment struct {
	id        string
	tenantID  string
	cycleID   string
	groupID   string
	subjectID string
	teacherID string
}

// NewTeachingAssignment constructs a teaching assignment. All six ids are
// required; the use case derives `cycleID` from the group so it cannot drift.
func NewTeachingAssignment(id, tenantID, cycleID, groupID, subjectID, teacherID string) (TeachingAssignment, error) {
	if strings.TrimSpace(id) == "" {
		return TeachingAssignment{}, glerrors.New(glerrors.KindInvalid, "teaching assignment id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return TeachingAssignment{}, glerrors.New(glerrors.KindInvalid, "teaching assignment tenant id is required")
	}
	if strings.TrimSpace(cycleID) == "" {
		return TeachingAssignment{}, glerrors.New(glerrors.KindInvalid, "teaching assignment cycle id is required")
	}
	if strings.TrimSpace(groupID) == "" {
		return TeachingAssignment{}, glerrors.New(glerrors.KindInvalid, "teaching assignment group id is required")
	}
	if strings.TrimSpace(subjectID) == "" {
		return TeachingAssignment{}, glerrors.New(glerrors.KindInvalid, "teaching assignment subject id is required")
	}
	if strings.TrimSpace(teacherID) == "" {
		return TeachingAssignment{}, glerrors.New(glerrors.KindInvalid, "teaching assignment teacher id is required")
	}
	return TeachingAssignment{
		id:        id,
		tenantID:  tenantID,
		cycleID:   cycleID,
		groupID:   groupID,
		subjectID: subjectID,
		teacherID: teacherID,
	}, nil
}

// ID returns the stable teaching-assignment identifier.
func (a TeachingAssignment) ID() string { return a.id }

// TenantID returns the tenant the assignment belongs to.
func (a TeachingAssignment) TenantID() string { return a.tenantID }

// CycleID returns the cycle the assignment is scoped to (derived from the group).
func (a TeachingAssignment) CycleID() string { return a.cycleID }

// GroupID returns the assigned group identifier.
func (a TeachingAssignment) GroupID() string { return a.groupID }

// SubjectID returns the assigned subject identifier.
func (a TeachingAssignment) SubjectID() string { return a.subjectID }

// TeacherID returns the assigned teacher identifier.
func (a TeachingAssignment) TeacherID() string { return a.teacherID }

// TeachingSlot is one (group, subject) place a teacher can be given. The group
// fixes the cycle, so a slot is unique per cycle and holds at most one teacher
// (the owner ruled out co-teaching; see migration 0002's UNIQUE).
type TeachingSlot struct {
	GroupID   string
	SubjectID string
}

// Slot returns the (group, subject) place this assignment occupies.
func (a TeachingAssignment) Slot() TeachingSlot {
	return TeachingSlot{GroupID: a.groupID, SubjectID: a.subjectID}
}

// HeldBy reports whether teacherID is the teacher holding this slot.
func (a TeachingAssignment) HeldBy(teacherID string) bool {
	return teacherID != "" && a.teacherID == teacherID
}

// ReassignTo returns the same assignment (same id, cycle and slot) handed to
// another teacher. The receiver is left untouched.
func (a TeachingAssignment) ReassignTo(teacherID string) (TeachingAssignment, error) {
	if strings.TrimSpace(teacherID) == "" {
		return TeachingAssignment{}, glerrors.New(glerrors.KindInvalid, "teaching assignment teacher id is required")
	}
	a.teacherID = teacherID
	return a, nil
}
