package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

type fakeLogin struct {
	res   client.LoginResult
	err   error
	cpErr error
}

func (f fakeLogin) Login(context.Context, string, string) (client.LoginResult, error) {
	return f.res, f.err
}

func (f fakeLogin) Refresh(context.Context, string) (client.LoginResult, error) {
	return f.res, f.err
}

func (f fakeLogin) ChangePassword(context.Context, string, string, string) error { return f.cpErr }

func TestRefreshProxySuccess(t *testing.T) {
	h := handler.NewAuthHandler(fakeLogin{res: client.LoginResult{AccessToken: "new-acc", RefreshToken: "new-ref", ExpiresIn: 900}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(`{"refreshToken":"old-ref"}`))
	h.Refresh(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["accessToken"] != "new-acc" || body["refreshToken"] != "new-ref" {
		t.Errorf("body = %v", body)
	}
}

func TestRefreshProxyInvalidIs401(t *testing.T) {
	h := handler.NewAuthHandler(fakeLogin{err: connect.NewError(connect.CodeUnauthenticated, errString("bad"))})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(`{"refreshToken":"garbage"}`))
	h.Refresh(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", rec.Code)
	}
}

func TestLoginProxySuccess(t *testing.T) {
	h := handler.NewAuthHandler(fakeLogin{res: client.LoginResult{AccessToken: "acc", RefreshToken: "ref", ExpiresIn: 900}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"ana@garcialorca.mx","password":"secret"}`))
	h.Login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["accessToken"] != "acc" || body["refreshToken"] != "ref" {
		t.Errorf("body = %v", body)
	}
}

func TestLoginProxyBadCredentialsIs401(t *testing.T) {
	h := handler.NewAuthHandler(fakeLogin{err: connect.NewError(connect.CodeUnauthenticated, errString("bad"))})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"a@b.mx","password":"x"}`))
	h.Login(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", rec.Code)
	}
}

func TestLoginProxyMalformedBodyIs400(t *testing.T) {
	h := handler.NewAuthHandler(fakeLogin{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`not json`))
	h.Login(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rec.Code)
	}
}

func TestMeReturnsPrincipal(t *testing.T) {
	h := handler.NewAuthHandler(fakeLogin{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	ctx := middleware.WithPrincipal(req.Context(), middleware.Principal{UserID: "u1", Email: "ana@garcialorca.mx", Permissions: []string{"students:read"}})
	h.Me(rec, req.WithContext(ctx))

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["userId"] != "u1" || body["email"] != "ana@garcialorca.mx" {
		t.Errorf("body = %v", body)
	}
}

func TestMeWithoutPrincipalIs401(t *testing.T) {
	h := handler.NewAuthHandler(fakeLogin{})
	rec := httptest.NewRecorder()
	h.Me(rec, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", rec.Code)
	}
}

func TestLoginResponseIncludesMustChange(t *testing.T) {
	h := handler.NewAuthHandler(fakeLogin{res: client.LoginResult{AccessToken: "a", RefreshToken: "r", ExpiresIn: 900, MustChangePassword: true}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"d@x","password":"p"}`))
	h.Login(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["mustChangePassword"] != true {
		t.Errorf("mustChangePassword=%v, want true", body["mustChangePassword"])
	}
	if c := rec.Result().Cookies(); len(c) == 0 || c[0].Name != "gl_access" || !c[0].HttpOnly {
		t.Errorf("expected HttpOnly gl_access cookie, got %v", rec.Result().Cookies())
	}
}

func TestChangePasswordSuccessIs204(t *testing.T) {
	h := handler.NewAuthHandler(fakeLogin{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", strings.NewReader(`{"currentPassword":"old","newPassword":"newlongpass"}`))
	ctx := middleware.WithPrincipal(req.Context(), middleware.Principal{UserID: "u1"})
	h.ChangePassword(rec, req.WithContext(ctx))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code=%d, want 204", rec.Code)
	}
}

func TestChangePasswordWithoutPrincipalIs401(t *testing.T) {
	h := handler.NewAuthHandler(fakeLogin{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", strings.NewReader(`{"currentPassword":"old","newPassword":"newlongpass"}`))
	h.ChangePassword(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code=%d, want 401", rec.Code)
	}
}

func TestChangePasswordMalformedBodyIs400(t *testing.T) {
	h := handler.NewAuthHandler(fakeLogin{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", strings.NewReader(`not json`))
	ctx := middleware.WithPrincipal(req.Context(), middleware.Principal{UserID: "u1"})
	h.ChangePassword(rec, req.WithContext(ctx))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code=%d, want 400", rec.Code)
	}
}

func TestChangePasswordPropagatesErrorKind(t *testing.T) {
	h := handler.NewAuthHandler(fakeLogin{cpErr: connect.NewError(connect.CodeInvalidArgument, errString("weak"))})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", strings.NewReader(`{"currentPassword":"old","newPassword":"x"}`))
	ctx := middleware.WithPrincipal(req.Context(), middleware.Principal{UserID: "u1"})
	h.ChangePassword(rec, req.WithContext(ctx))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code=%d, want 400", rec.Code)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
