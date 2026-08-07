package handler

import (
	"context"
	"net/http"

	"github.com/garcia-lorca/services/gateway/internal/client"
)

// CommsService is the communications surface the edge needs (consumer-defined,
// so the handler is testable with a fake). The Connect CommsClient implements it.
type CommsService interface {
	RegisterDevice(ctx context.Context, userID, fcmToken, platform string) (client.Device, error)
	ListNotifications(ctx context.Context, studentID string, unreadOnly bool) ([]client.Notification, error)
	MarkNotificationRead(ctx context.Context, id string) (client.Notification, error)
}

// CommsHandler exposes the front-facing (HTTP/JSON) comms endpoints, proxying to
// the comms service. It translates JSON ↔ the CommsClient and Connect error codes
// ↔ HTTP status. Every route must run behind the module guard (module_comms) plus
// RequireAuth + rotation-enforcement, with device registration and the read
// acknowledgement gated on comms:write and the notification list on comms:read.
//
// Comms sends nothing on request: notifications are materialised by consuming
// other services' domain events, so this surface only registers a push token and
// reads/acknowledges what is already stored.
type CommsHandler struct {
	svc CommsService
}

// NewCommsHandler wires the handler to a comms service.
func NewCommsHandler(svc CommsService) *CommsHandler {
	return &CommsHandler{svc: svc}
}

// unreadOnlyTrue is the only query value that turns the unread filter on;
// anything else (absent, empty, "1") means "give me everything".
const unreadOnlyTrue = "true"

// --- request bodies --------------------------------------------------------

// registerDeviceRequest carries the FCM token a user's device is reachable at.
// Platform is one of ""|ios|android|web; comms rejects anything else.
type registerDeviceRequest struct {
	UserID   string `json:"userId"`
	FcmToken string `json:"fcmToken"`
	Platform string `json:"platform"`
}

// --- response bodies -------------------------------------------------------

type deviceResponse struct {
	ID       string `json:"id"`
	TenantID string `json:"tenantId"`
	UserID   string `json:"userId"`
	FcmToken string `json:"fcmToken"`
	Platform string `json:"platform"`
}

func toDeviceResponse(d client.Device) deviceResponse {
	return deviceResponse{
		ID:       d.ID,
		TenantID: d.TenantID,
		UserID:   d.UserID,
		FcmToken: d.FcmToken,
		Platform: d.Platform,
	}
}

// notificationResponse renders one notification. ReadAt is empty while the
// notification is unread — the front distinguishes read state on that, so it is
// never omitted from the JSON.
type notificationResponse struct {
	ID        string `json:"id"`
	TenantID  string `json:"tenantId"`
	StudentID string `json:"studentId"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
	ReadAt    string `json:"readAt"`
}

func toNotificationResponse(n client.Notification) notificationResponse {
	return notificationResponse{
		ID:        n.ID,
		TenantID:  n.TenantID,
		StudentID: n.StudentID,
		Type:      n.Type,
		Title:     n.Title,
		Body:      n.Body,
		CreatedAt: n.CreatedAt,
		ReadAt:    n.ReadAt,
	}
}

// --- endpoints -------------------------------------------------------------

// RegisterDevice handles POST /api/v1/comms/devices — register (or refresh) the
// FCM token of the caller's device. Re-sending the same token upserts it, so the
// client may call this on every launch.
func (h *CommsHandler) RegisterDevice(w http.ResponseWriter, r *http.Request) {
	var req registerDeviceRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	device, err := h.svc.RegisterDevice(r.Context(), req.UserID, req.FcmToken, req.Platform)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toDeviceResponse(device))
}

// ListNotifications handles
// GET /api/v1/comms/notifications?studentId=...&unreadOnly=true — one student's
// notifications, newest first.
func (h *CommsHandler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := h.svc.ListNotifications(r.Context(), q.Get("studentId"), q.Get("unreadOnly") == unreadOnlyTrue)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	out := make([]notificationResponse, 0, len(res))
	for _, n := range res {
		out = append(out, toNotificationResponse(n))
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": out})
}

// MarkRead handles POST /api/v1/comms/notifications/{id}/read — acknowledge one
// notification. It is idempotent: re-marking keeps the original read instant.
func (h *CommsHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.MarkNotificationRead(r.Context(), r.PathValue("id"))
	if err != nil {
		writeConnectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toNotificationResponse(res))
}
