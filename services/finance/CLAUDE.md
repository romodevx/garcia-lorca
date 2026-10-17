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
  (`cash|card|transfer|credit_note`), reference ("con qué se pagó"), `paidAt`
  (instant) and `paidOn` (school-local day, ADR 0035).
- `Tender` (T8, ADR 0025) — one method of a combined payment. A deposit or a
  payment combines 1–4 tenders that add up EXACTLY to the amount to pay.
  `deposit_tenders` (0021) is the source of truth; `deposits.method` is a
  derived summary (`mixed` when several). Allocations are cut per tender, so
  every payment row has one method. A credit note is NOT money: cash-up and
  income report it apart (`credit_note_cents`).
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
- **"Today" is Morelia's calendar day (D8, ADR 0035).** ONE source of time,
  `usecase.SchoolClock`, built once in `cmd/main.go` from `SCHOOL_TIMEZONE`
  (default `America/Mexico_City`, `time/tzdata` embedded, an unknown zone fails
  startup) and injected into every service. `clock.Now()` stamps (paid_at,
  events); `clock.Today()` decides (surcharge days, overdue months, early
  window, `ListOverdue` cutoff, `RunBilling` default, `AsOf`, open charges,
  default paidOn/startsOn/cash-cut day). `domain.SchoolDate` is the only
  instant→day rule and returns midnight UTC, the representation of every
  stored date. NEVER truncate an instant to a day anywhere else (`.UTC()`,
  `Truncate(24h)`, `time.Date(..., time.UTC)` on `now`): that is the UTC day
  coming back in one calculation. Stored dates keep their UTC date
  (`domain.CalendarDay`).
- **A payment's DAY is `paid_on`, never `paid_at`** (migration 0028, ADR 0035).
  `deposits.paid_on` / `payments.paid_on` (`date NOT NULL`, no default) is the
  Morelia civil day: the counter's picked day, or `clock.Today()` for a live
  payment; `paid_at` is the audit instant only. `paymentDate` returns both
  (`paymentDay{at, on}`); a deposit's allocations carry its day; a correction
  without a new date keeps the original's day AND instant. Every day reader —
  `paid_by_due`, `ListDepositsOn`, `ListLoosePaymentsOn`, `IncomeLines`,
  `IncomeByMethod`, `DayChangedSince`, statement movements, receipts, correction
  snapshots — reads `paid_on`. The mapper refuses undated money (`KindInternal`).
  "No future date" compares days: a picked day after `Today()` is refused.
  New code that writes a payment or deposit MUST call `WithPaidOn`.
- **Cycle-wide charge reads are keyset-paged (T14).** `ListCharges` and
  `ListOverdue` RPCs take `page_token`/`page_size` (default and max 500) and
  answer `next_page_token`, an opaque `(due_date, id)` cursor
  (`adapters/grpc/page_token.go`); a token the service did not mint is
  InvalidArgument. The overdue cursor is the last row EXAMINED: the still-owed
  filter runs after the page is read, so a page may be empty while more remain.
  Walk until the cursor is zero (`walkPages`), never stop on a short page. The
  repo's unpaged `ListCharges` (capped at 500) is for per-pupil reads only.
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
- **A line already issued is recognised by WHAT it is, not only by its ref**
  (T1, `usecase/line_identity.go`). Inscription and fee refs are built from the
  plan's shape and from `Slugify(name)`, which drops accented letters; converting
  a single-amount plan to concepts, or re-spelling a line, minted a second ref and
  billed the line twice. Before inserting, the run and the enrolment consumer read
  `ListIssuedLines` and issue a line under the ref it ALREADY has when its
  `domain.LineKey` (name without accents/case/punctuation, from the charge's
  description; plus the month for fees) matches. Issued refs are never rewritten.
  Cancelled lines are ignored (they owe nothing). When a line exists in both
  shapes the ref of the wanted line's own shape wins. A single-amount inscription
  is NOT priced by any concept's ladder — neither by the run nor at the counter.
  `TuitionPlan.SharedLineName` refuses, on save only, two concepts that reduce to
  one key, or two such fees sharing a month.
- **Inscription window (T5, migration 0020).** `tuition_plans.enrollment_starts_on`
  + `enrollment_due_on` are the inscription's range; `charges.starts_on` is the
  day each charge opens. ONE rule, `domain.ValidateChargeWindow` (start needs a
  deadline, start ≤ deadline), read by the plan and by `Charge.WithStartsOn`, and
  mirrored by two CHECKs. Saving a plan re-dates the level's inscriptions still
  owing (untouched or part paid) at once via `RescheduleUnpaid`; paid ones are
  never touched; surcharge/early reward re-evaluate on read. A charge not yet
  open: hidden from `GetFamilyAccount` (unless it already has money on it) and
  listed but not asked for in `expectedFrom`. Fees and tuition months keep their
  due-day rule (no window).
- Surcharges are separate charges and are **excluded from their own input**: a
  fee that earned fees would turn a fixed monthly charge into compound interest.
- A level with no table is **reported by name**, never billed at zero.
- `CreateChargesBulk` splits its batch by whether the charge carries a ref: the
  two kinds are deduplicated by DIFFERENT partial indexes, and ON CONFLICT takes
  one arbiter. Naming only the keyless one made the second run of every month
  fail outright.

## T14 — alumnos migrados (ADR 0033, enmienda 2026-10-17)
- An imported enrolment (`origin = import`) is dated the cycle's first day, so
  tuition and fees run from September; no inscription (unchanged).
- **No surcharge on what fell due before the import.** `EnrollmentRef.ImportedOn`
  (students' `imported_at`, turned into the Morelia day by
  `schoolDayEnrollments`) is the day the pupil arrived. ONE rule,
  `CollectionPolicy.SurchargeFree(dueOn)` (strictly before that day), read by
  `domain.Surcharge` (daily) and `accrueLateFees` (monthly fixed fee). The grace
  is attached in ONE place, `pricedFor(plan, enrollment)`, used by `planFor`
  (statement, family account, counter quote, deposit, `policyFor`) and by the
  run's `planOfStudent`. It is per pupil and never stored with the plan. Such a
  month earns no early-payment reward either (it was not paid in time). An
  imported enrolment without `imported_at` keeps the normal rules.

## T17 — actividades (ADR 0031)
- `Activity` (schedule descriptive, periodicity `monthly|weekly|per_cycle`,
  first billing date + first due date repeated per period, fixed late fee,
  active) + `PriceOption` (days per week 1–7, or 0/NULL = flat fee) +
  `ActivityEnrollment` (option per pupil, `ends_on` = first day not enrolled).
  Tables `activities`, `activity_price_options`, `activity_enrollments` (0025).
- `RunBilling` issues `activity:<activity>:<student>:<period>` once the billing
  date arrives (`starts_on` = billing date), concept kind `activity`; enrolling
  issues what is already billable at once. Joined after a period's due date ⇒
  that period is still charged in full, due the day they joined (D3, plan T6); unenrolled ⇒ no period billed on/after `ends_on`.
- The fixed late fee is written once by the run (`latefee:<charge>:actividad`);
  the tuition's daily/monthly surcharge rules SKIP `activity:` refs
  (`accruedSurcharges`, `deriveSurcharge`, `accrueLateFees`).

## T18 — becas (ADR 0022)
- `Scholarship` (kind `percent|fixed_off|fixed_price`, value bp|cents, targets
  `enrollment|tuition|concept:<id>|activity:<id>`, active) +
  `ScholarshipHolding` (pupil, starts/ends). Tables `scholarships`,
  `scholarship_students` (0026); `charges.scholarship_cents|scholarship_id`,
  CHECK discount + scholarship ≤ amount. `Effective() = amount − discount −
  scholarship`.
- ONE rule, `domain.ApplyScholarship`, called by every path that prices a debt
  through `usecase.scholarshipPricer`: base → discount on the charge (level) →
  best scholarship (no stacking) → pronto pago (unchanged eligibility). Deadline
  = the charge's due date; after it only if `PaidByDueCents` ≥ scholarship price,
  else full price + normal surcharge. Part paid: `owed = max(0, price − paid)`.
- Read-time only, until settled: the deposit writes the part on charges its
  money settles (`DepositWrite.Scholarships`); unassign/edit first freezes what
  was settled (`FreezeSettledScholarships`); `releaseDeposit` takes the part back
  when the charge is no longer settled (T9 corrections re-price on the corrected
  date). The ladder never re-prices a charge with a written part.
- `ListCharges`, `ListOverdue`, the monthly late-fee run, the statement
  (`scholarship` movement) and receipts (`ScholarshipCents/Name`) all price it.

## Events
- **Consumed: `StudentEnrolled` and `StudentWithdrawn`**
  (student-events → `finance-student-events`). Enrolment charges the inscription
  through its unique `external_ref`; withdrawal cancels future unpaid charges
  under the charge row lock and ends activity enrolments on the effective date
  (ADR 0034). Both paths are safe on redelivery. Pull loop locally;
  authenticated `POST /pubsub` under Cloud Run scale-to-zero.
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
CONTPAQi XML export (S02 — `ExportContpaqi`
is a stub returning Unimplemented); scheduled `DebtOverdue` emission;
monthly-recurring bulk; `received_by` cashier attribution + discount-reason audit
(needs cashier auth); refunds/reversing entries.

## Feature flag
`module_finance` — default OFF. Disabled ⇒ gateway returns 404 for finance routes.
