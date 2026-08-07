package gcp

import (
	"context"
	"log/slog"
	"os"

	"github.com/garcia-lorca/pkg/pubsub"
)

// defaultProjectID matches the project the compose emulator starts with, so a
// local run needs only PUBSUB_EMULATOR_HOST.
const defaultProjectID = "garcia-lorca-local"

// PublisherFromEnv selects the event transport a producer publishes through,
// from the environment alone:
//
//   - neither PUBSUB_EMULATOR_HOST nor PUBSUB_PROJECT_ID set ⇒ no broker is
//     configured, so the service runs on pubsub.NewLogPublisher and still boots
//     standalone. Publishing is then visible in the log and nowhere else.
//   - either set ⇒ the broker IS configured, and the Cloud Pub/Sub client is
//     built for PUBSUB_PROJECT_ID (defaulting to the local compose project).
//     A client that cannot be built is returned as an ERROR: startup must fail.
//
// That asymmetry is the point. Falling back to the dev bus when a configured
// broker is broken would leave a service that serves reads and passes its
// healthcheck while silently dropping every event — consumers would go hungry
// with nothing failing. An unconfigured service dropping events is a local
// convenience; a configured one doing it is data loss.
//
// The caller OWNS the returned publisher and must Close it when the process
// shuts down: the Cloud Pub/Sub client keeps a batching goroutine per topic.
// Closing the dev bus is a no-op, so the release path needs no special case.
func PublisherFromEnv(ctx context.Context, log *slog.Logger) (pubsub.PublisherCloser, error) {
	return selectPublisher(ctx, log, New)
}

// selectPublisher is PublisherFromEnv with the client constructor injected, which
// is what makes the "configured but unbuildable" branch testable: dialling is
// lazy, so a real client is built successfully even against nothing.
func selectPublisher(
	ctx context.Context,
	log *slog.Logger,
	build func(ctx context.Context, projectID string) (*Client, error),
) (pubsub.PublisherCloser, error) {
	if os.Getenv("PUBSUB_EMULATOR_HOST") == "" && os.Getenv("PUBSUB_PROJECT_ID") == "" {
		log.Warn("pubsub not configured; events go to the dev log bus")
		return pubsub.NewLogPublisher(log), nil
	}

	projectID := envOr("PUBSUB_PROJECT_ID", defaultProjectID)
	client, err := build(ctx, projectID)
	if err != nil {
		return nil, err
	}
	log.Info("publishing events to Cloud Pub/Sub", slog.String("project", projectID))
	return client, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
