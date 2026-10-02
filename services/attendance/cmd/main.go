// Command attendance is the entrypoint for the García Lorca attendance service.
//
// It serves the generated garcialorca.attendance.v1 Connect service (the daily
// pass list, the group·day reconcile and one enrollment's history) backed by the
// AttendanceService, persisting marks via Postgres (RLS per tenant) and
// publishing AttendanceMarked / AbsenceRecorded. Attendance owns no enrollments
// and no groups: both are validated over Connect as server-to-server clients
// (network-private in shared environments) against students and config.
//
// TRANSPORT NOTE: events go to Cloud Pub/Sub (attendance-events topic) whenever a
// broker is configured — the compose emulator locally, the real service in shared
// environments. With NO broker configured the service runs standalone on the dev
// log bus; a configured broker that cannot be reached fails startup instead (see
// gcp.PublisherFromEnv). The service.Publisher seam is the same either way.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"
	// The school's zone must resolve inside a distroless image too: the
	// attendance report reads enrollment instants as Mexico City days.
	_ "time/tzdata"

	connect "connectrpc.com/connect"
	"github.com/google/uuid"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/garcia-lorca/gen/go/garcialorca/attendance/v1/attendancev1connect"
	"github.com/garcia-lorca/pkg/dbdsn"
	"github.com/garcia-lorca/pkg/dbpool"
	"github.com/garcia-lorca/pkg/gcpauth"
	"github.com/garcia-lorca/pkg/logger"
	"github.com/garcia-lorca/pkg/pubsub"
	"github.com/garcia-lorca/pkg/pubsub/gcp"
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
	// defaultSchoolTimezone is the school's calendar (SCHOOL_TIMEZONE), as in comms.
	defaultSchoolTimezone = "America/Mexico_City"
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

	if err := run(context.Background(), log); err != nil {
		log.Error("attendance stopped", slog.Any("error", err))
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
			log.Warn("attendance could not close the event publisher", slog.Any("error", err))
		}
	}()

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
		return err
	}
	return nil
}

// buildMux is the composition root: config → adapters (both reader clients, repo
// and publisher) → service → Connect handler, returning the mounted routes plus
// the event publisher the caller must close on shutdown.
func buildMux(ctx context.Context, log *slog.Logger) (*http.ServeMux, pubsub.PublisherCloser, error) {
	// Single-tenant for this phase: the tenant is fixed here (not derived from
	// the caller), so RLS binds this one tenant for every request. See ADR 0001.
	tenantID := envOr("ATTENDANCE_TENANT_ID", defaultTenant)

	db, err := openDB()
	if err != nil {
		return nil, nil, err
	}

	repo := repoadapter.New(db, tenantID)

	// Attendance stores marks and nothing else: the roster comes from students
	// (the group's enrollments) and the group reference is validated against
	// config, both by RPC. The service stays testable behind the
	// EnrollmentReader / GroupReader seams. http.DefaultClient speaks Connect
	// over HTTP/1.1, sufficient for these unary calls.
	studentsBaseURL := envOr("STUDENTS_BASE_URL", defaultStudentsBaseURL)
	configBaseURL := envOr("CONFIG_BASE_URL", defaultConfigBaseURL)
	enrollments := studentsclient.New(studentsBaseURL, gcpauth.HTTPClient(ctx, studentsBaseURL))
	groups := configclient.New(configBaseURL, gcpauth.HTTPClient(ctx, configBaseURL))

	events, err := gcp.PublisherFromEnv(ctx, log)
	if err != nil {
		return nil, nil, err
	}
	publisher := pubsubadapter.NewEventPublisher(events)

	school, err := schoolLocation()
	if err != nil {
		return nil, nil, err
	}
	newID := func() string { return uuid.NewString() }
	svc := service.NewAttendanceService(repo, enrollments, groups, publisher, tenantID, newID, log, service.WithSchoolLocation(school))
	// The level-deletion trash (ADR 0021): the gateway's saga archives and
	// restores through it, and the gateway's hourly sweep (always warm, unlike
	// this scale-to-zero service) purges each batch past config's
	// restorable_until through PurgeLevelBatch.
	trash := service.NewLevelArchiveService(repo, time.Now)
	h := handler.NewAttendanceHandler(svc).WithLevelArchive(trash)

	mux := http.NewServeMux()
	path, connectHandler := attendancev1connect.NewAttendanceServiceHandler(h, connect.WithRecover(recoverFn))
	mux.Handle(path, connectHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux, events, nil
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

func recoverFn(_ context.Context, _ connect.Spec, _ http.Header, _ any) error {
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}

// schoolLocation is the school's time zone (SCHOOL_TIMEZONE, default
// America/Mexico_City). An unknown zone fails startup: falling back to UTC
// would leave a pupil enrolled after 18:00 out of that day's list (T12).
func schoolLocation() (*time.Location, error) {
	name := envOr("SCHOOL_TIMEZONE", defaultSchoolTimezone)
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("SCHOOL_TIMEZONE %q: %w", name, err)
	}
	return loc, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
