package domain

import (
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// OwnerRoleName is the name of the seeded super-role (migrations/iam). Kept for
// documentation of the seed; it is NOT used for any authorization decision —
// role names are admin-typed and case-collidable, so they must never gate a
// grant (see IsOwnerRoleID).
const OwnerRoleName = "owner"

// OwnerRoleID is the fixed UUID of the seeded owner role
// (migrations/iam/0002_owner_and_password_policy.sql). It is the single stable,
// unforgeable identity of the tenant super-role: the owner-only grant guard keys
// off THIS, never the mutable role name. Role-name uniqueness in Postgres is
// case-sensitive, so a name like "Owner" is a distinct, insertable role that
// must not inherit the owner exemption.
const OwnerRoleID = "00000000-0000-0000-0000-0000000000a1"

// IsOwnerRoleID reports whether id is the seeded owner role. Grant-time guards
// use this — an id cannot be spoofed by choosing a colliding role name.
func IsOwnerRoleID(id string) bool {
	return strings.TrimSpace(id) == OwnerRoleID
}

// GuardianRoleID is the fixed UUID of the seeded `guardian` role
// (migrations/iam/0010_guardians.sql): exactly family:read, the family app.
//
// It is not an ordinary role to hand out. A guardian account means nothing
// without the guardian↔student link that students owns, so the role is given
// ONLY by AdminService.InviteGuardian — the road behind "Dar acceso a la app"
// on the pupil's record. Every manual road (invite, create, assign) refuses it.
const GuardianRoleID = "00000000-0000-0000-0000-0000000000a2"

// IsGuardianRoleID reports whether id names the seeded guardian role. Case and
// surrounding space are ignored because Postgres reads a uuid that way: a guard
// that compared bytes could be walked around by upper-casing the id.
func IsGuardianRoleID(id string) bool {
	return strings.EqualFold(strings.TrimSpace(id), GuardianRoleID)
}

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
