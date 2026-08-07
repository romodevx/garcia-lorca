package service_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	attendancev1 "github.com/garcia-lorca/gen/go/garcialorca/attendance/v1"
	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/pubsub"
	"github.com/garcia-lorca/services/comms/internal/service"
)

const (
	tenantID      = "11111111-1111-1111-1111-111111111111"
	otherTenantID = "22222222-2222-2222-2222-222222222222"
	studentID     = "aaaaaaaa-0000-0000-0000-000000000001"
	userID        = "bbbbbbbb-0000-0000-0000-000000000001"

	// Ids of already-materialised notifications, in the uuid form the column
	// holds: the service validates the shape before the id reaches Postgres.
	notifOneID     = "9a9a9a9a-0000-0000-0000-000000000001"
	notifTwoID     = "9a9a9a9a-0000-0000-0000-000000000002"
	notifMissingID = "9a9a9a9a-0000-0000-0000-00000000ffff"
)

var fixedNow = time.Date(2026, 8, 7, 9, 30, 0, 0, time.UTC)

// --- fakes -------------------------------------------------------------------

// fakeRepo is an in-memory comms store with the REAL idempotency semantics:
// RecordEvent claims an event key exactly once and only then materialises the
// notification, so a redelivered event writes nothing. It also records how it
// was called, which is what the ledger/push assertions read.
type fakeRepo struct {
	claimed       map[string]bool
	notifications []service.Notification
	devices       []service.Device

	recordCalls  int
	deviceCalls  int
	lastListArgs [2]string // studentID, "unread"|"all"

	recordErr  error
	devicesErr error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{claimed: map[string]bool{}}
}

func (f *fakeRepo) RecordEvent(_ context.Context, eventKey string, n service.Notification) (service.Notification, bool, error) {
	f.recordCalls++
	if f.recordErr != nil {
		return service.Notification{}, false, f.recordErr
	}
	if f.claimed[eventKey] {
		return service.Notification{}, false, nil
	}
	f.claimed[eventKey] = true
	f.notifications = append(f.notifications, n)
	return n, true, nil
}

func (f *fakeRepo) ListNotifications(_ context.Context, studentID string, unreadOnly bool) ([]service.Notification, error) {
	scope := "all"
	if unreadOnly {
		scope = "unread"
	}
	f.lastListArgs = [2]string{studentID, scope}

	out := make([]service.Notification, 0, len(f.notifications))
	for _, n := range f.notifications {
		if unreadOnly && !n.ReadAt.IsZero() {
			continue
		}
		out = append(out, n)
	}
	return out, nil
}

func (f *fakeRepo) MarkRead(_ context.Context, id string) (service.Notification, error) {
	for i, n := range f.notifications {
		if n.ID != id {
			continue
		}
		if n.ReadAt.IsZero() {
			f.notifications[i].ReadAt = fixedNow
		}
		return f.notifications[i], nil
	}
	return service.Notification{}, glerrors.ErrNotFound
}

func (f *fakeRepo) UpsertDevice(_ context.Context, d service.Device) (service.Device, error) {
	for i, existing := range f.devices {
		if existing.FCMToken != d.FCMToken {
			continue
		}
		f.devices[i].UserID = d.UserID
		f.devices[i].Platform = d.Platform
		return f.devices[i], nil
	}
	f.devices = append(f.devices, d)
	return d, nil
}

func (f *fakeRepo) DevicesForTenant(_ context.Context) ([]service.Device, error) {
	f.deviceCalls++
	if f.devicesErr != nil {
		return nil, f.devicesErr
	}
	return f.devices, nil
}

// spySender records what it was asked to push and can be made to fail, which is
// how the best-effort contract is proven.
type spySender struct {
	calls  int
	tokens []string
	title  string
	body   string
	err    error
}

func (s *spySender) Push(_ context.Context, tokens []string, title, body string) error {
	s.calls++
	s.tokens = append([]string(nil), tokens...)
	s.title, s.body = title, body
	return s.err
}

// --- helpers -----------------------------------------------------------------

// newService wires a deterministic service: ids come from a counter and the
// clock is fixed, so every assertion below is on an exact value.
func newService(t *testing.T, repo service.Repo, sender service.Sender) (*service.CommsService, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ids := 0
	newID := func() string {
		ids++
		return "id-" + string(rune('0'+ids))
	}
	return service.NewCommsService(repo, sender, tenantID, newID, func() time.Time { return fixedNow }, log), &logs
}

// absenceMessage builds the transport message attendance publishes for an
// absence: proto bytes plus the routing attributes the consumer dispatches on.
func absenceMessage(t *testing.T, recordID, student, tenant, date string) pubsub.Message {
	t.Helper()
	return protoMessage(t, "AbsenceRecorded", &attendancev1.AbsenceRecorded{
		RecordId:     recordID,
		EnrollmentId: "cccccccc-0000-0000-0000-000000000001",
		StudentId:    student,
		TenantId:     tenant,
		Date:         date,
	}, tenant)
}

func paymentMessage(t *testing.T, paymentID, student, tenant string, cents int64) pubsub.Message {
	t.Helper()
	return protoMessage(t, "PaymentRegistered", &financev1.PaymentRegistered{
		PaymentId:   paymentID,
		StudentId:   student,
		ChargeId:    "dddddddd-0000-0000-0000-000000000001",
		TenantId:    tenant,
		AmountCents: cents,
		Method:      "cash",
		PaidAt:      timestamppb.New(fixedNow),
	}, tenant)
}

func markedMessage(t *testing.T, recordID, student, tenant, date, status string) pubsub.Message {
	t.Helper()
	return protoMessage(t, "AttendanceMarked", &attendancev1.AttendanceMarked{
		RecordId:     recordID,
		EnrollmentId: "cccccccc-0000-0000-0000-000000000002",
		StudentId:    student,
		TenantId:     tenant,
		Date:         date,
		Status:       status,
	}, tenant)
}

func protoMessage(t *testing.T, eventType string, msg proto.Message, tenant string) pubsub.Message {
	t.Helper()
	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal %s fixture: %v", eventType, err)
	}
	return pubsub.Message{
		Topic:      "test-events",
		Data:       data,
		Attributes: map[string]string{"type": eventType, "tenant_id": tenant},
	}
}

// --- HandleEvent -------------------------------------------------------------

// TestHandleEventMaterialisesAnAbsence is the happy path of the consumer: an
// AbsenceRecorded event becomes exactly one stored notification for the event's
// student, carrying the rendered Spanish copy, and the push is attempted.
func TestHandleEventMaterialisesAnAbsence(t *testing.T) {
	repo := newFakeRepo()
	if _, err := repo.UpsertDevice(context.Background(), service.Device{
		ID: "device-1", UserID: userID, FCMToken: "token-abc", Platform: "android",
	}); err != nil {
		t.Fatalf("seed device: %v", err)
	}
	sender := &spySender{}
	svc, _ := newService(t, repo, sender)

	msg := absenceMessage(t, "eeeeeeee-0000-0000-0000-000000000001", studentID, tenantID, "2026-08-06")
	if err := svc.HandleEvent(context.Background(), msg); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	if len(repo.notifications) != 1 {
		t.Fatalf("stored %d notifications, want 1: %+v", len(repo.notifications), repo.notifications)
	}
	got := repo.notifications[0]
	if got.StudentID != studentID {
		t.Errorf("student_id = %q, want %q", got.StudentID, studentID)
	}
	if got.Type != "absence" {
		t.Errorf("type = %q, want %q", got.Type, "absence")
	}
	if got.Title != "Ausencia registrada" {
		t.Errorf("title = %q, want %q", got.Title, "Ausencia registrada")
	}
	if !strings.Contains(got.Body, "2026-08-06") {
		t.Errorf("body = %q, want it to name the day", got.Body)
	}
	if got.ID == "" {
		t.Error("notification id is empty; the service must stamp an injected id")
	}
	if !got.CreatedAt.Equal(fixedNow) {
		t.Errorf("created_at = %v, want the injected clock %v", got.CreatedAt, fixedNow)
	}
	if !got.ReadAt.IsZero() {
		t.Errorf("read_at = %v, want zero (a new notification is unread)", got.ReadAt)
	}

	if sender.calls != 1 {
		t.Fatalf("sender called %d times, want 1", sender.calls)
	}
	if len(sender.tokens) != 1 || sender.tokens[0] != "token-abc" {
		t.Errorf("pushed tokens = %v, want [token-abc]", sender.tokens)
	}
	if sender.title != got.Title || sender.body != got.Body {
		t.Errorf("push payload = %q/%q, want the notification's %q/%q", sender.title, sender.body, got.Title, got.Body)
	}
}

// TestHandleEventIsIdempotent is the load-bearing test of this service: Pub/Sub
// delivers at-least-once, so the SAME message arriving twice must leave exactly
// one notification. The second delivery is a no-op decided by the event ledger,
// and it must still be acked (no error) or the transport would redeliver forever.
func TestHandleEventIsIdempotent(t *testing.T) {
	repo := newFakeRepo()
	sender := &spySender{}
	svc, _ := newService(t, repo, sender)
	ctx := context.Background()
	// A registered device, so "the push happened exactly once" is an assertion
	// about the redelivery rather than about there being nobody to notify.
	repo.devices = []service.Device{{ID: "device-1", UserID: userID, FCMToken: "token-abc"}}

	msg := absenceMessage(t, "eeeeeeee-0000-0000-0000-000000000002", studentID, tenantID, "2026-08-06")
	if err := svc.HandleEvent(ctx, msg); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	if err := svc.HandleEvent(ctx, msg); err != nil {
		t.Fatalf("a redelivery must be acked, not failed: %v", err)
	}

	if len(repo.notifications) != 1 {
		t.Fatalf("a redelivered event produced %d notifications, want exactly 1", len(repo.notifications))
	}
	if repo.recordCalls != 2 {
		t.Errorf("the ledger was consulted %d times, want 2 (once per delivery)", repo.recordCalls)
	}
	if sender.calls != 1 {
		t.Errorf("push attempted %d times, want 1 — a redelivery must not re-notify the family", sender.calls)
	}
}

// TestHandleEventDistinctEventsBothLand guards against an over-eager ledger key:
// two different events must not collide just because they share a student.
func TestHandleEventDistinctEventsBothLand(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := newService(t, repo, &spySender{})
	ctx := context.Background()

	first := absenceMessage(t, "eeeeeeee-0000-0000-0000-000000000003", studentID, tenantID, "2026-08-06")
	second := paymentMessage(t, "ffffffff-0000-0000-0000-000000000001", studentID, tenantID, 150000)
	for _, msg := range []pubsub.Message{first, second} {
		if err := svc.HandleEvent(ctx, msg); err != nil {
			t.Fatalf("HandleEvent: %v", err)
		}
	}

	if len(repo.notifications) != 2 {
		t.Fatalf("stored %d notifications, want 2 (absence + payment)", len(repo.notifications))
	}
	if repo.notifications[1].Type != "payment" {
		t.Errorf("second type = %q, want %q", repo.notifications[1].Type, "payment")
	}
	if !strings.Contains(repo.notifications[1].Body, "1500.00") {
		t.Errorf("payment body = %q, want the amount in pesos", repo.notifications[1].Body)
	}
}

// TestHandleEventPushFailureIsBestEffort pins the durability rule: the
// notification is already stored, so a dead push must not fail the handler —
// returning an error would redeliver the event and (once the ledger has claimed
// it) silently drop the notification instead. The failure is logged, never
// swallowed silently.
func TestHandleEventPushFailureIsBestEffort(t *testing.T) {
	repo := newFakeRepo()
	if _, err := repo.UpsertDevice(context.Background(), service.Device{
		ID: "device-1", UserID: userID, FCMToken: "token-abc",
	}); err != nil {
		t.Fatalf("seed device: %v", err)
	}
	sender := &spySender{err: errors.New("fcm unreachable")}
	svc, logs := newService(t, repo, sender)

	msg := absenceMessage(t, "eeeeeeee-0000-0000-0000-000000000004", studentID, tenantID, "2026-08-06")
	if err := svc.HandleEvent(context.Background(), msg); err != nil {
		t.Fatalf("a push failure must not fail HandleEvent: %v", err)
	}
	if len(repo.notifications) != 1 {
		t.Errorf("stored %d notifications, want 1 — the feed entry is durable", len(repo.notifications))
	}
	if !strings.Contains(logs.String(), "fcm unreachable") {
		t.Errorf("the dropped push was not logged; logs = %q", logs.String())
	}
}

// TestHandleEventDeviceLookupFailureIsBestEffort covers the other half of the
// push path: not even a broken device read may cost us the stored notification.
func TestHandleEventDeviceLookupFailureIsBestEffort(t *testing.T) {
	repo := newFakeRepo()
	repo.devicesErr = errors.New("devices unavailable")
	sender := &spySender{}
	svc, logs := newService(t, repo, sender)

	msg := absenceMessage(t, "eeeeeeee-0000-0000-0000-000000000005", studentID, tenantID, "2026-08-06")
	if err := svc.HandleEvent(context.Background(), msg); err != nil {
		t.Fatalf("a device-read failure must not fail HandleEvent: %v", err)
	}
	if len(repo.notifications) != 1 {
		t.Errorf("stored %d notifications, want 1", len(repo.notifications))
	}
	if sender.calls != 0 {
		t.Errorf("sender called %d times, want 0 when the targets are unknown", sender.calls)
	}
	if !strings.Contains(logs.String(), "devices unavailable") {
		t.Errorf("the failed device read was not logged; logs = %q", logs.String())
	}
}

// TestHandleEventNilSenderIsSupported proves push is optional: a service wired
// without a sender still materialises the feed and never touches the devices.
func TestHandleEventNilSenderIsSupported(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := newService(t, repo, nil)

	msg := absenceMessage(t, "eeeeeeee-0000-0000-0000-000000000006", studentID, tenantID, "2026-08-06")
	if err := svc.HandleEvent(context.Background(), msg); err != nil {
		t.Fatalf("HandleEvent with a nil sender: %v", err)
	}
	if len(repo.notifications) != 1 {
		t.Errorf("stored %d notifications, want 1", len(repo.notifications))
	}
	if repo.deviceCalls != 0 {
		t.Errorf("devices were read %d times, want 0 without a sender", repo.deviceCalls)
	}
}

// TestHandleEventLedgerFailureRedelivers is the inverse of best-effort: when the
// ledger itself cannot be reached, nothing was stored, so the handler MUST fail
// and let the transport redeliver.
func TestHandleEventLedgerFailureRedelivers(t *testing.T) {
	repo := newFakeRepo()
	repo.recordErr = errors.New("database down")
	svc, _ := newService(t, repo, &spySender{})

	msg := absenceMessage(t, "eeeeeeee-0000-0000-0000-000000000007", studentID, tenantID, "2026-08-06")
	err := svc.HandleEvent(context.Background(), msg)
	if err == nil {
		t.Fatal("a ledger failure must be returned so the event is redelivered")
	}
	if !strings.Contains(err.Error(), "database down") {
		t.Errorf("error = %v, want the cause preserved", err)
	}
}

// TestHandleEventUnhandledMessagesAreAcked covers everything this consumer does
// not turn into a notification. None of it may error: an unknown type or a
// corrupt payload will never become handleable, so failing would redeliver it
// forever. Nothing is stored, and the LOG LEVEL tells the two kinds apart —
// which is the point of the split: an operator scanning warnings must find only
// events that are actually wrong, never the ones this consumer skips by design
// (an absence arriving twice, once per event, happens on every school day).
func TestHandleEventUnhandledMessagesAreAcked(t *testing.T) {
	absentMarked := markedMessage(t, "eeeeeeee-0000-0000-0000-000000000008", studentID, tenantID, "2026-08-06", "absent")
	corrupt := absenceMessage(t, "eeeeeeee-0000-0000-0000-000000000009", studentID, tenantID, "2026-08-06")
	corrupt.Data = []byte{0xff, 0xff, 0xff, 0xff}
	noID := protoMessage(t, "AbsenceRecorded", &attendancev1.AbsenceRecorded{
		StudentId: studentID, TenantId: tenantID, Date: "2026-08-06",
	}, tenantID)
	noStudent := protoMessage(t, "AbsenceRecorded", &attendancev1.AbsenceRecorded{
		RecordId: "eeeeeeee-0000-0000-0000-000000000010", TenantId: tenantID, Date: "2026-08-06",
	}, tenantID)

	const (
		levelSkipped = "level=DEBUG" // deliberately not notified: the normal path
		levelWrong   = "level=WARN"  // something an operator should look at
	)

	tests := []struct {
		name      string
		msg       pubsub.Message
		wantLevel string
	}{
		{"an unknown event type", pubsub.Message{
			Topic: "test-events", Data: []byte("{}"),
			Attributes: map[string]string{"type": "SomethingElse"},
		}, levelWrong},
		{"a message with no type attribute", pubsub.Message{
			Topic: "test-events", Data: []byte("{}"),
		}, levelWrong},
		{"a corrupt payload", corrupt, levelWrong},
		{"an event with no id to key the ledger on", noID, levelWrong},
		{"an event with no student to notify", noStudent, levelWrong},
		{"an event for another tenant", absenceMessage(t,
			"eeeeeeee-0000-0000-0000-000000000011", studentID, otherTenantID, "2026-08-06"), levelWrong},
		{"an attendance status outside the contract", markedMessage(t,
			"eeeeeeee-0000-0000-0000-000000000012", studentID, tenantID, "2026-08-06", "sick"), levelWrong},
		{"an absence arriving as AttendanceMarked (AbsenceRecorded already covers it)",
			absentMarked, levelSkipped},
		{"a presence, which no family is notified about", markedMessage(t,
			"eeeeeeee-0000-0000-0000-000000000013", studentID, tenantID, "2026-08-06", "present"), levelSkipped},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			sender := &spySender{}
			svc, logs := newService(t, repo, sender)

			if err := svc.HandleEvent(context.Background(), tt.msg); err != nil {
				t.Fatalf("an unhandleable message must be acked, not failed: %v", err)
			}
			if len(repo.notifications) != 0 {
				t.Errorf("stored %+v, want nothing", repo.notifications)
			}
			if repo.recordCalls != 0 {
				t.Errorf("the ledger was written %d times, want 0", repo.recordCalls)
			}
			if sender.calls != 0 {
				t.Errorf("sender called %d times, want 0", sender.calls)
			}
			if logs.Len() == 0 {
				t.Fatal("a dropped message must leave a log trace, not vanish")
			}
			if got := logs.String(); !strings.Contains(got, tt.wantLevel) {
				t.Errorf("logged %q, want it at %s", got, tt.wantLevel)
			}
		})
	}
}

// TestHandleEventRendersEveryNotifiableEvent is the table over the rendering
// contract: which events become which Spanish copy. It is the pure part of the
// consumer, so it is checked exhaustively here rather than case by case above.
func TestHandleEventRendersEveryNotifiableEvent(t *testing.T) {
	tests := []struct {
		name      string
		msg       pubsub.Message
		wantType  string
		wantTitle string
		wantBody  string
	}{
		{
			name:      "absence",
			msg:       absenceMessage(t, "eeeeeeee-1000-0000-0000-000000000001", studentID, tenantID, "2026-08-06"),
			wantType:  "absence",
			wantTitle: "Ausencia registrada",
			wantBody:  "Se registró una ausencia el 2026-08-06.",
		},
		{
			name:      "late arrival",
			msg:       markedMessage(t, "eeeeeeee-1000-0000-0000-000000000002", studentID, tenantID, "2026-08-06", "late"),
			wantType:  "attendance",
			wantTitle: "Llegada tarde",
			wantBody:  "Se registró una llegada tarde el 2026-08-06.",
		},
		{
			name:      "excused absence",
			msg:       markedMessage(t, "eeeeeeee-1000-0000-0000-000000000003", studentID, tenantID, "2026-08-06", "excused"),
			wantType:  "attendance",
			wantTitle: "Ausencia justificada",
			wantBody:  "Se registró una ausencia justificada el 2026-08-06.",
		},
		{
			name:      "payment",
			msg:       paymentMessage(t, "ffffffff-1000-0000-0000-000000000001", studentID, tenantID, 150000),
			wantType:  "payment",
			wantTitle: "Pago recibido",
			wantBody:  "Recibimos tu pago de $1500.00 MXN.",
		},
		{
			name:      "payment of a few cents keeps two decimals",
			msg:       paymentMessage(t, "ffffffff-1000-0000-0000-000000000002", studentID, tenantID, 5),
			wantType:  "payment",
			wantTitle: "Pago recibido",
			wantBody:  "Recibimos tu pago de $0.05 MXN.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc, _ := newService(t, repo, nil)

			if err := svc.HandleEvent(context.Background(), tt.msg); err != nil {
				t.Fatalf("HandleEvent: %v", err)
			}
			if len(repo.notifications) != 1 {
				t.Fatalf("stored %d notifications, want 1", len(repo.notifications))
			}
			got := repo.notifications[0]
			if got.Type != tt.wantType {
				t.Errorf("type = %q, want %q", got.Type, tt.wantType)
			}
			if got.Title != tt.wantTitle {
				t.Errorf("title = %q, want %q", got.Title, tt.wantTitle)
			}
			if got.Body != tt.wantBody {
				t.Errorf("body = %q, want %q", got.Body, tt.wantBody)
			}
		})
	}
}

// TestHandleEventOverTheBus wires the consumer to the real pkg/pubsub in-memory
// transport, proving HandleEvent is usable AS a pubsub.Handler (the signature the
// subscription hands messages to) and not merely a method that looks like one.
func TestHandleEventOverTheBus(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := newService(t, repo, &spySender{})

	bus := pubsub.NewMemoryBus()
	bus.CreateSubscription("attendance-events", "comms-attendance")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handled := make(chan struct{})
	var handler pubsub.Handler = func(ctx context.Context, msg pubsub.Message) error {
		err := svc.HandleEvent(ctx, msg)
		close(handled)
		return err
	}
	done := make(chan error, 1)
	go func() { done <- bus.Subscribe(ctx, "comms-attendance", handler) }()

	msg := absenceMessage(t, "eeeeeeee-2000-0000-0000-000000000001", studentID, tenantID, "2026-08-06")
	msg.Topic = "attendance-events"
	if err := bus.Publish(ctx, msg); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case <-handled:
	case <-time.After(2 * time.Second):
		t.Fatal("the subscription never dispatched the event to HandleEvent")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Subscribe returned %v, want nil on cancellation", err)
	}

	if len(repo.notifications) != 1 {
		t.Fatalf("consumed over the bus produced %d notifications, want 1", len(repo.notifications))
	}
	if repo.notifications[0].Type != "absence" {
		t.Errorf("type = %q, want %q", repo.notifications[0].Type, "absence")
	}
}

// --- RegisterDevice ----------------------------------------------------------

// TestRegisterDeviceValidatesInput pins the boundary checks: every rejection is
// a client error (KindInvalid), never an internal one, and nothing is stored.
func TestRegisterDeviceValidatesInput(t *testing.T) {
	tests := []struct {
		name     string
		userID   string
		token    string
		platform string
	}{
		{"an empty user", "", "token-1", "ios"},
		{"a malformed user id", "not-a-uuid", "token-1", "ios"},
		{"an empty token", userID, "", "ios"},
		{"a blank token", userID, "   ", "ios"},
		{"an unknown platform", userID, "token-1", "blackberry"},
		{"a case-mismatched platform", userID, "token-1", "iOS"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc, _ := newService(t, repo, nil)

			_, err := svc.RegisterDevice(context.Background(), tt.userID, tt.token, tt.platform)
			if err == nil {
				t.Fatal("want a validation error, got nil")
			}
			if got := glerrors.KindOf(err); got != glerrors.KindInvalid {
				t.Errorf("kind = %v, want KindInvalid", got)
			}
			if len(repo.devices) != 0 {
				t.Errorf("stored %+v, want nothing", repo.devices)
			}
		})
	}
}

// TestRegisterDeviceAcceptsEveryKnownPlatform covers the accepted vocabulary,
// including the empty string: a client that does not report its platform must
// still be registrable (the column defaults to ” for exactly that reason).
func TestRegisterDeviceAcceptsEveryKnownPlatform(t *testing.T) {
	for _, platform := range []string{"", "ios", "android", "web"} {
		t.Run("platform "+platform, func(t *testing.T) {
			repo := newFakeRepo()
			svc, _ := newService(t, repo, nil)

			got, err := svc.RegisterDevice(context.Background(), userID, "token-"+platform, platform)
			if err != nil {
				t.Fatalf("RegisterDevice: %v", err)
			}
			if got.Platform != platform {
				t.Errorf("platform = %q, want %q", got.Platform, platform)
			}
			if got.ID == "" {
				t.Error("device id is empty; the service must stamp an injected id")
			}
			if got.UserID != userID || got.FCMToken != "token-"+platform {
				t.Errorf("device = %+v, want the submitted user and token", got)
			}
		})
	}
}

// TestRegisterDeviceIsAnUpsert proves re-registering the same token refreshes the
// existing device instead of fanning the same phone out twice — the reason a
// client may call this on every launch.
func TestRegisterDeviceIsAnUpsert(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := newService(t, repo, nil)
	ctx := context.Background()

	if _, err := svc.RegisterDevice(ctx, userID, "token-shared", "ios"); err != nil {
		t.Fatalf("first registration: %v", err)
	}
	got, err := svc.RegisterDevice(ctx, userID, "token-shared", "web")
	if err != nil {
		t.Fatalf("re-registration: %v", err)
	}
	if len(repo.devices) != 1 {
		t.Fatalf("stored %d devices, want 1 (the token is the upsert key)", len(repo.devices))
	}
	if got.Platform != "web" {
		t.Errorf("platform = %q, want the refreshed %q", got.Platform, "web")
	}
}

// --- reads -------------------------------------------------------------------

// TestListNotificationsFiltersUnread proves the unread flag reaches the store
// (the badge count depends on it) and that the student id is validated first.
func TestListNotificationsFiltersUnread(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := newService(t, repo, nil)
	ctx := context.Background()

	repo.notifications = []service.Notification{
		{ID: notifOneID, StudentID: studentID, Type: "absence", Title: "a", Body: "b", CreatedAt: fixedNow},
		{ID: notifTwoID, StudentID: studentID, Type: "payment", Title: "c", Body: "d", CreatedAt: fixedNow, ReadAt: fixedNow},
	}

	all, err := svc.ListNotifications(ctx, studentID, false)
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("unfiltered list returned %d, want 2", len(all))
	}
	if repo.lastListArgs != [2]string{studentID, "all"} {
		t.Errorf("repo called with %v, want the student and the unfiltered scope", repo.lastListArgs)
	}

	unread, err := svc.ListNotifications(ctx, studentID, true)
	if err != nil {
		t.Fatalf("ListNotifications(unreadOnly): %v", err)
	}
	if len(unread) != 1 || unread[0].ID != notifOneID {
		t.Errorf("unread list = %+v, want only the unread one", unread)
	}
	if repo.lastListArgs != [2]string{studentID, "unread"} {
		t.Errorf("repo called with %v, want the unread scope forwarded", repo.lastListArgs)
	}

	if _, err := svc.ListNotifications(ctx, "not-a-uuid", false); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("a malformed student id gave %v, want KindInvalid", err)
	}
}

// TestMarkNotificationReadIsIdempotent proves the read stamp is set once and
// re-marking keeps the original instant, and that an unknown id is NotFound
// rather than a silent success.
func TestMarkNotificationReadIsIdempotent(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := newService(t, repo, nil)
	ctx := context.Background()
	repo.notifications = []service.Notification{
		{ID: notifOneID, StudentID: studentID, Type: "absence", Title: "a", Body: "b", CreatedAt: fixedNow},
	}

	first, err := svc.MarkNotificationRead(ctx, notifOneID)
	if err != nil {
		t.Fatalf("MarkNotificationRead: %v", err)
	}
	if first.ReadAt.IsZero() {
		t.Fatal("read_at is still unset after marking read")
	}
	second, err := svc.MarkNotificationRead(ctx, notifOneID)
	if err != nil {
		t.Fatalf("re-marking must be idempotent: %v", err)
	}
	if !second.ReadAt.Equal(first.ReadAt) {
		t.Errorf("read_at moved from %v to %v; the first stamp must stand", first.ReadAt, second.ReadAt)
	}

	if _, err := svc.MarkNotificationRead(ctx, notifMissingID); !errors.Is(err, glerrors.ErrNotFound) {
		t.Errorf("unknown id gave %v, want ErrNotFound", err)
	}
	if _, err := svc.MarkNotificationRead(ctx, ""); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("an empty id gave %v, want KindInvalid", err)
	}
}
