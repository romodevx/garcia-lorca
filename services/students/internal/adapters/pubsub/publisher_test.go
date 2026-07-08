package pubsubadapter_test

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	"github.com/garcia-lorca/pkg/pubsub"
	"github.com/garcia-lorca/services/students/internal/adapters/pubsub"
	"github.com/garcia-lorca/services/students/internal/domain"
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
		Level:     domain.LevelPrimaria,
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
	if got.GetLevel() != studentsv1.Level_LEVEL_PRIMARIA {
		t.Errorf("level = %v", got.GetLevel())
	}
}

func TestPublishAdmissionChangedMarshalsProtoToStudentEventsTopic(t *testing.T) {
	bus := &fakeBus{}
	pub := pubsubadapter.NewEventPublisher(bus)

	err := pub.PublishAdmissionChanged(context.Background(), domain.AdmissionChangedEvent{
		StudentID: "stu-1",
		TenantID:  "t1",
		Previous:  domain.AdmissionPending,
		New:       domain.AdmissionAccepted,
		Reason:    "",
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
	if got.GetNewStatus() != studentsv1.AdmissionStatus_ADMISSION_STATUS_ACCEPTED {
		t.Errorf("new = %v", got.GetNewStatus())
	}
}
