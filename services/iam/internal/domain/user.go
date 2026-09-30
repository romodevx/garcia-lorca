package domain

import (
	"errors"
	"strings"
	"time"

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
	// StatusInvited: the account was created and its owner has not yet proved
	// they read the mailbox. It holds roles and cannot log in. This is the
	// difference between "created" and "usable" — before it existed, a mistyped
	// address became a working account nobody could reach.
	StatusInvited
	// StatusDeleted: the account was removed by the director (T2). It cannot
	// log in and it leaves every listing, but the row stays: payments it
	// registered, grades it captured and the audit trail name it by id, so a
	// hard delete would orphan the school's history. Inviting the same address
	// again brings THIS account back, history included (owner answer 3).
	StatusDeleted
)

// ErrProviderPending marks a deletion whose row is done — nobody can sign in
// here any more — but whose identity-provider half failed. Deleting the same
// account again retries only that half. The transport gives it its own code so
// the screen can say exactly this instead of "it did not work".
var ErrProviderPending = errors.New("user deleted; identity provider not yet disabled")

// ErrLastOwner is the refusal to delete the only active account that holds the
// owner role: without one, nobody could administer the school again.
var ErrLastOwner = glerrors.New(glerrors.KindConflict,
	"no se puede eliminar al último director activo")

// User is the IAM principal: a stable identity with an email (unique per
// tenant), a lifecycle status, and a set of assigned roles. Authorization is
// derived from the union of the roles' permissions.
//
//	roles ─┬─ Docente {students:read, grades:create}
//	       └─ Tutor   {students:read, attendance:read}
//	EffectivePermissions = {students:read, grades:create, attendance:read}
type User struct {
	id                 string
	tenantID           string
	email              Email
	displayName        string
	status             Status
	mustChangePassword bool
	roles              []Role
	// passwordChangedAt is the instant the credential was last rotated. Refresh
	// tokens minted before it are no longer valid, so a password change revokes
	// every previously issued refresh token. The zero value means "no floor"
	// (a user built in memory imposes no revocation cutoff); the DB is the source
	// of truth, mapped in on load.
	passwordChangedAt time.Time
}

// NewUser constructs a principal. `id` and `tenantID` are required; the email is
// already a validated value object.
func NewUser(id, tenantID string, email Email, displayName string, status Status, mustChangePassword bool, roles []Role) (User, error) {
	if strings.TrimSpace(id) == "" {
		return User{}, glerrors.New(glerrors.KindInvalid, "user id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return User{}, glerrors.New(glerrors.KindInvalid, "user tenant id is required")
	}
	assigned := make([]Role, len(roles))
	copy(assigned, roles)
	return User{
		id:                 id,
		tenantID:           tenantID,
		email:              email,
		displayName:        displayName,
		status:             status,
		mustChangePassword: mustChangePassword,
		roles:              assigned,
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

// IsDeleted reports whether the account was removed (soft delete).
func (u User) IsDeleted() bool { return u.status == StatusDeleted }

// MustChangePassword reports whether the account still holds a temporary
// password and must rotate it before doing anything else.
func (u User) MustChangePassword() bool { return u.mustChangePassword }

// WithoutPendingRotation returns the user with the forced-rotation flag cleared.
//
// The flag means "this account holds a temporary password somebody else chose,
// and the only allowed action is replacing it". When an external provider owns
// the credential that sentence is not true of anything: there is no password of
// ours to replace. Carrying the flag into a provider session makes the gateway
// refuse every route while the login itself looks perfectly fine.
//
// A value method returning a copy, not a mutation: the row in the database is
// untouched, so an account can still fall back to the password path with its
// rotation still pending if the provider is ever switched off.
func (u User) WithoutPendingRotation() User {
	u.mustChangePassword = false
	return u
}

// PasswordChangedAt returns the instant the credential was last rotated (zero if
// unknown — imposes no refresh-token revocation floor).
func (u User) PasswordChangedAt() time.Time { return u.passwordChangedAt }

// WithPasswordChangedAt returns a copy of the user carrying the given rotation
// instant. The persistence layer uses it to hydrate the stored value on load,
// keeping NewUser's creation signature unchanged.
func (u User) WithPasswordChangedAt(t time.Time) User {
	u.passwordChangedAt = t
	return u
}

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

// RoleIDs returns the ids of the user's assigned roles, in assignment order.
// The admin surface projects this onto the wire (User.role_ids) so callers can
// see and edit a user's role set.
func (u User) RoleIDs() []string {
	out := make([]string, 0, len(u.roles))
	for _, role := range u.roles {
		out = append(out, role.ID())
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
