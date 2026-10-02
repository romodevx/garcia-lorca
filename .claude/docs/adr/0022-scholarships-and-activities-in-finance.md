# ADR 0022: Becas (y actividades) viven en finance; la beca se calcula al leer y al cobrar

- Status: Accepted
- Date: 2026-10-02
- Context: plan §1 A1 (reserved this number), T18 of the review sprint, owner
  answers 12, 13 and 14 (§7). The school grants some families a standing
  reduction on part of what it charges. It must reach, with ONE computation,
  every place a debt is priced: Registrar Pago (the counter's quote and the
  deposit), the parents' app, the account statement and the receipts.
  `services/scholarships` was an empty FASE 0 stub (health endpoint, proto with
  TODOs, no data, no traffic). Activities (T17, ADR 0031) already took the
  "in finance" half of A1.

## Decision

### Where

- **Becas live in finance** (migration finance/0026). A scholarship in another
  service would cost an RPC on every debt read and leave two truths about what a
  family owes. The stub service, its proto package, its go.work entry, its
  compose service and its CI shard weight are retired; nothing depended on them.
  The flag key `module_scholarships` is kept and now gates the finance-backed
  module (gateway routes `/api/v1/scholarships*`, behind `module_finance` too).
- Tables (RLS ENABLE + FORCE + tenant_isolation): `scholarships` (name unique
  per school ignoring case — and accents, in the use case —, `kind`
  `percent|fixed_off|fixed_price`, `value` = basis points for a percent
  (1–10000), cents otherwise, `targets` JSONB `["enrollment" | "tuition" |
  "concept:<id>" | "activity:<id>"]`, `active`) and `scholarship_students`
  (pupil, `starts_on`, `ends_on` = first day no longer held; one open row per
  pupil and scholarship). Scholarships are never deleted.
- `charges.scholarship_cents` + `scholarship_id`, apart from `discount_cents`:
  a month carries the level's discount AND the scholarship on what is left.
  CHECK `discount_cents + scholarship_cents <= amount_cents`.
- Targets: the inscription (every line of the enrolment concept), the monthly
  tuition, a catalog concept by id (the school's own concepts, and the one
  concept every plan fee hangs from — so "fees" is one target), an activity by
  id (read from the charge's `activity:` ref). Surcharges are never a target.
  The screen lists what exists today (`GET /scholarships/targets`).

### The rule — `domain.ApplyScholarship`, one function

Every path that prices a debt calls it: `expectedFrom` (the counter's quote,
the app's account), the deposit planner (and its preview, and T9 corrections),
the statement, `ListCharges`, `ListOverdue` and the monthly late-fee run.

1. **Order** (owner, answer 13): base price → the discount already on the
   charge (the level's `monthly_discount_cents`, most often) → **scholarship**
   on that price → early payment (pronto pago) on what is left, if eligible.
   Eligibility of pronto pago is unchanged: it still applies only to a tuition
   month with no discount written on it — so a level-discounted month gets
   level + scholarship, no pronto pago, exactly as before T18.
2. **Kinds.** percent: `round_half_up(base × bp / 10000)` (the half centavo
   goes to the family); fixed_off: `min(value, base)`; fixed_price:
   `max(0, base − value)` (a price above the base never raises it).
3. **Several scholarships on one charge do not stack**: the one that takes the
   most off applies (ties: lowest id). The assignment screen lists, per pupil,
   the other active scholarships sharing a target ("no se acumulan").
4. **Deadline = the charge's own due date** (owner, answer 12), the whole day,
   UTC like every finance date (school-day-is-utc-day). Up to it the
   scholarship applies. After it, it applies only if what was paid **by** the
   due date already covers the scholarship price (`paid_by_due`, summed per
   charge by the repository) — the charge was settled in time. Otherwise it is
   lost: full price, and the surcharge (daily or monthly, the plan's) runs on
   the full price like for anyone late. A 100 % month needs no money to be in
   time: it is never late.
5. **Computed at read/payment time, written when settled.** A scholarship is
   never written at issue. Assigning one therefore reaches every pending charge
   at once; taking it away (or deactivating, or editing it) gives pending
   charges their price back. When a **deposit settles** a charge priced with a
   scholarship (money reached it and covered what it owed), the part is written
   on the charge (`DepositWrite.Scholarships`): from then on it is history that
   no later change to the scholarship rewrites. Before a scholarship is taken
   away or edited, `FreezeSettledScholarships` writes the part of every charge
   it already settled — by money at any time, or by itself (owing nothing) once
   its due date has come — so a past free month is not re-billed at full price.
6. **Paid charges never change**: a charge already `paid` is never re-priced;
   a frozen part is never recomputed.
7. **Partly paid** ("la beca aplica al saldo restante"): the scholarship price
   is what the charge costs in all; money already on it counts toward it and is
   never refunded, so the part taken off is capped at what is still unpaid:
   `owed = max(0, scholarship_price − paid)`. Examples, a $3,000 month and a
   50 % scholarship:
   - $1,000 paid (before the scholarship existed or after, the same): the
     scholarship price is $1,500, $500 is left to pay by the due date.
   - $2,000 paid: nothing is left; the scholarship takes only the $1,000 unpaid
     (nothing is refunded).
   - $1,000 paid by the due date and the rest after it: the scholarship is lost
     (it was not settled in time): $2,000 + the surcharge are owed.
   One formula whether the money came before or after the assignment, so the
   order of events never changes what a family pays.
8. **T9 corrections and voids** re-evaluate: releasing a deposit takes the
   written part back when what is left no longer settles the charge
   (`releaseDeposit`); a correction then plans the corrected deposit on the
   **corrected date** through the same planner, so the rule decides again
   (paid late on correction ⇒ lost + surcharge).

### Where it shows

- Registrar Pago: each expected line carries `scholarshipCents` (already out of
  `owedCents`), and the quote its total.
- Statement (ADR 0024): a `scholarship` movement "Beca <name> · <charge>" per
  charge it applies to (contingent ones included, like the level discount line);
  the balance and `dueToday` are the counter's.
- Receipts (JSON + PDF): per concept "Beca <name> $X" on the payment that settled
  it, and a "Becas aplicadas" total.
- Parents' app: reads the statement (`scholarship` kind was already rendered).

## Consequences

- No new money path: the ledger, the deposit split, the receipts and the
  statement handle a scholarship as one more part of a charge.
- **Turning `module_scholarships` off hides the screens; scholarships already
  assigned keep pricing charges** (finance does not read flags, see
  feature-flags-env-not-db). Stopping one is taking it away or deactivating it.
- Loose payments (paid straight against a charge, e.g. a uniforms counter sale)
  do not apply scholarships: their path vetoes over-payment against the stored
  price. Becas are collected through Registrar pago (deposits).
- A free (100 %) month is `pending` in the database until frozen (as a full
  discount always was — "stays pending until money lands"); every reader prices
  it at zero and the overdue views and the late-fee run skip it.
- Pupils' becas are read per request (one query for the holdings, one for the
  scholarships) on every priced read; bounded (one school).
