package handler_test

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	commsv1 "github.com/garcia-lorca/gen/go/garcialorca/comms/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/comms/internal/handler"
	"github.com/garcia-lorca/services/comms/internal/service"
)

// --- fake repository (the service's storage seam) -----------------------------

// fakeRepo is an in-memory service.Repo. The handler tests drive the REAL
// service, so the boundary validation they assert on is the shipped one.
type fakeRepo struct {
	notifications []service.Notification
	devices       []service.Device
	failWith      error
}

func (f *fakeRepo) RecordEvent(_ context.Context, _ string, n service.Notification) (service.Notification, bool, error) {
	if f.failWith != nil {
		return service.Notification{}, false, f.failWith
	}
	f.notifications = append(f.notifications, n)
	return n, true, nil
}

func (f *fakeRepo) ListNotifications(_ context.Context, studentID string, unreadOnly bool) ([]service.Notification, error) {
	if f.failWith != nil {
		return nil, f.failWith
	}
	var out []service.Notification
	for _, n := range f.notifications {
		if n.StudentID != studentID {
			continue
		}
		if unreadOnly && !n.ReadAt.IsZero() {
			continue
		}
		out = append(out, n)
	}
	return out, nil
}

func (f *fakeRepo) MarkRead(_ context.Context, id string) (service.Notification, error) {
	if f.failWith != nil {
		return service.Notification{}, f.failWith
	}
	for i, n := range f.notifications {
		if n.ID != id {
			continue
		}
		if n.ReadAt.IsZero() {
			f.notifications[i].ReadAt = readInstant
		}
		return f.notifications[i], nil
	}
	return service.Notification{}, glerrors.ErrNotFound
}

func (f *fakeRepo) UpsertDevice(_ context.Context, d service.Device) (service.Device, error) {
	if f.failWith != nil {
		return service.Device{}, f.failWith
	}
	f.devices = append(f.devices, d)
	return d, nil
}

func (f *fakeRepo) DevicesForUsers(_ context.Context, userIDs []string) ([]service.Device, error) {
	if f.failWith != nil {
		return nil, f.failWith
	}
	out := make([]service.Device, 0, len(userIDs))
	for _, d := range f.devices {
		if slices.Contains(userIDs, d.UserID) {
			out = append(out, d)
		}
	}
	return out, nil
}

// --- fixtures -----------------------------------------------------------------

const (
	testTenant = "00000000-0000-0000-0000-0000000000aa"
	studentID  = "3f2504e0-4f89-41d3-9a0c-0305e82c3301"
	userID     = "3f2504e0-4f89-41d3-9a0c-0305e82c3302"
	deviceID   = "3f2504e0-4f89-41d3-9a0c-0305e82c3303"
	unreadID   = "3f2504e0-4f89-41d3-9a0c-0305e82c3304"
	readID     = "3f2504e0-4f89-41d3-9a0c-0305e82c3305"
)

var (
	createdInstant = time.Date(2026, 9, 15, 8, 30, 0, 0, time.UTC)
	readInstant    = time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC)
)

func newHandler(repo *fakeRepo) *handler.CommsHandler {
	svc := service.NewCommsService(
		// No guardian reader and no sender: these tests exercise the read/write
		// RPCs, and push is a side effect of CONSUMING an event, never of a call.
		repo, nil, nil, testTenant,
		func() string { return deviceID },
		func() time.Time { return createdInstant },
		slog.New(slog.DiscardHandler),
	)
	return handler.NewCommsHandler(svc, testTenant)
}

func unreadNotification() service.Notification {
	return service.Notification{
		ID:        unreadID,
		StudentID: studentID,
		Type:      "absence",
		Title:     "Ausencia registrada",
		Body:      "Se registró una ausencia el 2026-09-15.",
		CreatedAt: createdInstant,
	}
}

// --- tests --------------------------------------------------------------------

func TestHealth(t *testing.T) {
	res, err := newHandler(&fakeRepo{}).Health(context.Background(), connect.NewRequest(&commsv1.HealthRequest{}))
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if res.Msg.GetStatus() != "SERVING" {
		t.Errorf("status = %q, want SERVING", res.Msg.GetStatus())
	}
}

func TestRegisterDeviceReturnsStoredDevice(t *testing.T) {
	h := newHandler(&fakeRepo{})

	res, err := h.RegisterDevice(context.Background(), connect.NewRequest(&commsv1.RegisterDeviceRequest{
		UserId: userID, FcmToken: "tok-abcdef", Platform: "ios",
	}))
	if err != nil {
		t.Fatalf("RegisterDevice: %v", err)
	}
	d := res.Msg.GetDevice()
	if d.GetId() != deviceID || d.GetUserId() != userID || d.GetFcmToken() != "tok-abcdef" || d.GetPlatform() != "ios" {
		t.Errorf("device = %+v", d)
	}
	if d.GetTenantId() != testTenant {
		t.Errorf("tenant_id = %q, want %q", d.GetTenantId(), testTenant)
	}
}

func TestRegisterDeviceUnknownPlatformIsInvalidArgument(t *testing.T) {
	h := newHandler(&fakeRepo{})
	_, err := h.RegisterDevice(context.Background(), connect.NewRequest(&commsv1.RegisterDeviceRequest{
		UserId: userID, FcmToken: "tok-abcdef", Platform: "symbian",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("want InvalidArgument, got %v", connect.CodeOf(err))
	}
}

// An unread notification must leave read_at UNSET on the wire: the client reads
// its presence as "read", so a zero instant serialised as an epoch timestamp
// would mark every notification read.
func TestListNotificationsLeavesUnreadWithoutReadAt(t *testing.T) {
	repo := &fakeRepo{notifications: []service.Notification{unreadNotification()}}
	h := newHandler(repo)

	res, err := h.ListNotifications(context.Background(), connect.NewRequest(&commsv1.ListNotificationsRequest{
		StudentId: studentID,
	}))
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	got := res.Msg.GetNotifications()
	if len(got) != 1 {
		t.Fatalf("want 1 notification, got %d", len(got))
	}
	n := got[0]
	if n.GetId() != unreadID || n.GetStudentId() != studentID || n.GetType() != "absence" {
		t.Errorf("notification = %+v", n)
	}
	if n.GetTenantId() != testTenant {
		t.Errorf("tenant_id = %q, want %q", n.GetTenantId(), testTenant)
	}
	if !n.GetCreatedAt().AsTime().Equal(createdInstant) {
		t.Errorf("created_at = %v, want %v", n.GetCreatedAt().AsTime(), createdInstant)
	}
	if n.GetReadAt() != nil {
		t.Errorf("read_at = %v, want unset", n.GetReadAt().AsTime())
	}
}

func TestListNotificationsUnreadOnlyFiltersRead(t *testing.T) {
	read := unreadNotification()
	read.ID = readID
	read.ReadAt = readInstant
	repo := &fakeRepo{notifications: []service.Notification{unreadNotification(), read}}
	h := newHandler(repo)

	res, err := h.ListNotifications(context.Background(), connect.NewRequest(&commsv1.ListNotificationsRequest{
		StudentId: studentID, UnreadOnly: true,
	}))
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	if got := res.Msg.GetNotifications(); len(got) != 1 || got[0].GetId() != unreadID {
		t.Errorf("unread_only returned %+v", got)
	}
}

func TestListNotificationsMalformedStudentIDIsInvalidArgument(t *testing.T) {
	h := newHandler(&fakeRepo{})
	_, err := h.ListNotifications(context.Background(), connect.NewRequest(&commsv1.ListNotificationsRequest{
		StudentId: "abc",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("want InvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestMarkNotificationReadStampsReadAt(t *testing.T) {
	repo := &fakeRepo{notifications: []service.Notification{unreadNotification()}}
	h := newHandler(repo)

	res, err := h.MarkNotificationRead(context.Background(), connect.NewRequest(&commsv1.MarkNotificationReadRequest{
		Id: unreadID,
	}))
	if err != nil {
		t.Fatalf("MarkNotificationRead: %v", err)
	}
	n := res.Msg.GetNotification()
	if n.GetReadAt() == nil {
		t.Fatalf("read_at unset after marking read")
	}
	if !n.GetReadAt().AsTime().Equal(readInstant) {
		t.Errorf("read_at = %v, want %v", n.GetReadAt().AsTime(), readInstant)
	}
}

func TestMarkNotificationReadUnknownIDIsNotFound(t *testing.T) {
	h := newHandler(&fakeRepo{})
	_, err := h.MarkNotificationRead(context.Background(), connect.NewRequest(&commsv1.MarkNotificationReadRequest{
		Id: unreadID,
	}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("want NotFound, got %v", connect.CodeOf(err))
	}
}

// A storage failure is Internal and its detail never reaches the client: the
// wire message is the generic one, not the wrapped cause.
func TestStorageFailureIsInternalWithoutDetail(t *testing.T) {
	repo := &fakeRepo{failWith: glerrors.Wrap(glerrors.KindInternal, "listing notifications", errors.New("dial tcp: connection refused"))}
	h := newHandler(repo)

	_, err := h.ListNotifications(context.Background(), connect.NewRequest(&commsv1.ListNotificationsRequest{
		StudentId: studentID,
	}))
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Fatalf("want Internal, got %v", connect.CodeOf(err))
	}
	if strings.Contains(err.Error(), "connection refused") {
		t.Errorf("internal detail leaked to the client: %q", err.Error())
	}
}
