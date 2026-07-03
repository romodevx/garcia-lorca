package config_test

import (
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/config"
)

func TestParseEnabledModules(t *testing.T) {
	got := config.ParseEnabledModules(" students, finance ,, attendance ")
	for _, key := range []string{"module_students", "module_finance", "module_attendance"} {
		if !got[key] {
			t.Errorf("expected %q enabled, got %v", key, got)
		}
	}
	// Blank/whitespace entries are ignored, not turned into "module_".
	if _, bad := got["module_"]; bad {
		t.Errorf("blank entry leaked a module_ key: %v", got)
	}
	if got["module_billing"] {
		t.Errorf("unlisted module should be absent/false")
	}
}

func TestParseEnabledModulesEmpty(t *testing.T) {
	if len(config.ParseEnabledModules("")) != 0 {
		t.Errorf("empty input should yield no enabled flags")
	}
}

func TestParseEnabledModulesAcceptsFullKeys(t *testing.T) {
	// Tolerate both "students" and "module_students".
	got := config.ParseEnabledModules("module_students,grades")
	if !got["module_students"] || !got["module_grades"] {
		t.Errorf("should accept both bare names and module_ keys: %v", got)
	}
}

func TestAllModuleFlagsCoversRegistry(t *testing.T) {
	// FLAGS.md registers 13 module flags; the enumeration must match so
	// GET /features reports every module.
	if len(config.AllModuleFlags) != 13 {
		t.Errorf("AllModuleFlags has %d entries, want 13", len(config.AllModuleFlags))
	}
	seen := map[string]bool{}
	for _, k := range config.AllModuleFlags {
		if seen[k] {
			t.Errorf("duplicate flag key %q", k)
		}
		seen[k] = true
	}
	if !seen["module_students"] || !seen["module_iam"] {
		t.Errorf("expected core module keys present: %v", config.AllModuleFlags)
	}
}
