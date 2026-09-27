package main

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestStartSweep_StopWaitsForAnInFlightDrain pins the shutdown order's
// premise: once stop returns, no drain is running, so the publisher it uses
// can be closed right after.
func TestStartSweep_StopWaitsForAnInFlightDrain(t *testing.T) {
	started := make(chan struct{})
	var running atomic.Bool
	drain := func(ctx context.Context) (int, error) {
		running.Store(true)
		defer running.Store(false)
		select {
		case started <- struct{}{}:
		default:
		}
		<-ctx.Done()
		time.Sleep(20 * time.Millisecond) // finishing the in-flight publish
		return 0, ctx.Err()
	}

	stop := startSweep(context.Background(), drain, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Millisecond)
	<-started
	stop()

	assert.False(t, running.Load(), "stop must return only after the sweep goroutine ended")
}

// TestStartSweep_StopIsIdempotent: the deferred stop and an early one may both run.
func TestStartSweep_StopIsIdempotent(t *testing.T) {
	stop := startSweep(context.Background(), func(context.Context) (int, error) { return 0, nil },
		slog.New(slog.NewTextHandler(io.Discard, nil)), time.Hour)
	stop()
	stop()
}
