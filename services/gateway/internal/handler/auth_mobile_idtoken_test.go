package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

/*
The mobile half of the exchange (F5).

Same idea as the web route, opposite posture. A React Native client has no
cookie jar, so the pair comes back IN THE BODY and the app owns storage — which
is exactly what /auth/mobile/login already does. What changes is only who checks
the credentials.

The point of routing it through us at all: the session stays ours. Our TTLs, our
rotation, our revocation, and the session-generation guard the app already has.
Letting the app carry the provider's own token instead would hand session
lifetime to the provider and quietly undo all of it.
*/

func TestMobileGoogleLoginReturnsOurTokensInTheBody(t *testing.T) {
	fake := &fakeLogin{res: client.LoginResult{
		AccessToken: "a-token", RefreshToken: "r-token", ExpiresIn: 86400,
	}}
	h := handler.NewAuthHandler(fake)

	rec := httptest.NewRecorder()
	h.MobileGoogleLogin(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/mobile/google",
		strings.NewReader(`{"idToken":"google:madre"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if fake.gotIDToken != "google:madre" {
		t.Errorf("id token forwarded = %q", fake.gotIDToken)
	}

	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["accessToken"] != "a-token" || out["refreshToken"] != "r-token" {
		t.Fatalf("body = %v; the app has no cookie jar, the pair must come back in the body", out)
	}
	// And no cookies: a cookie here would be dead weight the app cannot read.
	if len(rec.Result().Cookies()) != 0 {
		t.Error("cookies were set on the mobile route")
	}
}

// TestMobileGoogleLoginAsksForTheFamilySession. The kind comes from the ROUTE.
// This is a parent's own phone, so the session lasts a day; logging them out
// every quarter of an hour would teach them to stop opening the app.
func TestMobileGoogleLoginAsksForTheFamilySession(t *testing.T) {
	fake := &fakeLogin{res: client.LoginResult{AccessToken: "a", RefreshToken: "r"}}
	h := handler.NewAuthHandler(fake)

	h.MobileGoogleLogin(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost,
		"/api/v1/auth/mobile/google", strings.NewReader(`{"idToken":"google:madre"}`)))

	if fake.gotKind != client.SessionFamily {
		t.Fatalf("session kind = %v, want family", fake.gotKind)
	}
}

// TestMobileGoogleLoginRejectsAnEmptyToken here, without spending a round trip
// on iam to be told the same.
func TestMobileGoogleLoginRejectsAnEmptyToken(t *testing.T) {
	fake := &fakeLogin{}
	h := handler.NewAuthHandler(fake)

	rec := httptest.NewRecorder()
	h.MobileGoogleLogin(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/mobile/google",
		strings.NewReader(`{"idToken":"   "}`)))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if fake.gotIDToken != "" {
		t.Error("iam was called for an empty token")
	}
}

// TestMobileGoogleLoginNeverEchoesTheProviderToken back to the app.
func TestMobileGoogleLoginNeverEchoesTheProviderToken(t *testing.T) {
	fake := &fakeLogin{res: client.LoginResult{AccessToken: "a", RefreshToken: "r"}}
	h := handler.NewAuthHandler(fake)

	rec := httptest.NewRecorder()
	h.MobileGoogleLogin(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/mobile/google",
		strings.NewReader(`{"idToken":"google:madre"}`)))

	if strings.Contains(rec.Body.String(), "google:madre") {
		t.Error("the response echoed the provider's ID token back")
	}
}
