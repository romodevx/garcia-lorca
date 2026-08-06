// Command iam is the entrypoint for the García Lorca identity & access service.
//
// It serves the generated garcialorca.iam.v1 Connect service (Login,
// RefreshToken, ValidateToken, CheckPermission, AssignRole, RevokeRole, Health)
// backed by the AuthService use case, plus a plain /healthz and the legacy
// HealthService/Ping route the gateway currently probes.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

	"github.com/garcia-lorca/gen/go/garcialorca/iam/v1/iamv1connect"
	"github.com/garcia-lorca/pkg/logger"
	"github.com/garcia-lorca/services/iam/internal/adapters/bcrypt"
	grpcadapter "github.com/garcia-lorca/services/iam/internal/adapters/grpc"
	"github.com/garcia-lorca/services/iam/internal/adapters/jwt"
	pgadapter "github.com/garcia-lorca/services/iam/internal/adapters/postgres"
	"github.com/garcia-lorca/services/iam/internal/usecase"
)

const (
	serviceName    = "iam"
	defaultAddr    = ":8080"
	defaultTenant  = "00000000-0000-0000-0000-000000000001"
	tokenIssuer    = "garcia-lorca"
	accessTokenTTL = 15 * time.Minute
	refreshTTL     = 24 * time.Hour
)

var errMissingDatabaseURL = errors.New("DATABASE_URL is required")

func main() {
	log := logger.Default()

	mux, err := buildMux(log)
	if err != nil {
		log.Error("iam failed to start", slog.Any("error", err))
		os.Exit(1)
	}

	addr := envOr("ADDR", defaultAddr)
	log.Info("iam starting", slog.String("addr", addr))
	// h2c: serve HTTP/2 cleartext so both gRPC and Connect clients work.
	srv := &http.Server{Addr: addr, Handler: h2c.NewHandler(mux, &http2.Server{})}
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("iam server stopped", slog.Any("error", err))
		os.Exit(1)
	}
}

// buildMux is the composition root: it wires config → adapters → use cases →
// the Connect handler, and returns the mounted routes.
func buildMux(log *slog.Logger) (*http.ServeMux, error) {
	tenantID := envOr("IAM_TENANT_ID", defaultTenant)

	db, err := openDB()
	if err != nil {
		return nil, err
	}

	userRepo := pgadapter.NewUserRepo(db, tenantID)
	roleRepo := pgadapter.NewRoleRepo(db, tenantID)
	hasher := bcrypt.NewHasher(0)
	tokens := jwt.NewHMAC(jwtSecret(log), tokenIssuer, refreshTTL, time.Now)

	auth := usecase.NewAuthService(userRepo, roleRepo, hasher, tokens, tokens, tenantID, accessTokenTTL, time.Now)
	admin := usecase.NewAdminService(userRepo, roleRepo, hasher, tenantID, func() string { return uuid.NewString() })
	health := usecase.NewHealthService(serviceName)
	iamHandler := grpcadapter.NewIAMHandler(auth, admin, health)

	mux := http.NewServeMux()
	// Generated Connect service (Login, ValidateToken, …, Health).
	path, handler := iamv1connect.NewIamServiceHandler(iamHandler, connect.WithRecover(recoverFn))
	mux.Handle(path, handler)
	// Plain /healthz + legacy HealthService/Ping (the gateway probes this today).
	grpcadapter.NewHealthHandler(health).Register(mux)
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

// jwtSecret reads IAM_JWT_SECRET, or generates an ephemeral one (dev only —
// tokens do not survive a restart; production supplies the secret).
func jwtSecret(log *slog.Logger) []byte {
	if s := os.Getenv("IAM_JWT_SECRET"); s != "" {
		return []byte(s)
	}
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	log.Warn("IAM_JWT_SECRET not set; generated an ephemeral signing key (dev only)")
	return []byte(hex.EncodeToString(b))
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
