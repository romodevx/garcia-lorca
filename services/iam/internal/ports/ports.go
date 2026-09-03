package ports

import (
	"context"
	"time"

	"github.com/garcia-lorca/services/iam/internal/domain"
)

// This file declares the seams the iam use cases depend on. Interfaces live here
// (consumer side); the adapters (postgres, jwt, bcrypt) implement them. The
// domain imports none of this — dependencies point inward.
//
//	usecase ──uses──> ports (interfaces) <──implements── adapters

// UserRepository loads and persists principals. All methods operate through
// RLS-bound transactions (pkg/authctx) in the adapter; the tenant scoping is a
// parameter here so the use case stays explicit.
type UserRepository interface {
	// FindAuthByEmail returns the user and their stored password hash for login.
	// The must-change-password flag rides on the domain User itself
	// (user.MustChangePassword()) — a single source of truth. A missing user is a
	// KindNotFound error (do not leak whether the email exists — the use case
	// maps it to a generic auth failure).
	FindAuthByEmail(ctx context.Context, tenantID string, email domain.Email) (user domain.User, passwordHash string, err error)

	// FindByID returns a user (with roles resolved) or KindNotFound.
	FindByID(ctx context.Context, userID string) (domain.User, error)

	// FindAuthByID returns the user and their stored password hash by id
	// (ChangePassword verifies the current password).
	FindAuthByID(ctx context.Context, userID string) (user domain.User, passwordHash string, err error)

	// UpdatePassword sets a new hash and clears must_change_password.
	UpdatePassword(ctx context.Context, userID, newHash string) error

	// AddRole / RemoveRole change a user's role assignments.
	AddRole(ctx context.Context, userID, roleID string) error
	RemoveRole(ctx context.Context, userID, roleID string) error

	// CreateUser persists a new principal together with its initial password
	// hash. A duplicate email (unique per tenant) is a KindConflict error.
	CreateUser(ctx context.Context, user domain.User, passwordHash string) error

	// ListUsers returns every principal in the tenant with its roles resolved.
	ListUsers(ctx context.Context) ([]domain.User, error)

	// SetStatus changes a user's lifecycle status (activate/suspend).
	SetStatus(ctx context.Context, userID string, status domain.Status) error

	// SetUserRoles replaces the user's entire role set with roleIDs.
	SetUserRoles(ctx context.Context, userID string, roleIDs []string) error
}

// RoleRepository loads and persists roles (used to validate AssignRole targets,
// to resolve permissions when composing a user, and by the admin surface).
type RoleRepository interface {
	FindByID(ctx context.Context, roleID string) (domain.Role, error)

	// CreateRole persists a new role. A duplicate name (unique per tenant) is a
	// KindConflict error.
	CreateRole(ctx context.Context, role domain.Role) error

	// ListRoles returns every role in the tenant.
	ListRoles(ctx context.Context) ([]domain.Role, error)

	// SetPermissions replaces the role's permission set with permissions.
	SetPermissions(ctx context.Context, roleID string, permissions []string) error
}

// PasswordHasher hashes and verifies credentials. The implementation is a slow
// KDF (e.g. bcrypt/argon2); the domain never sees plaintext or hashes.
type PasswordHasher interface {
	Hash(plain string) (string, error)
	// Verify returns nil on match, KindUnauthorized on mismatch.
	Verify(hash, plain string) error
}

// TokenIssuer mints signed tokens. iam is the ONLY issuer. Access tokens carry
// the full claims; refresh tokens carry just enough to re-mint (the user id).
type TokenIssuer interface {
	IssueAccessToken(ctx context.Context, claims domain.Claims) (string, error)
	//
	// The kind decides how long the token may sit unused: a staff session
	// closes after fifteen idle minutes, a family one lasts a day. It is stamped
	// into the token so a rotation cannot promote itself to the longer life.
	IssueRefreshToken(ctx context.Context, userID string, kind domain.SessionKind) (string, error)
}

// TokenValidator verifies signed tokens. An invalid signature or an expired
// token is a KindUnauthorized error.
type TokenValidator interface {
	ValidateAccessToken(ctx context.Context, token string) (domain.Claims, error)
	// ParseRefreshToken returns the subject (user id) and issued-at instant of a
	// valid refresh token. The issued-at supports revoking tokens minted before
	// the user's last password change.
	ParseRefreshToken(ctx context.Context, token string) (userID string, issuedAt time.Time, kind domain.SessionKind, err error)
}

// VerifiedIdentity is what an external identity provider proves about a caller:
// that whoever holds the token controls this email address on that provider.
//
// It carries no roles and no permissions ON PURPOSE. Those live in our own
// database, and the distinction is the whole security model: the provider says
// WHO you are, this service decides WHAT you may do. A provider that started
// claiming permissions would be a provider that could grant itself any.
type VerifiedIdentity struct {
	// Email is the verified address. It is the join key back to `users.email`,
	// which is unique per tenant.
	Email string
	// Subject is the provider's own stable id for the account. Not used to
	// authorize anything; kept so a future audit trail can tell two accounts
	// apart after somebody changes their email address.
	Subject string
}

// IDTokenVerifier checks an ID token minted by an external identity provider
// (Identity Platform) and reports the identity it proves. A bad signature, a
// wrong issuer or audience, and an expired token are all KindUnauthorized.
//
// Consumer-defined, so the use case is testable without the network and the
// provider stays replaceable: nothing above this line knows it is Google.
type IDTokenVerifier interface {
	Verify(ctx context.Context, idToken string) (VerifiedIdentity, error)
}

// IdentityProvisioner creates accounts at the external identity provider.
//
// Deliberately NO password parameter. An account is created without one and its
// owner sets it through a link they receive, which is what proves they control
// the mailbox — so the address ends up verified because they verified it, not
// because we asserted it. Nobody else ever knows their password, and this
// service stores no second copy of a credential to drift or leak.
//
// A duplicate address is a KindConflict error.
type IdentityProvisioner interface {
	CreateAccount(ctx context.Context, email, displayName string) error
}
