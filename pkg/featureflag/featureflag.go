// Package featureflag defines the feature-flag boundary for García Lorca.
//
// Every module ships behind a flag named "module_<name>" that defaults OFF.
// Server-side enforcement is the security boundary: the gateway's feature-flag
// middleware consumes a Checker and turns a disabled module's routes into 404s.
// Frontend checks are UX only.
//
// The contract is intentionally tiny — Enabled(ctx, key) bool — so it can be
// backed by an in-memory map (tests, local dev, presentation presets), a
// database table, or a remote flag service without touching call sites.
package featureflag

import (
	"context"
	"strings"
)

// keyPrefix is the mandatory prefix for module flags.
const keyPrefix = "module_"

// Checker reports whether a flag is enabled for the current request context.
//
// Semantics are DEFAULT-DENY: an unknown or unregistered key MUST return false.
// This guarantees that forgetting to register a flag fails closed (the module
// stays hidden) rather than open.
type Checker interface {
	Enabled(ctx context.Context, key string) bool
}

// Key builds the canonical flag key for a module name ("iam" -> "module_iam").
func Key(module string) string {
	return keyPrefix + module
}

// InMemory is a concurrency-safe-for-reads Checker backed by a static map. It is
// suitable for tests, local development, and applying presentation presets. The
// map is treated as immutable after construction.
type InMemory struct {
	enabled map[string]bool
}

// NewInMemory builds an InMemory Checker. Only keys present and set to true are
// enabled; everything else is denied (default-deny).
func NewInMemory(flags map[string]bool) *InMemory {
	cp := make(map[string]bool, len(flags))
	for k, v := range flags {
		cp[k] = v
	}
	return &InMemory{enabled: cp}
}

// Enabled implements Checker with default-deny semantics.
func (m *InMemory) Enabled(_ context.Context, key string) bool {
	if !strings.HasPrefix(key, keyPrefix) {
		// Defensive: only module_* keys are valid flags. Anything else is denied.
		return false
	}
	return m.enabled[key]
}

// AllowAll is a Checker that enables every module_* flag. It exists only for
// the hello-world / pipeline-validation path and local demos; it must never be
// wired into a shared environment.
type AllowAll struct{}

// Enabled returns true for any module_* key, false otherwise.
func (AllowAll) Enabled(_ context.Context, key string) bool {
	return strings.HasPrefix(key, keyPrefix)
}
