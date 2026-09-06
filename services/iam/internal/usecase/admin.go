package usecase

import (
	"context"
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/ports"
)

// AdminService is the identity administration surface: creating users, managing
// roles (bundles of permissions), assigning roles, and changing account status.
// Like AuthService it depends only on ports and the domain; the composition root
// injects concrete adapters (postgres, bcrypt). Who may call these methods is
// enforced at the gateway/RPC boundary (users:write / roles:write).
//
// New users receive an admin-provided temporary password: it is hashed before it
// ever reaches the repository, and the account is flagged must_change_password so
// the Phase-1 forced-rotation flow applies at first login.
type AdminService struct {
	users  ports.UserRepository
	roles  ports.RoleRepository
	hasher ports.PasswordHasher
	// provisioner creates the account at the external identity provider. NIL is
	// a valid state — an environment with no provider configured simply cannot
	// invite anyone (InviteUser refuses), while the older password-based
	// CreateUser keeps working.
	provisioner ports.IdentityProvisioner
	tenantID    string
	newID       func() string
}

// NewAdminService wires the admin use case. `tenantID` is the single-tenant scope
// for this phase; `newID` mints identifiers for new users and roles (injectable
// for deterministic tests).
func NewAdminService(
	users ports.UserRepository,
	roles ports.RoleRepository,
	hasher ports.PasswordHasher,
	provisioner ports.IdentityProvisioner,
	tenantID string,
	newID func() string,
) *AdminService {
	return &AdminService{
		users:       users,
		roles:       roles,
		hasher:      hasher,
		provisioner: provisioner,
		tenantID:    tenantID,
		newID:       newID,
	}
}

// InviteUser creates a principal WITHOUT anybody inventing a password for them.
//
// The account is created at the provider with no password. Its owner sets the
// first one through a password-reset link, and using that link is what proves
// they control the mailbox: the address ends up verified because they verified
// it, not because we asserted it. A mistyped address then fails loudly and
// early — the link never arrives and the account is never usable — instead of
// sitting there as a pre-verified account its real owner could later claim.
//
// WHAT THIS FUNCTION DOES NOT DO: send that link. Nothing here emails anybody.
// This service has no mail sender (comms does push, not email) and the Admin SDK
// generates action links without delivering them, so the send belongs to the
// caller — the admin screen asks the provider to mail it, which also gives the
// director a "resend invitation" button that doubles as the password reset iam
// never had. Until that screen exists, an invited account is created and
// unreachable, and this comment says so rather than implying otherwise.
//
// Contrast CreateUser below, which takes a temporary password the director
// types and hands over: somebody else knows your password, this service stores
// a credential nobody chose, and the account works before its owner has proved
// anything about the address.
//
// ORDER IS LOAD-BEARING: the provider first, the row second. A row written
// before a refused provider call would leave a user who exists here, holds
// permissions, and can never sign in — invisible until somebody tries.
func (s *AdminService) InviteUser(ctx context.Context, rawEmail, displayName string, roleIDs []string) (domain.User, error) {
	email, err := domain.NewEmail(rawEmail)
	if err != nil {
		return domain.User{}, err // KindInvalid, before either side is touched.
	}
	if s.provisioner == nil {
		// KindInternal, not KindInvalid: the caller did nothing wrong, the
		// environment is missing a provider.
		return domain.User{}, glerrors.New(glerrors.KindInternal,
			"no identity provider is configured; cannot invite users")
	}

	assigned, err := s.resolveRoles(ctx, roleIDs)
	if err != nil {
		return domain.User{}, err
	}

	if err := s.provisioner.CreateAccount(ctx, email.String(), displayName); err != nil {
		return domain.User{}, err
	}

	// mustChangePassword is FALSE: there is no password to change. The owner
	// chooses their first one through the invitation.
	user, err := domain.NewUser(s.newID(), s.tenantID, email, displayName, domain.StatusActive, false, assigned)
	if err != nil {
		return domain.User{}, err
	}

	// No hash. The provider owns credentials; a second copy here would be one
	// more thing to drift, leak, or be reverted by a re-import.
	if err := s.users.CreateUser(ctx, user, ""); err != nil {
		return domain.User{}, err // KindConflict on duplicate email
	}
	if err := s.users.SetUserRoles(ctx, user.ID(), roleIDs); err != nil {
		return domain.User{}, err
	}

	// Last, and NOT rolled back if it fails. An account created and never
	// announced is one nobody can use and the director has nothing to hand the
	// person — so the caller is told. But deleting the account over one
	// undelivered email would throw away something perfectly good and, worse,
	// free the address for somebody else to register. Resending is the fix, and
	// resending needs the account to still be there.
	if err := s.provisioner.SendInvitation(ctx, email.String()); err != nil {
		return user, glerrors.Wrap(glerrors.KindInternal,
			"the account was created but the invitation could not be sent; resend it", err)
	}
	return user, nil
}

// CreateUser provisions a new principal from an admin-supplied temporary
// password and assigns the given roles. The temp password is hashed (never
// stored in plaintext) and the account is flagged must_change_password. Every
// requested role is validated to exist BEFORE any write, so a bad role id is
// rejected (KindInvalid) without persisting anything. A blank temp password is
// KindInvalid; a duplicate email is KindConflict.
//
// NOTE: the user insert and the role assignment are two separate transactions
// (not one atomic write). This fails SAFE — a transient failure of the role
// assignment after the user is created leaves a role-less user (zero
// permissions, never an escalation), which an admin can fix by re-running
// SetUserRoles. Promote to a single-tx CreateUserWithRoles if provisioning must
// be all-or-nothing.
func (s *AdminService) CreateUser(ctx context.Context, rawEmail, displayName, tempPassword string, roleIDs []string) (domain.User, error) {
	if strings.TrimSpace(tempPassword) == "" {
		return domain.User{}, glerrors.New(glerrors.KindInvalid, "temporary password is required")
	}
	email, err := domain.NewEmail(rawEmail)
	if err != nil {
		return domain.User{}, err // KindInvalid
	}

	assigned, err := s.resolveRoles(ctx, roleIDs)
	if err != nil {
		return domain.User{}, err
	}

	hash, err := s.hasher.Hash(tempPassword)
	if err != nil {
		return domain.User{}, err
	}

	user, err := domain.NewUser(s.newID(), s.tenantID, email, displayName, domain.StatusActive, true, assigned)
	if err != nil {
		return domain.User{}, err
	}

	if err := s.users.CreateUser(ctx, user, hash); err != nil {
		return domain.User{}, err // KindConflict on duplicate email
	}
	if err := s.users.SetUserRoles(ctx, user.ID(), roleIDs); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

// resolveRoles validates every role id exists and returns the resolved roles. A
// missing role is KindInvalid (the request references a role that isn't there).
func (s *AdminService) resolveRoles(ctx context.Context, roleIDs []string) ([]domain.Role, error) {
	resolved := make([]domain.Role, 0, len(roleIDs))
	for _, id := range roleIDs {
		role, err := s.roles.FindByID(ctx, id)
		if err != nil {
			return nil, glerrors.New(glerrors.KindInvalid, "role does not exist: "+id)
		}
		resolved = append(resolved, role)
	}
	return resolved, nil
}

// GetUser returns a single principal (with roles resolved), or KindNotFound.
func (s *AdminService) GetUser(ctx context.Context, userID string) (domain.User, error) {
	return s.users.FindByID(ctx, userID)
}

// ListUsers returns every principal in the tenant with roles resolved.
func (s *AdminService) ListUsers(ctx context.Context) ([]domain.User, error) {
	return s.users.ListUsers(ctx)
}

// SetUserStatus activates or suspends an account. A suspended account cannot
// authenticate (domain.User.CanAuthenticate enforces this at login).
func (s *AdminService) SetUserStatus(ctx context.Context, userID string, status domain.Status) error {
	return s.users.SetStatus(ctx, userID, status)
}

// CreateRole creates a role from a name and a set of "resource:action"
// permission strings. Every permission must match the strict catalog grammar
// (KindInvalid otherwise); a duplicate name is KindConflict; a tenant-wide
// sensitive read on a non-owner role is KindForbidden.
func (s *AdminService) CreateRole(ctx context.Context, name string, permissions []string) (domain.Role, error) {
	if err := validatePermissions(permissions); err != nil {
		return domain.Role{}, err
	}
	role, err := domain.NewRole(s.newID(), name, permissions)
	if err != nil {
		return domain.Role{}, err // KindInvalid: blank name / malformed permission
	}
	// A freshly generated id is never the seeded owner id, so an owner-only
	// permission on a newly created role is always refused — the API cannot mint
	// a second super-role by choosing a name that collides with "owner".
	if err := guardOwnerOnly(role.ID(), permissions); err != nil {
		return domain.Role{}, err
	}
	if err := s.roles.CreateRole(ctx, role); err != nil {
		return domain.Role{}, err // KindConflict on duplicate name
	}
	return role, nil
}

// GetRole returns a single role (with its permissions), or KindNotFound.
func (s *AdminService) GetRole(ctx context.Context, roleID string) (domain.Role, error) {
	return s.roles.FindByID(ctx, roleID)
}

// ListRoles returns every role in the tenant.
func (s *AdminService) ListRoles(ctx context.Context) ([]domain.Role, error) {
	return s.roles.ListRoles(ctx)
}

// SetRolePermissions replaces a role's permission set. Every permission must
// match the strict catalog grammar (KindInvalid otherwise); granting a
// tenant-wide sensitive read to a role other than owner is KindForbidden.
//
// The target role is resolved only when the requested set actually contains a
// restricted permission — the common path keeps its single write, and when the
// role cannot be resolved the grant fails closed (KindNotFound) rather than
// being applied to an unverified name.
func (s *AdminService) SetRolePermissions(ctx context.Context, roleID string, permissions []string) error {
	if err := validatePermissions(permissions); err != nil {
		return err
	}
	if containsOwnerOnly(permissions) {
		role, err := s.roles.FindByID(ctx, roleID)
		if err != nil {
			return err // KindNotFound: cannot confirm the target is the owner role.
		}
		if err := guardOwnerOnly(role.ID(), permissions); err != nil {
			return err
		}
	}
	return s.roles.SetPermissions(ctx, roleID, permissions)
}

// validatePermissions rejects the whole set if any entry is not a canonical
// catalog permission string.
func validatePermissions(permissions []string) error {
	for _, p := range permissions {
		if !domain.IsValidPermission(p) {
			return glerrors.New(glerrors.KindInvalid, "invalid permission: "+p)
		}
	}
	return nil
}

// containsOwnerOnly reports whether the set requests any owner-only permission.
func containsOwnerOnly(permissions []string) bool {
	for _, p := range permissions {
		if domain.IsOwnerOnly(p) {
			return true
		}
	}
	return false
}

// guardOwnerOnly refuses to put a tenant-wide sensitive read on any role other
// than the seeded owner. Those reads are not scoped per object yet, so a
// non-owner holder would see every family's ledger / every group's attendance;
// the guard keeps the vulnerable configuration from being created through the
// admin API. The exemption is keyed on the owner role's fixed id, never its name
// — a name is admin-typed and case-collidable, so gating on it let a delegated
// admin mint an escalated "Owner" role. The seeded owner role (granted these by
// migration) is unaffected.
func guardOwnerOnly(roleID string, permissions []string) error {
	if domain.IsOwnerRoleID(roleID) {
		return nil
	}
	for _, p := range permissions {
		if domain.IsOwnerOnly(p) {
			return glerrors.New(glerrors.KindForbidden,
				p+" cannot be granted to a non-owner role yet (no per-object scoping)")
		}
	}
	return nil
}
