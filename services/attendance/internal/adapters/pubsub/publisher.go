// Package pubsubadapter adapts the attendance service to asynchronous
// messaging: publishing its events onto the pkg/pubsub boundary. It only
// translates — service event structs → garcialorca.attendance.v1 protos →
// transport messages; the service never learns what the transport is.
package pubsubadapter

import (
	"context"

	"google.golang.org/protobuf/proto"

	attendancev1 "github.com/garcia-lorca/gen/go/garcialorca/attendance/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/pubsub"
	"github.com/garcia-lorca/services/attendance/internal/service"
)

// Compile-time proof the adapter implements the service's Publisher seam.
var _ service.Publisher = (*EventPublisher)(nil)

const (
	// topicAttendanceEvents is the single per-domain topic; event types are
	// distinguished by the "type" attribute (mirrors finance's "payment-events").
	topicAttendanceEvents = "attendance-events"

	typeAttendanceMarked = "AttendanceMarked"
	typeAbsenceRecorded  = "AbsenceRecorded"

	attrType     = "type"
	attrTenantID = "tenant_id"
)

// EventPublisher publishes the attendance events onto the messaging boundary. It
// marshals each event to its garcialorca.attendance.v1 proto and hands the bytes
// to a transport-agnostic pubsub.Publisher.
type EventPublisher struct {
	bus pubsub.Publisher
}

// NewEventPublisher wires the adapter to a concrete transport (Cloud Pub/Sub in
// production, an in-memory bus in tests).
func NewEventPublisher(bus pubsub.Publisher) *EventPublisher {
	return &EventPublisher{bus: bus}
}

// PublishAttendanceMarked announces that an exception (absent|late|excused) was
// stored for a student on a school day. It is the general event; absences also
// get the narrower AbsenceRecorded below.
func (p *EventPublisher) PublishAttendanceMarked(ctx context.Context, e service.MarkedEvent) error {
	return p.publish(ctx, e.TenantID, typeAttendanceMarked, &attendancev1.AttendanceMarked{
		RecordId:     e.RecordID,
		EnrollmentId: e.EnrollmentID,
		StudentId:    e.StudentID,
		TenantId:     e.TenantID,
		Date:         e.Date,
		Status:       e.Status,
	})
}

// PublishAbsenceRecorded announces that a student was absent on a school day.
// Comms (M10) listens for it to notify the family; attendance does not wait on
// any consumer.
func (p *EventPublisher) PublishAbsenceRecorded(ctx context.Context, e service.AbsenceEvent) error {
	return p.publish(ctx, e.TenantID, typeAbsenceRecorded, &attendancev1.AbsenceRecorded{
		RecordId:     e.RecordID,
		EnrollmentId: e.EnrollmentID,
		StudentId:    e.StudentID,
		TenantId:     e.TenantID,
		Date:         e.Date,
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
		Topic: topicAttendanceEvents,
		Data:  data,
		Attributes: map[string]string{
			attrType:     eventType,
			attrTenantID: tenantID,
		},
	})
}
