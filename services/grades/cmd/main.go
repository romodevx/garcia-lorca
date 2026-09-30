// Command grades is the entrypoint for the García Lorca grades service.
//
// It serves the generated garcialorca.grades.v1 Connect service (CaptureGrade,
// GetRoster, ListGradeEntries, Health) backed by the GradesService use case,
// persisting grade_entries via Postgres (RLS per tenant). The capture knot and
// roster combine two cross-service seams — config (teaching assignment + term)
// and students (enrollments) — both reached over Connect as server-to-server
// clients (network-private in shared environments).
//
// T7 adds period closing and report cards (PeriodsService). An issued report
// card is announced as GradesPublished on grades-events through a
// transactional outbox (queued with the close, drained after its commit and by
// a periodic sweep) and the publisher selected from the environment
// (gcp.PublisherFromEnv: the Pub/Sub emulator or broker when configured, the
// logging dev bus otherwise).
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
	"github.com/garcia-lorca/pkg/dbpool"
	"github.com/garcia-lorca/pkg/gcpauth"
	"github.com/garcia-lorca/pkg/logger"
	"github.com/garcia-lorca/pkg/pubsub"
	"github.com/garcia-lorca/pkg/pubsub/gcp"
	"github.com/garcia-lorca/services/grades/internal/adapters/configclient"
	grpcadapter "github.com/garcia-lorca/services/grades/internal/adapters/grpc"
	pgadapter "github.com/garcia-lorca/services/grades/internal/adapters/postgres"
	pubsubadapter "github.com/garcia-lorca/services/grades/internal/adapters/pubsub"
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

	// outboxSweep is how often queued events are looked for (their backoff
	// decides when a failed one is tried again).
	outboxSweep = 30 * time.Second
)

func main() {
	log := logger.Default()

	if err := run(context.Background(), log); err != nil {
		log.Error("grades stopped", slog.Any("error", err))
		os.Exit(1)
	}
}

// run is the process lifecycle: build the composition root, serve until the
// listener stops, and release the event transport on the way out.
func run(ctx context.Context, log *slog.Logger) error {
	mux, events, drain, err := buildMux(ctx, log)
	if err != nil {
		return err
	}
	// The publisher owns the broker client's per-topic batching goroutines, so
	// releasing it belongs next to the code that built it. Defers run last in,
	// first out: the sweep (registered after) stops — and its goroutine ends —
	// before the publisher it publishes through is closed.
	defer func() {
		if err := events.Close(); err != nil {
			log.Warn("grades could not close the event publisher", slog.Any("error", err))
		}
	}()
	stopSweep := startSweep(ctx, drain, log, outboxSweep)
	defer stopSweep()

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
		return err
	}
	return nil
}

// buildMux is the composition root: config → adapters (both reader clients,
// repo and publisher) → use cases → Connect handler, returning the mounted
// routes plus the event publisher the caller must close on shutdown.
func buildMux(ctx context.Context, log *slog.Logger) (*http.ServeMux, pubsub.PublisherCloser, outboxDrain, error) {
	// Single-tenant for this phase: the tenant is fixed here (not derived from
	// the caller), so RLS binds this one tenant for every request. See ADR 0001.
	tenantID := envOr("GRADES_TENANT_ID", defaultTenant)

	db, err := openDB()
	if err != nil {
		return nil, nil, nil, err
	}

	repo := pgadapter.NewRepository(db, tenantID)

	// The capture knot + roster validate cross-service references by RPC. Both
	// readers are Connect clients (server-to-server, network-private in shared
	// envs); the use case stays testable behind the AssignmentReader /
	// EnrollmentReader ports. http.DefaultClient speaks Connect over HTTP/1.1,
	// sufficient for these unary calls.
	configBaseURL := envOr("CONFIG_BASE_URL", defaultConfigBaseURL)
	studentsBaseURL := envOr("STUDENTS_BASE_URL", defaultStudentsBaseURL)
	assignments := configclient.New(configBaseURL, gcpauth.HTTPClient(ctx, configBaseURL))
	enrollments := studentsclient.New(studentsBaseURL, gcpauth.HTTPClient(ctx, studentsBaseURL))

	events, err := gcp.PublisherFromEnv(ctx, log)
	if err != nil {
		return nil, nil, nil, err
	}

	newID := func() string { return uuid.NewString() }
	// The same config client reads the catalog (group → level scale, cycles,
	// a group's assignments) for the monthly knot and the term-level check.
	svc := usecase.NewGradesService(repo, assignments, assignments, enrollments, tenantID, newID, time.Now)
	monthly := usecase.NewMonthlyGradesService(repo, assignments, assignments, enrollments, tenantID, newID, time.Now)
	// Period closing (T7): the same repository stores closures and report
	// cards; the students client names the pupils a report card freezes.
	periods := usecase.NewPeriodsService(usecase.PeriodsDeps{
		Periods: repo, Grades: repo, Terms: assignments, Catalog: assignments, Enrollments: enrollments,
		Students: enrollments, Publisher: pubsubadapter.NewEventPublisher(events), TenantID: tenantID,
		NewID: newID, Now: time.Now, Log: log,
	})
	health := usecase.NewHealthService(serviceName)
	// Level deletion (ADR 0021): grades' step of the gateway's saga. The
	// gateway's hourly sweep (always warm, unlike this scale-to-zero service)
	// purges each batch past config's restorable_until via PurgeLevelBatch.
	levels := usecase.NewLevelArchiveService(repo, time.Now)
	handler := grpcadapter.NewGradesHandler(svc, monthly, periods, health).WithLevelArchive(levels)

	mux := http.NewServeMux()
	path, h := gradesv1connect.NewGradesServiceHandler(handler, connect.WithRecover(recoverFn))
	mux.Handle(path, h)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux, events, func(c context.Context) (int, error) { return periods.DrainOutbox(c, 0) }, nil
}

// outboxDrain publishes the queued GradesPublished events and says how many
// went out.
type outboxDrain func(ctx context.Context) (int, error)

// startSweep runs sweepOutbox in its own goroutine and returns the func that
// stops it: stop cancels the sweep and waits for the goroutine to end, so no
// drain is in flight once it returns. Calling stop again is a no-op.
func startSweep(ctx context.Context, drain outboxDrain, log *slog.Logger, every time.Duration) (stop func()) {
	sweepCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		sweepOutbox(sweepCtx, drain, log, every)
	}()
	return func() {
		cancel()
		<-done
	}
}

// sweepOutbox drains the outbox every `every` until ctx ends. A close
// drains its own events the moment it commits; this is for what that cannot
// cover — the broker was down, or the process died between the commit and the
// drain. A failed sweep is logged, never fatal: the next tick tries again.
func sweepOutbox(ctx context.Context, drain outboxDrain, log *slog.Logger, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			published, err := drain(ctx)
			if err != nil {
				log.Warn("grades could not drain the outbox", slog.Any("error", err))
				continue
			}
			if published > 0 {
				log.Info("grades published stranded events", slog.Int("count", published))
			}
		}
	}
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

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
