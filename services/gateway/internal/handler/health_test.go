package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// fakeIAM lets the unit tests drive the iam dependency without a network.
type fakeIAM struct {
	health client.IAMHealth
	err    error
}

func (f fakeIAM) Ping(context.Context) (client.IAMHealth, error) {
	return f.health, f.err
}

func TestReadyzReportsIAMFailureAs503(t *testing.T) {
	// NewHealthHandler accepts the iamPinger interface; fakeIAM satisfies it.
	h := handler.NewHealthHandler(fakeIAM{err: errors.New("dial tcp: refused")})
	mux := http.NewServeMux()
	h.Register(mux)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz with unreachable iam = %d, want 503", rr.Code)
	}
}

func TestHealthzAlwaysOK(t *testing.T) {
	h := handler.NewHealthHandler(fakeIAM{})
	mux := http.NewServeMux()
	h.Register(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("healthz = %d, want 200", rr.Code)
	}
}
