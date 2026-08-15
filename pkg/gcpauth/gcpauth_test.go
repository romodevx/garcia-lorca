package gcpauth

import (
	"context"
	"net/http"
	"testing"
)

func TestNeedsIDTokenSelection(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    bool
	}{
		{"cloud run https target", "https://iam-abc123-uc.a.run.app", true},
		{"https with path", "https://iam-abc123-uc.a.run.app/", true},
		{"local http compose target", "http://iam:8080", false},
		{"local http localhost", "http://localhost:8081", false},
		{"empty", "", false},
		{"garbage", "://not a url", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsIDToken(tt.baseURL); got != tt.want {
				t.Fatalf("needsIDToken(%q) = %v, want %v", tt.baseURL, got, tt.want)
			}
		})
	}
}

func TestHTTPClientLocalReturnsDefault(t *testing.T) {
	// A local http target must never attempt to mint a token; it returns the
	// shared default client unchanged.
	got := HTTPClient(context.Background(), "http://localhost:8081")
	if got != http.DefaultClient {
		t.Fatalf("local http target should yield http.DefaultClient, got %p", got)
	}
}
