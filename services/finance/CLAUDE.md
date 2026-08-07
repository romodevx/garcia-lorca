# finance — Adeudos & Pagos (M05)

> Per-service context. Global team rules (`~/.claude/`) and the repo root
> `CLAUDE.md` always apply. Depends on M01 (iam), M03 (students), M04 (config).

## Purpose
Debt + payment ledger per student, with overdue detection. Owns the CONTPAQi XML
export (spike S02) — that belongs to finance, NOT to the deferred billing track.
Scale-to-zero on Cloud Run.

## Architecture — Clean (rich domain)
`cmd/main.go` + `internal/{domain,usecase,ports,adapters/{postgres,grpc,pubsub}}`.
Dependency direction: `adapters → usecase → domain`. Domain imports no framework.
GORM lives ONLY in `adapters/postgres` (double model + `mapper.go`). Multi-write
ledger operations run in a single transaction (Unit of Work).

## Domain model (Phase 7 — finance core)
- `ChargeConcept` — the tenant's dynamic catalog entry (colegiatura, inscripción,
  uniforme…): name, `defaultAmount` (Cents), recurrence (`once`|`monthly`),
  active. Deactivated, never deleted, so issued charges keep pointing at it.
- `Charge` — one debt: student/cycle/concept (student & cycle are cross-service
  UUIDs), gross `amount` and `discount` (Cents), `dueDate`, and a status derived
  from payments (`pending|partially_paid|paid|cancelled`). `Effective() = amount
  - discount`. The charge stores no paid field — the ledger owns it.
- `Payment` — an append-only movement against a charge: amount (Cents), method
  (`cash|card|transfer`), reference ("con qué se pagó"), `paidAt`.
- An account statement is DERIVED (charges + payments + balance), not a stored row.

## Domain invariants
- Money is `Cents` (int64 minor units), never a float. `NewCents` rejects
  negatives AND caps at `MaxCents` (1e15) — the cap is what keeps the payment
  knot's `currentPaid + amount` from overflowing int64 and bypassing the overpay
  veto.
- A payment cannot push a charge's paid total past its `Effective()` amount, and
  a cancelled charge takes no payment — both vetoed INSIDE the payment
  transaction (repo does `SELECT … FOR UPDATE` then runs the use case's `derive`
  callback), so concurrent cashiers serialize on the charge row.
- Charge status is a projection of the summed payments, re-derived on every
  write; only `cancelled` is set independently and is terminal.
- Overdue is a function of due date + a clock passed in (deterministic; no
  `time.Now()` inside the domain).
- Bulk `GenerateCharges` is idempotent via `UNIQUE(tenant, student, cycle,
  concept)` + ON CONFLICT DO NOTHING (re-run reports created vs skipped).

## Events
- Published: `PaymentRegistered` (via `adapters/pubsub`, `pkg/pubsub`; a dev
  logging bus locally — swap the Cloud Pub/Sub adapter before shared use).
  `DebtOverdue` is a READ (`ListOverdue`) for now; the event emitter is a later
  phase. Consumers will include comms (notifications) and scholarships.

## RLS-sensitive tables
`charge_concepts`, `charges`, `payments` — each `ENABLE + FORCE ROW LEVEL
SECURITY` from the first migration with a `tenant_isolation` policy. All access
goes through `pkg/authctx.WithTenantContext`, which binds `app.current_tenant`
via `set_config(..., true)` (SET LOCAL semantics) inside the transaction.
Policies filter on `current_setting('app.current_tenant')`.

## Out of scope (Phase 7b / later)
Scholarships (M08) ledger integration; CONTPAQi XML export (S02 — `ExportContpaqi`
is a stub returning Unimplemented); scheduled `DebtOverdue` emission;
monthly-recurring bulk; `received_by` cashier attribution + discount-reason audit
(needs cashier auth); refunds/reversing entries.

## Feature flag
`module_finance` — default OFF. Disabled ⇒ gateway returns 404 for finance routes.
