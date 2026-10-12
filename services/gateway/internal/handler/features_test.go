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

// Presentation-only views are reported, and OFF while only the module they
// dress up is enabled: module_config_setup over config, module_announcements_view
// over comms.
func TestFeaturesReportsPresentationViewsOff(t *testing.T) {
	tests := []struct {
		key     string
		modules string
	}{
		{key: "module_config_setup", modules: "config"},
		{key: "module_announcements_view", modules: "comms"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			h := handler.NewFeaturesHandler(featureflag.NewInMemory(config.ParseEnabledModules(tt.modules)))
			rec := httptest.NewRecorder()
			h.Features(rec, httptest.NewRequest(http.MethodGet, "/api/v1/features", nil))

			var got map[string]bool
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			enabled, reported := got[tt.key]
			if !reported {
				t.Fatalf("/features does not report %s: %v", tt.key, got)
			}
			if enabled {
				t.Errorf("%s must be false by default", tt.key)
			}
		})
	}
}
