// Command iam is the entrypoint for the García Lorca identity & access service.
//
// FASE 0: iam boots an HTTP server exposing a health endpoint and a Ping RPC
// (hand-written JSON in adapters/grpc until the proto contract is generated).
// This is the iam half of the Sprint-1 "validate the deploy pipeline before
// business logic" hello-world: the gateway calls this Ping end to end.
package main

import (
	"log/slog"
	"net/http"
	"os"

	grpcadapter "github.com/garcia-lorca/services/iam/internal/adapters/grpc"
	"github.com/garcia-lorca/services/iam/internal/usecase"

	"github.com/garcia-lorca/pkg/logger"
)

const (
	serviceName = "iam"
	defaultAddr = ":8080"
)

func main() {
	log := logger.Default()

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = defaultAddr
	}

	mux := newMux()

	log.Info("iam starting", slog.String("addr", addr))
	srv := &http.Server{Addr: addr, Handler: mux}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("iam server stopped", slog.Any("error", err))
		os.Exit(1)
	}
}

// newMux builds the iam HTTP handler. It is extracted from main so the
// gateway<->iam integration test can mount the exact production routes.
func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	health := grpcadapter.NewHealthHandler(usecase.NewHealthService(serviceName))
	health.Register(mux)
	return mux
}
