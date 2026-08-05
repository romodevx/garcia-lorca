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
	res         client.LoginResult
	err         error
	cpErr       error
	cpGotToken  string
	cpGotCalled bool
}

func (f fakeLogin) Login(context.Context, string, string) (client.LoginResult, error) {
	return f.res, f.err
}

func (f fakeLogin) Refresh(context.Context, string) (client.LoginResult, error) {
	return f.res, f.err
}

func (f *fakeLogin) ChangePassword(_ context.Context, accessToken, _, _ string) error {
	f.cpGotCalled = true
	f.cpGotToken = accessToken
	return f.cpErr
}

// cookieByName returns the Set-Cookie matching name, or nil.
func cookieByName(cs []*http.Cookie, name string) *http.Cookie {
	for _, c := range cs {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestRefreshRotatesBothCookiesNoBodyTokens(t *testing.T) {
	h := handler.NewAuthHandler(&fakeLogin{res: client.LoginResult{AccessToken: "new-acc", RefreshToken: "new-ref", ExpiresIn: 900}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "gl_refresh", Value: "old-ref"})
	h.Refresh(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if _, ok := body["accessToken"]; ok {
		t.Errorf("body must not contain accessToken, got %v", body)
	}
	if _, ok := body["refreshToken"]; ok {
		t.Errorf("body must not contain refreshToken, got %v", body)
	}
	cs := rec.Result().Cookies()
	acc := cookieByName(cs, "gl_access")
	ref := cookieByName(cs, "gl_refresh")
	if acc == nil || acc.Value != "new-acc" || !acc.HttpOnly || acc.Path != "/" {
		t.Errorf("gl_access rotation cookie = %v", acc)
	}
	if ref == nil || ref.Value != "new-ref" || !ref.HttpOnly || ref.Path != "/api/v1/auth" {
		t.Errorf("gl_refresh rotation cookie = %v", ref)
	}
}

func TestRefreshMissingCookieIs401(t *testing.T) {
	h := handler.NewAuthHandler(&fakeLogin{res: client.LoginResult{AccessToken: "a", RefreshToken: "r", ExpiresIn: 900}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	h.Refresh(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", rec.Code)
	}
}

func TestRefreshProxyInvalidIs401(t *testing.T) {
	h := handler.NewAuthHandler(&fakeLogin{err: connect.NewError(connect.CodeUnauthenticated, errString("bad"))})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "gl_refresh", Value: "garbage"})
	h.Refresh(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", rec.Code)
	}
}

func TestLoginProxySuccess(t *testing.T) {
	h := handler.NewAuthHandler(&fakeLogin{res: client.LoginResult{AccessToken: "acc", RefreshToken: "ref", ExpiresIn: 900}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"ana@garcialorca.mx","password":"secret"}`))
	h.Login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if _, ok := body["accessToken"]; ok {
		t.Errorf("body must not contain accessToken, got %v", body)
	}
	if _, ok := body["refreshToken"]; ok {
		t.Errorf("body must not contain refreshToken, got %v", body)
	}
	cs := rec.Result().Cookies()
	if acc := cookieByName(cs, "gl_access"); acc == nil || acc.Value != "acc" {
		t.Errorf("gl_access cookie = %v", acc)
	}
	if ref := cookieByName(cs, "gl_refresh"); ref == nil || ref.Value != "ref" {
		t.Errorf("gl_refresh cookie = %v", ref)
	}
}

func TestLoginProxyBadCredentialsIs401(t *testing.T) {
	h := handler.NewAuthHandler(&fakeLogin{err: connect.NewError(connect.CodeUnauthenticated, errString("bad"))})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"a@b.mx","password":"x"}`))
	h.Login(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", rec.Code)
	}
}

func TestLoginProxyMalformedBodyIs400(t *testing.T) {
	h := handler.NewAuthHandler(&fakeLogin{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`not json`))
	h.Login(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rec.Code)
	}
}

func TestMeReturnsPrincipal(t *testing.T) {
	h := handler.NewAuthHandler(&fakeLogin{})
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
	h := handler.NewAuthHandler(&fakeLogin{})
	rec := httptest.NewRecorder()
	h.Me(rec, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", rec.Code)
	}
}

func TestLoginResponseIncludesMustChange(t *testing.T) {
	h := handler.NewAuthHandler(&fakeLogin{res: client.LoginResult{AccessToken: "a", RefreshToken: "r", ExpiresIn: 900, MustChangePassword: true}})
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
	if _, ok := body["expiresIn"]; !ok {
		t.Errorf("body must contain expiresIn, got %v", body)
	}
	if _, ok := body["accessToken"]; ok {
		t.Errorf("body must not contain accessToken, got %v", body)
	}
	if _, ok := body["refreshToken"]; ok {
		t.Errorf("body must not contain refreshToken, got %v", body)
	}
	cs := rec.Result().Cookies()
	acc := cookieByName(cs, "gl_access")
	if acc == nil || !acc.HttpOnly || acc.Path != "/" {
		t.Errorf("expected HttpOnly gl_access cookie (Path /), got %v", acc)
	}
	ref := cookieByName(cs, "gl_refresh")
	if ref == nil || !ref.HttpOnly || ref.Path != "/api/v1/auth" {
		t.Errorf("expected HttpOnly gl_refresh cookie (Path /api/v1/auth), got %v", ref)
	}
}

func TestChangePasswordSuccessIs204(t *testing.T) {
	h := handler.NewAuthHandler(&fakeLogin{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", strings.NewReader(`{"currentPassword":"old","newPassword":"newlongpass"}`))
	ctx := middleware.WithPrincipal(req.Context(), middleware.Principal{UserID: "u1"})
	h.ChangePassword(rec, req.WithContext(ctx))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code=%d, want 204", rec.Code)
	}
}

func TestChangePasswordForwardsAccessTokenNotUserID(t *testing.T) {
	fake := &fakeLogin{}
	h := handler.NewAuthHandler(fake)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", strings.NewReader(`{"currentPassword":"old","newPassword":"newlongpass"}`))
	req.Header.Set("Authorization", "Bearer caller-access-token")
	ctx := middleware.WithPrincipal(req.Context(), middleware.Principal{UserID: "u1"})
	h.ChangePassword(rec, req.WithContext(ctx))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("code=%d, want 204", rec.Code)
	}
	if !fake.cpGotCalled {
		t.Fatal("ChangePassword was not called")
	}
	if fake.cpGotToken != "caller-access-token" {
		t.Errorf("forwarded token = %q, want the caller's access token (not a user id)", fake.cpGotToken)
	}
}

func TestChangePasswordWithoutPrincipalIs401(t *testing.T) {
	h := handler.NewAuthHandler(&fakeLogin{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", strings.NewReader(`{"currentPassword":"old","newPassword":"newlongpass"}`))
	h.ChangePassword(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code=%d, want 401", rec.Code)
	}
}

func TestChangePasswordMalformedBodyIs400(t *testing.T) {
	h := handler.NewAuthHandler(&fakeLogin{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", strings.NewReader(`not json`))
	ctx := middleware.WithPrincipal(req.Context(), middleware.Principal{UserID: "u1"})
	h.ChangePassword(rec, req.WithContext(ctx))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code=%d, want 400", rec.Code)
	}
}

func TestChangePasswordPropagatesErrorKind(t *testing.T) {
	// A long-enough newPassword clears the edge guard so the request reaches iam,
	// whose InvalidArgument (e.g. a policy rejection) must surface as 400.
	h := handler.NewAuthHandler(&fakeLogin{cpErr: connect.NewError(connect.CodeInvalidArgument, errString("weak"))})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", strings.NewReader(`{"currentPassword":"old","newPassword":"newlongpass"}`))
	ctx := middleware.WithPrincipal(req.Context(), middleware.Principal{UserID: "u1"})
	h.ChangePassword(rec, req.WithContext(ctx))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code=%d, want 400", rec.Code)
	}
}

func TestChangePasswordShortNewPasswordRejectedAtEdge(t *testing.T) {
	// The edge guard must reject a too-short newPassword with 400 BEFORE calling
	// iam (defense-in-depth + avoids a needless round-trip). iam still enforces
	// server-side; this only saves the round-trip.
	fake := &fakeLogin{}
	h := handler.NewAuthHandler(fake)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", strings.NewReader(`{"currentPassword":"old","newPassword":"short"}`))
	ctx := middleware.WithPrincipal(req.Context(), middleware.Principal{UserID: "u1"})
	h.ChangePassword(rec, req.WithContext(ctx))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d, want 400", rec.Code)
	}
	if fake.cpGotCalled {
		t.Error("iam ChangePassword was called; the edge guard must short-circuit before iam")
	}
}

func TestLogoutClearsCookie(t *testing.T) {
	h := handler.NewAuthHandler(&fakeLogin{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	h.Logout(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
	cs := rec.Result().Cookies()
	acc := cookieByName(cs, "gl_access")
	if acc == nil || acc.MaxAge >= 0 || !acc.HttpOnly || acc.Path != "/" {
		t.Errorf("expected HttpOnly gl_access deletion cookie (Path /), got %v", acc)
	}
	ref := cookieByName(cs, "gl_refresh")
	if ref == nil || ref.MaxAge >= 0 || !ref.HttpOnly || ref.Path != "/api/v1/auth" {
		t.Errorf("expected HttpOnly gl_refresh deletion cookie (Path /api/v1/auth), got %v", ref)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
