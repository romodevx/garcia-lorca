package gcp_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/garcia-lorca/pkg/pubsub"
	"github.com/garcia-lorca/pkg/pubsub/gcp"
)

// pushSAEmail is the push subscription's service account, the one identity the
// hardened endpoint accepts when caller verification is enabled.
const pushSAEmail = "comms-push@garcia-lorca.iam.gserviceaccount.com"

// bearerJWT builds an Authorization header value carrying an OIDC token whose
// payload asserts email. Only the payload matters here: Cloud Run's frontend has
// already verified the signature, so the handler reads the middle segment and
// never checks the (throwaway) header/signature parts.
func bearerJWT(t *testing.T, email string) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, err := json.Marshal(map[string]string{"email": email, "aud": "https://comms"})
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(claims)
	return "Bearer " + header + "." + payload + ".not-a-real-signature"
}

// pushBody builds a valid Cloud Pub/Sub push envelope carrying opaque payload
// bytes and the routing attributes, exactly the shape the platform POSTs.
func pushBody(t *testing.T, data []byte, attrs map[string]string) string {
	t.Helper()
	env := map[string]any{
		"message": map[string]any{
			"data":        base64.StdEncoding.EncodeToString(data),
			"attributes":  attrs,
			"messageId":   "1234567890",
			"publishTime": "2026-08-15T10:00:00Z",
		},
		"subscription": "projects/garcia-lorca/subscriptions/comms-payment-events",
	}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return string(b)
}

// TestPushHandlerDispatchesDecodedMessage is the contract of the push path: the
// base64 data and the attributes it decodes must reach the handler byte-for-byte
// identical to what the pull adapter passes, and a handler that acks (returns
// nil) answers 2xx so Cloud Pub/Sub acks too.
func TestPushHandlerDispatchesDecodedMessage(t *testing.T) {
	wantData := []byte{0x08, 0x96, 0x01, 0x12, 0x03, 'a', 'b', 'c'} // arbitrary proto-ish bytes
	wantAttrs := map[string]string{pubsub.AttrType: "PaymentRegistered", "tenant_id": "t-1"}

	var got pubsub.Message
	called := 0
	h := func(_ context.Context, msg pubsub.Message) error {
		called++
		got = msg
		return nil
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pubsub", strings.NewReader(pushBody(t, wantData, wantAttrs)))
	// Empty expected SA: caller verification is off (the local/emulator path).
	gcp.NewPushHandler(h, "", nil).ServeHTTP(rec, req)

	if called != 1 {
		t.Fatalf("handler called %d times, want exactly 1", called)
	}
	if rec.Code/100 != 2 {
		t.Fatalf("status = %d, want a 2xx (Pub/Sub acks)", rec.Code)
	}
	if string(got.Data) != string(wantData) {
		t.Errorf("Data = %v, want %v (must equal the base64-decoded payload)", got.Data, wantData)
	}
	if got.Attributes[pubsub.AttrType] != "PaymentRegistered" {
		t.Errorf("Attributes[type] = %q, want PaymentRegistered", got.Attributes[pubsub.AttrType])
	}
	if got.Attributes["tenant_id"] != "t-1" {
		t.Errorf("Attributes[tenant_id] = %q, want t-1", got.Attributes["tenant_id"])
	}
}

// TestPushHandlerNacksOnHandlerError pins the redelivery contract: a handler
// error (e.g. the idempotency ledger unreachable) must answer non-2xx so
// Cloud Pub/Sub nacks and redelivers, eventually routing to the DLQ.
func TestPushHandlerNacksOnHandlerError(t *testing.T) {
	h := func(context.Context, pubsub.Message) error {
		return errors.New("ledger unreachable")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pubsub", strings.NewReader(pushBody(t, []byte("x"), nil)))
	gcp.NewPushHandler(h, "", nil).ServeHTTP(rec, req)

	if rec.Code/100 == 2 {
		t.Fatalf("status = %d, want non-2xx so Pub/Sub redelivers", rec.Code)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

// TestPushHandlerRejectsBadBody covers the malformed inputs the platform should
// never send but the handler must not dispatch on: a body that is not the
// envelope JSON, an empty body, and a data field that is not valid base64. Each
// is 400, and the handler is never invoked.
func TestPushHandlerRejectsBadBody(t *testing.T) {
	tests := map[string]string{
		"not json":        "{not-json",
		"empty body":      "",
		"data not base64": `{"message":{"data":"!!!not-base64!!!","attributes":{}},"subscription":"s"}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			called := 0
			h := func(context.Context, pubsub.Message) error {
				called++
				return nil
			}

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/pubsub", strings.NewReader(body))
			gcp.NewPushHandler(h, "", nil).ServeHTTP(rec, req)

			if called != 0 {
				t.Errorf("handler called %d times, want 0 (a malformed body is never dispatched)", called)
			}
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
		})
	}
}

// TestPushHandlerRejectsNonPost proves the endpoint answers only POST: even when
// mounted without method-aware routing the handler must refuse anything else.
func TestPushHandlerRejectsNonPost(t *testing.T) {
	called := 0
	h := func(context.Context, pubsub.Message) error {
		called++
		return nil
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pubsub", nil)
	gcp.NewPushHandler(h, "", nil).ServeHTTP(rec, req)

	if called != 0 {
		t.Errorf("handler called %d times on GET, want 0", called)
	}
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

// TestPushHandlerVerifiesCallerWhenSAConfigured is the defense-in-depth gate:
// with an expected SA email configured, only a request bearing that identity is
// dispatched. run.invoker is coarse (the shared runtime SA holds it on comms
// too), so the handler pins the caller to the push subscription's SA by reading
// the email claim of the OIDC token Cloud Run forwards. The signature is already
// verified upstream, so only the claim is read.
func TestPushHandlerVerifiesCallerWhenSAConfigured(t *testing.T) {
	body := pushBody(t, []byte("x"), map[string]string{pubsub.AttrType: "PaymentRegistered"})

	tests := []struct {
		name         string
		authHeader   string
		wantStatus   int
		wantDispatch bool
	}{
		{"correct email dispatches", bearerJWT(t, pushSAEmail), http.StatusNoContent, true},
		{"wrong email forbidden", bearerJWT(t, "attacker@garcia-lorca.iam.gserviceaccount.com"), http.StatusForbidden, false},
		{"missing header forbidden", "", http.StatusForbidden, false},
		{"garbage token forbidden", "Bearer not.a.jwt", http.StatusForbidden, false},
		{"not a bearer forbidden", "Basic Zm9vOmJhcg==", http.StatusForbidden, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called := 0
			h := func(context.Context, pubsub.Message) error {
				called++
				return nil
			}

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/pubsub", strings.NewReader(body))
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			gcp.NewPushHandler(h, pushSAEmail, nil).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			gotDispatch := called == 1
			if gotDispatch != tc.wantDispatch {
				t.Errorf("dispatched = %v, want %v (an unverified caller must never reach HandleEvent)", gotDispatch, tc.wantDispatch)
			}
		})
	}
}

// TestPushHandlerSkipsVerificationWhenSAEmpty proves the local/emulator path is
// unaffected: with no expected SA, a request with no Authorization header at all
// still dispatches, exactly as before the hardening.
func TestPushHandlerSkipsVerificationWhenSAEmpty(t *testing.T) {
	called := 0
	h := func(context.Context, pubsub.Message) error {
		called++
		return nil
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pubsub", strings.NewReader(pushBody(t, []byte("x"), nil)))
	gcp.NewPushHandler(h, "", nil).ServeHTTP(rec, req)

	if called != 1 {
		t.Errorf("handler called %d times, want 1 (verification is off when no SA is configured)", called)
	}
	if rec.Code/100 != 2 {
		t.Errorf("status = %d, want 2xx", rec.Code)
	}
}

// The push handler is an ordinary http.Handler, so it mounts next to the
// service's other routes.
var _ http.Handler = gcp.NewPushHandler(func(context.Context, pubsub.Message) error { return nil }, "", nil)
