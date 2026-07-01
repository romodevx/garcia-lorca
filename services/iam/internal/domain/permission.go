package domain

import (
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Permission is a "resource:action" capability, e.g. "students:read" or
// "finance.payment:create". Authorization is permission-based: a principal's
// effective permissions are the union across its roles, and gates check a
// specific Permission — never a role name.
//
//	"students:read" ──> Permission{resource:"students", action:"read"}
type Permission struct {
	resource string
	action   string
}

// NewPermission parses and validates a "resource:action" string (trimmed and
// lowercased). Exactly one ':' with non-empty sides is required.
func NewPermission(raw string) (Permission, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	resource, action, found := strings.Cut(value, ":")
	if !found {
		return Permission{}, glerrors.New(glerrors.KindInvalid, "permission must be 'resource:action'")
	}
	resource = strings.TrimSpace(resource)
	action = strings.TrimSpace(action)
	if resource == "" || action == "" || strings.Contains(action, ":") {
		return Permission{}, glerrors.New(glerrors.KindInvalid, "permission must be 'resource:action' with non-empty parts")
	}
	return Permission{resource: resource, action: action}, nil
}

// Resource is the object the permission applies to (e.g. "students").
func (p Permission) Resource() string { return p.resource }

// Action is the operation permitted (e.g. "read").
func (p Permission) Action() string { return p.action }

// String renders the canonical "resource:action" form.
func (p Permission) String() string { return p.resource + ":" + p.action }
