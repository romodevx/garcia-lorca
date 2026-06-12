package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/middleware"

	"github.com/garcia-lorca/pkg/featureflag"
)

func TestModuleGuard(t *testing.T) {
	reached := false
	protected := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name       string
		flags      map[string]bool
		wantStatus int
		wantReach  bool
	}{
		{
			name:       "enabled module serves through",
			flags:      map[string]bool{featureflag.Key("students"): true},
			wantStatus: http.StatusOK,
			wantReach:  true,
		},
		{
			name:       "disabled module is 404 (invisible)",
			flags:      map[string]bool{featureflag.Key("students"): false},
			wantStatus: http.StatusNotFound,
			wantReach:  false,
		},
		{
			name:       "unregistered module is 404 (default-deny)",
			flags:      map[string]bool{},
			wantStatus: http.StatusNotFound,
			wantReach:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reached = false
			guard := middleware.NewModuleGuard(featureflag.NewInMemory(tt.flags))
			h := guard.Protect("students", protected)

			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/students/x", nil))

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
			if reached != tt.wantReach {
				t.Errorf("inner handler reached = %v, want %v", reached, tt.wantReach)
			}
		})
	}
}
