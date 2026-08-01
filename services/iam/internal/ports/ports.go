package ports

import (
	"context"

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
}

// RoleRepository loads roles (used to validate AssignRole targets and to resolve
// permissions when composing a user).
type RoleRepository interface {
	FindByID(ctx context.Context, roleID string) (domain.Role, error)
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
	IssueRefreshToken(ctx context.Context, userID string) (string, error)
}

// TokenValidator verifies signed tokens. An invalid signature or an expired
// token is a KindUnauthorized error.
type TokenValidator interface {
	ValidateAccessToken(ctx context.Context, token string) (domain.Claims, error)
	// ParseRefreshToken returns the subject (user id) of a valid refresh token.
	ParseRefreshToken(ctx context.Context, token string) (userID string, err error)
}
