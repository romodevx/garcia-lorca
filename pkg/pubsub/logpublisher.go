package pubsub

import (
	"context"
	"log/slog"
)

// LogPublisher is a Publisher that records events in the log instead of
// transporting them. It exists for the standalone run: a service started with no
// broker configured (no PUBSUB_EMULATOR_HOST, no PUBSUB_PROJECT_ID) still boots
// and still shows, per event, that it WOULD have published — so publishing is
// never silently absent while a developer works on the read side.
//
// It logs the topic, the event type and the payload SIZE, never the payload:
// domain events carry student data, and logs are not the place for it.
//
// It is NOT a fallback for a broker that failed to connect. A configured
// transport that cannot be reached must fail startup (see gcp.PublisherFromEnv);
// degrading to this bus there would drop every event while the service reports
// itself healthy.
type LogPublisher struct{ log *slog.Logger }

// NewLogPublisher returns the dev bus. A nil log falls back to slog.Default, so
// events are never discarded without a trace.
func NewLogPublisher(log *slog.Logger) *LogPublisher {
	if log == nil {
		log = slog.Default()
	}
	return &LogPublisher{log: log}
}

// Publish logs the event's metadata and always succeeds.
func (p *LogPublisher) Publish(_ context.Context, msg Message) error {
	p.log.Info("event published (dev bus)",
		slog.String("topic", msg.Topic),
		slog.String("type", msg.Attributes[AttrType]),
		slog.Int("bytes", len(msg.Data)))
	return nil
}

// Close satisfies PublisherCloser. The dev bus owns no goroutines and no
// connections, so releasing it is a no-op that always succeeds.
func (p *LogPublisher) Close() error { return nil }
