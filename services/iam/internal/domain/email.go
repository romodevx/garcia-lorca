package domain

import (
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

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
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if err := validateEmailShape(normalized); err != nil {
		return "", err
	}
	return Email(normalized), nil
}

// String returns the normalized address.
func (e Email) String() string { return string(e) }

func validateEmailShape(value string) error {
	if value == "" {
		return glerrors.New(glerrors.KindInvalid, "email is empty")
	}
	if strings.ContainsAny(value, " \t\r\n") {
		return glerrors.New(glerrors.KindInvalid, "email contains whitespace")
	}
	at := strings.IndexByte(value, '@')
	// Exactly one '@', with a non-empty local part and a dotted domain.
	if at <= 0 || strings.IndexByte(value[at+1:], '@') != -1 {
		return glerrors.New(glerrors.KindInvalid, "email must contain a single '@'")
	}
	domainPart := value[at+1:]
	dot := strings.IndexByte(domainPart, '.')
	if dot <= 0 || dot == len(domainPart)-1 {
		return glerrors.New(glerrors.KindInvalid, "email domain must contain a dot")
	}
	return nil
}
