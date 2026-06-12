// Command scholarships is the entrypoint for the García Lorca scholarships service.
//
// FASE 0: this is a skeleton that boots a minimal HTTP server exposing a
// health endpoint, enough to validate the build/deploy pipeline. Real RPC
// handlers (generated from proto/ via buf) and wiring are added per the module
// workflow: contract -> data+RLS -> backend (TDD) -> flag.
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

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = defaultAddr
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	log.Info("scholarships starting", slog.String("addr", addr))
	srv := &http.Server{Addr: addr, Handler: mux}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("scholarships server stopped", slog.Any("error", err))
		os.Exit(1)
	}
}
