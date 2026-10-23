// Command students is the entrypoint for the García Lorca students service.
//
// It serves the generated garcialorca.students.v1 Connect service (CreateApplicant,
// GetStudent, ListStudents, ChangeAdmissionStatus, Health) backed by the
// StudentService use case, persisting via Postgres (RLS per tenant) and
// publishing StudentEnrolled / AdmissionStatusChanged domain events.
//
// TRANSPORT NOTE: events go to Cloud Pub/Sub (student-events topic) whenever a
// broker is configured — the compose emulator locally, the real service in shared
// environments. With NO broker configured the service runs standalone on the dev
// log bus; a configured broker that cannot be reached fails startup instead (see
// gcp.PublisherFromEnv). The EventPublisher port is the same either way.
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	stdlog "log"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"
	_ "time/tzdata" // the school's zone must load in a distroless image

	connect "connectrpc.com/connect"
	"github.com/google/uuid"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/garcia-lorca/gen/go/garcialorca/students/v1/studentsv1connect"
	"github.com/garcia-lorca/pkg/dbdsn"
	"github.com/garcia-lorca/pkg/dbpool"
	"github.com/garcia-lorca/pkg/gcpauth"
	"github.com/garcia-lorca/pkg/gormlog"
	"github.com/garcia-lorca/pkg/logger"
	"golang.org/x/sync/errgroup"

	"github.com/garcia-lorca/pkg/pubsub"
	"github.com/garcia-lorca/pkg/pubsub/gcp"
	"github.com/garcia-lorca/services/students/internal/adapters/configclient"
	"github.com/garcia-lorca/services/students/internal/adapters/gcsphotos"
	grpcadapter "github.com/garcia-lorca/services/students/internal/adapters/grpc"
	"github.com/garcia-lorca/services/students/internal/adapters/iamclient"
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
	// defaultIAMBaseURL targets the iam service on its local dev port (compose
	// overrides it to the in-network address http://iam:8080).
	defaultIAMBaseURL = "http://localhost:8091"
	// defaultSchoolTimezone is the school's calendar (SCHOOL_TIMEZONE), as in
	// finance: a matrícula is numbered under the school's year.
	defaultSchoolTimezone = "America/Mexico_City"

	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 120 * time.Second
	// shutdownTimeout bounds the drain: past it, Shutdown stops waiting for
	// in-flight requests rather than holding the process open indefinitely.
	shutdownTimeout = 15 * time.Second
	outboxSweep     = 30 * time.Second

	// defaultPubSubProject matches the project the compose emulator starts with.
	defaultPubSubProject = "garcia-lorca-local"
	// The topic finance owns, and students' OWN subscription on it. A separate
	// subscription per consumer is the point: comms reads this topic too, and two
	// consumers sharing one subscription would each see half the events.
	paymentEventsTopic        = "payment-events"
	paymentEventsSubscription = "students-payment-events"
)

func main() {
	log := logger.Default()

	if err := run(context.Background(), log); err != nil {
		log.Error("students stopped", slog.Any("error", err))
		os.Exit(1)
	}
}

// run is the process lifecycle: build the composition root, serve until the
// listener stops, and release the event transport on the way out.
func run(ctx context.Context, log *slog.Logger) error {
	mux, events, consumer, drain, err := buildMux(ctx, log)
	if err != nil {
		return err
	}
	// The publisher owns the broker client's per-topic batching goroutines, so
	// releasing it belongs next to the code that built it.
	defer func() {
		if err := events.Close(); err != nil {
			log.Warn("students could not close the event publisher", slog.Any("error", err))
		}
	}()

	addr := envOr("ADDR", defaultAddr)
	drainAtStartup(ctx, drain, log)
	// h2c: serve HTTP/2 cleartext so both gRPC and Connect clients work.
	srv := &http.Server{
		Addr:              addr,
		Handler:           h2c.NewHandler(mux, &http2.Server{}),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	// Students publishes AND consumes: a pupil is en trámite until their
	// inscription is paid, and finance is what knows that. The group's context is
	// cancelled by the signal or by the first goroutine that fails, so a
	// subscriber that dies for good takes the server down and the platform
	// restarts it, rather than leaving a service that answers reads while
	// silently consuming nothing.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	group, groupCtx := errgroup.WithContext(runCtx)
	stopSweep := startSweep(groupCtx, drain, log, outboxSweep)
	defer stopSweep()

	group.Go(func() error {
		log.Info("students starting", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	group.Go(func() error {
		<-groupCtx.Done()
		// Detached from groupCtx on purpose: it is already cancelled here, so
		// draining needs a deadline of its own.
		shutdownCtx, shutdownCancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer shutdownCancel()
		return srv.Shutdown(shutdownCtx)
	})

	closeSubscriber, err := startSubscriber(groupCtx, group, consumer, log)
	if err != nil {
		cancel()
		if waitErr := group.Wait(); waitErr != nil {
			log.Warn("students stopped after a subscriber failure", slog.Any("error", waitErr))
		}
		return err
	}
	defer closeSubscriber()

	return group.Wait()
}

// startSubscriber runs the streaming-pull loop that feeds the payment consumer,
// or nothing at all when there is no broker to pull from.
//
// Under Cloud Run scale-to-zero nothing is running to call Receive, so events
// arrive by POST /pubsub instead and this loop must stay off: starting it would
// create durable subscriptions no process drains, and pupils would stay en
// trámite after paying. Terraform sets PUBSUB_PUSH_MODE there.
//
// With no broker at all the service runs API-only. Refusing to boot would make
// students undevelopable on its own; the cost is that admission does not follow
// payment until a broker exists, which is a local-development trade, not a
// production one.
func startSubscriber(ctx context.Context, group *errgroup.Group, consumer *pubsubadapter.Consumer, log *slog.Logger) (release func(), err error) {
	if pushMode() {
		log.Info("students in pubsub push mode; pull loop disabled (events arrive via POST /pubsub)")
		return func() {}, nil
	}
	if os.Getenv("PUBSUB_EMULATOR_HOST") == "" && os.Getenv("PUBSUB_PROJECT_ID") == "" {
		log.Warn("pubsub not configured; a paid inscription will not move a pupil out of trámite")
		return func() {}, nil
	}

	projectID := envOr("PUBSUB_PROJECT_ID", defaultPubSubProject)
	client, err := gcp.New(ctx, projectID)
	if err != nil {
		return nil, err
	}
	group.Go(func() error {
		log.Info("students consuming",
			slog.String("topic", paymentEventsTopic),
			slog.String("subscription", paymentEventsSubscription))
		return client.SubscribeTopic(ctx, paymentEventsTopic, paymentEventsSubscription, consumer.Handle)
	})
	return func() {
		if err := client.Close(); err != nil {
			log.Warn("students could not close the broker client", slog.Any("error", err))
		}
	}, nil
}

// pushMode reports whether students consumes via Pub/Sub push (Cloud Run) rather
// than the streaming-pull loop (local). An unset or unparseable value is false.
func pushMode() bool {
	enabled, err := strconv.ParseBool(os.Getenv("PUBSUB_PUSH_MODE"))
	return err == nil && enabled
}

// buildMux is the composition root: config → adapters → use case → Connect
// handler, returning the mounted routes plus the event publisher the caller must
// close on shutdown.
func buildMux(ctx context.Context, log *slog.Logger) (*http.ServeMux, pubsub.PublisherCloser, *pubsubadapter.Consumer, outboxDrain, error) {
	// M03 is single-tenant: the tenant is fixed here (not derived from the
	// caller), so RLS binds this one tenant for every request. When multi-tenant
	// lands, the gateway must forward the authenticated principal's TenantID and
	// this must stop being a process-wide constant. See ADR 0001.
	tenantID := envOr("STUDENTS_TENANT_ID", defaultTenant)

	zone, err := schoolLocation()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	db, err := openDB()
	if err != nil {
		return nil, nil, nil, nil, err
	}

	repo := pgadapter.NewRepository(db, tenantID)
	publisher, err := gcp.PublisherFromEnv(ctx, log)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	events := pubsubadapter.NewEventPublisher(publisher)
	newID := func() string { return uuid.NewString() }

	// The enroll knot validates group/term references against the config bounded
	// context by RPC (the app's first service-to-service call). The catalog
	// validator is a Connect client to config; the use case stays testable behind
	// the CatalogValidator port. http.DefaultClient speaks Connect over HTTP/1.1,
	// which is sufficient for these unary calls. The same client resolves a
	// pupil's level against the school's own catalog (ADR 0014).
	configBaseURL := envOr("CONFIG_BASE_URL", defaultConfigBaseURL)
	catalog := configclient.New(configBaseURL, gcpauth.HTTPClient(ctx, configBaseURL))
	svc := usecase.NewStudentService(repo, events, catalog, tenantID, newID).WithSchoolZone(zone)
	if photos, err := openPhotoStore(ctx, log); err != nil {
		return nil, nil, nil, nil, err
	} else if photos != nil {
		svc = svc.WithPhotos(photos)
	}
	enroll := usecase.NewEnrollmentService(repo, repo, catalog, events, tenantID, newID, time.Now).WithSchoolZone(zone)

	// Guardian links reference iam users, so the use case validates them against
	// the iam bounded context by RPC (the user must exist AND hold the guardian
	// role). Server-to-server and network-private, like the config client.
	iamBaseURL := envOr("IAM_BASE_URL", defaultIAMBaseURL)
	users := iamclient.New(iamBaseURL, gcpauth.HTTPClient(ctx, iamBaseURL))
	// repo is also the FamilyAccessRepository: every account write takes the
	// family locks and keeps the cap of two accounts per family (ADR 0041).
	guardians := usecase.NewGuardianService(repo, repo, repo, users, tenantID, newID)

	// A pupil is en trámite until their inscription is PAID, and finance is what
	// knows that. The consumer is how the two facts meet.
	consumer := pubsubadapter.NewConsumer(enroll, log)

	health := usecase.NewHealthService(serviceName)
	notes := usecase.NewNoteService(repo, repo, tenantID, newID, time.Now)
	// The virtual card's codes: 128 bits from the operating system's source.
	cards := usecase.NewCardService(repo, repo, tenantID, newID, cardRandom, time.Now)
	// Students' step of a level deletion (ADR 0021). Its trash is purged by
	// the gateway's hourly sweep (always warm, unlike this scale-to-zero
	// service) through PurgeLevelBatch, on config's restorable_until.
	levels := usecase.NewLevelArchiveService(repo, time.Now)
	// The legacy import (ADR 0033): no publisher on purpose — an import
	// announces nothing, and its enrollments carry origin=import for finance.
	imports := usecase.NewImportService(repo, catalog, catalog, tenantID, newID, time.Now).WithSchoolZone(zone)
	// What the import could not keep as written, per pupil (PLAN-import-avisos).
	issues := usecase.NewImportIssueService(repo, repo, time.Now)
	handler := grpcadapter.NewStudentsHandler(svc, enroll, guardians, health).
		WithNotes(notes).WithCards(cards).WithLevelArchive(levels).WithImports(imports).
		WithGroupRecords(usecase.NewGroupRecordService(repo)).WithImportIssues(issues).
		// Families (ADR 0041): siblings grouped by the office from suggestions.
		WithFamilies(usecase.NewFamilyService(repo, repo, tenantID, newID, time.Now))

	mux := http.NewServeMux()
	path, h := studentsv1connect.NewStudentsServiceHandler(handler, connect.WithRecover(recoverFn))
	mux.Handle(path, h)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	// The push endpoint Cloud Run needs: under scale-to-zero nothing is running
	// to pull, so events arrive as authenticated POSTs instead. Mounted always —
	// harmless locally, where the pull loop does the work.
	mux.Handle("POST /pubsub", gcp.NewPushHandler(consumer.Handle, os.Getenv("PUBSUB_PUSH_SA_EMAIL"), log))
	return mux, publisher, consumer, func(c context.Context) (int, error) {
		return repo.DrainOutbox(c, 100, events.PublishOutboxEvent)
	}, nil
}

type outboxDrain func(context.Context) (int, error)

const startupOutboxDrainTimeout = 2 * time.Second

func drainAtStartup(ctx context.Context, drain outboxDrain, log *slog.Logger) {
	drainCtx, cancel := context.WithTimeout(ctx, startupOutboxDrainTimeout)
	defer cancel()
	count, err := drain(drainCtx)
	if err != nil {
		log.Warn("students could not drain the outbox at startup", slog.Any("error", err))
		return
	}
	if count > 0 {
		log.Info("students published stranded events at startup", slog.Int("count", count))
	}
}

func startSweep(ctx context.Context, drain outboxDrain, log *slog.Logger, every time.Duration) func() {
	sweepCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-sweepCtx.Done():
				return
			case <-ticker.C:
				count, err := drain(sweepCtx)
				if err != nil {
					log.Warn("students could not drain the outbox", slog.Any("error", err))
					continue
				}
				if count > 0 {
					log.Info("students published stranded events", slog.Int("count", count))
				}
			}
		}
	}()
	return func() { cancel(); <-done }
}

// openDB opens the GORM Postgres pool, resolving its DSN via pkg/dbdsn:
// DATABASE_URL verbatim when set (docker-compose), otherwise assembled
// from the Cloud SQL split env (DB_SOCKET_DIR/DB_USER/DB_PASSWORD/DB_NAME).
func openDB() (*gorm.DB, error) {
	dsn, err := dbdsn.Resolve()
	if err != nil {
		return nil, err
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlog.New(stdlog.New(os.Stdout, "\r\n", stdlog.LstdFlags))})
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

// openPhotoStore opens the private bucket that keeps pupils' photos (task 5).
// PHOTOS_BUCKET unset means photos are off: the rest of the file still works
// and photo calls answer that they are not configured. Against the local
// emulator (STORAGE_EMULATOR_HOST) the bucket is created on start; in the cloud
// it is Terraform's, and the service account cannot create buckets.
func openPhotoStore(ctx context.Context, log *slog.Logger) (*gcsphotos.Store, error) {
	bucket := os.Getenv("PHOTOS_BUCKET")
	if bucket == "" {
		log.Warn("PHOTOS_BUCKET not set; pupils' photos are disabled")
		return nil, nil
	}
	store, err := gcsphotos.New(ctx, bucket)
	if err != nil {
		return nil, err
	}
	if os.Getenv("STORAGE_EMULATOR_HOST") != "" {
		if err := store.EnsureBucket(ctx, envOr("GOOGLE_CLOUD_PROJECT", "local")); err != nil {
			return nil, err
		}
	}
	log.Info("students photo store ready", slog.String("bucket", bucket))
	return store, nil
}

// schoolLocation is the school's time zone (SCHOOL_TIMEZONE, default
// America/Mexico_City). An unknown zone fails startup: falling back to UTC
// would give a pupil registered at 19:00 on 31 December next year's matrícula.
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

// cardRandom mints the 128 random bits of a card code (task 13).
func cardRandom() ([16]byte, error) {
	var b [16]byte
	_, err := rand.Read(b[:])
	return b, err
}
