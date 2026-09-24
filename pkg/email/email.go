// Package email holds the one rule every service stores an email address by:
// trimmed and lowercase, with a plausible shape.
//
// It lives here, and not in each domain, because the rule is only worth
// anything if it is the SAME rule everywhere: iam looks a login up, students
// stores a contact that later becomes that login, config stores a teacher who
// is also a user. If one of them normalized differently, the same person would
// have two spellings. The SQL mirror of Normalize is the *_normalized_email
// function the email migrations create (migrations/iam/0018 and siblings).
package email

import (
	"strings"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// Normalize returns the one spelling an address is stored under: surrounding
// Unicode whitespace removed and lowercase.
//
//	"  Ana@Colegio.MX " → "ana@colegio.mx"
func Normalize(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// Validate reports, as a KindInvalid error, whether a NORMALIZED address lacks a
// plausible shape: no whitespace, exactly one '@' with a non-empty local part,
// and a dotted domain. It is a boundary sanity check, not RFC 5322 — the
// authoritative uniqueness lives in the database.
//
// It is byte-for-byte the rule iam enforced before it moved here, and iam
// re-validates every user it reads: making it stricter would make stored users
// unreadable. Tighten it only together with a migration that fixes the rows.
func Validate(normalized string) error {
	if normalized == "" {
		return glerrors.New(glerrors.KindInvalid, "email is empty")
	}
	if strings.ContainsAny(normalized, " \t\r\n") {
		return glerrors.New(glerrors.KindInvalid, "email contains whitespace")
	}
	at := strings.IndexByte(normalized, '@')
	if at <= 0 || strings.IndexByte(normalized[at+1:], '@') != -1 {
		return glerrors.New(glerrors.KindInvalid, "email must contain a single '@'")
	}
	domainPart := normalized[at+1:]
	dot := strings.IndexByte(domainPart, '.')
	if dot <= 0 || dot == len(domainPart)-1 {
		return glerrors.New(glerrors.KindInvalid, "email domain must contain a dot")
	}
	return nil
}
