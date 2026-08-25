// Package pubsubadapter adapts the finance use cases to asynchronous messaging:
// publishing domain events onto the pkg/pubsub boundary. It only translates —
// port event structs → garcialorca.finance.v1 protos → transport messages; the
// use case never learns what the transport is.
package pubsubadapter

import (
	"context"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/pubsub"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// Compile-time proof the adapter implements the use-case's PaymentPublisher port.
var _ ports.PaymentPublisher = (*EventPublisher)(nil)

const (
	// topicPaymentEvents is the single per-domain topic; event types are
	// distinguished by the "type" attribute (mirrors students' "student-events").
	topicPaymentEvents = "payment-events"

	typePaymentRegistered = "PaymentRegistered"
	typeChargeSettled     = "ChargeSettled"

	attrType     = "type"
	attrTenantID = "tenant_id"
)

// EventPublisher publishes the finance domain events onto the messaging
// boundary. It marshals each event to its garcialorca.finance.v1 proto and hands
// the bytes to a transport-agnostic pubsub.Publisher.
type EventPublisher struct {
	bus pubsub.Publisher
}

// NewEventPublisher wires the adapter to a concrete transport (Cloud Pub/Sub in
// production, an in-memory bus in tests).
func NewEventPublisher(bus pubsub.Publisher) *EventPublisher {
	return &EventPublisher{bus: bus}
}

// PublishPaymentRegistered announces that money was collected against a charge.
// Consumers (comms for the receipt, scholarships for its own accounting) react
// to it; finance does not wait on them.
func (p *EventPublisher) PublishPaymentRegistered(ctx context.Context, e ports.PaymentRegisteredEvent) error {
	return p.publish(ctx, e.TenantID, typePaymentRegistered, &financev1.PaymentRegistered{
		PaymentId:   e.PaymentID,
		StudentId:   e.StudentID,
		ChargeId:    e.ChargeID,
		TenantId:    e.TenantID,
		AmountCents: e.AmountCents,
		Method:      e.Method,
		PaidAt:      timestamppb.New(e.PaidAt),
	})
}

// PublishChargeSettled announces that a charge is now fully covered. It rides
// the same per-domain topic as PaymentRegistered and is told apart by the "type"
// attribute, so a consumer that only wants settlements filters on it instead of
// summing payments.
func (p *EventPublisher) PublishChargeSettled(ctx context.Context, e ports.ChargeSettledEvent) error {
	return p.publish(ctx, e.TenantID, typeChargeSettled, &financev1.ChargeSettled{
		ChargeId:    e.ChargeID,
		StudentId:   e.StudentID,
		TenantId:    e.TenantID,
		ExternalRef: e.ExternalRef,
		AmountCents: e.AmountCents,
		SettledAt:   timestamppb.New(e.SettledAt),
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
		Topic: topicPaymentEvents,
		Data:  data,
		Attributes: map[string]string{
			attrType:     eventType,
			attrTenantID: tenantID,
		},
	})
}
