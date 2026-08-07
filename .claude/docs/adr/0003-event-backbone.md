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
The comms consumer claims each message through a `processed_events` ledger AND
writes its Notification **in one transaction** (`repo.RecordEvent`): it inserts
`(tenant_id, event_key)` — `event_key = <event type>:<event id>` — with
`ON CONFLICT DO NOTHING`; if `RowsAffected == 0` the event was already handled, so
the handler returns nil (ack, no-op); otherwise it creates the Notification in the
same tx and commits. Because claim + create commit atomically, a crash before
commit rolls the claim back and redelivery re-runs cleanly — there is **no
mark-then-create loss window**. Handlers MUST be idempotent — this is the contract
for all future consumers.

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

- **No consume-side loss window:** claim + Notification commit atomically in
  `RecordEvent` (see above), so there is no mark-then-create gap. The remaining
  open problem is **cross-service** side effects (the FCM push happens after the
  commit and is best-effort — see below); a transactional outbox would be the
  stronger pattern if push delivery ever needs an at-least-once guarantee.
- **Pub/Sub client version:** the gcp adapter uses `cloud.google.com/go/pubsub`
  v1 (the v2 module is present transitively). Future consumers inherit v1; a
  v1→v2 migration is a tracked backlog item, not a per-consumer choice.
- **Event attributes are for routing only.** Producers stamp `type` + `tenant_id`
  message attributes; the consumer trusts the tenant inside the **payload**, not
  the attribute (payload-as-truth). Do not "fix" a consumer to trust the attribute.
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
