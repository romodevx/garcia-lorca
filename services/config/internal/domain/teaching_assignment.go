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
