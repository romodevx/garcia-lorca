package config_test

import (
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/config"
)

// env builds a getenv-style lookup from a fixed map for deterministic tests.
func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestParseLoginRateLimitDefaults(t *testing.T) {
	got := config.ParseLoginRateLimit(env(nil))
	if got.Burst != 10 {
		t.Errorf("Burst = %d, want default 10", got.Burst)
	}
	if got.PerMinute != 30 {
		t.Errorf("PerMinute = %d, want default 30", got.PerMinute)
	}
	// refillPerSec derives from PerMinute: 30/60 = 0.5 tokens/sec.
	if got.RefillPerSec() != 0.5 {
		t.Errorf("RefillPerSec() = %v, want 0.5", got.RefillPerSec())
	}
}

func TestParseLoginRateLimitOverrides(t *testing.T) {
	got := config.ParseLoginRateLimit(env(map[string]string{
		"LOGIN_RATELIMIT_BURST":      "5",
		"LOGIN_RATELIMIT_PER_MINUTE": "60",
	}))
	if got.Burst != 5 {
		t.Errorf("Burst = %d, want 5", got.Burst)
	}
	if got.PerMinute != 60 {
		t.Errorf("PerMinute = %d, want 60", got.PerMinute)
	}
	if got.RefillPerSec() != 1 {
		t.Errorf("RefillPerSec() = %v, want 1", got.RefillPerSec())
	}
}

func TestParseLoginRateLimitInvalidFallsBackToDefault(t *testing.T) {
	cases := []struct {
		name string
		envs map[string]string
	}{
		{"non-numeric", map[string]string{"LOGIN_RATELIMIT_BURST": "abc", "LOGIN_RATELIMIT_PER_MINUTE": "xyz"}},
		{"zero", map[string]string{"LOGIN_RATELIMIT_BURST": "0", "LOGIN_RATELIMIT_PER_MINUTE": "0"}},
		{"negative", map[string]string{"LOGIN_RATELIMIT_BURST": "-3", "LOGIN_RATELIMIT_PER_MINUTE": "-9"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := config.ParseLoginRateLimit(env(tc.envs))
			if got.Burst != 10 {
				t.Errorf("Burst = %d, want default 10 (invalid input ignored)", got.Burst)
			}
			if got.PerMinute != 30 {
				t.Errorf("PerMinute = %d, want default 30 (invalid input ignored)", got.PerMinute)
			}
		})
	}
}
