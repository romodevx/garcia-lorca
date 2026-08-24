// Command grades is the entrypoint for the García Lorca grades service.
//
// It serves the generated garcialorca.grades.v1 Connect service (CaptureGrade,
// GetRoster, ListGradeEntries, Health) backed by the GradesService use case,
// persisting grade_entries via Postgres (RLS per tenant). The capture knot and
// roster combine two cross-service seams — config (teaching assignment + term)
// and students (enrollments) — both reached over Connect as server-to-server
// clients (network-private in shared environments).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	connect "connectrpc.com/connect"
	"github.com/google/uuid"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/garcia-lorca/gen/go/garcialorca/grades/v1/gradesv1connect"
	"github.com/garcia-lorca/pkg/dbdsn"
	"github.com/garcia-lorca/pkg/gcpauth"
	"github.com/garcia-lorca/pkg/logger"
	"github.com/garcia-lorca/services/grades/internal/adapters/configclient"
	grpcadapter "github.com/garcia-lorca/services/grades/internal/adapters/grpc"
	pgadapter "github.com/garcia-lorca/services/grades/internal/adapters/postgres"
	"github.com/garcia-lorca/services/grades/internal/adapters/studentsclient"
	"github.com/garcia-lorca/services/grades/internal/usecase"
)

const (
	serviceName   = "grades"
	defaultAddr   = ":8080"
	defaultTenant = "00000000-0000-0000-0000-000000000001"
	// The reader clients target the config/students services on their local dev
	// ports; compose overrides them to the in-network addresses
	// http://config:8080 / http://students:8080.
	defaultConfigBaseURL   = "http://localhost:8101"
	defaultStudentsBaseURL = "http://localhost:8092"

	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 120 * time.Second
)

func main() {
	log := logger.Default()

	mux, err := buildMux()
	if err != nil {
		log.Error("grades failed to start", slog.Any("error", err))
		os.Exit(1)
	}

	addr := envOr("ADDR", defaultAddr)
	log.Info("grades starting", slog.String("addr", addr))
	// h2c: serve HTTP/2 cleartext so both gRPC and Connect clients work.
	srv := &http.Server{
		Addr:              addr,
		Handler:           h2c.NewHandler(mux, &http2.Server{}),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("grades server stopped", slog.Any("error", err))
		os.Exit(1)
	}
}

// buildMux is the composition root: config → adapters (both reader clients +
// repo) → use case → Connect handler, returning the mounted routes.
func buildMux() (*http.ServeMux, error) {
	// Single-tenant for this phase: the tenant is fixed here (not derived from
	// the caller), so RLS binds this one tenant for every request. See ADR 0001.
	tenantID := envOr("GRADES_TENANT_ID", defaultTenant)

	db, err := openDB()
	if err != nil {
		return nil, err
	}

	repo := pgadapter.NewRepository(db, tenantID)

	// The capture knot + roster validate cross-service references by RPC. Both
	// readers are Connect clients (server-to-server, network-private in shared
	// envs); the use case stays testable behind the AssignmentReader /
	// EnrollmentReader ports. http.DefaultClient speaks Connect over HTTP/1.1,
	// sufficient for these unary calls.
	configBaseURL := envOr("CONFIG_BASE_URL", defaultConfigBaseURL)
	studentsBaseURL := envOr("STUDENTS_BASE_URL", defaultStudentsBaseURL)
	assignments := configclient.New(configBaseURL, gcpauth.HTTPClient(context.Background(), configBaseURL))
	enrollments := studentsclient.New(studentsBaseURL, gcpauth.HTTPClient(context.Background(), studentsBaseURL))

	newID := func() string { return uuid.NewString() }
	svc := usecase.NewGradesService(repo, assignments, enrollments, tenantID, newID, time.Now)
	health := usecase.NewHealthService(serviceName)
	handler := grpcadapter.NewGradesHandler(svc, health)

	mux := http.NewServeMux()
	path, h := gradesv1connect.NewGradesServiceHandler(handler, connect.WithRecover(recoverFn))
	mux.Handle(path, h)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux, nil
}

// openDB opens the GORM Postgres pool, resolving its DSN via pkg/dbdsn:
// DATABASE_URL verbatim when set (docker-compose), otherwise assembled
// from the Cloud SQL split env (DB_SOCKET_DIR/DB_USER/DB_PASSWORD/DB_NAME).
func openDB() (*gorm.DB, error) {
	dsn, err := dbdsn.Resolve()
	if err != nil {
		return nil, err
	}
	return gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Warn)})
}

func recoverFn(_ context.Context, _ connect.Spec, _ http.Header, _ any) error {
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
