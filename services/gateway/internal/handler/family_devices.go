package handler

import (
	"context"
	"net/http"
	"regexp"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// FamilyDeviceRegistry is the comms surface the family app's push registration
// needs. The Connect CommsClient satisfies it.
type FamilyDeviceRegistry interface {
	RegisterDevice(ctx context.Context, userID, token, platform string) (client.Device, error)
	UnregisterDevice(ctx context.Context, userID, token string) error
}

// expoPushTokenPattern is the shape of the token the Expo family app obtains.
// Any other string could never be delivered to (comms' Expo sender skips it),
// so it is refused at the boundary rather than stored.
var expoPushTokenPattern = regexp.MustCompile(`^(ExponentPushToken|ExpoPushToken)\[[A-Za-z0-9_-]{1,200}\]$`)

// The platforms a family phone reports.
const (
	devicePlatformIOS     = "ios"
	devicePlatformAndroid = "android"
)

// msgInvalidPushToken / msgInvalidPlatform are the 400 sentences.
const (
	msgInvalidPushToken = "token must be an Expo push token"
	msgInvalidPlatform  = "platform must be ios or android"
)

// FamilyDevicesHandler registers the family app's push token (T5). The guardian
// role holds family:read only — NOT comms:write — so the admin
// /comms/devices route is unreachable to it; this is its own door.
//
// Self-scoped by construction: the device owner is ALWAYS the authenticated
// principal, never a request field, so nobody can route another family's pushes
// to their phone. Re-registering a token that belonged to someone else (a phone
// handed over) MOVES it to the caller — comms' upsert rewrites the owner — so
// the previous owner stops receiving on it; a logout removes it only for its
// owner.
type FamilyDevicesHandler struct {
	devices FamilyDeviceRegistry
}

// NewFamilyDevicesHandler wires the handler to comms.
func NewFamilyDevicesHandler(devices FamilyDeviceRegistry) *FamilyDevicesHandler {
	return &FamilyDevicesHandler{devices: devices}
}

// familyDeviceRequest is the body of POST /api/v1/family/devices. There is no
// user field on purpose (see FamilyDevicesHandler).
type familyDeviceRequest struct {
	Token    string `json:"token"`
	Platform string `json:"platform"`
}

// Register handles POST /api/v1/family/devices → 204. The app calls it on every
// launch; it is idempotent (upsert by token).
func (h *FamilyDevicesHandler) Register(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req familyDeviceRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if !expoPushTokenPattern.MatchString(req.Token) {
		writeBadRequest(w, msgInvalidPushToken)
		return
	}
	if req.Platform != devicePlatformIOS && req.Platform != devicePlatformAndroid {
		writeBadRequest(w, msgInvalidPlatform)
		return
	}
	if _, err := h.devices.RegisterDevice(r.Context(), p.UserID, req.Token, req.Platform); err != nil {
		writeConnectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Unregister handles DELETE /api/v1/family/devices/{token} → 204, on logout. It
// removes the token only when the CALLER owns it; anyone else's token is left
// alone with the same 204, so the call reveals nothing and is idempotent.
func (h *FamilyDevicesHandler) Unregister(w http.ResponseWriter, r *http.Request) {
	p, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	token := r.PathValue("token")
	if !expoPushTokenPattern.MatchString(token) {
		writeBadRequest(w, msgInvalidPushToken)
		return
	}
	if err := h.devices.UnregisterDevice(r.Context(), p.UserID, token); err != nil {
		writeConnectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
