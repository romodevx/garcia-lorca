package pubsub_test

import (
	"context"
	"errors"
	"testing"

	"github.com/garcia-lorca/pkg/pubsub"
)

const (
	testTopic = "garcialorca.students.v1.enrolled"
	testSub   = "comms-notify"
)

// subscribeAsync runs bus.Subscribe in its own goroutine and returns a channel
// carrying its error, so tests can assert on a graceful shutdown.
func subscribeAsync(ctx context.Context, bus *pubsub.MemoryBus, subscription string, h pubsub.Handler) <-chan error {
	done := make(chan error, 1)
	go func() { done <- bus.Subscribe(ctx, subscription, h) }()
	return done
}

func TestMemoryBusRoundTrip(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := pubsub.NewMemoryBus()
	bus.CreateSubscription(testTopic, testSub)

	sent := pubsub.Message{
		Topic:      testTopic,
		Data:       []byte(`{"student_id":"s-1"}`),
		Attributes: map[string]string{"tenant": "t-1"},
	}
	if err := bus.Publish(ctx, sent); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	received := make(chan pubsub.Message, 1)
	done := subscribeAsync(ctx, bus, testSub, func(_ context.Context, msg pubsub.Message) error {
		received <- msg
		return nil
	})

	got := <-received
	if got.Topic != sent.Topic {
		t.Fatalf("Topic = %q, want %q", got.Topic, sent.Topic)
	}
	if string(got.Data) != string(sent.Data) {
		t.Fatalf("Data = %q, want %q", got.Data, sent.Data)
	}
	if got.Attributes["tenant"] != "t-1" {
		t.Fatalf("Attributes[tenant] = %q, want t-1", got.Attributes["tenant"])
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Subscribe() after cancel error = %v, want nil", err)
	}
}

func TestMemoryBusRedeliversWhenHandlerFails(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := pubsub.NewMemoryBus()
	bus.CreateSubscription(testTopic, testSub)
	if err := bus.Publish(ctx, pubsub.Message{Topic: testTopic, Data: []byte("payload")}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	redelivered := make(chan struct{})
	deliveries := 0
	done := subscribeAsync(ctx, bus, testSub, func(_ context.Context, _ pubsub.Message) error {
		deliveries++
		if deliveries == 2 {
			close(redelivered) // the same message reached the handler twice
		}
		return errors.New("handler failed")
	})

	<-redelivered
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Subscribe() after cancel error = %v, want nil", err)
	}
	if deliveries < 2 {
		t.Fatalf("deliveries = %d, want at least 2", deliveries)
	}
}

func TestMemoryBusStopsRedeliveryAfterHandlerSucceeds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := pubsub.NewMemoryBus()
	bus.CreateSubscription(testTopic, testSub)
	if err := bus.Publish(ctx, pubsub.Message{Topic: testTopic, Data: []byte("first")}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	acked := make(chan struct{})
	order := make(chan string, 8)
	attempts := 0
	done := subscribeAsync(ctx, bus, testSub, func(_ context.Context, msg pubsub.Message) error {
		order <- string(msg.Data)
		attempts++
		if attempts == 1 {
			return errors.New("transient failure")
		}
		if attempts == 2 {
			close(acked) // succeeded on the retry
		}
		return nil
	})

	<-acked
	// A third delivery must be the next published message, never a redelivery
	// of the one the handler already acknowledged.
	if err := bus.Publish(ctx, pubsub.Message{Topic: testTopic, Data: []byte("second")}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	want := []string{"first", "first", "second"}
	for i, w := range want {
		if got := <-order; got != w {
			t.Fatalf("delivery %d = %q, want %q", i+1, got, w)
		}
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Subscribe() after cancel error = %v, want nil", err)
	}
}

func TestMemoryBusFansOutOneCopyPerSubscription(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const (
		notifySub = "comms-notify"
		auditSub  = "audit-log"
		otherSub  = "finance-charge"
	)
	bus := pubsub.NewMemoryBus()
	bus.CreateSubscription(testTopic, notifySub)
	bus.CreateSubscription(testTopic, auditSub)
	bus.CreateSubscription("garcialorca.finance.v1.charged", otherSub)

	if err := bus.Publish(ctx, pubsub.Message{
		Topic:      testTopic,
		Data:       []byte("shared"),
		Attributes: map[string]string{"tenant": "t-1"},
	}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	notify := make(chan pubsub.Message, 1)
	audit := make(chan pubsub.Message, 1)
	other := make(chan pubsub.Message, 1)
	subscribeAsync(ctx, bus, notifySub, func(_ context.Context, msg pubsub.Message) error {
		msg.Data[0] = 'X' // mutating one copy must not affect the others
		msg.Attributes["tenant"] = "mutated"
		notify <- msg
		return nil
	})
	subscribeAsync(ctx, bus, auditSub, func(_ context.Context, msg pubsub.Message) error {
		audit <- msg
		return nil
	})
	subscribeAsync(ctx, bus, otherSub, func(_ context.Context, msg pubsub.Message) error {
		other <- msg
		return nil
	})

	<-notify
	auditCopy := <-audit
	if string(auditCopy.Data) != "shared" {
		t.Fatalf("audit Data = %q, want shared (copies must be independent)", auditCopy.Data)
	}
	if auditCopy.Attributes["tenant"] != "t-1" {
		t.Fatalf("audit Attributes[tenant] = %q, want t-1", auditCopy.Attributes["tenant"])
	}

	select {
	case msg := <-other:
		t.Fatalf("subscription on another topic received %q", msg.Data)
	default:
	}
}

func TestMemoryBusSubscribeUnknownSubscription(t *testing.T) {
	bus := pubsub.NewMemoryBus()

	err := bus.Subscribe(context.Background(), "never-created", func(context.Context, pubsub.Message) error {
		return nil
	})
	if !errors.Is(err, pubsub.ErrSubscriptionNotFound) {
		t.Fatalf("Subscribe() error = %v, want ErrSubscriptionNotFound", err)
	}
}

func TestMemoryBusSubscribeTopicCreatesSubscription(t *testing.T) {
	bus := pubsub.NewMemoryBus()

	// An already-cancelled context makes the consume loop a no-op, leaving only
	// the get-or-create side effect to assert on — no goroutine, no polling.
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	noop := func(context.Context, pubsub.Message) error { return nil }
	if err := bus.SubscribeTopic(cancelled, testTopic, testSub, noop); err != nil {
		t.Fatalf("SubscribeTopic() error = %v, want nil", err)
	}
	if !bus.HasSubscription(testSub) {
		t.Fatal("SubscribeTopic() did not create the subscription")
	}

	// The created subscription is bound to the requested topic, so a later
	// Subscribe on its name receives that topic's messages.
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	if err := bus.Publish(ctx, pubsub.Message{Topic: testTopic, Data: []byte("late")}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	received := make(chan pubsub.Message, 1)
	done := subscribeAsync(ctx, bus, testSub, func(_ context.Context, msg pubsub.Message) error {
		received <- msg
		return nil
	})
	if got := <-received; string(got.Data) != "late" {
		t.Fatalf("Data = %q, want late", got.Data)
	}

	stop()
	if err := <-done; err != nil {
		t.Fatalf("Subscribe() after cancel error = %v, want nil", err)
	}
}

// MemoryBus must satisfy both sides of the messaging boundary.
var (
	_ pubsub.Publisher  = (*pubsub.MemoryBus)(nil)
	_ pubsub.Subscriber = (*pubsub.MemoryBus)(nil)
)
