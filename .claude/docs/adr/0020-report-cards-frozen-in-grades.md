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
  subjects taught in each group × EVERY month of the term, reached or not,
  from the month each pupil joined their group (enrolment date, or the latest
  transfer into it; earlier months are not expected) — and what is missing per
  group and subject, with the teacher. So closing a term whose last month has
  not ended reports those months missing and needs `force`; the progress lists
  them as `unfinishedMonths` (after the current UTC month) so the screen warns
  "Aún no terminan sus meses".
- Report cards are issued only for ACTIVE enrollments. A withdrawn pupil gets
  none and is not expected in the progress.
- Missing captures refuse the close with 409 `MISSING_GRADES` "Faltan N
  calificaciones por capturar" unless `force: true`. Other refusals (409):
  `TERM_WITHOUT_MONTHS`, `ALREADY_CLOSED`, `LEVEL_NOT_GRADED`, `ALREADY_OPEN`.
- Closed means NOBODY writes a grade of the term's months — teacher or
  director: 409 `MONTH_LOCKED` "El periodo está cerrado". The sheet lists those
  months in `closedMonths` and marks them read-only. The legacy per-term
  capture (`CaptureGrade`, grade_entries) honours the same lock.
- The close SNAPSHOTS the term's months (`period_closures.months`, migration
  `grades/0004`). A closed term locks the UNION of that snapshot and its
  current config mapping, so moving a month out of (or into) a closed term in
  config never unlocks it. The gateway also refuses to change a closed term's
  months — PUT `/config/terms/{id}/months` and `copy-months` ask grades
  (`ClosedTerms`) first and answer 409 `TERM_CLOSED` "El periodo está cerrado;
  reábrelo para cambiar sus meses".
- A term covers at most six months (config, 400 `TERM_TOO_LONG`): the PDF
  prints one column per month, landscape above four.
- Reopening keeps the report cards published. Closing again issues version
  `closure.version + 1` for every active enrollment and marks EVERY published
  card of the term `superseded` first — including the card of a pupil
  withdrawn between the two closes, who gets no new version: a re-close
  replaces the term's report cards as a whole (a partial unique index keeps one
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

### Unmapped months stay capturable (revised)

The first cut hid months no term maps. Reversed: the monthly sheet lists every
month of the cycle again and an unmapped month stays capturable; the sheet
names them in `unmappedMonths` so the screen marks them "Sin periodo" (no
report card prints them). Only a closed term's months lock.

### Notification and PDF

- `GradesPublished` (one per report card, on `grades-events`) rides a
  transactional OUTBOX (`grades_outbox`, migration `grades/0005`, RLS like
  `finance_outbox`): the events are written in the close's own transaction,
  drained right after the commit on a detached, 2-second-bounded context, and
  swept every 30 s. A refused publish counts the attempt and backs off (30 s,
  doubling, capped at 30 min); rows are taken `FOR UPDATE SKIP LOCKED`.
  Delivery is at least once — comms dedupes on report card id + version. A
  broker outage delays a notification, never loses it or fails the close.
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
- Closing reads every grade of the level in one query and names pupils in
  pages of 200: a level of a few hundred pupils is a handful of calls.

## Deploy order

**config → students → grades (migrations 0003–0005) → comms → gateway.**
Infrastructure: the `grades-events` topic, its DLQ and the
`comms-grades-events` push subscription must be added to
`deploy/terraform/pubsub.tf`, and the grades service needs the Pub/Sub
environment (as attendance has) in compose and Cloud Run; without it grades
falls back to the logging dev bus and no family is notified.
