package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// okHandler is the downstream the CORS middleware wraps; it records whether it ran.
func okHandler(ran *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*ran = true
		w.WriteHeader(http.StatusOK)
	})
}

func TestCORSReflectsAllowedOriginWithCredentials(t *testing.T) {
	var ran bool
	h := middleware.CORS([]string{"http://localhost:3002"})(okHandler(&ran))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.Header.Set("Origin", "http://localhost:3002")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3002" {
		t.Errorf("Allow-Origin = %q, want the exact request origin", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Allow-Credentials = %q, want \"true\"", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want \"Origin\" (cache correctness for per-origin reflection)", got)
	}
	if !ran {
		t.Error("downstream handler should run for a non-preflight request")
	}
}

func TestCORSPreflightShortCircuitsWith204(t *testing.T) {
	var ran bool
	h := middleware.CORS([]string{"http://localhost:3002"})(okHandler(&ran))

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	req.Header.Set("Origin", "http://localhost:3002")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "content-type")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d, want 204", rec.Code)
	}
	if ran {
		t.Error("preflight must NOT reach the downstream handler")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3002" {
		t.Errorf("preflight Allow-Origin = %q, want the request origin", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("preflight must advertise Allow-Methods")
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "content-type" {
		t.Errorf("preflight Allow-Headers = %q, want the requested headers echoed", got)
	}
}

func TestCORSDisallowedOriginGetsNoHeaders(t *testing.T) {
	var ran bool
	h := middleware.CORS([]string{"http://localhost:3002"})(okHandler(&ran))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q, want empty for a disallowed origin", got)
	}
	if !ran {
		t.Error("a non-preflight request still reaches downstream; the browser, not the gateway, enforces the block")
	}
}

func TestCORSNoOriginPassesThrough(t *testing.T) {
	var ran bool
	h := middleware.CORS([]string{"http://localhost:3002"})(okHandler(&ran))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q, want empty when no Origin header is present", got)
	}
	if !ran {
		t.Error("same-origin/no-Origin requests must pass through untouched")
	}
}

// A cross-origin fetch only sees the safelisted response headers unless the
// server exposes more. The web reads X-Confirm-Required on the teacher-access
// 409 to ask before reusing an account, so it must be exposed.
func TestCORSExposesTheConfirmRequiredHeader(t *testing.T) {
	var ran bool
	h := middleware.CORS([]string{"http://localhost:3002"})(okHandler(&ran))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/teachers/tch-1/access", nil)
	req.Header.Set("Origin", "http://localhost:3002")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Expose-Headers"); !strings.Contains(got, "X-Confirm-Required") {
		t.Errorf("Expose-Headers = %q, want it to include X-Confirm-Required", got)
	}
}

// T4: the web picks the scan's sound and colour from X-Scan-Result (two
// refusals share 422), so a cross-origin fetch must be able to read it.
func TestCORSExposesTheScanResultHeader(t *testing.T) {
	var ran bool
	h := middleware.CORS([]string{"http://localhost:3002"})(okHandler(&ran))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/attendance/scan", nil)
	req.Header.Set("Origin", "http://localhost:3002")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Expose-Headers"); !strings.Contains(got, "X-Scan-Result") {
		t.Errorf("Expose-Headers = %q, want it to include X-Scan-Result", got)
	}
}

// T6: the monthly grade screen reacts to a refusal's machine code (mark the
// cell, lock the row) from X-Error-Code, so a cross-origin fetch must read it.
func TestCORSExposesTheErrorCodeHeader(t *testing.T) {
	var ran bool
	h := middleware.CORS([]string{"http://localhost:3002"})(okHandler(&ran))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/grades/monthly", nil)
	req.Header.Set("Origin", "http://localhost:3002")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Expose-Headers"); !strings.Contains(got, "X-Error-Code") {
		t.Errorf("Expose-Headers = %q, want it to include X-Error-Code", got)
	}
}

// The web names the downloaded report-card PDF from Content-Disposition, which
// is not CORS-safelisted: without exposing it, a cross-origin read sees null.
func TestCORSExposesContentDisposition(t *testing.T) {
	var ran bool
	h := middleware.CORS([]string{"http://localhost:3002"})(okHandler(&ran))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/grades/report-cards/x/pdf", nil)
	req.Header.Set("Origin", "http://localhost:3002")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Expose-Headers"); !strings.Contains(got, "Content-Disposition") {
		t.Errorf("Expose-Headers = %q, want it to include Content-Disposition", got)
	}
}
