# ADR 0003 — event backbone: real Pub/Sub + idempotent at-least-once consumers

- Status: Accepted
- Date: 2026-08-07
- Context: Phase 9a. Until now every service that "emitted" a domain event
  (`PaymentRegistered`, `AttendanceMarked`/`AbsenceRecorded`, `StudentEnrolled`)
  published to a **dev log bus** that transported nothing — no consumer existed.
  Comms (M10) is the first consumer: it must turn absence/payment events into
  guardian-facing notifications. This ADR fixes the transport + the consumer
  contract that every future consumer (reports, scholarships, …) will copy.

## Decision

**Transport behind the `pkg/pubsub` port.** `pkg/pubsub` defines
`Publisher`/`Subscriber`/`Handler`/`Message`. Two implementations land now:
- `pkg/pubsub.MemoryBus` — stdlib-only, for tests/local; at-least-once with
  redelivery-on-error. Keeps `pkg` dependency-light.
- `pkg/pubsub/gcp.Client` — Cloud Pub/Sub over `cloud.google.com/go/pubsub`,
  the gcp-coupled corner of `pkg` (like `authctx/gormtx`). It honors
  `PUBSUB_EMULATOR_HOST` (the client library reads it), so the compose emulator
  is the local transport and real Cloud Pub/Sub is the same code in shared envs.
  Topics/subscriptions are get-or-create (emulator-friendly).

**Producers publish for real, with a fallback.** Each producer's composition
root picks the gcp client when a broker is configured
(`PUBSUB_EMULATOR_HOST`/`PUBSUB_PROJECT_ID`), else the dev log bus — so a
service still runs standalone. The `EventPublisher` adapters are unchanged; they
depend only on `pubsub.Publisher`.

**Consumers are idempotent because delivery is at-least-once.** Pub/Sub redelivers.
The comms consumer gates every message through a `processed_events` ledger:
`event_key = <event type>:<event id>`, `INSERT … ON CONFLICT DO NOTHING`; if the
row already existed the handler returns nil (ack, no-op). Only a genuinely new
event proceeds to create the Notification. Handlers MUST be idempotent — this is
the contract for all future consumers.

**Side effects are best-effort and ordered after durability.** The FCM push
happens *after* the notification is persisted and the event is marked processed,
and its error is logged and swallowed — a broker/push outage must never Nack the
message (which would redeliver and duplicate). The notification is the source of
truth; the push is a delivery attempt.

**Subscriber lifecycle.** A consumer's `main` runs the Connect API server and the
subscriber loop together under `errgroup` with a cancellable context; a
SIGINT/SIGTERM cancels the context, `SubscribeTopic` returns, and the server
shuts down gracefully. "No broker configured" degrades to API-only, not a crash.

## Consequences / limits

- **Crash window:** `MarkProcessed` commits before `CreateNotification`. A crash
  between them drops that one notification (it will be marked processed, so
  redelivery won't retry). Accepted for MVP (notifications are advisory, not
  ledger money); the stronger fix is a single transaction spanning mark+create,
  or an outbox — revisit if a notification-loss SLO demands it.
- **Guardian targeting is deferred (9b).** In 9a a Notification is student-scoped
  and the push fans out to tenant devices (no guardian↔student link yet). The
  render/target step refines when the guardian model + family auth land.
- **Real FCM** (Firebase + Secret Manager credentials) replaces the `LogSender`
  behind an env/flag — a `deploy/README.md` carry-forward.
- **Prod subscription hygiene:** at-least-once needs a dead-letter topic + retry
  policy on the Cloud Pub/Sub subscription (not needed on the emulator) — a
  deploy carry-forward.
- Every future event consumer (reports, scholarships, uniforms) reuses this
  seam: subscribe via `pkg/pubsub`, dedupe via a `processed_events`-style gate,
  keep side effects best-effort after durability.
