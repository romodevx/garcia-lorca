package middleware_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// BenchmarkRateLimiterAllow measures the steady-state hot path: repeated Allow on
// a single, already-warmed key. Capacity/refill are large so the take-a-token
// branch dominates. Expect 0 allocs/op — an existing bucket touches no heap.
func BenchmarkRateLimiterAllow(b *testing.B) {
	rl := middleware.NewRateLimiter(1_000_000_000, 1_000_000_000, time.Now)
	rl.Allow("1.2.3.4") // warm the bucket so the loop never allocates one

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Allow("1.2.3.4")
	}
}

// BenchmarkRateLimiterAllowManyKeys measures the map-lookup path across many
// pre-warmed keys (realistic multi-IP traffic). Eviction is disabled by a high
// maxKeys so the loop is pure lookup + arithmetic. Expect 0 allocs/op.
func BenchmarkRateLimiterAllowManyKeys(b *testing.B) {
	const n = 1024
	keys := make([]string, n)
	rl := middleware.NewRateLimiter(1_000_000_000, 1_000_000_000, time.Now,
		middleware.WithMaxKeys(n*4))
	for i := range keys {
		keys[i] = "10.0." + strconv.Itoa(i/256) + "." + strconv.Itoa(i%256)
		rl.Allow(keys[i]) // warm every bucket
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Allow(keys[i&(n-1)])
	}
}
