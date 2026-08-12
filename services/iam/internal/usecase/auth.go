package usecase

import (
	"context"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/ports"
)

// minPasswordLen is the smallest acceptable new password (defense-in-depth; the
// gateway/UI also validate).
const minPasswordLen = 8

// timingEqualizerHash is a fixed, valid bcrypt hash (cost 10) used ONLY to make
// the unknown-email login path do the same ~constant password-verification work
// as the known-email path. Without it, bcrypt runs only when the email exists,
// and the response-time delta leaks which emails are registered even though both
// paths return the identical generic error. It is not a credential — it matches
// no real password.
const timingEqualizerHash = "$2a$10$y5uQah8/CrIHsNKRCJzWJOHR/4SQOcCzbJGiBfN6wACA0k3FS9I0i"

// AuthService orchestrates authentication and authorization. It depends only on
// the ports (repositories, hasher, token issuer/validator) and the domain — the
// composition root injects concrete adapters (postgres, bcrypt, jwt).
//
// Login flow:
//
//	email ─> FindAuthByEmail ─> Verify(password) ─> CanAuthenticate ─> NewAccessClaims ─> Issue{Access,Refresh}
//	          │not found          │mismatch           │suspended
//	          └── Unauthorized ───┘                   └── Forbidden
//
// Enumeration defense: an unknown email and a wrong password return the SAME
// generic Unauthorized error, so callers cannot discover which emails exist.
type AuthService struct {
	users     ports.UserRepository
	roles     ports.RoleRepository
	hasher    ports.PasswordHasher
	issuer    ports.TokenIssuer
	validator ports.TokenValidator
	tenantID  string
	accessTTL time.Duration
	now       func() time.Time
}

// NewAuthService wires the use case. `tenantID` is the single-tenant scope for
// M01 (multi-tenant resolution is future work); `now` is injectable for
// deterministic tests.
func NewAuthService(
	users ports.UserRepository,
	roles ports.RoleRepository,
	hasher ports.PasswordHasher,
	issuer ports.TokenIssuer,
	validator ports.TokenValidator,
	tenantID string,
	accessTTL time.Duration,
	now func() time.Time,
) *AuthService {
	return &AuthService{
		users:     users,
		roles:     roles,
		hasher:    hasher,
		issuer:    issuer,
		validator: validator,
		tenantID:  tenantID,
		accessTTL: accessTTL,
		now:       now,
	}
}

// AuthResult is the outcome of a successful Authenticate/RefreshToken.
type AuthResult struct {
	AccessToken        string
	RefreshToken       string
	ExpiresIn          int64 // access token TTL in seconds
	User               domain.User
	MustChangePassword bool
}

// Authenticate verifies credentials and issues an access + refresh token pair.
func (s *AuthService) Authenticate(ctx context.Context, rawEmail, password string) (AuthResult, error) {
	email, err := domain.NewEmail(rawEmail)
	if err != nil {
		return AuthResult{}, err // KindInvalid: malformed input, not an auth attempt.
	}

	user, hash, err := s.users.FindAuthByEmail(ctx, s.tenantID, email)
	if err != nil {
		// Unknown email → generic auth failure (do not leak existence). Run a
		// throwaway verification against a fixed hash first so this path costs the
		// same as a real bcrypt compare — otherwise the timing delta reveals which
		// emails exist. The result is intentionally discarded.
		_ = s.hasher.Verify(timingEqualizerHash, password)
		return AuthResult{}, glerrors.New(glerrors.KindUnauthorized, "invalid credentials")
	}
	if err := s.hasher.Verify(hash, password); err != nil {
		return AuthResult{}, glerrors.New(glerrors.KindUnauthorized, "invalid credentials")
	}
	if !user.CanAuthenticate() {
		return AuthResult{}, glerrors.New(glerrors.KindForbidden, "account is not active")
	}

	res, err := s.issueFor(ctx, user)
	if err != nil {
		return AuthResult{}, err
	}
	res.MustChangePassword = user.MustChangePassword()
	return res, nil
}

// RefreshToken exchanges a valid refresh token for a fresh, rotated pair. A
// token minted before the user's last password change is rejected, so rotating
// the password revokes every refresh token issued before it (a stolen token
// stops working the moment the password is changed).
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (AuthResult, error) {
	userID, issuedAt, err := s.validator.ParseRefreshToken(ctx, refreshToken)
	if err != nil {
		return AuthResult{}, glerrors.New(glerrors.KindUnauthorized, "invalid refresh token")
	}
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return AuthResult{}, glerrors.New(glerrors.KindUnauthorized, "invalid refresh token")
	}
	// Compare at whole-second granularity: a JWT's `iat` is Unix seconds (floored),
	// while password_changed_at carries sub-second precision. Truncating the floor
	// to the second means a token issued in the SAME second as the change is NOT
	// revoked — critical, because the re-issued session (login right after the
	// change) always lands in that second and must survive. Tokens from any earlier
	// second are still revoked. The at-most-1s window is a negligible trade.
	if issuedAt.Before(user.PasswordChangedAt().Truncate(time.Second)) {
		// Revoked by a later password change — same generic error as any other
		// invalid refresh token (do not reveal why).
		return AuthResult{}, glerrors.New(glerrors.KindUnauthorized, "invalid refresh token")
	}
	if !user.CanAuthenticate() {
		return AuthResult{}, glerrors.New(glerrors.KindForbidden, "account is not active")
	}
	return s.issueFor(ctx, user)
}

// ChangePassword sets a new password for the principal identified by accessToken
// (the subject is derived from the verified token — never a caller-supplied id),
// after verifying the current password. It clears must_change_password (the
// forced-rotation flag). An invalid token or a wrong current password is
// Unauthorized; a too-short new password is Invalid.
func (s *AuthService) ChangePassword(ctx context.Context, accessToken, currentPassword, newPassword string) error {
	if len(newPassword) < minPasswordLen {
		return glerrors.New(glerrors.KindInvalid, "new password is too short")
	}
	claims, err := s.validator.ValidateAccessToken(ctx, accessToken)
	if err != nil {
		return glerrors.New(glerrors.KindUnauthorized, "invalid access token")
	}
	_, hash, err := s.users.FindAuthByID(ctx, claims.Subject)
	if err != nil {
		return glerrors.New(glerrors.KindUnauthorized, "invalid credentials")
	}
	if err := s.hasher.Verify(hash, currentPassword); err != nil {
		return glerrors.New(glerrors.KindUnauthorized, "invalid credentials")
	}
	newHash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return err
	}
	return s.users.UpdatePassword(ctx, claims.Subject, newHash)
}

// ValidateToken verifies an access token and returns its claims (principal +
// effective permissions). An invalid/expired token is a generic Unauthorized.
func (s *AuthService) ValidateToken(ctx context.Context, accessToken string) (domain.Claims, error) {
	claims, err := s.validator.ValidateAccessToken(ctx, accessToken)
	if err != nil {
		return domain.Claims{}, glerrors.New(glerrors.KindUnauthorized, "invalid access token")
	}
	return claims, nil
}

// CheckPermission validates an access token and reports whether its principal
// holds the given "resource:action" permission.
func (s *AuthService) CheckPermission(ctx context.Context, accessToken, permission string) (bool, error) {
	claims, err := s.validator.ValidateAccessToken(ctx, accessToken)
	if err != nil {
		return false, glerrors.New(glerrors.KindUnauthorized, "invalid access token")
	}
	return claims.Allows(permission), nil
}

// AssignRole grants a role to a user. The role must exist (KindNotFound
// otherwise). Who may call this is enforced at the gateway/RPC boundary.
func (s *AuthService) AssignRole(ctx context.Context, userID, roleID string) error {
	if _, err := s.roles.FindByID(ctx, roleID); err != nil {
		return err // KindNotFound propagates.
	}
	return s.users.AddRole(ctx, userID, roleID)
}

// RevokeRole removes a role from a user.
func (s *AuthService) RevokeRole(ctx context.Context, userID, roleID string) error {
	return s.users.RemoveRole(ctx, userID, roleID)
}

// issueFor mints a fresh access + refresh pair for an authenticated user.
func (s *AuthService) issueFor(ctx context.Context, user domain.User) (AuthResult, error) {
	claims := domain.NewAccessClaims(user, s.now(), s.accessTTL)
	access, err := s.issuer.IssueAccessToken(ctx, claims)
	if err != nil {
		return AuthResult{}, err
	}
	refresh, err := s.issuer.IssueRefreshToken(ctx, user.ID())
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(s.accessTTL.Seconds()),
		User:         user,
	}, nil
}
