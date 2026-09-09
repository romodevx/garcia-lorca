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
	// idTokens verifies ID tokens from an external identity provider. NIL is a
	// valid state: environments with no provider configured simply refuse that
	// sign-in path (see AuthenticateWithIDToken), rather than the service
	// refusing to boot. Password sign-in never touches it.
	idTokens ports.IDTokenVerifier
	// invitations is used for ONE thing here: killing a user's outstanding links
	// the moment they activate. NIL is valid — an environment without the store
	// simply skips that cleanup.
	invitations ports.InvitationRepository
	tenantID    string
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
	idTokens ports.IDTokenVerifier,
	invitations ports.InvitationRepository,
	tenantID string,
	accessTTL time.Duration,
	now func() time.Time,
) *AuthService {
	return &AuthService{
		users:       users,
		roles:       roles,
		hasher:      hasher,
		issuer:      issuer,
		validator:   validator,
		idTokens:    idTokens,
		invitations: invitations,
		tenantID:    tenantID,
		accessTTL:   accessTTL,
		now:         now,
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

// Authenticate verifies credentials and issues a STAFF session — the school's
// panel, which closes after fifteen idle minutes.
//
// It is the safe default: a caller that does not say which surface it is gets
// the shorter life, because guessing wrong that way costs somebody a login and
// guessing wrong the other way leaves an office session open all afternoon.
func (s *AuthService) Authenticate(ctx context.Context, rawEmail, password string) (AuthResult, error) {
	return s.AuthenticateAs(ctx, rawEmail, password, domain.SessionStaff)
}

// AuthenticateAs verifies credentials and issues a session of the given kind.
// The KIND comes from the surface the request arrived on — the panel or the
// family app — never from anything the caller can set.
func (s *AuthService) AuthenticateAs(ctx context.Context, rawEmail, password string, kind domain.SessionKind) (AuthResult, error) {
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

	res, err := s.issueFor(ctx, user, kind)
	if err != nil {
		return AuthResult{}, err
	}
	res.MustChangePassword = user.MustChangePassword()
	return res, nil
}

// AuthenticateWithIDToken signs a user in with an ID token from the external
// identity provider and issues OUR OWN session — the same pair a password login
// produces.
//
// That is the point of doing it here rather than at the edge. The provider owns
// password storage, recovery and MFA; this service keeps owning the session, so
// the gateway, its HttpOnly cookies, the mobile Bearer flow, our TTLs and the
// "a password change revokes older refresh tokens" rule all continue to work
// untouched.
//
// Verifying at the provider is NOT authorization. It establishes an email; the
// user row behind that email is what carries the roles and permissions, and a
// caller with no row here gets the same generic refusal as a bad token — never
// a hint that the address is unknown.
func (s *AuthService) AuthenticateWithIDToken(ctx context.Context, idToken string, kind domain.SessionKind) (AuthResult, error) {
	if s.idTokens == nil {
		// Fail closed. An environment with no provider configured refuses this
		// path outright; it must never fall through to accepting an unchecked
		// token.
		return AuthResult{}, glerrors.New(glerrors.KindUnauthorized, "invalid credentials")
	}

	identity, err := s.idTokens.Verify(ctx, idToken)
	if err != nil {
		return AuthResult{}, glerrors.New(glerrors.KindUnauthorized, "invalid credentials")
	}

	email, err := domain.NewEmail(identity.Email)
	if err != nil {
		return AuthResult{}, glerrors.New(glerrors.KindUnauthorized, "invalid credentials")
	}

	user, _, err := s.users.FindAuthByEmail(ctx, s.tenantID, email)
	if err != nil {
		// No user row for a verified address. Same generic refusal as a bad
		// token: the provider already knows the address exists, but we do not
		// confirm whether it exists HERE. No timing equalizer is needed — unlike
		// the password path, nothing on this branch runs a hash.
		return AuthResult{}, glerrors.New(glerrors.KindUnauthorized, "invalid credentials")
	}
	// ACTIVATION HAPPENS HERE, and it has to, because of where the password was
	// set.
	//
	// While the identity provider carries the invitation mail, its link lands on
	// ITS page: the person chooses a password somewhere we cannot see, and
	// nothing ever tells us they did. Without this they would stay `invited`
	// forever — unable to log in, with the resend button stuck on their row.
	//
	// So activation stops meaning "you redeemed our token" and starts meaning
	// "you proved you can sign in", which is the truer of the two and covers
	// both paths. The proof is real: the provider verified an address and a
	// password it holds, which is exactly what our own link was ever asking for.
	//
	// ONLY from `invited`, and only on THIS path. A suspended account stays
	// suspended — that is somebody's decision, and still holding the password
	// says nothing about it. And the local-password path never activates: the
	// hash it checks could not have been set by anyone who was not already
	// through the door, so it proves nothing new.
	if user.Status() == domain.StatusInvited {
		if err := s.users.SetStatus(ctx, user.ID(), domain.StatusActive); err != nil {
			return AuthResult{}, err
		}
		// Whatever links they were still holding die now. One handed out hours
		// ago must not keep being able to set the password of an account that
		// is from this moment in daily use.
		if s.invitations != nil {
			_ = s.invitations.Revoke(ctx, user.ID())
		}
	} else if !user.CanAuthenticate() {
		return AuthResult{}, glerrors.New(glerrors.KindForbidden, "account is not active")
	}

	// NOT propagating MustChangePassword, deliberately.
	//
	// The flag is a leftover from the password era, and carrying it here trapped
	// a real user: signing in through the provider answered true, the app routed
	// her to the change-password screen, and ChangePassword refused — correctly,
	// because the provider owns credentials now. She could sign in and never get
	// in. Found by running it; no test with a double would have shown it, because
	// the trap only exists when both halves are true at once.
	//
	// Where the provider holds the credential there is no password of ours to
	// change, so false is the honest answer. Password lifecycle belongs to the
	// provider's own reset flow.
	//
	// It comes off the TOKEN too, and that is the half with teeth — see issueFor,
	// which does it for every token this service mints, refreshes included.
	return s.issueFor(ctx, user, kind)
}

// RefreshToken exchanges a valid refresh token for a fresh, rotated pair. A
// token minted before the user's last password change is rejected, so rotating
// the password revokes every refresh token issued before it (a stolen token
// stops working the moment the password is changed).
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (AuthResult, error) {
	userID, issuedAt, kind, err := s.validator.ParseRefreshToken(ctx, refreshToken)
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
	// The rotated pair keeps the session's kind, so a family session stays long
	// and a staff one stays short. It cannot promote itself by rotating.
	return s.issueFor(ctx, user, kind)
}

// ChangePassword sets a new password for the principal identified by accessToken
// (the subject is derived from the verified token — never a caller-supplied id),
// after verifying the current password. It clears must_change_password (the
// forced-rotation flag). An invalid token or a wrong current password is
// Unauthorized; a too-short new password is Invalid.
func (s *AuthService) ChangePassword(ctx context.Context, accessToken, currentPassword, newPassword string) error {
	// FIRST, before validating anything about the new password.
	//
	// Where an identity provider is configured, it owns credentials: it is what
	// actually checks a password at sign-in. Writing a new hash to Postgres here
	// would report success, clear must_change_password, and change NOTHING about
	// what lets somebody in — the old password would keep working.
	//
	// That is worse than offering no rotation at all. Somebody whose password
	// was compromised rotates it, sees the confirmation, and closes the
	// incident, while the attacker's copy still opens the door. Refusing is
	// honest; a silent no-op is a lie the user acts on.
	//
	// Rotation lives at the provider once it is in play: the invitation link is
	// also the reset link (see AdminService.InviteUser).
	// KindInvalid, not KindInternal. The old kind mapped to HTTP 500 at the
	// gateway for a precondition the caller can do nothing about: it paged as a
	// server fault, and the screen showed a generic failure instead of the one
	// sentence that helps. The way to change a provider-held password is the
	// reset link, which this system now has.
	if s.idTokens != nil {
		return glerrors.New(glerrors.KindInvalid,
			"la contraseña la guarda el proveedor; usa \"olvidé mi contraseña\" para cambiarla")
	}
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
func (s *AuthService) issueFor(ctx context.Context, user domain.User, kind domain.SessionKind) (AuthResult, error) {
	// WHERE THE PROVIDER OWNS CREDENTIALS, NO TOKEN CARRIES THE ROTATION FLAG.
	//
	// It lives here rather than at each call site because tokens are minted from
	// three places — password login, provider login, and REFRESH — and clearing
	// it at login alone left the worst of the three unfixed: a refresh re-reads
	// the row and re-stamps the claim, so a session that had been working went
	// inert fifteen minutes in, with no error and nothing anybody did.
	//
	// The flag means "this account holds a temporary password somebody else
	// chose, and the only allowed action is replacing it". With a provider
	// configured that is not true of anything: ChangePassword already refuses,
	// so an account carrying the flag would be told to rotate a password it
	// cannot rotate, while the gateway's rotation gate refuses every route in the
	// meantime. Found in the deployed demo (2026-09-07): login fine, panel empty,
	// no error anywhere, because the web asks /me for its permissions and
	// swallows the 403 on purpose.
	//
	// The ROW is untouched, so switching the provider off restores the old
	// behaviour exactly, pending rotations included.
	if s.idTokens != nil {
		user = user.WithoutPendingRotation()
	}
	claims := domain.NewAccessClaims(user, s.now(), s.accessTTL)
	access, err := s.issuer.IssueAccessToken(ctx, claims)
	if err != nil {
		return AuthResult{}, err
	}
	refresh, err := s.issuer.IssueRefreshToken(ctx, user.ID(), kind)
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
