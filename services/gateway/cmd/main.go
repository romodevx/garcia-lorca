// Command gateway is the entrypoint for the García Lorca BFF / edge gateway.
//
// FASE 0: the gateway boots an HTTP server with its own health endpoint and a
// /readyz that calls the iam service end to end — the gateway half of the
// Sprint-1 pipeline-validation hello-world. The feature-flag middleware
// (internal/middleware) is wired but guards no module routes yet; modules plug
// in as they land.
package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"

	"github.com/garcia-lorca/pkg/featureflag"
	"github.com/garcia-lorca/pkg/logger"
)

const defaultAddr = ":8080"

func main() {
	log := logger.Default()

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = defaultAddr
	}
	iamBaseURL := os.Getenv("IAM_BASE_URL")
	if iamBaseURL == "" {
		iamBaseURL = "http://localhost:8081"
	}

	// flags is the source of truth for module visibility. FASE 0 wires an empty
	// in-memory set (default-deny); a real backend replaces it later.
	flags := featureflag.NewInMemory(map[string]bool{})
	_ = flags // consumed by middleware.NewModuleGuard once modules register routes

	iamClient := client.NewIAMClient(iamBaseURL, http.DefaultClient)
	mux := newMux(iamClient)

	log.Info("gateway starting", slog.String("addr", addr), slog.String("iam", iamBaseURL))
	srv := &http.Server{Addr: addr, Handler: mux}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("gateway server stopped", slog.Any("error", err))
		os.Exit(1)
	}
}

// newMux builds the gateway HTTP handler given an iam pinger. Extracted from
// main so the integration test can assemble the exact production routes against
// a test iam server.
func newMux(iam *client.IAMClient) *http.ServeMux {
	mux := http.NewServeMux()
	handler.NewHealthHandler(iam).Register(mux)
	return mux
}
