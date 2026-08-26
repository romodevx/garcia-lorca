// Command uniforms is the entrypoint for the García Lorca uniform shop.
//
// It serves the generated garcialorca.uniforms.v1 Connect service: the garment
// catalogue, stock by size, and the counter sale. The family ordering half of
// the shop — holds, deadlines, the expiry sweep — arrives in stage two, and its
// routes answer Unimplemented until then rather than pretending.
//
// The shop owns garments and stock, and nothing else. Who a pupil is comes from
// students by RPC; the money comes from and goes back to finance by RPC. That
// second one is a WRITE into another bounded context, which is unusual here and
// deliberate: the alternative was a second till, and a school cannot reconcile
// two.
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

	"github.com/garcia-lorca/gen/go/garcialorca/uniforms/v1/uniformsv1connect"
	"github.com/garcia-lorca/pkg/dbdsn"
	"github.com/garcia-lorca/pkg/gcpauth"
	"github.com/garcia-lorca/pkg/logger"
	"github.com/garcia-lorca/services/uniforms/internal/adapters/financeclient"
	grpcadapter "github.com/garcia-lorca/services/uniforms/internal/adapters/grpc"
	pgadapter "github.com/garcia-lorca/services/uniforms/internal/adapters/postgres"
	"github.com/garcia-lorca/services/uniforms/internal/adapters/studentsclient"
	"github.com/garcia-lorca/services/uniforms/internal/usecase"
)

const (
	serviceName   = "uniforms"
	defaultAddr   = ":8080"
	defaultTenant = "00000000-0000-0000-0000-000000000001"
	// Local dev ports; compose overrides them to the in-network addresses
	// http://students:8080 / http://finance:8080.
	defaultStudentsBaseURL = "http://localhost:8092"
	defaultFinanceBaseURL  = "http://localhost:8093"

	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 120 * time.Second
)

func main() {
	log := logger.Default()
	if err := run(context.Background(), log); err != nil {
		log.Error("uniforms stopped", slog.Any("error", err))
		os.Exit(1)
	}
}

// run is the process lifecycle: build the composition root and serve until the
// listener stops.
func run(ctx context.Context, log *slog.Logger) error {
	mux, err := buildMux(ctx)
	if err != nil {
		return err
	}

	addr := envOr("ADDR", defaultAddr)
	log.Info("uniforms starting", slog.String("addr", addr))
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
		return err
	}
	return nil
}

// buildMux is the composition root: config → adapters → use case → Connect
// handler. Dependencies are assembled here and nowhere else, so every layer
// below takes what it needs as an interface and can be tested without any of it.
func buildMux(ctx context.Context) (*http.ServeMux, error) {
	// Single-tenant for this phase: the tenant is fixed HERE rather than derived
	// from the caller, so RLS binds this one school for every request, and a
	// request cannot talk its way into another's data by claiming one (ADR 0001).
	tenantID := envOr("UNIFORMS_TENANT_ID", defaultTenant)

	db, err := openDB()
	if err != nil {
		return nil, err
	}

	now := time.Now
	repo := pgadapter.NewRepository(db, tenantID, now)

	studentsBaseURL := envOr("STUDENTS_BASE_URL", defaultStudentsBaseURL)
	financeBaseURL := envOr("FINANCE_BASE_URL", defaultFinanceBaseURL)
	students := studentsclient.New(studentsBaseURL, gcpauth.HTTPClient(ctx, studentsBaseURL))
	finance := financeclient.New(financeBaseURL, gcpauth.HTTPClient(ctx, financeBaseURL))

	// The event publisher arrives with stage two, when there is something worth
	// announcing: a hold that lapsed, a size that finally came in. A counter sale
	// is over before anyone could react to it, so nothing here publishes yet, and
	// a nil seam is honest about that.
	shop := usecase.NewService(repo, students, finance, nil, tenantID,
		func() string { return uuid.NewString() }, now)
	handler := grpcadapter.NewUniformsHandler(shop, usecase.NewHealthService(serviceName))

	mux := http.NewServeMux()
	path, connectHandler := uniformsv1connect.NewUniformsServiceHandler(handler, connect.WithRecover(recoverFn))
	mux.Handle(path, connectHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux, nil
}

// openDB opens the GORM Postgres pool, resolving its DSN via pkg/dbdsn:
// DATABASE_URL verbatim when set (docker compose), otherwise assembled from the
// Cloud SQL split env.
func openDB() (*gorm.DB, error) {
	dsn, err := dbdsn.Resolve()
	if err != nil {
		return nil, err
	}
	return gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Warn)})
}

// recoverFn turns a panic into a bare internal error. Whatever the panic was
// carrying is not the caller's business.
func recoverFn(_ context.Context, _ connect.Spec, _ http.Header, _ any) error {
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
