package expo_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/comms/internal/adapters/expo"
	"github.com/garcia-lorca/services/comms/internal/service"
)

type receiptStoreSpy struct {
	tickets       []service.PushTicket
	deleted       []string
	expired       []time.Time
	operations    []string
	devices       []string
	failures      []string
	checkedBefore []time.Time
}

func (s *receiptStoreSpy) DeleteExpiredPushTickets(_ context.Context, before time.Time) error {
	s.operations = append(s.operations, "expire")
	s.expired = append(s.expired, before)
	kept := s.tickets[:0]
	for _, ticket := range s.tickets {
		if !ticket.CreatedAt.Before(before) {
			kept = append(kept, ticket)
		}
	}
	s.tickets = kept
	return nil
}

func (s *receiptStoreSpy) ListPushTickets(_ context.Context, before, checkedBefore, afterCreatedAt time.Time, afterID string, limit int) ([]service.PushTicket, error) {
	s.operations = append(s.operations, "list")
	s.checkedBefore = append(s.checkedBefore, checkedBefore)
	var out []service.PushTicket
	for _, ticket := range s.tickets {
		if ticket.CreatedAt.After(before) || (afterID != "" && (ticket.CreatedAt.Before(afterCreatedAt) || (ticket.CreatedAt.Equal(afterCreatedAt) && ticket.ID <= afterID))) {
			continue
		}
		out = append(out, ticket)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}
func (s *receiptStoreSpy) DeletePushTicket(_ context.Context, id string) error {
	s.deleted = append(s.deleted, id)
	return nil
}
func (s *receiptStoreSpy) DeletePushDevice(_ context.Context, token string) error {
	s.devices = append(s.devices, token)
	return nil
}
func (s *receiptStoreSpy) RecordPushTicketFailure(_ context.Context, id string, _ time.Time) (int, error) {
	s.failures = append(s.failures, id)
	for _, ticket := range s.tickets {
		if ticket.ID == id {
			return ticket.Attempts + 1, nil
		}
	}
	return 1, nil
}

func TestReceiptCheckerDeletesTicketsOlderThanRetentionBeforeFetching(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	store := &receiptStoreSpy{tickets: []service.PushTicket{
		{ID: "expired", TicketID: "old-ticket", CreatedAt: now.Add(-24*time.Hour - time.Second)},
	}}
	checker := expo.NewReceiptChecker(store, expo.WithReceiptClock(func() time.Time { return now }))
	require.NoError(t, checker.Check(context.Background()))
	require.Equal(t, []time.Time{now.Add(-24 * time.Hour)}, store.expired)
	assert.Empty(t, store.tickets)
	assert.Equal(t, []string{"expire", "list"}, store.operations)
}

func TestReceiptCheckerUsesInjectedClockForRecheckWindow(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	store := &receiptStoreSpy{}
	checker := expo.NewReceiptChecker(store, expo.WithReceiptClock(func() time.Time { return now }))

	require.NoError(t, checker.Check(context.Background()))

	require.Equal(t, []time.Time{now.Add(-15 * time.Minute)}, store.checkedBefore)
}

func TestReceiptCheckerRetriesOnlyMissingReceipts(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	store := &receiptStoreSpy{tickets: []service.PushTicket{
		{ID: "missing", TicketID: "missing-ticket", CreatedAt: now.Add(-time.Hour)},
		{ID: "failed", TicketID: "failed-ticket", CreatedAt: now.Add(-time.Hour)},
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"failed-ticket": map[string]any{"status": "error", "details": map[string]any{"error": "MessageRateExceeded"}},
		}})
	}))
	defer server.Close()
	checker := expo.NewReceiptChecker(store, expo.WithReceiptsBaseURL(server.URL), expo.WithReceiptClock(func() time.Time { return now }))
	require.NoError(t, checker.Check(context.Background()))
	assert.Equal(t, []string{"missing"}, store.failures)
	assert.Equal(t, []string{"failed"}, store.deleted)
}

func TestReceiptCheckerDeletesMissingReceiptAfterAttemptLimit(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	store := &receiptStoreSpy{tickets: []service.PushTicket{{ID: "missing", TicketID: "missing-ticket", CreatedAt: now.Add(-time.Hour), Attempts: 4}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})
	}))
	defer server.Close()
	checker := expo.NewReceiptChecker(store, expo.WithReceiptsBaseURL(server.URL), expo.WithReceiptClock(func() time.Time { return now }))
	require.NoError(t, checker.Check(context.Background()))
	assert.Equal(t, []string{"missing"}, store.failures)
	assert.Equal(t, []string{"missing"}, store.deleted)
}

func TestReceiptCheckerPagesBeyondExpoReceiptLimit(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	tickets := make([]service.PushTicket, 1001)
	for i := range tickets {
		tickets[i] = service.PushTicket{ID: fmt.Sprintf("%04d", i), TicketID: fmt.Sprintf("ticket-%04d", i), CreatedAt: now.Add(-time.Hour)}
	}
	store := &receiptStoreSpy{tickets: tickets}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			IDs []string `json:"ids"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		if calls == 1 {
			assert.Len(t, body.IDs, 1000)
		} else {
			assert.Len(t, body.IDs, 1)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})
	}))
	defer server.Close()
	checker := expo.NewReceiptChecker(store, expo.WithReceiptsBaseURL(server.URL), expo.WithReceiptClock(func() time.Time { return now }))
	require.NoError(t, checker.Check(context.Background()))
	assert.Equal(t, 2, calls)
}

func TestReceiptCheckerReturnsNonOKHTTPStatus(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	store := &receiptStoreSpy{tickets: []service.PushTicket{{ID: "a", TicketID: "ticket-a", CreatedAt: now.Add(-time.Hour)}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer server.Close()
	checker := expo.NewReceiptChecker(store, expo.WithReceiptsBaseURL(server.URL), expo.WithReceiptClock(func() time.Time { return now }))
	require.ErrorContains(t, checker.Check(context.Background()), "HTTP 503")
}

func TestReceiptCheckerCleansSuccessfulAndDeadTicketsAndCapsRetries(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	store := &receiptStoreSpy{tickets: []service.PushTicket{
		{ID: "a", TicketID: "expo-ok", DeviceToken: "token-ok", CreatedAt: now.Add(-time.Hour)},
		{ID: "b", TicketID: "expo-dead", DeviceToken: "token-dead", CreatedAt: now.Add(-time.Hour)},
		{ID: "c", TicketID: "expo-error", DeviceToken: "token-error", CreatedAt: now.Add(-time.Hour), Attempts: 4},
		{ID: "d", TicketID: "expo-young", DeviceToken: "token-young", CreatedAt: now.Add(-time.Minute)},
	}}
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/--/api/v2/push/getReceipts", r.URL.Path)
		gotAuth = r.Header.Get("Authorization")
		var body struct {
			IDs []string `json:"ids"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, []string{"expo-ok", "expo-dead", "expo-error"}, body.IDs)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"expo-ok":    map[string]any{"status": "ok"},
			"expo-dead":  map[string]any{"status": "error", "details": map[string]any{"error": "DeviceNotRegistered"}},
			"expo-error": map[string]any{"status": "error", "details": map[string]any{"error": "MessageRateExceeded"}},
		}})
	}))
	defer server.Close()
	checker := expo.NewReceiptChecker(store, expo.WithReceiptsBaseURL(server.URL), expo.WithReceiptAccessToken("access"), expo.WithReceiptClock(func() time.Time { return now }))
	require.NoError(t, checker.Check(context.Background()))
	assert.Equal(t, "Bearer access", gotAuth)
	assert.Equal(t, []string{"a", "b", "c"}, store.deleted)
	assert.Equal(t, []string{"token-dead"}, store.devices)
	assert.Empty(t, store.failures)
}

func TestReceiptCheckerDefersTicketsYoungerThanFifteenMinutes(t *testing.T) {
	now := time.Now()
	store := &receiptStoreSpy{tickets: []service.PushTicket{{ID: "young", TicketID: "expo-young", CreatedAt: now.Add(-14 * time.Minute)}}}
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer server.Close()
	checker := expo.NewReceiptChecker(store, expo.WithReceiptsBaseURL(server.URL), expo.WithReceiptClock(func() time.Time { return now }))
	require.NoError(t, checker.Check(context.Background()))
	assert.False(t, called)
}
