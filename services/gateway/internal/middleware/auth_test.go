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

func TestRequirePermission(t *testing.T) {
	cases := []struct {
		name  string
		perms []string
		want  int
	}{
		{"has permission → 200", []string{"students:read", "finance:read"}, http.StatusOK},
		{"lacks permission → 403", []string{"finance:read"}, http.StatusForbidden},
		{"no permissions → 403", nil, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ran bool
			var seen middleware.Principal
			h := middleware.RequirePermission("students:read")(ok200(&seen, &ran))

			req := httptest.NewRequest(http.MethodGet, "/api/v1/students", nil)
			ctx := middleware.WithPrincipal(req.Context(), middleware.Principal{UserID: "u1", Permissions: tc.perms})
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req.WithContext(ctx))

			if rec.Code != tc.want {
				t.Errorf("code = %d, want %d", rec.Code, tc.want)
			}
			if (rec.Code == http.StatusOK) != ran {
				t.Errorf("next ran=%v but code=%d", ran, rec.Code)
			}
		})
	}
}

func TestRequirePermissionWithoutPrincipalIs401(t *testing.T) {
	var ran bool
	var seen middleware.Principal
	h := middleware.RequirePermission("students:read")(ok200(&seen, &ran))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/students", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401 (fail-closed)", rec.Code)
	}
	if ran {
		t.Error("next must not run without a principal")
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
