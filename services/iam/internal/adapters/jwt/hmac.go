// Package jwt implements ports.TokenIssuer and ports.TokenValidator using signed
// JWTs (HMAC-SHA256). iam is the only issuer. Access and refresh tokens are
// distinguished by a "typ" claim and are NOT interchangeable — a refresh token
// can never be used as an access token, and vice versa.
package jwt

import (
	"context"
	"errors"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/domain"
	"github.com/garcia-lorca/services/iam/internal/ports"
)

// Compile-time proof the adapter satisfies both token ports.
var (
	_ ports.TokenIssuer    = (*HMAC)(nil)
	_ ports.TokenValidator = (*HMAC)(nil)
)

const (
	typeAccess  = "access"
	typeRefresh = "refresh"
)

// accessClaims is the on-the-wire JWT body for an access token: registered
// claims (sub, exp, iat, iss) plus the principal + effective permissions.
type accessClaims struct {
	gojwt.RegisteredClaims
	TenantID           string   `json:"tid"`
	Email              string   `json:"email"`
	Permissions        []string `json:"perms"`
	MustChangePassword bool     `json:"mcp"`
	TokenType          string   `json:"typ"`
}

// refreshClaims is a minimal token that only carries the subject to re-mint.
type refreshClaims struct {
	gojwt.RegisteredClaims
	TokenType string `json:"typ"`
	// SessionKind decides this token's lifetime, and rides inside it so a
	// rotation gives the new token the SAME life. A session that could ask for
	// a longer one would be a session choosing its own security policy.
	SessionKind string `json:"skind,omitempty"`
}

// HMAC signs and verifies tokens with a shared secret. `now` is injectable so
// expiry is deterministic under test.
type HMAC struct {
	secret     []byte
	issuer     string
	refreshTTL time.Duration
	now        func() time.Time
}

// NewHMAC builds the adapter. `now` defaults to time.Now when nil.
func NewHMAC(secret []byte, issuer string, refreshTTL time.Duration, now func() time.Time) *HMAC {
	if now == nil {
		now = time.Now
	}
	return &HMAC{secret: secret, issuer: issuer, refreshTTL: refreshTTL, now: now}
}

// IssueAccessToken signs the domain claims into an access JWT. The expiry comes
// from the claims (the use case already computed the access TTL).
func (h *HMAC) IssueAccessToken(_ context.Context, claims domain.Claims) (string, error) {
	body := accessClaims{
		RegisteredClaims: gojwt.RegisteredClaims{
			Subject:   claims.Subject,
			Issuer:    h.issuer,
			IssuedAt:  gojwt.NewNumericDate(h.now()),
			ExpiresAt: gojwt.NewNumericDate(claims.ExpiresAt),
		},
		TenantID:           claims.TenantID,
		Email:              claims.Email,
		Permissions:        claims.Permissions,
		MustChangePassword: claims.MustChangePassword,
		TokenType:          typeAccess,
	}
	return h.sign(body)
}

// IssueRefreshToken signs a refresh JWT for userID, expiring after the lifetime
// its session kind allows: fifteen idle minutes for the school's panel, a day
// for a parent's phone.
//
// The kind is in the token, not in this service's configuration, because
// rotating a token has to reproduce the same lifetime and the only thing that
// survives between the two calls is the token itself.
func (h *HMAC) IssueRefreshToken(_ context.Context, userID string, kind domain.SessionKind) (string, error) {
	body := refreshClaims{
		RegisteredClaims: gojwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    h.issuer,
			IssuedAt:  gojwt.NewNumericDate(h.now()),
			ExpiresAt: gojwt.NewNumericDate(h.now().Add(kind.RefreshTTL())),
		},
		TokenType:   typeRefresh,
		SessionKind: kind.String(),
	}
	return h.sign(body)
}

// ValidateAccessToken verifies signature + expiry and returns the domain claims.
// Anything wrong (bad signature, expired, wrong token type) is Unauthorized.
func (h *HMAC) ValidateAccessToken(_ context.Context, token string) (domain.Claims, error) {
	var body accessClaims
	if err := h.parse(token, &body); err != nil {
		return domain.Claims{}, err
	}
	if body.TokenType != typeAccess {
		return domain.Claims{}, glerrors.New(glerrors.KindUnauthorized, "not an access token")
	}
	expiresAt := time.Time{}
	if body.ExpiresAt != nil {
		expiresAt = body.ExpiresAt.Time
	}
	return domain.Claims{
		Subject:            body.Subject,
		TenantID:           body.TenantID,
		Email:              body.Email,
		Permissions:        body.Permissions,
		MustChangePassword: body.MustChangePassword,
		ExpiresAt:          expiresAt,
	}, nil
}

// ParseRefreshToken verifies a refresh token and returns its subject (user id)
// and issued-at instant. The issued-at lets the use case revoke tokens minted
// before the user's last password change.
func (h *HMAC) ParseRefreshToken(_ context.Context, token string) (string, time.Time, domain.SessionKind, error) {
	var body refreshClaims
	if err := h.parse(token, &body); err != nil {
		return "", time.Time{}, domain.SessionStaff, err
	}
	if body.TokenType != typeRefresh {
		return "", time.Time{}, domain.SessionStaff, glerrors.New(glerrors.KindUnauthorized, "not a refresh token")
	}
	issuedAt := time.Time{}
	if body.IssuedAt != nil {
		issuedAt = body.IssuedAt.Time
	}
	// A token minted before kinds existed carries none, and reads as staff —
	// the shorter life. Erring the other way would leave an office session open
	// for a day on the strength of an old cookie.
	return body.Subject, issuedAt, domain.ParseSessionKind(body.SessionKind), nil
}

func (h *HMAC) sign(claims gojwt.Claims) (string, error) {
	token := gojwt.NewWithClaims(gojwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(h.secret)
	if err != nil {
		return "", glerrors.Wrap(glerrors.KindInternal, "signing token", err)
	}
	return signed, nil
}

// parse verifies the signature (HS256 only — no algorithm confusion) and expiry
// using the injected clock. All failures map to KindUnauthorized.
func (h *HMAC) parse(token string, into gojwt.Claims) error {
	_, err := gojwt.ParseWithClaims(
		token,
		into,
		func(*gojwt.Token) (any, error) { return h.secret, nil },
		gojwt.WithValidMethods([]string{"HS256"}),
		gojwt.WithTimeFunc(h.now),
	)
	if err != nil {
		return glerrors.Wrap(glerrors.KindUnauthorized, tokenErrorMessage(err), err)
	}
	return nil
}

func tokenErrorMessage(err error) string {
	switch {
	case errors.Is(err, gojwt.ErrTokenExpired):
		return "token expired"
	case errors.Is(err, gojwt.ErrTokenSignatureInvalid):
		return "invalid token signature"
	default:
		return "invalid token"
	}
}
