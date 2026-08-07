package pubsub

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"
)

// MemoryBus is an in-process Publisher and Subscriber: a topic fans a published
// message out to one independent copy per subscription, and a Handler that
// returns an error gets the message again (at-least-once). It is meant for unit
// tests and local runs — nothing is persisted, and messages published before a
// subscription exists are dropped, exactly as Cloud Pub/Sub drops them.
//
// Redelivery is immediate by default so tests need no sleeps; local runs that
// may hit a poison message can pace it with WithRedeliveryDelay.
// MemoryBus is safe for concurrent use.
type MemoryBus struct {
	mu              sync.Mutex
	subs            map[string]*memorySubscription
	redeliveryDelay time.Duration
}

// memorySubscription is one subscription's queue. wakeup is a buffered signal
// (capacity 1) so a push that lands between a failed pop and the consumer's
// wait is never lost.
type memorySubscription struct {
	topic  string
	mu     sync.Mutex
	queue  []Message
	wakeup chan struct{}
}

// MemoryOption configures a MemoryBus at construction.
type MemoryOption func(*MemoryBus)

// WithRedeliveryDelay makes the bus wait d before a failed message becomes
// available again, keeping an always-failing handler from spinning.
func WithRedeliveryDelay(d time.Duration) MemoryOption {
	return func(b *MemoryBus) { b.redeliveryDelay = d }
}

// NewMemoryBus returns an empty in-process bus.
func NewMemoryBus(opts ...MemoryOption) *MemoryBus {
	bus := &MemoryBus{subs: make(map[string]*memorySubscription)}
	for _, opt := range opts {
		opt(bus)
	}
	return bus
}

// CreateSubscription registers subscription against topic so later publishes to
// that topic reach it. It is idempotent; an already-registered subscription
// keeps the topic it was first bound to.
func (b *MemoryBus) CreateSubscription(topic, subscription string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.subs[subscription]; ok {
		return
	}
	b.subs[subscription] = &memorySubscription{topic: topic, wakeup: make(chan struct{}, 1)}
}

// HasSubscription reports whether subscription is registered.
func (b *MemoryBus) HasSubscription(subscription string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.subs[subscription]
	return ok
}

// Publish delivers msg to every subscription bound to its topic. Each
// subscription receives its own copy, so a handler mutating what it received
// cannot corrupt another subscriber's view. Publishing to a topic nobody
// subscribes to succeeds and drops the message.
func (b *MemoryBus) Publish(ctx context.Context, msg Message) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("pubsub: publish to %q: %w", msg.Topic, err)
	}

	b.mu.Lock()
	targets := make([]*memorySubscription, 0, len(b.subs))
	for _, sub := range b.subs {
		if sub.topic == msg.Topic {
			targets = append(targets, sub)
		}
	}
	b.mu.Unlock()

	for _, sub := range targets {
		sub.push(cloneMessage(msg))
	}
	return nil
}

// Subscribe dispatches the subscription's messages to h until ctx is cancelled,
// at which point it returns nil (a cancelled context is a graceful shutdown,
// not a failure). The subscription must already exist — use SubscribeTopic or
// CreateSubscription — otherwise it returns ErrSubscriptionNotFound.
func (b *MemoryBus) Subscribe(ctx context.Context, subscription string, h Handler) error {
	b.mu.Lock()
	sub, ok := b.subs[subscription]
	b.mu.Unlock()
	if !ok {
		return fmt.Errorf("pubsub: subscribe to %q: %w", subscription, ErrSubscriptionNotFound)
	}
	return b.consume(ctx, sub, h)
}

// SubscribeTopic binds subscription to topic if it does not exist yet, then
// consumes it like Subscribe. This is the entry point for a service that owns
// its subscription and must not depend on it having been created beforehand.
func (b *MemoryBus) SubscribeTopic(ctx context.Context, topic, subscription string, h Handler) error {
	b.CreateSubscription(topic, subscription)
	return b.Subscribe(ctx, subscription, h)
}

func (b *MemoryBus) consume(ctx context.Context, sub *memorySubscription, h Handler) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		msg, ok := sub.pop()
		if !ok {
			select {
			case <-ctx.Done():
				return nil
			case <-sub.wakeup:
			}
			continue
		}
		if err := h(ctx, msg); err != nil {
			sub.push(msg) // at-least-once: a failed handler sees it again
			if !b.waitBeforeRedelivery(ctx) {
				return nil
			}
		}
	}
}

// waitBeforeRedelivery reports whether consuming should continue.
func (b *MemoryBus) waitBeforeRedelivery(ctx context.Context) bool {
	if b.redeliveryDelay <= 0 {
		return true
	}
	timer := time.NewTimer(b.redeliveryDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (s *memorySubscription) push(msg Message) {
	s.mu.Lock()
	s.queue = append(s.queue, msg)
	s.mu.Unlock()

	select {
	case s.wakeup <- struct{}{}:
	default: // a pending signal already tells the consumer to look
	}
}

func (s *memorySubscription) pop() (Message, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queue) == 0 {
		return Message{}, false
	}
	msg := s.queue[0]
	s.queue = s.queue[1:]
	return msg, true
}

func cloneMessage(msg Message) Message {
	return Message{
		Topic:      msg.Topic,
		Data:       slices.Clone(msg.Data),
		Attributes: maps.Clone(msg.Attributes),
	}
}
