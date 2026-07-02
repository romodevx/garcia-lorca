package middleware

import (
	"context"
	"net/http"
	"strings"
)

// Principal is the authenticated caller as the edge sees it, derived from a
// verified iam access token. The gateway injects it into the request context so
// downstream handlers know who is calling and what they may do.
type Principal struct {
	UserID      string
	TenantID    string
	Email       string
	Permissions []string
}

// Authenticator verifies an access token and returns its principal. The gateway
// implements this by calling iam's ValidateToken (consumer-defined interface, so
// the middleware is testable with a fake).
type Authenticator interface {
	ValidateToken(ctx context.Context, accessToken string) (Principal, error)
}

type principalCtxKey struct{}

// WithPrincipal returns a copy of ctx carrying p.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalCtxKey{}, p)
}

// PrincipalFrom extracts the Principal from ctx; ok is false when unauthenticated.
func PrincipalFrom(ctx context.Context) (p Principal, ok bool) {
	p, ok = ctx.Value(principalCtxKey{}).(Principal)
	return p, ok
}

// RequireAuth wraps next so only requests bearing a valid iam access token pass.
// A missing/malformed Authorization header or an invalid token yields 401; a
// valid token injects the Principal and calls next. This is the authentication
// boundary — fail-closed.
func RequireAuth(auth Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				unauthorized(w)
				return
			}
			principal, err := auth.ValidateToken(r.Context(), token)
			if err != nil {
				unauthorized(w)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
		})
	}
}

// bearerToken extracts the token from an "Authorization: Bearer <token>" header.
func bearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	return token, token != ""
}

func unauthorized(w http.ResponseWriter) {
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}
