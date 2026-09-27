//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/garcia-lorca/services/grades/internal/ports"
)

// outboxTenant is used only by the outbox tests: the drain takes every
// pending row of its tenant, so no other test may queue under it.
const outboxTenant = "33333333-3333-3333-3333-333333333333"

func event(payload string) ports.OutboxEvent {
	return ports.OutboxEvent{Type: ports.OutboxGradesPublished, Payload: []byte(payload)}
}

func queue(t *testing.T, repo interface {
	WithPeriodLock(context.Context, string, func(context.Context, ports.PeriodTx) error) error
}, term string, fail error, events ...ports.OutboxEvent) error {
	t.Helper()
	return repo.WithPeriodLock(context.Background(), term, func(ctx context.Context, tx ports.PeriodTx) error {
		if err := tx.QueueEvents(ctx, events); err != nil {
			return err
		}
		return fail
	})
}

// Events are queued in the close's transaction: a rolled-back close leaves
// none; a committed one leaves them pending until a drain publishes and marks
// them, oldest first.
func TestOutboxQueuesInTheTransactionAndDrains(t *testing.T) {
	const term = "70700000-0000-0000-0000-0000000000a1"
	repo := appRepo(t, outboxTenant)
	ctx := context.Background()

	boom := errors.New("close refused")
	assert.ErrorIs(t, queue(t, repo, term, boom, event(`{"reportCardId":"rolled-back"}`)), boom)
	require.NoError(t, queue(t, repo, term, nil, event(`{"reportCardId":"a"}`), event(`{"reportCardId":"b"}`)))

	var got []string
	n, err := repo.DrainOutbox(ctx, 10, func(e ports.OutboxEvent) error {
		assert.Equal(t, ports.OutboxGradesPublished, e.Type)
		got = append(got, string(e.Payload))
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.JSONEq(t, `{"reportCardId":"a"}`, got[0])
	assert.JSONEq(t, `{"reportCardId":"b"}`, got[1])

	n, err = repo.DrainOutbox(ctx, 10, func(ports.OutboxEvent) error {
		t.Fatal("a published event was drained again")
		return nil
	})
	require.NoError(t, err)
	assert.Zero(t, n)
}

// A publish that fails leaves the event pending with its attempt counted and
// its next attempt pushed out (backoff): an immediate drain skips it; once
// due, it is retried and marked.
func TestOutboxRetriesAFailedPublishWithBackoff(t *testing.T) {
	const term = "70700000-0000-0000-0000-0000000000a2"
	repo := appRepo(t, outboxTenant)
	ctx := context.Background()
	require.NoError(t, queue(t, repo, term, nil, event(`{"reportCardId":"retry"}`)))

	n, err := repo.DrainOutbox(ctx, 10, func(ports.OutboxEvent) error { return errors.New("broker down") })
	require.NoError(t, err, "a refused publish is not a drain error")
	assert.Zero(t, n)
	attempts, nextAt := outboxRow(t, `{"reportCardId":"retry"}`)
	assert.Equal(t, 1, attempts)
	assert.True(t, nextAt.After(time.Now()), "the retry is scheduled later, not immediately")

	calls := 0
	n, err = repo.DrainOutbox(ctx, 10, func(ports.OutboxEvent) error { calls++; return nil })
	require.NoError(t, err)
	assert.Zero(t, calls, "not due yet: the backoff holds it")
	assert.Zero(t, n)

	makeDue(t, `{"reportCardId":"retry"}`)
	n, err = repo.DrainOutbox(ctx, 10, func(ports.OutboxEvent) error { calls++; return nil })
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, 1, calls)
	attempts, _ = outboxRow(t, `{"reportCardId":"retry"}`)
	assert.Equal(t, 2, attempts)
}

// RLS: tenant A never drains tenant B's events.
func TestRLSIsolatesTheOutbox(t *testing.T) {
	const term = "70700000-0000-0000-0000-0000000000a3"
	const isolated = "44444444-4444-4444-4444-444444444444"
	require.NoError(t, queue(t, appRepo(t, isolated), term, nil, event(`{"reportCardId":"tenant-b"}`)))
	n, err := appRepo(t, "55555555-5555-5555-5555-555555555555").DrainOutbox(context.Background(), 10, func(ports.OutboxEvent) error {
		t.Fatal("drained another tenant's event")
		return nil
	})
	require.NoError(t, err)
	assert.Zero(t, n)
}

func outboxRow(t *testing.T, payload string) (attempts int, nextAt time.Time) {
	t.Helper()
	var row struct {
		Attempts      int
		NextAttemptAt time.Time
	}
	require.NoError(t, superDB.Raw("SELECT attempts, next_attempt_at FROM grades_outbox WHERE payload = ?::jsonb", payload).Scan(&row).Error)
	return row.Attempts, row.NextAttemptAt
}

func makeDue(t *testing.T, payload string) {
	t.Helper()
	res := superDB.Session(&gorm.Session{}).Exec("UPDATE grades_outbox SET next_attempt_at = now() - interval '1 second' WHERE payload = ?::jsonb", payload)
	require.NoError(t, res.Error)
	require.Equal(t, int64(1), res.RowsAffected)
}
