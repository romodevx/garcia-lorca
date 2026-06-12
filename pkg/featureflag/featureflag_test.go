package featureflag_test

import (
	"context"
	"testing"

	"github.com/garcia-lorca/pkg/featureflag"
)

func TestKey(t *testing.T) {
	if got := featureflag.Key("iam"); got != "module_iam" {
		t.Fatalf("Key(iam) = %q, want module_iam", got)
	}
}

func TestInMemoryDefaultDeny(t *testing.T) {
	ctx := context.Background()
	checker := featureflag.NewInMemory(map[string]bool{
		featureflag.Key("iam"):      true,
		featureflag.Key("students"): false,
	})

	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"explicitly enabled", featureflag.Key("iam"), true},
		{"explicitly disabled", featureflag.Key("students"), false},
		{"unregistered key denied", featureflag.Key("finance"), false},
		{"non-module key denied", "random_flag", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checker.Enabled(ctx, tt.key); got != tt.want {
				t.Fatalf("Enabled(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestInMemoryIsolatedFromSourceMap(t *testing.T) {
	src := map[string]bool{featureflag.Key("iam"): true}
	checker := featureflag.NewInMemory(src)
	src[featureflag.Key("iam")] = false // mutate after construction

	if !checker.Enabled(context.Background(), featureflag.Key("iam")) {
		t.Fatal("checker must copy its source map and ignore later mutation")
	}
}

func TestAllowAll(t *testing.T) {
	var c featureflag.AllowAll
	if !c.Enabled(context.Background(), featureflag.Key("anything")) {
		t.Fatal("AllowAll should enable module_* keys")
	}
	if c.Enabled(context.Background(), "not_a_module") {
		t.Fatal("AllowAll should still reject non-module keys")
	}
}
