package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	attendancev1 "github.com/garcia-lorca/gen/go/garcialorca/attendance/v1"
	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/pubsub"
)

// The event types this consumer dispatches on, as the publishing services stamp
// them in the message's "type" attribute (see each service's adapters/pubsub).
const (
	eventAbsenceRecorded   = "AbsenceRecorded"
	eventAttendanceMarked  = "AttendanceMarked"
	eventPaymentRegistered = "PaymentRegistered"
)

// attrType is the routing attribute carrying the event type name. It comes from
// the messaging boundary, not from here: producers stamp the same constant.
const attrType = pubsub.AttrType

// The notification vocabulary stored in `type`, as declared by the proto
// contract (garcialorca.comms.v1.Notification): a short, stable kind the client
// switches on for its icon — NOT the producing event's name, which changes with
// the upstream contract.
const (
	typeAbsence    = "absence"
	typeAttendance = "attendance"
	typePayment    = "payment"
)

// The attendance statuses that reach this consumer. statusAbsent is deliberately
// NOT notified from AttendanceMarked: attendance publishes both events for an
// absence, and rendering each would notify the family twice for one absence.
// statusPresent is not notified either — attendance stores only exceptions, so it
// should never arrive, but a presence is a non-event rather than a fault.
const (
	statusAbsent  = "absent"
	statusPresent = "present"
	statusLate    = "late"
	statusExcused = "excused"
)

// The push platforms a device may report. platformUnknown, the empty string, is
// accepted on purpose — a client that does not report its platform must still be
// registrable, and the column defaults to it.
const (
	platformUnknown = ""
	platformIOS     = "ios"
	platformAndroid = "android"
	platformWeb     = "web"
)

// Both sentinels below mean "this message produces no notification, ACK it":
// redelivering could not change the outcome, so failing would loop forever. They
// are told apart only to be REPORTED apart — one is the normal path, the other is
// something an operator should look at.
var (
	// errNotNotifiable marks an event this consumer deliberately does not notify
	// on: an absence arriving as AttendanceMarked (AbsenceRecorded already
	// notified it), a presence. This happens on every school day, so it is logged
	// at Debug — warning on it would bury the real problems in noise.
	errNotNotifiable = errors.New("event is deliberately not notified")

	// errUndeliverable marks a message that is WRONG: an unknown event type, a
	// payload that will not unmarshal, an event with no id or no student, a status
	// outside the contract, another tenant's event. Each means comms and a
	// producer disagree — or someone is publishing where they should not — so it
	// is logged at Warn.
	errUndeliverable = errors.New("event cannot be turned into a notification")
)

// --- seams -------------------------------------------------------------------
//
// The interfaces below are consumer-defined: comms declares the narrow view it
// needs and the adapters (the GORM repository, the FCM sender) satisfy them from
// the outside. Comms owns no students and no users — both are cross-service
// UUIDs it only carries.

// Notification is one message materialised for a student out of a consumed
// domain event. Type is the stable client-facing kind (absence|attendance|
// payment); Title and Body are the rendered Spanish copy. A zero ReadAt means
// the notification is still unread, which is what the badge counts.
type Notification struct {
	ID        string
	StudentID string
	Type      string
	Title     string
	Body      string
	CreatedAt time.Time
	ReadAt    time.Time
}

// Device is one push target: the FCM token a user's client is reachable at, plus
// the platform it came from.
type Device struct {
	ID       string
	UserID   string
	FCMToken string
	Platform string
}

// Repo stores the notification feed, the push devices and the consumed-event
// ledger. All access is tenant-scoped for RLS (the adapter binds
// app.current_tenant per transaction, as in students/grades/attendance).
type Repo interface {
	// RecordEvent claims eventKey in the consumed-event ledger and materialises n
	// in the SAME transaction, returning the stored notification. isNew is false
	// when the key was already claimed — a redelivery — and NOTHING is written.
	//
	// Claiming and storing are one unit on purpose: split across two
	// transactions, a failure between them would leave the event marked handled
	// with no notification to show for it, and the redelivery would be skipped.
	RecordEvent(ctx context.Context, eventKey string, n Notification) (created Notification, isNew bool, err error)
	// ListNotifications returns one student's notifications, newest first. With
	// unreadOnly, only those whose read_at is unset.
	ListNotifications(ctx context.Context, studentID string, unreadOnly bool) ([]Notification, error)
	// MarkRead stamps a notification as read and returns it. Marking an
	// already-read notification keeps the original instant (idempotent). An
	// unknown id is glerrors.ErrNotFound.
	MarkRead(ctx context.Context, id string) (Notification, error)
	// UpsertDevice registers d, refreshing the existing row when its token is
	// already known rather than fanning the same device out twice.
	UpsertDevice(ctx context.Context, d Device) (Device, error)
	// DevicesForUsers returns the push targets OWNED by any of userIDs — for a
	// notification, its student's guardians. It is one batched read, never one
	// per user, and no userIDs means no targets (and no query).
	DevicesForUsers(ctx context.Context, userIDs []string) ([]Device, error)
}

// GuardianReader resolves which users are a student's guardians — the audience a
// notification about that student is pushed to. Comms owns no guardian link: the
// students bounded context does, and the adapter reads it by RPC.
//
// A student with no guardians linked yet is an EMPTY list, not an error: that is
// the ordinary state of a school still onboarding its families, and the durable
// feed is exact regardless of who can be reached.
type GuardianReader interface {
	GuardiansForStudent(ctx context.Context, studentID string) ([]string, error)
}

// Sender delivers a rendered notification to push targets. Delivery is
// best-effort (see push): a nil Sender disables it entirely.
type Sender interface {
	Push(ctx context.Context, tokens []string, title, body string) error
}

// --- service -----------------------------------------------------------------

// CommsService materialises domain events from other services into a per-student
// notification feed, and serves that feed plus device registration.
//
// It PUBLISHES NOTHING: sending is a side effect of consuming an event, never an
// RPC. Its one hard requirement is idempotency — Pub/Sub is at-least-once, so
// every delivery goes through the consumed-event ledger before anything is
// written:
//
//	HandleEvent: render (pure) ─> claim the event + store the feed entry
//	             (one tx) ─> push (best-effort, never fails the handler)
//
// Ids come from an injected newID and the timestamps from an injected now, so
// the consumer is deterministic under test.
type CommsService struct {
	repo      Repo
	guardians GuardianReader
	sender    Sender
	tenantID  string
	newID     func() string
	now       func() time.Time
	log       *slog.Logger
}

// NewCommsService wires the service. tenantID is the single-tenant scope for this
// phase and the tenant every consumed event must belong to; sender may be nil,
// which disables push and leaves only the durable feed, and so may guardians,
// which leaves the push untargetable and therefore not attempted. log receives
// the dropped messages and the best-effort push failures; a nil log falls back to
// slog.Default so nothing is ever silently discarded.
func NewCommsService(
	repo Repo,
	guardians GuardianReader,
	sender Sender,
	tenantID string,
	newID func() string,
	now func() time.Time,
	log *slog.Logger,
) *CommsService {
	if log == nil {
		log = slog.Default()
	}
	return &CommsService{
		repo:      repo,
		guardians: guardians,
		sender:    sender,
		tenantID:  tenantID,
		newID:     newID,
		now:       now,
		log:       log,
	}
}

// HandleEvent is the idempotent consumer, usable directly as a pubsub.Handler.
//
// Returning an error asks the transport to redeliver, so the two failure modes
// are told apart deliberately:
//
//   - Nothing was stored and a retry could succeed (the ledger is unreachable)
//     ⇒ return the error and let the event come back.
//   - The message can never be handled (errUndeliverable) or is one this consumer
//     does not notify on by design (errNotNotifiable) ⇒ log it — at Warn and at
//     Debug respectively — and ACK, because redelivering it would loop forever.
//
// Once the feed entry is stored the event is DONE: the push that follows is
// best-effort and its failure is never returned, since a redelivery would be
// skipped by the ledger and the notification lost for nothing.
func (s *CommsService) HandleEvent(ctx context.Context, msg pubsub.Message) error {
	eventType := msg.Attributes[attrType]

	eventKey, n, err := s.render(eventType, msg.Data)
	switch {
	case errors.Is(err, errNotNotifiable):
		s.log.Debug("comms skipped an event it does not notify on",
			slog.String("event_type", eventType),
			slog.Any("reason", err))
		return nil
	case err != nil:
		s.log.Warn("comms dropped an event it cannot notify on",
			slog.String("event_type", eventType),
			slog.Any("error", err))
		return nil
	}

	n.ID = s.newID()
	n.CreatedAt = s.now()
	created, isNew, err := s.repo.RecordEvent(ctx, eventKey, n)
	if err != nil {
		return fmt.Errorf("recording event %s: %w", eventKey, err)
	}
	if !isNew {
		// A redelivery of an event already handled: acking is the whole point of
		// the ledger.
		s.log.Debug("comms skipped an already-handled event", slog.String("event_key", eventKey))
		return nil
	}

	s.push(ctx, created)
	return nil
}

// RegisterDevice registers (or refreshes) the FCM token a user's device is
// reachable at. Re-registering a known token updates it rather than duplicating
// it, so a client may call this on every launch.
func (s *CommsService) RegisterDevice(ctx context.Context, userID, fcmToken, platform string) (Device, error) {
	if err := uuid.Validate(userID); err != nil {
		return Device{}, glerrors.New(glerrors.KindInvalid, "invalid user id")
	}
	if strings.TrimSpace(fcmToken) == "" {
		return Device{}, glerrors.New(glerrors.KindInvalid, "fcm token is required")
	}
	if !isKnownPlatform(platform) {
		return Device{}, glerrors.New(glerrors.KindInvalid, "platform must be one of ios, android, web")
	}
	return s.repo.UpsertDevice(ctx, Device{
		ID:       s.newID(),
		UserID:   userID,
		FCMToken: fcmToken,
		Platform: platform,
	})
}

// ListNotifications returns one student's notifications, newest first; with
// unreadOnly, only the unread ones. The student id is checked here rather than
// left to Postgres: the column is a uuid, so a malformed id would otherwise
// surface as an infrastructure error for a plain client mistake.
func (s *CommsService) ListNotifications(ctx context.Context, studentID string, unreadOnly bool) ([]Notification, error) {
	if err := uuid.Validate(studentID); err != nil {
		return nil, glerrors.New(glerrors.KindInvalid, "invalid student id")
	}
	return s.repo.ListNotifications(ctx, studentID, unreadOnly)
}

// MarkNotificationRead stamps a notification as read and returns it. Marking an
// already-read notification is idempotent: read_at keeps its original value.
func (s *CommsService) MarkNotificationRead(ctx context.Context, id string) (Notification, error) {
	if err := uuid.Validate(id); err != nil {
		return Notification{}, glerrors.New(glerrors.KindInvalid, "invalid notification id")
	}
	return s.repo.MarkRead(ctx, id)
}

// push delivers a stored notification to the devices of ITS STUDENT'S GUARDIANS,
// resolved in two steps: which users are the student's guardians (students owns
// that link), then which devices those users registered here.
//
// Every failure is logged and swallowed: the feed entry is already durable, and
// the family seeing it late is strictly better than failing the handler — which,
// with the event already claimed, would lose the notification rather than retry
// it. A failure never widens the audience either: when the guardians cannot be
// resolved, nothing is pushed, because pushing one family's absence to a guessed
// audience is worse than pushing late.
func (s *CommsService) push(ctx context.Context, n Notification) {
	if s.sender == nil {
		return
	}
	if s.guardians == nil {
		s.log.Warn("comms cannot target a push without a guardian reader",
			slog.String("notification_id", n.ID))
		return
	}
	guardianIDs, err := s.guardians.GuardiansForStudent(ctx, n.StudentID)
	if err != nil {
		s.log.Warn("comms could not resolve the student's guardians",
			slog.String("notification_id", n.ID),
			slog.String("student_id", n.StudentID),
			slog.Any("error", err))
		return
	}
	if len(guardianIDs) == 0 {
		// The ordinary state of a student whose family is not linked yet: the feed
		// entry stands, there is simply nobody to reach.
		s.log.Debug("comms notified a student with no guardians to push to",
			slog.String("notification_id", n.ID),
			slog.String("student_id", n.StudentID))
		return
	}

	devices, err := s.repo.DevicesForUsers(ctx, guardianIDs)
	if err != nil {
		s.log.Warn("comms could not resolve push targets",
			slog.String("notification_id", n.ID),
			slog.Any("error", err))
		return
	}
	if len(devices) == 0 {
		return
	}
	tokens := make([]string, 0, len(devices))
	for _, d := range devices {
		tokens = append(tokens, d.FCMToken)
	}
	if err := s.sender.Push(ctx, tokens, n.Title, n.Body); err != nil {
		s.log.Warn("comms push not delivered",
			slog.String("notification_id", n.ID),
			slog.Int("target_count", len(tokens)),
			slog.Any("error", err))
	}
}

// render turns a raw event into its ledger key and the notification to store. It
// is PURE — no clock, no ids, no I/O — so the whole rendering contract is
// table-testable, and it is the single place that knows the upstream protos.
//
// Every message it produces no notification for comes back as errUndeliverable
// (the message is wrong: an unknown type, an unreadable payload, an event with no
// id to key the ledger on or no student to notify, and — the tenant guard — an
// event belonging to someone else, which the repository would otherwise store
// under ours) or errNotNotifiable (a well-formed event this consumer skips by
// design). HandleEvent acks both and logs them at different levels.
func (s *CommsService) render(eventType string, data []byte) (eventKey string, n Notification, err error) {
	switch eventType {
	case eventAbsenceRecorded:
		var e attendancev1.AbsenceRecorded
		if err := proto.Unmarshal(data, &e); err != nil {
			return "", Notification{}, fmt.Errorf("%w: unreadable %s payload: %w", errUndeliverable, eventType, err)
		}
		return s.notification(eventType, e.GetRecordId(), e.GetMarkVersion(), e.GetStudentId(), e.GetTenantId(), Notification{
			Type:  typeAbsence,
			Title: "Ausencia registrada",
			Body:  fmt.Sprintf("Se registró una ausencia el %s.", e.GetDate()),
		})

	case eventAttendanceMarked:
		var e attendancev1.AttendanceMarked
		if err := proto.Unmarshal(data, &e); err != nil {
			return "", Notification{}, fmt.Errorf("%w: unreadable %s payload: %w", errUndeliverable, eventType, err)
		}
		title, body, err := attendanceCopy(e.GetStatus(), e.GetDate())
		if err != nil {
			return "", Notification{}, err
		}
		return s.notification(eventType, e.GetRecordId(), e.GetMarkVersion(), e.GetStudentId(), e.GetTenantId(), Notification{
			Type:  typeAttendance,
			Title: title,
			Body:  body,
		})

	case eventPaymentRegistered:
		var e financev1.PaymentRegistered
		if err := proto.Unmarshal(data, &e); err != nil {
			return "", Notification{}, fmt.Errorf("%w: unreadable %s payload: %w", errUndeliverable, eventType, err)
		}
		return s.notification(eventType, e.GetPaymentId(), unversioned, e.GetStudentId(), e.GetTenantId(), Notification{
			Type:  typePayment,
			Title: "Pago recibido",
			Body:  fmt.Sprintf("Recibimos tu pago de %s MXN.", pesos(e.GetAmountCents())),
		})

	default:
		return "", Notification{}, fmt.Errorf("%w: unknown event type %q", errUndeliverable, eventType)
	}
}

// unversioned is the version of an event whose id is never reused (a payment),
// and what a pre-versioning attendance event decodes to.
const unversioned int64 = 0

// notification completes a rendered notification with the parts every event
// shares — its ledger key and its student — after the checks that make it
// storable at all. The key is "<event type>:<event id>", the form the
// processed_events column documents, plus ":<version>" for a versioned event:
// attendance keeps a mark's id across corrections, so only the version tells a
// correction (a new notification) from a redelivery (deduped). An unversioned
// event keeps the bare key, so events published before versioning still match
// the claims they already made.
func (s *CommsService) notification(eventType, eventID string, version int64, studentID, tenantID string, n Notification) (string, Notification, error) {
	if eventID == "" {
		return "", Notification{}, fmt.Errorf("%w: %s carries no event id", errUndeliverable, eventType)
	}
	if studentID == "" {
		return "", Notification{}, fmt.Errorf("%w: %s carries no student id", errUndeliverable, eventType)
	}
	if tenantID != s.tenantID {
		return "", Notification{}, fmt.Errorf("%w: %s belongs to another tenant", errUndeliverable, eventType)
	}
	n.StudentID = studentID
	return ledgerKey(eventType, eventID, version), n, nil
}

// ledgerKey is the idempotency key of one event: "<type>:<id>" or, when the
// event is versioned, "<type>:<id>:<version>".
func ledgerKey(eventType, eventID string, version int64) string {
	key := eventType + ":" + eventID
	if version == unversioned {
		return key
	}
	return key + ":" + strconv.FormatInt(version, 10)
}

// attendanceCopy renders the copy for a general attendance exception. A status
// that produces no notification is reported as the sentinel that says WHY: an
// absence or a presence is skipped by design (an absence is already notified by
// AbsenceRecorded, and notifying on both would tell the family twice), while
// anything else means attendance's vocabulary and this consumer's have drifted.
func attendanceCopy(status, date string) (title, body string, err error) {
	switch status {
	case statusLate:
		return "Llegada tarde", fmt.Sprintf("Se registró una llegada tarde el %s.", date), nil
	case statusExcused:
		return "Ausencia justificada", fmt.Sprintf("Se registró una ausencia justificada el %s.", date), nil
	case statusAbsent, statusPresent:
		return "", "", fmt.Errorf("%w: attendance status %q", errNotNotifiable, status)
	default:
		return "", "", fmt.Errorf("%w: attendance status %q is outside the contract", errUndeliverable, status)
	}
}

// pesos renders minor units as the amount a family reads. Money crosses every
// boundary in this system as int64 cents, never a float, so the decimal point is
// placed by integer arithmetic here.
func pesos(cents int64) string {
	if cents < 0 {
		return fmt.Sprintf("-$%d.%02d", -cents/100, -cents%100)
	}
	return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
}

// isKnownPlatform reports whether platform is one the contract accepts. The empty
// string is valid: it is the unreported platform.
func isKnownPlatform(platform string) bool {
	switch platform {
	case platformUnknown, platformIOS, platformAndroid, platformWeb:
		return true
	default:
		return false
	}
}
