package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

func servedBy(t *testing.T, mw func(http.Handler) http.Handler, perms []string) *httptest.ResponseRecorder {
	t.Helper()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uniforms/counter-sale", nil)
	if perms != nil {
		req = req.WithContext(middleware.WithPrincipal(req.Context(), middleware.Principal{
			UserID: "usr-1", Permissions: perms,
		}))
	}
	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req)
	return rec
}

// TestRequireAllPermissionsNamesTheMissingOne is the whole reason this exists
// beside RequirePermission. The counter sale needs uniforms:write AND
// finance:write, because it moves goods and takes money. A cashier holding only
// the first would otherwise key in six lines and eat a bare 403 — the panel needs
// to disable the entry point and SAY WHY, and it can only do that if the refusal
// names the permission that is missing.
func TestRequireAllPermissionsNamesTheMissingOne(t *testing.T) {
	mw := middleware.RequireAllPermissions("uniforms:write", "finance:write")

	rec := servedBy(t, mw, []string{"uniforms:write"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}

	var body struct {
		Error struct {
			Code    string            `json:"code"`
			Message string            `json:"message"`
			Details map[string]string `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the refusal is not the JSON envelope: %v (%s)", err, rec.Body.String())
	}
	if body.Error.Code != "forbidden" {
		t.Errorf("code = %q, want forbidden", body.Error.Code)
	}
	if body.Error.Details["missingPermission"] != "finance:write" {
		t.Errorf("details = %v; the refusal must name what is missing", body.Error.Details)
	}
	if body.Error.Message == "" {
		t.Error("a refusal with no message tells the cashier nothing")
	}
}

// TestRequireAllPermissionsNamesTheFirstMissingOne: order is the caller's, so a
// principal missing both is told about the first one listed. Naming all of them
// would be worse — the panel shows one reason, and the cashier fixes one thing.
func TestRequireAllPermissionsNamesTheFirstMissingOne(t *testing.T) {
	mw := middleware.RequireAllPermissions("uniforms:write", "finance:write")
	rec := servedBy(t, mw, []string{"students:read"})

	var body struct {
		Error struct {
			Details map[string]string `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Error.Details["missingPermission"] != "uniforms:write" {
		t.Errorf("missing = %q, want the first one listed", body.Error.Details["missingPermission"])
	}
}

func TestRequireAllPermissionsLetsAFullyEntitledCallerThrough(t *testing.T) {
	mw := middleware.RequireAllPermissions("uniforms:write", "finance:write")
	rec := servedBy(t, mw, []string{"uniforms:write", "finance:write", "students:read"})
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
}

// TestRequireAllPermissionsFailsClosed: no principal at all is 401, never 403.
// The difference matters to the client — one means "log in", the other means
// "ask your director for access".
func TestRequireAllPermissionsFailsClosed(t *testing.T) {
	mw := middleware.RequireAllPermissions("uniforms:write")
	rec := servedBy(t, mw, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// TestRequireAnyPermissionIsForReadsWithTwoDoors: the orders panel is reachable
// by a director (uniforms:orders) and, later, by a family surface. Requiring both
// would lock out everyone who legitimately holds one.
func TestRequireAnyPermission(t *testing.T) {
	mw := middleware.RequireAnyPermission("uniforms:orders", "uniforms_own:read")

	if rec := servedBy(t, mw, []string{"uniforms_own:read"}); rec.Code != http.StatusOK {
		t.Errorf("holder of the second permission got %d, want 200", rec.Code)
	}
	if rec := servedBy(t, mw, []string{"uniforms:orders"}); rec.Code != http.StatusOK {
		t.Errorf("holder of the first permission got %d, want 200", rec.Code)
	}
	if rec := servedBy(t, mw, []string{"students:read"}); rec.Code != http.StatusForbidden {
		t.Errorf("holder of neither got %d, want 403", rec.Code)
	}
}
