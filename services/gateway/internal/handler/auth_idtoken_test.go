package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

/*
The exchange endpoint (F4 of .claude/docs/PLAN-identity-platform.md).

This is the route that keeps the migration from costing us the thing the login
does best today. The browser authenticates against Identity Platform and holds
an ID token — which its SDK keeps in IndexedDB, readable by any script on the
page. Handing that token straight to the API would make an XSS a session theft.

So it comes here ONCE, and goes back out as the same HttpOnly cookies the
password login already sets: unreadable to JavaScript, Secure, SameSite=Strict.
The provider verifies WHO you are; the session stays ours.
*/

func TestGoogleLoginSetsOurHttpOnlyCookies(t *testing.T) {
	fake := &fakeLogin{res: client.LoginResult{
		AccessToken: "a-token", RefreshToken: "r-token", ExpiresIn: 900,
	}}
	h := handler.NewAuthHandler(fake)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/google",
		strings.NewReader(`{"idToken":"google:ana"}`))
	rec := httptest.NewRecorder()
	h.GoogleLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if fake.gotIDToken != "google:ana" {
		t.Errorf("id token forwarded = %q", fake.gotIDToken)
	}

	// The point of the whole endpoint.
	access := cookieByName(rec.Result().Cookies(), "gl_access")
	if access == nil {
		t.Fatal("no access cookie was set; the ID token must be exchanged for OUR session")
	}
	if !access.HttpOnly {
		t.Error("the access cookie is readable by JavaScript — an XSS would take the session")
	}
	if !access.Secure || access.SameSite != http.SameSiteStrictMode {
		t.Error("the access cookie lost Secure/SameSite=Strict")
	}

	// And the provider's token is never echoed back: the browser has no reason
	// to receive it again, and a response body carrying it would be one more
	// copy for a script to find.
	if strings.Contains(rec.Body.String(), "google:ana") {
		t.Error("the response echoed the provider's ID token back to the browser")
	}
}

func TestGoogleLoginUsesTheStaffSessionKind(t *testing.T) {
	fake := &fakeLogin{res: client.LoginResult{AccessToken: "a", RefreshToken: "r"}}
	h := handler.NewAuthHandler(fake)

	h.GoogleLogin(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost,
		"/api/v1/auth/google", strings.NewReader(`{"idToken":"google:ana"}`)))

	// The kind belongs to the ROUTE, not to the body. This is the panel's route,
	// so it is a staff session — fifteen idle minutes, not the family app's day.
	if fake.gotKind != client.SessionStaff {
		t.Errorf("session kind = %q, want staff", fake.gotKind)
	}
}

func TestGoogleLoginRejectsAnEmptyToken(t *testing.T) {
	fake := &fakeLogin{res: client.LoginResult{AccessToken: "a", RefreshToken: "r"}}
	h := handler.NewAuthHandler(fake)

	rec := httptest.NewRecorder()
	h.GoogleLogin(rec, httptest.NewRequest(http.MethodPost,
		"/api/v1/auth/google", strings.NewReader(`{"idToken":""}`)))

	// Refuse before spending a round trip on iam, and never set a cookie.
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if cookieByName(rec.Result().Cookies(), "gl_access") != nil {
		t.Error("a cookie was set for an empty token")
	}
}
