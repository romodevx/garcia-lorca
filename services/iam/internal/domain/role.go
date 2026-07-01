package domain

import (
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Role is a named bundle of permissions. Roles exist only to group permissions;
// authorization always resolves to permissions, so callers never compare role
// names. Permissions are stored deduplicated and validated.
type Role struct {
	id          string
	name        string
	permissions []Permission
}

// NewRole builds a role from raw "resource:action" permission strings, parsing
// and de-duplicating them. A blank name or any malformed permission is a
// KindInvalid error.
func NewRole(id, name string, rawPermissions []string) (Role, error) {
	if strings.TrimSpace(name) == "" {
		return Role{}, glerrors.New(glerrors.KindInvalid, "role name is required")
	}
	seen := make(map[string]struct{}, len(rawPermissions))
	permissions := make([]Permission, 0, len(rawPermissions))
	for _, raw := range rawPermissions {
		perm, err := NewPermission(raw)
		if err != nil {
			return Role{}, err
		}
		key := perm.String()
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		permissions = append(permissions, perm)
	}
	return Role{id: id, name: strings.TrimSpace(name), permissions: permissions}, nil
}

// ID returns the role's stable identifier.
func (r Role) ID() string { return r.id }

// Name returns the human-readable role name.
func (r Role) Name() string { return r.name }

// Permissions returns a copy of the role's permission set (defensive; callers
// cannot mutate the role's internals).
func (r Role) Permissions() []Permission {
	out := make([]Permission, len(r.permissions))
	copy(out, r.permissions)
	return out
}

// HasPermission reports whether this role grants the given permission.
func (r Role) HasPermission(want Permission) bool {
	for _, p := range r.permissions {
		if p == want {
			return true
		}
	}
	return false
}
