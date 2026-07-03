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
	"github.com/garcia-lorca/services/gateway/internal/config"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"

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

	// flags is the authoritative source of module visibility (default-deny),
	// loaded from ENABLED_MODULES (comma-separated module names). The gateway
	// both enforces the boundary (ModuleGuard ⇒ 404) and reports it (GET
	// /features). Each module wires its routes behind the guard as it lands.
	flags := featureflag.NewInMemory(config.ParseEnabledModules(os.Getenv("ENABLED_MODULES")))

	iamHealth := client.NewIAMClient(iamBaseURL, http.DefaultClient)
	authClient := client.NewAuthClient(iamBaseURL, http.DefaultClient)
	mux := newMux(iamHealth, authClient, flags)

	log.Info("gateway starting", slog.String("addr", addr), slog.String("iam", iamBaseURL))
	srv := &http.Server{Addr: addr, Handler: mux}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("gateway server stopped", slog.Any("error", err))
		os.Exit(1)
	}
}

// newMux builds the gateway HTTP handler. Extracted from main so an integration
// test can assemble the exact production routes against a test iam server.
//
//	POST /api/v1/auth/login    → proxy to iam Login (unauthenticated)
//	POST /api/v1/auth/refresh  → proxy to iam RefreshToken
//	GET  /api/v1/me            → RequireAuth → the caller's principal
//	GET  /api/v1/features      → resolved module-flag map (frontend gating)
//	/healthz, /readyz          → liveness + iam reachability
//
// `flags` (via middleware.NewModuleGuard) is the module-visibility boundary used
// by module routes as they land (disabled module ⇒ 404).
func newMux(iamHealth *client.IAMClient, authClient *client.AuthClient, flags featureflag.Checker) *http.ServeMux {
	mux := http.NewServeMux()
	handler.NewHealthHandler(iamHealth).Register(mux)

	authH := handler.NewAuthHandler(authClient)
	requireAuth := middleware.RequireAuth(authClient)

	mux.HandleFunc("POST /api/v1/auth/login", authH.Login)
	mux.HandleFunc("POST /api/v1/auth/refresh", authH.Refresh)
	mux.Handle("GET /api/v1/me", requireAuth(http.HandlerFunc(authH.Me)))
	mux.HandleFunc("GET /api/v1/features", handler.NewFeaturesHandler(flags).Features)
	return mux
}
