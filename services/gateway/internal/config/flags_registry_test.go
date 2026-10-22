package config_test

import (
	"testing"

	"github.com/garcia-lorca/pkg/featureflag"
	"github.com/garcia-lorca/services/gateway/internal/config"
)

// TestEveryGuardedModuleIsReportedByFeatures closes a gap that cost a whole
// module its screen: a route can be guarded by guard.Protect("teachers", …) and
// work perfectly, while GET /features never mentions module_teachers — so the
// frontend, which defaults every flag OFF, renders the "próximamente" stub over
// a module that is fully alive. Nothing fails; the module is simply invisible.
//
// The list here is the set of module names the gateway actually guards. Adding a
// guard without adding the key is the mistake this catches.
func TestEveryGuardedModuleIsReportedByFeatures(t *testing.T) {
	guarded := []string{
		"iam", "gateway", "students", "config", "teachers", "finance",
		"attendance", "grades", "comms", "uniforms", "student_card", "expenses", "schedule", "activities", "student_import",
		"import_review", "payment_vouchers",
	}

	reported := make(map[string]bool, len(config.AllModuleFlags))
	for _, key := range config.AllModuleFlags {
		reported[key] = true
	}

	for _, name := range guarded {
		if !reported[featureflag.Key(name)] {
			t.Errorf("module %q is guarded but /features never reports %q: the frontend will hide it",
				name, featureflag.Key(name))
		}
	}
}
