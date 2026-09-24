package pubsubadapter

import (
	"context"

	"google.golang.org/protobuf/proto"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/pubsub"
	"github.com/garcia-lorca/services/students/internal/adapters/protoconv"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// Compile-time proof the adapter implements the use-case's EventPublisher port.
var _ ports.EventPublisher = (*EventPublisher)(nil)

const (
	// topicStudentEvents is the single per-domain topic; both event types are
	// published to it and distinguished by the "type" attribute (mirrors the
	// finance "payment-events" shape).
	topicStudentEvents = "student-events"

	typeStudentEnrolled        = "StudentEnrolled"
	typeAdmissionStatusChanged = "AdmissionStatusChanged"

	attrType     = "type"
	attrTenantID = "tenant_id"
)

// EventPublisher publishes the students domain events onto the messaging
// boundary. It marshals each domain event to its garcialorca.students.v1 proto
// and hands the bytes to a transport-agnostic pubsub.Publisher.
type EventPublisher struct {
	bus pubsub.Publisher
}

// NewEventPublisher wires the adapter to a concrete transport (Cloud Pub/Sub in
// production, an in-memory bus in tests).
func NewEventPublisher(bus pubsub.Publisher) *EventPublisher {
	return &EventPublisher{bus: bus}
}

// PublishStudentEnrolled emits a StudentEnrolled event.
func (p *EventPublisher) PublishStudentEnrolled(ctx context.Context, ev domain.StudentEnrolledEvent) error {
	return p.publish(ctx, ev.TenantID, typeStudentEnrolled, &studentsv1.StudentEnrolled{
		StudentId: ev.StudentID,
		TenantId:  ev.TenantID,
		CycleId:   ev.CycleID,
		LevelId:   ev.LevelID,
	})
}

// PublishAdmissionChanged emits an AdmissionStatusChanged event.
func (p *EventPublisher) PublishAdmissionChanged(ctx context.Context, ev domain.AdmissionChangedEvent) error {
	return p.publish(ctx, ev.TenantID, typeAdmissionStatusChanged, &studentsv1.AdmissionStatusChanged{
		StudentId:      ev.StudentID,
		TenantId:       ev.TenantID,
		PreviousStatus: protoconv.ToProtoStatus(ev.Previous),
		NewStatus:      protoconv.ToProtoStatus(ev.New),
		Reason:         ev.Reason,
	})
}

// publish marshals the proto event and hands it to the transport with routing
// attributes. A marshal failure is an internal error.
func (p *EventPublisher) publish(ctx context.Context, tenantID, eventType string, msg proto.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "marshal "+eventType+" event", err)
	}
	return p.bus.Publish(ctx, pubsub.Message{
		Topic: topicStudentEvents,
		Data:  data,
		Attributes: map[string]string{
			attrType:     eventType,
			attrTenantID: tenantID,
		},
	})
}
