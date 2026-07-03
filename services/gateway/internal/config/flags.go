// Package config holds the gateway's composition-root configuration helpers,
// including the feature-flag source. The gateway is the authoritative flag
// boundary (FLAGS.md): it resolves which modules are enabled and both enforces
// the boundary (disabled ⇒ 404) and reports it (GET /features) to the frontend.
package config

import (
	"strings"

	"github.com/garcia-lorca/pkg/featureflag"
)

// AllModuleFlags is the canonical list of every module flag key (FLAGS.md).
// GET /features reports the enabled state of each so the frontend never guesses.
var AllModuleFlags = []string{
	featureflag.Key("iam"),
	featureflag.Key("gateway"),
	featureflag.Key("students"),
	featureflag.Key("config"),
	featureflag.Key("finance"),
	featureflag.Key("attendance"),
	featureflag.Key("grades"),
	featureflag.Key("scholarships"),
	featureflag.Key("docs"),
	featureflag.Key("comms"),
	featureflag.Key("billing"),
	featureflag.Key("uniforms"),
	featureflag.Key("reports"),
}

// ParseEnabledModules turns a comma-separated list of module names (or full
// module_* keys) into a default-deny flag map. Blank entries are ignored. Only
// listed modules are enabled; everything else stays OFF (FLAGS.md default-deny).
//
//	"students, finance"        → {module_students:true, module_finance:true}
//	"module_grades,attendance" → {module_grades:true, module_attendance:true}
func ParseEnabledModules(csv string) map[string]bool {
	flags := map[string]bool{}
	for _, raw := range strings.Split(csv, ",") {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		key := name
		if !strings.HasPrefix(key, "module_") {
			key = featureflag.Key(name)
		}
		flags[key] = true
	}
	return flags
}
