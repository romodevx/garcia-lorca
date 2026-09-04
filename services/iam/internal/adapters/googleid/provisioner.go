package googleid

import (
	"context"

	firebaseauth "firebase.google.com/go/v4/auth"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/ports"
)

// accountCreator is the slice of the Admin SDK this adapter needs, declared
// where it is consumed so the mapping is testable without a project.
type accountCreator interface {
	CreateUser(ctx context.Context, user *firebaseauth.UserToCreate) (*firebaseauth.UserRecord, error)
}

// Provisioner implements ports.IdentityProvisioner over Identity Platform.
type Provisioner struct {
	sdk accountCreator
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
