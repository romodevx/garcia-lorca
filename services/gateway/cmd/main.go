// Command gateway is the entrypoint for the García Lorca BFF / edge gateway.
//
// FASE 0: the gateway boots an HTTP server with its own health endpoint and a
// /readyz that calls the iam service end to end — the gateway half of the
// Sprint-1 pipeline-validation hello-world. The feature-flag middleware
// (internal/middleware) is wired but guards no module routes yet; modules plug
// in as they land.
package main

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/config"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"

	"github.com/garcia-lorca/pkg/featureflag"
	"github.com/garcia-lorca/pkg/logger"
)

const (
	defaultAddr       = ":8080"
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 120 * time.Second
)

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
	studentsBaseURL := os.Getenv("STUDENTS_BASE_URL")
	if studentsBaseURL == "" {
		studentsBaseURL = "http://localhost:8083"
	}
	configBaseURL := os.Getenv("CONFIG_BASE_URL")
	if configBaseURL == "" {
		configBaseURL = "http://localhost:8085"
	}

	// flags is the authoritative source of module visibility (default-deny),
	// loaded from ENABLED_MODULES (comma-separated module names). The gateway
	// both enforces the boundary (ModuleGuard ⇒ 404) and reports it (GET
	// /features). Each module wires its routes behind the guard as it lands.
	flags := featureflag.NewInMemory(config.ParseEnabledModules(os.Getenv("ENABLED_MODULES")))

	iamHealth := client.NewIAMClient(iamBaseURL, http.DefaultClient)
	authClient := client.NewAuthClient(iamBaseURL, http.DefaultClient)
	studentsClient := client.NewStudentsClient(studentsBaseURL, http.DefaultClient)
	configClient := client.NewConfigClient(configBaseURL, http.DefaultClient)

	// loginLimiter fronts POST /auth/login with a per-IP token bucket so an
	// attacker cannot brute-force credentials from a single source. It is
	// in-process (fine while the gateway runs at min-instances=1).
	loginRL := config.ParseLoginRateLimit(os.Getenv)
	loginLimiter := middleware.NewRateLimiter(loginRL.Burst, loginRL.RefillPerSec(), time.Now)

	mux := newMux(iamHealth, authClient, studentsClient, configClient, flags, loginLimiter)

	log.Info("gateway starting",
		slog.String("addr", addr),
		slog.String("iam", iamBaseURL),
		slog.String("students", studentsBaseURL),
		slog.String("config", configBaseURL))
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("gateway server stopped", slog.Any("error", err))
		os.Exit(1)
	}
}

// newMux builds the gateway HTTP handler. Extracted from main so an integration
// test can assemble the exact production routes against a test iam server.
//
//	POST /api/v1/auth/login    → per-IP rate limit → proxy to iam Login (unauthenticated)
//	POST /api/v1/auth/refresh  → rotate both auth cookies via iam RefreshToken
//	POST /api/v1/auth/logout   → expire both auth cookies (unauthenticated)
//	GET  /api/v1/me            → RequireAuth → the caller's principal
//	GET  /api/v1/features      → resolved module-flag map (frontend gating)
//	POST /api/v1/students      → ModuleGuard(students) → RequireAuth →
//	                             RequirePermission(students:create) → students service
//	/api/v1/config/...         → ModuleGuard(config) → RequireAuth → rotation →
//	                             RequirePermission(config:write on POST, config:read
//	                             on GET) → config (academic catalog) service
//	/healthz, /readyz          → liveness + iam reachability
//
// `flags` (via middleware.NewModuleGuard) is the module-visibility boundary:
// disabled module ⇒ 404, evaluated before auth so a hidden module is invisible.
func newMux(iamHealth *client.IAMClient, authClient *client.AuthClient, studentsClient *client.StudentsClient, configClient *client.ConfigClient, flags featureflag.Checker, loginLimiter *middleware.RateLimiter) *http.ServeMux {
	mux := http.NewServeMux()
	handler.NewHealthHandler(iamHealth).Register(mux)

	authH := handler.NewAuthHandler(authClient)
	requireAuth := middleware.RequireAuth(authClient)
	// requireAuthChanged is the DEFAULT building block for authenticated routes:
	// it authenticates AND enforces the forced password rotation, so a new route
	// is safe unless it deliberately opts out (only change-password does, below).
	requireAuthChanged := func(h http.Handler) http.Handler {
		return requireAuth(middleware.RequirePasswordChanged(h))
	}
	guard := middleware.NewModuleGuard(flags)

	mux.Handle("POST /api/v1/auth/login", loginLimiter.Middleware(http.HandlerFunc(authH.Login)))
	mux.HandleFunc("POST /api/v1/auth/refresh", authH.Refresh)
	mux.Handle("GET /api/v1/me", requireAuthChanged(http.HandlerFunc(authH.Me)))
	// change-password is the one deliberate exception: it stays reachable while
	// the must-change flag is set, so it authenticates but does NOT enforce.
	mux.Handle("POST /api/v1/auth/change-password", requireAuth(http.HandlerFunc(authH.ChangePassword)))
	mux.HandleFunc("POST /api/v1/auth/logout", authH.Logout)
	mux.HandleFunc("GET /api/v1/features", handler.NewFeaturesHandler(flags).Features)

	// M03 students: flag-guarded, authenticated+rotation-enforced, permission-gated.
	studentsCreate := requireAuthChanged(middleware.RequirePermission("students:create")(
		http.HandlerFunc(handler.NewStudentsHandler(studentsClient).Create)))
	mux.Handle("POST /api/v1/students", guard.Protect("students", studentsCreate))

	// M04 config (academic catalog): flag-guarded, authenticated + rotation-
	// enforced, permission-gated. Writes need config:write, reads config:read;
	// every route sits behind guard.Protect("config", …) so module_config OFF ⇒
	// 404 (the module is invisible before auth even runs).
	configH := handler.NewConfigHandler(configClient)
	configWrite := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("config", requireAuthChanged(middleware.RequirePermission("config:write")(http.HandlerFunc(h))))
	}
	configRead := func(h http.HandlerFunc) http.Handler {
		return guard.Protect("config", requireAuthChanged(middleware.RequirePermission("config:read")(http.HandlerFunc(h))))
	}
	mux.Handle("POST /api/v1/config/cycles", configWrite(configH.CreateCycle))
	mux.Handle("GET /api/v1/config/cycles", configRead(configH.ListCycles))
	mux.Handle("POST /api/v1/config/levels", configWrite(configH.CreateLevel))
	mux.Handle("GET /api/v1/config/levels", configRead(configH.ListLevels))
	mux.Handle("POST /api/v1/config/grade-levels", configWrite(configH.CreateGradeLevel))
	mux.Handle("GET /api/v1/config/grade-levels", configRead(configH.ListGradeLevels))
	mux.Handle("POST /api/v1/config/subjects", configWrite(configH.CreateSubject))
	mux.Handle("GET /api/v1/config/subjects", configRead(configH.ListSubjects))
	mux.Handle("POST /api/v1/config/terms", configWrite(configH.CreateTerm))
	mux.Handle("GET /api/v1/config/terms", configRead(configH.ListTerms))
	mux.Handle("POST /api/v1/config/groups", configWrite(configH.CreateGroup))
	mux.Handle("GET /api/v1/config/groups", configRead(configH.ListGroups))
	mux.Handle("POST /api/v1/config/teachers", configWrite(configH.CreateTeacher))
	mux.Handle("GET /api/v1/config/teachers", configRead(configH.ListTeachers))
	mux.Handle("POST /api/v1/config/teaching-assignments", configWrite(configH.CreateTeachingAssignment))
	mux.Handle("GET /api/v1/config/teaching-assignments", configRead(configH.ListTeachingAssignments))
	return mux
}
