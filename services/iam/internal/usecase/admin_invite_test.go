package usecase_test

import (
	"context"
	"errors"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/ports"
	"github.com/garcia-lorca/services/iam/internal/usecase"
)

/*
Creating a user WITHOUT anybody inventing a password for them (F3b of
.claude/docs/PLAN-identity-platform.md).

The old shape had the director type a temporary password and hand it over. That
is three problems in one: somebody else knows your password, the system stores a
credential nobody chose, and — the one that mattered for this migration — the
account is usable before its owner has ever proved they can read mail at that
address.

The new shape: the account is created at the provider with NO password. Its
owner sets one through a reset link, and using that link is what proves they
control the mailbox, so the address ends up verified because they verified it,
not because we asserted it.

SENDING that link is NOT this function's job and no test here pretends it is —
an earlier version of this comment did, which is exactly the kind of claim that
reads as covered. iam has no mail sender; the admin screen asks the provider to
deliver it.

The consequence worth naming: a mistyped address now fails LOUDLY and early —
the invitation never arrives and the account is never usable. Under the old
shape a typo sat there quietly, and once password reset exists it would have
become a way in for whoever really owns that address.
*/

// fakeProvisioner implements ports.IdentityProvisioner.
type fakeProvisioner struct {
	created []string // emails, in call order
	// passwordFor is the address whose credential was last written. It replaced
	// the old `invited`, which recorded that we ASKED the provider to send mail —
	// a request the provider answered 200 to and then dropped. Recording the
	// request was the test believing the same lie the product did.
	passwordFor string
	err         error
	setErr      error
	// providerSent records the TEMPORARY path where the provider mails its own
	// link, so a test can tell "our message went out" from "we asked Google to
	// send its own" — two outcomes the director experiences very differently.
	providerSent    []string
	providerSendErr error
	// disabled records SetDisabled calls as "<email>:<true|false>".
	disabled    []string
	disabledErr error
	reenableErr error
}

// Reenable records "<email>:reenabled": sign-in back on, old credential gone.
func (f *fakeProvisioner) Reenable(_ context.Context, email string) error {
	if f.reenableErr != nil {
		return f.reenableErr
	}
	f.disabled = append(f.disabled, email+":reenabled")
	return nil
}

func (f *fakeProvisioner) SetDisabled(_ context.Context, email string, disabled bool) error {
	if f.disabledErr != nil {
		return f.disabledErr
	}
	state := "false"
	if disabled {
		state = "true"
	}
	f.disabled = append(f.disabled, email+":"+state)
	return nil
}

func (f *fakeProvisioner) SendPasswordSetupEmail(_ context.Context, email string) error {
	if f.providerSendErr != nil {
		return f.providerSendErr
	}
	f.providerSent = append(f.providerSent, email)
	return nil
}

func (f *fakeProvisioner) SetPassword(_ context.Context, email, _ string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.passwordFor = email
	return nil
}

func (f *fakeProvisioner) CreateAccount(_ context.Context, email, _ string) error {
	if f.err != nil {
		return f.err
	}
	f.created = append(f.created, email)
	return nil
}

func inviteService(t *testing.T, prov ports.IdentityProvisioner) (*usecase.AdminService, *fakeUsers) {
	t.Helper()
	users := &fakeUsers{
		byEmail: map[domain.Email]struct {
			user domain.User
			hash string
		}{},
		byID:      map[string]domain.User{},
		statusSet: map[string]domain.Status{},
		rolesSet:  map[string][]string{},
	}
	role, _ := domain.NewRole("r1", "Docente", []string{"students:read"})
	roles := &fakeRoles{byID: map[string]domain.Role{"r1": role}}
	n := 0
	svc := usecase.NewAdminService(users, roles, fakeHasher{}, prov,
		&fakeInvitations{}, &fakeMailer{}, "https://escuela.example/activar", testTenant,
		func() string {
			n++
			return "u" + string(rune('0'+n))
		}, nil)
	return svc, users
}

func TestInviteUserCreatesProviderAccount(t *testing.T) {
	prov := &fakeProvisioner{}
	svc, users := inviteService(t, prov)

	out, err := svc.InviteUser(context.Background(), "ana@garcialorca.mx", "Ana", []string{"r1"})
	if err != nil {
		t.Fatalf("InviteUser: %v", err)
	}

	if len(prov.created) != 1 || prov.created[0] != "ana@garcialorca.mx" {
		t.Fatalf("provider accounts created = %v, want exactly [ana@garcialorca.mx]", prov.created)
	}
	if out.User.Email().String() != "ana@garcialorca.mx" {
		t.Errorf("email = %q", out.User.Email().String())
	}
	if len(users.created) != 1 {
		t.Fatalf("rows written = %d, want 1", len(users.created))
	}
}

func TestInviteUserStoresNoPassword(t *testing.T) {
	prov := &fakeProvisioner{}
	svc, users := inviteService(t, prov)

	if _, err := svc.InviteUser(context.Background(), "ana@garcialorca.mx", "Ana", []string{"r1"}); err != nil {
		t.Fatalf("InviteUser: %v", err)
	}

	// The whole point. No temporary password is invented, so none is hashed and
	// none is stored: the provider owns credentials now, and there is no second
	// copy here to drift, leak, or be reverted by a re-import.
	if got := users.created[0].hash; got != "" {
		t.Errorf("a password hash was stored (%q); the provider owns credentials now", got)
	}
}

func TestInviteUserDoesNotCreateTheRowWhenTheProviderRefuses(t *testing.T) {
	prov := &fakeProvisioner{err: errors.New("EMAIL_EXISTS")}
	svc, users := inviteService(t, prov)

	_, err := svc.InviteUser(context.Background(), "ana@garcialorca.mx", "Ana", []string{"r1"})
	if err == nil {
		t.Fatal("want an error when the provider refuses, got nil")
	}

	// Order matters and this test is what fixes it. A row written before the
	// provider call would leave a user who exists here, holds permissions, and
	// can never sign in — invisible until somebody tries. Create at the provider
	// first; only then write the row.
	if len(users.created) != 0 {
		t.Errorf("a local row was written despite the provider refusing: %+v", users.created)
	}
}

func TestInviteUserRejectsMalformedEmail(t *testing.T) {
	prov := &fakeProvisioner{}
	svc, _ := inviteService(t, prov)

	_, err := svc.InviteUser(context.Background(), "no-es-un-correo", "Ana", []string{"r1"})
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Fatalf("error = %v, want KindInvalid", err)
	}
	// And nothing was created anywhere: the address is checked before either
	// side is touched.
	if len(prov.created) != 0 {
		t.Errorf("provider was called with a malformed address: %v", prov.created)
	}
}

// The inverse of what this test used to assert, and the inversion is the point.
//
// It refused to invite without a provider, because back then the provider was
// also what mailed the link: a row with no way to reach its owner was the ghost
// user the write ordering exists to prevent. Now the token and the delivery are
// ours, so the users row IS the account when there is no provider — which is
// what local development and CI run as, and what the school will run as until
// its Identity Platform project is wired.
func TestInviteUserWorksWithoutAProvisioner(t *testing.T) {
	svc, users := inviteService(t, nil)

	out, err := svc.InviteUser(context.Background(), "ana@garcialorca.mx", "Ana", []string{"r1"})
	if err != nil {
		t.Fatalf("InviteUser without a provisioner: %v", err)
	}
	if len(users.created) != 1 {
		t.Fatalf("created %d rows, want 1", len(users.created))
	}
	if out.User.Status() != domain.StatusInvited {
		t.Errorf("status = %v, want StatusInvited", out.User.Status())
	}
	if out.URL == "" {
		t.Error("no activation link returned")
	}
}
