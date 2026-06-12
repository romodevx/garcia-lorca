// Command billing is the entrypoint for the García Lorca billing service.
//
// FUTURE / post-MVP — NOT IMPLEMENTED.
//
// billing (Mexican CFDI / IEDU electronic invoicing) is intentionally out of the
// MVP scope. It exists only as a reserved frontier: a proto stub, a feature flag
// that stays OFF, and this skeleton module so the bounded context has a home and
// can be filled in later without reshaping the monorepo. Do not add business
// logic here until billing is formally pulled into a sprint.
package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/garcia-lorca/pkg/logger"
)

const defaultAddr = ":8080"

func main() {
	log := logger.Default()
	log.Warn("billing is post-MVP and not implemented; serving health only")

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = defaultAddr
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	log.Info("billing starting", slog.String("addr", addr))
	srv := &http.Server{Addr: addr, Handler: mux}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("billing server stopped", slog.Any("error", err))
		os.Exit(1)
	}
}
