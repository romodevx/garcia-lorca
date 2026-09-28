package config_test

import (
	"bytes"
	"io"
	"log/slog"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/config"
)

func TestParseDeletionTokenKey(t *testing.T) {
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	env := func(v string) func(string) string {
		return func(k string) string {
			if k == "LEVEL_DELETION_TOKEN_KEY" {
				return v
			}
			return ""
		}
	}
	strong := "0123456789abcdef0123456789abcdef"

	if got := config.ParseDeletionTokenKey(env(strong), quiet); !bytes.Equal(got, []byte(strong)) {
		t.Errorf("a 32-byte key is used as given, got %q", got)
	}
	for name, v := range map[string]string{"unset": "", "too short": "short"} {
		t.Run(name, func(t *testing.T) {
			a := config.ParseDeletionTokenKey(env(v), quiet)
			b := config.ParseDeletionTokenKey(env(v), quiet)
			if len(a) < 32 {
				t.Fatalf("fallback key is %d bytes, want >= 32", len(a))
			}
			if bytes.Equal(a, b) {
				t.Fatal("the fallback must be random per process, never a constant")
			}
		})
	}
}
