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
