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

## Domain invariants
- A `FinanceAccount` belongs to exactly one student; its balance is the derived
  sum of its ledger entries — never stored and mutated independently.
- Money is an integer minor-unit value object with an explicit currency; never a
  float. No silent currency mixing within an account.
- A payment cannot exceed the outstanding debt it settles; over/underpayment is
  an explicit, modeled case, not an arithmetic accident.
- Ledger entries are append-only (immutable once posted); corrections are
  reversing entries, preserving an audit trail.
- Overdue is a function of due date + clock passed in (deterministic; no
  `time.Now()` inside the domain).

## Events
- Published: `PaymentRegistered`, `DebtOverdue` (via `adapters/pubsub`,
  `pkg/pubsub`). Consumers include comms (notifications) and scholarships.

## RLS-sensitive tables
`finance_accounts` — `ENABLE + FORCE ROW LEVEL SECURITY` from the first
migration. All access goes through `pkg/authctx.WithTenantContext`, which binds
`app.current_tenant` via `set_config(..., true)` (SET LOCAL semantics) inside the
transaction. Policies filter on `current_setting('app.current_tenant')`.

## Feature flag
`module_finance` — default OFF. Disabled ⇒ gateway returns 404 for finance routes.
