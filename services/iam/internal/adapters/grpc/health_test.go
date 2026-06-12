package grpcadapter_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	grpcadapter "github.com/garcia-lorca/services/iam/internal/adapters/grpc"
	"github.com/garcia-lorca/services/iam/internal/usecase"
)

// TestHealthHandlerPing verifies iam's real health handler serves the contract
// the gateway depends on. Paired with the gateway-side integration test, this
// covers both ends of the Sprint-1 pipeline-validation path.
func TestHealthHandlerPing(t *testing.T) {
	mux := http.NewServeMux()
	grpcadapter.NewHealthHandler(usecase.NewHealthService("iam")).Register(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, grpcadapter.PingPath, nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("Ping status = %d, want 200", rr.Code)
	}

	var got grpcadapter.PingResponse
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decoding ping: %v", err)
	}
	if got.Service != "iam" || got.Status != "SERVING" {
		t.Fatalf("ping = %+v, want {iam SERVING}", got)
	}
}

func TestHealthzOK(t *testing.T) {
	mux := http.NewServeMux()
	grpcadapter.NewHealthHandler(usecase.NewHealthService("iam")).Register(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("healthz = %d, want 200", rr.Code)
	}
}
