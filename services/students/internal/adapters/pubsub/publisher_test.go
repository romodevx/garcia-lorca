package pubsubadapter_test

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/pkg/pubsub"
	"github.com/garcia-lorca/services/students/internal/adapters/pubsub"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// fakeBus captures every published message for assertion.
type fakeBus struct{ msgs []pubsub.Message }

func (b *fakeBus) Publish(_ context.Context, msg pubsub.Message) error {
	b.msgs = append(b.msgs, msg)
	return nil
}

func TestPublishStudentEnrolledMarshalsProtoToStudentEventsTopic(t *testing.T) {
	bus := &fakeBus{}
	pub := pubsubadapter.NewEventPublisher(bus)

	err := pub.PublishStudentEnrolled(context.Background(), domain.StudentEnrolledEvent{
		StudentID: "stu-1",
		TenantID:  "t1",
		CycleID:   "2026-2027",
		LevelID:   "lvl-primaria",
	})
	if err != nil {
		t.Fatalf("PublishStudentEnrolled: %v", err)
	}

	if len(bus.msgs) != 1 {
		t.Fatalf("published %d messages, want 1", len(bus.msgs))
	}
	msg := bus.msgs[0]
	if msg.Topic != "student-events" {
		t.Errorf("topic = %q, want student-events", msg.Topic)
	}
	if msg.Attributes["type"] != "StudentEnrolled" {
		t.Errorf("type attr = %q", msg.Attributes["type"])
	}
	if msg.Attributes["tenant_id"] != "t1" {
		t.Errorf("tenant_id attr = %q", msg.Attributes["tenant_id"])
	}

	var got studentsv1.StudentEnrolled
	if err := proto.Unmarshal(msg.Data, &got); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if got.GetStudentId() != "stu-1" || got.GetCycleId() != "2026-2027" {
		t.Errorf("payload = %+v", &got)
	}
	if got.GetLevelId() != "lvl-primaria" {
		t.Errorf("level_id = %q", got.GetLevelId())
	}
}

func TestPublishAdmissionChangedMarshalsProtoToStudentEventsTopic(t *testing.T) {
	bus := &fakeBus{}
	pub := pubsubadapter.NewEventPublisher(bus)

	err := pub.PublishAdmissionChanged(context.Background(), domain.AdmissionChangedEvent{
		StudentID: "stu-1",
		TenantID:  "t1",
		Previous:  domain.AdmissionPending,
		New:       domain.AdmissionRejected,
		Reason:    "incomplete documents",
	})
	if err != nil {
		t.Fatalf("PublishAdmissionChanged: %v", err)
	}

	if len(bus.msgs) != 1 {
		t.Fatalf("published %d messages, want 1", len(bus.msgs))
	}
	msg := bus.msgs[0]
	if msg.Topic != "student-events" {
		t.Errorf("topic = %q, want student-events", msg.Topic)
	}
	if msg.Attributes["type"] != "AdmissionStatusChanged" {
		t.Errorf("type attr = %q", msg.Attributes["type"])
	}

	var got studentsv1.AdmissionStatusChanged
	if err := proto.Unmarshal(msg.Data, &got); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if got.GetPreviousStatus() != studentsv1.AdmissionStatus_ADMISSION_STATUS_PENDING {
		t.Errorf("previous = %v", got.GetPreviousStatus())
	}
	if got.GetNewStatus() != studentsv1.AdmissionStatus_ADMISSION_STATUS_REJECTED {
		t.Errorf("new = %v", got.GetNewStatus())
	}
	if got.GetReason() != "incomplete documents" {
		t.Errorf("reason = %q, want %q", got.GetReason(), "incomplete documents")
	}
}

func TestPublishOutboxEventPreservesStudentWithdrawnRouting(t *testing.T) {
	bus := &fakeBus{}
	pub := pubsubadapter.NewEventPublisher(bus)
	event := &studentsv1.StudentWithdrawn{TenantId: "tenant-a", StudentId: "student-a", CycleId: "cycle-a", EnrollmentId: "enrollment-a", WithdrawnOn: "2026-10-01"}
	payload, err := proto.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if err := pub.PublishOutboxEvent(context.Background(), ports.OutboxEvent{TenantID: "tenant-a", Type: "StudentWithdrawn", Payload: payload}); err != nil {
		t.Fatalf("PublishOutboxEvent: %v", err)
	}
	if len(bus.msgs) != 1 {
		t.Fatalf("published %d messages, want one", len(bus.msgs))
	}
	msg := bus.msgs[0]
	if msg.Topic != "student-events" || msg.Attributes["type"] != "StudentWithdrawn" || msg.Attributes["tenant_id"] != "tenant-a" {
		t.Fatalf("routing = %q %v", msg.Topic, msg.Attributes)
	}
	var got studentsv1.StudentWithdrawn
	if err := proto.Unmarshal(msg.Data, &got); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(&got, event) {
		t.Fatalf("payload = %v, want %v", &got, event)
	}
}

func TestPublishOutboxEventRelaysAdmissionStatusChanged(t *testing.T) {
	bus := &fakeBus{}
	pub := pubsubadapter.NewEventPublisher(bus)
	event := &studentsv1.AdmissionStatusChanged{
		StudentId: "student-a", TenantId: "tenant-a",
		PreviousStatus: studentsv1.AdmissionStatus_ADMISSION_STATUS_ENROLLED,
		NewStatus:      studentsv1.AdmissionStatus_ADMISSION_STATUS_WITHDRAWN,
		Reason:         "moved",
	}
	payload, err := proto.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if err := pub.PublishOutboxEvent(context.Background(), ports.OutboxEvent{TenantID: "tenant-a", Type: "AdmissionStatusChanged", Payload: payload}); err != nil {
		t.Fatalf("PublishOutboxEvent: %v", err)
	}
	if len(bus.msgs) != 1 {
		t.Fatalf("published %d messages, want one", len(bus.msgs))
	}
	msg := bus.msgs[0]
	if msg.Topic != "student-events" || msg.Attributes["type"] != "AdmissionStatusChanged" || msg.Attributes["tenant_id"] != "tenant-a" {
		t.Fatalf("routing = %q %v", msg.Topic, msg.Attributes)
	}
	var got studentsv1.AdmissionStatusChanged
	if err := proto.Unmarshal(msg.Data, &got); err != nil {
		t.Fatal(err)
	}
	if got.GetStudentId() != event.GetStudentId() || got.GetPreviousStatus() != event.GetPreviousStatus() || got.GetNewStatus() != event.GetNewStatus() || got.GetReason() != event.GetReason() {
		t.Fatalf("payload = %+v", &got)
	}
}

// T9: StudentEnrolled carries when the place starts (the proto field existed
// and was never set); an event with no instant leaves it absent, not 1970.
func TestPublishStudentEnrolledCarriesEnrolledAt(t *testing.T) {
	at := time.Date(2026, time.August, 24, 6, 0, 0, 0, time.UTC) // 00:00 Morelia
	for _, tc := range []struct {
		name string
		at   time.Time
	}{{"set", at}, {"absent", time.Time{}}} {
		t.Run(tc.name, func(t *testing.T) {
			bus := &fakeBus{}
			pub := pubsubadapter.NewEventPublisher(bus)
			if err := pub.PublishStudentEnrolled(context.Background(), domain.StudentEnrolledEvent{
				StudentID: "stu-1", TenantID: "t1", CycleID: "cyc", EnrolledAt: tc.at,
			}); err != nil {
				t.Fatalf("publish: %v", err)
			}
			var got studentsv1.StudentEnrolled
			if err := proto.Unmarshal(bus.msgs[0].Data, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if tc.at.IsZero() {
				if got.GetEnrolledAt() != nil {
					t.Errorf("enrolled_at = %v, want absent", got.GetEnrolledAt())
				}
				return
			}
			if !got.GetEnrolledAt().AsTime().Equal(tc.at) {
				t.Errorf("enrolled_at = %v, want %s", got.GetEnrolledAt().AsTime(), tc.at)
			}
		})
	}
}
