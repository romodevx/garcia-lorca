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
	"github.com/garcia-lorca/pkg/dbdsn"
	"github.com/garcia-lorca/pkg/dbpool"
	"github.com/garcia-lorca/pkg/logger"
	"github.com/garcia-lorca/services/iam/internal/adapters/bcrypt"
	"github.com/garcia-lorca/services/iam/internal/adapters/googleid"
	grpcadapter "github.com/garcia-lorca/services/iam/internal/adapters/grpc"
	"github.com/garcia-lorca/services/iam/internal/adapters/jwt"
	pgadapter "github.com/garcia-lorca/services/iam/internal/adapters/postgres"
	"github.com/garcia-lorca/services/iam/internal/ports"
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
	secret, err := jwtSecret(log)
	if err != nil {
		return nil, err
	}
	tokens := jwt.NewHMAC(secret, tokenIssuer, refreshTTL, time.Now)

	// The external identity provider is OPTIONAL and off unless named. An empty
	// IDENTITY_PLATFORM_PROJECT leaves the verifier nil, which makes
	// AuthenticateWithIDToken refuse outright — local and any environment that
	// has not been migrated keep signing in with a password and are unaffected.
	// Nothing is inferred from ambient credentials: the project is stated or the
	// path is closed.
	var idTokens ports.IDTokenVerifier
	if project := os.Getenv("IDENTITY_PLATFORM_PROJECT"); project != "" {
		v, err := googleid.NewFromProject(context.Background(), project)
		if err != nil {
			// Fatal on purpose. A misconfigured provider that boots anyway would
			// silently refuse every sign-in through it, and the only symptom would
			// be users unable to log in with no error to read anywhere.
			return nil, err
		}
		idTokens = v
		log.Info("identity provider wired", "project", project)
	}

	auth := usecase.NewAuthService(userRepo, roleRepo, hasher, tokens, tokens, idTokens, tenantID, accessTokenTTL, time.Now)
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

// errMissingJWTSecret stops the boot when a shared environment has no signing
// key configured. Deliberately fatal rather than degraded: see jwtSecret.
var errMissingJWTSecret = errors.New("IAM_JWT_SECRET must be set outside a local environment")

// jwtSecret reads IAM_JWT_SECRET. When it is absent, the behaviour depends on
// where we are running:
//
//   - LOCAL (APP_ENV empty or "local"): generate an ephemeral key and warn.
//     docker-compose never restarts iam, so a per-process key is invisible and
//     saves developers from managing a secret.
//   - ANY SHARED ENVIRONMENT (dev/staging/prod): refuse to boot.
//
// The second case is not caution, it is a scar. Under Cloud Run with
// min_instances=0 this service sleeps after ~15 minutes of idle; the next
// request wakes a NEW process which, with an ephemeral key, cannot validate a
// single token it issued before sleeping. The user is silently thrown back to
// the login screen mid-session, and two concurrent instances sign with two
// different keys. A revision that refuses to start is a loud failure someone
// fixes in a minute; a service that quietly forgets sessions is a demo that
// falls apart in front of a customer.
func jwtSecret(log *slog.Logger) ([]byte, error) {
	if s := os.Getenv("IAM_JWT_SECRET"); s != "" {
		return []byte(s), nil
	}
	if appEnv := os.Getenv("APP_ENV"); appEnv != "" && appEnv != "local" {
		return nil, fmt.Errorf("%w (APP_ENV=%s)", errMissingJWTSecret, appEnv)
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate ephemeral signing key: %w", err)
	}
	log.Warn("IAM_JWT_SECRET not set; generated an ephemeral signing key (local only — tokens do not survive a restart)")
	return []byte(hex.EncodeToString(b)), nil
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
