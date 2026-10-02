# ADR 0023: Horarios — el horario escolar vive en config (A2)

- Status: Accepted
- Date: 2026-10-02
- Context: T16 of the review sprint (plan §1 A2, §T16, §7 answers 11 and 14).
  The school wants its weekly timetable in the system (Monday–Friday, by
  level/grade/group and by teacher), a "¿Qué está pasando ahora?" view, and a
  way to change ONE day without touching the base ("the music teacher is late;
  another free teacher takes that block today").

## Decision

- **config owns the timetable.** It already owns subjects, groups and teaching
  assignments (T13), so "a class is an existing assignment" and "nobody is
  double-booked" are local constraints, not RPCs. Migration `config/0017`:
  `schedule_settings`, `schedule_blocks`, `schedule_overrides`,
  `schedule_events`, all with RLS (ENABLE + FORCE + tenant_isolation) and the
  level-deletion freeze; the level trash archives and restores them.
- **A class is an assignment placed in time.** `schedule_blocks` carries the
  assignment and its (group, subject, teacher), tied by a composite FK to
  `teaching_assignments (id, tenant, cycle, group, subject, teacher)` —
  `ON UPDATE CASCADE` (reassigning the teacher moves the classes to them) and
  `ON DELETE CASCADE` (freeing the slot removes its classes). Times are minutes
  since midnight in 5-minute steps, half-open `[start, end)` so back-to-back
  classes do not collide; weekday 1–5.
- **No double-booking, enforced by the database on the base.** Two
  `EXCLUDE USING gist` (btree_gist, enabled by 0016): `(tenant, cycle, teacher,
  weekday, int4range &&)` and `(tenant, cycle, group, weekday, int4range &&)`.
  The use case checks first so the refusal names the conflicting class
  (`SCHEDULE_TEACHER_BUSY` "Ana Ruiz ya da Matemáticas a 3.º A el lunes de
  08:00 a 08:50", `SCHEDULE_GROUP_BUSY`); a race the database catches is
  `SCHEDULE_CHANGED`. Reassigning an assignment to a teacher who is busy at its
  class times trips the EXCLUDE through the cascade and is refused with
  `SCHEDULE_TEACHER_BUSY` (teaching load screen).
- **Date-specific changes never touch the base.** `schedule_overrides(block,
  on_date, kind substitute|swap|cancel, teacher, swap_block, reason, by)`; one
  change per class and date (unique, also for a swap's other class). A swap
  trades the times of two classes of the same group that day. The rules on the
  EFFECTIVE timetable of a date (base + its changes) are computed, so they
  cannot be an EXCLUDE: every timetable write of a cycle runs under one
  transaction-scoped advisory lock (`pg_advisory_xact_lock(hashtextextended(
  'config.schedule:' || cycle))`) and checks the whole day. Undoing a change is
  checked too (the replaced teacher may have covered another class meanwhile),
  and a base edit is checked against every date ahead that has changes
  (`SCHEDULE_OVERRIDE_CONFLICT`); moving a class with changes ahead to another
  weekday or assignment is refused (`SCHEDULE_BLOCK_HAS_OVERRIDES`).
- **The school's "today".** config now reads `SCHOOL_TIMEZONE` (default
  America/Mexico_City, tzdata embedded; an unknown zone fails startup), like
  comms/attendance/grades. The base is edited freely before the cycle's first
  day; from then on a base edit needs a reason and is written to
  `schedule_events`. Every override (and its undo) is always written there.
- **Substitutes (answer 11).** Any teacher with no class (base or cover) in the
  slot that day; those who teach that subject (same name, any grade) first,
  then those who teach in that grade, then the rest, by name.
- **Grid (answer 11).** Per level, `schedule_settings(grid_start, grid_end)`;
  no row = 07:00–15:00. A class must fit its level's grid when written.
- **Edge.** Gateway routes `/api/v1/schedule*` behind `module_config` AND the
  new `module_schedule` (default OFF; ON in local compose and in the dev
  terraform per answer 14), `schedule:read` / `schedule:write` (iam 0028,
  owner by default, delegable). `GET /api/v1/schedule` returns the catalog the
  calendar needs, so `schedule:read` without `config:read` draws the screen.
  Times travel as "HH:MM". The teacher's own timetable is
  `GET /api/v1/teaching/schedule[/day]` (`teaching:read`), the teacher taken
  from the principal; a caller with no teacher record gets an empty timetable.

## Consequences

- Freeing a teaching assignment silently removes its classes (and their
  changes) from the timetable; the history does not record it (the cascade is
  the database's). Acceptable: the class has no teacher any more.
- Non-working days (school calendar) are not consulted: a change can be
  recorded on a holiday. The "now" view is driven by the date picked.
- A level's grid is not re-checked when changed; the calendar widens itself
  to show any class outside it.
- Flags gate only the gateway (memory `feature-flags-env-not-db`); the
  timetable has no background work, so OFF hides it completely.
