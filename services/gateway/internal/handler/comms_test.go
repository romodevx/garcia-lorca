package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	connect "connectrpc.com/connect"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// fakeComms is a hand-written double for the comms surface the handler needs. It
// records the last input and returns canned results/errors so each handler is
// exercised in isolation (no Connect round-trip).
type fakeComms struct {
	deviceUserID   string
	deviceToken    string
	devicePlatform string
	device         client.Device

	listStudentID  string
	listUnreadOnly bool
	notifications  []client.Notification

	markID       string
	notification client.Notification

	err error
}

func (f *fakeComms) RegisterDevice(_ context.Context, userID, fcmToken, platform string) (client.Device, error) {
	f.deviceUserID, f.deviceToken, f.devicePlatform = userID, fcmToken, platform
	if f.err != nil {
		return client.Device{}, f.err
	}
	return f.device, nil
}

func (f *fakeComms) ListNotifications(_ context.Context, studentID string, unreadOnly bool) ([]client.Notification, error) {
	f.listStudentID, f.listUnreadOnly = studentID, unreadOnly
	return f.notifications, f.err
}

func (f *fakeComms) MarkNotificationRead(_ context.Context, id string) (client.Notification, error) {
	f.markID = id
	if f.err != nil {
		return client.Notification{}, f.err
	}
	return f.notification, nil
}

// --- register a device -----------------------------------------------------

// callerRequest builds a request carrying the principal RequireAuth injects in
// production, so handlers that derive identity from the caller (never from the
// body) are exercised the way they actually run.
func callerRequest(method, target, body, userID string) *http.Request {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	return r.WithContext(middleware.WithPrincipal(r.Context(), middleware.Principal{
		UserID: userID, TenantID: "t1", Email: "mama@example.com", Permissions: []string{"comms:write"},
	}))
}

func doCallerReq(t *testing.T, hf http.HandlerFunc, method, target, body, userID string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	hf(rec, callerRequest(method, target, body, userID))
	return rec
}

func TestCommsRegisterDeviceReturnsDevice(t *testing.T) {
	svc := &fakeComms{device: client.Device{
		ID: "dev-1", TenantID: "t1", UserID: "usr-1", FcmToken: "tok-abc", Platform: "android",
	}}
	h := handler.NewCommsHandler(svc)

	rec := doCallerReq(t, h.RegisterDevice, http.MethodPost, "/api/v1/comms/devices",
		`{"fcmToken":"tok-abc","platform":"android"}`, "usr-1")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.deviceUserID != "usr-1" || svc.deviceToken != "tok-abc" || svc.devicePlatform != "android" {
		t.Errorf("forwarded input = %q %q %q", svc.deviceUserID, svc.deviceToken, svc.devicePlatform)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if out["id"] != "dev-1" || out["userId"] != "usr-1" ||
		out["fcmToken"] != "tok-abc" || out["platform"] != "android" {
		t.Errorf("body = %v", out)
	}
}

// A device is ALWAYS registered for the authenticated caller. A userId in the
// body is ignored, otherwise any comms:write holder could point another
// family's push notifications at their own phone.
func TestCommsRegisterDeviceIgnoresBodySuppliedUserID(t *testing.T) {
	svc := &fakeComms{device: client.Device{ID: "dev-1", UserID: "usr-1", FcmToken: "tok-abc"}}
	h := handler.NewCommsHandler(svc)

	rec := doCallerReq(t, h.RegisterDevice, http.MethodPost, "/api/v1/comms/devices",
		`{"userId":"victim-usr","fcmToken":"tok-abc","platform":"android"}`, "usr-1")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.deviceUserID != "usr-1" {
		t.Errorf("device owner = %q, want the principal's usr-1 (the body must not decide it)", svc.deviceUserID)
	}
}

func TestCommsRegisterDeviceWithoutPrincipalIs401(t *testing.T) {
	svc := &fakeComms{}
	h := handler.NewCommsHandler(svc)
	rec := doReq(t, h.RegisterDevice, http.MethodPost, "/api/v1/comms/devices",
		`{"fcmToken":"tok-abc","platform":"android"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if svc.deviceUserID != "" {
		t.Errorf("comms was called with owner %q; an unauthenticated request must register nothing", svc.deviceUserID)
	}
}

func TestCommsRegisterDeviceInvalidBodyIs400(t *testing.T) {
	h := handler.NewCommsHandler(&fakeComms{})
	rec := doCallerReq(t, h.RegisterDevice, http.MethodPost, "/api/v1/comms/devices", `{`, "usr-1")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// An unknown platform is rejected upstream as InvalidArgument and must surface
// at the edge as 400, not 500. What comms itself rejects is pinned by its own
// service tests; this covers the mapping only.
func TestCommsRegisterDeviceUnknownPlatformIs400(t *testing.T) {
	svc := &fakeComms{err: connect.NewError(connect.CodeInvalidArgument, nil)}
	h := handler.NewCommsHandler(svc)
	rec := doCallerReq(t, h.RegisterDevice, http.MethodPost, "/api/v1/comms/devices",
		`{"fcmToken":"tok-abc","platform":"blackberry"}`, "usr-1")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// --- list notifications ----------------------------------------------------

func TestCommsListNotificationsForwardsFiltersAndReturnsRows(t *testing.T) {
	svc := &fakeComms{notifications: []client.Notification{
		{ID: "ntf-1", TenantID: "t1", StudentID: "stu-1", Type: "absence", Title: "Ausencia",
			Body: "Ana faltó hoy", CreatedAt: "2026-09-01T10:00:00Z"},
		{ID: "ntf-2", StudentID: "stu-1", Type: "payment", Title: "Pago recibido",
			CreatedAt: "2026-08-31T10:00:00Z", ReadAt: "2026-08-31T12:00:00Z"},
	}}
	h := handler.NewCommsHandler(svc)

	rec := doReq(t, h.ListNotifications, http.MethodGet,
		"/api/v1/comms/notifications?studentId=stu-1&unreadOnly=true", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.listStudentID != "stu-1" || !svc.listUnreadOnly {
		t.Errorf("forwarded filters = %q %v", svc.listStudentID, svc.listUnreadOnly)
	}
	var out struct {
		Notifications []map[string]any `json:"notifications"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(out.Notifications) != 2 || out.Notifications[0]["id"] != "ntf-1" ||
		out.Notifications[0]["type"] != "absence" || out.Notifications[0]["title"] != "Ausencia" {
		t.Fatalf("body = %v", out.Notifications)
	}
	// An unread notification carries an empty readAt, never a bogus instant.
	if out.Notifications[0]["readAt"] != "" || out.Notifications[1]["readAt"] != "2026-08-31T12:00:00Z" {
		t.Errorf("readAt = %v / %v", out.Notifications[0]["readAt"], out.Notifications[1]["readAt"])
	}
}

// unreadOnly is a flag, not a tri-state: anything other than "true" (absent,
// empty, "1", "yes") means "give me everything".
func TestCommsListNotificationsUnreadOnlyOnlyTrueEnablesFilter(t *testing.T) {
	for _, q := range []string{"", "&unreadOnly=", "&unreadOnly=false", "&unreadOnly=1"} {
		svc := &fakeComms{}
		h := handler.NewCommsHandler(svc)
		rec := doReq(t, h.ListNotifications, http.MethodGet,
			"/api/v1/comms/notifications?studentId=stu-1"+q, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("query %q: status = %d, want 200", q, rec.Code)
		}
		if svc.listUnreadOnly {
			t.Errorf("query %q: unreadOnly = true, want false", q)
		}
	}
}

func TestCommsListNotificationsUnknownStudentIs404(t *testing.T) {
	svc := &fakeComms{err: connect.NewError(connect.CodeNotFound, nil)}
	h := handler.NewCommsHandler(svc)
	rec := doReq(t, h.ListNotifications, http.MethodGet, "/api/v1/comms/notifications?studentId=ghost", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestCommsListNotificationsUpstreamFailureIs500(t *testing.T) {
	svc := &fakeComms{err: connect.NewError(connect.CodeInternal, nil)}
	h := handler.NewCommsHandler(svc)
	rec := doReq(t, h.ListNotifications, http.MethodGet, "/api/v1/comms/notifications?studentId=stu-1", "")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

// --- mark read -------------------------------------------------------------

// The id travels in the path, so the handler must read it from the mux path
// value (a directly-invoked handler gets none unless the test binds it).
func TestCommsMarkReadUsesPathValueAndReturnsNotification(t *testing.T) {
	svc := &fakeComms{notification: client.Notification{
		ID: "ntf-1", StudentID: "stu-1", Type: "absence", Title: "Ausencia",
		CreatedAt: "2026-09-01T10:00:00Z", ReadAt: "2026-09-01T18:00:00Z",
	}}
	h := handler.NewCommsHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/comms/notifications/ntf-1/read", strings.NewReader(""))
	req.SetPathValue("id", "ntf-1")
	rec := httptest.NewRecorder()
	h.MarkRead(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if svc.markID != "ntf-1" {
		t.Errorf("forwarded id = %q", svc.markID)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if out["id"] != "ntf-1" || out["readAt"] != "2026-09-01T18:00:00Z" {
		t.Errorf("body = %v", out)
	}
}

func TestCommsMarkReadUnknownNotificationIs404(t *testing.T) {
	svc := &fakeComms{err: connect.NewError(connect.CodeNotFound, nil)}
	h := handler.NewCommsHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/comms/notifications/ghost/read", nil)
	req.SetPathValue("id", "ghost")
	rec := httptest.NewRecorder()
	h.MarkRead(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
