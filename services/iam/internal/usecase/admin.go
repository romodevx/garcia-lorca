package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"

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
	// invitations stores the single-use tokens that let somebody set a password
	// without being logged in.
	invitations ports.InvitationRepository
	// mailer delivers those tokens. NIL is a valid, expected state: the school
	// has no SMTP credential yet, and refusing to create accounts until one
	// appears would be worse than handing the director a link to pass on.
	mailer ports.Mailer
	// activateURL is where an invitation link points — our own page, not the
	// provider's English one.
	activateURL string
	tenantID    string
	newID       func() string
	// now is injected so every expiry assertion is deterministic. A bare
	// time.Now() here would make "expires in 24 hours" untestable.
	now func() time.Time
	// sleep pads the password-reset response to a constant cost. Injected for
	// the same reason as the clock: a test must be able to read what was padded
	// instead of waiting for it.
	sleep func(time.Duration)
}

// AdminOption tunes the service. There is one today and it exists for tests.
type AdminOption func(*AdminService)

// WithSleeper replaces the real timer used to pad the password-reset response.
func WithSleeper(f func(time.Duration)) AdminOption {
	return func(s *AdminService) { s.sleep = f }
}

// NewAdminService wires the admin use case. `tenantID` is the single-tenant scope
// for this phase; `newID` mints identifiers for new users and roles (injectable
// for deterministic tests).
func NewAdminService(
	users ports.UserRepository,
	roles ports.RoleRepository,
	hasher ports.PasswordHasher,
	provisioner ports.IdentityProvisioner,
	invitations ports.InvitationRepository,
	mailer ports.Mailer,
	activateURL string,
	tenantID string,
	newID func() string,
	now func() time.Time,
	opts ...AdminOption,
) *AdminService {
	if now == nil {
		now = time.Now
	}
	svc := &AdminService{
		users:       users,
		roles:       roles,
		hasher:      hasher,
		provisioner: provisioner,
		invitations: invitations,
		mailer:      mailer,
		activateURL: activateURL,
		tenantID:    tenantID,
		newID:       newID,
		now:         now,
		sleep:       time.Sleep,
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// resetFloor is what every password-reset request costs, whatever the answer.
//
// The send is back INSIDE the request, and it had to come back: it was detached
// into a goroutine to close the timing oracle, and on Cloud Run with cpu_idle
// and scale-to-zero — how iam runs in dev — the CPU is throttled the instant the
// response is written and the instance is reclaimed. The mail never left and the
// failure was never recorded. A silent non-delivery is the exact thing this
// change exists to end, so "works" wins over "elegant".
//
// Which puts the oracle back, and this closes it the blunt way: an address with
// an account costs a mail round trip, one without costs a lookup, so both are
// padded to the same floor. It is uglier than detaching and it is honest —
// nobody is waiting on this endpoint, and a public form that answers faster for
// addresses that exist is a directory anyone can read.
const resetFloor = 900 * time.Millisecond

// InviteUser creates a principal WITHOUT anybody inventing a password for them.
//
// The account is created at the provider with no password. Its owner sets the
// first one through a password-reset link, and using that link is what proves
// they control the mailbox: the address ends up verified because they verified
// it, not because we asserted it. A mistyped address then fails loudly and
// early — the link never arrives and the account is never usable — instead of
// sitting there as a pre-verified account its real owner could later claim.
//
// The link IS sent, from here, by the provider itself. That send doubles as the
// "resend invitation" button and as the administrator password reset iam never
// had: for an account with no password the link chooses the first one, and for
// one that has a password it resets it. Same request either way.
//
// Contrast CreateUser below, which takes a temporary password the director
// types and hands over: somebody else knows your password, this service stores
// a credential nobody chose, and the account works before its owner has proved
// anything about the address.
//
// ORDER IS LOAD-BEARING: the provider first, the row second. A row written
// before a refused provider call would leave a user who exists here, holds
// permissions, and can never sign in — invisible until somebody tries.
func (s *AdminService) InviteUser(ctx context.Context, rawEmail, displayName string, roleIDs []string) (Invitation, error) {
	email, err := domain.NewEmail(rawEmail)
	if err != nil {
		return Invitation{}, err // KindInvalid, before either side is touched.
	}
	assigned, err := s.resolveManualRoles(ctx, roleIDs)
	if err != nil {
		return Invitation{}, err
	}
	return s.invite(ctx, email, displayName, roleIDs, assigned)
}

// InviteGuardian invites a family contact into the guardian role — the ONLY
// road to it (ISSUE-CC-115). The role is chosen here, never by the caller: the
// gateway reaches this from "Dar acceso a la app", which links the account to
// the pupil right after, and that link is what gives the role any meaning.
//
// A deployment without the seeded role (migrations/iam/0010) is OUR fault, not
// the caller's: inviting into no role would create a login that sees nothing.
func (s *AdminService) InviteGuardian(ctx context.Context, rawEmail, displayName string) (Invitation, error) {
	email, err := domain.NewEmail(rawEmail)
	if err != nil {
		return Invitation{}, err
	}
	guardian, err := s.roles.FindByID(ctx, domain.GuardianRoleID)
	if err != nil {
		return Invitation{}, glerrors.Wrap(glerrors.KindInternal, "the guardian role is missing from this deployment", err)
	}
	return s.invite(ctx, email, displayName, []string{guardian.ID()}, []domain.Role{guardian})
}

// invite is InviteUser after the roles are settled: restore a deleted address,
// refuse a live one, otherwise create the account at the provider and here.
func (s *AdminService) invite(ctx context.Context, email domain.Email, displayName string, roleIDs []string, assigned []domain.Role) (Invitation, error) {

	// An address already on file is either a deleted account — which comes
	// back, history and all (owner answer 3) — or a live one, which is the
	// duplicate it always was. Asked before the provider, so a duplicate no
	// longer costs a round trip to find out.
	existing, _, err := s.users.FindAuthByEmail(ctx, s.tenantID, email)
	switch {
	case err == nil && existing.IsDeleted():
		return s.reactivate(ctx, existing, displayName, roleIDs, assigned)
	case err == nil:
		return Invitation{}, glerrors.New(glerrors.KindConflict, "a user with this email already exists")
	case glerrors.KindOf(err) != glerrors.KindNotFound:
		return Invitation{}, err
	}

	// A provider is OPTIONAL now, and this used to be a hard refusal.
	//
	// It was right to refuse when the provider was also the thing that mailed
	// the link: without one there was no way to reach the person. Now we own
	// both the token and the delivery, so "this environment has no Identity
	// Platform project" says nothing about whether somebody can be given an
	// account. Where there is a provider it holds the credential; where there
	// is not, the users row is the account and accepting writes the hash here.
	//
	// ORDER IS LOAD-BEARING: the provider first, the row second. A row written
	// before a refused provider call would leave a user who exists here, holds
	// permissions, and can never sign in — invisible until somebody tries.
	if s.provisioner != nil {
		if err := s.provisioner.CreateAccount(ctx, email.String(), displayName); err != nil {
			return Invitation{}, err
		}
	}

	// StatusInvited, not StatusActive. This is the line the whole feature turns
	// on: the account exists and holds roles, and it cannot be used until its
	// owner opens the link. Before it, "created" and "usable" were the same
	// word, so a mistyped address became a working account nobody could reach
	// and the resend button had no state to key on.
	user, err := domain.NewUser(s.newID(), s.tenantID, email, displayName, domain.StatusInvited, false, assigned)
	if err != nil {
		return Invitation{}, err
	}

	// No hash. The provider owns credentials; a second copy here would be one
	// more thing to drift, leak, or be reverted by a re-import.
	if err := s.users.CreateUser(ctx, user, ""); err != nil {
		return Invitation{}, err // KindConflict on duplicate email
	}
	if err := s.users.SetUserRoles(ctx, user.ID(), roleIDs); err != nil {
		return Invitation{}, err
	}

	out, err := s.issueAndSend(ctx, user, domain.PurposeInvitation, domain.InvitationLifetime)
	if err != nil {
		return Invitation{}, err
	}
	out.User = user
	return out, nil
}

// reactivate brings a deleted account back as `invited` and sends it a fresh
// invitation — the SAME row, so the payments it registered and the grades it
// captured stay attached to the person (owner answer 3).
//
// It is exactly as strong as inviting a new address, and no stronger: the
// roles are the ones this invitation asks for (the gateway only lets a caller
// assign what it holds), every old link dies first, and at the provider the old
// password is replaced by one nobody knows — so the account opens only from
// the new link, by whoever reads that mailbox.
func (s *AdminService) reactivate(ctx context.Context, user domain.User, displayName string, roleIDs []string, assigned []domain.Role) (Invitation, error) {
	if s.invitations != nil {
		if err := s.invitations.Revoke(ctx, user.ID()); err != nil {
			return Invitation{}, err
		}
	}
	if err := s.reenableAtProvider(ctx, user.Email().String(), displayName); err != nil {
		return Invitation{}, err
	}
	if err := s.users.Restore(ctx, user.ID(), displayName, roleIDs); err != nil {
		return Invitation{}, err
	}
	// What was just written, built in memory like InviteUser does: same id,
	// same address, the new name and roles, waiting on its link.
	restored, err := domain.NewUser(user.ID(), user.TenantID(), user.Email(), displayName, domain.StatusInvited, false, assigned)
	if err != nil {
		return Invitation{}, err
	}
	out, err := s.issueAndSend(ctx, restored, domain.PurposeInvitation, domain.InvitationLifetime)
	if err != nil {
		return Invitation{}, err
	}
	out.User = restored
	return out, nil
}

// reenableAtProvider switches the provider account back on with a fresh,
// unknown credential, creating the account when the provider has none. No
// provider configured: nothing to do.
func (s *AdminService) reenableAtProvider(ctx context.Context, email, displayName string) error {
	if s.provisioner == nil {
		return nil
	}
	err := s.provisioner.Reenable(ctx, email)
	if glerrors.KindOf(err) == glerrors.KindNotFound {
		return s.provisioner.CreateAccount(ctx, email, displayName)
	}
	return err
}

// DeleteUser removes an account the way the school can live with: a soft
// delete (T2). The person can no longer sign in by any road and leaves every
// listing; the row stays, because payments, grades and the audit trail name it
// by id and a hard delete would orphan the school's history.
//
// actorID is the verified caller (from the access token, never the request).
// The gateway already demanded users:delete; this is the second lock, and the
// only one that knows who the caller is right now rather than when their token
// was minted:
//
//   - nobody deletes themself — compared on the LOADED rows, not on the raw
//     strings the request carried;
//   - the caller must be active and hold users:delete (owner-only);
//   - the last active owner cannot go (enforced atomically by the repository).
//
// ORDER IS LOAD-BEARING: the row first, because it is what every sign-in path
// here checks and because the repository is where the refusals live — a
// refused deletion touches nothing else. Then the links, then the provider.
// If either of those fails the answer carries ErrDeletionPending, the account
// already cannot sign in, and deleting it again finishes the job: an
// already-deleted account only repeats those two idempotent steps.
func (s *AdminService) DeleteUser(ctx context.Context, actorID, targetID string) error {
	actor, err := s.requireDeleter(ctx, actorID)
	if err != nil {
		return err
	}
	target, err := s.users.FindByID(ctx, targetID)
	if err != nil {
		return err
	}
	if target.ID() == actor.ID() {
		return glerrors.New(glerrors.KindForbidden, "no puedes eliminar tu propia cuenta")
	}
	if !target.IsDeleted() {
		if err := s.users.SoftDelete(ctx, target.ID(), actor.ID(), domain.OwnerRoleID); err != nil {
			return err
		}
	}
	if err := s.finishDeletion(ctx, target); err != nil {
		return glerrors.Wrap(glerrors.KindInternal,
			"la cuenta ya no puede entrar, pero no se terminó de cerrar (enlaces pendientes o proveedor de identidad); vuelve a intentarlo",
			errors.Join(domain.ErrDeletionPending, err))
	}
	return nil
}

// finishDeletion kills the account's pending links and switches it off at the
// identity provider. Both steps are idempotent, which is what makes a retry of
// DeleteUser safe.
func (s *AdminService) finishDeletion(ctx context.Context, target domain.User) error {
	if s.invitations != nil {
		if err := s.invitations.Revoke(ctx, target.ID()); err != nil {
			return err
		}
	}
	if s.provisioner == nil {
		return nil
	}
	err := s.provisioner.Disable(ctx, target.Email().String())
	if glerrors.KindOf(err) == glerrors.KindNotFound {
		// Created while passwords were local: nothing there to switch off.
		return nil
	}
	return err
}

// permUsersDelete is the permission DeleteUser demands of its caller.
const permUsersDelete = "users:delete"

// requireDeleter loads the caller as they are NOW and demands an active
// account holding users:delete. Any failure to establish that is a refusal.
func (s *AdminService) requireDeleter(ctx context.Context, actorID string) (domain.User, error) {
	refused := glerrors.New(glerrors.KindForbidden, "no tienes permiso para eliminar usuarios")
	actor, err := s.users.FindByID(ctx, actorID)
	if glerrors.KindOf(err) == glerrors.KindNotFound {
		return domain.User{}, refused
	}
	if err != nil {
		return domain.User{}, err
	}
	want, err := domain.NewPermission(permUsersDelete)
	if err != nil {
		return domain.User{}, err
	}
	if !actor.CanAuthenticate() || !actor.HasPermission(want) {
		return domain.User{}, refused
	}
	return actor, nil
}

// ResendInvitation mints a FRESH token for somebody who has not walked through
// the door yet, killing whatever link they had before.
//
// It refuses on an account that is already active, and that refusal is the
// feature: the button disappearing from the screen is a courtesy, this is the
// boundary. Somebody who has a working password does not need an invitation,
// and handing out one anyway would be a way to hand out access to an account
// that is already in use.
//
// Keyed by USER ID, never by email. An email-keyed version would be a way to
// make this system mail an access link to any address at all.
func (s *AdminService) ResendInvitation(ctx context.Context, userID string) (Invitation, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return Invitation{}, err // KindNotFound for an unknown id
	}
	// ONLY an invited account. This used to refuse just `active`, which let a
	// SUSPENDED one be re-invited on demand — and since redeeming an invitation
	// activated the account, that was an un-suspend button wearing a different
	// name. Suspension is somebody's decision; the way back is reactivating the
	// account, not minting a new way in.
	if user.Status() != domain.StatusInvited {
		return Invitation{}, glerrors.New(glerrors.KindInvalid,
			"esta cuenta no está esperando una invitación")
	}
	out, err := s.issueAndSend(ctx, user, domain.PurposeInvitation, domain.InvitationLifetime)
	if err != nil {
		return Invitation{}, err
	}
	out.User = user
	return out, nil
}

// RequestPasswordReset mails somebody a short-lived link to choose a new
// password. It is the endpoint behind "I forgot my password", which until now
// was a dead anchor on the login screen.
//
// IT NEVER REPORTS ANYTHING. Not whether the address exists, not whether the
// mail went out, not whether the mailer is even configured. The endpoint is
// anonymous, so any difference between "that address has an account" and "it
// does not" is a way to read the school's staff and guardian directory one
// request at a time.
//
// For the same reason the send is fire-and-forget rather than awaited: with a
// synchronous send, an address with an account costs a few hundred milliseconds
// and one without costs five, which is the same oracle measured with a stopwatch
// instead of read off the response.
func (s *AdminService) RequestPasswordReset(ctx context.Context, rawEmail string) error {
	// Padded on EVERY path, including the ones that return early, so the cheap
	// answers cost what the expensive one does.
	defer s.padTo(s.now())

	email, err := domain.NewEmail(rawEmail)
	if err != nil {
		return nil // A malformed address has no account either. Say nothing.
	}
	user, _, err := s.users.FindAuthByEmail(ctx, s.tenantID, email)
	if err != nil {
		return nil // Unknown address. Same silence, same shape, same cost.
	}
	// ACTIVE ONLY, and this is a guard rather than a policy preference.
	//
	// It used to escalate an `invited` account to a fresh invitation token, and
	// issuing one DELETES the live row for that user and purpose. So anyone who
	// knew an invitee's address could destroy the link the director had just
	// handed over, from an endpoint with no session, repeatably, with no signal
	// to anybody. A person who lost their invitation asks the school to resend
	// it; that is a button, and it is behind an administrator.
	if user.Status() != domain.StatusActive {
		return nil
	}
	if _, err := s.issueAndSend(ctx, user, domain.PurposeReset, domain.ResetLifetime); err != nil {
		return nil // Even a failure here is not the caller's business.
	}
	return nil
}

// padTo holds the response until the request has cost resetFloor.
func (s *AdminService) padTo(start time.Time) {
	if elapsed := s.now().Sub(start); elapsed < resetFloor {
		s.sleep(resetFloor - elapsed)
	}
}

// AcceptInvitation is the redemption: somebody arrives with a token and the
// password they just chose.
//
// ORDER IS LOAD-BEARING, and it is not the obvious one:
//
//  1. Validate the password BEFORE anything is spent. A token burned on a
//     password the domain was always going to reject locks somebody out over a
//     typo.
//  2. Consume the token. One statement, matching the hash AND unconsumed AND
//     unexpired, so two simultaneous redemptions leave exactly one winner.
//  3. Write the credential at the provider. If this fails the account is NOT
//     activated, so the person never ends up "active" with no password.
//  4. Activate — and ONLY for an invitation. A reset leaves the status exactly
//     as it found it, because otherwise a suspended person resets their
//     password and walks back in through the front door.
func (s *AdminService) AcceptInvitation(ctx context.Context, token, password string) error {
	if len(password) < minPasswordLen {
		return glerrors.New(glerrors.KindInvalid, "the password is too short")
	}
	inv, err := s.invitations.Consume(ctx, hashToken(token), s.now())
	if err != nil {
		// Unknown, expired and already-spent all arrive here as the same
		// KindNotFound. Telling them apart would be an oracle for guessing.
		return err
	}
	user, err := s.users.FindByID(ctx, inv.UserID())
	if err != nil {
		return err
	}
	// The account's CURRENT state decides, not the token's purpose. Reading the
	// purpose alone treated an invitation as authority to make an account
	// active from any state — so a person the school suspended after inviting
	// them could open the link they already had and come straight back in.
	//
	// Checked BEFORE setCredential, so a locked-out account does not even get
	// its password rewritten. And it answers with the SAME error as a dead
	// token: telling the holder "that account is suspended" would leak the
	// account's state to whoever is holding the link.
	if inv.ActivatesAccount() && user.Status() != domain.StatusInvited {
		return errInvitationDead()
	}
	// A reset leaves the status alone, so it needs its own lock: a deleted
	// account has no password worth writing.
	if user.IsDeleted() {
		return errInvitationDead()
	}
	if err := s.setCredential(ctx, user, password); err != nil {
		// HAND THE TOKEN BACK. Consuming first is what makes two simultaneous
		// redemptions leave exactly one winner, so the order stays — but a
		// provider that answers 503 must not cost somebody their only way in.
		// Without this, the token is spent, the retry says "esta invitación ya
		// no es válida", and the resend guard refuses a new one, so a transient
		// failure locks a person out permanently.
		_ = s.invitations.Release(ctx, inv.ID())
		return err
	}
	if !inv.ActivatesAccount() {
		return nil
	}
	return s.users.SetStatus(ctx, user.ID(), domain.StatusActive, domain.OwnerRoleID)
}

// setCredential writes the password wherever this environment keeps
// credentials, and refuses if that is nowhere.
//
// With a provider configured it owns the credential and a local hash would be a
// second copy to drift. Without one — local development, CI — the local hash IS
// the credential. Writing neither is the failure mode that matters: it would
// leave an ACTIVE account whose stored hash is the empty string.
func (s *AdminService) setCredential(ctx context.Context, user domain.User, password string) error {
	if s.provisioner != nil {
		if err := s.provisioner.SetPassword(ctx, user.Email().String(), password); err != nil {
			return err
		}
		// Stamp the rotation instant even though the credential lives elsewhere.
		//
		// RefreshToken rejects any token minted before password_changed_at
		// (ADR 0005), which is how changing a password revokes older sessions.
		// The local path gets that for free from UpdatePassword; the provider
		// path wrote nothing, so a refresh token stolen before "I forgot my
		// password" kept working for its full life after it.
		return s.users.TouchPasswordChangedAt(ctx, user.ID())
	}
	if s.hasher == nil {
		return glerrors.New(glerrors.KindInternal,
			"no credential store is configured; cannot set a password")
	}
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return err
	}
	return s.users.UpdatePassword(ctx, user.ID(), hash)
}

// Invitation is what the caller gets back after an invitation is issued: enough
// to tell the director what happened, and nothing that would be unsafe to show
// them.
type Invitation struct {
	User domain.User
	// URL is the activation link. Returned so the director can hand it over
	// when the mail did not go out — which, with no SMTP credential yet, is
	// every time. It grants no power the director does not already hold over
	// that account.
	URL       string
	ExpiresAt time.Time
	// Sent says whether the message actually left. It is the signal that did
	// not exist before: the provider answered 200 to sends it silently dropped,
	// so "invitation sent" was a sentence nobody could check.
	Sent bool
	// SendError is why it did not, in the provider's words. Empty when Sent.
	SendError string
	// Via says WHICH road the message took, because the two are not equivalent
	// and the person reading the screen has to know which one they got.
	//
	//   "smtp"     our own message: our link, twenty-four hours, our page.
	//   "provider" the identity provider's: ITS link, one hour, its page, and a
	//              shared sender that Gmail tends to file as spam.
	//   ""         nothing went out; the link is on screen to hand over.
	Via string
}

// issueAndSend mints a token, stores only its hash, and tries to deliver it.
//
// Minting and delivering are together because they are one act; what is NOT
// shared is the delivery POLICY. An invitation is sent while a director watches
// and needs the truth; a reset is sent to nobody watching and must cost the
// same time whether or not the account exists. RequestPasswordReset therefore
// discards this result rather than reporting it.
func (s *AdminService) issueAndSend(ctx context.Context, user domain.User, purpose domain.InvitationPurpose, lifetime time.Duration) (Invitation, error) {
	inv, out, err := s.issueToken(ctx, user, purpose, lifetime)
	if err != nil {
		return Invitation{}, err
	}
	return s.sendAndRecord(ctx, user, inv, out), nil
}

// issueToken mints a token, stores only its hash, and builds the link.
func (s *AdminService) issueToken(ctx context.Context, user domain.User, purpose domain.InvitationPurpose, lifetime time.Duration) (domain.Invitation, Invitation, error) {
	if s.invitations == nil {
		return domain.Invitation{}, Invitation{}, glerrors.New(glerrors.KindInternal,
			"no invitation store is configured")
	}
	token, err := newToken()
	if err != nil {
		return domain.Invitation{}, Invitation{}, err
	}
	expires := s.now().Add(lifetime)
	inv, err := domain.NewInvitation(s.newID(), user.ID(), hashToken(token), purpose, expires, time.Time{})
	if err != nil {
		return domain.Invitation{}, Invitation{}, err
	}
	// Issue REPLACES any live token this user still holds for this purpose.
	// Two live invitations would mean an old mail still opens the door after
	// the director sent a new one.
	if err := s.invitations.Issue(ctx, inv); err != nil {
		return domain.Invitation{}, Invitation{}, err
	}
	return inv, Invitation{
		URL:       s.activateURL + "?token=" + url.QueryEscape(token),
		ExpiresAt: expires,
	}, nil
}

// sendAndRecord delivers the message and writes down what happened.
func (s *AdminService) sendAndRecord(ctx context.Context, user domain.User, inv domain.Invitation, out Invitation) Invitation {
	if via, sendErr := s.deliver(ctx, user, out.URL, out.ExpiresAt); sendErr != nil {
		out.SendError = sendErr.Error()
	} else {
		out.Sent, out.Via = true, via
	}
	// Recorded either way, so the admin screen can show "Sin enviar" with a
	// reason instead of leaving the director to guess. A failure to record must
	// not turn a successful invitation into an error.
	_ = s.invitations.MarkSent(ctx, inv.ID(), s.now(), out.SendError)
	return out
}

// deliver tries the roads out, best first, and reports which one it took.
//
// THE ORDER IS THE POLICY:
//
//  1. Our own mailer, when one is configured. It carries our link — the
//     twenty-four hours the school asked for, our page, in Spanish — and it
//     tells us whether the relay accepted the message.
//
//  2. The identity provider, otherwise. TEMPORARY, and chosen by the owner with
//     the trade on the table: mail that goes out today without waiting on an
//     SMTP credential, at the cost of Google's link (one hour), Google's page
//     (English), and a shared sender Gmail tends to treat as spam. The seam it
//     opens — the password gets set where we cannot see it — is closed by
//     activating on first provider sign-in.
//
//  3. Neither. Not a failure to hide: the account exists, the link is returned,
//     and the screen hands it over. Refusing instead would turn "nobody has
//     given us an SMTP password yet" into "nobody can be given an account".
//
// The first road that works wins, and a configured mailer always beats the
// provider — so the day the school's credential arrives, this stops using
// Firebase on its own, with nothing to remember to switch off.
func (s *AdminService) deliver(ctx context.Context, user domain.User, link string, expires time.Time) (via string, err error) {
	if s.mailer != nil {
		subject, html, text := invitationMessage(user.DisplayName(), link, expires)
		if err := s.mailer.Send(ctx, user.Email().String(), subject, html, text); err != nil {
			return "", err
		}
		return "smtp", nil
	}
	if s.provisioner != nil {
		if err := s.provisioner.SendPasswordSetupEmail(ctx, user.Email().String()); err != nil {
			return "", err
		}
		return "provider", nil
	}
	return "", glerrors.New(glerrors.KindInternal,
		"no hay un servidor de correo configurado; entrega el enlace tú mismo")
}

// errInvitationDead is the ONE answer for every unusable token: unknown,
// expired, already spent, or aimed at an account that is no longer waiting for
// it. They must stay indistinguishable — each difference is a question somebody
// holding a link could ask about an account they do not own.
func errInvitationDead() error {
	return glerrors.New(glerrors.KindNotFound, "esta invitación ya no es válida")
}

// newToken mints 32 bytes of cryptographic randomness, base64url-encoded.
//
// 256 bits: unguessable by construction, so the token needs no rate limit of
// its own to be safe against a brute force. The clear value is returned once
// and never stored.
func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", glerrors.Wrap(glerrors.KindInternal, "minting an invitation token", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// hashToken is what gets stored. A database dump therefore leaks nothing
// usable, and lookups compare hashes through an index rather than comparing
// secrets byte by byte.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
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
	// NOT WHERE THE PROVIDER OWNS CREDENTIALS, and this closes a trap rather
	// than expressing a preference.
	//
	// In that environment `issueFor` strips the rotation flag from every token
	// and ChangePassword refuses — both right, because there is no local
	// password to replace. But this function kept handing out temporary ones
	// anyway, so the account ended up holding a password a director typed, with
	// nothing forcing a rotation and no way to perform one. A credential
	// somebody else knows, valid indefinitely.
	//
	// Refusing here beats re-adding the flag, which would only send the person
	// to a screen that cannot help them. Invite them instead: nobody but the
	// account's owner ever learns the password.
	if s.provisioner != nil {
		return domain.User{}, glerrors.New(glerrors.KindInvalid,
			"aquí las contraseñas las guarda el proveedor; invita al usuario en vez de asignarle una")
	}
	if strings.TrimSpace(tempPassword) == "" {
		return domain.User{}, glerrors.New(glerrors.KindInvalid, "temporary password is required")
	}
	email, err := domain.NewEmail(rawEmail)
	if err != nil {
		return domain.User{}, err // KindInvalid
	}

	assigned, err := s.resolveManualRoles(ctx, roleIDs)
	if err != nil {
		return domain.User{}, err
	}

	// A deleted address is not "already taken" as far as the director can
	// see — the list hides it. This route cannot restore anybody (it would
	// hand a known password to an account with history); inviting does.
	existing, _, err := s.users.FindAuthByEmail(ctx, s.tenantID, email)
	switch {
	case err == nil && existing.IsDeleted():
		return domain.User{}, glerrors.New(glerrors.KindConflict,
			"ese correo es de un usuario eliminado; invítalo para recuperar su cuenta")
	case err != nil && glerrors.KindOf(err) != glerrors.KindNotFound:
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

// resolveManualRoles is resolveRoles for the roads an administrator drives by
// hand, which may not hand out the guardian role (ISSUE-CC-115). The ids are
// checked as typed AND as resolved: Postgres reads a uuid in several spellings,
// and the resolved role carries the one canonical id.
func (s *AdminService) resolveManualRoles(ctx context.Context, roleIDs []string) ([]domain.Role, error) {
	if slices.ContainsFunc(roleIDs, domain.IsGuardianRoleID) {
		return nil, domain.ErrGuardianRoleIsNotManual
	}
	resolved, err := s.resolveRoles(ctx, roleIDs)
	if err != nil {
		return nil, err
	}
	if slices.ContainsFunc(resolved, func(r domain.Role) bool { return domain.IsGuardianRoleID(r.ID()) }) {
		return nil, domain.ErrGuardianRoleIsNotManual
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
// authenticate (domain.User.CanAuthenticate at login and refresh, and
// AuthService.ValidateToken on every request).
//
// actorID is the verified caller (from the access token, never the request).
// Suspending follows the two rules deleting already did (ISSUE-CC-117):
//
//   - nobody suspends themself — compared on the LOADED row, so another
//     spelling of the caller's own uuid is still the caller;
//   - the last active owner cannot be suspended (enforced atomically by the
//     repository, under the same row locks as SoftDelete).
//
// Reactivating needs neither: it adds an active holder, it cannot remove one.
func (s *AdminService) SetUserStatus(ctx context.Context, actorID, targetID string, status domain.Status) error {
	if actorID == "" {
		return glerrors.New(glerrors.KindUnauthorized, "invalid access token")
	}
	if status == domain.StatusSuspended {
		if err := s.refuseSelfSuspension(ctx, actorID, targetID); err != nil {
			return err
		}
	}
	if err := s.users.SetStatus(ctx, targetID, status, domain.OwnerRoleID); err != nil {
		if errors.Is(err, domain.ErrLastOwner) {
			return domain.ErrLastOwnerSuspension
		}
		return err
	}
	// Locking somebody out has to reach the links they are still holding.
	// Otherwise the invitation sitting in their mailbox outlives the decision,
	// and the guard in AcceptInvitation is the only thing standing between it
	// and a way back in. Two locks on the same door, deliberately.
	if status == domain.StatusSuspended && s.invitations != nil {
		_ = s.invitations.Revoke(ctx, targetID)
	}
	return nil
}

// refuseSelfSuspension loads the target and refuses when it is the caller.
func (s *AdminService) refuseSelfSuspension(ctx context.Context, actorID, targetID string) error {
	target, err := s.users.FindByID(ctx, targetID)
	if err != nil {
		return err
	}
	if target.ID() == actorID {
		return domain.ErrSelfSuspension
	}
	return nil
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
