package googleid

import (
	"context"

	firebaseauth "firebase.google.com/go/v4/auth"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/ports"
)

// accountCreator is the slice of the Admin SDK this adapter needs, declared
// where it is consumed so the mapping is testable without a project.
//
// Three methods, and no more. The Admin SDK surface is wide enough to delete
// accounts and rewrite the project's auth configuration, and the runtime
// service account is granted exactly the permissions these three need
// (deploy/terraform/identity_platform.tf) — so this interface is also the
// written form of what iam is allowed to do.
type accountCreator interface {
	CreateUser(ctx context.Context, user *firebaseauth.UserToCreate) (*firebaseauth.UserRecord, error)
	GetUserByEmail(ctx context.Context, email string) (*firebaseauth.UserRecord, error)
	UpdateUser(ctx context.Context, uid string, user *firebaseauth.UserToUpdate) (*firebaseauth.UserRecord, error)
}

// Provisioner implements ports.IdentityProvisioner over Identity Platform.
type Provisioner struct {
	sdk accountCreator
	// inviter is the TEMPORARY path where the provider mails its own link.
	// NIL when no web api key is configured, which makes SendPasswordSetupEmail
	// refuse with a message naming what is missing.
	inviter *Inviter
	// userNotFound recognizes the SDK's "no such account" answer. A field only
	// so a test can stand in for an error the SDK does not let anyone build.
	userNotFound func(error) bool
}

// SetPassword writes the credential its owner just chose and marks the address
// verified.
//
// Verified is honest here, and it was not before: following a link we mailed to
// that address is what proves the person reads mail there. The account was
// created unverified precisely so this moment could be the thing that changes
// it.
//
// This replaced SendInvitation, which asked the provider to MAIL the link. That
// call returned HTTP 200 for messages the provider never delivered — including
// to addresses with no account at all — so it reported success for a thing that
// did not happen. Delivery moved to a mailer we can hold accountable; the
// provider is left holding the credential, which it does well.
func (p *Provisioner) SetPassword(ctx context.Context, email, password string) error {
	uid, err := p.uidOf(ctx, email)
	if err != nil {
		return err
	}
	params := (&firebaseauth.UserToUpdate{}).Password(password).EmailVerified(true)
	if _, err := p.sdk.UpdateUser(ctx, uid, params); err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "writing the password at the identity provider", err)
	}
	return nil
}

// SetDisabled switches sign-in at the provider off or on, and touches nothing
// else about the account: its password and verified address survive, so a
// person invited back after being deleted (T2) keeps the credential they had.
//
// UpdateUser is one of the three calls this adapter is allowed, so this needs
// no new grant on the runtime service account.
func (p *Provisioner) SetDisabled(ctx context.Context, email string, disabled bool) error {
	uid, err := p.uidOf(ctx, email)
	if err != nil {
		return err
	}
	if _, err := p.sdk.UpdateUser(ctx, uid, (&firebaseauth.UserToUpdate{}).Disabled(disabled)); err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "changing sign-in at the identity provider", err)
	}
	return nil
}

// uidOf resolves the provider's id for an address. No account there is
// KindNotFound; anything else is KindInternal.
func (p *Provisioner) uidOf(ctx context.Context, email string) (string, error) {
	rec, err := p.sdk.GetUserByEmail(ctx, email)
	if err != nil {
		if p.userNotFound(err) {
			return "", glerrors.New(glerrors.KindNotFound, "that account does not exist at the identity provider")
		}
		return "", glerrors.Wrap(glerrors.KindInternal, "looking up the provider account", err)
	}
	return rec.UID, nil
}

// SendPasswordSetupEmail delegates to the Inviter. See its doc for what this
// path costs and why it exists anyway.
func (p *Provisioner) SendPasswordSetupEmail(ctx context.Context, email string) error {
	if p.inviter == nil {
		return glerrors.New(glerrors.KindInternal,
			"no hay llave del proveedor configurada; no se puede enviar por Firebase")
	}
	return p.inviter.SendPasswordSetupEmail(ctx, email)
}

// NewProvisioner wraps an SDK client.
func NewProvisioner(sdk accountCreator, inviter *Inviter) *Provisioner {
	return &Provisioner{sdk: sdk, inviter: inviter, userNotFound: firebaseauth.IsUserNotFound}
}

// CreateAccount creates the account WITHOUT a password and WITHOUT marking the
// address verified.
//
// Both omissions are the design, not an oversight:
//
//   - No password, because its owner sets the first one through the invitation
//     link. Nobody else ever knows it, and this service never stores a second
//     copy of a credential.
//   - Not verified, because we have not verified anything. Following the link
//     is what proves the person reads mail at that address, and the provider
//     marks it verified then. Asserting it here would make every mistyped
//     address a pre-verified account that its real owner could later claim
//     through a password reset.
func (p *Provisioner) CreateAccount(ctx context.Context, email, displayName string) error {
	params := (&firebaseauth.UserToCreate{}).
		Email(email).
		EmailVerified(false).
		DisplayName(displayName)

	if _, err := p.sdk.CreateUser(ctx, params); err != nil {
		if firebaseauth.IsEmailAlreadyExists(err) {
			return glerrors.New(glerrors.KindConflict, "that email already has an account")
		}
		return glerrors.Wrap(glerrors.KindInternal, "creating the provider account", err)
	}
	return nil
}

// compile-time proof the adapter satisfies the port it exists for.
var _ ports.IdentityProvisioner = (*Provisioner)(nil)
