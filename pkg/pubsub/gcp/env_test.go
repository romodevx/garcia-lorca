package gcp_test

import (
	"context"
	"testing"

	"github.com/garcia-lorca/pkg/logger"
	"github.com/garcia-lorca/pkg/pubsub"
	"github.com/garcia-lorca/pkg/pubsub/gcp"
)

// TestPublisherFromEnvUnconfigured is the standalone case: with no broker named
// in the environment, a producer must still boot, on the dev bus. This is the
// single decision every producer's composition root used to duplicate.
func TestPublisherFromEnvUnconfigured(t *testing.T) {
	t.Setenv("PUBSUB_EMULATOR_HOST", "")
	t.Setenv("PUBSUB_PROJECT_ID", "")

	publisher, err := gcp.PublisherFromEnv(context.Background(), logger.Default())
	if err != nil {
		t.Fatalf("PublisherFromEnv() error = %v, want nil (an unconfigured service must still boot)", err)
	}
	if _, ok := publisher.(*pubsub.LogPublisher); !ok {
		t.Fatalf("PublisherFromEnv() = %T, want *pubsub.LogPublisher", publisher)
	}
	if err := publisher.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

// TestPublisherFromEnvConfigured covers the other branch end to end: a named
// broker wins and the caller gets the real client. The Google client library
// reads PUBSUB_EMULATOR_HOST itself and dials lazily, so the selection is
// observable without an emulator running (the round trip through one is
// TestClientRoundTripAcksOnSuccess).
func TestPublisherFromEnvConfigured(t *testing.T) {
	t.Setenv("PUBSUB_EMULATOR_HOST", "127.0.0.1:8085")
	t.Setenv("PUBSUB_PROJECT_ID", emulatorProject)

	publisher, err := gcp.PublisherFromEnv(context.Background(), logger.Default())
	if err != nil {
		t.Fatalf("PublisherFromEnv() error = %v", err)
	}
	if _, ok := publisher.(*gcp.Client); !ok {
		t.Fatalf("PublisherFromEnv() = %T, want *gcp.Client", publisher)
	}
	t.Cleanup(func() { _ = publisher.Close() })
}
