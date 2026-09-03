package googleid

import (
	firebaseauth "firebase.google.com/go/v4/auth"
	"firebase.google.com/go/v4/auth/hash"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
)

// ImportUser is one account on its way into the provider, in a plain shape so
// the mapping can be asserted without building SDK types in a test.
type ImportUser struct {
	UID           string
	Email         string
	DisplayName   string
	PasswordHash  string
	EmailVerified bool
	Disabled      bool
}

// ToImportUser maps one of our users, plus the bcrypt hash we already store,
// into the account to create at the provider.
//
// Importing the EXISTING hash is what makes this migration invisible: every
// user keeps the password they have today, so nobody is locked out at cutover
// and nobody has to read an email before they can work. With a demo running
// daily, that was the largest risk in the plan.
func ToImportUser(user domain.User, passwordHash string) (ImportUser, error) {
	if passwordHash == "" {
		// An account imported with no password is one anybody could claim by
		// "resetting" it. Refuse, and let the operator go look at the row.
		return ImportUser{}, glerrors.New(glerrors.KindInvalid,
			"user "+user.ID()+" has no stored password hash")
	}
	return ImportUser{
		UID:         user.ID(),
		Email:       user.Email().String(),
		DisplayName: user.DisplayName(),

		// WE know this address is legitimate — the row exists because the
		// director created it. It matters that this is true: the verifier
		// REFUSES unverified emails (see verifier.go), so importing these as
		// unverified would lock out every migrated user on their first sign-in.
		EmailVerified: true,

		// Suspension travels with the account. The use case checks it again on
		// every sign-in; disabling it here means a suspended user cannot even
		// obtain a token to present.
		Disabled: !user.CanAuthenticate(),

		PasswordHash: passwordHash,
	}, nil
}

// ToSDKUsers converts a batch for the Admin SDK. Kept separate from
// ToImportUser so the decisions above stay testable without SDK types.
func ToSDKUsers(users []ImportUser) []*firebaseauth.UserToImport {
	out := make([]*firebaseauth.UserToImport, 0, len(users))
	for _, u := range users {
		out = append(out, (&firebaseauth.UserToImport{}).
			UID(u.UID).
			Email(u.Email).
			DisplayName(u.DisplayName).
			EmailVerified(u.EmailVerified).
			Disabled(u.Disabled).
			PasswordHash([]byte(u.PasswordHash)))
	}
	return out
}

// BcryptOption is the hash config the import must carry, because our stored
// hashes are bcrypt. Without it the provider would treat the hash as a
// plaintext password.
func BcryptOption() firebaseauth.UserImportOption {
	return firebaseauth.WithHash(hash.Bcrypt{})
}
