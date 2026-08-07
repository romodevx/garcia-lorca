// Package fcm adapts the comms service to push delivery. It only translates —
// a rendered notification plus its target tokens → a transport call; the service
// never learns what the transport is.
//
// Only LogSender ships in this phase: it reports the delivery to the log instead
// of calling Firebase, which is enough to develop and demo the whole consumer
// path (the durable feed is the product; push is the courtesy on top). Wiring the
// real Firebase Cloud Messaging client is a deliberate carry-forward — it needs a
// service-account credential in the secret manager and, more importantly, the
// guardian↔student link that tells us WHICH devices a notification belongs to
// (see service.Repo.DevicesForTenant).
package fcm

import (
	"context"
	"log/slog"

	"github.com/garcia-lorca/services/comms/internal/service"
)

// Compile-time proof the adapter implements the service's Sender seam.
var _ service.Sender = (*LogSender)(nil)

// tokenPrefixLength is how much of a token a log line may carry: enough to
// correlate two lines about the same device, far too little to push to it.
const tokenPrefixLength = 6

// LogSender is the development push sender: it records that a notification would
// have been delivered, to how many targets, without contacting Firebase.
type LogSender struct {
	log *slog.Logger
}

// NewLogSender builds the sender. A nil log falls back to slog.Default, so a
// half-wired composition root degrades to noisy rather than to a panic.
func NewLogSender(log *slog.Logger) *LogSender {
	if log == nil {
		log = slog.Default()
	}
	return &LogSender{log: log}
}

// Push reports the delivery. It logs the target COUNT and a truncated first
// token, never a token in full: an FCM token is the credential for pushing to
// that device, so a log line carrying one is a leak that outlives the request.
// The notification body is family-facing content and stays out of the log too;
// the title is enough to tell deliveries apart.
//
// Pushing to nobody is a no-op, not a log line: with targeting still tenant-wide
// (see service.Repo.DevicesForTenant), an empty device list is the ordinary state
// of a tenant that has registered none.
func (s *LogSender) Push(_ context.Context, tokens []string, title, _ string) error {
	if len(tokens) == 0 {
		return nil
	}
	s.log.Info("comms push (log sender: no delivery)",
		slog.Int("target_count", len(tokens)),
		slog.String("first_target", truncateToken(tokens[0])),
		slog.String("title", title))
	return nil
}

// truncateToken reduces a token to a correlation handle.
func truncateToken(token string) string {
	if len(token) <= tokenPrefixLength {
		return "…"
	}
	return token[:tokenPrefixLength] + "…"
}
