package usecase_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/ports"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/usecase"
)

/*
Activation by first sign-in.

The owner asked for the identity provider to carry the invitation mail for now,
even though the link, the page and the one-hour lifetime are Google's. That is a
deliberate trade and it works — except for one seam: the person sets their
password on a page we do not own, so nothing tells US it happened. Without this,
they would stay `invited` forever, unable to log in, and the resend button the
owner asked to disappear would never disappear.

So activation stops being "you redeemed our token" and becomes "you proved you
can sign in". That is the truer definition of the two, and it covers both paths:
our own link, and the provider's.

The line that must not move: only a PROVIDER sign-in may do this. The provider
verified an email and a password it holds, which is the same proof our own link
was ever asking for. A local password sign-in proves nothing new — the hash it
checks is one nobody could have set without already being active.
*/

func TestProviderSignInActivatesAnInvitedAccount(t *testing.T) {
	invited := mustUser(t, "u1", "maestra@gl.mx", domain.StatusInvited)
	users := usersByEmail(t, map[string]domain.User{"u1": invited})
	inv := &fakeInvitations{}

	svc := usecase.NewAuthService(users, &fakeRoles{}, &fakeHasher{}, &fakeTokens{}, &fakeTokens{},
		&fakeIDTokens{verified: map[string]ports.VerifiedIdentity{"tok": {Email: "maestra@gl.mx"}}}, inv, "t1", time.Minute, time.Now)

	if _, err := svc.AuthenticateWithIDToken(context.Background(), "tok", domain.SessionStaff); err != nil {
		t.Fatalf("an invited user who proved they hold the credential must get in: %v", err)
	}
	if users.statusSet["u1"] != domain.StatusActive {
		t.Errorf("status = %v, want StatusActive after the first provider sign-in", users.statusSet["u1"])
	}
	// Any invitation still outstanding dies here. Otherwise a link handed out
	// hours earlier would keep being able to set the password of an account
	// that is now in daily use.
	if len(inv.revoked) != 1 || inv.revoked[0] != "u1" {
		t.Errorf("revoked = %v, want the live invitations for u1 to be killed", inv.revoked)
	}
}

// A SUSPENDED account is not an invited one, and no sign-in resurrects it.
// Suspension is a decision somebody made; proving you still hold the password
// says nothing about that decision.
func TestProviderSignInDoesNotResurrectASuspendedAccount(t *testing.T) {
	users := usersByEmail(t, map[string]domain.User{
		"u1": mustUser(t, "u1", "suspendida@gl.mx", domain.StatusSuspended),
	})
	inv := &fakeInvitations{}
	svc := usecase.NewAuthService(users, &fakeRoles{}, &fakeHasher{}, &fakeTokens{}, &fakeTokens{},
		&fakeIDTokens{verified: map[string]ports.VerifiedIdentity{"tok": {Email: "suspendida@gl.mx"}}}, inv, "t1", time.Minute, time.Now)

	_, err := svc.AuthenticateWithIDToken(context.Background(), "tok", domain.SessionStaff)
	if err == nil || glerrors.KindOf(err) != glerrors.KindForbidden {
		t.Fatalf("want KindForbidden for a suspended account, got %v", err)
	}
	if _, touched := users.statusSet["u1"]; touched {
		t.Error("a suspended account was reactivated by signing in — suspension must survive a login")
	}
}

// An already-active account is left alone: no pointless write on every sign-in.
func TestProviderSignInDoesNotRewriteAnActiveAccount(t *testing.T) {
	users := usersByEmail(t, map[string]domain.User{
		"u1": mustUser(t, "u1", "activa@gl.mx", domain.StatusActive),
	})
	inv := &fakeInvitations{}
	svc := usecase.NewAuthService(users, &fakeRoles{}, &fakeHasher{}, &fakeTokens{}, &fakeTokens{},
		&fakeIDTokens{verified: map[string]ports.VerifiedIdentity{"tok": {Email: "activa@gl.mx"}}}, inv, "t1", time.Minute, time.Now)

	if _, err := svc.AuthenticateWithIDToken(context.Background(), "tok", domain.SessionStaff); err != nil {
		t.Fatalf("AuthenticateWithIDToken: %v", err)
	}
	if len(users.statusSet) != 0 || len(inv.revoked) != 0 {
		t.Errorf("an active sign-in wrote something: statuses=%v revoked=%v", users.statusSet, inv.revoked)
	}
}

// The local password path must NOT activate. Its hash could only have been set
// by somebody already through the door, so it proves nothing new — and making
// it an activation route would be a second, quieter way out of `invited`.
func TestLocalPasswordSignInNeverActivates(t *testing.T) {
	invited := mustUser(t, "u1", "maestra@gl.mx", domain.StatusInvited)
	users := usersByEmail(t, map[string]domain.User{"u1": invited})
	users.byEmail[invited.Email()] = struct {
		user domain.User
		hash string
	}{user: invited, hash: "hash"}
	inv := &fakeInvitations{}
	svc := usecase.NewAuthService(users, &fakeRoles{}, &fakeHasher{}, &fakeTokens{}, &fakeTokens{},
		nil, inv, "t1", time.Minute, time.Now)

	_, err := svc.Authenticate(context.Background(), "maestra@gl.mx", "cualquiera")
	if err == nil {
		t.Fatal("an invited account must not sign in with a local password")
	}
	if len(users.statusSet) != 0 {
		t.Error("the local password path activated an account")
	}
}
