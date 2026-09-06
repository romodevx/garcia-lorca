package grpcadapter_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	iamv1 "github.com/garcia-lorca/gen/go/garcialorca/iam/v1"
	"github.com/garcia-lorca/services/iam/internal/adapters/bcrypt"
	grpcadapter "github.com/garcia-lorca/services/iam/internal/adapters/grpc"
	"github.com/garcia-lorca/services/iam/internal/adapters/jwt"
	"github.com/garcia-lorca/services/iam/internal/ports"
	"github.com/garcia-lorca/services/iam/internal/usecase"
)

// recordingProvisioner stands in for Identity Platform: it remembers what it was
// asked to do so the tests can assert the invitation actually goes out, and can
// be told to refuse.
type recordingProvisioner struct {
	created []string
	sent    []string
	sendErr error
}

func (p *recordingProvisioner) CreateAccount(_ context.Context, email, _ string) error {
	p.created = append(p.created, email)
	return nil
}

func (p *recordingProvisioner) SendInvitation(_ context.Context, email string) error {
	p.sent = append(p.sent, email)
	return p.sendErr
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
	if len(prov.created) != 1 || len(prov.sent) != 1 {
		t.Fatalf("provider calls: created=%v sent=%v", prov.created, prov.sent)
	}
}

// TestInviteUserWithoutProviderIsInternal: an environment with no provider
// cannot invite. It must say so rather than write a user nobody can sign in as.
func TestInviteUserWithoutProviderIsInternal(t *testing.T) {
	users := &fakeUsers{}
	h := handlerWithProvisioner(t, users, &fakeRoles{known: "r1"}, nil)

	_, err := h.InviteUser(context.Background(), connect.NewRequest(&iamv1.InviteUserRequest{
		Email: "alguien@garcialorca.mx", DisplayName: "Alguien",
	}))
	assertCode(t, err, connect.CodeInternal)
	if users.created != nil {
		t.Fatal("no user row may be written when the provider is missing")
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
func TestInviteUserReportsAnUndeliveredInvitation(t *testing.T) {
	prov := &recordingProvisioner{sendErr: errors.New("quota exceeded")}
	users := &fakeUsers{}
	h := handlerWithProvisioner(t, users, &fakeRoles{known: "r1"}, prov)

	_, err := h.InviteUser(context.Background(), connect.NewRequest(&iamv1.InviteUserRequest{
		Email: "otra@garcialorca.mx", DisplayName: "Otra",
	}))
	if err == nil {
		t.Fatal("an undelivered invitation must be reported")
	}
	if users.created == nil {
		t.Fatal("the account must survive an undelivered invitation, so it can be resent")
	}
}

func handlerWithProvisioner(t *testing.T, users *fakeUsers, roles *fakeRoles, prov ports.IdentityProvisioner) *grpcadapter.IAMHandler {
	t.Helper()
	hasher := bcrypt.NewHasher(4)
	tokens := jwt.NewHMAC([]byte(strings.Repeat("gl-key-", 6)), "garcia-lorca", time.Hour, time.Now)
	auth := usecase.NewAuthService(users, roles, hasher, tokens, tokens, nil, "t1", 15*time.Minute, time.Now)
	admin := usecase.NewAdminService(users, roles, hasher, prov, "t1", func() string { return "generated-id" })
	return grpcadapter.NewIAMHandler(auth, admin, usecase.NewHealthService("iam"))
}
