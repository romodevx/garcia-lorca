package domain

import (
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Status is a user's account lifecycle state. Only an active user may
// authenticate.
type Status uint8

const (
	// StatusUnspecified is the zero value; treat as non-authenticable.
	StatusUnspecified Status = iota
	// StatusActive: the account is usable and may log in.
	StatusActive
	// StatusSuspended: the account exists but cannot log in.
	StatusSuspended
)

// User is the IAM principal: a stable identity with an email (unique per
// tenant), a lifecycle status, and a set of assigned roles. Authorization is
// derived from the union of the roles' permissions.
//
//	roles ─┬─ Docente {students:read, grades:create}
//	       └─ Tutor   {students:read, attendance:read}
//	EffectivePermissions = {students:read, grades:create, attendance:read}
type User struct {
	id          string
	tenantID    string
	email       Email
	displayName string
	status      Status
	roles       []Role
}

// NewUser constructs a principal. `id` and `tenantID` are required; the email is
// already a validated value object.
func NewUser(id, tenantID string, email Email, displayName string, status Status, roles []Role) (User, error) {
	if strings.TrimSpace(id) == "" {
		return User{}, glerrors.New(glerrors.KindInvalid, "user id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return User{}, glerrors.New(glerrors.KindInvalid, "user tenant id is required")
	}
	assigned := make([]Role, len(roles))
	copy(assigned, roles)
	return User{
		id:          id,
		tenantID:    tenantID,
		email:       email,
		displayName: displayName,
		status:      status,
		roles:       assigned,
	}, nil
}

// ID returns the stable user identifier.
func (u User) ID() string { return u.id }

// TenantID returns the tenant the user belongs to.
func (u User) TenantID() string { return u.tenantID }

// Email returns the user's normalized email value object.
func (u User) Email() Email { return u.email }

// DisplayName returns the human-readable name.
func (u User) DisplayName() string { return u.displayName }

// Status returns the account lifecycle state.
func (u User) Status() Status { return u.status }

// CanAuthenticate reports whether the account may log in (active only).
func (u User) CanAuthenticate() bool { return u.status == StatusActive }

// EffectivePermissions returns the deduplicated union of permissions across all
// of the user's roles — the authoritative set every authorization check uses.
func (u User) EffectivePermissions() []Permission {
	seen := make(map[string]struct{})
	out := make([]Permission, 0)
	for _, role := range u.roles {
		for _, perm := range role.Permissions() {
			key := perm.String()
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, perm)
		}
	}
	return out
}

// HasPermission reports whether the user's effective permissions include want.
func (u User) HasPermission(want Permission) bool {
	for _, role := range u.roles {
		if role.HasPermission(want) {
			return true
		}
	}
	return false
}
