package middleware_test

import (
	"errors"
	"net/http"
	"testing"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// iam now reads the account's status on every ValidateToken (ISSUE-CC-116), so
// "iam could not answer" became a real answer of its own. It must not read as
// "your session is over": a 401 tells the client to end the session, a 503
// tells it to retry. Only a refusal is a 401.
func TestRequireAuthTellsARefusalFromAnOutage(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"iam refuses the token", connect.NewError(connect.CodeUnauthenticated, errors.New("invalid access token")), http.StatusUnauthorized},
		{"iam failed to read the account", connect.NewError(connect.CodeInternal, errors.New("internal error")), http.StatusServiceUnavailable},
		{"iam is unreachable", connect.NewError(connect.CodeUnavailable, errors.New("connection refused")), http.StatusServiceUnavailable},
		{"iam timed out", connect.NewError(connect.CodeDeadlineExceeded, errors.New("deadline")), http.StatusServiceUnavailable},
		// Anything else stays fail-closed: no principal, no access.
		{"an unexpected refusal", connect.NewError(connect.CodePermissionDenied, errors.New("no")), http.StatusUnauthorized},
		{"a plain error", errors.New("invalid"), http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ran bool
			var seen middleware.Principal
			h := middleware.RequireAuth(&fakeAuth{err: tc.err})(ok200(&seen, &ran))

			rec := do(h, "Bearer tok")

			if rec.Code != tc.want {
				t.Errorf("code = %d, want %d", rec.Code, tc.want)
			}
			if ran {
				t.Error("next handler must NOT run when the token was not validated")
			}
		})
	}
}
