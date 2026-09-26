package pubsubadapter_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	attendancev1 "github.com/garcia-lorca/gen/go/garcialorca/attendance/v1"
	"github.com/garcia-lorca/pkg/pubsub"
	pubsubadapter "github.com/garcia-lorca/services/attendance/internal/adapters/pubsub"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

// fakeBus captures every published message for assertion; err forces a
// transport failure.
type fakeBus struct {
	msgs []pubsub.Message
	err  error
}

func (b *fakeBus) Publish(_ context.Context, msg pubsub.Message) error {
	if b.err != nil {
		return b.err
	}
	b.msgs = append(b.msgs, msg)
	return nil
}

func TestPublishAttendanceMarkedMarshalsProtoToAttendanceEventsTopic(t *testing.T) {
	bus := &fakeBus{}
	pub := pubsubadapter.NewEventPublisher(bus)

	err := pub.PublishAttendanceMarked(context.Background(), service.MarkedEvent{
		RecordID:     "rec-1",
		EnrollmentID: "enr-1",
		StudentID:    "stu-1",
		TenantID:     "t1",
		Date:         "2026-09-15",
		Status:       "late",
		SubjectID:    "sub-1",
		MarkVersion:  3,
	})
	if err != nil {
		t.Fatalf("PublishAttendanceMarked: %v", err)
	}

	if len(bus.msgs) != 1 {
		t.Fatalf("published %d messages, want 1", len(bus.msgs))
	}
	msg := bus.msgs[0]
	if msg.Topic != "attendance-events" {
		t.Errorf("topic = %q, want attendance-events", msg.Topic)
	}
	if msg.Attributes["type"] != "AttendanceMarked" {
		t.Errorf("type attr = %q", msg.Attributes["type"])
	}
	// Routing by tenant is what keeps one school's absences from reaching
	// another school's consumers.
	if msg.Attributes["tenant_id"] != "t1" {
		t.Errorf("tenant_id attr = %q", msg.Attributes["tenant_id"])
	}

	var got attendancev1.AttendanceMarked
	if err := proto.Unmarshal(msg.Data, &got); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if got.GetRecordId() != "rec-1" || got.GetEnrollmentId() != "enr-1" || got.GetStudentId() != "stu-1" {
		t.Errorf("payload ids = %+v", &got)
	}
	if got.GetTenantId() != "t1" || got.GetDate() != "2026-09-15" || got.GetStatus() != "late" || got.GetSubjectId() != "sub-1" || got.GetMarkVersion() != 3 {
		t.Errorf("payload body = %+v", &got)
	}
}

func TestPublishAbsenceRecordedMarshalsProtoToAttendanceEventsTopic(t *testing.T) {
	bus := &fakeBus{}
	pub := pubsubadapter.NewEventPublisher(bus)

	err := pub.PublishAbsenceRecorded(context.Background(), service.AbsenceEvent{
		RecordID:     "rec-2",
		EnrollmentID: "enr-2",
		StudentID:    "stu-2",
		TenantID:     "t1",
		Date:         "2026-09-16",
		SubjectID:    "sub-2",
		MarkVersion:  4,
	})
	if err != nil {
		t.Fatalf("PublishAbsenceRecorded: %v", err)
	}

	if len(bus.msgs) != 1 {
		t.Fatalf("published %d messages, want 1", len(bus.msgs))
	}
	msg := bus.msgs[0]
	// Both event types share the per-domain topic and are told apart by the
	// "type" attribute, so a consumer subscribes once.
	if msg.Topic != "attendance-events" {
		t.Errorf("topic = %q, want attendance-events", msg.Topic)
	}
	if msg.Attributes["type"] != "AbsenceRecorded" {
		t.Errorf("type attr = %q", msg.Attributes["type"])
	}
	if msg.Attributes["tenant_id"] != "t1" {
		t.Errorf("tenant_id attr = %q", msg.Attributes["tenant_id"])
	}

	var got attendancev1.AbsenceRecorded
	if err := proto.Unmarshal(msg.Data, &got); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if got.GetRecordId() != "rec-2" || got.GetEnrollmentId() != "enr-2" || got.GetStudentId() != "stu-2" {
		t.Errorf("payload ids = %+v", &got)
	}
	if got.GetTenantId() != "t1" || got.GetDate() != "2026-09-16" || got.GetSubjectId() != "sub-2" || got.GetMarkVersion() != 4 {
		t.Errorf("payload body = %+v", &got)
	}
}

func TestPublishPropagatesTransportFailure(t *testing.T) {
	wantErr := errors.New("bus down")
	pub := pubsubadapter.NewEventPublisher(&fakeBus{err: wantErr})

	err := pub.PublishAbsenceRecorded(context.Background(), service.AbsenceEvent{RecordID: "rec-3", TenantID: "t1"})
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want it to wrap %v", err, wantErr)
	}
}
