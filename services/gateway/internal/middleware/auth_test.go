package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

type fakeAuth struct {
	principal middleware.Principal
	err       error
	gotToken  string
}

func (f *fakeAuth) ValidateToken(_ context.Context, token string) (middleware.Principal, error) {
	f.gotToken = token
	return f.principal, f.err
}

// ok200 records whether next ran and what principal it saw.
func ok200(seen *middleware.Principal, ran *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*ran = true
		if p, ok := middleware.PrincipalFrom(r.Context()); ok {
			*seen = p
		}
		w.WriteHeader(http.StatusOK)
	})
}

func do(h http.Handler, authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRequireAuthValidToken(t *testing.T) {
	auth := &fakeAuth{principal: middleware.Principal{UserID: "u1", Permissions: []string{"students:read"}}}
	var seen middleware.Principal
	var ran bool
	h := middleware.RequireAuth(auth)(ok200(&seen, &ran))

	rec := do(h, "Bearer good-token")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if !ran {
		t.Fatal("next handler should have run")
	}
	if auth.gotToken != "good-token" {
		t.Errorf("token passed = %q, want good-token", auth.gotToken)
	}
	if seen.UserID != "u1" {
		t.Errorf("principal not injected: %+v", seen)
	}
}

func TestRequireAuthRejects(t *testing.T) {
	cases := []struct {
		name   string
		header string
		err    error
	}{
		{"no header", "", nil},
		{"not bearer", "Basic abc", nil},
		{"empty bearer", "Bearer ", nil},
		{"invalid token", "Bearer bad", errors.New("invalid")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ran bool
			var seen middleware.Principal
			h := middleware.RequireAuth(&fakeAuth{err: tc.err})(ok200(&seen, &ran))
			rec := do(h, tc.header)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("code = %d, want 401", rec.Code)
			}
			if ran {
				t.Error("next handler must NOT run on auth failure")
			}
		})
	}
}
