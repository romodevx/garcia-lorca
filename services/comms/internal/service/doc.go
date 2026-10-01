// Package service holds the comms application logic (thin layout). Architecture
// is earned, not imposed: this layer stays small until comms grows enough to
// warrant a full domain model.
//
// Comms is a CONSUMER, not a producer: it publishes nothing and its RPC surface
// is deliberately small (register a device, read a student's feed, mark one
// read). Everything in the feed is materialised as a side effect of consuming
// another service's domain event — attendance's AbsenceRecorded /
// AttendanceMarked, finance's PaymentRegistered and PaymentCorrected — whose protos ARE the wire
// contract; comms never redefines them.
//
// Because the transport is at-least-once, the consumer's correctness rests on
// the consumed-event ledger: HandleEvent renders the event (a pure function),
// then claims its key and stores the feed entry in ONE transaction, so a
// redelivery is a no-op instead of a second notification. Pushing to devices
// happens after that and is best-effort — the feed entry is already durable, and
// failing the handler for a dead push would redeliver an event the ledger now
// skips, losing the notification rather than retrying it.
//
// The seams (Repo, Sender) are declared here and satisfied from the outside, so
// no framework — GORM, Connect, FCM — reaches this package.
package service
