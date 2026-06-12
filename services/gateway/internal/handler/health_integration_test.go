package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// TestGatewayToIAMHealthEndToEnd is the Sprint-1 pipeline-validation artifact.
//
// It stands up an httptest server that serves the iam health contract on the
// exact route the gateway client targets (client.IAMPingPath), points the
// gateway's real iam client at it, mounts the gateway's real readyz handler, and
// asserts the aggregate readiness reflects iam's status. This exercises the full
// gateway -> HTTP -> iam wire path with no mocks of the gateway code itself.
//
// We model iam by its wire contract rather than importing the iam service: in
// production the gateway only ever reaches iam over the network (Connect/gRPC),
// never by importing its internals — and Go's internal-package rule enforces
// exactly that boundary. The iam handler is verified inside the iam module.
func TestGatewayToIAMHealthEndToEnd(t *testing.T) {
	// 1. iam stand-in: serves the health contract on the agreed route.
	iamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != client.IAMPingPath {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(client.IAMHealth{Service: "iam", Status: "SERVING"})
	}))
	defer iamServer.Close()

	// 2. Gateway, wired to call the iam server through its real client.
	iamClient := client.NewIAMClient(iamServer.URL, iamServer.Client())
	gwMux := http.NewServeMux()
	handler.NewHealthHandler(iamClient).Register(gwMux)
	gwServer := httptest.NewServer(gwMux)
	defer gwServer.Close()

	// 3. Call the gateway's aggregate readiness end to end.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, gwServer.URL+"/readyz", http.NoBody)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	resp, err := gwServer.Client().Do(req)
	if err != nil {
		t.Fatalf("calling gateway: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("gateway /readyz status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		Gateway string `json:"gateway"`
		IAM     struct {
			Service string `json:"service"`
			Status  string `json:"status"`
		} `json:"iam"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding gateway response: %v", err)
	}

	if body.Gateway != "SERVING" {
		t.Errorf("gateway status = %q, want SERVING", body.Gateway)
	}
	if body.IAM.Service != "iam" || body.IAM.Status != "SERVING" {
		t.Errorf("iam health = %+v, want {iam SERVING}", body.IAM)
	}
}
