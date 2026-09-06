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
	invited string   // address the invitation went to
	err     error
	sendErr error
}

func (f *fakeProvisioner) SendInvitation(_ context.Context, email string) error {
	if f.sendErr != nil {
		return f.sendErr
	}
	f.invited = email
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
	svc := usecase.NewAdminService(users, roles, fakeHasher{}, prov, testTenant, func() string {
		n++
		return "u" + string(rune('0'+n))
	})
	return svc, users
}

func TestInviteUserCreatesProviderAccount(t *testing.T) {
	prov := &fakeProvisioner{}
	svc, users := inviteService(t, prov)

	user, err := svc.InviteUser(context.Background(), "ana@garcialorca.mx", "Ana", []string{"r1"})
	if err != nil {
		t.Fatalf("InviteUser: %v", err)
	}

	if len(prov.created) != 1 || prov.created[0] != "ana@garcialorca.mx" {
		t.Fatalf("provider accounts created = %v, want exactly [ana@garcialorca.mx]", prov.created)
	}
	if user.Email().String() != "ana@garcialorca.mx" {
		t.Errorf("email = %q", user.Email().String())
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

func TestInviteUserFailsClosedWithoutProvisioner(t *testing.T) {
	svc, users := inviteService(t, nil)

	// No provider configured means no account can be created, and therefore no
	// row either. Creating the row anyway would produce exactly the ghost user
	// the ordering above exists to prevent.
	_, err := svc.InviteUser(context.Background(), "ana@garcialorca.mx", "Ana", []string{"r1"})
	if err == nil {
		t.Fatal("want an error with no provisioner configured, got nil")
	}
	if len(users.created) != 0 {
		t.Errorf("a local row was written with no provider to back it: %+v", users.created)
	}
}

func TestInviteUserSendsTheInvitation(t *testing.T) {
	prov := &fakeProvisioner{}
	svc, _ := inviteService(t, prov)

	if _, err := svc.InviteUser(context.Background(), "ana@garcialorca.mx", "Ana", []string{"r1"}); err != nil {
		t.Fatalf("InviteUser: %v", err)
	}

	// An account created and never announced is an account nobody can use, and
	// the director has nothing to hand the person. Creating without sending was
	// the state this code shipped in for one commit, with a doc comment claiming
	// otherwise — the claim is now a test.
	if prov.invited != "ana@garcialorca.mx" {
		t.Fatalf("invitation sent to %q, want the invited address", prov.invited)
	}
}

func TestInviteUserReportsAnUnsentInvitation(t *testing.T) {
	prov := &fakeProvisioner{sendErr: errors.New("quota exhausted")}
	svc, users := inviteService(t, prov)

	_, err := svc.InviteUser(context.Background(), "ana@garcialorca.mx", "Ana", []string{"r1"})
	if err == nil {
		t.Fatal("want an error when the invitation cannot be sent, got nil")
	}

	// The row and the account STAY. Rolling them back would delete an account
	// that is perfectly good and, worse, free the address for somebody else to
	// register. What is missing is one email, and resending is exactly the
	// operation that fixes it — so the caller is told, and the state it needs is
	// still there.
	if len(users.created) != 1 {
		t.Errorf("the user row was rolled back over an unsent email: %+v", users.created)
	}
}
