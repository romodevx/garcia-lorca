package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// clock is a deterministic, injectable time source: no real sleeps in tests.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newClock() *clock { return &clock{t: time.Unix(0, 0)} }

func TestRateLimiterAllowsBurstThenDenies(t *testing.T) {
	clk := newClock()
	rl := middleware.NewRateLimiter(3, 1, clk.now) // capacity 3, refill 1/s

	for i := 1; i <= 3; i++ {
		ok, retry := rl.Allow("1.2.3.4")
		if !ok {
			t.Fatalf("request %d: denied, want allowed", i)
		}
		if retry != 0 {
			t.Errorf("request %d: retryAfter = %v, want 0 while allowed", i, retry)
		}
	}

	ok, retry := rl.Allow("1.2.3.4")
	if ok {
		t.Fatal("4th request: allowed, want denied (burst exhausted)")
	}
	if retry <= 0 {
		t.Errorf("4th request: retryAfter = %v, want > 0", retry)
	}
}

func TestRateLimiterPerKeyIndependent(t *testing.T) {
	clk := newClock()
	rl := middleware.NewRateLimiter(1, 1, clk.now)

	if ok, _ := rl.Allow("1.2.3.4"); !ok {
		t.Fatal("first request from 1.2.3.4 should pass")
	}
	if ok, _ := rl.Allow("1.2.3.4"); ok {
		t.Fatal("second request from 1.2.3.4 should be denied")
	}
	// A different IP has its own bucket, untouched by 1.2.3.4's traffic.
	if ok, _ := rl.Allow("5.6.7.8"); !ok {
		t.Fatal("first request from 5.6.7.8 should pass (independent bucket)")
	}
}

func TestRateLimiterRefillsOverTime(t *testing.T) {
	clk := newClock()
	rl := middleware.NewRateLimiter(1, 1, clk.now) // 1 token, refill 1/s

	if ok, _ := rl.Allow("1.2.3.4"); !ok {
		t.Fatal("first request should pass")
	}
	if ok, _ := rl.Allow("1.2.3.4"); ok {
		t.Fatal("second request should be denied (no tokens)")
	}
	clk.advance(time.Second) // one refill interval → +1 token
	if ok, _ := rl.Allow("1.2.3.4"); !ok {
		t.Fatal("after 1s the bucket should have refilled one token")
	}
}

func TestRateLimiterMiddlewareReturns429WithRetryAfter(t *testing.T) {
	clk := newClock()
	rl := middleware.NewRateLimiter(3, 1, clk.now)
	var served int
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served++
		w.WriteHeader(http.StatusOK)
	})
	h := rl.Middleware(next)

	call := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.RemoteAddr = "1.2.3.4:5555"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	for i := 1; i <= 3; i++ {
		if rec := call(); rec.Code != http.StatusOK {
			t.Fatalf("request %d: code = %d, want 200", i, rec.Code)
		}
	}
	rec := call()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("4th request: code = %d, want 429", rec.Code)
	}
	if ra := rec.Header().Get("Retry-After"); ra == "" {
		t.Error("429 response is missing a Retry-After header")
	} else if secs, err := strconv.Atoi(ra); err != nil || secs < 1 {
		t.Errorf("Retry-After = %q, want a positive integer seconds value", ra)
	}
	if served != 3 {
		t.Errorf("next handler served %d times, want 3 (4th blocked at the edge)", served)
	}
}

func TestRateLimiterMiddlewarePerIPIndependent(t *testing.T) {
	clk := newClock()
	rl := middleware.NewRateLimiter(1, 1, clk.now)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := rl.Middleware(next)

	call := func(remote string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := call("1.2.3.4:1111"); code != http.StatusOK {
		t.Fatalf("first from 1.2.3.4: code = %d, want 200", code)
	}
	if code := call("1.2.3.4:2222"); code != http.StatusTooManyRequests {
		t.Fatalf("second from 1.2.3.4 (different port, same IP): code = %d, want 429", code)
	}
	if code := call("9.9.9.9:1111"); code != http.StatusOK {
		t.Fatalf("first from 9.9.9.9: code = %d, want 200 (independent IP)", code)
	}
}

func TestRateLimiterMiddlewareIgnoresXFFWithoutTrustProxy(t *testing.T) {
	clk := newClock()
	rl := middleware.NewRateLimiter(1, 1, clk.now) // TRUST_PROXY unset → RemoteAddr wins
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := rl.Middleware(next)

	call := func(remote, xff string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.RemoteAddr = remote
		if xff != "" {
			req.Header.Set("X-Forwarded-For", xff)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	// Same RemoteAddr, spoofed different XFF: without TRUST_PROXY the limiter must
	// key on RemoteAddr, so the second request is throttled regardless of XFF.
	if code := call("1.2.3.4:1111", "8.8.8.8"); code != http.StatusOK {
		t.Fatalf("first: code = %d, want 200", code)
	}
	if code := call("1.2.3.4:2222", "9.9.9.9"); code != http.StatusTooManyRequests {
		t.Fatalf("second (spoofed XFF): code = %d, want 429 — XFF must be ignored", code)
	}
}

func TestRateLimiterMiddlewareHonorsXFFWhenTrustProxy(t *testing.T) {
	t.Setenv("TRUST_PROXY", "1")
	clk := newClock()
	rl := middleware.NewRateLimiter(1, 1, clk.now)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := rl.Middleware(next)

	call := func(remote, xff string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.RemoteAddr = remote
		req.Header.Set("X-Forwarded-For", xff)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	// With TRUST_PROXY on, the left-most XFF entry is the key. Same client IP
	// behind the proxy → throttled; a different left-most XFF → independent.
	if code := call("10.0.0.1:1", "1.2.3.4, 10.0.0.1"); code != http.StatusOK {
		t.Fatalf("first: code = %d, want 200", code)
	}
	if code := call("10.0.0.1:1", "1.2.3.4, 10.0.0.1"); code != http.StatusTooManyRequests {
		t.Fatalf("second (same left-most XFF): code = %d, want 429", code)
	}
	if code := call("10.0.0.1:1", "5.6.7.8, 10.0.0.1"); code != http.StatusOK {
		t.Fatalf("third (different left-most XFF): code = %d, want 200", code)
	}
}

func TestRateLimiterEvictionBoundsMap(t *testing.T) {
	clk := newClock()
	const maxKeys = 100
	rl := middleware.NewRateLimiter(1, 1, clk.now,
		middleware.WithMaxKeys(maxKeys),
		middleware.WithEvictWindow(10*time.Second))

	// Insert far more than maxKeys distinct keys, advancing the clock so older
	// buckets age past the eviction window and are swept lazily.
	for i := 0; i < maxKeys*20; i++ {
		clk.advance(time.Second)
		rl.Allow("key-" + strconv.Itoa(i))
	}

	if n := rl.Len(); n > maxKeys {
		t.Errorf("bucket map holds %d entries, want <= %d (eviction must bound memory)", n, maxKeys)
	}
}
