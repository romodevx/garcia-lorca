// Package gcpauth builds the HTTP client a service uses to call a sibling
// service.
//
// Internal Cloud Run services deploy with ingress=INTERNAL_ONLY guarded by IAM
// (roles/run.invoker); Cloud Run's frontend then demands a Google-signed OIDC
// ID token whose audience is the target service URL. HTTPClient returns an
// idtoken-authenticated client for https targets and the plain default client
// for local http targets (docker-compose), which are not GCP-authenticated.
package gcpauth

import (
	"context"
	"net/http"
	"net/url"

	"google.golang.org/api/idtoken"
)

// HTTPClient returns the *http.Client a caller should use to reach baseURL.
//
// For an https target it returns a client that attaches a Google OIDC ID token
// (audience = baseURL) to every request. For any other scheme — notably the
// http://service:8080 targets docker-compose uses — it returns
// http.DefaultClient unchanged.
//
// Token-source failures fail soft: if no credentials are available the plain
// default client is returned so local runs and tests never break. The https
// guard means a local http target never even attempts to mint a token; on Cloud
// Run the metadata server always supplies credentials.
func HTTPClient(ctx context.Context, baseURL string) *http.Client {
	if !needsIDToken(baseURL) {
		return http.DefaultClient
	}
	client, err := idtoken.NewClient(ctx, baseURL)
	if err != nil {
		// Deliberate soft-fail: degrade to an unauthenticated client rather than
		// refuse to boot when no credential source is reachable.
		return http.DefaultClient
	}
	return client
}

// needsIDToken reports whether baseURL is an https Cloud Run target that
// requires an OIDC ID token. A parse failure or any non-https scheme (local
// http) selects the unauthenticated path.
func needsIDToken(baseURL string) bool {
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	return u.Scheme == "https"
}
