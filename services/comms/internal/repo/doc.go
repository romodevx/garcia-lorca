// Package repo is the GORM-backed persistence layer for comms. GORM lives ONLY
// here; it binds transactions via pkg/authctx for RLS on sensitive tables.
//
// A double model (the GORM structs in models.go ↔ the service's Notification and
// Device via mapper.go) keeps the application layer framework-free: it depends on
// the service.Repo interface it declares itself, never on this package.
//
// Three tables back three jobs: `notifications` is the per-student feed,
// `devices` the push targets keyed on their FCM token, and `processed_events` the
// idempotency ledger. The ledger is the reason RecordEvent exists as one method:
// claiming an event and materialising its notification happen in the SAME
// transaction, so a redelivered Pub/Sub message is a no-op and a failed write
// rolls the claim back instead of losing the notification.
package repo
