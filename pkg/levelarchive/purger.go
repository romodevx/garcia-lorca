package levelarchive

import (
	"context"
	"log/slog"
	"time"
)

// PurgeInterval is how often a service sweeps its archive.
const PurgeInterval = time.Hour

// StartPurger runs a retention sweep once at start and then every
// PurgeInterval, until ctx ends. The gateway runs the level trash's sweep with
// it (ADR 0021); purge answers how many batches (or rows) it purged. The caller passes a DETACHED context (the
// process's, not a request's): the sweep must outlive any single call. Each
// run is logged; a failed run is logged and retried on the next tick — a
// purge that waits an hour loses nothing but disk.
func StartPurger(ctx context.Context, purge func(context.Context) (int64, error), log *slog.Logger) {
	ticker := time.NewTicker(PurgeInterval)
	go func() {
		defer ticker.Stop()
		runPurger(ctx, ticker.C, purge, log)
	}()
}

func runPurger(ctx context.Context, tick <-chan time.Time, purge func(context.Context) (int64, error), log *slog.Logger) {
	sweep := func() {
		n, err := purge(ctx)
		if err != nil {
			log.Error("level archive purge failed", slog.Any("error", err))
			return
		}
		log.Info("level archive sweep done", slog.Int64("purged", n))
	}
	sweep()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			sweep()
		}
	}
}
