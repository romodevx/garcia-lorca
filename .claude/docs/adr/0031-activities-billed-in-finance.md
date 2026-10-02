# ADR 0031: Actividades — en finance, cobradas por periodo por el mismo run diario

- Status: Accepted
- Date: 2026-10-02
- Context: T17 of the review sprint (plan §T17, §1 A1/A5, §7 answer 14). The
  school runs extracurricular activities (natación, música, robótica…) with
  their own weekly schedule and price, charged on top of the tuition. Nothing in
  the ledger knew about them: the office billed them by hand and the parents'
  app never showed them.

## Decision

- **Activities live in finance** (A1, finance/0025). Their only business rule is
  the charge, and putting them elsewhere would make every debt read call another
  service. Three tables, RLS ENABLE + FORCE + tenant_isolation:
  `activities` (name unique per school, schedule JSONB `[{weekday 1–7, start,
  end}]` — descriptive only, periodicity `monthly|weekly|per_cycle`, first
  billing date, first due date, fixed late fee, active),
  `activity_price_options` (`days_per_week` 1–7, or NULL = flat fee; amount per
  period; retired, never deleted, so an ended enrolment keeps its option) and
  `activity_enrollments` (pupil, option, `starts_on`, `ends_on` = first day no
  longer enrolled; one open enrolment per pupil and activity; composite FK so
  the option belongs to the SAME activity).
- **Periods repeat the two dates the school types.** Monthly: same day of the
  month (clamped to the month's end), keyed `YYYY-MM`. Weekly: +7 days, keyed by
  ISO week `YYYY-Www`. Per cycle: one charge, keyed by its billing date. Keys are
  coarse on purpose, so moving the billing day inside its period re-issues
  nothing. The due date must fall inside the first period. (The plan sketched
  `bill_day/due_day`; two dates express every periodicity with one rule.)
- **Charges are ordinary charges** under a new system concept kind `activity`,
  ref `activity:<activityId>:<studentId>:<period>` — ids, never names, so a
  rename (or an accent fixed, T1) re-issues nothing. Amount = the pupil's option
  at issue time (a later price change or option change applies to later
  periods only). `starts_on` = the billing date, `due_date` = the due date moved
  off a closed day. They are issued only once the billing date arrives, so the
  statement (T7), the counter (Registrar pago, receipts) and the parents' app
  show them as pending from that day, and several activities simply add up.
- **Mid-period enrolment**: the period in course is charged when the pupil
  joined on or before its due date; otherwise billing starts with the next one.
  No proration. **Unenrolling** stops periods billed on or after `ends_on`;
  issued charges, paid or not, are never touched. A pupil is billed in the cycle
  they are actively enrolled in (a charge needs a cycle); one not yet placed in
  a group is billed by the run of the cycle that takes them.
- **Late fee: fixed, once, per activity** ($0 = none). The billing run writes it
  the day after the due date while the period still owes
  (`latefee:<chargeId>:actividad`), so every read path (counter quote, deposit
  split, statement, receipts) treats it as the written-down surcharge riding on
  its period, exactly as the tuition's monthly-mode surcharge. The tuition
  table's surcharge rules (daily accrual at read time, or monthly fees) never
  apply to activity charges: `accruedSurcharges`, `deriveSurcharge` and
  `accrueLateFees` skip `activity:` refs. Inactive activities still earn the fee
  on what they already issued.
- **One run, daily.** `RunBilling(cycle)` also issues activity periods; it was
  always idempotent, so the Cloud Scheduler job now fires every day at 06:00
  school time (`billing_schedule_cron`, repo only, no apply) to cover weekly
  activities and arbitrary billing days. Enrolling also issues, at once, what is
  already billable — the same refs the run would write. The panel's existing
  "Generar cargos" button (`POST /api/v1/finance/billing-runs`) is the local and
  manual trigger.
- **Access**: `module_activities` (default OFF; ON in compose and dev
  ENABLED_MODULES) behind `module_finance`; `activities:read|write` (iam 0029,
  owner by default, delegable). Turning the flag off does not stop the run; an
  activity is stopped by marking it inactive.
- **Web** (A5): the enrolment screen is the shared
  `apps/web/app/_design/students/DualPanelPicker.tsx`, generic over "items
  assigned to a target", which T18 (becas) reuses.

## Consequences

- No new money path: the ledger, the deposit split and the receipts handle
  activity charges with no special case beyond "no tuition surcharge".
- A fee appears the morning after the due date (the run), not at the instant the
  date passes. Same convention as the monthly tuition surcharge; the day is the
  UTC day like the rest of finance (school-day-is-utc-day).
- Editing an activity's first billing date to another month/week re-keys future
  periods; already issued periods keep their refs.
