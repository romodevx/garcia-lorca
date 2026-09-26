// Package expo delivers push notifications through the Expo Push Service
// (https://docs.expo.dev/push-notifications/sending-notifications/): the family
// app is an Expo app, so its devices register Expo push tokens, and Expo relays
// to APNs/FCM. The adapter only translates — a rendered notification plus its
// target tokens → HTTP batches — and reports back the tokens Expo says are dead.
//
// Only push TICKETS are read. Expo can also report DeviceNotRegistered later, in
// push RECEIPTS (/getReceipts); polling receipts needs a durable ticket store and
// is a deliberate follow-up. Tickets already catch an uninstalled app on the next
// push, which is the case that matters.
package expo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/garcia-lorca/services/comms/internal/service"
)

// Compile-time proof the adapter implements the service's Sender seam.
var _ service.Sender = (*Sender)(nil)

const (
	// DefaultEndpoint is Expo's push-send API.
	DefaultEndpoint = "https://exp.host/--/api/v2/push/send"
	// maxBatch is Expo's per-request limit (PUSH_TOO_MANY_NOTIFICATIONS above it).
	maxBatch = 100
	// defaultTimeout bounds one batch round trip, so a slow Expo never holds the
	// consumer (push is best-effort; the feed entry is already durable).
	defaultTimeout = 10 * time.Second
	// maxErrorBody caps how much of a failed response is read into the error.
	maxErrorBody = 512
	// errDeviceNotRegistered is the ticket error for a token that can no longer
	// receive pushes (the app was uninstalled or the token rotated).
	errDeviceNotRegistered = "DeviceNotRegistered"
	// ticketStatusError marks a per-message failure in a ticket.
	ticketStatusError = "error"
	// defaultSound makes the phone ring as for any other app notification.
	defaultSound = "default"
)

// pushTokenPattern is the shape of an Expo push token.
var pushTokenPattern = regexp.MustCompile(`^(ExponentPushToken|ExpoPushToken)\[[A-Za-z0-9_-]{1,200}\]$`)

// IsPushToken reports whether token has the shape of an Expo push token. A token
// of any other shape would fail its whole batch at Expo, so it is never sent.
func IsPushToken(token string) bool {
	return pushTokenPattern.MatchString(token)
}

// Sender posts notifications to the Expo Push Service.
type Sender struct {
	endpoint    string
	accessToken string
	client      *http.Client
	log         *slog.Logger
}

// Option configures a Sender (functional options).
type Option func(*Sender)

// WithEndpoint replaces the Expo endpoint (tests point it at a fake).
func WithEndpoint(url string) Option { return func(s *Sender) { s.endpoint = url } }

// WithAccessToken sends `Authorization: Bearer <token>` — required once the
// project enables Expo's enhanced push security. Empty sends no header.
func WithAccessToken(token string) Option { return func(s *Sender) { s.accessToken = token } }

// WithTimeout bounds each batch's round trip.
func WithTimeout(d time.Duration) Option { return func(s *Sender) { s.client.Timeout = d } }

// WithLogger sets where skipped tokens are reported; nil keeps slog.Default.
func WithLogger(log *slog.Logger) Option {
	return func(s *Sender) {
		if log != nil {
			s.log = log
		}
	}
}

// NewSender builds a Sender with the default endpoint and timeout.
func NewSender(opts ...Option) *Sender {
	s := &Sender{
		endpoint: DefaultEndpoint,
		client:   &http.Client{Timeout: defaultTimeout},
		log:      slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// message is one Expo push message.
type message struct {
	To    string            `json:"to"`
	Title string            `json:"title"`
	Body  string            `json:"body"`
	Data  map[string]string `json:"data,omitempty"`
	Sound string            `json:"sound"`
}

// ticket is Expo's per-message answer, in request order.
type ticket struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Details struct {
		Error string `json:"error"`
	} `json:"details"`
}

type sendResponse struct {
	Data []ticket `json:"data"`
}

// Push sends m to every valid token in batches of maxBatch. Every batch is
// attempted even when one fails; the report carries the dead tokens of the
// batches that answered, and the error joins the batches that did not.
func (s *Sender) Push(ctx context.Context, m service.PushMessage) (service.PushReport, error) {
	targets := s.validTokens(m.Tokens)
	var (
		report service.PushReport
		errs   []error
	)
	for start := 0; start < len(targets); start += maxBatch {
		batch := targets[start:min(start+maxBatch, len(targets))]
		dead, err := s.send(ctx, batch, m)
		report.Unregistered = append(report.Unregistered, dead...)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return report, errors.Join(errs...)
}

// validTokens keeps the Expo-shaped tokens, reporting how many were skipped
// (never the tokens themselves: a token is a credential to push to a phone).
func (s *Sender) validTokens(tokens []string) []string {
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if IsPushToken(t) {
			out = append(out, t)
		}
	}
	if skipped := len(tokens) - len(out); skipped > 0 {
		s.log.Warn("expo push skipped tokens that are not Expo push tokens", slog.Int("skipped", skipped))
	}
	return out
}

// send posts one batch and returns the tokens its tickets report as dead.
func (s *Sender) send(ctx context.Context, tokens []string, m service.PushMessage) ([]string, error) {
	payload := make([]message, 0, len(tokens))
	for _, t := range tokens {
		payload = append(payload, message{To: t, Title: m.Title, Body: m.Body, Data: m.Data, Sound: defaultSound})
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encoding expo push batch: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building expo push request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if s.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.accessToken)
	}
	res, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending expo push batch: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(res.Body, maxErrorBody))
		return nil, fmt.Errorf("expo push batch refused: HTTP %d: %s", res.StatusCode, bytes.TrimSpace(snippet))
	}
	var parsed sendResponse
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decoding expo push tickets: %w", err)
	}
	return deadTokens(tokens, parsed.Data), nil
}

// deadTokens pairs each ticket with the token at the same position (Expo answers
// in request order) and keeps the ones reported DeviceNotRegistered.
func deadTokens(tokens []string, tickets []ticket) []string {
	var dead []string
	for i, t := range tickets {
		if i >= len(tokens) {
			break
		}
		if t.Status == ticketStatusError && t.Details.Error == errDeviceNotRegistered {
			dead = append(dead, tokens[i])
		}
	}
	return dead
}
