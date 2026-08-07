package client

import (
	"context"
	"net/http"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	commsv1 "github.com/garcia-lorca/gen/go/garcialorca/comms/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/comms/v1/commsv1connect"
)

// CommsClient is the gateway's typed client for the comms service over the
// generated Connect client. Handlers depend on its small methods and the plain
// structs below, never on the generated proto types.
//
// Comms PUBLISHES nothing: notifications are materialised by consuming other
// services' domain events, so the edge surface is deliberately small —
// registering a push token and reading/acknowledging what is already stored.
type CommsClient struct {
	rpc commsv1connect.CommsServiceClient
}

// Notification is the edge's view of one notification materialised for a
// student. Type names the event that produced it (e.g. "absence", "payment",
// "debt_overdue"). CreatedAt and ReadAt are rendered RFC3339 so the edge never
// leaks a proto timestamp; ReadAt is EMPTY while the notification is unread.
type Notification struct {
	ID        string
	TenantID  string
	StudentID string
	Type      string
	Title     string
	Body      string
	CreatedAt string
	ReadAt    string
}

// Device is one push target for a user: the FCM token plus the platform it came
// from. Platform is one of ""|ios|android|web (empty = unknown client).
type Device struct {
	ID       string
	TenantID string
	UserID   string
	FcmToken string
	Platform string
}

// NewCommsClient builds a CommsClient targeting the comms base URL over
// httpClient.
func NewCommsClient(baseURL string, httpClient connect.HTTPClient) *CommsClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &CommsClient{rpc: commsv1connect.NewCommsServiceClient(httpClient, baseURL)}
}

// RegisterDevice registers (or refreshes) the FCM token a user's device is
// reachable at. Re-registering the same token upserts the existing device, so a
// client may call this on every launch.
func (c *CommsClient) RegisterDevice(ctx context.Context, userID, fcmToken, platform string) (Device, error) {
	res, err := c.rpc.RegisterDevice(ctx, connect.NewRequest(&commsv1.RegisterDeviceRequest{
		UserId:   userID,
		FcmToken: fcmToken,
		Platform: platform,
	}))
	if err != nil {
		return Device{}, err
	}
	return toDevice(res.Msg.GetDevice()), nil
}

// ListNotifications returns the notifications materialised for one student,
// newest first. With unreadOnly, only notifications that have not been read yet
// come back.
func (c *CommsClient) ListNotifications(ctx context.Context, studentID string, unreadOnly bool) ([]Notification, error) {
	res, err := c.rpc.ListNotifications(ctx, connect.NewRequest(&commsv1.ListNotificationsRequest{
		StudentId:  studentID,
		UnreadOnly: unreadOnly,
	}))
	if err != nil {
		return nil, err
	}
	out := make([]Notification, 0, len(res.Msg.GetNotifications()))
	for _, n := range res.Msg.GetNotifications() {
		out = append(out, toNotification(n))
	}
	return out, nil
}

// MarkNotificationRead stamps a notification as read and returns it. Marking an
// already-read notification is idempotent: ReadAt keeps its original value.
func (c *CommsClient) MarkNotificationRead(ctx context.Context, id string) (Notification, error) {
	res, err := c.rpc.MarkNotificationRead(ctx, connect.NewRequest(&commsv1.MarkNotificationReadRequest{
		Id: id,
	}))
	if err != nil {
		return Notification{}, err
	}
	return toNotification(res.Msg.GetNotification()), nil
}

// toNotification renders both timestamps as RFC3339. An unset timestamp yields
// "" rather than the zero-time string, so an unread notification never reports a
// bogus read instant.
func toNotification(n *commsv1.Notification) Notification {
	return Notification{
		ID:        n.GetId(),
		TenantID:  n.GetTenantId(),
		StudentID: n.GetStudentId(),
		Type:      n.GetType(),
		Title:     n.GetTitle(),
		Body:      n.GetBody(),
		CreatedAt: rfc3339OrEmpty(n.GetCreatedAt()),
		ReadAt:    rfc3339OrEmpty(n.GetReadAt()),
	}
}

// rfc3339OrEmpty renders a proto timestamp as RFC3339, or "" when it is unset.
func rfc3339OrEmpty(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return ""
	}
	return ts.AsTime().Format(time.RFC3339)
}

func toDevice(d *commsv1.Device) Device {
	return Device{
		ID:       d.GetId(),
		TenantID: d.GetTenantId(),
		UserID:   d.GetUserId(),
		FcmToken: d.GetFcmToken(),
		Platform: d.GetPlatform(),
	}
}
