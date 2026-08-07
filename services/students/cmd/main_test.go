package main

import (
	"context"
	"testing"

	"github.com/garcia-lorca/pkg/logger"
	"github.com/garcia-lorca/pkg/pubsub/gcp"
)

// TestSelectPublisher covers the only decision this composition root makes about
// the event transport: a configured broker wins, and a bare environment keeps
// the service runnable standalone on the dev bus.
func TestSelectPublisher(t *testing.T) {
	log := logger.Default()

	t.Run("dev bus when no broker is configured", func(t *testing.T) {
		t.Setenv("PUBSUB_EMULATOR_HOST", "")
		t.Setenv("PUBSUB_PROJECT_ID", "")

		publisher := selectPublisher(context.Background(), log)
		if _, ok := publisher.(*devBus); !ok {
			t.Fatalf("selectPublisher() = %T, want *devBus", publisher)
		}
	})

	t.Run("cloud client when the emulator is configured", func(t *testing.T) {
		// The Google client library reads PUBSUB_EMULATOR_HOST itself and dials
		// lazily, so this needs no emulator running.
		t.Setenv("PUBSUB_EMULATOR_HOST", "127.0.0.1:8085")
		t.Setenv("PUBSUB_PROJECT_ID", "garcia-lorca-test")

		publisher := selectPublisher(context.Background(), log)
		client, ok := publisher.(*gcp.Client)
		if !ok {
			t.Fatalf("selectPublisher() = %T, want *gcp.Client", publisher)
		}
		t.Cleanup(func() { _ = client.Close() })
	})
}
