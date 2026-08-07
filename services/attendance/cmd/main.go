// Command attendance is the entrypoint for the García Lorca attendance service.
//
// It serves the generated garcialorca.attendance.v1 Connect service (the daily
// pass list, the group·day reconcile and one enrollment's history) backed by the
// AttendanceService, persisting marks via Postgres (RLS per tenant) and
// publishing AttendanceMarked / AbsenceRecorded. Attendance owns no enrollments
// and no groups: both are validated over Connect as server-to-server clients
// (network-private in shared environments) against students and config.
//
// TRANSPORT NOTE: the Pub/Sub transport is a dev-only logging publisher for now;
// the Cloud Pub/Sub adapter (attendance-events topic) is the remaining infra
// follow-up. The service.Publisher seam stays unchanged when it lands.
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

	"github.com/garcia-lorca/gen/go/garcialorca/attendance/v1/attendancev1connect"
	"github.com/garcia-lorca/pkg/logger"
	"github.com/garcia-lorca/pkg/pubsub"
	"github.com/garcia-lorca/services/attendance/internal/adapters/configclient"
	pubsubadapter "github.com/garcia-lorca/services/attendance/internal/adapters/pubsub"
	"github.com/garcia-lorca/services/attendance/internal/adapters/studentsclient"
	"github.com/garcia-lorca/services/attendance/internal/handler"
	repoadapter "github.com/garcia-lorca/services/attendance/internal/repo"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

const (
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

	mux, err := buildMux(log)
	if err != nil {
		log.Error("attendance failed to start", slog.Any("error", err))
		os.Exit(1)
	}

	addr := envOr("ADDR", defaultAddr)
	log.Info("attendance starting", slog.String("addr", addr))
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
		log.Error("attendance server stopped", slog.Any("error", err))
		os.Exit(1)
	}
}

// buildMux is the composition root: config → adapters (both reader clients, repo
// and publisher) → service → Connect handler, returning the mounted routes.
func buildMux(log *slog.Logger) (*http.ServeMux, error) {
	// Single-tenant for this phase: the tenant is fixed here (not derived from
	// the caller), so RLS binds this one tenant for every request. See ADR 0001.
	tenantID := envOr("ATTENDANCE_TENANT_ID", defaultTenant)

	db, err := openDB()
	if err != nil {
		return nil, err
	}

	repo := repoadapter.New(db, tenantID)

	// Attendance stores marks and nothing else: the roster comes from students
	// (the group's enrollments) and the group reference is validated against
	// config, both by RPC. The service stays testable behind the
	// EnrollmentReader / GroupReader seams. http.DefaultClient speaks Connect
	// over HTTP/1.1, sufficient for these unary calls.
	enrollments := studentsclient.New(envOr("STUDENTS_BASE_URL", defaultStudentsBaseURL), http.DefaultClient)
	groups := configclient.New(envOr("CONFIG_BASE_URL", defaultConfigBaseURL), http.DefaultClient)

	publisher := pubsubadapter.NewEventPublisher(newDevBus(log))

	newID := func() string { return uuid.NewString() }
	svc := service.NewAttendanceService(repo, enrollments, groups, publisher, tenantID, newID, log)
	h := handler.NewAttendanceHandler(svc)

	mux := http.NewServeMux()
	path, connectHandler := attendancev1connect.NewAttendanceServiceHandler(h, connect.WithRecover(recoverFn))
	mux.Handle(path, connectHandler)
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
