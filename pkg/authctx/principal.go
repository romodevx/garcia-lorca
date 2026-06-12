package authctx

import "context"

// Principal is the authenticated caller extracted from a verified credential
// (JWT/OIDC at the gateway). It is propagated through context so that any layer
// can read the identity without re-parsing a token.
type Principal struct {
	UserID string
	Roles  []string
}

// ctxKey is an unexported type to avoid context key collisions.
type ctxKey struct{}

// WithPrincipal returns a copy of ctx carrying p.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// PrincipalFrom extracts the Principal from ctx. ok is false when none is set,
// which callers must treat as unauthenticated (fail closed).
func PrincipalFrom(ctx context.Context) (p Principal, ok bool) {
	p, ok = ctx.Value(ctxKey{}).(Principal)
	return p, ok
}
