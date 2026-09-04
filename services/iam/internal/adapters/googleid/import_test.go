package googleid_test

import (
	"errors"
	"testing"

	"github.com/garcia-lorca/services/iam/internal/adapters/googleid"
	"github.com/garcia-lorca/services/iam/internal/domain"
)

/*
Moving the existing accounts into the provider (F3 of
.claude/docs/PLAN-identity-platform.md).

The decision this file encodes, and the reason the migration is not disruptive:
we import the BCRYPT HASH we already store, so every user keeps the password
they have today. Nobody is locked out on cutover day, nobody has to go and read
an email first. With a demo running daily, "everyone must reset before they can
work" was the single largest risk in the plan, and importing hashes removes it
outright.

The mapping is a pure function so it can be tested without a project or a
network. The command that calls it does the batching and the network, and has
nothing left to get wrong.
*/

func fixtureUser(t *testing.T, id, email, name string, status domain.Status) domain.User {
	t.Helper()
	e, err := domain.NewEmail(email)
	if err != nil {
		t.Fatalf("NewEmail(%q): %v", email, err)
	}
	u, err := domain.NewUser(id, "t1", e, name, status, false, nil)
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	return u
}

func TestToImportUserCarriesIdentityAndPassword(t *testing.T) {
	u := fixtureUser(t, "u1", "ana@garcialorca.mx", "Ana", domain.StatusActive)

	got, err := googleid.ToImportUser(u, "$2a$10$abcdefghijklmnopqrstuv")
	if err != nil {
		t.Fatalf("ToImportUser: %v", err)
	}

	// The provider's uid IS our user id. Keeping them equal means an audit, a
	// support question or a future join never needs a translation table.
	if got.UID != "u1" {
		t.Errorf("uid = %q, want our own user id", got.UID)
	}
	if got.Email != "ana@garcialorca.mx" {
		t.Errorf("email = %q", got.Email)
	}
	if got.DisplayName != "Ana" {
		t.Errorf("displayName = %q", got.DisplayName)
	}
	if got.PasswordHash != "$2a$10$abcdefghijklmnopqrstuv" {
		t.Errorf("password hash was not carried over: %q", got.PasswordHash)
	}
	if got.Disabled {
		t.Error("an active user must not arrive disabled")
	}
}

func TestToImportUserMarksEmailVerified(t *testing.T) {
	u := fixtureUser(t, "u1", "ana@garcialorca.mx", "Ana", domain.StatusActive)

	got, err := googleid.ToImportUser(u, "$2a$10$hash")
	if err != nil {
		t.Fatalf("ToImportUser: %v", err)
	}

	// WE know this address is legitimate: the row exists because the director
	// created it. And the verifier REFUSES unverified emails on purpose, so
	// importing these as unverified would lock out every migrated user on the
	// first sign-in — the exact failure this whole migration exists to prevent.
	if !got.EmailVerified {
		t.Error("migrated users must arrive verified, or none of them can sign in")
	}
}

func TestToImportUserDisablesSuspendedAccounts(t *testing.T) {
	u := fixtureUser(t, "u2", "baja@garcialorca.mx", "Baja", domain.StatusSuspended)

	got, err := googleid.ToImportUser(u, "$2a$10$hash")
	if err != nil {
		t.Fatalf("ToImportUser: %v", err)
	}

	// Suspension has to travel with the account. The use case checks it again on
	// every sign-in, but an account that is disabled at the provider cannot even
	// obtain a token to present — the outer of the two locks.
	if !got.Disabled {
		t.Error("a suspended user must arrive disabled at the provider")
	}
}

func TestToImportUserSkipsAccountsTheProviderAlreadyOwns(t *testing.T) {
	u := fixtureUser(t, "u3", "invitada@garcialorca.mx", "Invitada", domain.StatusActive)

	// No stored hash means InviteUser created this account and the provider
	// already holds its credential. Nothing to import, and NOT a failure.
	//
	// This used to be a hard error that aborted the entire run, which made the
	// command unusable forever the moment anybody was invited: it died on the
	// first invited row and never reached the rest. A migration in a system that
	// is half-migrated by design has to be able to skip what is already done.
	_, err := googleid.ToImportUser(u, "")
	if !errors.Is(err, googleid.ErrAlreadyProviderManaged) {
		t.Fatalf("error = %v, want ErrAlreadyProviderManaged", err)
	}
}
