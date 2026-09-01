// Package gcp adapts Cloud Pub/Sub to the pubsub.Publisher / pubsub.Subscriber
// boundary. It is the only place in the monorepo that knows the transport is
// Google's — services depend on the interfaces in pkg/pubsub.
//
// The emulator needs no special wiring: the Google client library reads
// PUBSUB_EMULATOR_HOST itself, so New talks to the emulator whenever that
// variable is set and to the real service otherwise.
//
// Topics and subscriptions are created on demand (get-or-create). That keeps
// local and CI runs against a blank emulator working; in shared environments
// the resources already exist and the Exists check short-circuits.
package gcp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	gcppubsub "cloud.google.com/go/pubsub"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/garcia-lorca/pkg/pubsub"
)

// ackDeadline is how long a handler has to finish before Cloud Pub/Sub
// considers the message unacknowledged. Handlers do database work, so the
// library's 10s default is tight; Receive keeps extending it while the handler
// runs, up to ReceiveSettings.MaxExtension.
const ackDeadline = 30 * time.Second

const (
	// maxResubscribes bounds the recovery from a vanished subscription. Enough
	// that an emulator restart or a deleted subscription is absorbed; small
	// enough that a genuinely broken configuration still stops and says so
	// rather than spinning forever.
	maxResubscribes = 10
	// resubscribeDelay gives the broker a moment to come back before trying
	// again. Seconds, because the thing on the other side is usually restarting.
	resubscribeDelay = 2 * time.Second
)

// Client publishes to and consumes from Cloud Pub/Sub. It is safe for
// concurrent use, and callers must Close it to release the publisher goroutines
// each topic keeps for batching.
type Client struct {
	client *gcppubsub.Client

	mu     sync.Mutex
	topics map[string]*gcppubsub.Topic
}

// New connects to the project's Cloud Pub/Sub — or to the emulator when
// PUBSUB_EMULATOR_HOST is set.
func New(ctx context.Context, projectID string) (*Client, error) {
	client, err := gcppubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("pubsub/gcp: new client for project %q: %w", projectID, err)
	}
	return &Client{client: client, topics: make(map[string]*gcppubsub.Topic)}, nil
}

// Publish sends msg to its topic and blocks until the server assigns it an id,
// so a returned nil error means the event is durably accepted.
func (c *Client) Publish(ctx context.Context, msg pubsub.Message) error {
	topic, err := c.topic(ctx, msg.Topic)
	if err != nil {
		return err
	}

	result := topic.Publish(ctx, &gcppubsub.Message{Data: msg.Data, Attributes: msg.Attributes})
	if _, err := result.Get(ctx); err != nil {
		return fmt.Errorf("pubsub/gcp: publish to %q: %w", msg.Topic, err)
	}
	return nil
}

// Subscribe consumes an existing subscription, dispatching each message to h
// until ctx is cancelled — then it returns nil, so a graceful shutdown is not
// an error. A handler returning nil acks the message; returning an error nacks
// it for redelivery (at-least-once, so handlers must be idempotent).
//
// The subscription's topic is read from its server-side config, which is what
// fills pubsub.Message.Topic. Subscribe does not create anything: an unknown
// subscription returns pubsub.ErrSubscriptionNotFound. Use SubscribeTopic to
// create-then-consume.
func (c *Client) Subscribe(ctx context.Context, subscription string, h pubsub.Handler) error {
	sub := c.client.Subscription(subscription)
	exists, err := sub.Exists(ctx)
	if err != nil {
		return fmt.Errorf("pubsub/gcp: check subscription %q: %w", subscription, err)
	}
	if !exists {
		return fmt.Errorf("pubsub/gcp: subscribe to %q: %w", subscription, pubsub.ErrSubscriptionNotFound)
	}

	cfg, err := sub.Config(ctx)
	if err != nil {
		return fmt.Errorf("pubsub/gcp: config of subscription %q: %w", subscription, err)
	}
	topic := ""
	if cfg.Topic != nil {
		topic = cfg.Topic.ID()
	}
	return c.consume(ctx, topic, subscription, h)
}

// DeleteSubscription removes a subscription. It exists for tests that need to
// simulate one disappearing under a running consumer, which is what an emulator
// restart does.
func (c *Client) DeleteSubscription(ctx context.Context, subscription string) error {
	if err := c.client.Subscription(subscription).Delete(ctx); err != nil {
		return fmt.Errorf("pubsub/gcp: delete subscription %q: %w", subscription, err)
	}
	return nil
}

// consume receives until the caller's context ends, recreating the subscription
// if it disappears underneath.
//
// A subscription CAN vanish: restarting the emulator wipes every one of them,
// and a person or a script can delete one in a real project. When that happened,
// Receive returned NotFound, the service treated it as fatal and exited — and
// because the dev container keeps running with a dead process inside, the school
// saw a module that answered nothing until somebody restarted it by hand. dev
// runs demos every day; a broker blip has to cost a reconnection, not an outage.
//
// Only NotFound is retried, and only so many times. Anything else — a permission
// error, a broken config — is still fatal, because a service that quietly
// consumes nothing forever is worse than one that stops and says why.
func (c *Client) consume(ctx context.Context, topic, subscription string, h pubsub.Handler) error {
	for attempt := 0; ; attempt++ {
		sub, err := c.ensureSubscription(ctx, topic, subscription)
		if err != nil {
			return err
		}
		err = c.receive(ctx, sub, topic, h)
		if err == nil || ctx.Err() != nil {
			// A clean stop, or the caller asked us to.
			return err
		}
		if status.Code(errors.Unwrap(err)) != codes.NotFound || attempt >= maxResubscribes {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(resubscribeDelay):
		}
	}
}

// ensureSubscription returns the subscription, creating it when it is missing.
func (c *Client) ensureSubscription(ctx context.Context, topic, subscription string) (*gcppubsub.Subscription, error) {
	gcpTopic, err := c.topic(ctx, topic)
	if err != nil {
		return nil, err
	}
	sub := c.client.Subscription(subscription)
	exists, err := sub.Exists(ctx)
	if err != nil {
		return nil, fmt.Errorf("pubsub/gcp: check subscription %q: %w", subscription, err)
	}
	if exists {
		return sub, nil
	}
	created, err := c.client.CreateSubscription(ctx, subscription, gcppubsub.SubscriptionConfig{
		Topic:       gcpTopic,
		AckDeadline: ackDeadline,
	})
	if err != nil {
		// A concurrent replica may have won the race; fall back to the existing
		// subscription rather than failing startup.
		if !isAlreadyExists(err) {
			return nil, fmt.Errorf("pubsub/gcp: create subscription %q on %q: %w", subscription, topic, err)
		}
		return c.client.Subscription(subscription), nil
	}
	return created, nil
}

// SubscribeTopic creates the topic and the subscription bound to it when they
// are missing, then consumes it exactly like Subscribe. This is the entry point
// for a service that owns its subscription (its main wires it at startup) and
// must run against a blank emulator.
func (c *Client) SubscribeTopic(ctx context.Context, topic, subscription string, h pubsub.Handler) error {
	return c.consume(ctx, topic, subscription, h)
}

// Close stops the batching publisher of every topic used so far and closes the
// underlying client.
func (c *Client) Close() error {
	c.mu.Lock()
	for _, topic := range c.topics {
		topic.Stop()
	}
	c.topics = make(map[string]*gcppubsub.Topic)
	c.mu.Unlock()

	if err := c.client.Close(); err != nil {
		return fmt.Errorf("pubsub/gcp: close client: %w", err)
	}
	return nil
}

func (c *Client) receive(ctx context.Context, sub *gcppubsub.Subscription, topic string, h pubsub.Handler) error {
	err := sub.Receive(ctx, func(ctx context.Context, msg *gcppubsub.Message) {
		if err := h(ctx, pubsub.Message{Topic: topic, Data: msg.Data, Attributes: msg.Attributes}); err != nil {
			msg.Nack()
			return
		}
		msg.Ack()
	})
	if err != nil {
		return fmt.Errorf("pubsub/gcp: receive on %q: %w", sub.ID(), err)
	}
	return nil
}

// topic returns the topic handle for id, creating the topic if it does not
// exist yet, and caches it: every *Topic owns a batching goroutine, so one
// handle per id is reused for the client's lifetime.
func (c *Client) topic(ctx context.Context, id string) (*gcppubsub.Topic, error) {
	c.mu.Lock()
	cached, ok := c.topics[id]
	c.mu.Unlock()
	if ok {
		return cached, nil
	}

	topic := c.client.Topic(id)
	exists, err := topic.Exists(ctx)
	if err != nil {
		return nil, fmt.Errorf("pubsub/gcp: check topic %q: %w", id, err)
	}
	if !exists {
		created, err := c.client.CreateTopic(ctx, id)
		if err != nil {
			if !isAlreadyExists(err) {
				return nil, fmt.Errorf("pubsub/gcp: create topic %q: %w", id, err)
			}
		} else {
			topic = created
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	// Another goroutine may have cached its own handle meanwhile; keep that one
	// and drop ours so a single batching goroutine survives per topic.
	if cached, ok := c.topics[id]; ok {
		topic.Stop()
		return cached, nil
	}
	c.topics[id] = topic
	return topic, nil
}

// isAlreadyExists reports whether err is Cloud Pub/Sub's "resource exists"
// status, which get-or-create races produce and callers can ignore.
func isAlreadyExists(err error) bool {
	return status.Code(err) == codes.AlreadyExists
}
