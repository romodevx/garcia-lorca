package domain

import "time"

// Claims is the verified content of an access token: the principal plus the
// effective permission set gateways/services authorize against. It is a pure
// value — the cryptographic signing/verification lives in an adapter (the
// TokenIssuer/TokenValidator ports), never in the domain.
//
// Permissions are canonical "resource:action" strings so the value is
// transport-friendly (it maps 1:1 to the proto Claims message).
type Claims struct {
	Subject            string // user id
	TenantID           string
	Email              string
	Permissions        []string
	MustChangePassword bool
	ExpiresAt          time.Time
	// IssuedAt is the token's `iat`, set when a token is VALIDATED (the issuer
	// stamps its own clock when signing). Zero when the token carries none,
	// which every floor check treats as older than any floor.
	IssuedAt time.Time
}

// RefreshClaims is the verified content of a refresh token: whose session it
// is, when it was minted, its kind (which fixes its lifetime) and the
// credential version it was minted under. A token minted before versions
// existed reads as version 0.
type RefreshClaims struct {
	UserID            string
	IssuedAt          time.Time
	Kind              SessionKind
	CredentialVersion int64
}

// RevokedFor reports whether a credential rotation of user revoked this token:
// it was minted under an older credential version (exact, CC-302), or before
// the last rotation instant at whole-second granularity (ADR 0005; it still
// guards the tokens minted before versions existed). A token minted in the
// same second right AFTER the change carries the new version and survives.
func (c RefreshClaims) RevokedFor(user User) bool {
	if c.CredentialVersion != user.CredentialVersion() {
		return true
	}
	return c.IssuedAt.Before(user.PasswordChangedAt().Truncate(time.Second))
}

// NewAccessClaims builds the claims for a user's access token, expiring ttl
// after now. Permissions are the user's effective (deduplicated, union) set.
func NewAccessClaims(user User, now time.Time, ttl time.Duration) Claims {
	effective := user.EffectivePermissions()
	permissions := make([]string, len(effective))
	for i, perm := range effective {
		permissions[i] = perm.String()
	}
	return Claims{
		Subject:            user.ID(),
		TenantID:           user.TenantID(),
		Email:              user.Email().String(),
		Permissions:        permissions,
		MustChangePassword: user.MustChangePassword(),
		ExpiresAt:          now.Add(ttl),
	}
}

// IsExpiredAt reports whether the token is expired at instant t.
func (c Claims) IsExpiredAt(t time.Time) bool {
	return !t.Before(c.ExpiresAt)
}

// Allows reports whether the claims include the given "resource:action"
// permission string.
func (c Claims) Allows(permission string) bool {
	for _, p := range c.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}
