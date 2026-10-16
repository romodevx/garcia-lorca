package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestDrainOutboxOnceAtStartup(t *testing.T) {
	calls := 0
	drain := func(ctx context.Context) (int, error) {
		calls++
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 2*time.Second {
			t.Errorf("startup drain deadline = %v, ok %v; want <= 2s", deadline, ok)
		}
		return 1, nil
	}

	drainAtStartup(context.Background(), drain, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if calls != 1 {
		t.Errorf("startup drains = %d, want 1", calls)
	}
}
