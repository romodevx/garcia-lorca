# ADR 0020: Boletas — foto fija en grades, cierre transaccional, redondeo

- Status: Accepted
- Date: 2026-09-27
- Context: T7 part B (cierre de periodos y boletas). The owner approved the
  rules; this records them and how they are built. Part A (config term months,
  `grades:close` in iam 0023) is already in place.

## Decisions

### A period is a config term; closing it lives in grades

- A period is one config term: exactly one level of one cycle, covering the
  months configured for it (`config.term_months`).
- `period_closures`, `report_cards` and `report_card_lines` live in the grades
  database (migration `grades/0003`, ENABLE+FORCE RLS on all three). Closing
  writes the closure and every report card in ONE grades transaction. There is
  no cross-service commit: everything the snapshot needs from other services
  (the level's catalog from config, the cycle's enrollments and the pupils'
  names and matrículas from students) is read BEFORE the transaction; if any
  read fails, nothing is written.
- A report card is a **frozen snapshot**: names (pupil, group, level, cycle,
  term), the term's months, the exact monthly grades (as decimal strings), the
  averages and the level's scale are copied at issue time. A later rename or a
  scale change never alters an issued report card.

### Averages: exact, half-up, one decimal

- A subject's trimester grade is the arithmetic mean of its CAPTURED monthly
  grades in the term's months; a month without a grade is ignored, and a
  subject with no grade has no trimester grade (never a zero).
- The general average is the mean of the subjects' trimester grades (only
  subjects with one).
- Both are rounded HALF-UP to one decimal with integer arithmetic on exact
  hundredths: `floor((2n + d) / 2d)`. 8.45 → 8.5, 8.25 → 8.3. No float takes
  part (a float64 reads 8.45 as 8.4499…).

### Closing, reopening, versions

- Only `grades:close` (owner-only) closes or reopens. Before closing, the
  progress says what is expected — active enrollments of the level's groups ×
  subjects taught in each group × the term's months up to the current UTC
  month — and what is missing per group and subject, with the teacher.
- Missing captures refuse the close with 409 `MISSING_GRADES` "Faltan N
  calificaciones por capturar" unless `force: true`. Other refusals (409):
  `TERM_WITHOUT_MONTHS`, `ALREADY_CLOSED`, `LEVEL_NOT_GRADED`, `ALREADY_OPEN`.
- Closed means NOBODY writes a grade of the term's months — teacher or
  director: 409 `MONTH_LOCKED` "El periodo está cerrado". The sheet lists those
  months in `closedMonths` and marks them read-only.
- Reopening keeps the report cards published. Closing again issues version
  `closure.version + 1` for every active enrollment and marks the enrollment's
  previous published card `superseded` (a partial unique index keeps one
  published card per enrollment and term). Families see only the published
  one; a superseded card is 404 on the family routes.

### No write slips between the snapshot and the closure

Checking "is the period closed?" in the use case alone is a race: a grade
written after the close read the grades but before it committed would change a
closed period. Writers and the closer therefore serialise on a
transaction-scoped advisory lock keyed by tenant and term: a grade write takes
it SHARED and re-reads the closure inside its own transaction; a close takes it
EXCLUSIVE and reads the grades under it. The lock needs no row, so a term never
closed is covered too. An integration test proves a write that arrives during a
close waits and is refused.

### Once a level maps months, only those months exist

When a level has any month mapped to a term, the monthly sheet lists (and a
write accepts) only mapped months; a level without mappings keeps every month
of the cycle, as before T7.

### Notification and PDF

- After the commit, grades publishes `GradesPublished` per report card on
  `grades-events` (best-effort and logged, like the other publishers: the cards
  are already committed, so a failed publish never fails the close).
- comms consumes it (subscription `comms-grades-events`), keyed
  `GradesPublished:<report_card_id>:<version>`: v1 "La boleta del <periodo> de
  <Nombre> ya está disponible."; v>1 "Boleta actualizada: <periodo> de
  <Nombre>." Push data `{type: report_card, studentId, reportCardId}`.
- The PDF is the school's internal format (not SEP), rendered in the gateway
  (`internal/reportcard`) like receipts (ADR 0017). The embedded Poppins faces
  moved to `internal/pdffont`, shared by receipts and report cards.

## Consequences

- The school day is the UTC day (see the `school-day-is-utc-day` note): the
  "months up to the current month" cut and the issue date follow UTC.
- A best-effort publish can lose a notification if the broker is down at
  close time; the report card itself is durable and visible in the app. An
  outbox (as finance has) is the upgrade path if that proves to matter.
- Closing reads every grade of the level in one query and names pupils in
  pages of 200: a level of a few hundred pupils is a handful of calls.

## Deploy order

**config → students → grades (migration 0003) → comms → gateway.**
Infrastructure: the `grades-events` topic, its DLQ and the
`comms-grades-events` push subscription must be added to
`deploy/terraform/pubsub.tf`, and the grades service needs the Pub/Sub
environment (as attendance has) in compose and Cloud Run; without it grades
falls back to the logging dev bus and no family is notified.
