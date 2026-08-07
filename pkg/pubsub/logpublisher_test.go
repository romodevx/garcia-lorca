package pubsub_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/garcia-lorca/pkg/pubsub"
)

// TestLogPublisherLogsTheEventMetadata pins what the dev bus is for: a service
// running without a broker must still show that it WOULD have published, and it
// must show it without leaking the payload — the topic, the event type and the
// size, never the bytes.
func TestLogPublisherLogsTheEventMetadata(t *testing.T) {
	var logs bytes.Buffer
	publisher := pubsub.NewLogPublisher(slog.New(slog.NewTextHandler(&logs, nil)))

	err := publisher.Publish(context.Background(), pubsub.Message{
		Topic:      "payment-events",
		Data:       []byte("secret-payload"),
		Attributes: map[string]string{"type": "PaymentRegistered"},
	})
	if err != nil {
		t.Fatalf("Publish() error = %v, want nil", err)
	}

	got := logs.String()
	for _, want := range []string{"payment-events", "PaymentRegistered", "bytes=14"} {
		if !strings.Contains(got, want) {
			t.Errorf("log = %q, want it to contain %q", got, want)
		}
	}
	if strings.Contains(got, "secret-payload") {
		t.Errorf("log = %q, want the payload NOT logged (it may carry PII)", got)
	}
}

// TestLogPublisherCloseIsANoOp covers the release half of the contract: the dev
// bus owns nothing, so closing it succeeds — which is what lets a composition
// root release its publisher without knowing which one it got.
func TestLogPublisherCloseIsANoOp(t *testing.T) {
	publisher := pubsub.NewLogPublisher(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	if err := publisher.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

// TestLogPublisherNilLoggerFallsBack proves a caller cannot silently discard
// events by passing no logger: the default logger takes over instead of a panic.
func TestLogPublisherNilLoggerFallsBack(t *testing.T) {
	publisher := pubsub.NewLogPublisher(nil)
	if err := publisher.Publish(context.Background(), pubsub.Message{Topic: "student-events"}); err != nil {
		t.Fatalf("Publish() with a nil logger error = %v, want nil", err)
	}
}

// The dev bus must be usable everywhere a real transport is.
var (
	_ pubsub.Publisher       = (*pubsub.LogPublisher)(nil)
	_ pubsub.PublisherCloser = (*pubsub.LogPublisher)(nil)
)
