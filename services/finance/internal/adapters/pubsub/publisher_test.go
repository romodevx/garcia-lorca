package pubsubadapter_test

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	"github.com/garcia-lorca/pkg/pubsub"
	pubsubadapter "github.com/garcia-lorca/services/finance/internal/adapters/pubsub"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// fakeBus captures every published message for assertion.
type fakeBus struct{ msgs []pubsub.Message }

func (b *fakeBus) Publish(_ context.Context, msg pubsub.Message) error {
	b.msgs = append(b.msgs, msg)
	return nil
}

func TestPublishPaymentRegisteredMarshalsProtoToPaymentEventsTopic(t *testing.T) {
	paidAt := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	bus := &fakeBus{}
	pub := pubsubadapter.NewEventPublisher(bus)

	err := pub.PublishPaymentRegistered(context.Background(), ports.PaymentRegisteredEvent{
		PaymentID:   "pay-1",
		StudentID:   "stu-1",
		ChargeID:    "chg-1",
		TenantID:    "t1",
		Method:      "cash",
		AmountCents: 100000,
		PaidAt:      paidAt,
	})
	if err != nil {
		t.Fatalf("PublishPaymentRegistered: %v", err)
	}

	if len(bus.msgs) != 1 {
		t.Fatalf("published %d messages, want 1", len(bus.msgs))
	}
	msg := bus.msgs[0]
	if msg.Topic != "payment-events" {
		t.Errorf("topic = %q, want payment-events", msg.Topic)
	}
	if msg.Attributes["type"] != "PaymentRegistered" {
		t.Errorf("type attr = %q", msg.Attributes["type"])
	}
	// Routing by tenant is what keeps a consumer from fanning one school's
	// receipts out to another.
	if msg.Attributes["tenant_id"] != "t1" {
		t.Errorf("tenant_id attr = %q", msg.Attributes["tenant_id"])
	}

	var got financev1.PaymentRegistered
	if err := proto.Unmarshal(msg.Data, &got); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if got.GetPaymentId() != "pay-1" || got.GetStudentId() != "stu-1" || got.GetChargeId() != "chg-1" {
		t.Errorf("payload ids = %+v", &got)
	}
	if got.GetAmountCents() != 100000 || got.GetMethod() != "cash" {
		t.Errorf("payload amount/method = %+v", &got)
	}
	if !got.GetPaidAt().AsTime().Equal(paidAt) {
		t.Errorf("paid_at = %v, want %v", got.GetPaidAt().AsTime(), paidAt)
	}
}

// TestPublishChargeSettled proves the settled event carries what a consumer
// needs to close its own row without calling back into finance: the external
// reference it created the charge with, and nothing it would have to derive.
func TestPublishChargeSettled(t *testing.T) {
	bus := &fakeBus{}
	p := pubsubadapter.NewEventPublisher(bus)

	settledAt := time.Date(2026, 9, 12, 10, 30, 0, 0, time.UTC)
	err := p.PublishChargeSettled(context.Background(), ports.ChargeSettledEvent{
		ChargeID:    "ch-1",
		StudentID:   "stu-1",
		TenantID:    "t-1",
		ExternalRef: "uniforms:ful-42",
		AmountCents: 89000,
		SettledAt:   settledAt,
	})
	if err != nil {
		t.Fatalf("PublishChargeSettled: %v", err)
	}
	if len(bus.msgs) != 1 {
		t.Fatalf("published %d messages, want 1", len(bus.msgs))
	}
	msg := bus.msgs[0]
	if msg.Attributes["type"] != "ChargeSettled" {
		t.Errorf("type attribute = %q, want ChargeSettled", msg.Attributes["type"])
	}
	if msg.Attributes["tenant_id"] != "t-1" {
		t.Errorf("tenant_id attribute = %q, want t-1", msg.Attributes["tenant_id"])
	}
	var got financev1.ChargeSettled
	if err := proto.Unmarshal(msg.Data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.GetChargeId() != "ch-1" || got.GetExternalRef() != "uniforms:ful-42" ||
		got.GetAmountCents() != 89000 || !got.GetSettledAt().AsTime().Equal(settledAt) {
		t.Errorf("event mismatch: %+v", &got)
	}
}
