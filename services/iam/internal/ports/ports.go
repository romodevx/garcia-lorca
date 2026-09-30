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

	// TouchPasswordChangedAt stamps the rotation instant WITHOUT touching the
	// stored hash.
	//
	// It exists for the case where the credential lives at an external provider:
	// there is no local hash to write, but RefreshToken still rejects tokens
	// minted before this instant (ADR 0005), and that is how a password change
	// revokes older sessions. Without it, a reset done at the provider left
	// every previously stolen refresh token working.
	TouchPasswordChangedAt(ctx context.Context, userID string) error

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
	// and a family one both last a day, and may diverge again. It is stamped
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

// ProviderPasswordChecker re-checks a password at the external identity
// provider, for the re-authentication a destructive action demands (ADR 0021).
// A wrong password, an unknown or disabled account and a throttled attempt are
// all KindUnauthorized; a provider that cannot be asked (network, bad key) is
// KindInternal — never a silent pass.
type ProviderPasswordChecker interface {
	CheckPassword(ctx context.Context, email, password string) error
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

	// SetPassword writes the credential its owner just chose, and marks the
	// address verified because following the link is what proved they read mail
	// there.
	//
	// This replaced SendInvitation, which asked the provider to MAIL the link.
	// The provider accepted those requests with HTTP 200 and delivered nothing —
	// measured, repeatedly, including to an address that did not exist — so the
	// call reported success for a message nobody ever got. Delivery is ours now;
	// the provider is left doing the one thing it does well, holding the
	// credential.
	SetPassword(ctx context.Context, email, password string) error

	// SendPasswordSetupEmail asks the PROVIDER to mail its own set-a-password
	// link. TEMPORARY, and the owner chose it knowingly.
	//
	// What it costs, stated plainly: the link is Google's, so it expires in one
	// hour instead of twenty-four, it lands on Google's page in English instead
	// of ours, and it is sent from a shared address that Gmail files as spam.
	// None of that is fixable from here.
	//
	// What it buys: mail that goes out today, with no SMTP credential from
	// anybody. The seam it leaves — the password gets set on a page we cannot
	// see — is closed by activating on first provider sign-in
	// (AuthService.AuthenticateWithIDToken), which is why that exists.
	//
	// It is the LAST fallback: a configured mailer always wins.
	SendPasswordSetupEmail(ctx context.Context, email string) error
}

// InvitationRepository stores the single-use tokens that let somebody set a
// password without being logged in.
//
// Every method is tenant-bound in the adapter (pkg/authctx), and NONE of them
// takes a tenant parameter — deliberately. The redeem endpoints are anonymous,
// so if the tenant travelled in the request it would be caller-controlled, and
// the RLS story would quietly become "whatever the URL said". The tenant is
// fixed at construction instead, where a caller cannot reach it.
type InvitationRepository interface {
	// Issue stores a new invitation, REPLACING any live one the user still has
	// for the same purpose. Replacing rather than inserting is what makes
	// "resend" kill the previous link: two live invitations would mean an old
	// mail still works after the director sent a new one.
	Issue(ctx context.Context, inv domain.Invitation) error

	// Consume spends the token and returns what it was, atomically: it must
	// match the hash, be unconsumed, and be unexpired in ONE statement. Two
	// simultaneous redemptions therefore leave exactly one winner, and the
	// loser is indistinguishable from a token that never existed.
	//
	// Every failure — unknown, expired, already spent — is KindNotFound with
	// the same message. Telling them apart would be an oracle for guessing.
	Consume(ctx context.Context, tokenHash string, now time.Time) (domain.Invitation, error)

	// MarkSent records the outcome of trying to deliver the invitation.
	// sendErr is empty on success. This is what the admin screen reads to show
	// "Sin enviar" instead of leaving the director to guess.
	MarkSent(ctx context.Context, invitationID string, sentAt time.Time, sendErr string) error

	// Release hands a spent token back, undoing a Consume whose follow-up work
	// failed.
	//
	// Consuming before writing the credential is what keeps two simultaneous
	// redemptions to one winner, so that order stays — but it means a provider
	// answering 503 would otherwise burn somebody's only way in, and the resend
	// guard would refuse them a new one. This is the compensating half.
	Release(ctx context.Context, invitationID string) error

	// Revoke kills every live invitation a user still holds.
	//
	// Called when somebody activates by signing in rather than by redeeming our
	// link — which is what happens while the identity provider carries the mail.
	// Without it, a link handed out hours earlier would keep being able to set
	// the password of an account that is now in daily use.
	Revoke(ctx context.Context, userID string) error

	// ListLive returns every unconsumed, unexpired invitation in the tenant,
	// keyed by user id. ONE query, not one per user: the admin list renders a
	// state pill per row and a naive implementation is an N+1 waiting to happen.
	ListLive(ctx context.Context, now time.Time) (map[string]domain.Invitation, error)
}

// Mailer delivers a message. One method, defined here because iam is what
// needs it, so the transport stays a detail that can be swapped without the use
// case noticing.
//
// NIL IS A VALID STATE, and it is the state the school starts in: with no mail
// configured, inviting still works and the use case reports that the message did
// not go out, so the director can hand the link over another way. That beats
// refusing to create accounts until somebody finds an SMTP password.
type Mailer interface {
	// Send delivers one message. Implementations must enforce their own
	// deadline: this call sits in a request path and net/smtp will otherwise
	// hang on a half-open connection until the OS gives up.
	Send(ctx context.Context, to, subject, htmlBody, textBody string) error
}
