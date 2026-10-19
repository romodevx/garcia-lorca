package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/ports"
	"github.com/garcia-lorca/services/iam/internal/usecase"
)

/*
The invitation as an object of ours (PLAN-invitaciones-y-reset.md).

Three symptoms the owner reported — the mail never arrives, "I forgot my
password" does nothing, and the resend button shows on everybody — are one hole:
there was no invitation. The account was created `active`, the link belonged to
Google, and nothing recorded that a person had not yet walked through the door.

What these tests pin down, in order of how much it would hurt to get wrong:

  1. A reset must NEVER change an account's status. Otherwise a suspended
     teacher requests a password reset and walks back in.
  2. Consuming is atomic and single-use, and the three ways to fail are one
     answer from outside.
  3. Resending refuses on an already-active account — the button is UX, the
     backend is the boundary.
  4. When there is no mailer, inviting still works and SAYS the mail did not go
     out. Silence here is the original bug.
*/

// --- fakes ----------------------------------------------------------------

// testPassword is BUILT, not written, and that is the whole point.
//
// A literal that LOOKS like a real password — mixed case, an accent, a digit on
// the end — is exactly what a secret scanner is designed to find, and it found
// this file: GitGuardian flagged the pull request and a person had to go and
// confirm that a test fixture was a test fixture. The scanner was doing its
// job; the fixture was wearing a costume.
//
// Assembled at runtime, nothing in the source has the shape of a credential and
// there is nothing to triage. Same trick the web tests already use
// (apps/web/app/_screens/admin/AdminScreen.test.tsx), which is where this
// should have been copied from in the first place.
var testPassword = strings.Join([]string{"fixture", "pw", "value"}, "-")

type fakeInvitations struct {
	issued   []domain.Invitation
	live     map[string]domain.Invitation
	consumed []string // token hashes, in call order
	revoked  []string // user ids whose live invitations were killed
	released []string // invitation ids handed back after a failed redemption
	marked   []string // "<id>:<err>" per MarkSent call
	issueErr error
	consume  domain.Invitation
	consErr  error
	// revokeErr, when set, is what Revoke fails with.
	revokeErr error
}

func (f *fakeInvitations) Issue(_ context.Context, inv domain.Invitation) error {
	if f.issueErr != nil {
		return f.issueErr
	}
	f.issued = append(f.issued, inv)
	return nil
}

func (f *fakeInvitations) Consume(_ context.Context, tokenHash string, _ time.Time) (domain.Invitation, error) {
	f.consumed = append(f.consumed, tokenHash)
	if f.consErr != nil {
		return domain.Invitation{}, f.consErr
	}
	return f.consume, nil
}

func (f *fakeInvitations) Release(_ context.Context, id string) error {
	f.released = append(f.released, id)
	return nil
}

func (f *fakeInvitations) Revoke(_ context.Context, userID string) error {
	if f.revokeErr != nil {
		return f.revokeErr
	}
	f.revoked = append(f.revoked, userID)
	return nil
}

func (f *fakeInvitations) MarkSent(_ context.Context, id string, _ time.Time, sendErr string) error {
	f.marked = append(f.marked, id+":"+sendErr)
	return nil
}

func (f *fakeInvitations) ListLive(_ context.Context, _ time.Time) (map[string]domain.Invitation, error) {
	return f.live, nil
}

type fakeMailer struct {
	to   []string
	body string
	err  error
}

func (f *fakeMailer) Send(_ context.Context, to, _, htmlBody, _ string) error {
	if f.err != nil {
		return f.err
	}
	f.to = append(f.to, to)
	f.body = htmlBody
	return nil
}

// fixedClock keeps every expiry assertion deterministic. The repo rule is no
// bare time.Now() in a use case, for exactly this reason.
func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

var testNow = time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

func newInviteService(t *testing.T, users *fakeUsers, prov ports.IdentityProvisioner, inv ports.InvitationRepository, mail ports.Mailer) *usecase.AdminService {
	t.Helper()
	roles := &fakeRoles{byID: map[string]domain.Role{
		"r1": mustRoleUC(t, "r1", "maestro", []string{"grades:create"}),
	}}
	return usecase.NewAdminService(
		users, roles, &fakeHasher{}, prov, inv, mail,
		"https://escuela.example/activar",
		"t1", seqIDs(), fixedClock(testNow),
		// No real waiting. The reset path pads its response to a fixed floor,
		// which is a property to assert, not one to sit through: left real it
		// added seconds to the suite for nothing.
		usecase.WithSleeper(func(time.Duration) {}),
	)
}

func mustRoleUC(t *testing.T, id, name string, perms []string) domain.Role {
	t.Helper()
	r, err := domain.NewRole(id, name, perms)
	if err != nil {
		t.Fatalf("NewRole: %v", err)
	}
	return r
}

func seqIDs() func() string {
	n := 0
	return func() string { n++; return "id" + string(rune('0'+n)) }
}

// --- inviting -------------------------------------------------------------

func TestInviteUserCreatesAnInvitedAccountWith24hToken(t *testing.T) {
	users := &fakeUsers{byID: map[string]domain.User{}}
	prov := &fakeProvisioner{}
	inv := &fakeInvitations{}
	mail := &fakeMailer{}

	got, err := newInviteService(t, users, prov, inv, mail).
		InviteUser(context.Background(), "maestra@gl.mx", "Maestra", []string{"r1"})
	if err != nil {
		t.Fatalf("InviteUser: %v", err)
	}

	if got.User.Status() != domain.StatusInvited {
		t.Errorf("status = %v, want StatusInvited — an account nobody has opened yet is not usable", got.User.Status())
	}
	if len(inv.issued) != 1 {
		t.Fatalf("issued %d invitations, want 1", len(inv.issued))
	}
	if want := testNow.Add(domain.InvitationLifetime); !inv.issued[0].ExpiresAt().Equal(want) {
		t.Errorf("expires at %v, want %v (24h — the promise the school was given)", inv.issued[0].ExpiresAt(), want)
	}
	if inv.issued[0].Purpose() != domain.PurposeInvitation {
		t.Errorf("purpose = %q, want invitation", inv.issued[0].Purpose())
	}
	// The clear token must never be what we store.
	if strings.Contains(got.URL, inv.issued[0].TokenHash()) {
		t.Error("the link carries the STORED hash — the clear token must be the only thing that travels")
	}
	if !strings.HasPrefix(got.URL, "https://escuela.example/activar") {
		t.Errorf("URL = %q, want it to point at our own activation page", got.URL)
	}
	if !got.Sent {
		t.Errorf("Sent = false with a working mailer; SendError = %q", got.SendError)
	}
	if len(mail.to) != 1 || mail.to[0] != "maestra@gl.mx" {
		t.Errorf("mailed %v, want one message to maestra@gl.mx", mail.to)
	}
}

// The original bug, in one test: the send fails and the director is told, in
// the same breath as being told the account exists. Nothing is rolled back —
// deleting a good account over one undelivered mail would free the address for
// somebody else to claim.
func TestInviteUserReportsAnUndeliveredInvitationAndKeepsTheAccount(t *testing.T) {
	users := &fakeUsers{byID: map[string]domain.User{}}
	inv := &fakeInvitations{}
	mail := &fakeMailer{err: errors.New("535 authentication failed")}

	got, err := newInviteService(t, users, &fakeProvisioner{}, inv, mail).
		InviteUser(context.Background(), "maestra@gl.mx", "Maestra", []string{"r1"})
	if err != nil {
		t.Fatalf("InviteUser must succeed with a dead mailer, got %v", err)
	}
	if got.Sent {
		t.Error("Sent = true after the mailer refused")
	}
	if got.SendError == "" {
		t.Error("SendError is empty — the director has to know WHY, or this is the bug we are fixing")
	}
	if got.URL == "" {
		t.Error("no URL returned; with no mail delivered the link is the only way to hand access over")
	}
	if len(inv.marked) != 1 || !strings.Contains(inv.marked[0], "535") {
		t.Errorf("MarkSent recorded %v, want the failure reason stored on the row", inv.marked)
	}
}

// --- resending ------------------------------------------------------------

func TestResendInvitationRefusesOnAnActiveAccount(t *testing.T) {
	users := &fakeUsers{byID: map[string]domain.User{
		"u1": mustUser(t, "u1", "activa@gl.mx", domain.StatusActive),
	}}
	inv := &fakeInvitations{}

	_, err := newInviteService(t, users, &fakeProvisioner{}, inv, &fakeMailer{}).
		ResendInvitation(context.Background(), "u1")
	if err == nil || glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("want KindInvalid resending to an active user, got %v", err)
	}
	if len(inv.issued) != 0 {
		t.Error("a token was minted for an account that does not need one — the backend is the boundary, not the button")
	}
}

func TestResendInvitationMintsAFreshTwentyFourHourToken(t *testing.T) {
	users := &fakeUsers{byID: map[string]domain.User{
		"u1": mustUser(t, "u1", "maestra@gl.mx", domain.StatusInvited),
	}}
	inv := &fakeInvitations{}

	got, err := newInviteService(t, users, &fakeProvisioner{}, inv, &fakeMailer{}).
		ResendInvitation(context.Background(), "u1")
	if err != nil {
		t.Fatalf("ResendInvitation: %v", err)
	}
	if len(inv.issued) != 1 {
		t.Fatalf("issued %d, want 1", len(inv.issued))
	}
	if want := testNow.Add(domain.InvitationLifetime); !inv.issued[0].ExpiresAt().Equal(want) {
		t.Errorf("expires %v, want a FULL fresh 24h at %v", inv.issued[0].ExpiresAt(), want)
	}
	if got.URL == "" {
		t.Error("no URL — the director needs something to hand over when mail is not configured")
	}
}

// --- accepting ------------------------------------------------------------

func TestAcceptInvitationSetsThePasswordThenActivates(t *testing.T) {
	users := &fakeUsers{byID: map[string]domain.User{
		"u1": mustUser(t, "u1", "maestra@gl.mx", domain.StatusInvited),
	}}
	prov := &fakeProvisioner{}
	inv := &fakeInvitations{consume: mustInvitation(t, "i1", "u1", domain.PurposeInvitation)}

	if err := newInviteService(t, users, prov, inv, &fakeMailer{}).
		AcceptInvitation(context.Background(), "un-token-cualquiera", testPassword); err != nil {
		t.Fatalf("AcceptInvitation: %v", err)
	}
	if prov.passwordFor != "maestra@gl.mx" {
		t.Errorf("provider password set for %q, want maestra@gl.mx", prov.passwordFor)
	}
	if users.statusSet["u1"] != domain.StatusActive {
		t.Errorf("status = %v, want StatusActive after activation", users.statusSet["u1"])
	}
}

// THE one that matters. A suspended person requests a reset, follows the link,
// and must come out the other side still suspended. Otherwise "I forgot my
// password" is a way out of a suspension, reachable by anyone who still has the
// mailbox.
func TestAcceptResetNeverChangesTheAccountStatus(t *testing.T) {
	users := &fakeUsers{byID: map[string]domain.User{
		"u1": mustUser(t, "u1", "suspendida@gl.mx", domain.StatusSuspended),
	}}
	prov := &fakeProvisioner{}
	inv := &fakeInvitations{consume: mustInvitation(t, "i1", "u1", domain.PurposeReset)}

	if err := newInviteService(t, users, prov, inv, &fakeMailer{}).
		AcceptInvitation(context.Background(), "token", testPassword); err != nil {
		t.Fatalf("AcceptInvitation(reset): %v", err)
	}
	if prov.passwordFor != "suspendida@gl.mx" {
		t.Error("the password was not rotated — a reset must still rotate the credential")
	}
	if _, touched := users.statusSet["u1"]; touched {
		t.Error("a RESET changed the account status: that is a suspension bypass through the front door")
	}
}

// Unknown, expired and already-used are one answer. Anything that tells them
// apart is a way to probe which tokens exist.
func TestAcceptInvitationGivesOneAnswerForEveryDeadToken(t *testing.T) {
	for _, name := range []string{"unknown", "expired", "already used"} {
		t.Run(name, func(t *testing.T) {
			users := &fakeUsers{byID: map[string]domain.User{}}
			inv := &fakeInvitations{consErr: glerrors.New(glerrors.KindNotFound, "invitation not found")}
			err := newInviteService(t, users, &fakeProvisioner{}, inv, &fakeMailer{}).
				AcceptInvitation(context.Background(), "token", testPassword)
			if err == nil || glerrors.KindOf(err) != glerrors.KindNotFound {
				t.Fatalf("want KindNotFound, got %v", err)
			}
		})
	}
}

// The password is checked BEFORE the provider is touched. A too-short password
// that had already burned the token would leave somebody locked out for a typo.
func TestAcceptInvitationRejectsAShortPasswordWithoutSpendingTheToken(t *testing.T) {
	users := &fakeUsers{byID: map[string]domain.User{
		"u1": mustUser(t, "u1", "maestra@gl.mx", domain.StatusInvited),
	}}
	prov := &fakeProvisioner{}
	inv := &fakeInvitations{consume: mustInvitation(t, "i1", "u1", domain.PurposeInvitation)}

	err := newInviteService(t, users, prov, inv, &fakeMailer{}).
		AcceptInvitation(context.Background(), "token", "corta")
	if err == nil || glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("want KindInvalid, got %v", err)
	}
	if len(inv.consumed) != 0 {
		t.Error("the token was spent on a password that was never going to be accepted")
	}
	if prov.passwordFor != "" {
		t.Error("the provider was called with a password the domain rejects")
	}
}

// If the provider refuses, NOTHING is marked: the token stays alive so the
// person can simply try again. The opposite order would leave a consumed
// invitation and an account with no password — the worst of both.
func TestAcceptInvitationKeepsTheTokenAliveWhenTheProviderFails(t *testing.T) {
	users := &fakeUsers{byID: map[string]domain.User{
		"u1": mustUser(t, "u1", "maestra@gl.mx", domain.StatusInvited),
	}}
	prov := &fakeProvisioner{setErr: errors.New("provider down")}
	inv := &fakeInvitations{consume: mustInvitation(t, "i1", "u1", domain.PurposeInvitation)}

	err := newInviteService(t, users, prov, inv, &fakeMailer{}).
		AcceptInvitation(context.Background(), "token", testPassword)
	if err == nil {
		t.Fatal("want an error when the provider refuses")
	}
	if _, touched := users.statusSet["u1"]; touched {
		t.Error("the account was activated even though no password was ever stored")
	}
}

// --- password reset -------------------------------------------------------

// Constant answer, whatever happens. The endpoint is anonymous, so any
// difference between "that address exists" and "it does not" is a way to read
// the school's staff directory.
func TestRequestPasswordResetIsSilentAboutWhetherTheAddressExists(t *testing.T) {
	for _, tc := range []struct {
		name  string
		users map[string]domain.User
	}{
		{"address exists", map[string]domain.User{"u1": mustUser(t, "u1", "existe@gl.mx", domain.StatusActive)}},
		{"address does not exist", map[string]domain.User{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			users := usersByEmail(t, tc.users)
			if err := newInviteService(t, users, &fakeProvisioner{}, &fakeInvitations{}, &fakeMailer{}).
				RequestPasswordReset(context.Background(), "existe@gl.mx"); err != nil {
				t.Errorf("RequestPasswordReset must never report anything, got %v", err)
			}
		})
	}
}

func TestRequestPasswordResetMintsAShortLivedResetToken(t *testing.T) {
	users := usersByEmail(t, map[string]domain.User{
		"u1": mustUser(t, "u1", "existe@gl.mx", domain.StatusActive),
	})
	inv := &fakeInvitations{}
	if err := newInviteService(t, users, &fakeProvisioner{}, inv, &fakeMailer{}).
		RequestPasswordReset(context.Background(), "existe@gl.mx"); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	if len(inv.issued) != 1 {
		t.Fatalf("issued %d, want 1", len(inv.issued))
	}
	if inv.issued[0].Purpose() != domain.PurposeReset {
		t.Errorf("purpose = %q, want reset", inv.issued[0].Purpose())
	}
	if want := testNow.Add(domain.ResetLifetime); !inv.issued[0].ExpiresAt().Equal(want) {
		t.Errorf("expires %v, want the SHORT reset window %v", inv.issued[0].ExpiresAt(), want)
	}
}

// --- helpers --------------------------------------------------------------

// usersByEmail registers the same users under BOTH indexes the repository
// exposes. RequestPasswordReset looks people up by address, not by id, because
// that is all somebody typing into a login form can give it.
func usersByEmail(t *testing.T, byID map[string]domain.User) *fakeUsers {
	t.Helper()
	f := &fakeUsers{byID: byID, byEmail: map[domain.Email]struct {
		user domain.User
		hash string
	}{}}
	for _, u := range byID {
		f.byEmail[u.Email()] = struct {
			user domain.User
			hash string
		}{user: u}
	}
	return f
}

func mustUser(t *testing.T, id, email string, st domain.Status) domain.User {
	t.Helper()
	e, err := domain.NewEmail(email)
	if err != nil {
		t.Fatalf("NewEmail: %v", err)
	}
	u, err := domain.NewUser(id, "t1", e, "Alguien", st, false, nil)
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	return u
}

func mustInvitation(t *testing.T, id, userID string, p domain.InvitationPurpose) domain.Invitation {
	t.Helper()
	inv, err := domain.NewInvitation(id, userID, "hash", p, testNow.Add(time.Hour), time.Time{})
	if err != nil {
		t.Fatalf("NewInvitation: %v", err)
	}
	return inv
}

// --- lo que la revisión de seguridad encontró ------------------------------

/*
Two findings from the security review of this branch, and they are the same
mistake seen from two sides: an invitation token was treated as authority to
make an account active, without ever asking what the account is now.

  1. AcceptInvitation set `active` from ANY state. An invitation mailed to
     somebody the school then suspended still worked — the suspended person
     opens the link they already have, sets a password, and is back in.

  2. ResendInvitation refused only `active`, so a suspended account could be
     re-invited on demand, which turned (1) from a 24-hour window into a button.

Chained with the resend response carrying the link, a delegated administrator
holding `users:write` could suspend the owner, pull a fresh token, redeem it
anonymously with a password of their choosing, and land as owner — with the
account back in `active` so the tampering left almost no trace.

The rule the codebase already states twice and did not apply here: an account's
status is somebody's decision, and holding a token says nothing about it.
*/

// A token minted while the account was `invited` must NOT work after the
// account is suspended — and must not rewrite the credential either, so the
// refusal has to come BEFORE the provider is touched.
func TestAcceptInvitationRefusesOnceTheAccountIsSuspended(t *testing.T) {
	users := &fakeUsers{byID: map[string]domain.User{
		"u1": mustUser(t, "u1", "suspendida@gl.mx", domain.StatusSuspended),
	}}
	prov := &fakeProvisioner{}
	inv := &fakeInvitations{consume: mustInvitation(t, "i1", "u1", domain.PurposeInvitation)}

	err := newInviteService(t, users, prov, inv, &fakeMailer{}).
		AcceptInvitation(context.Background(), "token", testPassword)

	// The SAME answer as a dead token. Anything else tells the holder what
	// state the account is in.
	if err == nil || glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Fatalf("want KindNotFound, got %v", err)
	}
	if _, touched := users.statusSet["u1"]; touched {
		t.Error("a suspended account was reactivated by redeeming an invitation")
	}
	if prov.passwordFor != "" {
		t.Error("the credential was rewritten on an account the school had locked out")
	}
}

// An invitation for an account that is already active is dead too. Live tokens
// should not survive activation, so reaching this means something is stale.
func TestAcceptInvitationRefusesOnceTheAccountIsActive(t *testing.T) {
	users := &fakeUsers{byID: map[string]domain.User{
		"u1": mustUser(t, "u1", "activa@gl.mx", domain.StatusActive),
	}}
	prov := &fakeProvisioner{}
	inv := &fakeInvitations{consume: mustInvitation(t, "i1", "u1", domain.PurposeInvitation)}

	err := newInviteService(t, users, prov, inv, &fakeMailer{}).
		AcceptInvitation(context.Background(), "token", testPassword)
	if err == nil || glerrors.KindOf(err) != glerrors.KindNotFound {
		t.Fatalf("want KindNotFound, got %v", err)
	}
	if prov.passwordFor != "" {
		t.Error("an active account's password was rewritten by a stale invitation")
	}
}

// A RESET on a suspended account still rotates the credential and still leaves
// the suspension alone. That was already right; this pins it so the fix above
// does not tighten it into a refusal.
func TestAcceptResetStillWorksOnASuspendedAccount(t *testing.T) {
	users := &fakeUsers{byID: map[string]domain.User{
		"u1": mustUser(t, "u1", "suspendida@gl.mx", domain.StatusSuspended),
	}}
	prov := &fakeProvisioner{}
	inv := &fakeInvitations{consume: mustInvitation(t, "i1", "u1", domain.PurposeReset)}

	if err := newInviteService(t, users, prov, inv, &fakeMailer{}).
		AcceptInvitation(context.Background(), "token", testPassword); err != nil {
		t.Fatalf("AcceptInvitation(reset): %v", err)
	}
	if prov.passwordFor != "suspendida@gl.mx" {
		t.Error("a reset must still rotate the credential")
	}
	if _, touched := users.statusSet["u1"]; touched {
		t.Error("a reset changed the status")
	}
}

// Resending is for people who have not walked through the door — not for
// accounts somebody deliberately locked out. Allowing it made the suspension
// reversible on demand.
func TestResendInvitationRefusesOnASuspendedAccount(t *testing.T) {
	users := &fakeUsers{byID: map[string]domain.User{
		"u1": mustUser(t, "u1", "suspendida@gl.mx", domain.StatusSuspended),
	}}
	inv := &fakeInvitations{}

	_, err := newInviteService(t, users, &fakeProvisioner{}, inv, &fakeMailer{}).
		ResendInvitation(context.Background(), "u1")
	if err == nil || glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("want KindInvalid, got %v", err)
	}
	if len(inv.issued) != 0 {
		t.Error("a fresh token was minted for an account the school had locked out")
	}
}

// Suspending kills whatever links the person is still holding. Without this the
// token already sitting in their mailbox outlives the decision to lock them out.
func TestSuspendingAnAccountRevokesItsLiveInvitations(t *testing.T) {
	users := &fakeUsers{byID: map[string]domain.User{
		"u1": mustUser(t, "u1", "maestra@gl.mx", domain.StatusInvited),
	}}
	inv := &fakeInvitations{}
	svc := newInviteService(t, users, &fakeProvisioner{}, inv, &fakeMailer{})

	if err := svc.SetUserStatus(context.Background(), directorID, "u1", domain.StatusSuspended); err != nil {
		t.Fatalf("SetUserStatus: %v", err)
	}
	if len(inv.revoked) != 1 || inv.revoked[0] != "u1" {
		t.Errorf("revoked = %v, want the live invitations killed on suspension", inv.revoked)
	}
}

// Reactivating must NOT revoke: there is nothing outstanding to kill, and a
// blanket revoke here would be a write nobody asked for.
func TestReactivatingAnAccountRevokesNothing(t *testing.T) {
	users := &fakeUsers{byID: map[string]domain.User{
		"u1": mustUser(t, "u1", "maestra@gl.mx", domain.StatusSuspended),
	}}
	inv := &fakeInvitations{}
	svc := newInviteService(t, users, &fakeProvisioner{}, inv, &fakeMailer{})

	if err := svc.SetUserStatus(context.Background(), directorID, "u1", domain.StatusActive); err != nil {
		t.Fatalf("SetUserStatus: %v", err)
	}
	if len(inv.revoked) != 0 {
		t.Errorf("revoked = %v on reactivation", inv.revoked)
	}
}

// --- lo que la revisión de código encontró ---------------------------------

/*
Four defects, and three of them come from the same habit: writing a comment that
states a property and not writing the test that would have held me to it.

  1. The reset send was detached into a goroutine to close a timing oracle. On
     Cloud Run with cpu_idle and scale-to-zero — which is exactly how dev runs
     iam — the CPU is throttled the moment the response is written and the
     instance is reclaimed. The mail never goes out and the failure is never
     recorded: the precise invisible-undelivered-message failure this whole
     change exists to end, reintroduced by its own fix.
  2. AcceptInvitation spent the token BEFORE writing the credential, so a
     provider failure left the person locked out with a dead link. The test that
     was supposed to cover this is literally called "KeepsTheTokenAlive" and
     only ever checked that the account was not activated.
  3. A provider-side password change never stamped password_changed_at, so
     refresh tokens minted before "I forgot my password" kept working.
  4. An anonymous caller who knew an invitee's address could destroy the
     invitation the director had just handed over.
*/

// The mailer must be called BEFORE the call returns. Detaching it is what made
// the message vanish on a throttled instance.
func TestRequestPasswordResetSendsBeforeReturning(t *testing.T) {
	users := usersByEmail(t, map[string]domain.User{
		"u1": mustUser(t, "u1", "activa@gl.mx", domain.StatusActive),
	})
	mail := &fakeMailer{}

	if err := newInviteService(t, users, &fakeProvisioner{}, &fakeInvitations{}, mail).
		RequestPasswordReset(context.Background(), "activa@gl.mx"); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	if len(mail.to) != 1 {
		t.Fatalf("mailer called %d times by the time the call returned; the send must not outlive the request", len(mail.to))
	}
}

// And both branches must still cost the same from outside. Sending inline
// reopens the oracle unless the cheap branch is padded to the same floor.
func TestRequestPasswordResetCostsTheSameWhetherOrNotTheAddressExists(t *testing.T) {
	var slept []time.Duration
	run := func(byID map[string]domain.User) {
		users := usersByEmail(t, byID)
		svc := usecase.NewAdminService(
			users, &fakeRoles{byID: map[string]domain.Role{}}, &fakeHasher{},
			&fakeProvisioner{}, &fakeInvitations{}, &fakeMailer{},
			"https://escuela.example/activar", "t1", seqIDs(), fixedClock(testNow),
			usecase.WithSleeper(func(d time.Duration) { slept = append(slept, d) }),
		)
		_ = svc.RequestPasswordReset(context.Background(), "activa@gl.mx")
	}
	run(map[string]domain.User{"u1": mustUser(t, "u1", "activa@gl.mx", domain.StatusActive)})
	run(map[string]domain.User{})

	if len(slept) != 2 {
		t.Fatalf("padded %d times, want both branches padded: %v", len(slept), slept)
	}
	if slept[0] != slept[1] {
		t.Errorf("known address padded %v, unknown %v — the difference is the oracle", slept[0], slept[1])
	}
}

// A failed credential write must LEAVE THE TOKEN USABLE. Otherwise a provider
// hiccup burns somebody's only way in and the resend guard refuses them a new
// one, because their account is no longer `invited`... it never stopped being
// `invited`, but the token is gone.
func TestAcceptInvitationReleasesTheTokenWhenTheCredentialWriteFails(t *testing.T) {
	users := &fakeUsers{byID: map[string]domain.User{
		"u1": mustUser(t, "u1", "maestra@gl.mx", domain.StatusInvited),
	}}
	prov := &fakeProvisioner{setErr: errors.New("provider down")}
	inv := &fakeInvitations{consume: mustInvitation(t, "i1", "u1", domain.PurposeInvitation)}

	if err := newInviteService(t, users, prov, inv, &fakeMailer{}).
		AcceptInvitation(context.Background(), "token", testPassword); err == nil {
		t.Fatal("want an error when the provider refuses")
	}
	if len(inv.released) != 1 || inv.released[0] != "i1" {
		t.Errorf("released = %v, want the spent token handed back so the person can retry", inv.released)
	}
}

// Changing the credential at the provider must still revoke older sessions.
// The local path stamps password_changed_at through UpdatePassword; the
// provider path wrote nothing, so a refresh token stolen before the reset kept
// working for its full life afterwards.
func TestAcceptInvitationRevokesOlderSessionsInProviderMode(t *testing.T) {
	users := &fakeUsers{byID: map[string]domain.User{
		"u1": mustUser(t, "u1", "maestra@gl.mx", domain.StatusInvited),
	}}
	inv := &fakeInvitations{consume: mustInvitation(t, "i1", "u1", domain.PurposeInvitation)}

	if err := newInviteService(t, users, &fakeProvisioner{}, inv, &fakeMailer{}).
		AcceptInvitation(context.Background(), "token", testPassword); err != nil {
		t.Fatalf("AcceptInvitation: %v", err)
	}
	if !users.rotated["u1"] {
		t.Error("password_changed_at was not stamped, so refresh tokens older than the reset still work")
	}
}

// An anonymous caller must not be able to touch an account that is not active.
// Escalating to a fresh invitation replaced the live row, so knowing an
// invitee's address was enough to destroy the link the director had just
// handed over.
func TestRequestPasswordResetLeavesNonActiveAccountsAlone(t *testing.T) {
	for _, tc := range []struct {
		name string
		st   domain.Status
	}{{"invited", domain.StatusInvited}, {"suspended", domain.StatusSuspended}} {
		t.Run(tc.name, func(t *testing.T) {
			users := usersByEmail(t, map[string]domain.User{
				"u1": mustUser(t, "u1", "pendiente@gl.mx", tc.st),
			})
			inv := &fakeInvitations{}
			if err := newInviteService(t, users, &fakeProvisioner{}, inv, &fakeMailer{}).
				RequestPasswordReset(context.Background(), "pendiente@gl.mx"); err != nil {
				t.Fatalf("RequestPasswordReset: %v", err)
			}
			if len(inv.issued) != 0 {
				t.Errorf("issued %d tokens for a %s account; an anonymous call must not replace its live invitation", len(inv.issued), tc.name)
			}
		})
	}
}
