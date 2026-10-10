package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garcia-lorca/pkg/featureflag"
	"github.com/garcia-lorca/services/gateway/internal/config"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

func TestFeaturesReportsEnabledAndDisabled(t *testing.T) {
	flags := featureflag.NewInMemory(map[string]bool{"module_students": true, "module_finance": true})
	h := handler.NewFeaturesHandler(flags)

	rec := httptest.NewRecorder()
	h.Features(rec, httptest.NewRequest(http.MethodGet, "/api/v1/features", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var got map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Every registered module is present (default-deny for the rest).
	if len(got) != len(config.AllModuleFlags) {
		t.Errorf("got %d flags, want %d", len(got), len(config.AllModuleFlags))
	}
	if !got["module_students"] || !got["module_finance"] {
		t.Errorf("enabled modules should be true: %v", got)
	}
	if got["module_billing"] || got["module_grades"] {
		t.Errorf("unlisted modules must be false (default-deny): %v", got)
	}
}

func TestFeaturesAllOffByDefault(t *testing.T) {
	h := handler.NewFeaturesHandler(featureflag.NewInMemory(map[string]bool{}))
	rec := httptest.NewRecorder()
	h.Features(rec, httptest.NewRequest(http.MethodGet, "/api/v1/features", nil))

	var got map[string]bool
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	for key, enabled := range got {
		if enabled {
			t.Errorf("%q should be false by default", key)
		}
	}
}

// The Configuración setup view (module_config_setup) is reported, and OFF while
// only module_config is enabled.
func TestFeaturesReportsConfigSetupOff(t *testing.T) {
	h := handler.NewFeaturesHandler(featureflag.NewInMemory(config.ParseEnabledModules("config")))
	rec := httptest.NewRecorder()
	h.Features(rec, httptest.NewRequest(http.MethodGet, "/api/v1/features", nil))

	var got map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	enabled, reported := got["module_config_setup"]
	if !reported {
		t.Fatalf("/features does not report module_config_setup: %v", got)
	}
	if enabled {
		t.Errorf("module_config_setup must be false by default")
	}
}
