package config_test

import (
	"slices"
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
	// FLAGS.md registers 21 module flags; the enumeration must match so
	// GET /features reports every module. (M14 added module_teachers; task 13,
	// module_student_card; T10, module_expenses; T16, module_schedule; T17,
	// module_activities; the legacy student import, module_student_import; the
	// Configuración setup view, module_config_setup; the app-wide look refresh,
	// module_ui_refresh.)
	if len(config.AllModuleFlags) != 21 {
		t.Errorf("AllModuleFlags has %d entries, want 21", len(config.AllModuleFlags))
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

// The legacy student import (PLAN-student-import) ships behind its own flag:
// GET /features must report it, and it is default-deny — enabling students and
// config does NOT enable the import; only listing student_import does.
func TestStudentImportFlagIsRegisteredAndDefaultOff(t *testing.T) {
	const key = "module_student_import"
	if !slices.Contains(config.AllModuleFlags, key) {
		t.Fatalf("AllModuleFlags does not report %q: /features would hide the import", key)
	}
	if config.ParseEnabledModules("students,config")[key] {
		t.Errorf("%q must stay OFF unless listed explicitly", key)
	}
	if !config.ParseEnabledModules("students,config,student_import")[key] {
		t.Errorf("listing student_import must enable %q", key)
	}
}

// Flags that only switch a presentation (no routes of their own) still reach
// the frontend through GET /features, and stay OFF unless listed explicitly:
// enabling the module they dress up does not enable them.
// composeDefaultModules mirrors docker-compose.yml's default ENABLED_MODULES.
const composeDefaultModules = "iam,gateway,students,config,teachers,grades,finance,attendance,comms,uniforms,expenses,schedule,activities,scholarships,student_import"

func TestPresentationFlagsAreRegisteredAndDefaultOff(t *testing.T) {
	tests := []struct {
		key      string
		baseline string
		enabling string
	}{
		{key: "module_config_setup", baseline: "config", enabling: "config,config_setup"},
		// The app-wide look (PLAN-redesign-home F2b) dresses every module, so
		// its baseline is the whole local compose list.
		{key: "module_ui_refresh", baseline: composeDefaultModules, enabling: composeDefaultModules + ",ui_refresh"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if !slices.Contains(config.AllModuleFlags, tt.key) {
				t.Fatalf("AllModuleFlags does not report %q: /features would hide it", tt.key)
			}
			if config.ParseEnabledModules(tt.baseline)[tt.key] {
				t.Errorf("%q must stay OFF unless listed explicitly", tt.key)
			}
			if !config.ParseEnabledModules(tt.enabling)[tt.key] {
				t.Errorf("listing it must enable %q", tt.key)
			}
		})
	}
}
