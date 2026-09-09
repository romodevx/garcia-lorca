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
	rec, err := p.sdk.GetUserByEmail(ctx, email)
	if err != nil {
		if firebaseauth.IsUserNotFound(err) {
			return glerrors.New(glerrors.KindNotFound, "that account does not exist at the identity provider")
		}
		return glerrors.Wrap(glerrors.KindInternal, "looking up the provider account", err)
	}
	params := (&firebaseauth.UserToUpdate{}).Password(password).EmailVerified(true)
	if _, err := p.sdk.UpdateUser(ctx, rec.UID, params); err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "writing the password at the identity provider", err)
	}
	return nil
}

// NewProvisioner wraps an SDK client.
func NewProvisioner(sdk accountCreator) *Provisioner {
	return &Provisioner{sdk: sdk}
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
