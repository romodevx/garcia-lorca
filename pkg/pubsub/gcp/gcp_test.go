package gcp_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/garcia-lorca/pkg/pubsub"
	"github.com/garcia-lorca/pkg/pubsub/gcp"
)

// The adapter is pure I/O, so it is exercised against the Cloud Pub/Sub
// emulator rather than mocks. Tests skip when PUBSUB_EMULATOR_HOST is unset.
//
// With the gcloud SDK (needs the pubsub-emulator component and a JRE):
//
//	gcloud beta emulators pubsub start --project=garcia-lorca-test
//	export PUBSUB_EMULATOR_HOST=localhost:8085
//
// Or with Docker, which needs neither:
//
//	docker run -d --rm -p 8681:8681 thekevjames/gcloud-pubsub-emulator:latest
//	export PUBSUB_EMULATOR_HOST=localhost:8681
const emulatorProject = "garcia-lorca-test"

func newEmulatorClient(t *testing.T, ctx context.Context) *gcp.Client {
	t.Helper()
	if os.Getenv("PUBSUB_EMULATOR_HOST") == "" {
		t.Skip("PUBSUB_EMULATOR_HOST not set; skipping Cloud Pub/Sub emulator test")
	}
	client, err := gcp.New(ctx, emulatorProject)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return client
}

func TestClientRoundTripAcksOnSuccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := newEmulatorClient(t, ctx)

	const (
		topic = "garcialorca.students.v1.enrolled"
		sub   = "gcp-test-roundtrip"
	)
	received := make(chan pubsub.Message, 1)
	consume, stop := context.WithCancel(ctx)
	defer stop()
	done := make(chan error, 1)
	go func() {
		done <- client.SubscribeTopic(consume, topic, sub, func(_ context.Context, msg pubsub.Message) error {
			received <- msg
			return nil
		})
	}()

	// SubscribeTopic creates topic and subscription; publishing to the same
	// topic is a create-or-get too, so it is safe to publish immediately, but
	// only messages published after the subscription exists are delivered.
	// Retry the publish until the handler sees one.
	publish := time.NewTicker(200 * time.Millisecond)
	defer publish.Stop()
	var got pubsub.Message
wait:
	for {
		select {
		case <-publish.C:
			if err := client.Publish(ctx, pubsub.Message{
				Topic:      topic,
				Data:       []byte(`{"student_id":"s-1"}`),
				Attributes: map[string]string{"tenant": "t-1"},
			}); err != nil {
				t.Fatalf("Publish() error = %v", err)
			}
		case got = <-received:
			break wait
		case err := <-done:
			t.Fatalf("SubscribeTopic() returned early: %v", err)
		case <-ctx.Done():
			t.Fatal("timed out waiting for delivery")
		}
	}

	if got.Topic != topic {
		t.Fatalf("Topic = %q, want %q", got.Topic, topic)
	}
	if got.Attributes["tenant"] != "t-1" {
		t.Fatalf("Attributes[tenant] = %q, want t-1", got.Attributes["tenant"])
	}

	stop()
	if err := <-done; err != nil {
		t.Fatalf("SubscribeTopic() after cancel error = %v, want nil", err)
	}
}

func TestClientNacksOnHandlerError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client := newEmulatorClient(t, ctx)

	const (
		topic = "garcialorca.students.v1.transferred"
		sub   = "gcp-test-nack"
	)
	consume, stop := context.WithCancel(ctx)
	defer stop()

	redelivered := make(chan struct{})
	deliveries := make(chan string, 8)
	done := make(chan error, 1)
	go func() {
		seen := 0
		done <- client.SubscribeTopic(consume, topic, sub, func(_ context.Context, msg pubsub.Message) error {
			deliveries <- string(msg.Data)
			seen++
			if seen == 2 {
				close(redelivered)
			}
			return errors.New("handler failed")
		})
	}()

	publish := time.NewTicker(500 * time.Millisecond)
	defer publish.Stop()
	published := false
	for {
		select {
		case <-publish.C:
			if published {
				continue // one message is enough; redelivery must supply the rest
			}
			if err := client.Publish(ctx, pubsub.Message{Topic: topic, Data: []byte("payload")}); err != nil {
				t.Fatalf("Publish() error = %v", err)
			}
		case <-deliveries:
			published = true
		case <-redelivered:
			stop()
			if err := <-done; err != nil {
				t.Fatalf("SubscribeTopic() after cancel error = %v, want nil", err)
			}
			return
		case <-ctx.Done():
			t.Fatal("timed out waiting for redelivery of the nacked message")
		}
	}
}

func TestClientSubscribeUnknownSubscription(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := newEmulatorClient(t, ctx)

	err := client.Subscribe(ctx, "gcp-test-never-created", func(context.Context, pubsub.Message) error {
		return nil
	})
	if !errors.Is(err, pubsub.ErrSubscriptionNotFound) {
		t.Fatalf("Subscribe() error = %v, want ErrSubscriptionNotFound", err)
	}
}

// The adapter must satisfy both sides of the messaging boundary.
var (
	_ pubsub.Publisher  = (*gcp.Client)(nil)
	_ pubsub.Subscriber = (*gcp.Client)(nil)
)

// A subscription that disappears under a running consumer is recreated and the
// consumer carries on.
//
// This is not hypothetical: restarting the local emulator wipes every
// subscription, and both finance and students died on it — the services stayed
// up as containers with a dead process inside, so the school's demo showed zero
// pupils until somebody restarted them by hand. dev runs demos every day; a
// broker blip must cost a reconnection, not an outage.
func TestSubscribeTopicRecoversFromALostSubscription(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client := newEmulatorClient(t, ctx)

	const (
		topic = "garcialorca.students.v1.enrolled"
		sub   = "gcp-test-recovery"
	)
	received := make(chan pubsub.Message, 2)
	consume, stop := context.WithCancel(ctx)
	defer stop()
	done := make(chan error, 1)
	go func() {
		done <- client.SubscribeTopic(consume, topic, sub, func(_ context.Context, msg pubsub.Message) error {
			received <- msg
			return nil
		})
	}()

	// It is consuming.
	if err := client.Publish(ctx, pubsub.Message{Topic: topic, Data: []byte("before")}); err != nil {
		t.Fatalf("Publish(before): %v", err)
	}
	waitForMessage(t, received, "before")

	// The subscription vanishes underneath it, exactly as an emulator restart
	// does.
	if err := client.DeleteSubscription(ctx, sub); err != nil {
		t.Fatalf("DeleteSubscription: %v", err)
	}

	// And the consumer comes back on its own.
	deadline := time.After(45 * time.Second)
	for {
		if err := client.Publish(ctx, pubsub.Message{Topic: topic, Data: []byte("after")}); err != nil {
			t.Fatalf("Publish(after): %v", err)
		}
		select {
		case msg := <-received:
			if string(msg.Data) == "after" {
				stop()
				<-done
				return
			}
		case <-time.After(2 * time.Second):
		case <-deadline:
			t.Fatal("the consumer never recovered from losing its subscription")
		}
	}
}

// waitForMessage reads one message and asserts its body.
func waitForMessage(t *testing.T, ch <-chan pubsub.Message, want string) {
	t.Helper()
	select {
	case msg := <-ch:
		if string(msg.Data) != want {
			t.Fatalf("message = %q, want %q", msg.Data, want)
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("no message %q arrived", want)
	}
}
