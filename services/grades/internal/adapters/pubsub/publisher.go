package pubsubadapter

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/pubsub"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

// Compile-time proof the adapter implements the use case's publisher seam.
var _ ports.ReportCardPublisher = (*EventPublisher)(nil)

const (
	// topicGradesEvents is the grades domain's single topic; event types are
	// told apart by the "type" attribute (like payment-events, attendance-events).
	topicGradesEvents = "grades-events"

	typeGradesPublished = "GradesPublished"
	attrTenantID        = "tenant_id"
)

// EventPublisher publishes the grades events onto the messaging boundary: it
// marshals each to its garcialorca.grades.v1 proto and hands the bytes to a
// transport-agnostic pubsub.Publisher.
type EventPublisher struct {
	bus pubsub.Publisher
}

// NewEventPublisher wires the adapter to a transport (Cloud Pub/Sub in
// production, an in-memory bus in tests).
func NewEventPublisher(bus pubsub.Publisher) *EventPublisher {
	return &EventPublisher{bus: bus}
}

// PublishReportCardIssued announces an issued report card (T7): comms turns it
// into the family's "boleta disponible / actualizada" notification.
func (p *EventPublisher) PublishReportCardIssued(ctx context.Context, rc domain.ReportCard) error {
	h := rc.Header()
	data, err := proto.Marshal(&gradesv1.GradesPublished{
		StudentId: h.StudentID, CycleId: h.CycleID, TenantId: h.TenantID, ReportCardId: h.ID,
		TermId: h.TermID, TermName: h.TermName, Version: int32(h.Version), EnrollmentId: h.EnrollmentID,
	})
	if err != nil {
		return glerrors.Wrap(glerrors.KindInternal, "marshal "+typeGradesPublished+" event", err)
	}
	if err := p.bus.Publish(ctx, pubsub.Message{
		Topic: topicGradesEvents,
		Data:  data,
		Attributes: map[string]string{
			pubsub.AttrType: typeGradesPublished,
			attrTenantID:    h.TenantID,
		},
	}); err != nil {
		return fmt.Errorf("publishing report card %s: %w", h.ID, err)
	}
	return nil
}
