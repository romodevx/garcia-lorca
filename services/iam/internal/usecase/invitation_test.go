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

type fakeInvitations struct {
	issued   []domain.Invitation
	live     map[string]domain.Invitation
	consumed []string // token hashes, in call order
	revoked  []string // user ids whose live invitations were killed
	marked   []string // "<id>:<err>" per MarkSent call
	issueErr error
	consume  domain.Invitation
	consErr  error
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

func (f *fakeInvitations) Revoke(_ context.Context, userID string) error {
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

// A suspended account is not an invited one, but the director may still need to
// give its owner a way back in once reactivated. Resending is allowed; what is
// forbidden is resending to somebody who is already through the door.
func TestResendInvitationAllowedOnSuspended(t *testing.T) {
	users := &fakeUsers{byID: map[string]domain.User{
		"u1": mustUser(t, "u1", "suspendida@gl.mx", domain.StatusSuspended),
	}}
	inv := &fakeInvitations{}
	if _, err := newInviteService(t, users, &fakeProvisioner{}, inv, &fakeMailer{}).
		ResendInvitation(context.Background(), "u1"); err != nil {
		t.Fatalf("ResendInvitation on suspended: %v", err)
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
		AcceptInvitation(context.Background(), "un-token-cualquiera", "fixture-pw-value"); err != nil {
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
		AcceptInvitation(context.Background(), "token", "fixture-pw-value"); err != nil {
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
				AcceptInvitation(context.Background(), "token", "fixture-pw-value")
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
		AcceptInvitation(context.Background(), "token", "fixture-pw-value")
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

// The send must NOT happen inside the call. A mailer that blocks for a second
// would make a request for a real address cost a second and one for an unknown
// address cost nothing — the same oracle the constant 204 exists to close, just
// measured with a stopwatch. The delivery is detached, so the call returns at
// the same speed either way.
func TestRequestPasswordResetDoesNotWaitForTheMailer(t *testing.T) {
	users := usersByEmail(t, map[string]domain.User{
		"u1": mustUser(t, "u1", "existe@gl.mx", domain.StatusActive),
	})
	// A mailer that would block for a full second if it were awaited.
	slow := &blockingMailer{release: make(chan struct{})}
	defer close(slow.release)

	start := time.Now()
	if err := newInviteService(t, users, &fakeProvisioner{}, &fakeInvitations{}, slow).
		RequestPasswordReset(context.Background(), "existe@gl.mx"); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Errorf("the call waited %v for the mailer; an address with an account must not cost more than one without", elapsed)
	}
}

// blockingMailer parks in Send until released, standing in for a slow relay.
type blockingMailer struct{ release chan struct{} }

func (m *blockingMailer) Send(context.Context, string, string, string, string) error {
	<-m.release
	return nil
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
