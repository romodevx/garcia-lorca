package domain

import (
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Group is a grupo within a cycle for a given grade level, e.g. "A". `capacity`
// is the seat count (0 means unbounded / not yet set).
type Group struct {
	id           string
	tenantID     string
	cycleID      string
	gradeLevelID string
	name         string
	capacity     int
}

// NewGroup constructs a group. `id`, `tenantID`, `cycleID`, `gradeLevelID`, and
// `name` are required, and `capacity` must not be negative.
func NewGroup(id, tenantID, cycleID, gradeLevelID, name string, capacity int) (Group, error) {
	if strings.TrimSpace(id) == "" {
		return Group{}, glerrors.New(glerrors.KindInvalid, "group id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return Group{}, glerrors.New(glerrors.KindInvalid, "group tenant id is required")
	}
	if strings.TrimSpace(cycleID) == "" {
		return Group{}, glerrors.New(glerrors.KindInvalid, "group cycle id is required")
	}
	if strings.TrimSpace(gradeLevelID) == "" {
		return Group{}, glerrors.New(glerrors.KindInvalid, "group grade level id is required")
	}
	if strings.TrimSpace(name) == "" {
		return Group{}, glerrors.New(glerrors.KindInvalid, "group name is required")
	}
	if capacity < 0 {
		return Group{}, glerrors.New(glerrors.KindInvalid, "group capacity must not be negative")
	}
	return Group{
		id:           id,
		tenantID:     tenantID,
		cycleID:      cycleID,
		gradeLevelID: gradeLevelID,
		name:         name,
		capacity:     capacity,
	}, nil
}

// ID returns the stable group identifier.
func (g Group) ID() string { return g.id }

// TenantID returns the tenant the group belongs to.
func (g Group) TenantID() string { return g.tenantID }

// CycleID returns the parent cycle identifier.
func (g Group) CycleID() string { return g.cycleID }

// GradeLevelID returns the grade level the group belongs to.
func (g Group) GradeLevelID() string { return g.gradeLevelID }

// Name returns the human-readable group name (e.g. "A").
func (g Group) Name() string { return g.name }

// Capacity returns the seat count. 0 means not set yet: no placement path
// (automatic, chosen group, transfer) puts a pupil in such a group.
func (g Group) Capacity() int { return g.capacity }
