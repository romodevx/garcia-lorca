// Command students is the entrypoint for the García Lorca students service.
//
// It serves the generated garcialorca.students.v1 Connect service (CreateApplicant,
// GetStudent, ListStudents, ChangeAdmissionStatus, Health) backed by the
// StudentService use case, persisting via Postgres (RLS per tenant) and
// publishing StudentEnrolled / AdmissionStatusChanged domain events.
//
// TRANSPORT NOTE: the Pub/Sub transport is a dev-only logging publisher for now;
// the Cloud Pub/Sub adapter (student-events topic, provisioned in Terraform) is
// the remaining infra follow-up. The EventPublisher port stays unchanged when it
// lands.
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

	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
	"github.com/garcia-lorca/pkg/logger"
	"github.com/garcia-lorca/pkg/pubsub"
	"github.com/garcia-lorca/services/students/internal/adapters/configclient"
	grpcadapter "github.com/garcia-lorca/services/students/internal/adapters/grpc"
	pgadapter "github.com/garcia-lorca/services/students/internal/adapters/postgres"
	pubsubadapter "github.com/garcia-lorca/services/students/internal/adapters/pubsub"
	"github.com/garcia-lorca/services/students/internal/usecase"
)

const (
	serviceName   = "students"
	defaultAddr   = ":8080"
	defaultTenant = "00000000-0000-0000-0000-000000000001"
	// defaultConfigBaseURL targets the config service on its local dev port
	// (compose overrides it to the in-network address http://config:8080).
	defaultConfigBaseURL = "http://localhost:8101"

	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 120 * time.Second
)

var errMissingDatabaseURL = errors.New("DATABASE_URL is required")

func main() {
	log := logger.Default()

	mux, err := buildMux(log)
	if err != nil {
		log.Error("students failed to start", slog.Any("error", err))
		os.Exit(1)
	}

	addr := envOr("ADDR", defaultAddr)
	log.Info("students starting", slog.String("addr", addr))
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
		log.Error("students server stopped", slog.Any("error", err))
		os.Exit(1)
	}
}

// buildMux is the composition root: config → adapters → use case → Connect
// handler, returning the mounted routes.
func buildMux(log *slog.Logger) (*http.ServeMux, error) {
	// M03 is single-tenant: the tenant is fixed here (not derived from the
	// caller), so RLS binds this one tenant for every request. When multi-tenant
	// lands, the gateway must forward the authenticated principal's TenantID and
	// this must stop being a process-wide constant. See ADR 0001.
	tenantID := envOr("STUDENTS_TENANT_ID", defaultTenant)

	db, err := openDB()
	if err != nil {
		return nil, err
	}

	repo := pgadapter.NewRepository(db, tenantID)
	events := pubsubadapter.NewEventPublisher(newDevBus(log))
	newID := func() string { return uuid.NewString() }
	svc := usecase.NewStudentService(repo, events, tenantID, newID)

	// The enroll knot validates group/term references against the config bounded
	// context by RPC (the app's first service-to-service call). The catalog
	// validator is a Connect client to config; the use case stays testable behind
	// the CatalogValidator port. http.DefaultClient speaks Connect over HTTP/1.1,
	// which is sufficient for these unary calls.
	configBaseURL := envOr("CONFIG_BASE_URL", defaultConfigBaseURL)
	catalog := configclient.New(configBaseURL, http.DefaultClient)
	enroll := usecase.NewEnrollmentService(repo, repo, catalog, events, tenantID, newID, time.Now)

	health := usecase.NewHealthService(serviceName)
	handler := grpcadapter.NewStudentsHandler(svc, enroll, health)

	mux := http.NewServeMux()
	path, h := studentsv1connect.NewStudentsServiceHandler(handler, connect.WithRecover(recoverFn))
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

// devBus is a dev-only pubsub.Publisher that logs events instead of transporting
// them. It is replaced by the Cloud Pub/Sub adapter in shared environments.
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
