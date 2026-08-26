package handler

import (
	"context"
	"errors"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	commsv1 "github.com/garcia-lorca/gen/go/garcialorca/comms/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/comms/v1/commsv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/comms/internal/service"
)

// Compile-time proof the handler implements the generated Connect service.
var _ commsv1connect.CommsServiceHandler = (*CommsHandler)(nil)

// statusServing is what a healthy service reports (the fleet-wide value).
const statusServing = "SERVING"

// CommsHandler adapts the comms service to the generated Connect service. It only
// translates: proto messages ↔ service calls and error Kinds ↔ Connect codes.
// Validation and logic live in the service layer.
//
// It carries the tenant purely to stamp it on outgoing messages: the contract
// declares tenant_id on Notification and Device, while the service types do not —
// comms is bound to ONE tenant for this phase, the same one its repository binds
// for RLS (see ADR 0001). It is never read from a request.
type CommsHandler struct {
	svc      *service.CommsService
	tenantID string
}

// NewCommsHandler wires the handler to the comms service for tenantID, which must
// be the tenant the service and its repository were built with.
func NewCommsHandler(svc *service.CommsService, tenantID string) *CommsHandler {
	return &CommsHandler{svc: svc, tenantID: tenantID}
}

// Health is the liveness/ping probe present on every service. Comms is a thin
// service with no readiness dependencies of its own, so liveness is answered
// here; when it grows any, the check moves into the service layer without
// changing this seam.
func (h *CommsHandler) Health(_ context.Context, _ *connect.Request[commsv1.HealthRequest]) (*connect.Response[commsv1.HealthResponse], error) {
	return connect.NewResponse(&commsv1.HealthResponse{Status: statusServing}), nil
}

// RegisterDevice registers (or refreshes) the FCM token a user's device is
// reachable at. The response carries the STORED device, whose id may differ from
// any the client sent: re-registering a known token updates that row.
func (h *CommsHandler) RegisterDevice(ctx context.Context, req *connect.Request[commsv1.RegisterDeviceRequest]) (*connect.Response[commsv1.RegisterDeviceResponse], error) {
	m := req.Msg
	device, err := h.svc.RegisterDevice(ctx, m.GetUserId(), m.GetFcmToken(), m.GetPlatform())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&commsv1.RegisterDeviceResponse{Device: h.device(device)}), nil
}

// ListNotifications returns one student's notifications, newest first; with
// unread_only, just the unread ones.
func (h *CommsHandler) ListNotifications(ctx context.Context, req *connect.Request[commsv1.ListNotificationsRequest]) (*connect.Response[commsv1.ListNotificationsResponse], error) {
	m := req.Msg
	notifications, err := h.svc.ListNotifications(ctx, m.GetStudentId(), m.GetUnreadOnly())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*commsv1.Notification, 0, len(notifications))
	for _, n := range notifications {
		out = append(out, h.notification(n))
	}
	return connect.NewResponse(&commsv1.ListNotificationsResponse{Notifications: out}), nil
}

// MarkNotificationRead stamps a notification as read and returns it. Marking an
// already-read notification is idempotent: read_at keeps its original instant.
func (h *CommsHandler) MarkNotificationRead(ctx context.Context, req *connect.Request[commsv1.MarkNotificationReadRequest]) (*connect.Response[commsv1.MarkNotificationReadResponse], error) {
	n, err := h.svc.MarkNotificationRead(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&commsv1.MarkNotificationReadResponse{Notification: h.notification(n)}), nil
}

// notification maps a stored notification onto the wire.
func (h *CommsHandler) notification(n service.Notification) *commsv1.Notification {
	return &commsv1.Notification{
		Id:        n.ID,
		TenantId:  h.tenantID,
		StudentId: n.StudentID,
		Type:      n.Type,
		Title:     n.Title,
		Body:      n.Body,
		CreatedAt: timestamp(n.CreatedAt),
		ReadAt:    timestamp(n.ReadAt),
	}
}

// device maps a stored push target onto the wire.
func (h *CommsHandler) device(d service.Device) *commsv1.Device {
	return &commsv1.Device{
		Id:       d.ID,
		TenantId: h.tenantID,
		UserId:   d.UserID,
		FcmToken: d.FCMToken,
		Platform: d.Platform,
	}
}

// timestamp maps an instant onto the wire, leaving an UNSET field for the zero
// value. That distinction is the contract: read_at's presence is what tells a
// client the notification is read, so serialising a zero time as an epoch
// timestamp would mark the whole feed read.
func timestamp(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

// connectErr maps a service error Kind to the Connect status code. Internal
// failures return a generic message (no internal detail leaks to the client);
// the other Kinds carry already-generic messages.
// Every code goes through glerrors.SafeMessage, not just the internal one. The
// kind decides the STATUS; it does not decide whether the wrapped cause is the
// caller's business, and it never is. Error() concatenates the cause, so handing
// the error straight to connect.NewError shipped Postgres constraint names and
// SQLSTATEs — and on some constraints the conflicting value — to whoever called.
func connectErr(err error) *connect.Error {
	safe := errors.New(glerrors.SafeMessage(err))

	switch glerrors.KindOf(err) {
	case glerrors.KindInvalid:
		return connect.NewError(connect.CodeInvalidArgument, safe)
	case glerrors.KindUnauthorized:
		return connect.NewError(connect.CodeUnauthenticated, safe)
	case glerrors.KindForbidden:
		return connect.NewError(connect.CodePermissionDenied, safe)
	case glerrors.KindNotFound:
		return connect.NewError(connect.CodeNotFound, safe)
	case glerrors.KindConflict:
		return connect.NewError(connect.CodeAlreadyExists, safe)
	default:
		return connect.NewError(connect.CodeInternal, glerrors.New(glerrors.KindInternal, "internal error"))
	}
}
