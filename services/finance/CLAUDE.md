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
  A charge with no cycle exists only as another module's sale to a pupil not yet
  placed in a group (task 10, ADR 0016): `NewSaleCharge`, ref required, and a
  CHECK in finance/0017.
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
- `RegisterPayment` is idempotent when the caller sends an `idempotency_key`:
  a repeat under the same key returns the ORIGINAL payment and writes nothing —
  no second row, no status change, and NO second `PaymentRegistered` event. The
  lookup runs inside the payment transaction, under the charge's `FOR UPDATE`
  lock; the partial `UNIQUE (tenant_id, idempotency_key) WHERE idempotency_key
  <> ''` (migration 0002) is the backstop for retries that share no lock. An
  empty key opts out (every call posts a payment). The web ventanilla mints one
  key per payment ATTEMPT and reuses it across retries of that submit.

## M15 — colegiaturas, inscripción y adeudo mensual
- `TuitionPlan` — what one level costs in one cycle: inscription, monthly fee,
  how many months and from which, the due day, and the fixed late fee. Keyed by
  **(cycle, level)**, because a school raises tuition every year and the old year
  must keep reconciling against its own table.
- `due_day` is capped at **28**: a limit on the 31st does not exist in February
  and would be moved by the calendar. Refused at the boundary, never adjusted.
- `charge_concepts.kind` (`custom|tuition|enrollment|late_fee`, at most one of
  each system kind per school) is how the run finds its concepts. By KIND and not
  by name, so renaming "Colegiatura" cannot stop a school being billed.
- `RunBilling(cycle, asOf)` issues the tuition whose due date has arrived, the
  inscription of anybody enrolled without one, and **one fixed surcharge per
  COMPLETED overdue month**. Idempotent through `external_ref`
  (`tuition:<cycle>:<student>:<YYYY-MM>`, `enrollment:<cycle>:<student>`,
  `latefee:<charge>:<YYYY-MM>`), so a button and a scheduler may both call it.
- Surcharges are separate charges and are **excluded from their own input**: a
  fee that earned fees would turn a fixed monthly charge into compound interest.
- A level with no table is **reported by name**, never billed at zero.
- `CreateChargesBulk` splits its batch by whether the charge carries a ref: the
  two kinds are deduplicated by DIFFERENT partial indexes, and ON CONFLICT takes
  one arbiter. Naming only the keyless one made the second run of every month
  fail outright.

## Events
- **Consumed: `StudentEnrolled`** (student-events → `finance-student-events`).
  It charges the pupil's inscription the moment they take a place in a group,
  which is the school's rule. **No idempotency ledger** — unlike comms on the
  same topic — because the charge's own `external_ref` makes a redelivery insert
  nothing. Pull loop locally; authenticated `POST /pubsub` under Cloud Run
  scale-to-zero.
- Published: `PaymentRegistered` (via `adapters/pubsub`, `pkg/pubsub`; a dev
  logging bus locally — swap the Cloud Pub/Sub adapter before shared use).
  `DebtOverdue` is a READ (`ListOverdue`) for now; the event emitter is a later
  phase. Consumers will include comms (notifications) and scholarships.

## RLS-sensitive tables
`charge_concepts`, `charges`, `payments`, `tuition_plans` — each `ENABLE + FORCE ROW LEVEL
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
