package config_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"

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
	// FLAGS.md registers 24 module flags; the enumeration must match so
	// GET /features reports every module. (M14 added module_teachers; task 13,
	// module_student_card; T10, module_expenses; T16, module_schedule; T17,
	// module_activities; the legacy student import, module_student_import; the
	// Configuración setup view, module_config_setup; the app-wide look refresh,
	// module_ui_refresh; the Avisos view, module_announcements_view; the
	// «Estructura de la escuela» step, module_school_structure; the import
	// review, module_import_review.)
	if len(config.AllModuleFlags) != 24 {
		t.Errorf("AllModuleFlags has %d entries, want 24", len(config.AllModuleFlags))
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

// The import review (PLAN-import-avisos) ships behind its own flag, default
// OFF: enabling students, config and the import does not enable it, and the
// local compose default list does not carry it.
func TestImportReviewFlagIsRegisteredAndDefaultOff(t *testing.T) {
	const key = "module_import_review"
	if !slices.Contains(config.AllModuleFlags, key) {
		t.Fatalf("AllModuleFlags does not report %q: /features would hide the review", key)
	}
	if config.ParseEnabledModules("students,config,student_import")[key] {
		t.Errorf("%q must stay OFF unless listed explicitly", key)
	}
	if config.ParseEnabledModules(composeDefaultModules(t))[key] {
		t.Errorf("%q must stay OFF in the local compose default list until its gates pass", key)
	}
	if !config.ParseEnabledModules("students,config,student_import,import_review")[key] {
		t.Errorf("listing import_review must enable %q", key)
	}
}

// composeFile is the repo-root docker-compose.yml, seen from this package.
var composeFile = filepath.Join("..", "..", "..", "..", "docker-compose.yml")

// composeDefault matches the gateway's `${ENABLED_MODULES:-<default>}`.
var composeDefault = regexp.MustCompile(`^\$\{ENABLED_MODULES:-([^}]*)\}$`)

// composeDefaultModules reads the default ENABLED_MODULES of the gateway in
// docker-compose.yml, so the test follows the local stack instead of a copy
// of it that silently drifts.
func composeDefaultModules(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(composeFile)
	if err != nil {
		t.Fatalf("read %s: %v", composeFile, err)
	}
	var compose struct {
		Services map[string]struct {
			Environment map[string]string `yaml:"environment"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &compose); err != nil {
		t.Fatalf("parse %s: %v", composeFile, err)
	}
	m := composeDefault.FindStringSubmatch(compose.Services["gateway"].Environment["ENABLED_MODULES"])
	if m == nil || m[1] == "" {
		t.Fatalf("%s: gateway ENABLED_MODULES has no ${ENABLED_MODULES:-<default>}", composeFile)
	}
	return m[1]
}

// Flags that only switch a presentation (no routes of their own) still reach
// the frontend through GET /features, and stay OFF unless listed explicitly:
// enabling the module they dress up does not enable them.
func TestPresentationFlagsAreRegisteredAndDefaultOff(t *testing.T) {
	composeModules := composeDefaultModules(t)
	tests := []struct {
		key      string
		baseline string
		enabling string
	}{
		{key: "module_config_setup", baseline: "config", enabling: "config,config_setup"},
		{key: "module_announcements_view", baseline: "comms", enabling: "comms,announcements_view"},
		{key: "module_school_structure", baseline: "config,config_setup", enabling: "config,config_setup,school_structure"},
		// The app-wide look (PLAN-redesign-home F2b) dresses every module, so
		// its baseline is the whole local compose list.
		{key: "module_ui_refresh", baseline: composeModules, enabling: composeModules + ",ui_refresh"},
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
