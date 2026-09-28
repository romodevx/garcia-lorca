package googleid_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/iam/internal/adapters/googleid"
)

// fakeToolkit answers accounts:signInWithPassword like Identity Platform does.
func fakeToolkit(t *testing.T, status int, body string, seen *map[string]any, query, apiKey *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/accounts:signInWithPassword", r.URL.Path)
		*query = r.URL.RawQuery
		*apiKey = r.Header.Get("X-Goog-Api-Key")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, seen)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestPasswordCheckerAsksTheProvider(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		wantKind glerrors.Kind
	}{
		{"accepted for this address", http.StatusOK, `{"localId":"u1","email":"Owner@Example.com"}`, glerrors.KindUnknown},
		{"accepted for ANOTHER address is refused", http.StatusOK, `{"localId":"u2","email":"someone@else.mx"}`, glerrors.KindUnauthorized},
		{"wrong password", http.StatusBadRequest, `{"error":{"message":"INVALID_PASSWORD"}}`, glerrors.KindUnauthorized},
		{"enumeration-protected refusal", http.StatusBadRequest, `{"error":{"message":"INVALID_LOGIN_CREDENTIALS"}}`, glerrors.KindUnauthorized},
		{"throttled", http.StatusBadRequest, `{"error":{"message":"TOO_MANY_ATTEMPTS_TRY_LATER : slow down"}}`, glerrors.KindUnauthorized},
		{"disabled account", http.StatusBadRequest, `{"error":{"message":"USER_DISABLED"}}`, glerrors.KindUnauthorized},
		{"bad api key is a server fault", http.StatusBadRequest, `{"error":{"message":"API key not valid"}}`, glerrors.KindInternal},
		{"provider down", http.StatusServiceUnavailable, `oops`, glerrors.KindInternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var seen map[string]any
			var query, apiKey string
			srv := fakeToolkit(t, tc.status, tc.body, &seen, &query, &apiKey)
			c := googleid.NewPasswordChecker("web-key", srv.URL, srv.Client())

			err := c.CheckPassword(context.Background(), "owner@example.com", "s3cret")
			if tc.wantKind == glerrors.KindUnknown {
				require.NoError(t, err)
			} else {
				assert.Equal(t, tc.wantKind, glerrors.KindOf(err), "err = %v", err)
			}
			assert.Empty(t, query, "the key never rides in the URL (proxies and logs keep URLs)")
			assert.Equal(t, "web-key", apiKey, "the key travels in the X-Goog-Api-Key header")
			if err != nil {
				assert.NotContains(t, err.Error(), "web-key")
			}
			assert.Equal(t, "owner@example.com", seen["email"])
			assert.Equal(t, "s3cret", seen["password"])
			assert.Equal(t, false, seen["returnSecureToken"], "no session is minted for a re-check")
		})
	}
}

func TestPasswordCheckerWithoutAKeyFailsClosed(t *testing.T) {
	err := googleid.NewPasswordChecker("", "http://127.0.0.1:1", nil).CheckPassword(context.Background(), "a@b.mx", "x")
	assert.Equal(t, glerrors.KindInternal, glerrors.KindOf(err))
}

// A transport failure (http.Client.Do's *url.Error embeds the request URL) must
// never carry the key into an error, a log line or a response.
func TestPasswordCheckerTransportErrorNeverCarriesTheKey(t *testing.T) {
	const key = "AIzaSECRET-web-key"
	err := googleid.NewPasswordChecker(key, "http://127.0.0.1:1", nil).CheckPassword(context.Background(), "a@b.mx", "x")
	require.Error(t, err)
	assert.Equal(t, glerrors.KindInternal, glerrors.KindOf(err))
	assert.NotContains(t, err.Error(), key)
	assert.NotContains(t, glerrors.SafeMessage(err), key)
}
