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

	"github.com/garcia-lorca/gen/go/garcialorca/finance/v1/financev1connect"
	"github.com/garcia-lorca/pkg/dbdsn"
	"github.com/garcia-lorca/pkg/dbpool"
	"github.com/garcia-lorca/pkg/gcpauth"
	"github.com/garcia-lorca/pkg/gormlog"
	"github.com/garcia-lorca/pkg/logger"
	"golang.org/x/sync/errgroup"

	"github.com/garcia-lorca/pkg/pubsub"
	"github.com/garcia-lorca/pkg/pubsub/gcp"
	"github.com/garcia-lorca/services/finance/internal/adapters/configclient"
	"github.com/garcia-lorca/services/finance/internal/adapters/gcsreceipts"
	grpcadapter "github.com/garcia-lorca/services/finance/internal/adapters/grpc"
	pgadapter "github.com/garcia-lorca/services/finance/internal/adapters/postgres"
	pubsubadapter "github.com/garcia-lorca/services/finance/internal/adapters/pubsub"
	"github.com/garcia-lorca/services/finance/internal/adapters/studentsclient"
	"github.com/garcia-lorca/services/finance/internal/ports"
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
	// The topic students owns, and finance's OWN subscription on it. A separate
	// subscription per consumer is the point: comms reads the same topic, and
	// two consumers sharing one subscription would each see half the events.
	studentEventsTopic        = "student-events"
	studentEventsSubscription = "finance-student-events"

	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	// outboxSweep is how often stranded events are looked for. Seconds, not
	// minutes: the events it rescues are the ones that decide whether a pupil
	// who paid shows as inscribed, and a demo runs every day.
	outboxSweep = 30 * time.Second

	// defaultSchoolTimezone is the school's calendar (SCHOOL_TIMEZONE), as in
	// gateway, comms, attendance and grades. Finance's "today" is its day.
	defaultSchoolTimezone = "America/Mexico_City"

	idleTimeout = 120 * time.Second
	// shutdownTimeout bounds the drain: past it, Shutdown stops waiting for
	// in-flight requests rather than holding the process open indefinitely.
	shutdownTimeout = 15 * time.Second
)

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
	mux, events, consumer, drain, err := buildMux(ctx, log)
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
	// h2c: serve HTTP/2 cleartext so both gRPC and Connect clients work.
	srv := &http.Server{
		Addr:              addr,
		Handler:           h2c.NewHandler(mux, &http2.Server{}),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	// Finance is a producer AND, since M15, a consumer: StudentEnrolled is what
	// makes the school's rule true — the inscription is charged when the pupil
	// takes a place in a group, not when somebody bills the whole roll.
	//
	// The group's context is cancelled by the signal OR by the first goroutine
	// that fails, so the two halves stop together: a subscriber that dies for
	// good takes the server down and the platform restarts the process, rather
	// than leaving a ledger that answers reads while silently consuming nothing.
	//
	// The cancel handle is ours because errgroup's own context is cancelled only
	// when one of ITS goroutines fails, and an error on run's path (the
	// subscriber setup) has to be able to stop what the group already owns.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	group, groupCtx := errgroup.WithContext(runCtx)

	group.Go(func() error {
		log.Info("finance starting", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	group.Go(func() error {
		<-groupCtx.Done()
		// Detached from groupCtx on purpose: that context is already cancelled
		// here, so draining needs a deadline of its own.
		shutdownCtx, shutdownCancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer shutdownCancel()
		return srv.Shutdown(shutdownCtx)
	})

	// The outbox sweep. A payment drains its own events the moment it commits,
	// so this is for what that cannot cover: the broker was down, or the process
	// died between the commit and the send. Without it a family could pay and
	// the pupil stay "en trámite" forever, with nothing able to notice.
	group.Go(func() error {
		ticker := time.NewTicker(outboxSweep)
		defer ticker.Stop()
		for {
			select {
			case <-groupCtx.Done():
				return nil
			case <-ticker.C:
				published, err := drain(groupCtx)
				if err != nil {
					// Never fatal: a sweep that cannot run must not take the
					// ledger down. The next tick tries again.
					log.Warn("finance could not drain the outbox", slog.Any("error", err))
					continue
				}
				if published > 0 {
					// Only when it did something. A line every thirty seconds
					// saying "nothing" is a log nobody reads.
					log.Info("finance published stranded events", slog.Int("count", published))
				}
			}
		}
	})

	closeSubscriber, err := startSubscriber(groupCtx, group, consumer, log)
	if err != nil {
		// The group already owns the server and its shutdown watcher; returning
		// now would leave both running behind a run() that reported the process
		// dead. Stop and join them first.
		cancel()
		if waitErr := group.Wait(); waitErr != nil {
			log.Warn("finance stopped after a subscriber failure", slog.Any("error", waitErr))
		}
		return err
	}
	// Deferred, so the broker client is released only once Wait has seen the
	// consumer return: closing it mid-drain would cut the acks of messages
	// already handled.
	defer closeSubscriber()

	return group.Wait()
}

// buildMux is the composition root: config → adapters (both reader clients,
// repo and publisher) → use case → Connect handler, returning the mounted routes
// plus the event publisher the caller must close on shutdown.
func buildMux(ctx context.Context, log *slog.Logger) (*http.ServeMux, pubsub.PublisherCloser, *pubsubadapter.Consumer, func(context.Context) (int, error), error) {
	// Single-tenant for this phase: the tenant is fixed here (not derived from
	// the caller), so RLS binds this one tenant for every request. See ADR 0001.
	tenantID := envOr("FINANCE_TENANT_ID", defaultTenant)

	// The school's zone, resolved ONCE: every "today" the service decides —
	// surcharge days, overdue cutoffs, the early window, the run's default
	// date, the account's "Al {fecha}" — is Morelia's day (ADR 0035).
	zone, err := schoolLocation()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	clock := usecase.NewSchoolClock(time.Now, zone)

	db, err := openDB()
	if err != nil {
		return nil, nil, nil, nil, err
	}

	repo := pgadapter.NewRepository(db, tenantID)

	// The ledger validates its cross-service references by RPC. One students
	// client serves both seams (StudentReader for the debtor, EnrollmentReader
	// for bulk generation) because both read the same bounded context; config
	// answers for the cycle. The use case stays testable behind the ports.
	// http.DefaultClient speaks Connect over HTTP/1.1, sufficient for these
	// unary calls.
	studentsBaseURL := envOr("STUDENTS_BASE_URL", defaultStudentsBaseURL)
	configBaseURL := envOr("CONFIG_BASE_URL", defaultConfigBaseURL)
	studentsClient := studentsclient.New(studentsBaseURL, gcpauth.HTTPClient(ctx, studentsBaseURL))
	// How long the school calendar is held, and how long a read of it may take.
	// Minutes, not per cycle: a holiday entered this morning has to be visible
	// this morning. Seconds, not indefinitely: a cashier is standing in front of
	// a parent, and a slow config service must not become a slow counter.
	const (
		calendarTTL     = 5 * time.Minute
		calendarTimeout = 3 * time.Second
	)

	cyclesClient := configclient.New(configBaseURL, gcpauth.HTTPClient(ctx, configBaseURL))

	events, err := gcp.PublisherFromEnv(ctx, log)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	publisher := pubsubadapter.NewEventPublisher(events)

	newID := func() string { return uuid.NewString() }
	// cyclesClient is the config client twice over: it answers both "is this a
	// real school year" and "which level does this group belong to".
	svc := usecase.NewFinanceService(repo, studentsClient, cyclesClient, studentsClient, cyclesClient,
		configclient.NewCachedCalendar(cyclesClient, calendarTTL, calendarTimeout), publisher, tenantID, newID, clock)
	health := usecase.NewHealthService(serviceName)
	// Level deletion (ADR 0021): finance's step of the gateway's saga (tuition
	// plans only; money is never touched). The gateway's hourly sweep (always
	// warm, unlike this scale-to-zero service) purges each batch past config's
	// restorable_until through PurgeLevelBatch.
	levels := usecase.NewLevelArchiveService(repo, time.Now)
	// T10: expenses and the cash cut. Receipts live in a private bucket; with
	// EXPENSE_RECEIPTS_BUCKET unset, expenses work and receipts are off.
	receipts, err := openReceiptStore(ctx, log)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	expenses := usecase.NewExpenseService(repo, svc, receipts, newID, clock)
	// T17: activities. Their periods are issued by the same billing run, so the
	// run learns of them here and enrolling charges what is already due.
	svc.WithActivities(repo)
	activities := usecase.NewActivityService(repo, studentsClient, svc, newID, clock)
	// T18: scholarships. Every read and payment prices them (ADR 0022); the
	// service only manages them and who holds them.
	svc.WithScholarships(repo)
	scholarships := usecase.NewScholarshipService(repo, svc, repo, repo, studentsClient, tenantID, newID, clock)
	// ADR 0040: a deposit may be registered by a family's payment voucher, and
	// one registered by hand at a voucher-aware counter (voucher_aware, set by
	// the gateway while module_payment_vouchers is on) links the pupil's only
	// pending voucher. Finance reads no flag; the request says whether the
	// counter can see vouchers.
	svc.WithPaymentVouchers(repo)
	// The vouchers themselves share the expense receipts' private bucket
	// (prefix vouchers/); with no bucket, uploads answer NotFound.
	vouchers := usecase.NewPaymentVoucherService(repo, studentsClient, receipts, newID, clock)
	handler := grpcadapter.NewFinanceHandler(svc, health).WithLevelArchive(levels).WithExpenses(expenses).
		WithActivities(activities).WithScholarships(scholarships).WithPaymentVouchers(vouchers)
	consumer := pubsubadapter.NewConsumer(svc, log)

	mux := http.NewServeMux()
	path, h := financev1connect.NewFinanceServiceHandler(handler, connect.WithRecover(recoverFn))
	mux.Handle(path, h)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	// The push endpoint Cloud Run needs: under scale-to-zero nothing is running
	// to pull, so events arrive as authenticated POSTs instead. Mounted always —
	// harmless locally, where the pull loop below does the work.
	mux.Handle("POST /pubsub", gcp.NewPushHandler(consumer.Handle, os.Getenv("PUBSUB_PUSH_SA_EMAIL"), log))
	return mux, events, consumer, func(c context.Context) (int, error) { return svc.DrainOutbox(c, 0) }, nil
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

// openReceiptStore opens the private bucket that keeps expense receipts (T10)
// — the mechanism of the pupils' photos. EXPENSE_RECEIPTS_BUCKET unset means
// receipts are off (nil store). Against the local emulator the bucket is
// created on start; in the cloud it is Terraform's.
func openReceiptStore(ctx context.Context, log *slog.Logger) (ports.ReceiptStore, error) {
	bucket := os.Getenv("EXPENSE_RECEIPTS_BUCKET")
	if bucket == "" {
		log.Warn("EXPENSE_RECEIPTS_BUCKET not set; expense receipts are disabled")
		return nil, nil
	}
	store, err := gcsreceipts.New(ctx, bucket)
	if err != nil {
		return nil, err
	}
	if os.Getenv("STORAGE_EMULATOR_HOST") != "" {
		if err := store.EnsureBucket(ctx, envOr("GOOGLE_CLOUD_PROJECT", "local")); err != nil {
			return nil, err
		}
	}
	log.Info("finance receipt store ready", slog.String("bucket", bucket))
	return store, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// startSubscriber runs the streaming-pull loop that feeds the enrolment consumer,
// or nothing at all when there is no broker to pull from.
//
// Under Cloud Run scale-to-zero nothing is running to call Receive, so events
// arrive by POST /pubsub instead and this loop must stay off: starting it would
// create durable subscriptions no process drains, and the school's inscriptions
// would silently queue. Terraform sets PUBSUB_PUSH_MODE there; docker-compose
// leaves it unset and keeps the loop.
//
// With no broker at all the ledger runs API-only. Refusing to boot would make
// finance undevelopable on its own, and the monthly run still issues every
// inscription the events would have — the event buys immediacy, not correctness.
func startSubscriber(ctx context.Context, group *errgroup.Group, consumer *pubsubadapter.Consumer, log *slog.Logger) (release func(), err error) {
	if pushMode() {
		log.Info("finance in pubsub push mode; pull loop disabled (events arrive via POST /pubsub)")
		return func() {}, nil
	}
	if os.Getenv("PUBSUB_EMULATOR_HOST") == "" && os.Getenv("PUBSUB_PROJECT_ID") == "" {
		log.Warn("pubsub not configured; finance will not bill inscriptions on enrolment (the monthly run still will)")
		return func() {}, nil
	}

	projectID := envOr("PUBSUB_PROJECT_ID", defaultPubSubProject)
	client, err := gcp.New(ctx, projectID)
	if err != nil {
		return nil, err
	}
	group.Go(func() error {
		log.Info("finance consuming",
			slog.String("topic", studentEventsTopic),
			slog.String("subscription", studentEventsSubscription))
		return client.SubscribeTopic(ctx, studentEventsTopic, studentEventsSubscription, consumer.Handle)
	})
	return func() {
		if err := client.Close(); err != nil {
			log.Warn("finance could not close the broker client", slog.Any("error", err))
		}
	}, nil
}

// pushMode reports whether finance consumes via Pub/Sub push (Cloud Run) rather
// than the streaming-pull loop (local). An unset or unparseable value is false,
// so the safe default is the loop the emulator relies on.
func pushMode() bool {
	enabled, err := strconv.ParseBool(os.Getenv("PUBSUB_PUSH_MODE"))
	return err == nil && enabled
}

// schoolLocation is the school's time zone (SCHOOL_TIMEZONE, default
// America/Mexico_City). An unknown zone fails startup: falling back to UTC
// would bring back the day that starts at 18:00 in Morelia (ADR 0035).
func schoolLocation() (*time.Location, error) {
	name := envOr("SCHOOL_TIMEZONE", defaultSchoolTimezone)
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("SCHOOL_TIMEZONE %q: %w", name, err)
	}
	return loc, nil
}
