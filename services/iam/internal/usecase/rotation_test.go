package usecase_test

import (
	"context"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
)

/*
The temporary password, closed at the source.

Where an identity provider holds credentials, `issueFor` strips the rotation
flag from every token and `ChangePassword` refuses — both correct, because there
is no local password to rotate. What was NOT correct is that `CreateUser` kept
handing out temporary passwords in exactly that environment. The result was an
account holding a password a director typed, with nothing forcing a rotation
(the flag was gone) and no way to perform one (the rotation was refused): a
credential somebody else knows, valid indefinitely.

Refusing at CreateUser is the fix rather than re-adding the flag, because the
flag would send them to a screen that cannot help. The way to add somebody where
the provider owns credentials is to invite them — which is what the admin screen
now always does, and what leaves nobody but the account's owner knowing the
password.
*/

func TestCreateUserRefusesWhereTheProviderOwnsCredentials(t *testing.T) {
	users := &fakeUsers{byID: map[string]domain.User{}}
	// A provisioner present is what "the provider owns credentials here" means.
	svc := newInviteService(t, users, &fakeProvisioner{}, &fakeInvitations{}, &fakeMailer{})

	_, err := svc.CreateUser(context.Background(), "temporal@gl.mx", "Temporal", testPassword, []string{"r1"})
	if err == nil {
		t.Fatal("want a refusal: a temporary password nobody can rotate is a permanent one")
	}
	if glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("kind = %v, want KindInvalid — the caller asked for the wrong thing, nothing failed", glerrors.KindOf(err))
	}
	if len(users.created) != 0 {
		t.Error("a row was written anyway")
	}
}

// Without a provider the older path is untouched: local development and any
// environment that has not migrated keep adding people with a temporary
// password, and that password can still be rotated.
func TestCreateUserStillWorksWithoutAProvider(t *testing.T) {
	users := &fakeUsers{byID: map[string]domain.User{}}
	svc := newInviteService(t, users, nil, &fakeInvitations{}, &fakeMailer{})

	u, err := svc.CreateUser(context.Background(), "temporal@gl.mx", "Temporal", testPassword, []string{"r1"})
	if err != nil {
		t.Fatalf("CreateUser without a provider: %v", err)
	}
	if !u.MustChangePassword() {
		t.Error("the account must be flagged to rotate the password somebody else chose")
	}
}
