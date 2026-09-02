package configclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/garcia-lorca/services/finance/internal/ports"
)

// VersionUnavailable is the version stamped when there is no calendar to read
// and none cached: config was unreachable and the only honest answer left is
// "weekends only". A payment carrying this version says plainly that its
// deadline was worked out without the school's holidays.
const VersionUnavailable = "unavailable"

// CachedCalendar holds the school calendar in memory for a short while and
// refuses to let a slow config service hold up a payment.
//
// Two problems, one type. The calendar changes a few times a year and is read
// on every payment, so a TTL in minutes removes almost every round trip. And
// when the read does happen, a cashier is standing in front of a parent: the
// call is cut off at a deadline, and what we already hold is used instead. The
// TTL is in minutes rather than per cycle because a holiday added this morning
// has to be visible this morning, not next August.
//
// Safe for concurrent use: the gateway calls this from every request.
type CachedCalendar struct {
	source  ports.CalendarReader
	ttl     time.Duration
	timeout time.Duration

	mu     sync.Mutex
	cached map[string]cacheEntry
}

type cacheEntry struct {
	snap    ports.CalendarSnapshot
	fetched time.Time
}

// Compile-time proof the decorator is itself a CalendarReader, so callers cannot
// tell the difference and nothing downstream depends on the caching.
var _ ports.CalendarReader = (*CachedCalendar)(nil)

// NewCachedCalendar wraps source with a TTL cache and a per-read timeout.
func NewCachedCalendar(source ports.CalendarReader, ttl, timeout time.Duration) *CachedCalendar {
	return &CachedCalendar{
		source:  source,
		ttl:     ttl,
		timeout: timeout,
		cached:  map[string]cacheEntry{},
	}
}

// NonWorkingDays returns the cycle's closed days, from cache when it is fresh
// and from config when it is not. It never returns an error a caller has to
// handle: every failure path degrades to a snapshot marked Stale, because
// failing a payment over a calendar read would stop the school collecting money
// for a reason no cashier could explain.
func (c *CachedCalendar) NonWorkingDays(ctx context.Context, cycleID string) (ports.CalendarSnapshot, error) {
	if snap, ok := c.fresh(cycleID); ok {
		return snap, nil
	}

	readCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	snap, err := c.source.NonWorkingDays(readCtx, cycleID)
	if err != nil {
		return c.degrade(cycleID), nil
	}
	snap.Version = versionOf(snap.Days)
	snap.Stale = false
	c.store(cycleID, snap)
	return snap, nil
}

// fresh returns the cached snapshot while it is still within the TTL.
func (c *CachedCalendar) fresh(cycleID string) (ports.CalendarSnapshot, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.cached[cycleID]
	if !ok || time.Since(entry.fetched) >= c.ttl {
		return ports.CalendarSnapshot{}, false
	}
	return entry.snap, true
}

// degrade is what a failed read falls back to: the last calendar we held, or —
// with nothing held — no closed days at all. Weekends still do not count as
// working days; that rule lives in pkg/schoolcal and needs no calendar. Either
// way the snapshot is marked Stale so what was computed from it is auditable.
func (c *CachedCalendar) degrade(cycleID string) ports.CalendarSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.cached[cycleID]; ok {
		snap := entry.snap
		snap.Stale = true
		return snap
	}
	return ports.CalendarSnapshot{Version: VersionUnavailable, Stale: true}
}

func (c *CachedCalendar) store(cycleID string, snap ports.CalendarSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cached[cycleID] = cacheEntry{snap: snap, fetched: time.Now()}
}

// versionOf fingerprints a calendar. Two schools' calendars, or the same
// calendar before and after a holiday is added, must not share a version — that
// is the whole value of stamping it on a payment. Truncated to 16 hex characters:
// enough to distinguish calendars, short enough to read in a ledger row.
func versionOf(days []time.Time) string {
	h := sha256.New()
	for _, d := range days {
		h.Write([]byte(d.Format("2006-01-02")))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
