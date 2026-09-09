package grpcadapter_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	iamv1 "github.com/garcia-lorca/gen/go/garcialorca/iam/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/adapters/bcrypt"
	grpcadapter "github.com/garcia-lorca/services/iam/internal/adapters/grpc"
	"github.com/garcia-lorca/services/iam/internal/adapters/jwt"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/ports"
	"github.com/garcia-lorca/services/iam/internal/usecase"
)

// recordingProvisioner stands in for Identity Platform: it remembers what it
// was asked to do, and can be told to refuse.
//
// It used to record SendInvitation calls and the tests asserted on them — which
// is how a suite stays green while the product mails nothing. Asking the
// provider to send was never evidence that anything was delivered; the provider
// answered 200 either way. The recorder now tracks the two things that ARE
// true afterwards: an account exists, and a credential was written.
type recordingProvisioner struct {
	created []string
	// passwords is the addresses whose credential was written, in call order.
	passwords []string
	setErr    error
}

func (p *recordingProvisioner) CreateAccount(_ context.Context, email, _ string) error {
	p.created = append(p.created, email)
	return nil
}

func (p *recordingProvisioner) SetPassword(_ context.Context, email, _ string) error {
	if p.setErr != nil {
		return p.setErr
	}
	p.passwords = append(p.passwords, email)
	return nil
}

// recordingMailer is what actually carries the invitation now, and therefore
// what the tests assert against.
type recordingMailer struct {
	to  []string
	err error
}

func (m *recordingMailer) Send(_ context.Context, to, _, _, _ string) error {
	if m.err != nil {
		return m.err
	}
	m.to = append(m.to, to)
	return nil
}

// stubInvitations keeps issued tokens in memory so the handler tests exercise
// the real use-case flow without a database.
type stubInvitations struct {
	live map[string]domain.Invitation // by token hash
}

func (s *stubInvitations) Issue(_ context.Context, inv domain.Invitation) error {
	if s.live == nil {
		s.live = map[string]domain.Invitation{}
	}
	for h, existing := range s.live {
		if existing.UserID() == inv.UserID() && existing.Purpose() == inv.Purpose() {
			delete(s.live, h)
		}
	}
	s.live[inv.TokenHash()] = inv
	return nil
}

func (s *stubInvitations) Consume(_ context.Context, hash string, now time.Time) (domain.Invitation, error) {
	inv, ok := s.live[hash]
	if !ok || !inv.IsUsable(now) {
		return domain.Invitation{}, glerrors.New(glerrors.KindNotFound, "esta invitación ya no es válida")
	}
	delete(s.live, hash)
	return inv, nil
}

func (s *stubInvitations) MarkSent(context.Context, string, time.Time, string) error { return nil }

func (s *stubInvitations) ListLive(context.Context, time.Time) (map[string]domain.Invitation, error) {
	return nil, nil
}

// TestInviteUserCreatesTheAccountAndMailsTheLink is the whole point of the RPC:
// reachable over the wire, and the person actually gets the mail.
func TestInviteUserCreatesTheAccountAndMailsTheLink(t *testing.T) {
	prov := &recordingProvisioner{}
	h := handlerWithProvisioner(t, &fakeUsers{}, &fakeRoles{known: "r1"}, prov)

	res, err := h.InviteUser(context.Background(), connect.NewRequest(&iamv1.InviteUserRequest{
		Email:       "nueva.maestra@garcialorca.mx",
		DisplayName: "Nueva Maestra",
		RoleIds:     []string{"r1"},
	}))
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if got := res.Msg.GetUser().GetEmail(); got != "nueva.maestra@garcialorca.mx" {
		t.Fatalf("email = %q", got)
	}
	// No temporary password exists, so nothing is pending rotation. A true here
	// would send the person straight into a change-password screen for a
	// password they never had.
	if res.Msg.GetUser().GetMustChangePassword() {
		t.Fatal("must_change_password should be false: there is no password to change")
	}
	// The provider CREATES; it no longer sends. Asserting on a send here was
	// asserting that we asked Google to mail something, which it accepted and
	// dropped. The delivery assertion moved to the mailer, where it can be true.
	if len(prov.created) != 1 {
		t.Fatalf("provider created = %v, want exactly one account", prov.created)
	}
	if res.Msg.GetInvitation().GetUrl() == "" {
		t.Error("no activation link on the response")
	}
}

// TestInviteUserWithoutProviderIsInternal: an environment with no provider
// Without a provider, inviting WORKS — and this test asserted the opposite.
//
// The old rule made sense while the provider was also what mailed the link:
// with no provider there was no way to reach the person, so writing a user row
// would have created somebody nobody could ever sign in as. Now the token and
// the delivery are ours, so a missing Identity Platform project says nothing
// about whether an account can be handed out. It is what local development and
// CI run as, and refusing there meant the flow could not be exercised at all
// without a Google project.
func TestInviteUserWorksWithoutAProvider(t *testing.T) {
	users := &fakeUsers{}
	h := handlerWithProvisioner(t, users, &fakeRoles{known: "r1"}, nil)

	res, err := h.InviteUser(context.Background(), connect.NewRequest(&iamv1.InviteUserRequest{
		Email: "alguien@garcialorca.mx", DisplayName: "Alguien",
	}))
	if err != nil {
		t.Fatalf("InviteUser without a provider: %v", err)
	}
	if users.created == nil {
		t.Fatal("no user row was written")
	}
	if res.Msg.GetUser().GetStatus() != iamv1.UserStatus_USER_STATUS_INVITED {
		t.Errorf("status = %v, want INVITED", res.Msg.GetUser().GetStatus())
	}
	if res.Msg.GetInvitation().GetUrl() == "" {
		t.Error("no activation link returned")
	}
}

// TestInviteUserRejectsABadAddress before either side is touched — a malformed
// address is the caller's mistake, not a provider failure.
func TestInviteUserRejectsABadAddress(t *testing.T) {
	prov := &recordingProvisioner{}
	h := handlerWithProvisioner(t, &fakeUsers{}, &fakeRoles{known: "r1"}, prov)

	_, err := h.InviteUser(context.Background(), connect.NewRequest(&iamv1.InviteUserRequest{
		Email: "no-es-un-correo", DisplayName: "X",
	}))
	assertCode(t, err, connect.CodeInvalidArgument)
	if len(prov.created) != 0 {
		t.Fatal("the provider must not be called for a malformed address")
	}
}

// TestInviteUserReportsAnUndeliveredInvitation: the account survives (deleting it
// would free the address for somebody else), but the director is told, because
// otherwise the person waits for a mail that never came.
// An undelivered invitation is REPORTED, not raised.
//
// This test inverted when the sender changed, and the inversion is the fix: the
// old shape returned an error, which told the director something went wrong and
// left them unable to see what, or to act on it. Now the call succeeds, the
// account stands, and the response carries `sent: false`, the reason, and the
// link — which is the difference between "something failed" and "here is what
// to hand the person".
func TestInviteUserReportsAnUndeliveredInvitation(t *testing.T) {
	users := &fakeUsers{}
	h := handlerWithMailer(t, users, &fakeRoles{known: "r1"}, &recordingProvisioner{},
		&recordingMailer{err: errors.New("535 authentication failed")})

	res, err := h.InviteUser(context.Background(), connect.NewRequest(&iamv1.InviteUserRequest{
		Email: "otra@garcialorca.mx", DisplayName: "Otra",
	}))
	if err != nil {
		t.Fatalf("a dead mailer must not fail the invitation: %v", err)
	}
	inv := res.Msg.GetInvitation()
	if inv.GetSent() {
		t.Error("sent = true after the mailer refused")
	}
	if inv.GetSendError() == "" {
		t.Error("no reason given — the director has to know WHY, or this is the bug being fixed")
	}
	if inv.GetUrl() == "" {
		t.Error("no link returned; with nothing delivered it is the only way to hand access over")
	}
	if users.created == nil {
		t.Fatal("the account must survive an undelivered invitation, so it can be resent")
	}
}

func handlerWithProvisioner(t *testing.T, users *fakeUsers, roles *fakeRoles, prov ports.IdentityProvisioner) *grpcadapter.IAMHandler {
	t.Helper()
	return handlerWithMailer(t, users, roles, prov, &recordingMailer{})
}

func handlerWithMailer(t *testing.T, users *fakeUsers, roles *fakeRoles, prov ports.IdentityProvisioner, mail ports.Mailer) *grpcadapter.IAMHandler {
	t.Helper()
	hasher := bcrypt.NewHasher(4)
	tokens := jwt.NewHMAC([]byte(strings.Repeat("gl-key-", 6)), "garcia-lorca", time.Hour, time.Now)
	auth := usecase.NewAuthService(users, roles, hasher, tokens, tokens, nil, "t1", 15*time.Minute, time.Now)
	n := 0
	admin := usecase.NewAdminService(users, roles, hasher, prov, &stubInvitations{}, mail,
		"https://escuela.example/activar", "t1",
		func() string { n++; return fmt.Sprintf("generated-id-%d", n) }, nil)
	return grpcadapter.NewIAMHandler(auth, admin, usecase.NewHealthService("iam"))
}

// Reachable over the wire, and it mails the address the USER ROW carries — not
// one the caller supplies. An email-keyed resend would be a way to make this
// system send an access link to any address at all.
func TestResendInvitationMailsTheUser(t *testing.T) {
	users := &fakeUsers{}
	mail := &recordingMailer{}
	h := handlerWithMailer(t, users, &fakeRoles{known: "r1"}, &recordingProvisioner{}, mail)

	created, err := h.InviteUser(context.Background(), connect.NewRequest(&iamv1.InviteUserRequest{
		Email: "rezagada@garcialorca.mx", DisplayName: "Rezagada",
	}))
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	mail.to = nil

	res, err := h.ResendInvitation(context.Background(), connect.NewRequest(&iamv1.ResendInvitationRequest{
		UserId: created.Msg.GetUser().GetId(),
	}))
	if err != nil {
		t.Fatalf("resend: %v", err)
	}
	if len(mail.to) != 1 || mail.to[0] != "rezagada@garcialorca.mx" {
		t.Fatalf("mailed %v, want the address on the user row", mail.to)
	}
	if res.Msg.GetInvitation().GetUrl() == "" {
		t.Error("no link returned")
	}
}

// The owner's third request, at the boundary that enforces it: an account that
// is already active does not get a new invitation. The button disappearing from
// the screen is a courtesy; this is the rule.
func TestResendInvitationRefusesAnActiveAccount(t *testing.T) {
	users := &fakeUsers{}
	mail := &recordingMailer{}
	h := handlerWithMailer(t, users, &fakeRoles{known: "r1"}, &recordingProvisioner{}, mail)

	created, err := h.InviteUser(context.Background(), connect.NewRequest(&iamv1.InviteUserRequest{
		Email: "yaentro@garcialorca.mx", DisplayName: "Ya Entró",
	}))
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	id := created.Msg.GetUser().GetId()
	if _, err := h.SetUserStatus(context.Background(), connect.NewRequest(&iamv1.SetUserStatusRequest{
		UserId: id, Status: iamv1.UserStatus_USER_STATUS_ACTIVE,
	})); err != nil {
		t.Fatalf("activate: %v", err)
	}
	mail.to = nil

	_, err = h.ResendInvitation(context.Background(), connect.NewRequest(&iamv1.ResendInvitationRequest{UserId: id}))
	assertCode(t, err, connect.CodeInvalidArgument)
	if len(mail.to) != 0 {
		t.Errorf("mailed %v to an account that is already in use", mail.to)
	}
}

// TestResendInvitationForAnUnknownUserIsNotFound.
func TestResendInvitationForAnUnknownUserIsNotFound(t *testing.T) {
	prov := &recordingProvisioner{}
	h := handlerWithProvisioner(t, &fakeUsers{}, &fakeRoles{known: "r1"}, prov)

	_, err := h.ResendInvitation(context.Background(), connect.NewRequest(&iamv1.ResendInvitationRequest{UserId: "no-existe"}))
	assertCode(t, err, connect.CodeNotFound)
	if len(prov.passwords) != 0 {
		t.Fatal("nothing may happen for an unknown user")
	}
}
