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
	users    ports.UserRepository
	roles    ports.RoleRepository
	hasher   ports.PasswordHasher
	tenantID string
	newID    func() string
}

// NewAdminService wires the admin use case. `tenantID` is the single-tenant scope
// for this phase; `newID` mints identifiers for new users and roles (injectable
// for deterministic tests).
func NewAdminService(
	users ports.UserRepository,
	roles ports.RoleRepository,
	hasher ports.PasswordHasher,
	tenantID string,
	newID func() string,
) *AdminService {
	return &AdminService{
		users:    users,
		roles:    roles,
		hasher:   hasher,
		tenantID: tenantID,
		newID:    newID,
	}
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
// (KindInvalid otherwise); a duplicate name is KindConflict.
func (s *AdminService) CreateRole(ctx context.Context, name string, permissions []string) (domain.Role, error) {
	if err := validatePermissions(permissions); err != nil {
		return domain.Role{}, err
	}
	role, err := domain.NewRole(s.newID(), name, permissions)
	if err != nil {
		return domain.Role{}, err // KindInvalid: blank name / malformed permission
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
// match the strict catalog grammar (KindInvalid otherwise).
func (s *AdminService) SetRolePermissions(ctx context.Context, roleID string, permissions []string) error {
	if err := validatePermissions(permissions); err != nil {
		return err
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
