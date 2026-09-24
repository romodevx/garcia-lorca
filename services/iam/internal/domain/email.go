package domain

import "github.com/garcia-lorca/pkg/email"

// Email is a normalized, validated email address value object. It is compared by
// value (normalization guarantees two spellings of the same address are equal),
// so uniqueness-per-tenant can be enforced on the normalized form.
//
//	"  Ana@Colegio.MX " ──NewEmail──> Email("ana@colegio.mx")
//
// Validation here is a boundary sanity check (shape), not full RFC 5322 — the
// authoritative uniqueness constraint lives at the database level.
type Email string

// NewEmail normalizes (trim + lowercase) and validates an address, returning a
// KindInvalid error when the shape is not a plausible email.
func NewEmail(raw string) (Email, error) {
	normalized := email.Normalize(raw)
	if err := email.Validate(normalized); err != nil {
		return "", err
	}
	return Email(normalized), nil
}

// String returns the normalized address.
func (e Email) String() string { return string(e) }
