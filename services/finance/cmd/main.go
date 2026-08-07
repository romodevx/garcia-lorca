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
// environments. Without one the service falls back to a logging dev bus so it
// runs standalone. The ports.PaymentPublisher port is the same either way.
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
	// defaultPubSubProject matches the project the compose emulator starts with.
	defaultPubSubProject = "garcia-lorca-local"

	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 120 * time.Second
)

var errMissingDatabaseURL = errors.New("DATABASE_URL is required")

func main() {
	log := logger.Default()

	mux, err := buildMux(context.Background(), log)
	if err != nil {
		log.Error("finance failed to start", slog.Any("error", err))
		os.Exit(1)
	}

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
		log.Error("finance server stopped", slog.Any("error", err))
		os.Exit(1)
	}
}

// buildMux is the composition root: config → adapters (both reader clients,
// repo and publisher) → use case → Connect handler, returning the mounted
// routes.
func buildMux(ctx context.Context, log *slog.Logger) (*http.ServeMux, error) {
	// Single-tenant for this phase: the tenant is fixed here (not derived from
	// the caller), so RLS binds this one tenant for every request. See ADR 0001.
	tenantID := envOr("FINANCE_TENANT_ID", defaultTenant)

	db, err := openDB()
	if err != nil {
		return nil, err
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

	publisher := pubsubadapter.NewEventPublisher(selectPublisher(ctx, log))

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
	return mux, nil
}

// openDB opens the GORM Postgres pool from DATABASE_URL.
func openDB() (*gorm.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, errMissingDatabaseURL
	}
	return gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Warn)})
}

// selectPublisher returns the Cloud Pub/Sub client when a broker is configured
// (PUBSUB_EMULATOR_HOST locally, PUBSUB_PROJECT_ID in a real environment), else
// the dev log bus so the service still runs standalone. A client that fails to
// initialise degrades to the dev bus rather than blocking startup: publishing is
// not on the read path.
func selectPublisher(ctx context.Context, log *slog.Logger) pubsub.Publisher {
	if os.Getenv("PUBSUB_EMULATOR_HOST") == "" && os.Getenv("PUBSUB_PROJECT_ID") == "" {
		return newDevBus(log)
	}

	projectID := envOr("PUBSUB_PROJECT_ID", defaultPubSubProject)
	client, err := gcp.New(ctx, projectID)
	if err != nil {
		log.Error("pubsub client init failed; falling back to dev bus", slog.Any("error", err))
		return newDevBus(log)
	}
	log.Info("publishing events to Cloud Pub/Sub", slog.String("project", projectID))
	return client
}

// devBus is a dev-only pubsub.Publisher that logs events instead of transporting
// them. It is the fallback when no broker is configured.
type devBus struct{ log *slog.Logger }

func newDevBus(log *slog.Logger) *devBus { return &devBus{log: log} }

func (b *devBus) Publish(_ context.Context, msg pubsub.Message) error {
	b.log.Info("event published (dev bus)",
		slog.String("topic", msg.Topic),
		slog.String("type", msg.Attributes["type"]),
		slog.Int("bytes", len(msg.Data)))
	return nil
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
