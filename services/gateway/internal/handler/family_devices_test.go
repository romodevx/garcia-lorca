package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// fakeDeviceRegistry doubles comms' device registration: it records the owner
// each call was made for, which is the assertion that matters.
type fakeDeviceRegistry struct {
	registered   [][3]string // user, token, platform
	unregistered [][2]string // user, token
	err          error
}

func (f *fakeDeviceRegistry) RegisterDevice(_ context.Context, userID, token, platform string) (client.Device, error) {
	f.registered = append(f.registered, [3]string{userID, token, platform})
	return client.Device{ID: "dev-1", UserID: userID, FcmToken: token, Platform: platform}, f.err
}

func (f *fakeDeviceRegistry) UnregisterDevice(_ context.Context, userID, token string) error {
	f.unregistered = append(f.unregistered, [2]string{userID, token})
	return f.err
}

const expoToken = "ExponentPushToken[xxxxxxxxxxxxxxxxxxxxxx]"

func familyDeviceReq(method, target, userID, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if userID == "" {
		return req
	}
	return req.WithContext(middleware.WithPrincipal(req.Context(), middleware.Principal{
		UserID: userID, TenantID: "t1", Permissions: []string{"family:read"},
	}))
}

func TestFamilyRegisterDevice(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode int
		wantCall bool
	}{
		{name: "ios", body: `{"token":"` + expoToken + `","platform":"ios"}`, wantCode: http.StatusNoContent, wantCall: true},
		{name: "android, legacy prefix", body: `{"token":"ExpoPushToken[abc_DEF-1]","platform":"android"}`, wantCode: http.StatusNoContent, wantCall: true},
		{name: "not an Expo token", body: `{"token":"fcm-raw-token","platform":"ios"}`, wantCode: http.StatusBadRequest},
		{name: "empty token", body: `{"token":"","platform":"ios"}`, wantCode: http.StatusBadRequest},
		{name: "web is not a phone", body: `{"token":"` + expoToken + `","platform":"web"}`, wantCode: http.StatusBadRequest},
		{name: "missing platform", body: `{"token":"` + expoToken + `"}`, wantCode: http.StatusBadRequest},
		{name: "malformed JSON", body: `{`, wantCode: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &fakeDeviceRegistry{}
			rec := httptest.NewRecorder()
			handler.NewFamilyDevicesHandler(reg).Register(rec, familyDeviceReq(http.MethodPost, "/api/v1/family/devices", "usr-mama", tt.body))
			assert.Equal(t, tt.wantCode, rec.Code, rec.Body.String())
			if !tt.wantCall {
				assert.Empty(t, reg.registered)
				return
			}
			require.Len(t, reg.registered, 1)
			assert.Equal(t, "usr-mama", reg.registered[0][0], "the owner is the principal")
		})
	}
}

// A body naming another user is ignored: the owner is ALWAYS the principal, so
// nobody can route another family's pushes to their phone.
func TestFamilyRegisterDeviceOwnerIsThePrincipal(t *testing.T) {
	reg := &fakeDeviceRegistry{}
	rec := httptest.NewRecorder()
	handler.NewFamilyDevicesHandler(reg).Register(rec, familyDeviceReq(http.MethodPost, "/api/v1/family/devices", "usr-papa",
		`{"token":"`+expoToken+`","platform":"android","userId":"usr-someone-else"}`))
	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, [][3]string{{"usr-papa", expoToken, "android"}}, reg.registered)
}

func TestFamilyUnregisterDevice(t *testing.T) {
	reg := &fakeDeviceRegistry{}
	req := familyDeviceReq(http.MethodDelete, "/api/v1/family/devices/"+url.PathEscape(expoToken), "usr-mama", "")
	req.SetPathValue("token", expoToken)
	rec := httptest.NewRecorder()
	handler.NewFamilyDevicesHandler(reg).Unregister(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, [][2]string{{"usr-mama", expoToken}}, reg.unregistered, "only the caller's own token")

	bad := familyDeviceReq(http.MethodDelete, "/api/v1/family/devices/nope", "usr-mama", "")
	bad.SetPathValue("token", "nope")
	rec = httptest.NewRecorder()
	handler.NewFamilyDevicesHandler(reg).Unregister(rec, bad)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Len(t, reg.unregistered, 1)
}

func TestFamilyDevicesWithoutPrincipalAre401(t *testing.T) {
	reg := &fakeDeviceRegistry{}
	h := handler.NewFamilyDevicesHandler(reg)
	rec := httptest.NewRecorder()
	h.Register(rec, familyDeviceReq(http.MethodPost, "/api/v1/family/devices", "", `{"token":"`+expoToken+`","platform":"ios"}`))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	rec = httptest.NewRecorder()
	h.Unregister(rec, familyDeviceReq(http.MethodDelete, "/api/v1/family/devices/x", "", ""))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Empty(t, reg.registered)
	assert.Empty(t, reg.unregistered)
}

func TestFamilyRegisterDevicePropagatesCommsErrors(t *testing.T) {
	reg := &fakeDeviceRegistry{err: connect.NewError(connect.CodeUnavailable, errString("comms down"))}
	rec := httptest.NewRecorder()
	handler.NewFamilyDevicesHandler(reg).Register(rec, familyDeviceReq(http.MethodPost, "/api/v1/family/devices", "usr-mama", `{"token":"`+expoToken+`","platform":"ios"}`))
	assert.GreaterOrEqual(t, rec.Code, 500)
}
