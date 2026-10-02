# ADR 0030: Calificaciones por periodo de evaluación (T15)

- Status: Accepted
- Date: 2026-10-01
- Context: T15 of the review sprint. Monthly capture (T6, ADR 0019) is
  replaced by capture per evaluation period of the pupil's GRADE (T14, ADR
  0029). Owner decisions (plan §7): grades are captured as WHOLE NUMBERS; only
  the averages the system computes carry decimals — exact, truncated to 2.
  `monthly_grades` holds test data only and must stay intact; production must
  be left with nothing harmful for the future.

## Decisions

### A grade is a whole number per (enrollment, subject, period)

- New table `grades/0008_period_grades.sql`: `period_grades(tenant, enrollment,
  subject, period, value numeric(5,2), captured_by, captured_by_name,
  timestamps)` with `CHECK (value = trunc(value))`, `CHECK (0 ≤ value ≤ 100)`,
  `UNIQUE (tenant, enrollment, subject, period)`, ENABLE + FORCE RLS
  (`tenant_isolation`). The value travels and is stored as exact decimal TEXT
  — never a float (the GORM model reads/writes `numeric` as string).
- **No FK to `evaluation_periods`**: another service, and T14's save rewrites a
  grade's rows keeping their ids (ADR 0029).
- The level's scale (`config/0009`) still bounds the value: `grade_min`,
  `grade_max`, `passing`. Its `grade_decimals` no longer applies to capture —
  `GradingScale.ParseWholeValue` refuses any fraction with `GRADE_NOT_INTEGER`
  ("Solo se permiten calificaciones enteras"), range first. A scale change
  (`CountScaleViolations`) now counts period grades by range only: a whole
  number can never break a decimals rule.
- The period must be one of the group's grade in the assignment's cycle
  (`PERIOD_NOT_IN_GRADE`) and must have STARTED on the school's local day
  (`PERIOD_NOT_STARTED`, "Aún no empieza ese periodo"). A finished period stays
  writable until its term closes.
- "Today" is the school's calendar day: grades now loads `SCHOOL_TIMEZONE`
  (default `America/Mexico_City`, `time/tzdata` embedded; an unknown zone fails
  startup), as attendance and comms do. The sheet opens on the period that
  contains today, else the latest that already started, else the first
  (`domain.DefaultPeriod`). grades reads the periods from config by RPC
  (`ListEvaluationPeriods`) and applies the same `PeriodOn` rule locally.
- Previous-group rule (ADR 0019), restated for periods: for a teacher, a
  period that ENDED before the pupil joined the group (local day of the latest
  transfer into it) belongs to the previous group — `PERIOD_PREVIOUS_GROUP`.
  The director (school-wide) may write it.

### Closing stays per level term; a term stands for the period of its ordinal

Option chosen: keep `period_closures` / `report_cards` keyed by `term_id`
(no table changes there) and map period ↔ term by **(cycle, level, ordinal)**,
the mapping ADR 0029 anticipated. Moving closures to period keys would have
meant closing each grade separately and a second closure table; the term is
already the unit the director closes and the families' report cards are
indexed by.

- Writes to period P (grade g, ordinal N) take the advisory lock of the
  level's term with ordinal N and are refused with `PERIOD_LOCKED` (409) while
  it is closed — the same transaction-scoped lock T7 built (ADR 0020). A period
  whose ordinal no term has is unguarded (as an unmapped month was).
- Progress / close of term N: expected = active enrollments × subjects taught
  in their group, ONE grade each for the period N of their grade; a pupil who
  joined the group after that period ended owes none. A group (with active
  pupils) whose grade has no period N blocks the close with 409
  `PERIOD_NOT_CONFIGURED` naming the groups; progress lists them
  (`groupsWithoutPeriod`). `TERM_WITHOUT_MONTHS` is gone; `unfinished` (any
  grade's period N not over on the local day) replaces `unfinishedMonths`.
- The closure's months snapshot (grades/0004) is no longer written (empty);
  term months no longer lock anything in grades.

### Report cards: one column per period up to the one closed

- A card for term N freezes the pupil's grade's periods 1…N as columns
  (`report_cards.periods` jsonb `[{id, ordinal, name, startsOn}]`) and each
  line's grade per period (`report_card_lines.period_values` jsonb). The card is
  named after the pupil's period N (each grade may name it).
- Line `average` = exact mean of the line's columns; `generalAverage` = exact
  mean of the subjects' EXACT averages; both stored truncated to 2 decimals
  (numeric(5,2) as text). One rule, one table of cases
  (`packages/domain/src/gradeAverage.vectors.json`) run by Go
  (`domain/average.go`) and TS (`@garcia-lorca/domain/grade-average`, used by
  the web and the parents' app). A re-close compares averages as they read
  (`Average.SameAs`), so 26/3 equals a stored 8.66.
- Cards issued before T15 keep `months` + `month_values` and their one-decimal
  averages, and read back exactly as issued (JSON, PDF and the app render
  months when `periods` is empty). Sorting a pupil's cards dates a T15 card by
  the start of its closed period.

### Monthly capture is retired — data kept, no path depends on it

- `monthly_grades` and its rows are untouched (no migration, no conversion).
  The four monthly RPCs are `deprecated` in the proto and answer
  `Unimplemented` (the handler embeds the generated Unimplemented server), so
  nothing can write them; the gateway routes `/grades/sheet`,
  `/grades/monthly`, `/teaching/grades/{sheet,monthly}` and
  `/family/students/{id}/grades/monthly` are gone (404). The RPCs will be
  removed from the contract in a later release (`buf breaking` is advisory).
- The level trash archives `period_grades` too (appended LAST to grades'
  steps, so batches archived before T15 keep their step positions).

### Config refuses removing a period that holds grades

Config must not depend on grades (ADR 0019), so the GATEWAY guards T14's
writes: before `UpsertEvaluationPeriods` it lists the grade's current periods
and asks grades `PeriodsWithGrades(removed ids)`; before
`CopyEvaluationPeriods` it computes the target periods the copy drops (ordinals
the source lacks). Any with grades → 409 `PERIOD_HAS_GRADES` "«name» ya tiene
calificaciones; no se puede quitar", and config is not called. A grades
failure refuses the change (never lets it through). As with the scale guard,
check and write are two calls.

## Consequences

- Deploy order: **config → students → grades (migration 0008) → gateway →
  web**; the parents' app needs a new EAS build (the old build's monthly route
  answers 404).
- Renumbering a grade's periods after a term closed would move a closed
  period's grades under another term's lock; config's save keeps ids, and the
  guard above stops removals, but reordering stays possible — a known edge,
  rare (periods are set at the start of the cycle).
