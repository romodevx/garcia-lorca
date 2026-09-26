// Package fcm adapts the comms service to push delivery. It only translates —
// a rendered notification plus its target tokens → a transport call; the service
// never learns what the transport is.
//
// LogSender reports the delivery to the log instead of sending it: the default
// (PUSH_PROVIDER=log) for development and for any environment without push
// credentials. Real delivery is the Expo adapter (adapters/expo,
// PUSH_PROVIDER=expo), since the family app registers Expo push tokens. WHICH devices a notification
// belongs to is already exact: the service targets the student's guardians (see
// service.GuardianReader and service.Repo.DevicesForUsers).
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
// Pushing to nobody is a no-op, not a log line: a student whose guardians have
// registered no device is the ordinary state of a school still onboarding its
// families, and the service already reports the cases worth looking at.
func (s *LogSender) Push(_ context.Context, m service.PushMessage) (service.PushReport, error) {
	if len(m.Tokens) == 0 {
		return service.PushReport{}, nil
	}
	s.log.Info("comms push (log sender: no delivery)",
		slog.Int("target_count", len(m.Tokens)),
		slog.String("first_target", truncateToken(m.Tokens[0])),
		slog.String("title", m.Title))
	return service.PushReport{}, nil
}

// truncateToken reduces a token to a correlation handle.
func truncateToken(token string) string {
	if len(token) <= tokenPrefixLength {
		return "…"
	}
	return token[:tokenPrefixLength] + "…"
}
