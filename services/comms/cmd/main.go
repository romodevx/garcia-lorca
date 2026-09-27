// Command comms is the entrypoint for the García Lorca comms service.
//
// Comms is BOTH a server and a consumer, and the two run side by side in one
// process:
//
//   - the generated garcialorca.comms.v1 Connect service — device registration
//     plus reading the notification feed materialised for a student;
//   - a Pub/Sub subscriber loop over the domain events other services own
//     (payment-events, attendance-events, grades-events — see consumedTopics), which is what
//     fills that feed. Comms PUBLISHES NOTHING: sending is a side effect of
//     consuming an event, never an RPC.
//
// Both share one cancellable context, so a SIGINT/SIGTERM stops consuming and
// drains the server together (see run).
//
// TRANSPORT NOTE: without a broker configured (no PUBSUB_EMULATOR_HOST and no
// PUBSUB_PROJECT_ID) the subscriber is not started and the service runs API-only,
// so it still boots standalone — the feed is then whatever is already stored.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	connect "connectrpc.com/connect"
	"github.com/google/uuid"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/sync/errgroup"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/garcia-lorca/gen/go/garcialorca/comms/v1/commsv1connect"
	"github.com/garcia-lorca/pkg/dbdsn"
	"github.com/garcia-lorca/pkg/dbpool"
	"github.com/garcia-lorca/pkg/gcpauth"
	"github.com/garcia-lorca/pkg/logger"
	"github.com/garcia-lorca/pkg/pubsub/gcp"
	"github.com/garcia-lorca/services/comms/internal/adapters/expo"
	"github.com/garcia-lorca/services/comms/internal/adapters/fcm"
	"github.com/garcia-lorca/services/comms/internal/adapters/studentsclient"
	"github.com/garcia-lorca/services/comms/internal/handler"
	repoadapter "github.com/garcia-lorca/services/comms/internal/repo"
	"github.com/garcia-lorca/services/comms/internal/service"
)

const (
	defaultAddr   = ":8080"
	defaultTenant = "00000000-0000-0000-0000-000000000001"
	// defaultPubSubProject matches the project the compose emulator starts with.
	defaultPubSubProject = "garcia-lorca-local"
	// defaultStudentsBaseURL is the students service on its local port; compose
	// overrides it with the in-network http://students:8080.
	defaultStudentsBaseURL = "http://localhost:8092"

	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 120 * time.Second
	// shutdownTimeout bounds the drain of in-flight requests once a signal has
	// arrived. The subscriber loop needs no such bound: cancelling the context
	// makes Receive stop after the handlers it has already started.
	shutdownTimeout = 15 * time.Second
)

// consumedTopics maps every topic comms consumes to the durable subscription it
// owns on that topic. One subscription PER TOPIC, named after this service, so a
// second consumer of the same events never competes with comms for them — and so
// a restart resumes where it left off instead of replaying from nothing.
//
// It lists ONLY topics the service renders (see internal/service.render). This
// phase notifies on absence, attendance, payment and report-card (grades-events,
// T7) events; student-events
// (StudentEnrolled, AdmissionStatusChanged) is deliberately absent, because
// subscribing to events nothing renders is worse than not subscribing at all — it
// creates a durable subscription whose backlog only grows while every delivery is
// logged as undeliverable. It returns together with the copy for enrollment
// events.
var consumedTopics = map[string]string{
	"payment-events":    "comms-payment-events",
	"attendance-events": "comms-attendance-events",
	"grades-events":     "comms-grades-events",
}

func main() {
	log := logger.Default()

	// The signal context is the one both halves of the process watch: a
	// SIGINT/SIGTERM cancels it, which stops the subscriber loop and starts the
	// server's drain.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, log); err != nil {
		log.Error("comms stopped", slog.Any("error", err))
		os.Exit(1)
	}
	log.Info("comms stopped cleanly")
}

// run is the composition root: config → adapters (repo, push sender) → service →
// the Connect handler and the subscriber loop, both supervised until ctx is
// cancelled. It returns nil when the shutdown was the graceful one.
func run(ctx context.Context, log *slog.Logger) error {
	// Single-tenant for this phase: the tenant is fixed here (not derived from
	// the caller), so RLS binds this one tenant for every request, and it is also
	// the tenant a consumed event must belong to. See ADR 0001.
	tenantID := envOr("COMMS_TENANT_ID", defaultTenant)

	db, err := openDB()
	if err != nil {
		return err
	}

	repo := repoadapter.New(db, tenantID)
	// WHO a notification reaches is students' knowledge, read by RPC on every
	// push: the student's guardians, then the devices those users registered here.
	// http.DefaultClient speaks Connect over HTTP/1.1, sufficient for this unary
	// call (mirrors attendance and grades).
	studentsBaseURL := envOr("STUDENTS_BASE_URL", defaultStudentsBaseURL)
	guardians := studentsclient.New(studentsBaseURL, gcpauth.HTTPClient(ctx, studentsBaseURL))
	// Push is the courtesy on top of the durable feed. PUSH_PROVIDER=expo delivers
	// through the Expo Push Service (EXPO_ACCESS_TOKEN optional, required once
	// enhanced push security is on); anything unset keeps the logging sender.
	sender, err := selectSender(os.Getenv("PUSH_PROVIDER"), os.Getenv("EXPO_ACCESS_TOKEN"), log)
	if err != nil {
		return err
	}
	// The same students client names the pupil in the copy ("Ana faltó…").
	svc := service.NewCommsService(repo, guardians, sender, tenantID, uuid.NewString, time.Now, log,
		service.WithStudentNames(guardians))

	mux := http.NewServeMux()
	path, connectHandler := commsv1connect.NewCommsServiceHandler(handler.NewCommsHandler(svc, tenantID), connect.WithRecover(recoverFn))
	mux.Handle(path, connectHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	// The Pub/Sub PUSH receiver: Cloud Run delivers events here when the process
	// is scaled to zero (see startSubscribers). It dispatches to the SAME
	// idempotent consumer as the pull loop, so it is always mounted — harmless in
	// pull mode, where no push delivery ever arrives. The route is method-aware,
	// so anything but POST gets a 405.
	//
	// PUBSUB_PUSH_SA_EMAIL, when set by Terraform, pins the caller to the push
	// subscription's service account (defense-in-depth over the coarse run.invoker
	// gate); unset locally, the receiver skips that check.
	pushSA := os.Getenv("PUBSUB_PUSH_SA_EMAIL")
	mux.Handle("POST /pubsub", gcp.NewPushHandler(svc.HandleEvent, pushSA, log))

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

	// The group's context is cancelled by the signal OR by the first goroutine
	// that fails, which is what makes the two halves stop together: a subscriber
	// that dies for good takes the server down with it and the platform restarts
	// the process, rather than leaving a service that answers reads while
	// silently consuming nothing.
	//
	// The cancel handle is ours because errgroup's own context is cancelled only
	// when one of ITS goroutines fails: an error on run's path (the subscriber
	// setup below) has to be able to stop the goroutines the group already owns.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	group, groupCtx := errgroup.WithContext(runCtx)

	group.Go(func() error {
		log.Info("comms serving", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	group.Go(func() error {
		<-groupCtx.Done()
		// Detached from groupCtx on purpose: that context is already cancelled
		// here, so draining needs a deadline of its own.
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	})

	closeSubscribers, err := startSubscribers(groupCtx, group, svc, log)
	if err != nil {
		// The group already owns the server and its shutdown watcher. Returning
		// straight away would leave both running behind a run() that has reported
		// the process dead, so they are stopped and joined first.
		stopGroup(cancel, group, log)
		return err
	}
	// Deferred, so the broker client is released only once Wait has seen every
	// consumer return: closing it while a Receive is still draining would cut the
	// acks of messages already handled.
	defer closeSubscribers()

	return group.Wait()
}

// stopGroup cancels the supervised context and joins every goroutine the group
// owns, so no exit path leaves one behind. Its error is only logged: it is the
// consequence of a failure the caller is already returning, and reporting it
// instead would hide the cause.
func stopGroup(cancel context.CancelFunc, group *errgroup.Group, log *slog.Logger) {
	cancel()
	if err := group.Wait(); err != nil {
		log.Warn("comms stopped its running goroutines after a startup failure", slog.Any("error", err))
	}
}

// startSubscribers adds one supervised consumer per topic to group and returns the
// release of the broker client the caller must run after Wait. Each consumer
// blocks in SubscribeTopic — get-or-create, so a blank emulator works —
// dispatching to the service's idempotent handler until ctx is cancelled, then
// returns nil.
//
// With no broker configured the service runs API-only and the returned release is
// a no-op: consuming is how the feed is filled, but refusing to boot without a
// broker would make the read side of the module undevelopable on its own.
func startSubscribers(ctx context.Context, group *errgroup.Group, svc *service.CommsService, log *slog.Logger) (release func(), err error) {
	// Under Cloud Run scale-to-zero nothing runs to call Receive, so events are
	// delivered by POST /pubsub instead (see run's route) and the streaming-pull
	// loop must stay off — starting it would create durable subscriptions no
	// running process drains. Terraform sets PUBSUB_PUSH_MODE on Cloud Run; local
	// docker-compose leaves it unset and keeps the pull loop below.
	if pushMode() {
		log.Info("comms in pubsub push mode; pull subscriber loop disabled (events arrive via POST /pubsub)")
		return func() {}, nil
	}

	if os.Getenv("PUBSUB_EMULATOR_HOST") == "" && os.Getenv("PUBSUB_PROJECT_ID") == "" {
		log.Warn("pubsub not configured; comms runs API-only")
		return func() {}, nil
	}

	projectID := envOr("PUBSUB_PROJECT_ID", defaultPubSubProject)
	client, err := gcp.New(ctx, projectID)
	if err != nil {
		return nil, err
	}

	for topic, subscription := range consumedTopics {
		group.Go(func() error {
			log.Info("comms consuming",
				slog.String("topic", topic),
				slog.String("subscription", subscription))
			return client.SubscribeTopic(ctx, topic, subscription, svc.HandleEvent)
		})
	}
	log.Info("comms subscriber loop started",
		slog.String("project", projectID),
		slog.Int("topic_count", len(consumedTopics)))

	return func() {
		if err := client.Close(); err != nil {
			log.Warn("comms could not close the broker client", slog.Any("error", err))
		}
	}, nil
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

// pushMode reports whether comms consumes via Pub/Sub push (Cloud Run) rather
// than the streaming-pull loop (local docker-compose). Terraform sets
// PUBSUB_PUSH_MODE on Cloud Run; an unset or unparseable value is false, so the
// safe default is the pull loop the emulator relies on.
func pushMode() bool {
	enabled, err := strconv.ParseBool(os.Getenv("PUBSUB_PUSH_MODE"))
	return err == nil && enabled
}

// The push providers PUSH_PROVIDER accepts.
const (
	pushProviderLog  = "log"
	pushProviderExpo = "expo"
)

// selectSender builds the push sender PUSH_PROVIDER names: "" or "log" logs
// instead of sending (development, or no credentials), "expo" delivers through
// the Expo Push Service. Any other value fails startup: a misspelt provider must
// not silently turn every push into a log line.
func selectSender(provider, expoAccessToken string, log *slog.Logger) (service.Sender, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", pushProviderLog:
		return fcm.NewLogSender(log), nil
	case pushProviderExpo:
		log.Info("comms push via Expo", slog.Bool("access_token", expoAccessToken != ""))
		return expo.NewSender(expo.WithAccessToken(expoAccessToken), expo.WithLogger(log)), nil
	default:
		return nil, fmt.Errorf("PUSH_PROVIDER %q is not one of log, expo", provider)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
