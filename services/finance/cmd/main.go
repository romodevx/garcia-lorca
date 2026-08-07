// Command finance is the entrypoint for the García Lorca finance service.
//
// It serves the generated garcialorca.finance.v1 Connect service (the charge
// catalog, the charges issued from it, the payment ledger, the account
// statement and the overdue view) backed by the FinanceService use case,
// persisting via Postgres (RLS per tenant) and publishing PaymentRegistered.
// Two cross-service seams are reached over Connect as server-to-server clients
// (network-private in shared environments): students (the debtor exists, and
// the enrollments a bulk generation expands over) and config (the school
// cycle).
//
// TRANSPORT NOTE: events go to Cloud Pub/Sub (payment-events topic) whenever a
// broker is configured — the compose emulator locally, the real service in shared
// environments. With NO broker configured the service runs standalone on the dev
// log bus; a configured broker that cannot be reached fails startup instead (see
// gcp.PublisherFromEnv). The ports.PaymentPublisher port is the same either way.
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

	"github.com/garcia-lorca/gen/go/garcialorca/finance/v1/financev1connect"
	"github.com/garcia-lorca/pkg/logger"
	"github.com/garcia-lorca/pkg/pubsub"
	"github.com/garcia-lorca/pkg/pubsub/gcp"
	"github.com/garcia-lorca/services/finance/internal/adapters/configclient"
	grpcadapter "github.com/garcia-lorca/services/finance/internal/adapters/grpc"
	pgadapter "github.com/garcia-lorca/services/finance/internal/adapters/postgres"
	pubsubadapter "github.com/garcia-lorca/services/finance/internal/adapters/pubsub"
	"github.com/garcia-lorca/services/finance/internal/adapters/studentsclient"
	"github.com/garcia-lorca/services/finance/internal/usecase"
)

const (
	serviceName   = "finance"
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

var errMissingDatabaseURL = errors.New("DATABASE_URL is required")

func main() {
	log := logger.Default()

	if err := run(context.Background(), log); err != nil {
		log.Error("finance stopped", slog.Any("error", err))
		os.Exit(1)
	}
}

// run is the process lifecycle: build the composition root, serve until the
// listener stops, and release the event transport on the way out.
func run(ctx context.Context, log *slog.Logger) error {
	mux, events, err := buildMux(ctx, log)
	if err != nil {
		return err
	}
	// The publisher owns the broker client's per-topic batching goroutines, so
	// releasing it belongs next to the code that built it.
	defer func() {
		if err := events.Close(); err != nil {
			log.Warn("finance could not close the event publisher", slog.Any("error", err))
		}
	}()

	addr := envOr("ADDR", defaultAddr)
	log.Info("finance starting", slog.String("addr", addr))
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

// buildMux is the composition root: config → adapters (both reader clients,
// repo and publisher) → use case → Connect handler, returning the mounted routes
// plus the event publisher the caller must close on shutdown.
func buildMux(ctx context.Context, log *slog.Logger) (*http.ServeMux, pubsub.PublisherCloser, error) {
	// Single-tenant for this phase: the tenant is fixed here (not derived from
	// the caller), so RLS binds this one tenant for every request. See ADR 0001.
	tenantID := envOr("FINANCE_TENANT_ID", defaultTenant)

	db, err := openDB()
	if err != nil {
		return nil, nil, err
	}

	repo := pgadapter.NewRepository(db, tenantID)

	// The ledger validates its cross-service references by RPC. One students
	// client serves both seams (StudentReader for the debtor, EnrollmentReader
	// for bulk generation) because both read the same bounded context; config
	// answers for the cycle. The use case stays testable behind the ports.
	// http.DefaultClient speaks Connect over HTTP/1.1, sufficient for these
	// unary calls.
	studentsClient := studentsclient.New(envOr("STUDENTS_BASE_URL", defaultStudentsBaseURL), http.DefaultClient)
	cyclesClient := configclient.New(envOr("CONFIG_BASE_URL", defaultConfigBaseURL), http.DefaultClient)

	events, err := gcp.PublisherFromEnv(ctx, log)
	if err != nil {
		return nil, nil, err
	}
	publisher := pubsubadapter.NewEventPublisher(events)

	newID := func() string { return uuid.NewString() }
	svc := usecase.NewFinanceService(repo, studentsClient, cyclesClient, studentsClient, publisher, tenantID, newID, time.Now)
	health := usecase.NewHealthService(serviceName)
	handler := grpcadapter.NewFinanceHandler(svc, health)

	mux := http.NewServeMux()
	path, h := financev1connect.NewFinanceServiceHandler(handler, connect.WithRecover(recoverFn))
	mux.Handle(path, h)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux, events, nil
}

// openDB opens the GORM Postgres pool from DATABASE_URL.
func openDB() (*gorm.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, errMissingDatabaseURL
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
