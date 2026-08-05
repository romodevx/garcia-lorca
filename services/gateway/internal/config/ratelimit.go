package config

import "strconv"

const (
	// defaultLoginBurst is the token-bucket capacity for /auth/login per client IP:
	// how many login attempts may arrive back-to-back before throttling kicks in.
	defaultLoginBurst = 10
	// defaultLoginPerMinute is the sustained refill rate (attempts per minute per
	// client IP) once the burst is spent.
	defaultLoginPerMinute = 30
	// secondsPerMinute converts the per-minute knob into the limiter's per-second
	// refill rate.
	secondsPerMinute = 60.0
)

// LoginRateLimit is the resolved per-IP throttle for the login endpoint.
type LoginRateLimit struct {
	Burst     int // token-bucket capacity (burst size)
	PerMinute int // sustained attempts per minute per IP
}

// RefillPerSec is the token-bucket refill rate the limiter consumes.
func (l LoginRateLimit) RefillPerSec() float64 {
	return float64(l.PerMinute) / secondsPerMinute
}

// ParseLoginRateLimit resolves the login throttle from the environment via
// getenv (injected for testability). LOGIN_RATELIMIT_BURST and
// LOGIN_RATELIMIT_PER_MINUTE override the defaults; any missing, non-numeric, or
// non-positive value falls back to the default so a bad env never disables the
// throttle.
func ParseLoginRateLimit(getenv func(string) string) LoginRateLimit {
	return LoginRateLimit{
		Burst:     positiveIntOr(getenv("LOGIN_RATELIMIT_BURST"), defaultLoginBurst),
		PerMinute: positiveIntOr(getenv("LOGIN_RATELIMIT_PER_MINUTE"), defaultLoginPerMinute),
	}
}

// positiveIntOr parses raw as a positive integer, returning fallback when it is
// empty, non-numeric, or not > 0.
func positiveIntOr(raw string, fallback int) int {
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return n
	}
	return fallback
}
