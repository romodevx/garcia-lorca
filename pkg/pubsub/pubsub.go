// Package pubsub defines the asynchronous-messaging boundary for García Lorca.
//
// Services communicate asynchronously where possible (per the architecture
// rules). On GCP the transport is Cloud Pub/Sub, but call sites depend only on
// the Publisher / Subscriber interfaces here so the transport stays a
// replaceable detail (an in-memory implementation backs unit tests).
//
// STATUS: interface stub. Concrete Cloud Pub/Sub and in-memory implementations
// are added when the first event-producing module needs them, test-first.
package pubsub

import "context"

// Message is a published event: a topic plus an opaque payload and headers.
// Payloads are serialized protobuf (the contract types from gen/go).
type Message struct {
	Topic      string
	Data       []byte
	Attributes map[string]string
}

// Publisher emits messages to a topic. Implementations must be safe for
// concurrent use.
type Publisher interface {
	Publish(ctx context.Context, msg Message) error
}

// Handler processes a single received message. Returning an error signals the
// transport to redeliver (at-least-once semantics); handlers must be idempotent.
type Handler func(ctx context.Context, msg Message) error

// Subscriber consumes messages from a subscription and dispatches them to h
// until ctx is cancelled.
type Subscriber interface {
	Subscribe(ctx context.Context, subscription string, h Handler) error
}

// TODO(events): add the Cloud Pub/Sub adapter and an in-memory bus with a
// redelivery test once the first domain event is defined.
