package middleware

import (
	"net/http"

	"github.com/garcia-lorca/pkg/featureflag"
)

// ModuleGuard is the server-side feature-flag boundary. It wraps a module's
// handler so that, when the module's flag is disabled, the routes are invisible:
// requests get a 404, exactly as if the module did not exist. This is a security
// boundary (default-deny via featureflag.Checker), not a UX nicety — the
// frontend's own flag checks are cosmetic.
//
// FASE 0: this is the middleware skeleton wired against pkg/featureflag and
// covered by a test. Per-module route registration is added as each module
// lands.
type ModuleGuard struct {
	flags featureflag.Checker
}

// NewModuleGuard builds a guard backed by a flag Checker.
func NewModuleGuard(flags featureflag.Checker) *ModuleGuard {
	return &ModuleGuard{flags: flags}
}

// Protect wraps next so it only serves when the flag for module is enabled.
// A disabled (or unregistered) module yields http.StatusNotFound.
func (g *ModuleGuard) Protect(module string, next http.Handler) http.Handler {
	key := featureflag.Key(module)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !g.flags.Enabled(r.Context(), key) {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
