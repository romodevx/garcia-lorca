package middleware

import (
	"math"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RateLimiter is a per-key (client IP) token-bucket limiter for a single gateway
// instance. It is in-process (no shared state across instances) — sufficient for
// the MVP gateway (min-instances=1). Each Allow on an existing key is O(1) under a
// single mutex and allocation-free (an existing key touches no heap).
//
// Memory is a HARD bound: the bucket map never exceeds maxKeys. Idle buckets are
// reclaimed by a time-throttled sweep that runs at most once per evictWindow, so a
// within-window flood of distinct IPs does NOT pay an O(maxKeys) scan on every new
// key — amortized new-key cost stays O(1). If a burst of all-fresh keys outpaces the
// sweep, the oldest buckets (by last-touch) are dropped in a batch to hold the cap.
//
// An unparseable RemoteAddr is NOT fail-open: it collapses to a single shared bucket
// (the raw RemoteAddr string), so malformed callers throttle together rather than
// each escaping the limit.
type RateLimiter struct {
	mu           sync.Mutex
	buckets      map[string]*bucket
	capacity     float64
	refillPerSec float64
	now          func() time.Time
	maxKeys      int
	evictWindow  time.Duration
	nextSweep    time.Time
	sweeps       int
	trustProxy   bool
}

// bucket is one key's token reservoir. tokens is fractional so refill accrues
// continuously; last is the instant tokens was last recomputed.
type bucket struct {
	tokens float64
	last   time.Time
}

const (
	// defaultMaxKeys caps distinct tracked IPs before a lazy eviction sweep runs.
	defaultMaxKeys = 10000
	// defaultEvictWindow is how long an untouched bucket survives a sweep.
	defaultEvictWindow = 10 * time.Minute
	// evictBatchDivisor sets the hard-cap batch size: when the map is full of fresh
	// keys the oldest 1/N are dropped at once, leaving headroom so the O(n) oldest-
	// selection amortizes to O(1) per insert instead of firing on every new key.
	evictBatchDivisor = 8
)

// Option customizes a RateLimiter at construction (functional options keep the
// common NewRateLimiter(cap, refill, now) call site clean).
type Option func(*RateLimiter)

// WithMaxKeys sets the distinct-key cap that triggers lazy eviction. Ignored if
// n <= 0.
func WithMaxKeys(n int) Option {
	return func(rl *RateLimiter) {
		if n > 0 {
			rl.maxKeys = n
		}
	}
}

// WithEvictWindow sets how long an untouched bucket survives an eviction sweep.
// Ignored if d <= 0.
func WithEvictWindow(d time.Duration) Option {
	return func(rl *RateLimiter) {
		if d > 0 {
			rl.evictWindow = d
		}
	}
}

// NewRateLimiter builds a token-bucket limiter with the given burst capacity and
// steady refill rate (tokens per second). now is the injectable clock; when nil
// it defaults to time.Now (tests pass a deterministic clock). Whether a proxy's
// X-Forwarded-For is trusted is read once from TRUST_PROXY.
func NewRateLimiter(capacity int, refillPerSec float64, now func() time.Time, opts ...Option) *RateLimiter {
	if now == nil {
		now = time.Now
	}
	rl := &RateLimiter{
		buckets:      make(map[string]*bucket),
		capacity:     float64(capacity),
		refillPerSec: refillPerSec,
		now:          now,
		maxKeys:      defaultMaxKeys,
		evictWindow:  defaultEvictWindow,
		trustProxy:   os.Getenv("TRUST_PROXY") != "",
	}
	for _, opt := range opts {
		opt(rl)
	}
	return rl
}

// Allow reports whether the key may take one token now. It refills the key's
// bucket by elapsed*refillPerSec (capped at capacity), then takes a token if at
// least one is available. On denial it returns the time until the next token.
func (rl *RateLimiter) Allow(key string) (ok bool, retryAfter time.Duration) {
	now := rl.now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	b := rl.buckets[key]
	if b == nil {
		if len(rl.buckets) >= rl.maxKeys {
			rl.enforceCapLocked(now)
		}
		rl.buckets[key] = &bucket{tokens: rl.capacity, last: now}
		b = rl.buckets[key]
	} else if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens += elapsed * rl.refillPerSec
		if b.tokens > rl.capacity {
			b.tokens = rl.capacity
		}
		b.last = now
	}

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	return false, rl.timeToNextToken(b.tokens)
}

// timeToNextToken is how long until the bucket accrues enough to reach one token.
// With a non-positive refill rate a denied caller can never recover, so it
// reports the eviction window as a bounded, honest Retry-After.
func (rl *RateLimiter) timeToNextToken(tokens float64) time.Duration {
	if rl.refillPerSec <= 0 {
		return rl.evictWindow
	}
	seconds := (1 - tokens) / rl.refillPerSec
	return time.Duration(seconds * float64(time.Second))
}

// enforceCapLocked keeps the bucket map within maxKeys. The caller must hold rl.mu
// and call it only on the new-key path (never on the O(1) existing-key hot path).
//
// It first runs a time-throttled idle sweep — at most once per evictWindow — which
// is the common case: buckets that went idle age out and are reclaimed cheaply. The
// throttle is what bounds WORK: a within-window flood of distinct IPs no longer
// triggers an O(maxKeys) scan on every new key, so amortized new-key cost is O(1).
//
// The idle sweep cannot free a burst of all-fresh keys (none are old enough), so a
// hard-cap fallback drops the oldest buckets (by last-touch) in a batch down to a
// low-water mark below maxKeys. Batching amortizes the O(n) oldest-selection across
// many inserts, so even a sustained fresh-key flood stays O(1) per request while the
// map remains a HARD cap — never more than maxKeys entries.
func (rl *RateLimiter) enforceCapLocked(now time.Time) {
	if !now.Before(rl.nextSweep) {
		rl.sweepIdleLocked(now)
		rl.nextSweep = now.Add(rl.evictWindow)
	}
	if len(rl.buckets) >= rl.maxKeys {
		headroom := rl.maxKeys / evictBatchDivisor
		if headroom < 1 {
			headroom = 1
		}
		target := rl.maxKeys - headroom
		rl.dropOldestLocked(len(rl.buckets) - target)
	}
}

// sweepIdleLocked drops every bucket untouched for longer than evictWindow. The
// caller must hold rl.mu. It is O(n) but the throttle in enforceCapLocked bounds how
// often it runs. sweeps is incremented for observability in tests.
func (rl *RateLimiter) sweepIdleLocked(now time.Time) {
	rl.sweeps++
	cutoff := now.Add(-rl.evictWindow)
	for key, b := range rl.buckets {
		if b.last.Before(cutoff) {
			delete(rl.buckets, key)
		}
	}
}

// dropOldestLocked evicts the count least-recently-touched buckets. The caller must
// hold rl.mu. It allocates and sorts, but runs only on the hard-cap fallback path
// (never the hot path) and, because enforceCapLocked drops in batches to a low-water
// mark, only once per batch of inserts.
func (rl *RateLimiter) dropOldestLocked(count int) {
	if count <= 0 {
		return
	}
	type keyed struct {
		key  string
		last time.Time
	}
	entries := make([]keyed, 0, len(rl.buckets))
	for key, b := range rl.buckets {
		entries = append(entries, keyed{key: key, last: b.last})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].last.Before(entries[j].last)
	})
	if count > len(entries) {
		count = len(entries)
	}
	for i := 0; i < count; i++ {
		delete(rl.buckets, entries[i].key)
	}
}

// Len reports the number of tracked buckets (exposed for eviction tests).
func (rl *RateLimiter) Len() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return len(rl.buckets)
}

// Middleware throttles requests per client IP. On denial it sets Retry-After
// (whole seconds, rounded up) and returns 429; otherwise it calls next.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return rl.middlewareKeyedBy(rl.clientIP, next)
}

// principalKeyPrefix keeps principal keys apart from IP keys in a shared bucket map.
const principalKeyPrefix = "principal:"

// PerPrincipalMiddleware throttles by the authenticated principal instead of
// the client IP. Behind Cloud Run every request arrives from the Google front
// end, so an IP key is one budget for the whole school; for a password check
// that already sits behind a session, the person is the right key. It must
// run AFTER RequireAuth; with no principal it falls back to the client IP.
func (rl *RateLimiter) PerPrincipalMiddleware(next http.Handler) http.Handler {
	return rl.middlewareKeyedBy(func(r *http.Request) string {
		if p, ok := PrincipalFrom(r.Context()); ok && p.UserID != "" {
			return principalKeyPrefix + p.UserID
		}
		return rl.clientIP(r)
	}, next)
}

func (rl *RateLimiter) middlewareKeyedBy(key func(*http.Request) string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, retryAfter := rl.Allow(key(r))
		if !ok {
			seconds := int(math.Ceil(retryAfter.Seconds()))
			if seconds < 1 {
				seconds = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP is the limiter key for a request. By default it is the connection's
// remote host (RemoteAddr minus port). Only when TRUST_PROXY is set does it honor
// X-Forwarded-For, and then it takes the RIGHT-most entry: with a single trusted
// proxy, that proxy appends the address it actually observed (the real client) to
// the end of the list, while every value to its left is supplied by the caller and
// is therefore spoofable. Keying on the left-most would let a client rotate a fake
// token per request and slip the limit; the right-most is the only hop we trust.
//
// The right-most value must parse as an IP; a missing, blank, or unparseable XFF
// falls back to the RemoteAddr host — never a blank or bogus shared bucket. Trusting
// XFF without a proxy would let any caller spoof a fresh key, so it stays off by
// default.
func (rl *RateLimiter) clientIP(r *http.Request) string {
	if rl.trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			candidate := xff
			if comma := strings.LastIndexByte(xff, ','); comma >= 0 {
				candidate = xff[comma+1:]
			}
			candidate = strings.TrimSpace(candidate)
			if net.ParseIP(candidate) != nil {
				return candidate
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
