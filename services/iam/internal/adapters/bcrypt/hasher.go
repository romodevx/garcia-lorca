// Package bcrypt implements ports.PasswordHasher using the bcrypt adaptive KDF
// (golang.org/x/crypto/bcrypt). It is the only place the application touches
// password crypto; the domain never sees plaintext or hashes.
package bcrypt

import (
	"errors"

	"golang.org/x/crypto/bcrypt"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/ports"
)

// Compile-time proof the adapter satisfies the port.
var _ ports.PasswordHasher = (*Hasher)(nil)

// Hasher hashes and verifies passwords with bcrypt at a configurable cost.
type Hasher struct {
	cost int
}

// NewHasher returns a Hasher at the given cost. A cost of 0 (or out of range)
// uses bcrypt's default cost.
func NewHasher(cost int) *Hasher {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		cost = bcrypt.DefaultCost
	}
	return &Hasher{cost: cost}
}

// Hash returns the bcrypt hash of plain. bcrypt rejects inputs longer than 72
// bytes; that surfaces as a KindInvalid error rather than a silent truncation.
func (h *Hasher) Hash(plain string) (string, error) {
	digest, err := bcrypt.GenerateFromPassword([]byte(plain), h.cost)
	if err != nil {
		if errors.Is(err, bcrypt.ErrPasswordTooLong) {
			return "", glerrors.Wrap(glerrors.KindInvalid, "password too long", err)
		}
		return "", glerrors.Wrap(glerrors.KindInternal, "hashing password", err)
	}
	return string(digest), nil
}

// Verify returns nil when plain matches hash, KindUnauthorized on mismatch, and
// KindInternal for a malformed/unreadable hash.
func (h *Hasher) Verify(hash, plain string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
	if err == nil {
		return nil
	}
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return glerrors.Wrap(glerrors.KindUnauthorized, "invalid credentials", err)
	}
	return glerrors.Wrap(glerrors.KindInternal, "verifying password", err)
}
