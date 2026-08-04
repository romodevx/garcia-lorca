package middleware

import (
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RateLimiter is a per-key (client IP) token-bucket limiter for a single gateway
// instance. It is in-process (no shared state across instances) — sufficient for
// the MVP gateway (min-instances=1). Each Allow is O(1) under a single mutex and
// allocation-free in steady state (an existing key touches no heap). Stale
// buckets are evicted lazily so long-lived processes stay bounded in memory.
//
// An unparseable client IP is NOT fail-open: it collapses to a single shared
// bucket (the raw RemoteAddr string), so malformed callers throttle together
// rather than each escaping the limit.
type RateLimiter struct {
	mu           sync.Mutex
	buckets      map[string]*bucket
	capacity     float64
	refillPerSec float64
	now          func() time.Time
	maxKeys      int
	evictWindow  time.Duration
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
			rl.evictLocked(now)
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

// evictLocked drops every bucket untouched for longer than evictWindow. The
// caller must hold rl.mu. It is O(n) but runs only when the map crosses maxKeys.
func (rl *RateLimiter) evictLocked(now time.Time) {
	cutoff := now.Add(-rl.evictWindow)
	for key, b := range rl.buckets {
		if b.last.Before(cutoff) {
			delete(rl.buckets, key)
		}
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
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, retryAfter := rl.Allow(rl.clientIP(r))
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
// the left-most X-Forwarded-For entry — the address the trusted proxy recorded
// for the real client. Trusting XFF without a proxy would let any caller spoof a
// fresh key per request and bypass the limit, so it stays off by default.
func (rl *RateLimiter) clientIP(r *http.Request) string {
	if rl.trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if comma := strings.IndexByte(xff, ','); comma >= 0 {
				return strings.TrimSpace(xff[:comma])
			}
			return strings.TrimSpace(xff)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
