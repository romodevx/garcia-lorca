package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// IAMPingPath is the iam route this client calls. It mirrors the iam adapter's
// route; once proto is generated, this hand-written client is replaced by the
// generated Connect client and the constant disappears.
const IAMPingPath = "/garcialorca.iam.v1.HealthService/Ping"

// IAMHealth is the gateway's view of an iam health Ping.
type IAMHealth struct {
	Service string `json:"service"`
	Status  string `json:"status"`
}

// IAMClient calls the internal iam service. In FASE 0 it speaks plain HTTP/JSON;
// the production version will be the generated Connect client over gRPC.
type IAMClient struct {
	baseURL string
	http    *http.Client
}

// NewIAMClient builds a client targeting baseURL (e.g. "http://iam:8080"),
// using httpClient (nil falls back to http.DefaultClient).
func NewIAMClient(baseURL string, httpClient *http.Client) *IAMClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &IAMClient{baseURL: baseURL, http: httpClient}
}

// Ping calls the iam health endpoint and returns its status.
func (c *IAMClient) Ping(ctx context.Context) (IAMHealth, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+IAMPingPath, http.NoBody)
	if err != nil {
		return IAMHealth{}, fmt.Errorf("iam ping: building request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return IAMHealth{}, fmt.Errorf("iam ping: calling iam: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return IAMHealth{}, fmt.Errorf("iam ping: unexpected status %d", resp.StatusCode)
	}

	var out IAMHealth
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return IAMHealth{}, fmt.Errorf("iam ping: decoding response: %w", err)
	}
	return out, nil
}
