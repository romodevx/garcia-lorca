package middleware

import (
	"strconv"
	"testing"
	"time"
)

// TestEvictionHardCapUnderFixedClockFlood verifies Fix #2: a within-window flood
// of all-fresh distinct keys (no clock advance) is bounded HARD at maxKeys, and
// the idle sweep is throttled — it is NOT invoked once per new key.
func TestEvictionHardCapUnderFixedClockFlood(t *testing.T) {
	clk := &clockStub{t: time.Unix(1000, 0)}
	const maxKeys = 100
	rl := NewRateLimiter(1, 1, clk.now,
		WithMaxKeys(maxKeys),
		WithEvictWindow(10*time.Second))

	// Fixed clock: no key ever ages past evictWindow, so the idle sweep can free
	// nothing — the hard-cap batch drop is what must keep the map bounded.
	for i := 0; i < maxKeys*20; i++ {
		rl.Allow("key-" + strconv.Itoa(i))
		if n := rl.Len(); n > maxKeys {
			t.Fatalf("after insert %d: Len()=%d exceeds hard cap %d", i, n, maxKeys)
		}
	}

	if n := rl.Len(); n > maxKeys {
		t.Fatalf("final Len()=%d, want <= %d (hard cap)", n, maxKeys)
	}

	// The sweep must be time-throttled, not per-key. Over the whole fixed-clock
	// run it can fire at most once (nextSweep jumps evictWindow into the future
	// after the first sweep and the clock never advances).
	rl.mu.Lock()
	sweeps := rl.sweeps
	rl.mu.Unlock()
	if sweeps > 1 {
		t.Fatalf("idle sweep ran %d times on a fixed clock, want <= 1 (must be time-throttled, not per-key)", sweeps)
	}
}

// clockStub is an in-package deterministic clock (the external clock helper lives
// in the _test package and is not visible here).
type clockStub struct{ t time.Time }

func (c *clockStub) now() time.Time { return c.t }
