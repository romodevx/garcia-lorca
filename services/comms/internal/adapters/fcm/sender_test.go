package fcm_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/garcia-lorca/services/comms/internal/adapters/fcm"
	"github.com/garcia-lorca/services/comms/internal/service"
)

// newSender wires a LogSender over a buffer so the tests can read exactly what
// an operator would see.
func newSender(t *testing.T) (*fcm.LogSender, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return fcm.NewLogSender(log), &logs
}

// TestLogSenderReportsTheDeliveryWithoutLeakingTokens is the whole contract of
// the dev sender: it says how many devices a notification reached and which
// notification it was, and it NEVER writes a token out in full — an FCM token is
// a credential for pushing to that device, so a log line carrying one is a leak
// that survives in log storage.
func TestLogSenderReportsTheDeliveryWithoutLeakingTokens(t *testing.T) {
	const fakeToken = "device-token-fixture-not-a-real-credential"
	sender, logs := newSender(t)

	if _, err := sender.Push(context.Background(), service.PushMessage{
		Tokens: []string{fakeToken, "otro-token-largo-de-dispositivo"},
		Title:  "Ausencia registrada", Body: "Se registró una ausencia el 2026-08-06.",
	}); err != nil {
		t.Fatalf("Push: %v", err)
	}

	out := logs.String()
	if strings.Contains(out, fakeToken) {
		t.Errorf("the log leaked a full FCM token:\n%s", out)
	}
	if !strings.Contains(out, "target_count=2") {
		t.Errorf("log = %q, want the number of targets", out)
	}
	if !strings.Contains(out, "Ausencia registrada") {
		t.Errorf("log = %q, want the notification title", out)
	}
	if strings.Contains(out, "Se registró una ausencia") {
		t.Errorf("the log carried the notification body, which is family-facing content:\n%s", out)
	}
}

// TestLogSenderWithNoTargetsIsANoOp keeps the dev log readable: a notification
// with nobody registered to receive it is not an event worth a line.
func TestLogSenderWithNoTargetsIsANoOp(t *testing.T) {
	sender, logs := newSender(t)

	if _, err := sender.Push(context.Background(), service.PushMessage{Title: "Pago recibido", Body: "body"}); err != nil {
		t.Fatalf("Push with no targets: %v", err)
	}
	if logs.Len() != 0 {
		t.Errorf("logged %q for zero targets, want nothing", logs.String())
	}
}

// TestNewLogSenderToleratesANilLogger proves the adapter cannot panic on a
// half-wired composition root: a nil logger falls back to the default.
func TestNewLogSenderToleratesANilLogger(t *testing.T) {
	if _, err := fcm.NewLogSender(nil).Push(context.Background(), service.PushMessage{Tokens: []string{"token"}, Title: "t", Body: "b"}); err != nil {
		t.Fatalf("Push with a nil logger: %v", err)
	}
}
