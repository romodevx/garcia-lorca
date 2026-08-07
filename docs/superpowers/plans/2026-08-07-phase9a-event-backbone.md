# Phase 9a — Event backbone (real Pub/Sub + comms consumer + notifications) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]` checkboxes.

**Goal:** Close the producer→consumer chain the roadmap has been building toward. Producers (finance/attendance/students) **actually publish** to Pub/Sub (the compose emulator locally), and the **`comms` service consumes** `AbsenceRecorded` / `PaymentRegistered` (and student events), **persists a Notification** per event (idempotently — Pub/Sub is at-least-once), lets a device register an **FCM token**, and **sends the push through a stub sender** (real FCM behind an env/flag). No guardian model, no mobile, no agenda/messages yet — those are 9b/9c/9d.

**Architecture:** `comms` is a **thin** service (handler→service→repo) that ALSO runs a **subscriber loop** (the repo's first Pub/Sub consumer). `pkg/pubsub` gets its first concrete implementations: a **Cloud Pub/Sub** adapter (honors `PUBSUB_EMULATOR_HOST`) in a `pkg/pubsub/gcp` subpackage (the gcp-coupled corner), and an **in-memory bus** in `pkg/pubsub` for tests (with a redelivery test). GORM only in `comms/internal/repo`. Same `pkg/authctx` RLS. FCM is a `Sender` port with a dev/log stub.

**Decisions (confirmed):** slice = event backbone first; family auth (guardian=iam user + Bearer) is 9b; FCM stubbed now, real behind env. **Consumer must be idempotent** (dedupe by the event's message id / natural key).

**Tech Stack:** Go 1.24, `cloud.google.com/go/pubsub`, GORM (repo only), Connect/gRPC, Postgres 16 + RLS, buf, testcontainers-go, Pub/Sub emulator (already in compose: `pubsub:8085`, `PUBSUB_EMULATOR_HOST`).

---

## Model & rules
- **At-least-once → idempotent consume.** Each incoming message carries a stable id (use the event's own id — `payment_id`/`record_id` — plus the event `type` as the dedupe key). A `processed_events` ledger (unique on `(tenant_id, event_key)`) makes re-delivery a no-op.
- A **Notification** is student-scoped for now (recipient targeting to specific guardians is 9b): `{id, tenant_id, student_id, type, title, body, created_at, read_at NULL}`. Title/body are rendered server-side from the event (Spanish; i18n of push copy is a later concern).
- A **Device** registers an FCM token for a user: `{id, tenant_id, user_id, fcm_token, platform, created_at}` — `user_id` is the (future guardian) iam user; in 9a it's just whoever calls RegisterDevice. UNIQUE on `(tenant_id, fcm_token)`.
- **Push is best-effort** via the `Sender` port; failure is logged, never blocks the consume (the notification is already persisted). Dev sender logs; real FCM behind `FCM_ENABLED`/credentials (carry-forward).
- Deterministic: injected `now`/`newID`; no clock reads in logic.

## Relational model → DDL (sub-phase 9a.1)
`migrations/comms/0001_init.sql`, tenant-scoped, `ENABLE + FORCE RLS`, `tenant_isolation` (copy grades' `0001`).
```
notifications
  id, tenant_id, student_id uuid NOT NULL, type text NOT NULL,
  title text NOT NULL, body text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(), read_at timestamptz NULL
  INDEX (tenant_id, student_id), INDEX (tenant_id, created_at)

devices
  id, tenant_id, user_id uuid NOT NULL, fcm_token text NOT NULL,
  platform text NOT NULL DEFAULT '' CHECK (platform IN ('','ios','android','web')),
  created_at timestamptz NOT NULL DEFAULT now()
  UNIQUE (tenant_id, fcm_token), INDEX (tenant_id, user_id)

processed_events
  tenant_id uuid NOT NULL, event_key text NOT NULL,
  processed_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, event_key)
```
All three RLS ENABLE+FORCE + tenant_isolation. No seed.

## Proto contract (sub-phase 9a.1)
Rewrite `comms.proto` `CommsService` (keep `Health`): `RegisterDevice(RegisterDeviceRequest{user_id, fcm_token, platform}) → RegisterDeviceResponse{Device}`; `ListNotifications(ListNotificationsRequest{student_id, unread_only bool}) → ListNotificationsResponse{repeated Notification}`; `MarkNotificationRead(MarkNotificationReadRequest{id}) → (Notification)`. Messages: `Notification{id, tenant_id, student_id, type, title, body, created_at, read_at}`, `Device{id, tenant_id, user_id, fcm_token, platform}`. Drop the placeholder `AgendaEntry` (agenda is 9d) — reserve it. Comms does NOT publish; it consumes finance/attendance/students events (their proto messages are the wire contract, already defined).

---

# Sub-phase 9a.1 — Foundations (`feature/phase9a-backbone-be`)
Branch off `dev`. These three are disjoint → parallel-safe.

## Task 9a.1.1: `pkg/pubsub` concrete implementations
- [ ] In-memory bus in `pkg/pubsub` (`memory.go`): a `MemoryBus` implementing BOTH `Publisher` and `Subscriber` (topic→subscription fan-out, at-least-once with a redelivery-on-error path). Test `memory_test.go`: publish→subscribe round-trip; a handler error triggers redelivery; idempotent handler sees the dupe. Keep pkg dependency-light (no gcp import here).
- [ ] `pkg/pubsub/gcp` subpackage (`gcp.go`): a `Client` implementing `Publisher` + `Subscriber` over `cloud.google.com/go/pubsub`, honoring `PUBSUB_EMULATOR_HOST` (the client library reads it automatically) and a `PUBSUB_PROJECT_ID`. Auto-create topics/subscriptions if missing (emulator-friendly) or document that the emulator init does. Add `cloud.google.com/go/pubsub` to `pkg/go.mod` (use `go mod edit -require=...@<version>` + `GOTOOLCHAIN=local GOWORK=off go mod tidy` then `go mod edit -go=1.24` to AVOID the go-directive bump gotcha; then rebuild workspace). Update the pkg go.mod header comment (gcp is another coupled corner like gormtx).
- [ ] Commit.

## Task 9a.1.2: Migration + RLS (`migrations/comms/0001_init.sql`)
- [ ] Per the DDL; mirror grades' style; verify on a throwaway Postgres (3 tables, RLS forced, CHECK + UNIQUE + PK hold). Commit (pathspec).

## Task 9a.1.3: Proto (`proto/garcialorca/comms/v1/comms.proto`)
- [ ] Per the contract; `make gen`; `cd gen/go && go build ./...`; buf lint + breaking (the `AgendaEntry`/`Notification` stub change is the expected break — reserve fields). Commit ONLY the proto (pathspec).

---

# Sub-phase 9a.2 — comms service + producer rewiring (`feature/phase9a-backbone-be`, after 9a.1)
Two disjoint work items (comms internals vs producers' main.go) — sequence or parallel with care.

## Task 9a.2.1: comms service (subscriber + repo + FCM stub + API)
- [ ] `internal/service`: `CommsService` with `Repo`, `Sender` (FCM port), `tenantID`, `newID`, `now`. A `HandleEvent(ctx, msg pubsub.Message)` idempotent handler: derive the event_key from `msg.Attributes["type"]` + the event id in the payload; `repo.MarkProcessed` (INSERT … ON CONFLICT DO NOTHING → if already processed, return nil, no-op); unmarshal the proto event (finance/attendance/students `gen/go`); render `{type,title,body,student_id}`; `repo.CreateNotification`; best-effort `sender.Push(...)` to the student's devices (in 9a, no guardian link yet → push to devices by student is deferred; for now push to ALL devices registered for the tenant OR skip push and just log intent — keep it best-effort + logged; document the 9b targeting refinement). `RegisterDevice`, `ListNotifications`, `MarkNotificationRead`. Status/platform validation.
- [ ] `internal/repo` (GORM, RLS via `pkg/authctx/gormtx`): `MarkProcessed` (ON CONFLICT DO NOTHING, returns whether newly inserted), `CreateNotification`, `ListNotifications(studentID, unreadOnly)`, `MarkRead`, `UpsertDevice`, `DevicesForUser`/`DevicesForTenant`. RLS integration tests (isolation on all 3 tables + idempotent MarkProcessed).
- [ ] `internal/adapters/fcm`: `Sender` port + a `LogSender` dev stub (logs token+title). Real FCM behind env is a carry-forward (documented, not built).
- [ ] Event-rendering: a small mapper from each proto event → notification title/body (Spanish). Table-driven test.
- [ ] Service unit tests (in-memory bus + fakes): consuming an AbsenceRecorded creates one notification; re-delivery (same event_key) creates NONE (idempotent); RegisterDevice; ListNotifications unread filter. Commit.

## Task 9a.2.2: producers publish for real
- [ ] finance/attendance/students `cmd/main.go`: replace `newDevBus(log)` with the real publisher — `pkg/pubsub/gcp` client when `PUBSUB_EMULATOR_HOST` or `PUBSUB_PROJECT_ID` is set, else fall back to the dev log bus (so nothing breaks without the emulator). Keep the existing `EventPublisher` adapters unchanged (they depend on `pkg/pubsub.Publisher`). Verify each service still builds + its tests pass. Commit.

## Task 9a.2.3: comms composition root + subscriber loop
- [ ] `comms/cmd/main.go`: build repo + FCM LogSender + service; start the Connect server (Health/RegisterDevice/ListNotifications/MarkNotificationRead) AND a background **subscriber** (gcp client) subscribing to `payment-events` + `attendance-events` (+ `student-events`), dispatching to `service.HandleEvent`, with graceful shutdown (errgroup + signal). Envs: `DATABASE_URL`, `COMMS_TENANT_ID`, `PUBSUB_EMULATOR_HOST`/`PUBSUB_PROJECT_ID`. `docker-compose.yml`: comms `depends_on` postgres/migrate/pubsub + the env. Compose smoke: comms boots + subscribes. Commit.

**Checkpoint 9a.2** — merge → `feature/phase9a-backbone`.

---

# Sub-phase 9a.3 — Gateway (`feature/phase9a-backbone-gw`)
- [ ] `client/comms.go` + `handler/comms.go` (+ test): `POST /api/v1/comms/devices` (RegisterDevice, write), `GET /api/v1/comms/notifications?studentId=&unreadOnly=` (read), `POST /api/v1/comms/notifications/{id}/read` (write). Behind `guard.Protect("comms", requireAuthChanged(RequirePermission("comms:read"|"comms:write")))`. `migrations/iam/0009_comms_permissions.sql` grants comms:read/write to owner. Add comms to gateway `ENABLED_MODULES` + `COMMS_BASE_URL` + depends_on. Update `newMux` + `main_test.go` (flag-off 404 / 401 / 403). Note: `comms:read` is a candidate for the `IsOwnerOnly` guard later (it can expose any student's notifications) — add it to the guard set in 9b when guardian scoping lands; for now owner-only.
- [ ] Tests green; compose parses. Commit.

**Checkpoint 9a.3** — merge → `feature/phase9a-backbone`.

---

## Final gates (after 9a.1–9a.3 merged)
- Full suite: Go `-race` + comms/finance/attendance/students integration (RLS + idempotent consume + in-memory-bus redelivery); gateway; buf lint/breaking. **CI must be green** (it now runs everything, incl. the new pubsub/gcp dep in govulncheck).
- **Security gate:** RLS on notifications/devices/processed_events; flag boundary (comms OFF ⇒ 404) + comms:read/write; consumer is idempotent (no duplicate notifications on redelivery); FCM tokens are secrets-adjacent (don't log the full token in prod — dev stub may, note it); no PII beyond ids in events; internal service network-private (carry-forward); single-tenant `COMMS_TENANT_ID` (carry-forward); real FCM credentials via Secret Manager (carry-forward); `comms:read` owner-only until guardian scoping (carry-forward, ties to B2b).
- **Tech-lead gate:** thin-layout (GORM only in repo, pubsub behind the port), idempotency correctness, best-effort push doesn't block consume, graceful subscriber shutdown, no N+1, TDD/coverage.
- Update MODULES.md (M10) + FLAGS.md (`module_comms`). ADR 0003 for the event-backbone (real Pub/Sub adapter + idempotent-consumer pattern — attendance/finance/comms all reference it; the pattern is reused by every future consumer). Hand `feature/phase9a-backbone` for review (PR → dev; do NOT merge until approved).

## Out of scope (9b/9c/9d)
9b: guardian model (`guardians` + `guardian_students`) + family auth (guardian iam role + Bearer token for mobile) + guardian-scoped notification targeting + `comms:read` per-object scoping. 9c: the Expo family app wired to the gateway (Bearer auth, register device, list notifications). 9d: **mensajes personalizados** (targeted messages to guardians) + **agenda/calendar**. Real FCM (Firebase project + Secret Manager credentials).

## Testing strategy
Unit (idempotent HandleEvent + event→notification rendering with fakes + in-memory bus; RegisterDevice/ListNotifications; the in-memory bus redelivery), integration (RLS isolation on the 3 tables; idempotent MarkProcessed under real Postgres; comms boots + consumes from the emulator if feasible in CI), contract (proto), flag ON/OFF. Deterministic. Neutral literals. No secret literals (FCM tokens are neutral test fixtures).
