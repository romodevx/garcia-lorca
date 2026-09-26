package expo_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/comms/internal/adapters/expo"
	"github.com/garcia-lorca/services/comms/internal/service"
)

type sentMessage struct {
	To    string            `json:"to"`
	Title string            `json:"title"`
	Body  string            `json:"body"`
	Data  map[string]string `json:"data"`
	Sound string            `json:"sound"`
}

// fakeExpo records every batch and answers one ticket per message; dead tokens
// get a DeviceNotRegistered ticket, like the real service.
type fakeExpo struct {
	mu      sync.Mutex
	batches [][]sentMessage
	auth    []string
	dead    map[string]bool
	status  int // non-zero: answer the whole request with this status
}

func (f *fakeExpo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var batch []sentMessage
	if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.batches = append(f.batches, batch)
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	f.mu.Unlock()
	if r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "content type", http.StatusUnsupportedMediaType)
		return
	}
	if f.status != 0 {
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(`{"errors":[{"code":"TOO_MANY_REQUESTS","message":"slow down"}]}`))
		return
	}
	tickets := make([]map[string]any, 0, len(batch))
	for i, m := range batch {
		if f.dead[m.To] {
			tickets = append(tickets, map[string]any{
				"status": "error", "message": "not registered",
				"details": map[string]any{"error": "DeviceNotRegistered", "expoPushToken": m.To},
			})
			continue
		}
		tickets = append(tickets, map[string]any{"status": "ok", "id": fmt.Sprintf("ticket-%d", i)})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": tickets})
}

func tokens(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("ExponentPushToken[tok%03d]", i)
	}
	return out
}

func newSender(t *testing.T, f http.Handler, opts ...expo.Option) *expo.Sender {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return expo.NewSender(append([]expo.Option{expo.WithEndpoint(srv.URL)}, opts...)...)
}

var message = service.PushMessage{
	Title: "Llegada tarde", Body: "Ana llegó tarde el 15 de septiembre.",
	Data: map[string]string{"type": "attendance", "studentId": "s1", "date": "2026-09-15"},
}

func TestPushBatchesAtMostOneHundredMessages(t *testing.T) {
	f := &fakeExpo{}
	m := message
	m.Tokens = tokens(250)
	report, err := newSender(t, f).Push(context.Background(), m)
	require.NoError(t, err)
	assert.Empty(t, report.Unregistered)
	require.Len(t, f.batches, 3)
	assert.Len(t, f.batches[0], 100)
	assert.Len(t, f.batches[1], 100)
	assert.Len(t, f.batches[2], 50)
	first := f.batches[0][0]
	assert.Equal(t, "ExponentPushToken[tok000]", first.To)
	assert.Equal(t, message.Title, first.Title)
	assert.Equal(t, message.Body, first.Body)
	assert.Equal(t, message.Data, first.Data)
	assert.Equal(t, "default", first.Sound)
}

func TestPushSendsTheAccessTokenOnlyWhenConfigured(t *testing.T) {
	m := message
	m.Tokens = tokens(1)

	with := &fakeExpo{}
	_, err := newSender(t, with, expo.WithAccessToken("secret-token")).Push(context.Background(), m)
	require.NoError(t, err)
	assert.Equal(t, []string{"Bearer secret-token"}, with.auth)

	without := &fakeExpo{}
	_, err = newSender(t, without).Push(context.Background(), m)
	require.NoError(t, err)
	assert.Equal(t, []string{""}, without.auth)
}

// DeviceNotRegistered tickets are matched to their token by position, across
// batches.
func TestPushReportsUnregisteredDevices(t *testing.T) {
	all := tokens(150)
	f := &fakeExpo{dead: map[string]bool{all[3]: true, all[120]: true}}
	m := message
	m.Tokens = all
	report, err := newSender(t, f).Push(context.Background(), m)
	require.NoError(t, err)
	assert.Equal(t, []string{all[3], all[120]}, report.Unregistered)
}

// A token that is not an Expo push token is never sent (it would fail the whole
// batch), and no valid token means no request at all.
func TestPushSkipsMalformedTokens(t *testing.T) {
	f := &fakeExpo{}
	m := message
	m.Tokens = []string{"fcm-legacy-token", "ExponentPushToken[ok1]", "ExpoPushToken[ok2]", "ExponentPushToken[]"}
	_, err := newSender(t, f).Push(context.Background(), m)
	require.NoError(t, err)
	require.Len(t, f.batches, 1)
	require.Len(t, f.batches[0], 2)
	assert.Equal(t, "ExponentPushToken[ok1]", f.batches[0][0].To)

	none := &fakeExpo{}
	m.Tokens = []string{"nope"}
	_, err = newSender(t, none).Push(context.Background(), m)
	require.NoError(t, err)
	assert.Empty(t, none.batches)
}

// A failed batch is an error, but the other batches still go out and their
// dead tokens are still reported.
func TestPushRequestErrorsDoNotHideOtherBatches(t *testing.T) {
	all := tokens(150)
	calls := 0
	var mu sync.Mutex
	ok := &fakeExpo{dead: map[string]bool{all[120]: true}}
	failing := &fakeExpo{status: http.StatusTooManyRequests}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			failing.ServeHTTP(w, r)
			return
		}
		ok.ServeHTTP(w, r)
	})
	m := message
	m.Tokens = all
	report, err := newSender(t, handler).Push(context.Background(), m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "429")
	assert.Equal(t, []string{all[120]}, report.Unregistered)
}

func TestPushHonoursTheTimeout(t *testing.T) {
	release := make(chan struct{})
	defer close(release) // before the server's Cleanup Close, which waits on this handler
	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select { // outlive the client's timeout; released when the test ends
		case <-release:
		case <-r.Context().Done():
		}
	})
	m := message
	m.Tokens = tokens(1)
	start := time.Now()
	_, err := newSender(t, slow, expo.WithTimeout(50*time.Millisecond)).Push(context.Background(), m)
	require.Error(t, err)
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestPushHonoursContextCancellation(t *testing.T) {
	f := &fakeExpo{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m := message
	m.Tokens = tokens(1)
	_, err := newSender(t, f).Push(ctx, m)
	require.ErrorIs(t, err, context.Canceled)
}

func TestIsPushToken(t *testing.T) {
	tests := map[string]bool{
		"ExponentPushToken[xxxxxxxxxxxxxxxxxxxxxx]": true,
		"ExpoPushToken[abc-DEF_123]":                true,
		"ExponentPushToken[]":                       false,
		"ExponentPushToken[a b]":                    false,
		"fcm:abc":                                   false,
		"":                                          false,
	}
	for token, want := range tests {
		assert.Equal(t, want, expo.IsPushToken(token), token)
	}
}
