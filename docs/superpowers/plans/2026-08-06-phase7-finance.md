# Phase 7 — Finance / Adeudos (charges + payments + balance) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]` checkboxes.

**Goal:** The school's #1 ask — a per-student debt & payment ledger. An admin manages **dynamic charge concepts**, applies **charges** to students (individually or **in bulk per cycle/group**), records **window payments** (cash/card/transfer + "con qué se pagó" + reference), applies **discounts**, and sees each student's **account statement** (charges, payments, balance) and **overdue** debts. Money is integer **cents (MXN)**.

**Architecture:** Build out the `finance` service skeleton (Clean Arch; today `doc.go` stubs + `cmd/main.go`; already in `go.work` + compose). Cross-service seams (consumer-defined ports + Connect clients, like `grades`/`students`): **StudentReader/EnrollmentReader** (→ students `GetStudent`/`ListEnrollments`) to validate a student and to expand a cycle/group into its enrolled students for bulk charging, and **CycleReader** (→ config `GetCycle`/`GetGroup`) to validate the cycle/group. Cross-service refs (`student_id`, `cycle_id`, `group_id`) are plain UUID columns validated by RPC — no cross-service FKs. `concept_id` (within finance) IS a real FK. Same `pkg/authctx` RLS. Emits `PaymentRegistered` (pubsub).

**Scope (confirmed):** finance CORE only — concepts, charges (single + bulk), payments, discounts, balance/statement, overdue. **Scholarships (M08) + CONTPAQi export (S02) are a later Phase 7b** (the `ExportContpaqi` proto stub stays).

**Tech Stack:** Go 1.24, GORM (adapters only), Connect/gRPC, Postgres 16 + RLS, buf, Next.js, testcontainers-go, Pub/Sub emulator.

**Design source:** MODULES.md M05 + the school notes (conceptos dinámicos, pago de inscripción, migrar por ciclo, pagos en ventanilla, descuentos, con qué se pagó).

---

## Money & status rules (domain)
- All amounts are **`int64` cents**, non-negative. A `Money`/cents value object validates `>= 0`.
- `charge.effective = amount_cents - discount_cents` (discount ≤ amount; else Invalid).
- `paid(charge) = sum(payments for the charge)`; `remaining = effective - paid`.
- `status`: `pending` (paid==0), `partially_paid` (0<paid<effective), `paid` (paid>=effective), `cancelled` (explicit). Status is DERIVED on write (stored for query) from paid vs effective.
- A payment that would make `paid > effective` (overpay) → `KindInvalid`. Payment amount `> 0`.
- `overdue`: `due_date < today AND remaining > 0 AND status != cancelled`.

## Relational model → DDL (sub-phase 7.1)
Tenant-scoped, `ENABLE + FORCE RLS`, `tenant_isolation` on `app.current_tenant` (copy an existing service's `0001`).
```
charge_concepts (catalog)
  id, tenant_id, name, default_amount_cents bigint NOT NULL CHECK (>=0),
  recurrence text CHECK (recurrence IN ('once','monthly')) DEFAULT 'once',
  active bool NOT NULL DEFAULT true, created_at, updated_at
  UNIQUE(tenant_id, name)

charges
  id, tenant_id,
  student_id uuid NOT NULL,   -- students (RPC-validated)
  cycle_id   uuid NOT NULL,   -- config.cycles (RPC-validated)
  concept_id uuid NOT NULL REFERENCES charge_concepts(id) ON DELETE RESTRICT,  -- same-service FK
  description text NOT NULL,
  amount_cents   bigint NOT NULL CHECK (amount_cents >= 0),
  discount_cents bigint NOT NULL DEFAULT 0 CHECK (discount_cents >= 0),
  due_date date NOT NULL,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','partially_paid','paid','cancelled')),
  created_at, updated_at
  CHECK (discount_cents <= amount_cents)
  -- one charge per (student, cycle, concept) unless the concept is recurring:
  -- for 'once' concepts, UNIQUE(tenant_id, student_id, cycle_id, concept_id) prevents dupes on bulk re-run.
  (partial UNIQUE index WHERE recurrence semantics — see task; simplest: UNIQUE(tenant_id, student_id, cycle_id, concept_id))

payments
  id, tenant_id,
  charge_id uuid NOT NULL REFERENCES charges(id) ON DELETE RESTRICT,  -- same-service FK
  amount_cents bigint NOT NULL CHECK (amount_cents > 0),
  method text NOT NULL CHECK (method IN ('cash','card','transfer')),
  reference text NOT NULL DEFAULT '',   -- "con qué se pagó" / folio
  paid_at timestamptz NOT NULL DEFAULT now(),
  created_at
  -- received_by (the acting user id) reserved: add a nullable uuid column, unused until teacher/cashier auth
```
Indexes: `charges (tenant_id, student_id)`, `(tenant_id, cycle_id)`, `(tenant_id, status)`, `(tenant_id, due_date)`; `payments (tenant_id, charge_id)`; concepts `(tenant_id)`. No seed.

> **Bulk uniqueness:** `UNIQUE(tenant_id, student_id, cycle_id, concept_id)` keeps a bulk "Inscripción to all enrolled" idempotent (re-run skips existing) — the usecase inserts with ON CONFLICT DO NOTHING and reports how many were created vs skipped. (Monthly concepts that legitimately recur are out of the bulk-once path; note as a follow-up if monthly bulk is needed.)

## Proto contract (sub-phase 7.1)
Rewrite `finance.proto` `FinanceService` (keep `Health` + the `ExportContpaqi` stub): `CreateConcept`/`ListConcepts`/`UpdateConcept`; `CreateCharge` (single); `GenerateCharges` (bulk: concept + cycle [+ optional group] → a charge per active enrollment); `ListCharges` (by student or cycle); `ApplyDiscount` (charge_id, discount_cents, reason); `RegisterPayment` (charge_id, amount_cents, method, reference); `GetAccount` (student_id → statement: charges w/ effective/paid/remaining/status + payments + total_balance_cents); `CancelCharge` (charge_id); `ListOverdue` (optional cycle_id). Money fields are `int64` cents. Emit `PaymentRegistered` (already declared).

---

# Sub-phase 7.1 — Backend (`feature/phase7-finance-be`)

Branch: `git checkout -b feature/phase7-finance-be feature/phase7-finance`.

## Task 7.1.1: Migration + RLS
- [ ] `migrations/finance/0001_init.sql`: the 3 tables above (concepts/charges/payments) + RLS + CHECKs + UNIQUE + indexes; `concept_id`/`charge_id` are real same-service FKs (RESTRICT); `student_id`/`cycle_id`/`group_id` are plain UUID (no FK). `BEGIN;…COMMIT;`, no seed. Verify via `migrate` (`\dt`, RLS on, CHECKs reject bad amounts, the money columns are `bigint`). Commit.

## Task 7.1.2: Proto — rewrite the finance surface
- [ ] Per the contract; `buf generate`/`lint`/`breaking --against '.git#branch=dev'` (record output — the FASE-0 stub's placeholder `FinanceAccount` message removal is the expected/acceptable break, like grades' `Grade`). `cd gen/go && go build ./...`. Commit.

## Task 7.1.3: Domain
- [ ] `money.go` (cents value object, `>=0`), `concept.go` (`NewChargeConcept`: name non-empty, amount `>=0`, recurrence valid), `charge.go` (`NewCharge`: ids/description non-empty, amount `>=0`, discount `<= amount`, due date; `Effective()`, `Status` derivation helper `DeriveStatus(paidCents)`, `CanCancel`), `payment.go` (`NewPayment`: amount `>0`, method valid). TDD each invariant. Commit.

## Task 7.1.4: Cross-service reader clients
- [ ] Consumer ports: `StudentReader{ByID(id)→StudentRef{ID,Status}}`, `EnrollmentReader{ByCycle(cycleID)→[]EnrollmentRef{StudentID,GroupID,Status}}` (→ students `ListEnrollments`), `CycleReader{ByID(id)→CycleRef{ID}}`+`GroupByID` (→ config). Adapters `internal/adapters/{studentsclient,configclient}` over the generated clients; `CodeNotFound`→`ErrNotFound`. Light NotFound unit tests. Commit.

## Task 7.1.5: Ports (repo) + usecase (the finance core)
- [ ] Repo `FinanceRepository`: concept CRUD; `CreateCharge` (unique(student,cycle,concept) → Conflict); `CreateChargesBulk(charges)` (ON CONFLICT DO NOTHING, returns created count); `GetCharge`; `ListCharges(studentID, cycleID)`; `ApplyDiscount(chargeID, discountCents)` (re-derive status); `CancelCharge`; `RegisterPayment(payment)` + it recomputes & stores the charge status in ONE tx; `PaymentsForCharge`; `ListOverdue(cycleID, today)`; `AccountStatement(studentID)` (charges + payments + computed balance).
- [ ] Usecase `FinanceService`: `CreateConcept/ListConcepts/UpdateConcept`; `CreateCharge(studentID, cycleID, conceptID, amountOverride?, dueDate)` — validate student (RPC) + cycle (RPC), default amount/description from the concept; `GenerateCharges(conceptID, cycleID, groupID?, dueDate)` — validate cycle; `enrollments.ByCycle(cycleID)` (filter group + active) → build one charge per student → `CreateChargesBulk` → return {created, skipped}; `ApplyDiscount(chargeID, discountCents, reason)` (discount ≤ amount); `RegisterPayment(chargeID, amountCents, method, reference)` — the **payment knot**: load charge, compute remaining, reject overpay/non-positive (Invalid), insert payment + re-derive status in one tx, emit `PaymentRegistered` best-effort; `CancelCharge`; `GetAccount(studentID)` → statement; `ListOverdue(cycleID?)` (uses injected `now`/today).
- [ ] Usecase tests (fakes for repo + the readers): concept CRUD; create-charge validates student+cycle (missing → Invalid); bulk generates per active enrollment + idempotent re-run (skipped count); discount > amount → Invalid; payment partial→partially_paid, full→paid, overpay→Invalid, on cancelled→Invalid; account balance math; overdue selection by date (injected clock). Neutral literals.
- [ ] Commit (domain / ports+usecase / clients as sensible commits).

## Task 7.1.6: Postgres adapter + RLS IT
- [ ] models/mapper/repository + txrunner; `RegisterPayment` inserts the payment AND updates the charge status in ONE `withTenant` tx (atomic; overpay guarded in the usecase, backstopped by re-derivation). `CreateChargesBulk` ON CONFLICT DO NOTHING. Money as `bigint`. RLS integration: `TestRLSIsolatesCharges`/`...Payments`/`...Concepts` (read+write, non-owner app role); payment-updates-status atomicity; bulk idempotency; account-balance query; overdue query. Neutral literals. Commit.

## Task 7.1.7: gRPC handler + composition root
- [ ] Handler for all RPCs (reuse connectErr; money int64 both ways). `cmd/main.go`: `DATABASE_URL`, `FINANCE_TENANT_ID`, `CONFIG_BASE_URL`, `STUDENTS_BASE_URL`, Pub/Sub publisher (mirror students' publisher for `PaymentRegistered`); build the two reader clients + repo + usecase + handler; mount + `/healthz`. Add `config`+`students` to the finance `depends_on` + the `*_BASE_URL` envs in `docker-compose.yml`. Compose smoke: finance boots + reaches config+students. Commit.

**Checkpoint 7.1** — merge → `feature/phase7-finance`.

---

# Sub-phase 7.2 — Gateway (`feature/phase7-finance-gw`)
- [ ] client `internal/client/finance.go` over `financev1connect`; handler `internal/handler/finance.go` (+ test); routes under `/api/v1/finance/...` via a `financeWrite`/`financeRead` pair = `guard.Protect("finance", requireAuthChanged(RequirePermission("finance:write"|"finance:read")))`. Writes: create/generate charges, apply discount, register payment, cancel, concept create/update → `finance:write`; reads: list charges, account, overdue, list concepts → `finance:read`. `migrations/iam/0007_finance_permissions.sql` grants finance:read/write to owner. Add `finance` to gateway `ENABLED_MODULES` + `FINANCE_BASE_URL` + `depends_on`.
- [ ] E2E: create a concept → generate charges for a cycle (needs an enrolled student) → register a window payment (partial then full → status flows) → GET account (balance) → apply a discount → list overdue; out-of-range/overpay → 400; module OFF → 404. Commit.

**Checkpoint 7.2** — merge → `feature/phase7-finance`.

---

# Sub-phase 7.3 — Web (`feature/phase7-finance-web`)
- [ ] `apps/web/app/finance/api.ts` (+ test) for the finance endpoints (money as integer cents; format to MXN in the UI). Screens under `_screens/finance/`: **Concepts** admin; **Charges** (per student or bulk-generate for a cycle/group); **Register payment** (ventanilla: amount, method, reference — fewest clicks); **Account statement** (charges/payments/balance per student); **Overdue** list. Flag-gated `module_finance`; Cmd+K. RTL tests: register a payment (assert cents + method), bulk-generate, account render. Verify `pnpm ... test && tsc && lint`. Commit.

**Checkpoint 7.3** — merge → `feature/phase7-finance`.

---

## Final gates (after 7.1–7.3 merged)
- Full suite (Go race + finance/config/students integration incl. RLS isolation on the 3 finance tables + payment-status atomicity; web) green.
- buf lint + breaking vs `dev` (the finance stub rewrite: expected `FinanceAccount` placeholder removal).
- **Security gate:** RLS on concepts/charges/payments; money can't go negative / overpay rejected (domain + DB CHECK); the charge knot validates student+cycle by RPC; no client-injected status/derived fields; finance→students/config server-to-server (network-private in shared envs — checklist); flag boundary + `finance:read/write`; no secret literals; **money integrity** (int64 cents end to end, no float).
- Tech-lead gate. Update MODULES.md (M05 note) + FLAGS.md. Hand `feature/phase7-finance` to the user for review (do NOT merge to `dev`).

## Out of scope (Phase 7b / later)
Scholarships (M08, adjusts the ledger); CONTPAQi XML export (S02, real field mapping); scheduled `DebtOverdue` event emission (Phase 7 provides the overdue READ; event-emitting cron is later); monthly recurring bulk generation; `received_by` cashier attribution (needs cashier auth); refunds/reversals.

## Testing strategy
Unit (money/charge/payment invariants, the payment knot + bulk + overdue with fakes + injected clock, handler mapping), integration (RLS isolation on the 3 tables, payment-status atomicity, bulk idempotency, balance/overdue queries), contract (proto), web (Vitest/RTL), flag-ON/OFF. Deterministic (injected id/clock). Neutral literals. **No floats for money — int64 cents throughout.**
