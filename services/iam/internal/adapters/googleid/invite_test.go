package googleid_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/iam/internal/adapters/googleid"
)

// The invitation uses the same web key as the password re-check; it too must
// travel in the X-Goog-Api-Key header, never in the URL.
func TestInviterSendsTheKeyInAHeader(t *testing.T) {
	var query, apiKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/accounts:sendOobCode", r.URL.Path)
		query, apiKey = r.URL.RawQuery, r.Header.Get("X-Goog-Api-Key")
		_, _ = w.Write([]byte(`{"email":"a@b.mx"}`))
	}))
	t.Cleanup(srv.Close)

	require.NoError(t, googleid.NewInviter("web-key", srv.URL, srv.Client()).SendPasswordSetupEmail(context.Background(), "a@b.mx"))
	assert.Empty(t, query)
	assert.Equal(t, "web-key", apiKey)
}

func TestInviterTransportErrorNeverCarriesTheKey(t *testing.T) {
	const key = "AIzaSECRET-web-key"
	err := googleid.NewInviter(key, "http://127.0.0.1:1", nil).SendPasswordSetupEmail(context.Background(), "a@b.mx")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), key)
}
