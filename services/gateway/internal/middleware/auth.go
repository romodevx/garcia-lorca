package middleware

import (
	"context"
	"net/http"
	"strings"

	connect "connectrpc.com/connect"
)

// Principal is the authenticated caller as the edge sees it, derived from a
// verified iam access token. The gateway injects it into the request context so
// downstream handlers know who is calling and what they may do.
type Principal struct {
	UserID             string
	TenantID           string
	Email              string
	Permissions        []string
	MustChangePassword bool
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
			token := TokenFrom(r)
			if token == "" {
				unauthorized(w)
				return
			}
			principal, err := auth.ValidateToken(r.Context(), token)
			if isAuthOutage(err) {
				// iam could not answer — it did not refuse. A 401 would tell
				// the client the session is over; 503 tells it to retry.
				http.Error(w, "authentication temporarily unavailable", http.StatusServiceUnavailable)
				return
			}
			if err != nil {
				unauthorized(w)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
		})
	}
}

// RequirePermission wraps next so only a principal holding the given
// "resource:action" permission passes. It MUST run after RequireAuth (it reads
// the injected principal): no principal ⇒ 401 (fail-closed), principal without
// the permission ⇒ 403. This is the authorization boundary — authentication
// proves who you are, this proves you may do it.
func RequirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := PrincipalFrom(r.Context())
			if !ok {
				unauthorized(w)
				return
			}
			if !hasPermission(p, permission) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequirePasswordChanged blocks a principal that still must rotate its temporary
// password from doing anything except changing it. It MUST run after RequireAuth.
// Fail-closed: no principal ⇒ 401. Returns 403 with a stable
// "password_change_required" marker so clients can route to the change-password
// screen.
func RequirePasswordChanged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			unauthorized(w)
			return
		}
		if p.MustChangePassword {
			http.Error(w, "password_change_required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func hasPermission(p Principal, permission string) bool {
	for _, granted := range p.Permissions {
		if granted == permission {
			return true
		}
	}
	return false
}

// HasPermission reports whether p holds permission using the middleware's
// permission matching semantics.
func HasPermission(p Principal, permission string) bool {
	return hasPermission(p, permission)
}

// TokenFrom returns the caller's access token from the Authorization header or
// the gl_access cookie ("" if neither). Shared with RequireAuth's sourcing so the
// change-password handler forwards the exact same token to iam.
func TokenFrom(r *http.Request) string {
	token := bearerToken(r)
	if token == "" {
		if c, err := r.Cookie("gl_access"); err == nil {
			token = c.Value
		}
	}
	return token
}

// bearerToken extracts the token from a request's "Authorization: Bearer <token>"
// header, returning "" when the header is absent or malformed.
func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

// isAuthOutage reports whether ValidateToken failed because iam could not
// answer (it reads the account's status on every call, ISSUE-CC-116) rather
// than because it refused the token. Every other failure stays a 401:
// fail-closed, no principal, no access.
func isAuthOutage(err error) bool {
	switch connect.CodeOf(err) {
	case connect.CodeInternal, connect.CodeUnavailable, connect.CodeDeadlineExceeded:
		return err != nil
	default:
		return false
	}
}

func unauthorized(w http.ResponseWriter) {
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}
