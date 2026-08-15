package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/garcia-lorca/pkg/dbdsn"
	"github.com/garcia-lorca/pkg/logger"
)

// waitTimeout bounds every assertion about a goroutine that must NOT exist. It is
// a failure deadline, not a sleep: the passing path returns immediately.
const waitTimeout = 2 * time.Second

// TestStartSubscribersAPIOnly covers the standalone boot: with no broker in the
// environment comms serves its API and consumes nothing. What is asserted is that
// NOTHING was added to the group — a subscriber goroutine would block until the
// context is cancelled, so a Wait that returns while the context is still live
// proves the loop was never started and nothing was left running.
func TestStartSubscribersAPIOnly(t *testing.T) {
	t.Setenv("PUBSUB_EMULATOR_HOST", "")
	t.Setenv("PUBSUB_PROJECT_ID", "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	group, groupCtx := errgroup.WithContext(ctx)

	release, err := startSubscribers(groupCtx, group, nil, logger.Default())
	if err != nil {
		t.Fatalf("startSubscribers() error = %v, want nil (an unconfigured comms must still boot)", err)
	}
	if release == nil {
		t.Fatal("startSubscribers() returned a nil release; the caller always runs it")
	}
	release()

	waited := make(chan error, 1)
	go func() { waited <- group.Wait() }()
	select {
	case err := <-waited:
		if err != nil {
			t.Fatalf("Wait() = %v, want nil", err)
		}
	case <-time.After(waitTimeout):
		t.Fatal("Wait() blocked with no broker configured: a subscriber goroutine was started anyway")
	}
}

// TestStartSubscribersPushModeSkipsPullLoop is the scale-to-zero guard: on Cloud
// Run events arrive by POST /pubsub, so the streaming-pull loop must NOT start
// even though a broker is configured (PUBSUB_PROJECT_ID is set). Nothing was
// added to the group — a Wait that returns while the context is still live proves
// no subscriber goroutine was started — and the release is the usual no-op.
func TestStartSubscribersPushModeSkipsPullLoop(t *testing.T) {
	t.Setenv("PUBSUB_PUSH_MODE", "true")
	t.Setenv("PUBSUB_PROJECT_ID", "garcia-lorca-local") // a broker IS configured
	t.Setenv("PUBSUB_EMULATOR_HOST", "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	group, groupCtx := errgroup.WithContext(ctx)

	release, err := startSubscribers(groupCtx, group, nil, logger.Default())
	if err != nil {
		t.Fatalf("startSubscribers() error = %v, want nil (push mode must not fail startup)", err)
	}
	if release == nil {
		t.Fatal("startSubscribers() returned a nil release; the caller always runs it")
	}
	release()

	waited := make(chan error, 1)
	go func() { waited <- group.Wait() }()
	select {
	case err := <-waited:
		if err != nil {
			t.Fatalf("Wait() = %v, want nil", err)
		}
	case <-time.After(waitTimeout):
		t.Fatal("Wait() blocked in push mode: the pull subscriber loop was started anyway")
	}
}

// TestConsumedTopicsAreOnlyWhatCommsRenders guards the pairing between the
// subscriptions this process creates and the events the service can actually turn
// into a notification. A topic subscribed but not rendered is worse than useless:
// it creates a durable subscription that accumulates unacked events and warns on
// every one of them. student-events is the case that happened — it returns when
// comms renders enrollment events.
func TestConsumedTopicsAreOnlyWhatCommsRenders(t *testing.T) {
	want := map[string]string{
		"payment-events":    "comms-payment-events",
		"attendance-events": "comms-attendance-events",
	}

	if len(consumedTopics) != len(want) {
		t.Fatalf("consumedTopics = %v, want %v", consumedTopics, want)
	}
	for topic, subscription := range want {
		got, ok := consumedTopics[topic]
		if !ok {
			t.Errorf("no subscription for %q", topic)
			continue
		}
		if got != subscription {
			t.Errorf("subscription for %q = %q, want %q", topic, got, subscription)
		}
	}
	for topic, subscription := range consumedTopics {
		// One subscription per topic, named after this service, so a second
		// consumer of the same events never competes with comms for them.
		if !strings.HasPrefix(subscription, "comms-") {
			t.Errorf("subscription %q on %q is not owned by comms", subscription, topic)
		}
	}
}

// TestStopGroupJoinsEveryGoroutine is the leak fix: when the subscriber setup
// fails, the group already owns the server and its shutdown watcher — goroutines
// that run until the supervised context is cancelled. stopGroup must cancel AND
// join them, or run returns while both are still alive.
func TestStopGroupJoinsEveryGoroutine(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	group, groupCtx := errgroup.WithContext(ctx)

	// Two goroutines shaped like the real ones: they end only on cancellation.
	stopped := make(chan struct{}, 2)
	for range 2 {
		group.Go(func() error {
			<-groupCtx.Done()
			stopped <- struct{}{}
			return nil
		})
	}

	joined := make(chan struct{})
	go func() {
		stopGroup(cancel, group, logger.Default())
		close(joined)
	}()

	select {
	case <-joined:
	case <-time.After(waitTimeout):
		t.Fatal("stopGroup() blocked: the supervised goroutines were never cancelled")
	}
	if len(stopped) != 2 {
		t.Errorf("%d of 2 goroutines finished before stopGroup() returned; the rest were left running", len(stopped))
	}
}

// TestRunRequiresADatabase pins the one configuration comms cannot start without.
// It is also the guard on run's early-return path: the feed is durable storage, so
// booting without it would serve empty reads and drop every consumed event.
func TestRunRequiresADatabase(t *testing.T) {
	// Neither DATABASE_URL nor the Cloud SQL split vars are set, so dbdsn.Resolve
	// cannot assemble a DSN and run must fail fast rather than boot databaseless.
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DB_SOCKET_DIR", "")
	t.Setenv("DB_USER", "")
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("DB_NAME", "")
	t.Setenv("PUBSUB_EMULATOR_HOST", "")
	t.Setenv("PUBSUB_PROJECT_ID", "")

	done := make(chan error, 1)
	go func() { done <- run(context.Background(), logger.Default()) }()

	select {
	case err := <-done:
		if !errors.Is(err, dbdsn.ErrMissingConfig) {
			t.Fatalf("run() = %v, want dbdsn.ErrMissingConfig", err)
		}
	case <-time.After(waitTimeout):
		t.Fatal("run() did not return without DATABASE_URL")
	}
}
