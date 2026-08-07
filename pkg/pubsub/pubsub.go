// Package pubsub defines the asynchronous-messaging boundary for García Lorca.
//
// Services communicate asynchronously where possible (per the architecture
// rules). On GCP the transport is Cloud Pub/Sub, but call sites depend only on
// the Publisher / Subscriber interfaces here so the transport stays a
// replaceable detail (an in-memory implementation backs unit tests).
//
// Two implementations ship with this package: MemoryBus (in-process, for unit
// tests and local runs) and pubsub/gcp.Client (Cloud Pub/Sub, emulator-aware).
package pubsub

import (
	"context"
	"errors"
)

// ErrSubscriptionNotFound is returned by Subscribe when the subscription does
// not exist. Creating it is a deployment/bootstrap concern, exposed by each
// implementation as SubscribeTopic.
var ErrSubscriptionNotFound = errors.New("subscription not found")

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

// Subscriber consumes messages from an existing subscription and dispatches
// them to h until ctx is cancelled, returning nil on that graceful shutdown.
// The subscription carries its own topic binding, so Subscribe needs only its
// name; implementations return ErrSubscriptionNotFound when it does not exist
// and offer SubscribeTopic to create-then-consume.
type Subscriber interface {
	Subscribe(ctx context.Context, subscription string, h Handler) error
}
