package handler

import (
	"encoding/json"
	"net/http"

	"github.com/garcia-lorca/pkg/featureflag"
	"github.com/garcia-lorca/services/gateway/internal/config"
)

// FeaturesHandler serves the resolved module-flag map so the frontend can gate
// its UI from ONE authoritative source (the gateway) instead of hardcoding it.
// This is UX plumbing — the gateway's ModuleGuard is the real security boundary
// (a disabled module's routes 404 regardless of what /features reports).
type FeaturesHandler struct {
	flags featureflag.Checker
}

// NewFeaturesHandler wires the handler to the flag checker.
func NewFeaturesHandler(flags featureflag.Checker) *FeaturesHandler {
	return &FeaturesHandler{flags: flags}
}

// Features handles GET /api/v1/features: returns { "module_<name>": bool } for
// every registered module flag (default-deny — absent modules report false).
func (h *FeaturesHandler) Features(w http.ResponseWriter, r *http.Request) {
	out := make(map[string]bool, len(config.AllModuleFlags))
	for _, key := range config.AllModuleFlags {
		out[key] = h.flags.Enabled(r.Context(), key)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}
