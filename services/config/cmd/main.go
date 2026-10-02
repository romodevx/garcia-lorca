// Command config is the entrypoint for the García Lorca config service (the
// academic catalog: cycles, levels, grade_levels, subjects, terms, groups).
//
// It serves the generated garcialorca.config.v1 Connect service (Create/Get/List
// per catalog entity, plus Health) backed by the CatalogService use case,
// persisting via Postgres (RLS per tenant), plus a plain /healthz.
package main

import (
	"context"
	"errors"
	"fmt"
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

	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	"github.com/garcia-lorca/pkg/dbdsn"
	"github.com/garcia-lorca/pkg/dbpool"
	"github.com/garcia-lorca/pkg/logger"
	grpcadapter "github.com/garcia-lorca/services/config/internal/adapters/grpc"
	pgadapter "github.com/garcia-lorca/services/config/internal/adapters/postgres"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

const (
	serviceName   = "config"
	defaultAddr   = ":8080"
	defaultTenant = "00000000-0000-0000-0000-000000000001"

	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 120 * time.Second
)

func main() {
	log := logger.Default()

	mux, err := buildMux(log)
	if err != nil {
		log.Error("config failed to start", slog.Any("error", err))
		os.Exit(1)
	}

	addr := envOr("ADDR", defaultAddr)
	log.Info("config starting", slog.String("addr", addr))
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
		log.Error("config server stopped", slog.Any("error", err))
		os.Exit(1)
	}
}

// buildMux is the composition root: config → adapters → use case → Connect
// handler, returning the mounted routes.
func buildMux(log *slog.Logger) (*http.ServeMux, error) {
	// M04 is single-tenant: the tenant is fixed here (not derived from the
	// caller), so RLS binds this one tenant for every request. When multi-tenant
	// lands, the gateway must forward the authenticated principal's TenantID and
	// this must stop being a process-wide constant (as in iam/students).
	tenantID := envOr("CONFIG_TENANT_ID", defaultTenant)

	db, err := openDB()
	if err != nil {
		return nil, err
	}

	repo := pgadapter.NewRepository(db, tenantID)
	catalog := usecase.NewCatalogService(repo, tenantID, func() string { return uuid.NewString() })
	health := usecase.NewHealthService(serviceName)
	// The level trash (ADR 0021). Its retention sweep runs in the gateway
	// (always warm), batch by batch on restorable_until; this scale-to-zero
	// service keeps no ticker of its own.
	trash := usecase.NewLevelDeletionService(repo, time.Now)
	periods := usecase.NewEvaluationPeriodService(repo, tenantID, func() string { return uuid.NewString() })
	handler := grpcadapter.NewConfigHandler(catalog, health).WithLevelDeletion(trash).WithEvaluationPeriods(periods)

	mux := http.NewServeMux()
	path, h := configv1connect.NewConfigServiceHandler(handler, connect.WithRecover(recoverFn))
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
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Warn)})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	// database/sql opens UNLIMITED connections by default. Nine services on Cloud
	// Run, each allowed several instances, would exhaust a small managed Postgres.
	dbpool.Tune(sqlDB)
	return db, nil
}

func recoverFn(_ context.Context, _ connect.Spec, _ http.Header, p any) error {
	return connect.NewError(connect.CodeInternal, fmt.Errorf("panic: %v", p))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
