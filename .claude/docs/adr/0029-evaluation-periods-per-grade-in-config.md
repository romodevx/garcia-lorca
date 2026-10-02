# ADR 0029: Periodos de evaluación por grado, en config (A3)

- Status: Accepted
- Date: 2026-10-01
- Context: T14 of the review sprint (plan §1 A3, §T14). Grades stop being
  captured month by month (T15) and are captured per evaluation period. Until
  now a "period" was a `term` per (cycle, level), mapped to months
  (`term_months`, 0010). The school wants each grade of a level to keep its own
  calendar, with real start and end dates.

## Decision

- **`config` owns the periods; `grades` asks for them by RPC** (as it already
  asks for `term_months`). New table `evaluation_periods(tenant, cycle,
  grade_level, ordinal, name, starts_on, ends_on, term_id)` — migration
  `config/0016` — with RLS (ENABLE + FORCE + tenant_isolation) and the
  level-deletion freeze trigger.
- **No overlap, enforced by the database.** `EXCLUDE USING gist (tenant_id =,
  cycle_id =, grade_level_id =, daterange(starts_on, ends_on, '[]') &&)` with
  `btree_gist` (standard contrib: Cloud SQL, Neon and `postgres:16`), plus
  `UNIQUE (tenant, cycle, grade, ordinal)` and `CHECK (starts_on <= ends_on)`.
  Two editors racing cannot leave two periods sharing a day. "Inside the cycle"
  is checked by the use case (cycle dates are not race-sensitive).
- **Neither constraint is DEFERRABLE.** The level trash restores rows with
  `ON CONFLICT DO NOTHING`, which refuses deferrable arbiters. To rewrite a
  grade's set — shifting every period passes through overlapping states — the
  repository deletes the grade's rows and inserts the new set with the SAME ids
  (created_at carried over), in one transaction.
- **A grade's set is replaced as a whole; ids follow the row.** Each input row
  may carry the id of the grade's period it edits; that period keeps its id
  (T15's grades will point at it) even when renumbered — removing the 1st
  trimester removes IT. Rows without a known id are new periods; periods no
  row names are removed. The web numbers periods 1…n in date order on save.
  Two saves of one grade racing: the later trips the primary key and gets 409
  `PERIODS_CHANGED`. Copy matches the target's periods by ordinal (a copy has
  no rows of its own) and replaces each target grade, in ONE transaction; "Aplicar a todo el nivel" is a copy onto the level's other
  grades. The web offers Deshacer (8 s) by re-saving the targets' previous sets.
- **RPCs** `ListEvaluationPeriods` / `UpsertEvaluationPeriods` /
  `CopyEvaluationPeriods`; refusal codes `PERIOD_OVERLAP` (409),
  `PERIOD_OUTSIDE_CYCLE`, `PERIOD_ENDS_BEFORE_START`, `PERIOD_ORDINAL_TAKEN`,
  `TOO_MANY_PERIODS` (400). Gateway: reads `config:read` (teachers will need
  read in T15 through grades' own RPC, not this route), writes `config:write`,
  all behind `module_config` (no new flag).
- **Domain function for T15:** `domain.PeriodOn(periods, grade, date)` and
  `EvaluationPeriodService.EvaluationPeriodOn(ctx, cycle, grade, date)` — the
  period containing the calendar date (pass the school's LOCAL date; see memory
  `school-day-is-utc-day`).
- **Data migration, additive.** For each `term` (+ its `term_months`) every
  grade of the term's level gets the same period: first day of the first month
  to last day of the last month, clamped to the cycle; a term without months
  uses its own dates. In ordinal order; one that would overlap a period already
  derived for that grade is skipped with a NOTICE (interleaved months). Terms
  and term_months are NOT deleted.

## How T15 relates periods to closures and report cards

`period_closures` and `report_cards` (grades) are keyed by `term_id` (per cycle
+ level). Until T15 decides otherwise, a grade's period maps to the level term
with the same `(cycle, level of the grade, ordinal)`; `evaluation_periods.term_id`
records that link for migrated rows (provenance, `ON DELETE SET NULL`). T15
should either keep closing per term through that mapping, or move closures to
`(period_id)` — and, if periods may be deleted once grades exist, add a guard
(today a period of a grade can be removed freely: nothing references it yet).

## Consequences

- Two sources of dates exist until T15 retires monthly capture: `terms`
  (+months, used by the monthly sheet and closures) and `evaluation_periods`.
  The Configuración screen keeps both: the level's trimesters (months) and the
  new page "Periodos por grado".
- A new grade added later starts with no periods; the school copies them from
  another grade in one click.
